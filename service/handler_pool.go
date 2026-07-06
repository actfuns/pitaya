package service

import (
	"context"
	"fmt"

	"github.com/topfreegames/pitaya/v2/component"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/constants"
	e "github.com/topfreegames/pitaya/v2/errors"
	"github.com/topfreegames/pitaya/v2/logger/interfaces"
	"github.com/topfreegames/pitaya/v2/pipeline"
	"github.com/topfreegames/pitaya/v2/serialize"
	"github.com/topfreegames/pitaya/v2/session"
	"github.com/topfreegames/pitaya/v2/util"
)

// HandlerPool ...
type HandlerPool struct {
	handlers map[string]*component.Handler // all handler method
}

// NewHandlerPool ...
func NewHandlerPool() *HandlerPool {
	return &HandlerPool{
		handlers: make(map[string]*component.Handler),
	}
}

// Register ...
func (h *HandlerPool) Register(domain string, service string, method string, handler *component.Handler) {
	h.handlers[fmt.Sprintf("%s.%s.%s", domain, service, method)] = handler
}

// GetHandlers ...
func (h *HandlerPool) GetHandlers() map[string]*component.Handler {
	return h.handlers
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
	ctx = context.WithValue(ctx, constants.SessionCtxKey, session)
	ctx = util.CtxWithDefaultLogger(ctx, route, session.UID())

	if handler == nil {
		handler, err = h.getHandler(route)
		if err != nil {
			return nil, e.NewError(err, e.ErrNotFoundCode)
		}
	}

	if !handler.Client {
		return nil, e.NewError(err, e.ErrNotFoundCode)
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

	pre := func(ctx context.Context, arg interface{}) (context.Context, interface{}, error) {
		if err := serializer.Unmarshal(data, arg); err != nil {
			return ctx, nil, err
		}

		ctx, arg, err = handlerHooks.BeforeHandler.ExecuteBeforePipeline(ctx, arg)
		if err != nil {
			return ctx, nil, err
		}

		return ctx, arg, nil
	}
	logger.Debugf("SID=%d, Data=%s", session.ID(), data)

	resp, err := handler.Fn(handler.Receiver, ctx, pre)
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

func (h *HandlerPool) getHandler(route string) (*component.Handler, error) {
	handler, ok := h.handlers[route]
	if !ok {
		e := fmt.Errorf("pitaya/handler: %s not found", route)
		return nil, e
	}
	return handler, nil
}
