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

package context

import (
	"context"
	"encoding/json"

	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
)

// AddToPropagateCtx adds a key and value that will be propagated through RPC calls
func AddToPropagateCtx(ctx context.Context, key string, val interface{}) context.Context {
	propagate := ToMap(ctx)
	propagate[key] = val
	return context.WithValue(ctx, constants.PropagateCtxKey, propagate)
}

// GetFromPropagateCtx get a value from the propagate
func GetFromPropagateCtx(ctx context.Context, key string) interface{} {
	propagate := ToMap(ctx)
	if val, ok := propagate[key]; ok {
		return val
	}
	return nil
}

func FromPropagateContext(ctx context.Context) (map[string]interface{}, bool) {
	if ctx == nil {
		return nil, false
	}
	p, ok := ctx.Value(constants.PropagateCtxKey).(map[string]interface{})
	if !ok {
		return nil, false
	}
	out := make(map[string]interface{}, len(p))
	for k, v := range p {
		if !isSafeToCopy(v) {
			logger.WithCtx(ctx).Warnf("propagate key %s is not safe to copy", k)
			continue
		}
		out[k] = v
	}
	return out, true
}

func NewPropagateContext(ctx context.Context, md map[string]interface{}) context.Context {
	if md == nil {
		return ctx
	}
	return context.WithValue(ctx, constants.PropagateCtxKey, md)
}

// ToMap returns the values that will be propagated through RPC calls in map[string]interface{} format
func ToMap(ctx context.Context) map[string]interface{} {
	if ctx == nil {
		return map[string]interface{}{}
	}
	p := ctx.Value(constants.PropagateCtxKey)
	if p != nil {
		return p.(map[string]interface{})
	}
	return map[string]interface{}{}
}

// FromMap creates a new context from a map with propagated values
func FromMap(val map[string]interface{}) context.Context {
	return context.WithValue(context.Background(), constants.PropagateCtxKey, val)
}

// Encode returns the given propagatable context encoded in binary format
func Encode(ctx context.Context) ([]byte, error) {
	m := ToMap(ctx)
	if len(m) > 0 {
		return json.Marshal(m)
	}
	return nil, nil
}

// Decode returns a context given a binary encoded message
func Decode(m []byte) (context.Context, error) {
	if len(m) == 0 {
		// TODO maybe return an error
		return nil, nil
	}
	mp := make(map[string]interface{}, 0)
	err := json.Unmarshal(m, &mp)
	if err != nil {
		return nil, err
	}
	return FromMap(mp), nil
}

func isSafeToCopy(v interface{}) bool {
	switch v.(type) {
	case string,
		int, int8, int16, int32, int64,
		uint, uint8, uint16, uint32, uint64, uintptr,
		float32, float64,
		complex64, complex128,
		bool, nil:
		return true
	default:
		return false
	}
}
