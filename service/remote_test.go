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
	"context"
	"errors"
	"math/rand"
	"sync/atomic"
	"testing"

	agentmocks "github.com/actfuns/pitaya/v2/agent/mocks"
	"github.com/actfuns/pitaya/v2/cluster"
	clustermocks "github.com/actfuns/pitaya/v2/cluster/mocks"
	"github.com/actfuns/pitaya/v2/component"
	"github.com/actfuns/pitaya/v2/conn/codec"
	"github.com/actfuns/pitaya/v2/conn/message"
	messagemocks "github.com/actfuns/pitaya/v2/conn/message/mocks"
	"github.com/actfuns/pitaya/v2/constants"
	"github.com/actfuns/pitaya/v2/pipeline"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/prpc"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/actfuns/pitaya/v2/router"
	serializemocks "github.com/actfuns/pitaya/v2/serialize/mocks"
	"github.com/actfuns/pitaya/v2/session"
	sessionmocks "github.com/actfuns/pitaya/v2/session/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestNewRemoteService(t *testing.T) {
	packetEncoder := codec.NewPomeloPacketEncoder()
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockSerializer := serializemocks.NewMockSerializer(ctrl)
	mockSD := clustermocks.NewMockServiceDiscovery(ctrl)
	mockRPCClient := clustermocks.NewMockRPCClient(ctrl)
	mockRPCServer := clustermocks.NewMockRPCServer(ctrl)
	mockMessageEncoder := messagemocks.NewMockEncoder(ctrl)
	router := router.New()
	sv := &cluster.Server{}
	sessionPool := session.NewSessionPool()
	remoteHooks := pipeline.NewRemoteHooks()
	handlerHooks := pipeline.NewHandlerHooks()
	handlerPool := NewHandlerPool()
	svc := NewRemoteService(mockRPCClient, mockRPCServer, mockSD, packetEncoder, mockSerializer, router, mockMessageEncoder, sv, sessionPool, remoteHooks, handlerHooks, handlerPool, nil)

	assert.NotNil(t, svc)
	assert.Equal(t, mockRPCClient, svc.rpcClient)
	assert.Equal(t, mockRPCServer, svc.rpcServer)
	assert.Equal(t, packetEncoder, svc.encoder)
	assert.Equal(t, mockSD, svc.serviceDiscovery)
	assert.Equal(t, mockSerializer, svc.serializer)
	assert.Equal(t, router, svc.router)
	assert.Equal(t, sv, svc.server)
	assert.Equal(t, sessionPool, svc.sessionPool)
	assert.Equal(t, remoteHooks, svc.remoteHooks)
	assert.Equal(t, handlerHooks, svc.handlerHooks)
	assert.Equal(t, handlerPool, svc.handlerPool)
}

func TestRemoteServiceAddRemoteBindingListener(t *testing.T) {
	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockBindingListener := clustermocks.NewMockRemoteBindingListener(ctrl)

	svc.AddRemoteBindingListener(mockBindingListener)
	assert.Equal(t, mockBindingListener, svc.remoteBindingListeners[0])
}

func TestRemoteServiceSessionBindRemote(t *testing.T) {
	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil, nil)
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()
	mockBindingListener := clustermocks.NewMockRemoteBindingListener(ctrl)

	svc.AddRemoteBindingListener(mockBindingListener)
	assert.Equal(t, mockBindingListener, svc.remoteBindingListeners[0])

	msg := &protos.BindMsg{
		Uid: "uid",
		Fid: "fid",
	}

	mockBindingListener.EXPECT().OnUserBind(msg.Uid, msg.Fid)

	_, err := svc.SessionBindRemote(context.Background(), msg)

	assert.NoError(t, err)
}

func TestRemoteServicePushToUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	existingUID := "uid1"
	nonexistingUID := "uid2"

	mockSession := sessionmocks.NewMockSession(ctrl)

	mockSessionPool := sessionmocks.NewMockSessionPool(ctrl)
	mockSessionPool.EXPECT().GetSessionByUID(existingUID).Return(mockSession).Times(1)
	mockSessionPool.EXPECT().GetSessionByUID(nonexistingUID).Return(nil).Times(1)

	tables := []struct {
		name string
		uid  string
		sess session.Session
		p    *protos.Push
		err  error
	}{
		{"success", "uid1", mockSession, &protos.Push{
			Route: "sv.svc.mth",
			Uid:   existingUID,
			Data:  []byte{0x01},
		}, nil},
		{"no_sess_found", "uid2", nil, &protos.Push{
			Route: "sv.svc.mth",
			Uid:   nonexistingUID,
			Data:  []byte{0x01},
		}, constants.ErrSessionNotFound},
	}

	mockSession.EXPECT().Push(tables[0].p.Route, tables[0].p.Data).Times(1)
	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, nil, mockSessionPool, nil, nil, nil, nil)

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			_, err := svc.PushToUser(context.Background(), table.p)
			if table.err != nil {
				assert.EqualError(t, err, table.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestRemoteServiceKickUser(t *testing.T) {
	ctrl := gomock.NewController(t)
	mockSessionPool := sessionmocks.NewMockSessionPool(ctrl)
	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, nil, mockSessionPool, nil, nil, nil, nil)

	existingUID := "uid1"
	nonexistingUID := "uid2"

	mockSession := sessionmocks.NewMockSession(ctrl)
	mockSession.EXPECT().Kick(context.Background()).Times(1)

	mockSessionPool.EXPECT().GetSessionByUID(existingUID).Return(mockSession).Times(1)
	mockSessionPool.EXPECT().GetSessionByUID(nonexistingUID).Return(nil).Times(1)

	defer ctrl.Finish()

	tables := []struct {
		name string
		uid  string
		sess session.Session
		p    *protos.KickMsg
		err  error
	}{
		{"success", existingUID, mockSession, &protos.KickMsg{
			UserId: existingUID,
		}, nil},
		{"sessionNotFound", nonexistingUID, nil, &protos.KickMsg{
			UserId: nonexistingUID,
		}, constants.ErrSessionNotFound},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			_, err := svc.KickUser(context.Background(), table.p)
			if table.err != nil {
				assert.EqualError(t, err, table.err.Error())
			} else {
				assert.NoError(t, err)
			}
		})
	}

}

func TestRemoteServiceRemoteProcess(t *testing.T) {
	sv := &cluster.Server{}
	rt := route.NewRoute("sv", "svc", "method")

	tables := []struct {
		name           string
		msgType        message.Type
		remoteCallErr  error
		responseMIDErr error
	}{
		{"failed_remote_call", message.Request, errors.New("rpc failed"), nil},
		{"failed_response_mid", message.Request, nil, errors.New("err")},
		{"success_request", message.Request, nil, nil},
		{"success_notify", message.Notify, nil, nil},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()

			expectedMsg := &message.Message{
				ID:    uint(rand.Int()),
				Type:  table.msgType,
				Route: rt.Short(),
				Data:  []byte("ok"),
			}
			ctx := context.Background()

			packetEncoder := codec.NewPomeloPacketEncoder()
			mockSerializer := serializemocks.NewMockSerializer(ctrl)
			mockSD := clustermocks.NewMockServiceDiscovery(ctrl)
			mockRPCClient := clustermocks.NewMockRPCClient(ctrl)
			mockRPCServer := clustermocks.NewMockRPCServer(ctrl)
			messageEncoder := message.NewMessagesEncoder(false)
			router := router.New()
			sessionPool := session.NewSessionPool()
			mockSession := sessionmocks.NewMockSession(ctrl)

			mockAgent := agentmocks.NewMockAgent(ctrl)
			mockAgent.EXPECT().GetSession().Return(mockSession).AnyTimes()

			mockRPCClient.EXPECT().Call(ctx, protos.RPCType_Sys, rt, gomock.Any(), expectedMsg, gomock.Any()).Return(&protos.Response{Data: []byte("ok")}, table.remoteCallErr)

			if table.remoteCallErr != nil {
				mockAgent.EXPECT().AnswerWithError(ctx, expectedMsg.ID, gomock.Any())
			} else if expectedMsg.Type != message.Notify {
				mockSession.EXPECT().ResponseMID(ctx, expectedMsg.ID, gomock.Any()).Return(table.responseMIDErr)
			}

			if table.responseMIDErr != nil {
				mockAgent.EXPECT().AnswerWithError(ctx, expectedMsg.ID, table.responseMIDErr)
			}

			svc := NewRemoteService(mockRPCClient, mockRPCServer, mockSD, packetEncoder, mockSerializer, router, messageEncoder, &cluster.Server{}, sessionPool, nil, pipeline.NewHandlerHooks(), nil, nil)
			svc.remoteProcess(ctx, sv, mockAgent, rt, expectedMsg)
		})
	}
}

func TestRemoteServiceDoRPCHitsInterceptorsOnLoopback(t *testing.T) {
	sv := cluster.NewServer("server-1", "chat", false, cluster.WithLoopbackEnabled(true))
	rt := route.NewRoute("sv", "svc", "method")
	ctx := context.Background()

	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, sv, nil, pipeline.NewRemoteHooks(), pipeline.NewHandlerHooks(), NewHandlerPool(), nil)

	calls := 0
	var gotTarget *cluster.Server
	var gotRoute *route.Route
	svc.AddRPCInterceptor(func(next cluster.UnaryInvoker) cluster.UnaryInvoker {
		return func(c *cluster.RPCContext) (*protos.Response, error) {
			calls++
			gotTarget = c.Target
			gotRoute = c.Route
			return &protos.Response{Data: []byte("intercepted")}, nil
		}
	})

	res, err := svc.DoRPC(ctx, protos.RPCType_Sys, sv.ID, rt, nil, prpc.CallOptions{})

	assert.NoError(t, err)
	assert.Equal(t, []byte("intercepted"), res.Data)
	assert.Equal(t, 1, calls)
	assert.Equal(t, sv, gotTarget)
	assert.Equal(t, rt, gotRoute)
}

func TestRemoteServiceDoRPCHitsInterceptorsOnRemotePath(t *testing.T) {
	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	sv := cluster.NewServer("server-1", "chat", false)
	target := cluster.NewServer("server-2", "chat", false)
	rt := route.NewRoute("sv", "svc", "method")
	ctx := context.Background()
	msg := &message.Message{Type: message.Request, Route: rt.String(), Data: []byte("ok")}

	mockSD := clustermocks.NewMockServiceDiscovery(ctrl)
	mockRPCClient := clustermocks.NewMockRPCClient(ctrl)

	mockSD.EXPECT().GetServer(target.ID).Return(target, nil)
	mockRPCClient.EXPECT().Call(ctx, protos.RPCType_Sys, rt, gomock.Any(), msg, target).Return(&protos.Response{Data: []byte("remote")}, nil)

	svc := NewRemoteService(mockRPCClient, nil, mockSD, nil, nil, nil, nil, sv, nil, pipeline.NewRemoteHooks(), pipeline.NewHandlerHooks(), nil, nil)

	calls := 0
	var gotTarget *cluster.Server
	svc.AddRPCInterceptor(func(next cluster.UnaryInvoker) cluster.UnaryInvoker {
		return func(c *cluster.RPCContext) (*protos.Response, error) {
			calls++
			gotTarget = c.Target
			return next(c)
		}
	})

	res, err := svc.DoRPC(ctx, protos.RPCType_Sys, target.ID, rt, []byte("ok"), prpc.CallOptions{})

	assert.NoError(t, err)
	assert.Equal(t, []byte("remote"), res.Data)
	assert.Equal(t, 1, calls)
	assert.Equal(t, target, gotTarget)
}

func TestRemoteServiceDoRPCLoopbackDispatchesThroughInterceptors(t *testing.T) {
	sv := cluster.NewServer("server-1", "chat", false, cluster.WithLoopbackEnabled(true))
	rt := route.NewRoute("sv", "svc", "method")
	ctx := context.Background()

	var handlerRan int32
	handlerPool := NewHandlerPool()
	handlerPool.Register(prpc.KindRPC, rt.Domain, rt.Service, rt.Method, &component.Handler{
		Codec:     true,
		Reentrant: true,
		Fn: func(srv interface{}, hctx context.Context, data []byte, prepare func(context.Context, interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			atomic.AddInt32(&handlerRan, 1)
			return []byte("pong"), nil
		},
	})

	taskService, err := NewTaskService(1, 1, 60)
	assert.NoError(t, err)
	defer taskService.Shutdown()

	// no rpcClient: a loopback call that tried to reach the wire would panic.
	svc := NewRemoteService(nil, nil, nil, nil, nil, nil, nil, sv, nil, pipeline.NewRemoteHooks(), pipeline.NewHandlerHooks(), handlerPool, taskService)

	var order []string
	svc.AddRPCInterceptor(func(next cluster.UnaryInvoker) cluster.UnaryInvoker {
		return func(c *cluster.RPCContext) (*protos.Response, error) {
			order = append(order, "before")
			res, err := next(c)
			order = append(order, "after")
			return res, err
		}
	})

	res, err := svc.DoRPC(ctx, protos.RPCType_User, sv.ID, rt, nil, prpc.CallOptions{})

	assert.NoError(t, err)
	assert.Equal(t, []byte("pong"), res.Data)
	assert.Equal(t, int32(1), atomic.LoadInt32(&handlerRan))
	assert.Equal(t, []string{"before", "after"}, order)
}
