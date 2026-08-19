package cluster

import (
	"context"
	"sync"

	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/actfuns/pitaya/v2/session"
)

// UnaryInvoker performs the actual RPC call. It is the innermost part of the
// chain and is never meant to be implemented by users; it is the plain
// RPCClient.Call wrapped by RemoteService.
type UnaryInvoker func(c *RPCContext) (*protos.Response, error)

// UnaryInterceptor wraps a downstream handler in the spirit of a functional
// middleware (like http.Handler or grpc.UnaryServerInterceptor). It receives
// the next handler in the chain and returns its own handler. Interceptors may:
//   - mutate the call parameters by writing to the fields of the *RPCContext
//     passed to the returned handler (e.g. c.Target, c.Route, c.Msg, c.Context)
//     before invoking next;
//   - retry by calling next(c) more than once;
//   - short-circuit by returning a response/error without calling next.
type UnaryInterceptor func(next UnaryInvoker) UnaryInvoker

// RPCContext carries the state of a single outbound RPC call through the
// interceptor chain. The call parameters are flattened onto the context so
// interceptors can read or mutate them directly.
type RPCContext struct {
	context.Context

	RPCType protos.RPCType
	Route   *route.Route
	Session session.Session
	Msg     *message.Message
	Target  *Server
}

// InterceptorChain holds the ordered list of unary interceptors.
// Interceptors registered first are the outermost ones, following the same
// onion model as gin and gRPC.
type InterceptorChain struct {
	interceptors []UnaryInterceptor

	composeOnce sync.Once
	compose     func(UnaryInvoker) UnaryInvoker
}

// NewInterceptorChain creates an empty interceptor chain.
func NewInterceptorChain() *InterceptorChain {
	return &InterceptorChain{}
}

// Add appends interceptors to the chain.
// Should not be used after the server is running.
func (c *InterceptorChain) Add(interceptors ...UnaryInterceptor) {
	c.interceptors = append(c.interceptors, interceptors...)
}

// composeFn returns a function that, given an invoker, returns the fully
// composed handler. The composition is computed once and cached, so repeated
// Execute calls do not rebuild the closure chain on every RPC.
func (c *InterceptorChain) composeFn() func(UnaryInvoker) UnaryInvoker {
	c.composeOnce.Do(func() {
		interceptors := c.interceptors
		c.compose = func(invoker UnaryInvoker) UnaryInvoker {
			handler := invoker
			for i := len(interceptors) - 1; i >= 0; i-- {
				handler = interceptors[i](handler)
			}
			return handler
		}
	})
	return c.compose
}

// Execute runs the interceptor chain followed by the actual RPC invoker.
// Interceptors are composed functionally from the innermost (the invoker)
// outward, so the first registered interceptor is the outermost wrapper.
// A nil chain or an empty chain simply runs the invoker.
//
// A single RPCContext must not be reused across multiple Execute calls.
func (c *InterceptorChain) Execute(rpcCtx *RPCContext, invoker UnaryInvoker) (*protos.Response, error) {
	if rpcCtx.Context == nil {
		rpcCtx.Context = context.Background()
	}

	handler := invoker
	if c != nil {
		handler = c.composeFn()(invoker)
	}

	return handler(rpcCtx)
}
