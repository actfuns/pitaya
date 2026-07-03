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
	"errors"

	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/serialize"
	"github.com/topfreegames/pitaya/v2/util"
)

var errInvalidMsg = errors.New("invalid message type provided")

func getMsgType(msgTypeIface interface{}) (message.Type, error) {
	var msgType message.Type
	if val, ok := msgTypeIface.(message.Type); ok {
		msgType = val
	} else if val, ok := msgTypeIface.(protos.MsgType); ok {
		msgType = util.ConvertProtoToMessageType(val)
	} else {
		return msgType, errInvalidMsg
	}
	return msgType, nil
}

func serializeReturn(ser serialize.Serializer, ret interface{}) ([]byte, error) {
	res, err := util.SerializeOrRaw(ser, ret)
	if err != nil {
		logger.Log.Errorf("Failed to serialize return: %s", err.Error())
		res, err = util.GetErrorPayload(ser, err)
		if err != nil {
			logger.Log.Error("cannot serialize message and respond to the client ", err.Error())
			return nil, err
		}
	}
	return res, nil
}
