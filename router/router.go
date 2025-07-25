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
	"fmt"
	"math/rand"
	"time"

	"github.com/topfreegames/pitaya/v2/cluster"
	"github.com/topfreegames/pitaya/v2/conn/message"
	"github.com/topfreegames/pitaya/v2/constants"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"github.com/topfreegames/pitaya/v2/logger"
	"github.com/topfreegames/pitaya/v2/protos"
	"github.com/topfreegames/pitaya/v2/route"
)

// Router struct
type Router struct {
	serviceDiscovery cluster.ServiceDiscovery
	routesMap        map[string]RoutingFunc
	dispatch         DispatchFunc
}

// RoutingFunc defines a routing function
type RoutingFunc func(
	ctx context.Context,
	rpcType protos.RPCType,
	route *route.Route,
	payload []byte,
	servers map[string]*cluster.Server,
) (*cluster.Server, error)

type Session interface {
	GetId() int64
	GetUid() string
}
type DispatchFunc func(route *route.Route, data []byte) (string, error)

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

func (r *Router) defaultRoute(
	servers map[string]*cluster.Server,
) *cluster.Server {
	srvList := make([]*cluster.Server, 0)
	s := rand.NewSource(time.Now().Unix())
	rnd := rand.New(s)
	for _, v := range servers {
		srvList = append(srvList, v)
	}
	server := srvList[rnd.Intn(len(srvList))]
	return server
}

// Route gets the right server to use in the call
func (r *Router) Route(
	ctx context.Context,
	rpcType protos.RPCType,
	svType string,
	route *route.Route,
	msg *message.Message,
) (*cluster.Server, error) {
	if r.serviceDiscovery == nil {
		return nil, constants.ErrServiceDiscoveryNotInitialized
	}
	serversOfType, err := r.serviceDiscovery.GetServersByType(svType)
	if err != nil {
		return nil, err
	}
	if rpcType == protos.RPCType_User {
		val := pcontext.GetFromPropagateCtx(ctx, constants.RouteCustomKey)
		if val == nil {
			server := r.defaultRoute(serversOfType)
			return server, nil
		}
	}
	routeFunc, ok := r.routesMap[svType]
	if !ok {
		logger.Log.Debugf("no specific route for svType: %s, using default route", svType)
		server := r.defaultRoute(serversOfType)
		return server, nil
	}
	return routeFunc(ctx, rpcType, route, msg.Data, serversOfType)
}

// AddRoute adds a routing function to a server type
func (r *Router) AddRoute(
	serverType string,
	routingFunction RoutingFunc,
) {
	if _, ok := r.routesMap[serverType]; ok {
		logger.Log.Warnf("overriding the route to svType %s", serverType)
	}
	r.routesMap[serverType] = routingFunction
}

// defaultDispatch returns a random dispatch id
func (r *Router) defaultDispatch() (string, error) {
	return fmt.Sprintf("%d", rand.Int63n(1000000)), nil
}

// Dispatch gets the right server to use in the call
func (r *Router) Dispatch(route *route.Route, data []byte) (string, error) {
	if r.dispatch != nil {
		return r.dispatch(route, data)
	}
	return r.defaultDispatch()
}

// SetDispatch sets the dispatch function
func (r *Router) SetDispatch(dispatchFunc DispatchFunc) {
	r.dispatch = dispatchFunc
}
