package service

import (
	"context"
	"fmt"

	"github.com/actfuns/pitaya/v2/component"
	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/constants"
	e "github.com/actfuns/pitaya/v2/errors"
	"github.com/actfuns/pitaya/v2/logger/interfaces"
	"github.com/actfuns/pitaya/v2/pipeline"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/prpc"
	"github.com/actfuns/pitaya/v2/serialize"
	"github.com/actfuns/pitaya/v2/session"
	"github.com/actfuns/pitaya/v2/util"
)

// HandlerPool ...
type HandlerPool struct {
	handlers map[string]*component.Handler // all handler method
	remotes  map[string]*component.Handler // all remote method
}

// NewHandlerPool ...
func NewHandlerPool() *HandlerPool {
	return &HandlerPool{
		handlers: make(map[string]*component.Handler),
		remotes:  make(map[string]*component.Handler),
	}
}

// Register ...
func (h *HandlerPool) Register(kind prpc.Kind, domain string, service string, method string, handler *component.Handler) {
	switch kind {
	case prpc.KindHandler:
		h.handlers[fmt.Sprintf("%s.%s.%s", domain, service, method)] = handler
	case prpc.KindRPC:
		h.remotes[fmt.Sprintf("%s.%s.%s", domain, service, method)] = handler
	}
}

// GetHandlers ...
func (h *HandlerPool) GetHandlers(kind prpc.Kind) map[string]*component.Handler {
	switch kind {
	case prpc.KindHandler:
		return h.handlers
	case prpc.KindRPC:
		return h.remotes
	}
	return nil
}

// ProcessHandlerMessage ...
func (h *HandlerPool) ProcessHandlerMessage(
	ctx context.Context,
	route string,
	serializer serialize.Serializer,
	handlerHooks *pipeline.HandlerHooks,
	session session.Session,
	data []byte,
	msgTypeIface interface{},
	remote bool,
	handler *component.Handler,
) (ret []byte, err error) {
	if ctx == nil {
		ctx = context.Background()
	}

	if session != nil {
		ctx = context.WithValue(ctx, constants.SessionCtxKey, session)
		ctx = util.CtxWithDefaultLogger(ctx, route, session.UID())
	}

	if handler == nil {
		handler, err = h.getHandler(protos.RPCType_Sys, route)
		if err != nil {
			return nil, e.NewError(err, e.ErrNotFoundCode)
		}
	}

	msgType, err := getMsgType(msgTypeIface)
	if err != nil {
		return nil, e.NewError(err, e.ErrInternalCode)
	}

	logger := ctx.Value(constants.LoggerCtxKey).(interfaces.Logger)
	defer func() {
		if r := recover(); r != nil {
			logger.Errorf("panic: %v", r)
			ret = nil
			err = e.NewError(fmt.Errorf("%v", r), e.ErrInternalCode)
		}
	}()

	prepare := func(ctx context.Context, arg interface{}) (context.Context, interface{}, error) {
		if !handler.Codec {
			if err := serializer.Unmarshal(data, arg); err != nil {
				return ctx, nil, err
			}
		}

		ctx, arg, err = handlerHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
		if err != nil {
			return ctx, nil, err
		}

		return ctx, arg, nil
	}

	resp, err := handler.Fn(handler.Receiver, ctx, data, prepare)
	if remote && msgType == message.Notify {
		// This is a special case and should only happen with nats rpc client
		// because we used nats request we have to answer to it or else a timeout
		// will happen in the caller server and will be returned to the client
		// the reason why we don't just Publish is to keep track of failed rpc requests
		// with timeouts, maybe we can improve this flow
		resp = []byte{}
	}

	resp, err = handlerHooks.AfterHandler.ExecuteAfterPipeline(ctx, resp, err)
	if err != nil {
		return nil, err
	}

	ret, err = serializeReturn(serializer, resp)
	if err != nil {
		return nil, err
	}

	return ret, nil
}

func (h *HandlerPool) getHandler(rpcType protos.RPCType, route string) (*component.Handler, error) {
	switch rpcType {
	case protos.RPCType_Sys:
		handler, ok := h.handlers[route]
		if !ok {
			e := fmt.Errorf("pitaya/handler: %s not found", route)
			return nil, e
		}
		return handler, nil
	case protos.RPCType_User:
		handler, ok := h.remotes[route]
		if !ok {
			e := fmt.Errorf("pitaya/remote: %s not found", route)
			return nil, e
		}
		return handler, nil
	default:
		return nil, fmt.Errorf("pitaya/handler: unknown kind: %d", rpcType)
	}
}
