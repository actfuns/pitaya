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
	"fmt"
	"testing"

	"github.com/actfuns/pitaya/v2/component"
	"github.com/actfuns/pitaya/v2/conn/message"
	e "github.com/actfuns/pitaya/v2/errors"
	"github.com/actfuns/pitaya/v2/pipeline"
	"github.com/actfuns/pitaya/v2/protos/test"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/actfuns/pitaya/v2/serialize/mocks"
	session_mocks "github.com/actfuns/pitaya/v2/session/mocks"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"go.uber.org/mock/gomock"
)

func TestGetHandlerExists(t *testing.T) {
	rt := route.NewRoute("", uuid.New().String(), uuid.New().String())
	expected := &component.Handler{}
	handlerPool := NewHandlerPool()
	handlerPool.handlers[rt.String()] = expected
	defer func() { delete(handlerPool.handlers, rt.String()) }()

	h, err := handlerPool.getHandler(rt.String())
	assert.NoError(t, err)
	assert.Equal(t, expected, h)
}

func TestGetHandlerDoesntExist(t *testing.T) {
	rt := route.NewRoute("", uuid.New().String(), uuid.New().String())
	handlerPool := NewHandlerPool()
	h, err := handlerPool.getHandler(rt.String())
	assert.Nil(t, h)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), fmt.Sprintf("%s not found", rt.String()))
}

func TestProcessHandlerMessage(t *testing.T) {
	tObj := &TestType{}

	handlerPool := NewHandlerPool()

	rt := route.NewRoute("domain", uuid.New().String(), uuid.New().String())
	handlerPool.handlers[rt.String()] = &component.Handler{
		Receiver: tObj,
		Client:   true,
		Fn: func(srv interface{}, ctx context.Context, data []byte, prepare func(ctx context.Context, arg interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			arg := &test.SomeStruct{}
			if _, _, err := prepare(ctx, arg); err != nil {
				return nil, err
			}
			return tObj.HandlerPointerRaw(ctx, arg)
		},
	}

	rtErr := route.NewRoute("domain", uuid.New().String(), uuid.New().String())
	handlerPool.handlers[rtErr.String()] = &component.Handler{
		Receiver: tObj,
		Client:   true,
		Fn: func(srv interface{}, ctx context.Context, data []byte, prepare func(ctx context.Context, arg interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			arg := &test.SomeStruct{}
			if _, _, err := prepare(ctx, arg); err != nil {
				return nil, err
			}
			return tObj.HandlerPointerErr(ctx, arg)
		},
	}

	rtSt := route.NewRoute("domain", uuid.New().String(), uuid.New().String())
	handlerPool.handlers[rtSt.String()] = &component.Handler{
		Receiver: tObj,
		Client:   true,
		Fn: func(srv interface{}, ctx context.Context, data []byte, prepare func(ctx context.Context, arg interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			arg := &test.SomeStruct{}
			if _, _, err := prepare(ctx, arg); err != nil {
				return nil, err
			}
			return tObj.HandlerPointerStruct(ctx, arg)
		},
	}

	tables := []struct {
		name         string
		route        *route.Route
		errSerReturn error
		errSerialize error
		outSerialize interface{}
		msgType      interface{}
		remote       bool
		out          []byte
		err          error
	}{
		{"invalid_route", route.NewRoute("", "no", "no"), nil, nil, nil, nil, false, nil, e.NewError(errors.New("pitaya/handler: .no.no not found"), e.ErrNotFoundCode)},
		{"invalid_msg_type", rt, nil, nil, nil, nil, false, nil, e.NewError(errInvalidMsg, e.ErrInternalCode)},
		{"failed_handle_args_unmarshal", rt, nil, errors.New("some error"), &test.SomeStruct{}, message.Request, false, nil, errors.New("some error")},
		{"failed_pcall", rtErr, nil, nil, &test.SomeStruct{A: 1, B: "ok"}, message.Request, false, nil, errors.New("HandlerPointerErr")},
		{"failed_serialize_return", rtSt, errors.New("ser ret error"), nil, &test.SomeStruct{A: 1, B: "ok"}, message.Request, false, []byte("failed"), nil},
		{"ok", rt, nil, nil, &test.SomeStruct{}, message.Request, false, []byte("ok"), nil},
		{"notify_on_request", rt, nil, nil, &test.SomeStruct{}, message.Notify, false, []byte("ok"), nil},
		{"remote_notify", rt, nil, nil, &test.SomeStruct{}, message.Notify, true, []byte{}, nil},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			ss := session_mocks.NewMockSession(ctrl)
			ss.EXPECT().UID().Return("uid").AnyTimes()
			ss.EXPECT().ID().Return(int64(1)).AnyTimes()
			mockSerializer := mocks.NewMockSerializer(ctrl)
			if table.outSerialize != nil {
				mockSerializer.EXPECT().Unmarshal(gomock.Any(), gomock.Any()).Return(table.errSerialize).Do(
					func(p []byte, arg interface{}) {
						arg = table.outSerialize
					})

				if table.errSerReturn != nil {
					mockSerializer.EXPECT().Marshal(gomock.Any()).Return(table.out, table.errSerReturn)
					mockSerializer.EXPECT().Marshal(gomock.Any()).Return(table.out, nil)
				}
			}
			handlerHooks := pipeline.NewHandlerHooks()
			out, err := handlerPool.ProcessHandlerMessage(context.Background(), table.route.String(), mockSerializer, handlerHooks, ss, nil, table.msgType, table.remote, nil)
			assert.Equal(t, table.out, out)
			assert.Equal(t, table.err, err)
		})
	}
}

func TestProcessHandlerMessageBrokenBeforePipeline(t *testing.T) {
	ctrl := gomock.NewController(t)
	rt := route.NewRoute("", uuid.New().String(), uuid.New().String())
	handlerPool := NewHandlerPool()
	handlerPool.handlers[rt.String()] = &component.Handler{
		Client: true,
		Fn: func(srv interface{}, ctx context.Context, data []byte, prepare func(ctx context.Context, arg interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			arg := &test.SomeStruct{}
			if _, _, err := prepare(ctx, arg); err != nil {
				return nil, err
			}
			return []byte("ok"), nil
		},
	}
	expected := errors.New("oh noes")
	before := func(ctx context.Context, in interface{}) (context.Context, interface{}, error) {
		return ctx, nil, expected
	}
	beforeHandler := pipeline.NewChannel()
	beforeHandler.PushFront(before)

	handlerHooks := pipeline.NewHandlerHooks()
	handlerHooks.BeforeHandler = beforeHandler
	ss := session_mocks.NewMockSession(ctrl)
	ss.EXPECT().UID().Return("uid").AnyTimes()
	ss.EXPECT().ID().Return(int64(1)).AnyTimes()
	mockSerializer := mocks.NewMockSerializer(ctrl)
	mockSerializer.EXPECT().Unmarshal(gomock.Any(), gomock.Any()).Return(nil)
	out, err := handlerPool.ProcessHandlerMessage(context.Background(), rt.String(), mockSerializer, handlerHooks, ss, nil, message.Request, false, nil)
	assert.Nil(t, out)
	assert.Equal(t, expected, err)
}

func TestProcessHandlerMessageBrokenAfterPipeline(t *testing.T) {
	tObj := &TestType{}
	rt := route.NewRoute("", uuid.New().String(), uuid.New().String())
	handlerPool := NewHandlerPool()
	handlerPool.handlers[rt.String()] = &component.Handler{
		Receiver: tObj,
		Client:   true,
		Fn: func(srv interface{}, ctx context.Context, data []byte, prepare func(ctx context.Context, arg interface{}) (context.Context, interface{}, error)) (interface{}, error) {
			arg := &test.SomeStruct{}
			if _, _, err := prepare(ctx, arg); err != nil {
				return nil, err
			}
			return tObj.HandlerPointerRaw(ctx, arg)
		},
	}

	after := func(ctx context.Context, out interface{}, err error) (interface{}, error) {
		return nil, errors.New("oh noes")
	}
	afterHandler := pipeline.NewAfterChannel()
	afterHandler.PushFront(after)

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	ss := session_mocks.NewMockSession(ctrl)
	ss.EXPECT().UID().Return("uid").AnyTimes()
	ss.EXPECT().ID().Return(int64(1)).AnyTimes()

	mockSerializer := mocks.NewMockSerializer(ctrl)
	mockSerializer.EXPECT().Unmarshal(gomock.Any(), gomock.Any()).Return(nil).Do(
		func(p []byte, arg interface{}) {
			arg = &test.SomeStruct{}
		})

	handlerHooks := pipeline.NewHandlerHooks()
	handlerHooks.AfterHandler = afterHandler
	out, err := handlerPool.ProcessHandlerMessage(context.Background(), rt.String(), mockSerializer, handlerHooks, ss, nil, message.Request, false, nil)
	assert.Nil(t, out)
	assert.Equal(t, errors.New("oh noes"), err)
}
