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
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/topfreegames/pitaya/v2/component"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/pipeline"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/protos/test"
	"github.com/topfreegames/pitaya/v2/serialize/mocks"
	"go.uber.org/mock/gomock"
)

type TestType struct {
	component.Base
}

func (t *TestType) HandlerNil(context.Context)                              {}
func (t *TestType) HandlerRaw(ctx context.Context, msg []byte)              {}
func (t *TestType) HandlerPointer(ctx context.Context, ss *test.SomeStruct) {}
func (t *TestType) HandlerPointerRaw(ctx context.Context, ss *test.SomeStruct) ([]byte, error) {
	return []byte("ok"), nil
}
func (t *TestType) HandlerPointerStruct(ctx context.Context, ss *test.SomeStruct) (*test.SomeStruct, error) {
	return &test.SomeStruct{A: 1, B: "ok"}, nil
}
func (t *TestType) HandlerPointerErr(ctx context.Context, ss *test.SomeStruct) ([]byte, error) {
	return nil, errors.New("HandlerPointerErr")
}

func TestGetMsgType(t *testing.T) {
	t.Parallel()
	tables := []struct {
		name    string
		in      interface{}
		msgType message.Type
		err     error
	}{
		{"request", message.Request, message.Request, nil},
		{"notify", message.Notify, message.Notify, nil},
		{"response", message.Response, message.Response, nil},
		{"push", message.Push, message.Push, nil},
		{"protos_request", protos.MsgType_MsgRequest, message.Request, nil},
		{"protos_notify", protos.MsgType_MsgNotify, message.Notify, nil},
		{"invalid", "oops", message.Request, errInvalidMsg},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			msgType, err := getMsgType(table.in)
			assert.Equal(t, table.err, err)
			assert.Equal(t, table.msgType, msgType)
		})
	}
}

func TestExecuteBeforePipelineEmpty(t *testing.T) {
	expected := []byte("ok")
	beforeHandler := pipeline.NewChannel()
	_, res, err := beforeHandler.ExecuteBeforePipeline(context.Background(), expected)
	assert.NoError(t, err)
	assert.Equal(t, expected, res)
}

func TestExecuteBeforePipelineSuccess(t *testing.T) {
	c := context.Background()
	data := []byte("ok")
	expected1 := []byte("oh noes 1")
	expected2 := []byte("oh noes 2")
	before1 := func(ctx context.Context, in interface{}) (context.Context, interface{}, error) {
		assert.Equal(t, c, ctx)
		assert.Equal(t, data, in)
		return ctx, expected1, nil
	}
	before2 := func(ctx context.Context, in interface{}) (context.Context, interface{}, error) {
		assert.Equal(t, c, ctx)
		assert.Equal(t, expected1, in)
		return ctx, expected2, nil
	}

	beforeHandler := pipeline.NewChannel()
	beforeHandler.PushBack(before1)
	beforeHandler.PushBack(before2)
	defer beforeHandler.Clear()

	_, res, err := beforeHandler.ExecuteBeforePipeline(c, data)
	assert.NoError(t, err)
	assert.Equal(t, expected2, res)
}

func TestExecuteBeforePipelineError(t *testing.T) {
	c := context.Background()
	expected := errors.New("oh noes")
	before := func(ctx context.Context, in interface{}) (context.Context, interface{}, error) {
		assert.Equal(t, c, ctx)
		return ctx, nil, expected
	}
	beforeHandler := pipeline.NewChannel()
	beforeHandler.PushFront(before)
	defer beforeHandler.Clear()

	_, _, err := beforeHandler.ExecuteBeforePipeline(c, []byte("ok"))
	assert.Equal(t, expected, err)
}

func TestExecuteAfterPipelineEmpty(t *testing.T) {
	expected := []byte("whatever")
	afterHandler := pipeline.NewAfterChannel()
	res, err := afterHandler.ExecuteAfterPipeline(context.Background(), expected, nil)
	assert.Equal(t, expected, res)
	assert.Nil(t, err)
}

func TestExecuteAfterPipelineSuccess(t *testing.T) {
	c := context.Background()
	data := []byte("ok")
	expected1 := []byte("oh noes 1")
	expected2 := []byte("oh noes 2")
	err0 := errors.New("start with this")
	err1 := errors.New("send this error")
	after1 := func(ctx context.Context, out interface{}, err error) (interface{}, error) {
		assert.Equal(t, c, ctx)
		assert.Equal(t, data, out)
		assert.Equal(t, err0, err)
		return expected1, err1
	}
	after2 := func(ctx context.Context, out interface{}, err error) (interface{}, error) {
		assert.Equal(t, c, ctx)
		assert.Equal(t, expected1, out)
		assert.Equal(t, err1, err)
		return expected2, nil
	}
	afterHandler := pipeline.NewAfterChannel()
	afterHandler.PushBack(after1)
	afterHandler.PushBack(after2)
	defer afterHandler.Clear()

	res, err := afterHandler.ExecuteAfterPipeline(c, []byte("ok"), err0)
	assert.Equal(t, expected2, res)
	assert.Nil(t, err)
}

func TestExecuteAfterPipelineError(t *testing.T) {
	c := context.Background()
	after := func(ctx context.Context, out interface{}, err error) (interface{}, error) {
		assert.Equal(t, c, ctx)
		return nil, errors.New("oh noes")
	}
	afterHandler := pipeline.NewAfterChannel()
	afterHandler.PushFront(after)
	defer afterHandler.Clear()

	res, err := afterHandler.ExecuteAfterPipeline(c, []byte("ok"), nil)
	assert.Nil(t, res)
	assert.Equal(t, errors.New("oh noes"), err)
}

func TestSerializeReturn(t *testing.T) {
	tables := []struct {
		name               string
		isRawArg           bool
		in                 interface{}
		out                []byte
		errSerialize       error
		errGetErrorPayload error
	}{
		{"raw_arg", true, []byte("hello"), []byte("hello"), nil, nil},
		{"success", false, test.SomeStruct{A: 1, B: "hello"}, []byte("hello"), nil, nil},
		{"serialize_fail", false, test.SomeStruct{A: 1, B: "hello"}, nil, errors.New("some error"), nil},
		{"serialize_fail_err_payload", false, test.SomeStruct{A: 1, B: "hello"}, nil, errors.New("some error"), errors.New("some other error")},
	}

	for _, table := range tables {
		t.Run(table.name, func(t *testing.T) {
			ctrl := gomock.NewController(t)
			defer ctrl.Finish()
			mockSerializer := mocks.NewMockSerializer(ctrl)
			if !table.isRawArg {
				mockSerializer.EXPECT().Marshal(gomock.Any()).Return(table.out, table.errSerialize)
				if table.errSerialize != nil {
					mockSerializer.EXPECT().Marshal(gomock.Any()).Return(table.out, table.errGetErrorPayload)
				}
			}
			out, err := serializeReturn(mockSerializer, table.in)
			assert.Equal(t, table.out, out)
			assert.Equal(t, table.errGetErrorPayload, err)
		})
	}
}
