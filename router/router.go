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

package router

import (
	"context"
	"strconv"

	"github.com/topfreegames/pitaya/v2/cluster"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/constants"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/route"
	"github.com/topfreegames/pitaya/v2/session"
)

// Router struct
type Router struct {
	server           *cluster.Server
	serviceDiscovery cluster.ServiceDiscovery
	routesMap        map[string]RoutingFunc
}

// RoutingFunc defines a routing function
type RoutingFunc func(
	ctx context.Context,
	rpcType protos.RPCType,
	route *route.Route,
	payload []byte,
) (context.Context, string, *cluster.Server, error)

type Session interface {
	GetId() int64
	GetUid() string
}

// New returns the router
func New() *Router {
	return &Router{
		routesMap: make(map[string]RoutingFunc),
	}
}

// SetServiceDiscovery sets the sd client
func (r *Router) SetServiceDiscovery(sd cluster.ServiceDiscovery) {
	r.serviceDiscovery = sd
}

// SetServer sets the server
func (r *Router) SetServer(server *cluster.Server) {
	r.server = server
}

func (r *Router) defaultRoute(
	ctx context.Context,
	route *route.Route,
) (context.Context, string, *cluster.Server, error) {
	if r.server != nil {
		sessionVal, ok := ctx.Value(constants.SessionCtxKey).(session.Session)
		if !ok {
			return ctx, route.Domain, r.server, nil
		}
		return ctx, strconv.FormatInt(sessionVal.ID(), 10), r.server, nil
	}

	servers, err := r.serviceDiscovery.GetServersByDomain(route.Domain)
	if err != nil {
		return ctx, "", nil, err
	}

	for _, srv := range servers {
		return ctx, route.Domain, srv, nil
	}

	return ctx, "", nil, constants.ErrNoServersAvailableOfType
}

// Resolve gets the right server to use in the call
func (r *Router) Resolve(
	ctx context.Context,
	rpcType protos.RPCType,
	route *route.Route,
	msg *message.Message,
) (context.Context, string, *cluster.Server, error) {
	if r.serviceDiscovery == nil {
		return ctx, "", nil, constants.ErrServiceDiscoveryNotInitialized
	}
	routeFunc, ok := r.routesMap[route.Domain]
	if !ok {
		logger.Log.Debugf("no specific route for svType: %s, using default route", route.Domain)
		return r.defaultRoute(ctx, route)
	}
	return routeFunc(ctx, rpcType, route, msg.Data)
}

// AddRoute adds a routing function to a server type
func (r *Router) AddRoute(
	domain string,
	routingFunction RoutingFunc,
) {
	if _, ok := r.routesMap[domain]; ok {
		logger.Log.Warnf("overriding the route to svType %s", domain)
	}
	r.routesMap[domain] = routingFunction
}
