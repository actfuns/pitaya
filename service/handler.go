// Copyright (c) nano Author and TFG Co. All Rights Reserved.
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
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/nats-io/nuid"

	"github.com/topfreegames/pitaya/v2/acceptor"
	"github.com/topfreegames/pitaya/v2/pipeline"

	opentracing "github.com/opentracing/opentracing-go"
	"github.com/topfreegames/pitaya/v2/agent"
	"github.com/topfreegames/pitaya/v2/cluster"
	"github.com/topfreegames/pitaya/v2/component"
	"github.com/topfreegames/pitaya/v2/conn/codec"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/conn/packet"
	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/docgenerator"
	e "github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/metrics"
	"github.com/topfreegames/pitaya/v2/route"
	"github.com/topfreegames/pitaya/v2/serialize"
	"github.com/topfreegames/pitaya/v2/session"
	"github.com/topfreegames/pitaya/v2/tracing"
)

var (
	handlerType = "handler"
)

type (
	// HandlerService service
	HandlerService struct {
		baseService
		decoder          codec.PacketDecoder // binary decoder
		remoteService    *RemoteService
		serializer       serialize.Serializer          // message serializer
		server           *cluster.Server               // server obj
		services         map[string]*component.Service // all registered service
		metricsReporters []metrics.Reporter
		agentFactory     agent.AgentFactory
		handlerPool      *HandlerPool
		handlers         map[string]*component.Handler // all handler method
		taskService      *TaskService
	}
)

// NewHandlerService creates and returns a new handler service
func NewHandlerService(
	packetDecoder codec.PacketDecoder,
	serializer serialize.Serializer,
	server *cluster.Server,
	remoteService *RemoteService,
	agentFactory agent.AgentFactory,
	metricsReporters []metrics.Reporter,
	handlerHooks *pipeline.HandlerHooks,
	handlerPool *HandlerPool,
	taskService *TaskService,
) *HandlerService {
	h := &HandlerService{
		services:         make(map[string]*component.Service),
		decoder:          packetDecoder,
		serializer:       serializer,
		server:           server,
		remoteService:    remoteService,
		agentFactory:     agentFactory,
		metricsReporters: metricsReporters,
		handlerPool:      handlerPool,
		handlers:         make(map[string]*component.Handler),
		taskService:      taskService,
	}

	h.handlerHooks = handlerHooks

	return h
}

// Register registers components
func (h *HandlerService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := h.services[s.Name]; ok {
		return fmt.Errorf("handler: service already defined: %s", s.Name)
	}

	if err := s.ExtractHandler(); err != nil {
		return err
	}

	// register all handlers
	h.services[s.Name] = s
	for name, handler := range s.Handlers {
		h.handlerPool.Register(s.Name, name, handler)
	}
	return nil
}

// Handle handles messages from a conn
func (h *HandlerService) Handle(conn acceptor.PlayerConn) {
	// create a client agent and startup write goroutine
	a := h.agentFactory.CreateAgent(conn)

	// startup agent goroutine
	go a.Handle()

	logger := logger.Log.WithFields(map[string]interface{}{
		"session_id": a.GetSession().ID(),
		"uid":        a.GetSession().UID(),
		"remote":     a.RemoteAddr().String(),
	})

	logger.Debugf("New session established")

	// guarantee agent related resource is destroyed
	defer func() {
		a.GetSession().Close()
		logger.Debugf("Session read goroutine exit")
	}()

	for {
		msg, err := conn.GetNextMessage()

		if err != nil {
			// Check if this is an expected error due to connection being closed
			if errors.Is(err, net.ErrClosed) || err == constants.ErrConnectionClosed {
				logger.WithError(err).Debug("Connection no longer available while reading next available message")
			} else {
				// Differentiate errors for valid sessions, to avoid noise from load balancer healthchecks and other internet noise
				if a.GetStatus() != constants.StatusStart {
					logger.WithError(err).Error("Error reading next available message")
				} else {
					logger.WithError(err).Debug("Error reading next available message on initial connection")
				}
			}

			return
		}

		packets, err := h.decoder.Decode(msg)
		if err != nil {
			logger.WithError(err).Errorf("Failed to decode message")
			return
		}

		if len(packets) < 1 {
			logger.WithField("data", msg).Warnf("Read no packets")
			continue
		}

		// process all packet
		for i := range packets {
			if err := h.processPacket(a, packets[i]); err != nil {
				logger.WithError(err).Errorf("Failed to process packet")
				return
			}
		}
	}
}

func (h *HandlerService) processPacket(a agent.Agent, p *packet.Packet) error {
	switch p.Type {
	case packet.Handshake:
		logger.Log.Debug("Received handshake packet")

		// Parse the json sent with the handshake by the client
		handshakeData := &session.HandshakeData{}
		if err := json.Unmarshal(p.Data, handshakeData); err != nil {
			defer a.Close()
			logger.Log.Errorf("Failed to unmarshal handshake data: %s", err.Error())
			if serr := a.SendHandshakeErrorResponse(); serr != nil {
				logger.Log.Errorf("Error sending handshake error response: %s", err.Error())
				return err
			}

			return fmt.Errorf("invalid handshake data. Id=%d", a.GetSession().ID())
		}

		if err := a.GetSession().ValidateHandshake(handshakeData); err != nil {
			defer a.Close()
			logger.Log.Errorf("Handshake validation failed: %s", err.Error())
			if serr := a.SendHandshakeErrorResponse(); serr != nil {
				logger.Log.Errorf("Error sending handshake error response: %s", err.Error())
				return err
			}

			return fmt.Errorf("handshake validation failed: %w. SessionId=%d", err, a.GetSession().ID())
		}

		if err := a.SendHandshakeResponse(); err != nil {
			logger.Log.Errorf("Error sending handshake response: %s", err.Error())
			return err
		}
		logger.Log.Debugf("Session handshake Id=%d, Remote=%s", a.GetSession().ID(), a.RemoteAddr())

		a.GetSession().SetHandshakeData(handshakeData)
		a.SetStatus(constants.StatusHandshake)
		err := a.GetSession().Set(constants.IPVersionKey, a.IPVersion())
		if err != nil {
			logger.Log.Warnf("failed to save ip version on session: %q\n", err)
		}

		logger.Log.Debug("Successfully saved handshake data")

	case packet.HandshakeAck:
		a.SetStatus(constants.StatusWorking)
		logger.Log.Debugf("Receive handshake ACK Id=%d, Remote=%s", a.GetSession().ID(), a.RemoteAddr())

	case packet.Data:
		if a.GetStatus() < constants.StatusWorking {
			return fmt.Errorf("receive data on socket which is not yet ACK, session will be closed immediately, remote=%s",
				a.RemoteAddr().String())
		}

		msg, err := message.Decode(p.Data)
		if err != nil {
			return err
		}
		h.processMessage(a, msg)

	case packet.Heartbeat:
		// expected
	}

	a.SetLastAt()
	return nil
}

func (h *HandlerService) processMessage(a agent.Agent, msg *message.Message) {
	requestID := nuid.New().Next()
	ctx := pcontext.AddToPropagateCtx(context.Background(), constants.StartTimeKey, time.Now().UnixNano())
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RouteKey, msg.Route)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestIDKey, requestID)
	session := a.GetSession()
	tags := opentracing.Tags{
		"local.id":   h.server.ID,
		"span.kind":  "server",
		"msg.type":   strings.ToLower(msg.Type.String()),
		"user.id":    session.UID(),
		"request.id": requestID,
	}
	ctx = tracing.StartSpan(ctx, msg.Route, tags)
	ctx = context.WithValue(ctx, constants.SessionCtxKey, session)
	ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestUidKey, session.UID())

	r, err := route.Decode(msg.Route)
	if err != nil {
		logger.Log.Errorf("Failed to decode route: %s", err.Error())
		a.AnswerWithError(ctx, msg.ID, e.NewError(err, e.ErrBadRequestCode))
		return
	}

	if r.SvType == "" {
		r.SvType = h.server.Type
	}

	if err := h.taskService.Submit(ctx, fmt.Sprintf("handler_%d", session.ID()), func(tctx context.Context) {
		if r.SvType == h.server.Type {
			metrics.ReportMessageProcessDelayFromCtx(tctx, h.metricsReporters, "local")
			h.localProcess(tctx, a, r, msg)
		} else {
			if h.remoteService != nil {
				metrics.ReportMessageProcessDelayFromCtx(tctx, h.metricsReporters, "remote")
				h.remoteService.remoteProcess(tctx, nil, a, r, msg)
			} else {
				logger.Log.Warnf("request made to another server type but no remoteService running")
			}
		}
	}); err != nil {
		logger.Log.Errorf("Failed to submit task: %s", err.Error())
	}
}

func (h *HandlerService) localProcess(ctx context.Context, a agent.Agent, route *route.Route, msg *message.Message) {
	var mid uint
	switch msg.Type {
	case message.Request:
		mid = msg.ID
	case message.Notify:
		mid = 0
	}

	ret, err := h.handlerPool.ProcessHandlerMessage(ctx, route, h.serializer, h.handlerHooks, a.GetSession(), msg.Data, msg.Type, false)
	if msg.Type != message.Notify {
		if err != nil {
			logLevel := 0
			if val, ok := err.(e.PitayaError); ok {
				logLevel = int(val.GetLevel())
			}
			if logLevel == 0 {
				logger.Log.Errorf("handler %s failed to process message: %s", route.String(), err.Error())
			} else {
				logger.Log.Warnf("handler %s encountered a warning while processing message: %s", route.String(), err.Error())
			}
			a.AnswerWithError(ctx, mid, err)
		} else {
			err := a.GetSession().ResponseMID(ctx, mid, ret)
			if err != nil {
				logger.Log.Errorf("Failed to process handler message: %s", err.Error())
				tracing.FinishSpan(ctx, err)
				metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, err)
			}
		}
	} else {
		metrics.ReportTimingFromCtx(ctx, h.metricsReporters, handlerType, err)
		tracing.FinishSpan(ctx, err)
		if err != nil {
			logger.Log.Errorf("Failed to process notify message: %s", err.Error())
		}
	}
}

// DumpServices outputs all registered services
func (h *HandlerService) DumpServices() {
	handlers := h.handlerPool.GetHandlers()
	for name := range handlers {
		logger.Log.Infof("registered handler %s, isRawArg: %v", name, handlers[name].IsRawArg)
	}
}

// Docs returns documentation for handlers
func (h *HandlerService) Docs(getPtrNames bool) (map[string]interface{}, error) {
	if h == nil {
		return map[string]interface{}{}, nil
	}
	return docgenerator.HandlersDocs(h.server.Type, h.services, getPtrNames)
}
