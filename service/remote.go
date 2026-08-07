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
	"fmt"
	"time"

	opentracing "github.com/opentracing/opentracing-go"
	"google.golang.org/protobuf/proto"

	"github.com/actfuns/pitaya/v2/agent"
	"github.com/actfuns/pitaya/v2/cluster"
	"github.com/actfuns/pitaya/v2/component"
	"github.com/actfuns/pitaya/v2/conn/codec"
	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/constants"
	pcontext "github.com/actfuns/pitaya/v2/context"
	e "github.com/actfuns/pitaya/v2/errors"
	"github.com/actfuns/pitaya/v2/logger"
	"github.com/actfuns/pitaya/v2/pipeline"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/prpc"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/actfuns/pitaya/v2/router"
	"github.com/actfuns/pitaya/v2/serialize"
	"github.com/actfuns/pitaya/v2/session"
	"github.com/actfuns/pitaya/v2/tracing"
	"github.com/actfuns/pitaya/v2/util"
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
	router                 *router.Router
	messageEncoder         message.Encoder
	server                 *cluster.Server // server obj
	remoteBindingListeners []cluster.RemoteBindingListener
	remoteHooks            *pipeline.RemoteHooks
	interceptorChain       *cluster.InterceptorChain
	sessionPool            session.SessionPool
	handlerPool            *HandlerPool
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
		taskSevice:             taskSevice,
	}

	remote.remoteHooks = remoteHooks
	remote.handlerHooks = handlerHooks
	remote.interceptorChain = cluster.NewInterceptorChain()

	return remote
}

// AddRPCInterceptor registers client-side unary interceptors that wrap every
// outbound RPC call made through this RemoteService.
// Interceptors registered first are the outermost ones. Should not be used
// after the server is running.
func (r *RemoteService) AddRPCInterceptor(interceptors ...cluster.UnaryInterceptor) {
	if r.interceptorChain == nil {
		r.interceptorChain = cluster.NewInterceptorChain()
	}
	r.interceptorChain.Add(interceptors...)
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
			// logger.Log.Errorf("Failed to process remote server: %s", err.Error())
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
		// if err == nil && res.Error != nil {
		// 	err = errors.New(res.Error.GetMsg())
		// }
		// if err != nil {
		// 	logger.Log.Errorf("error while sending a notify to server: %s", err.Error())
		// }
	}
}

// AddRemoteBindingListener adds a listener
func (r *RemoteService) AddRemoteBindingListener(bindingListener cluster.RemoteBindingListener) {
	r.remoteBindingListeners = append(r.remoteBindingListeners, bindingListener)
}

// Call processes a remote call
func (r *RemoteService) Call(ctx context.Context, req *protos.Request) (*protos.Response, error) {
	c, err := util.GetContextFromRequest(req, r.server.ID)
	c = util.StartSpanFromRequest(c, r.server.ID, req.Msg.Route)
	defer tracing.FinishSpan(c, err)

	if err == nil {
		reqTimeout := pcontext.GetFromPropagateCtx(c, constants.RequestTimeout)
		var timeout time.Duration
		if reqTimeout != nil {
			timeout, _ = time.ParseDuration(reqTimeout.(string))
		}

		ret, err := r.dispatchRemoteMessage(c, req, timeout)
		if err == nil {
			return ret, nil
		}
	}

	res := &protos.Response{
		Error: &protos.Error{
			Code:    e.ErrInternalCode,
			Message: err.Error(),
		},
	}
	logger.WithCtx(ctx).Errorf("[remote] failed to process remote message for route '%s': %v", req.Msg.Route, err)
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
	return nil, e.NewError(constants.ErrSessionNotFound, e.ErrSessionNotFound)
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
	return nil, e.NewError(constants.ErrSessionNotFound, e.ErrSessionNotFound)
}

// DoRPC do rpc and get answer
func (r *RemoteService) DoRPC(ctx context.Context, rpcType protos.RPCType, serverID string, route *route.Route, protoData []byte, opt prpc.CallOptions) (*protos.Response, error) {
	msg := &message.Message{
		Type:  message.Request,
		Route: route.String(),
		Data:  protoData,
	}

	if opt.OneWay {
		msg.Type = message.Notify
	}

	if serverID == "" {
		sctx, shardKey, target, err := r.router.Resolve(ctx, rpcType, route, msg)
		if err != nil {
			logger.Log.Errorf("error making call for route %s: %v", msg.Route, err)
			return nil, e.NewError(err, e.ErrInternalCode)
		}
		serverID = target.ID
		msg.ShardKey = shardKey
		ctx = sctx
	}

	if serverID == r.server.ID && r.server.IsLoopbackEnabled() {
		return r.Loopback(ctx, rpcType, route, msg)
	}

	target, _ := r.serviceDiscovery.GetServer(serverID)
	if target == nil {
		return nil, constants.ErrServerNotFound
	}

	return r.remoteCall(ctx, target, rpcType, route, nil, msg)
}

// RPC makes rpcs
func (r *RemoteService) RPC(ctx context.Context, rpcType protos.RPCType, serverID string, route *route.Route, reply proto.Message, arg proto.Message, opt prpc.CallOptions) error {
	var data []byte
	var err error
	if arg != nil {
		data, err = r.serializer.Marshal(arg)
		if err != nil {
			return err
		}
	}
	res, err := r.DoRPC(ctx, rpcType, serverID, route, data, opt)
	if err != nil {
		return err
	}

	if res.Error != nil {
		return &e.Error{
			Code:     res.Error.Code,
			Level:    res.Error.Level,
			Message:  res.Error.Message,
			Metadata: res.Error.Metadata,
		}
	}

	if reply != nil && !opt.OneWay {
		err = r.serializer.Unmarshal(res.GetData(), reply)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *RemoteService) Loopback(ctx context.Context, rpcType protos.RPCType, route *route.Route, msg *message.Message) (*protos.Response, error) {
	req, err := cluster.BuildRequest(ctx, rpcType, nil, msg, r.server)
	if err != nil {
		return nil, err
	}
	subCtx, err := util.GetContextFromRequest(req, r.server.ID)
	if taskId := ctx.Value(constants.TaskIDKey); taskId != nil {
		subCtx = context.WithValue(subCtx, constants.TaskIDKey, taskId)
	}
	subCtx = util.StartSpanFromRequest(subCtx, r.server.ID, req.Msg.Route)
	subCtx = pcontext.AddToPropagateCtx(subCtx, constants.RequestShardKey, msg.ShardKey)
	defer tracing.FinishSpan(subCtx, err)

	if err != nil {
		logger.WithCtx(ctx).Warnf("[remote] failed to retrieve context from request: %s", err.Error())
	}

	// this span replicates the span generated by our nats or grpc clients
	parent, err := tracing.ExtractSpan(subCtx)
	tags := opentracing.Tags{
		"span.kind":       "loopback",
		"local.id":        r.server.ID,
		"peer.serverType": r.server.Type,
		"peer.id":         r.server.ID,
	}
	subCtx = tracing.StartSpan(subCtx, "Loopback RPC Call", tags, parent)
	defer tracing.FinishSpan(subCtx, err)
	if err != nil {
		logger.WithCtx(ctx).Warnf("[remote] failed to retrieve parent span: %s", err.Error())
	}

	var res *protos.Response
	if err == nil {
		res, err = r.dispatchRemoteMessage(subCtx, req, 5*time.Second)
		if err == nil {
			return res, nil
		}
	}

	res = &protos.Response{
		Error: &protos.Error{
			Code:    e.ErrInternalCode,
			Message: err.Error(),
		},
	}
	logger.WithCtx(ctx).Errorf("[remote] failed to process loopback message for route '%s': %v", req.Msg.Route, err)
	return res, err
}

func (r *RemoteService) dispatchRemoteMessage(
	ctx context.Context,
	req *protos.Request,
	timeout time.Duration,
) (*protos.Response, error) {
	h, err := r.handlerPool.getHandler(req.Type, req.Msg.Route)
	if err != nil {
		logger.WithCtx(ctx).Warnf("[remote] failed to get handler for route '%s': %v", req.Msg.Route, err)
		return &protos.Response{
			Error: &protos.Error{
				Code:    e.ErrNotFoundCode,
				Message: "route not found",
				Metadata: map[string]string{
					"route": req.Msg.Route,
				},
			},
		}, nil
	}

	var taskId string
	if h.Reentrant {
		taskId = r.taskSevice.NewAnonymousTaskId()
	} else {
		if req.Msg.ShardKey == "" {
			return &protos.Response{
				Error: &protos.Error{
					Code:    e.ErrInternalCode,
					Message: "shard key is required for non-reentrant methods",
				},
			}, nil
		}
		taskId = req.Msg.ShardKey
	}

	if req.Msg.Type == protos.MsgType_MsgNotify {
		err := r.taskSevice.Submit(ctx, taskId, func(tctx context.Context) {
			processRemoteMessage(tctx, req, r, h)
		})
		if err != nil {
			return nil, err
		}
		return &protos.Response{}, nil
	}

	result := make(chan *protos.Response, 1)
	err = r.taskSevice.Submit(ctx, taskId, func(tctx context.Context) {
		result <- processRemoteMessage(tctx, req, r, h)
	})
	if err != nil {
		return nil, err
	}

	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()

		select {
		case <-timer.C:
			return nil, constants.ErrRPCRequestTimeout
		case res := <-result:
			return res, nil
		}
	}

	return <-result, nil
}

func processRemoteMessage(ctx context.Context, req *protos.Request, r *RemoteService, handler *component.Handler) *protos.Response {
	switch req.Type {
	case protos.RPCType_Sys:
		return r.handleRPCSys(ctx, req, handler)
	case protos.RPCType_User:
		return r.handleRPCUser(ctx, req, handler)
	default:
		return &protos.Response{
			Error: &protos.Error{
				Code:    e.ErrBadRequestCode,
				Message: "invalid rpc type",
				Metadata: map[string]string{
					"route": req.GetMsg().GetRoute(),
				},
			},
		}
	}
}

func (r *RemoteService) handleRPCUser(ctx context.Context, req *protos.Request, handler *component.Handler) (response *protos.Response) {
	defer func() {
		if r := recover(); r != nil {
			logger.WithCtx(ctx).Errorf("panic: %v", r)
			response = &protos.Response{
				Error: &protos.Error{
					Code:    e.ErrInternalCode,
					Message: fmt.Sprintf("%v", r),
				},
			}
		}
	}()

	var err error
	prepare := func(ctx context.Context, arg interface{}) (context.Context, interface{}, error) {
		if !handler.Codec {
			if err := r.serializer.Unmarshal(req.Msg.Data, arg); err != nil {
				return ctx, nil, err
			}
		}

		ctx, arg, err = r.remoteHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
		if err != nil {
			return ctx, nil, err
		}

		return ctx, arg, nil
	}

	ret, err := handler.Fn(handler.Receiver, ctx, req.Msg.Data, prepare)
	ret, err = r.remoteHooks.AfterHandler.ExecuteAfterPipeline(ctx, ret, err)
	if err != nil {
		response = &protos.Response{
			Error: &protos.Error{},
		}
		code := e.ErrUnknownCode
		msg := err.Error()
		if val, ok := err.(e.PitayaError); ok {
			code = val.GetCode()
			msg = val.GetMessage()
			response.Error.Level = val.GetLevel()
			response.Error.Metadata = val.GetMetadata()
		}
		response.Error.Code = code
		response.Error.Message = msg
		logger.WithCtx(ctx).LogfWithErrorLevel(err, "RPC %s failed to process message: %s", req.Msg.Route, err.Error())
		return
	}

	var b []byte
	if ret != nil {
		pb, ok := ret.(proto.Message)
		if !ok {
			if b, ok = ret.([]byte); !ok {
				response = &protos.Response{
					Error: &protos.Error{
						Code:    e.ErrUnknownCode,
						Message: constants.ErrWrongValueType.Error(),
					},
				}
				return
			}
		} else if b, err = proto.Marshal(pb); err != nil {
			response = &protos.Response{
				Error: &protos.Error{
					Code:    e.ErrUnknownCode,
					Message: err.Error(),
				},
			}
			return
		}
	}

	response = &protos.Response{}
	response.Data = b
	return
}

func (r *RemoteService) handleRPCSys(ctx context.Context, req *protos.Request, handler *component.Handler) (response *protos.Response) {
	reply := req.GetMsg().GetReply()
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
		response = &protos.Response{
			Error: &protos.Error{
				Code:    e.ErrInternalCode,
				Message: err.Error(),
			},
		}
		return
	}

	ret, err := r.handlerPool.ProcessHandlerMessage(ctx, req.Msg.Route, r.serializer, r.handlerHooks, a.Session, req.GetMsg().GetData(), req.GetMsg().GetType(), true, handler)
	if err != nil {
		response = &protos.Response{
			Error: &protos.Error{},
		}
		code := e.ErrUnknownCode
		msg := err.Error()
		if val, ok := err.(e.PitayaError); ok {
			code = val.GetCode()
			msg = val.GetMessage()
			response.Error.Level = val.GetLevel()
			response.Error.Metadata = val.GetMetadata()
		}
		response.Error.Code = code
		response.Error.Message = msg
		logger.WithCtx(ctx).LogfWithErrorLevel(err, "Remote handler %s failed to process message: %s", req.Msg.Route, err.Error())
	} else {
		response = &protos.Response{Data: ret}
	}
	return
}

func (r *RemoteService) remoteCall(
	ctx context.Context,
	target *cluster.Server,
	rpcType protos.RPCType,
	route *route.Route,
	session session.Session,
	msg *message.Message,
) (*protos.Response, error) {
	rpcCtx := &cluster.RPCContext{
		Context: ctx,
		RPCType: rpcType,
		Route:   route,
		Session: session,
		Msg:     msg,
		Target:  target,
	}
	return r.interceptorChain.Execute(rpcCtx, r.invokeRPC)
}

// invokeRPC is the innermost invoker of the interceptor chain; it performs the
// actual RPC call through the underlying RPC client.
func (r *RemoteService) invokeRPC(c *cluster.RPCContext) (*protos.Response, error) {
	res, err := r.rpcClient.Call(c.Context, c.RPCType, c.Route, c.Session, c.Msg, c.Target)
	if err != nil {
		logger.Log.LogfWithErrorLevel(err, "error making call to target with id %s, route %s and host %s: %v", c.Target.ID, c.Msg.Route, c.Target.Hostname, err)
		return nil, err
	}
	return res, err
}
