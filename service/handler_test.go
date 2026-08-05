// Copyright (c) TFG Co. All Rights Reserved.
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

package service

import (
	encjson "encoding/json"
	"errors"
	"sync"
	"testing"

	agentmocks "github.com/actfuns/pitaya/v2/agent/mocks"
	"github.com/actfuns/pitaya/v2/cluster"
	"github.com/actfuns/pitaya/v2/conn/codec"
	"github.com/actfuns/pitaya/v2/conn/packet"
	"github.com/actfuns/pitaya/v2/constants"
	"github.com/actfuns/pitaya/v2/metrics"
	metricsmocks "github.com/actfuns/pitaya/v2/metrics/mocks"
	connmock "github.com/actfuns/pitaya/v2/mocks"
	"github.com/actfuns/pitaya/v2/pipeline"
	"github.com/actfuns/pitaya/v2/serialize/json"
	serializemocks "github.com/actfuns/pitaya/v2/serialize/mocks"
	"github.com/actfuns/pitaya/v2/session"
	"github.com/actfuns/pitaya/v2/session/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

type mockAddr struct{}

func (m *mockAddr) Network() string { return "" }
func (m *mockAddr) String() string  { return "remote-string" }

func TestNewHandlerService(t *testing.T) {
	packetDecoder := codec.NewPomeloPacketDecoder()
	serializer := json.NewSerializer()
	sv := &cluster.Server{}
	remoteSvc := &RemoteService{}
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockMetricsReporter := metricsmocks.NewMockReporter(ctrl)
	mockMetricsReporters := []metrics.Reporter{mockMetricsReporter}
	mockAgentFactory := agentmocks.NewMockAgentFactory(ctrl)
	handlerHooks := pipeline.NewHandlerHooks()
	handlerPool := NewHandlerPool()
	taskSvc, _ := NewTaskService(1000, 10, 5)
	svc := NewHandlerService(
		packetDecoder,
		serializer,
		sv,
		nil,
		remoteSvc,
		taskSvc,
		mockAgentFactory,
		mockMetricsReporters,
		handlerHooks,
		handlerPool,
	)

	assert.NotNil(t, svc)
	assert.Equal(t, packetDecoder, svc.decoder)
	assert.Equal(t, serializer, svc.serializer)
	assert.Equal(t, mockMetricsReporters, svc.metricsReporters)
	assert.Equal(t, sv, svc.server)
	assert.Equal(t, remoteSvc, svc.remoteService)
	assert.Equal(t, mockAgentFactory, svc.agentFactory)
	assert.Equal(t, handlerHooks, svc.handlerHooks)
	assert.Equal(t, handlerPool, svc.handlerPool)
}

func TestHandlerServiceProcessPacketHandshake(t *testing.T) {
	tables := []struct {
		name         string
		packet       *packet.Packet
		socketStatus int32
		validator    func(data *session.HandshakeData) error
		errStr       string
	}{
		{"invalid_handshake_data", &packet.Packet{Type: packet.Handshake, Data: []byte("asiodjasd")}, constants.StatusClosed, nil, "invalid handshake data"},
		{"validator_error", &packet.Packet{Type: packet.Handshake, Data: []byte(`{"sys":{"platform":"mac"}}`)}, constants.StatusClosed, func(data *session.HandshakeData) error { return errors.New("validation failed") }, "handshake validation failed"},
		{"valid_handshake_data", &packet.Packet{Type: packet.Handshake, Data: []byte(`{"sys":{"platform":"mac"}}`)}, constants.StatusHandshake, func(data *session.HandshakeData) error { return nil }, ""},
	}
	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			mockSession := mocks.NewMockSession(ctrl)
			mockSession.EXPECT().ID().Return(int64(1)).Times(1)

			mockAgent := agentmocks.NewMockAgent(ctrl)
			mockAgent.EXPECT().GetSession().Return(mockSession).Times(1)

			if table.validator != nil {
				mockAgent.EXPECT().GetSession().Return(mockSession).Times(1)
				mockSession.EXPECT().ValidateHandshake(gomock.Any()).DoAndReturn(func(data *session.HandshakeData) error {
					return table.validator(data)
				}).Times(1)
			}

			if table.errStr == "" {
				handshakeData := &session.HandshakeData{}
				_ = encjson.Unmarshal(table.packet.Data, handshakeData)
				mockAgent.EXPECT().GetSession().Return(mockSession).Times(2)
				mockAgent.EXPECT().IPVersion().Return(constants.IPv4).Times(1)
				mockAgent.EXPECT().RemoteAddr().Return(&mockAddr{})
				mockAgent.EXPECT().SetStatus(table.socketStatus).Times(1)
				mockAgent.EXPECT().SendHandshakeResponse().Return(nil).Times(1)
				mockAgent.EXPECT().SetLastAt().Times(1)

				mockSession.EXPECT().SetHandshakeData(handshakeData).Times(1)
				mockSession.EXPECT().Set(constants.IPVersionKey, constants.IPv4).Times(1)
			} else {
				mockAgent.EXPECT().SendHandshakeErrorResponse().Times(1)
				mockAgent.EXPECT().Close().Times(1)
			}

			handlerPool := NewHandlerPool()
			svc := NewHandlerService(nil, nil, nil, nil, nil, nil, nil, nil, pipeline.NewHandlerHooks(), handlerPool)
			err := svc.processPacket(mockAgent, table.packet)
			if table.errStr == "" {
				assert.Nil(t, err)
			} else {
				assert.NotNil(t, err)
				assert.Contains(t, err.Error(), table.errStr)
			}
		})
	}
}

func TestHandlerServiceProcessPacketHandshakeAck(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockSession := mocks.NewMockSession(ctrl)
	mockSession.EXPECT().ID().Return(int64(1)).Times(1)

	handlerPool := NewHandlerPool()
	svc := NewHandlerService(nil, nil, nil, nil, nil, nil, nil, nil, nil, handlerPool)

	mockAgent := agentmocks.NewMockAgent(ctrl)
	mockAgent.EXPECT().GetSession().Return(mockSession).Times(1)
	mockAgent.EXPECT().SetStatus(constants.StatusWorking).Times(1)
	mockAgent.EXPECT().RemoteAddr().Return(&mockAddr{})
	mockAgent.EXPECT().SetLastAt()

	err := svc.processPacket(mockAgent, &packet.Packet{Type: packet.HandshakeAck})
	assert.NoError(t, err)
}

func TestHandlerServiceProcessPacketHeartbeat(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	mockAgent := agentmocks.NewMockAgent(ctrl)
	mockAgent.EXPECT().SetLastAt()

	handlerPool := NewHandlerPool()
	svc := NewHandlerService(nil, nil, nil, nil, nil, nil, nil, nil, nil, handlerPool)

	err := svc.processPacket(mockAgent, &packet.Packet{Type: packet.Heartbeat})
	assert.NoError(t, err)
}

func TestHandlerServiceHandle(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	packetEncoder := codec.NewPomeloPacketEncoder()
	packetDecoder := codec.NewPomeloPacketDecoder()
	handshakeBuffer := `{"sys":{"platform":"mac","libVersion":"0.3.5-release","clientBuildNumber":"20","clientVersion":"2.1"},"user":{"age":30}}`
	bbb, err := packetEncoder.Encode(packet.Handshake, []byte(handshakeBuffer))
	assert.NoError(t, err)

	mockSerializer := serializemocks.NewMockSerializer(ctrl)

	mockConn := connmock.NewMockPlayerConn(ctrl)

	mockAgent := agentmocks.NewMockAgent(ctrl)
	mockAgentFactory := agentmocks.NewMockAgentFactory(ctrl)
	mockAgentFactory.EXPECT().CreateAgent(mockConn).Return(mockAgent).Times(1)

	var wg sync.WaitGroup
	wg.Add(4)
	defer wg.Wait()

	mockAgent.EXPECT().Handle().Do(func() {
		wg.Done()
	})

	mockAgent.EXPECT().SendHandshakeResponse().Return(nil)

	mockSession := mocks.NewMockSession(ctrl)
	mockSession.EXPECT().GetHandshakeData().Return(&session.HandshakeData{Sys: session.HandshakeClientData{BuildNumber: "10"}}).AnyTimes()
	mockSession.EXPECT().SetHandshakeData(gomock.Any()).Times(1)
	mockSession.EXPECT().ValidateHandshake(gomock.Any()).Times(1)
	mockSession.EXPECT().UID().Return("uid").AnyTimes()
	mockSession.EXPECT().ID().Return(int64(1)).Times(2)
	mockSession.EXPECT().Set(constants.IPVersionKey, constants.IPv4)
	mockSession.EXPECT().Close()

	mockAgent.EXPECT().String().Return("").AnyTimes()
	mockAgent.EXPECT().GetStatus().AnyTimes()
	mockAgent.EXPECT().SetStatus(constants.StatusHandshake)
	mockAgent.EXPECT().GetSession().Return(mockSession).AnyTimes()
	mockAgent.EXPECT().IPVersion().Return(constants.IPv4)
	mockAgent.EXPECT().RemoteAddr().Return(&mockAddr{}).AnyTimes()
	mockAgent.EXPECT().SetLastAt().Do(func() {
		wg.Done()
	})

	firstCall := mockConn.EXPECT().GetNextMessage().Return(bbb, nil).Do(func() {
		wg.Done()
	})

	mockConn.EXPECT().GetNextMessage().Return(nil, errors.New("die")).Do(func() {
		wg.Done()
	}).After(firstCall)

	mockConn.EXPECT().Close().MaxTimes(1)

	handlerPool := NewHandlerPool()
	svc := NewHandlerService(packetDecoder, mockSerializer, nil, nil, nil, nil, mockAgentFactory, nil, pipeline.NewHandlerHooks(), handlerPool)
	svc.Handle(mockConn)
}
