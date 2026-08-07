package cluster

import (
	"context"
	"errors"
	"testing"

	"github.com/actfuns/pitaya/v2/conn/message"
	"github.com/actfuns/pitaya/v2/protos"
	"github.com/actfuns/pitaya/v2/route"
	"github.com/stretchr/testify/assert"
)

func testParams() *RPCContext {
	ctx := context.Background()
	rt, _ := route.Decode("game.room.test")
	return &RPCContext{
		Context: ctx,
		RPCType: protos.RPCType_User,
		Route:   rt,
		Msg:     &message.Message{Route: rt.String()},
		Target:  &Server{ID: "sv-1"},
	}
}

func TestInterceptorChainEmpty(t *testing.T) {
	chain := NewInterceptorChain()
	called := false
	res, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		called = true
		return &protos.Response{Data: []byte("ok")}, nil
	})
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []byte("ok"), res.Data)

	// a nil chain is safe too and still runs the invoker
	res, err = (*InterceptorChain)(nil).Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		return &protos.Response{Data: []byte("nil-ok")}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []byte("nil-ok"), res.Data)
}

// TestInterceptorChainOrdering verifies the gin-style nesting: outer interceptor
// runs first (before) and last (after), inner runs last (before) and first
// (after). The actual RPC call is observed between the inner after and the
// outer after.
func TestInterceptorChainOrdering(t *testing.T) {
	chain := NewInterceptorChain()
	var order []string
	chain.Add(
		func(c *RPCContext) {
			order = append(order, "outer:before")
			c.Next()
			order = append(order, "outer:after")
		},
		func(c *RPCContext) {
			order = append(order, "inner:before")
			c.Next()
			order = append(order, "inner:after")
		},
	)

	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		order = append(order, "invoker")
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"outer:before", "inner:before", "invoker", "inner:after", "outer:after"}, order)
}

// TestInterceptorChainShortCircuit verifies that an interceptor can set an
// error and abort, skipping the rest of the chain and the actual RPC.
func TestInterceptorChainShortCircuit(t *testing.T) {
	chain := NewInterceptorChain()
	shortErr := errors.New("short circuit")
	chain.Add(func(c *RPCContext) {
		c.Error = shortErr
		c.Abort()
	})
	chain.Add(func(c *RPCContext) {
		t.Fatal("this interceptor should not run")
	})

	called := false
	res, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		called = true
		return &protos.Response{}, nil
	})
	assert.Equal(t, shortErr, err)
	assert.False(t, called)
	assert.Nil(t, res)
}

// TestInterceptorChainForgotNext verifies gin semantics: an interceptor that
// returns without calling Next does NOT break the chain — the remaining
// interceptors and the actual RPC still run (the outer loop advances). Its own
// code simply runs to completion immediately, since there is no c.Next() point
// to pause at.
func TestInterceptorChainForgotNext(t *testing.T) {
	chain := NewInterceptorChain()
	var order []string
	chain.Add(func(c *RPCContext) {
		order = append(order, "mw:before")
		// forgot to call c.Next()
		order = append(order, "mw:rest")
	})
	chain.Add(func(c *RPCContext) {
		order = append(order, "mw2:before")
		c.Next()
		order = append(order, "mw2:after")
	})

	called := false
	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		called = true
		order = append(order, "invoker")
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.True(t, called)
	assert.Equal(t, []string{"mw:before", "mw:rest", "mw2:before", "invoker", "mw2:after"}, order)
}

// TestInterceptorChainAbortWithoutResult verifies that aborting without setting
// a response or error short-circuits with an empty response, mirroring gin.
func TestInterceptorChainAbortWithoutResult(t *testing.T) {
	chain := NewInterceptorChain()
	chain.Add(func(c *RPCContext) {
		c.Abort()
	})

	called := false
	res, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		called = true
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.False(t, called)
	assert.NotNil(t, res)
}

// TestInterceptorChainAbortWithResponse verifies an interceptor can provide a
// response and abort, short-circuiting the actual RPC.
func TestInterceptorChainAbortWithResponse(t *testing.T) {
	chain := NewInterceptorChain()
	chain.Add(func(c *RPCContext) {
		c.Response = &protos.Response{Data: []byte("cached")}
		c.Abort()
	})

	called := false
	res, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		called = true
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.False(t, called)
	assert.Equal(t, []byte("cached"), res.Data)
}

// TestInterceptorChainErrorPropagation verifies errors from the actual RPC
// flow back through all interceptors via c.Error.
func TestInterceptorChainErrorPropagation(t *testing.T) {
	chain := NewInterceptorChain()
	invokeErr := errors.New("invoker failed")
	chain.Add(func(c *RPCContext) {
		c.Next()
		assert.Error(t, c.Error)
		assert.Equal(t, invokeErr, c.Error)
	})

	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		return nil, invokeErr
	})
	assert.Equal(t, invokeErr, err)
}

// TestInterceptorContextMutation verifies an interceptor can replace the
// context and mutate the call target, and the downstream sees the changes.
func TestInterceptorContextMutation(t *testing.T) {
	chain := NewInterceptorChain()
	chain.Add(func(c *RPCContext) {
		c.Context = context.WithValue(c.Context, "k", "v")
		c.Target = &Server{ID: "sv-2"}
		c.Next()
	})

	var gotTarget *Server
	var gotVal interface{}
	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		gotTarget = c.Target
		gotVal = c.Context.Value("k")
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "sv-2", gotTarget.ID)
	assert.Equal(t, "v", gotVal)
}
