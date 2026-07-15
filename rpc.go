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

package pitaya

import (
	"context"
	"reflect"
	"slices"

	"github.com/actfuns/pitaya/v2/constants"
	pcontext "github.com/actfuns/pitaya/v2/context"
	e "github.com/actfuns/pitaya/v2/errors"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/prpc"
	"github.com/actfuns/pitaya/v2/route"
	"google.golang.org/protobuf/proto"
)

const defaultMaxRPCRedirects = 3

// RPC calls a method in a different server
func (app *App) RPC(ctx context.Context, routeStr string, reply proto.Message, arg proto.Message, opts ...prpc.CallOption) error {
	opt := applyOptions(opts)

	ctx = pcontext.AddPairsToPropagateCtx(ctx, opt.PropagateCtx...)

	if opt.Reliable {
		meta := opt.ReliableMetadata
		if meta == nil {
			meta = make(map[string]interface{})
		}
		storedOpt := opt
		storedOpt.Reliable = false
		storedOpt.ReliableMetadata = nil
		storedOpt.ReliableEnqueueOpts = nil
		meta[constants.ReliableRPCOptionsKey] = storedOpt

		if opt.ReliableEnqueueOpts != nil {
			_, err := app.worker.EnqueueRPCWithOptions(routeStr, meta, reply, arg, opt.ReliableEnqueueOpts)
			return err
		}
		_, err := app.worker.EnqueueRPC(routeStr, meta, reply, arg)
		return err
	}

	rpcType := protos.RPCType_User
	if opt.Client {
		rpcType = protos.RPCType_Sys
	}

	return app.doSendRPC(ctx, rpcType, opt.ServerID, routeStr, reply, arg, opt)
}

func (app *App) doSendRPC(ctx context.Context, rpcType protos.RPCType, serverID, routeStr string, reply proto.Message, arg proto.Message, opt prpc.CallOptions) error {
	if app.rpcServer == nil {
		return constants.ErrRPCServerNotInitialized
	}

	if reflect.TypeOf(reply).Kind() != reflect.Ptr {
		return constants.ErrReplyShouldBePtr
	}

	r, err := route.Decode(routeStr)
	if err != nil {
		return err
	}

	if r.Domain == "" {
		return constants.ErrNoServerTypeChosenForRPC
	}

	if ((slices.Contains(app.server.Domains, r.Domain) && serverID == "") || serverID == app.server.ID) && !app.server.IsLoopbackEnabled() {
		return constants.ErrNonsenseRPC
	}

	return app.doSendRPCWithRetry(ctx, rpcType, serverID, r, reply, arg, opt, 0)
}

func (app *App) doSendRPCWithRetry(ctx context.Context, rpcType protos.RPCType, serverID string, r *route.Route, reply proto.Message, arg proto.Message, opt prpc.CallOptions, retryCount int) error {
	err := app.remoteService.RPC(ctx, rpcType, serverID, r, reply, arg, opt)
	if err != nil {
		if pitayaErr, ok := err.(*e.Error); ok && pitayaErr.GetCode() == e.ErrRPCRedirect {
			if newServerID := pitayaErr.GetMetadata()["server_id"]; newServerID != "" && newServerID != serverID && retryCount < defaultMaxRPCRedirects {
				return app.doSendRPCWithRetry(ctx, rpcType, newServerID, r, reply, arg, opt, retryCount+1)
			}
		}
	}
	return err
}

func applyOptions(opts []prpc.CallOption) prpc.CallOptions {
	cfg := prpc.CallOptions{}
	for _, opt := range opts {
		opt(&cfg)
	}
	return cfg
}
