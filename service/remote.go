//
// Copyright (c) TFG Co. All Rights Reserved.
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
	"fmt"
	"reflect"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"google.golang.org/protobuf/proto"

	"github.com/topfreegames/pitaya/v2/agent"
	"github.com/topfreegames/pitaya/v2/cluster"
	"github.com/topfreegames/pitaya/v2/component"
	"github.com/topfreegames/pitaya/v2/conn/codec"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/docgenerator"
	e "github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/pipeline"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/route"
	"github.com/topfreegames/pitaya/v2/router"
	"github.com/topfreegames/pitaya/v2/serialize"
	"github.com/topfreegames/pitaya/v2/session"
	"github.com/topfreegames/pitaya/v2/tracing"
	"github.com/topfreegames/pitaya/v2/util"
)

// RemoteService struct
type RemoteService struct {
	protos.UnimplementedPitayaServer
	baseService
	rpcServer              cluster.RPCServer
	serviceDiscovery       cluster.ServiceDiscovery
	serializer             serialize.Serializer
	encoder                codec.PacketEncoder
	rpcClient              cluster.RPCClient
	services               map[string]*component.Service // all registered service
	router                 *router.Router
	messageEncoder         message.Encoder
	server                 *cluster.Server // server obj
	remoteBindingListeners []cluster.RemoteBindingListener
	remoteHooks            *pipeline.RemoteHooks
	sessionPool            session.SessionPool
	handlerPool            *HandlerPool
	remotes                map[string]*component.Remote // all remote method
	taskSevice             *TaskService
}

// NewRemoteService creates and return a new RemoteService
func NewRemoteService(
	rpcClient cluster.RPCClient,
	rpcServer cluster.RPCServer,
	sd cluster.ServiceDiscovery,
	encoder codec.PacketEncoder,
	serializer serialize.Serializer,
	router *router.Router,
	messageEncoder message.Encoder,
	server *cluster.Server,
	sessionPool session.SessionPool,
	remoteHooks *pipeline.RemoteHooks,
	handlerHooks *pipeline.HandlerHooks,
	handlerPool *HandlerPool,
	taskSevice *TaskService,
) *RemoteService {
	remote := &RemoteService{
		services:               make(map[string]*component.Service),
		rpcClient:              rpcClient,
		rpcServer:              rpcServer,
		encoder:                encoder,
		serviceDiscovery:       sd,
		serializer:             serializer,
		router:                 router,
		messageEncoder:         messageEncoder,
		server:                 server,
		remoteBindingListeners: make([]cluster.RemoteBindingListener, 0),
		sessionPool:            sessionPool,
		handlerPool:            handlerPool,
		remotes:                make(map[string]*component.Remote),
		taskSevice:             taskSevice,
	}

	remote.remoteHooks = remoteHooks
	remote.handlerHooks = handlerHooks

	return remote
}

func (r *RemoteService) remoteProcess(
	ctx context.Context,
	server *cluster.Server,
	a agent.Agent,
	route *route.Route,
	msg *message.Message,
) {
	res, err := r.remoteCall(ctx, server, protos.RPCType_Sys, route, a.GetSession(), msg)
	switch msg.Type {
	case message.Request:
		if err != nil {
			logger.Log.Errorf("Failed to process remote server: %s", err.Error())
			a.AnswerWithError(ctx, msg.ID, err)
			return
		}
		err := a.GetSession().ResponseMID(ctx, msg.ID, res.Data)
		if err != nil {
			logger.Log.Errorf("Failed to respond to remote server: %s", err.Error())
			a.AnswerWithError(ctx, msg.ID, err)
		}
	case message.Notify:
		defer tracing.FinishSpan(ctx, err)
		if err == nil && res.Error != nil {
			err = errors.New(res.Error.GetMsg())
		}
		if err != nil {
			logger.Log.Errorf("error while sending a notify to server: %s", err.Error())
		}
	}
}

// AddRemoteBindingListener adds a listener
func (r *RemoteService) AddRemoteBindingListener(bindingListener cluster.RemoteBindingListener) {
	r.remoteBindingListeners = append(r.remoteBindingListeners, bindingListener)
}

// Call processes a remote call
func (r *RemoteService) Call(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	c, err := util.GetContextFromRequest(req, r.server.ID)
	c = util.StartSpanFromRequest(c, r.server.ID, req.GetMsg().GetRoute())
	defer tracing.FinishSpan(c, err)
	var res *protos.Response

	if err == nil {
		result := make(chan *protos.Response, 1)
		go func() {
			rt, err := route.Decode(req.GetMsg().GetRoute())
			if err != nil {
				response := &protos.Response{
					Error: &protos.Error{
						Code: e.ErrBadRequestCode,
						Msg:  "cannot decode route",
						Metadata: map[string]string{
							"route": req.GetMsg().GetRoute(),
						},
					},
				}
				result <- response
				return
			}
			c = pcontext.AddToPropagateCtx(c, constants.RequestInstanceKey, rt.Instance)
			dispatchId, err := r.router.Dispatch(rt, req.Msg.Data)
			if err != nil {
				response := &protos.Response{
					Error: &protos.Error{
						Code: e.ErrBadRequestCode,
						Msg:  "cannot decode dispatch",
						Metadata: map[string]string{
							"route": req.GetMsg().GetRoute(),
						},
					},
				}
				result <- response
				return
			}
			r.taskSevice.Submit(c, dispatchId, func(tctx context.Context) {
				result <- processRemoteMessage(tctx, req, r, rt)
			})
		}()

		reqTimeout := pcontext.GetFromPropagateCtx(ctx, constants.RequestTimeout)
		if reqTimeout != nil {
			var timeout time.Duration
			timeout, err = time.ParseDuration(reqTimeout.(string))
			if err == nil {
				select {
				case <-time.After(timeout):
					err = constants.ErrRPCRequestTimeout
				case res := <-result:
					return res, nil
				}
			}
		} else {
			res := <-result
			return res, nil
		}
	}

	if err != nil {
		res = &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
	}

	if res.Error != nil {
		err = errors.New(res.Error.Msg)
	}

	return res, err
}

// SessionBindRemote is called when a remote server binds a user session and want us to acknowledge it
func (r *RemoteService) SessionBindRemote(ctx context.Context, msg *protos.BindMsg) (*protos.Response, error) {
	for _, r := range r.remoteBindingListeners {
		r.OnUserBind(msg.Uid, msg.Fid)
	}
	return &protos.Response{
		Data: []byte("ack"),
	}, nil
}

// PushToUser sends a push to user
func (r *RemoteService) PushToUser(ctx context.Context, push *protos.Push) (*protos.Response, error) {
	logger.Log.Debugf("sending push to user %s: %v", push.GetUid(), string(push.Data))
	s := r.sessionPool.GetSessionByUID(push.GetUid())
	if s != nil {
		err := s.Push(push.Route, push.Data)
		if err != nil {
			return nil, err
		}
		return &protos.Response{
			Data: []byte("ack"),
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// KickUser sends a kick to user
func (r *RemoteService) KickUser(ctx context.Context, kick *protos.KickMsg) (*protos.KickAnswer, error) {
	logger.Log.Debugf("sending kick to user %s", kick.GetUserId())
	s := r.sessionPool.GetSessionByUID(kick.GetUserId())
	if s != nil {
		err := s.Kick(ctx)
		if err != nil {
			return nil, err
		}
		return &protos.KickAnswer{
			Kicked: true,
		}, nil
	}
	return nil, constants.ErrSessionNotFound
}

// DoRPC do rpc and get answer
func (r *RemoteService) DoRPC(ctx context.Context, rpcType protos.RPCType, serverID string, route *route.Route, protoData []byte) (*protos.Response, error) {
	msg := &message.Message{
		Type:  message.Request,
		Route: route.String(),
		Data:  protoData,
	}

	if ((route.SvType == r.server.Type && serverID == "") || serverID == r.server.ID) && r.server.IsLoopbackEnabled() {
		return r.Loopback(ctx, rpcType, route, msg)
	}

	if serverID == "" {
		return r.remoteCall(ctx, nil, rpcType, route, nil, msg)
	}

	target, _ := r.serviceDiscovery.GetServer(serverID)
	if target == nil {
		return nil, constants.ErrServerNotFound
	}

	return r.remoteCall(ctx, target, rpcType, route, nil, msg)
}

// RPC makes rpcs
func (r *RemoteService) RPC(ctx context.Context, rpcType protos.RPCType, serverID string, route *route.Route, reply proto.Message, arg proto.Message) error {
	var data []byte
	var err error
	if arg != nil {
		data, err = proto.Marshal(arg)
		if err != nil {
			return err
		}
	}
	res, err := r.DoRPC(ctx, rpcType, serverID, route, data)
	if err != nil {
		return err
	}

	if res.Error != nil {
		return &e.Error{
			Code:     res.Error.Code,
			Message:  res.Error.Msg,
			Metadata: res.Error.Metadata,
		}
	}

	if reply != nil {
		err = proto.Unmarshal(res.GetData(), reply)
		if err != nil {
			return err
		}
	}
	return nil
}

func (r *RemoteService) Loopback(ctx context.Context, rpcType protos.RPCType, route *route.Route, msg *message.Message) (*protos.Response, error) {
	req, err := cluster.BuildRequest(ctx, rpcType, route, nil, msg, r.server)
	if err != nil {
		return nil, err
	}

	ctx, err = util.GetContextFromRequest(&req, r.server.ID)
	ctx = util.StartSpanFromRequest(ctx, r.server.ID, req.GetMsg().GetRoute())
	defer tracing.FinishSpan(ctx, err)

	if err != nil {
		logger.Log.Warnf("[remote] failed to retrieve context from request: %s", err.Error())
	}

	// this span replicates the span generated by our nats or grpc clients
	parent, err := tracing.ExtractSpan(ctx)
	tags := opentracing.Tags{
		"span.kind":       "loopback",
		"local.id":        r.server.ID,
		"peer.serverType": r.server.Type,
		"peer.id":         r.server.ID,
	}
	ctx = tracing.StartSpan(ctx, "Loopback RPC Call", tags, parent)
	defer tracing.FinishSpan(ctx, err)
	if err != nil {
		logger.Log.Warnf("[remote] failed to retrieve parent span: %s", err.Error())
	}

	result := make(chan *protos.Response, 1)
	go func() {
		ctx = pcontext.AddToPropagateCtx(ctx, constants.RequestInstanceKey, route.Instance)
		dispatchId, err := r.router.Dispatch(route, req.Msg.Data)
		if err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrBadRequestCode,
					Msg:  "cannot decode dispatch",
					Metadata: map[string]string{
						"route": req.GetMsg().GetRoute(),
					},
				},
			}
			result <- response
			return
		}
		r.taskSevice.Submit(ctx, dispatchId, func(tctx context.Context) {
			result <- processRemoteMessage(ctx, &req, r, route)
		})
	}()

	res := <-result
	return res, nil
}

// Register registers components
func (r *RemoteService) Register(comp component.Component, opts []component.Option) error {
	s := component.NewService(comp, opts)

	if _, ok := r.services[s.Name]; ok {
		return fmt.Errorf("remote: service already defined: %s", s.Name)
	}

	if err := s.ExtractRemote(); err != nil {
		return err
	}

	r.services[s.Name] = s
	// register all remotes
	for name, remote := range s.Remotes {
		r.remotes[fmt.Sprintf("%s.%s", s.Name, name)] = remote
	}

	return nil
}

func processRemoteMessage(ctx context.Context, req *protos.Request, r *RemoteService, rt *route.Route) *protos.Response {
	switch {
	case req.Type == protos.RPCType_Sys:
		return r.handleRPCSys(ctx, req, rt)
	case req.Type == protos.RPCType_User:
		return r.handleRPCUser(ctx, req, rt)
	case req.Type == protos.RPCType_Handle:
		return r.handleRPCHandle(ctx, req, rt)
	default:
		return &protos.Response{
			Error: &protos.Error{
				Code: e.ErrBadRequestCode,
				Msg:  "invalid rpc type",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
	}
}

func (r *RemoteService) handleRPCUser(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	serviceKey := rt.ServiceKey()
	remote, ok := r.remotes[serviceKey]
	if !ok {
		logger.Log.Warnf("pitaya/remote: %s not found", serviceKey)
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrNotFoundCode,
				Msg:  "route not found",
				Metadata: map[string]string{
					"route": serviceKey,
				},
			},
		}
		return response
	}

	var ret interface{}
	var arg interface{}
	var err error

	if remote.HasArgs {
		arg, err = unmarshalRemoteArg(remote.Type, req.GetMsg().GetData())
		if err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrBadRequestCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
	}

	ctx, arg, err = r.remoteHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
		return response
	}

	params := []reflect.Value{remote.Receiver, reflect.ValueOf(ctx)}
	if remote.HasArgs {
		params = append(params, reflect.ValueOf(arg))
	}
	ret, err = util.Pcall(remote.Method, params)

	ret, err = r.remoteHooks.AfterHandler.ExecuteAfterPipeline(ctx, ret, err)
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{},
		}
		logLevel := 0
		code := e.ErrUnknownCode
		msg := err.Error()
		if val, ok := err.(e.PitayaError); ok {
			logLevel = val.GetLevel()
			code = val.GetCode()
			msg = val.GetMsg()
			response.Error.Metadata = val.GetMetadata()
		}
		response.Error.Code = code
		response.Error.Msg = msg
		if logLevel == 0 {
			logger.Log.Errorf("RPC %s failed to process message: %s", rt.String(), err.Error())
		} else {
			logger.Log.Warnf("RPC %s encountered an issue processing message: %s", rt.String(), err.Error())
		}
		return response
	}

	var b []byte
	if ret != nil {
		pb, ok := ret.(proto.Message)
		if !ok {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  constants.ErrWrongValueType.Error(),
				},
			}
			return response
		}
		if b, err = proto.Marshal(pb); err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
	}

	response := &protos.Response{}
	response.Data = b
	return response
}

func (r *RemoteService) handleRPCSys(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	reply := req.GetMsg().GetReply()
	response := &protos.Response{}
	// (warning) a new agent is created for every new request
	a, err := agent.NewRemote(
		req.GetSession(),
		reply,
		r.rpcClient,
		r.encoder,
		r.serializer,
		r.serviceDiscovery,
		req.FrontendID,
		r.messageEncoder,
		r.sessionPool,
	)
	if err != nil {
		logger.Log.Warn("pitaya/handler: cannot instantiate remote agent")
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
		return response
	}

	ret, err := r.handlerPool.ProcessHandlerMessage(ctx, rt, r.serializer, r.handlerHooks, a.Session, req.GetMsg().GetData(), req.GetMsg().GetType(), true)
	if err != nil {
		response = &protos.Response{
			Error: &protos.Error{},
		}
		logLevel := 0
		code := e.ErrUnknownCode
		msg := err.Error()
		if val, ok := err.(e.PitayaError); ok {
			logLevel = val.GetLevel()
			code = val.GetCode()
			msg = val.GetMsg()
			response.Error.Metadata = val.GetMetadata()
		}
		response.Error.Code = code
		response.Error.Msg = msg
		if logLevel == 0 {
			logger.Log.Errorf("Remote handler %s failed to process message: %s", rt.String(), err.Error())
		} else {
			logger.Log.Warnf("Remote handler %s encountered an issue processing message: %s", rt.String(), err.Error())
		}
	} else {
		response = &protos.Response{Data: ret}
	}
	return response
}

func (r *RemoteService) handleRPCHandle(ctx context.Context, req *protos.Request, rt *route.Route) *protos.Response {
	serviceKey := rt.ServiceKey()
	handler, ok := r.handlerPool.handlers[serviceKey]
	if !ok {
		logger.Log.Warnf("pitaya/handler: %s not found", serviceKey)
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrNotFoundCode,
				Msg:  "route not found",
				Metadata: map[string]string{
					"route": serviceKey,
				},
			},
		}
		return response
	}

	var ret interface{}
	var arg interface{}
	var err error

	arg, err = unmarshalRemoteArg(handler.Type, req.GetMsg().GetData())
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrBadRequestCode,
				Msg:  err.Error(),
			},
		}
		return response
	}

	val := pcontext.GetFromPropagateCtx(ctx, constants.RequestUidKey)
	uid, _ := val.(string)
	ctx = util.CtxWithDefaultLogger(ctx, rt.Short(), uid)
	ctx, arg, err = r.handlerHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{
				Code: e.ErrInternalCode,
				Msg:  err.Error(),
			},
		}
		return response
	}

	params := []reflect.Value{handler.Receiver, reflect.ValueOf(ctx)}
	params = append(params, reflect.ValueOf(arg))
	ret, err = util.Pcall(handler.Method, params)
	ret, err = r.handlerHooks.AfterHandler.ExecuteAfterPipeline(ctx, ret, err)
	if err != nil {
		response := &protos.Response{
			Error: &protos.Error{},
		}
		logLevel := 0
		code := e.ErrUnknownCode
		msg := err.Error()
		if val, ok := err.(e.PitayaError); ok {
			logLevel = val.GetLevel()
			code = val.GetCode()
			msg = val.GetMsg()
			response.Error.Metadata = val.GetMetadata()
		}
		response.Error.Code = code
		response.Error.Msg = msg
		if logLevel == 0 {
			logger.Log.Errorf("RPC handler %s failed to process message: %s", rt.String(), err.Error())
		} else {
			logger.Log.Warnf("RPC handler %s encountered a warning while processing message: %s", rt.String(), err.Error())
		}
		return response
	}

	var b []byte
	if ret != nil {
		pb, ok := ret.(proto.Message)
		if !ok {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  constants.ErrWrongValueType.Error(),
				},
			}
			return response
		}
		if b, err = proto.Marshal(pb); err != nil {
			response := &protos.Response{
				Error: &protos.Error{
					Code: e.ErrUnknownCode,
					Msg:  err.Error(),
				},
			}
			return response
		}
	}

	response := &protos.Response{}
	response.Data = b
	return response
}

func (r *RemoteService) remoteCall(
	ctx context.Context,
	server *cluster.Server,
	rpcType protos.RPCType,
	route *route.Route,
	session session.Session,
	msg *message.Message,
) (*protos.Response, error) {
	svType := route.SvType

	var err error
	target := server

	if target == nil {
		target, err = r.router.Route(ctx, rpcType, svType, route, msg)
		if err != nil {
			logger.Log.Errorf("error making call for route %s: %w", route.String(), err)
			return nil, e.NewError(err, e.ErrInternalCode)
		}
	}

	res, err := r.rpcClient.Call(ctx, rpcType, route, session, msg, target)
	if err != nil {
		logger.Log.Errorf("error making call to target with id %s, route %s and host %s: %s", target.ID, route.String(), target.Hostname, err.Error())
		return nil, err
	}
	return res, err
}

// DumpServices outputs all registered services
func (r *RemoteService) DumpServices() {
	for name := range r.remotes {
		logger.Log.Infof("registered remote %s", name)
	}
}

// Docs returns documentation for remotes
func (r *RemoteService) Docs(getPtrNames bool) (map[string]interface{}, error) {
	if r == nil {
		return map[string]interface{}{}, nil
	}
	return docgenerator.RemotesDocs(r.server.Type, r.services, getPtrNames)
}
