package cluster

import (
	"context"

	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/actfuns/pitaya/v2/session"
)

// UnaryInvoker performs the actual RPC call. It is the innermost part of the
// chain and is never meant to be implemented by users; it is the plain
// RPCClient.Call wrapped by RemoteService.
type UnaryInvoker func(c *RPCContext) (*protos.Response, error)

// UnaryInterceptor processes a single outbound RPC call in the spirit of a gin
// middleware: it may inspect or mutate the call state, and must call c.Next()
// to let the rest of the chain (and the actual RPC) run. Code placed after
// c.Next() runs once the downstream chain has completed.
type UnaryInterceptor func(c *RPCContext)

// RPCContext carries the state of a single outbound RPC call through the
// interceptor chain, modeled after gin.Context. The call parameters are
// flattened onto the context so interceptors can read or mutate them directly.
type RPCContext struct {
	context.Context

	RPCType protos.RPCType
	Route   *route.Route
	Session session.Session
	Msg     *message.Message
	Target  *Server

	// Response and Error are populated by the actual RPC call at the end of
	// the chain. Interceptors may set them before aborting to short-circuit.
	Response *protos.Response
	Error    error

	chain   []UnaryInterceptor
	index   int
	aborted bool
}

// Next advances to the next interceptor in the chain. Interceptors must call
// it exactly once unless they abort. Code placed after c.Next() runs when the
// downstream chain (and the actual RPC call) has completed.
func (c *RPCContext) Next() {
	if c.aborted {
		return
	}
	c.index++
	for c.index < len(c.chain) && !c.aborted {
		c.chain[c.index](c)
		c.index++
	}
}

// Abort short-circuits the chain: the remaining interceptors and the actual
// RPC call are skipped. Interceptors that abort should set Response or Error
// beforehand. Handlers already on the call stack still finish their post-Next
// code, mirroring gin.Abort.
func (c *RPCContext) Abort() {
	c.aborted = true
	c.index = len(c.chain)
}

// Aborted reports whether the chain was aborted.
func (c *RPCContext) Aborted() bool {
	return c.aborted
}

// InterceptorChain holds the ordered list of unary interceptors.
// Interceptors registered first are the outermost ones, following the same
// onion model as gin and gRPC.
type InterceptorChain struct {
	interceptors []UnaryInterceptor
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

// Execute runs the interceptor chain followed by the actual RPC invoker.
// The invoker is appended as the final link of the chain so that post-Next
// code in every interceptor observes the completed call. A nil chain or an
// empty chain simply runs the invoker. The call result is always written back
// to rpcCtx.Response / rpcCtx.Error.
//
// A single RPCContext must not be reused across multiple Execute calls.
func (c *InterceptorChain) Execute(rpcCtx *RPCContext, invoker UnaryInvoker) (*protos.Response, error) {
	var interceptors []UnaryInterceptor
	if c != nil {
		interceptors = c.interceptors
	}

	rpcCtx.chain = make([]UnaryInterceptor, 0, len(interceptors)+1)
	rpcCtx.chain = append(rpcCtx.chain, interceptors...)
	rpcCtx.chain = append(rpcCtx.chain, func(c *RPCContext) {
		c.Response, c.Error = invoker(c)
	})
	rpcCtx.index = -1
	rpcCtx.aborted = false
	if rpcCtx.Context == nil {
		rpcCtx.Context = context.Background()
	}

	rpcCtx.Next()

	if rpcCtx.Error != nil {
		return rpcCtx.Response, rpcCtx.Error
	}
	if rpcCtx.Response == nil {
		rpcCtx.Response = &protos.Response{}
	}
	return rpcCtx.Response, nil
}
