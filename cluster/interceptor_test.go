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

// TestInterceptorChainOrdering verifies the functional nesting: outer
// interceptor runs first (before) and last (after), inner runs last (before)
// and first (after). The actual RPC call is observed between the inner after
// and the outer after.
func TestInterceptorChainOrdering(t *testing.T) {
	chain := NewInterceptorChain()
	var order []string
	chain.Add(
		func(next UnaryInvoker) UnaryInvoker {
			return func(c *RPCContext) (*protos.Response, error) {
				order = append(order, "outer:before")
				res, err := next(c)
				order = append(order, "outer:after")
				return res, err
			}
		},
		func(next UnaryInvoker) UnaryInvoker {
			return func(c *RPCContext) (*protos.Response, error) {
				order = append(order, "inner:before")
				res, err := next(c)
				order = append(order, "inner:after")
				return res, err
			}
		},
	)

	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		order = append(order, "invoker")
		return &protos.Response{}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, []string{"outer:before", "inner:before", "invoker", "inner:after", "outer:after"}, order)
}

// TestInterceptorChainShortCircuit verifies that an interceptor can return an
// error without calling next, skipping the rest of the chain and the actual
// RPC.
func TestInterceptorChainShortCircuit(t *testing.T) {
	chain := NewInterceptorChain()
	shortErr := errors.New("short circuit")
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			return nil, shortErr
		}
	})
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			t.Fatal("this interceptor should not run")
			return nil, nil
		}
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

// TestInterceptorChainShortCircuitWithResponse verifies an interceptor can
// provide a response and short-circuit without calling next.
func TestInterceptorChainShortCircuitWithResponse(t *testing.T) {
	chain := NewInterceptorChain()
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			return &protos.Response{Data: []byte("cached")}, nil
		}
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
// flow back through all interceptors via the return value.
func TestInterceptorChainErrorPropagation(t *testing.T) {
	chain := NewInterceptorChain()
	invokeErr := errors.New("invoker failed")
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			res, err := next(c)
			assert.Error(t, err)
			assert.Equal(t, invokeErr, err)
			return res, err
		}
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
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			c.Context = context.WithValue(c.Context, "k", "v")
			c.Target = &Server{ID: "sv-2"}
			return next(c)
		}
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

// TestInterceptorChainRetry verifies an interceptor can retry by calling next
// a second time, mutating the target between attempts.
func TestInterceptorChainRetry(t *testing.T) {
	chain := NewInterceptorChain()
	redirectErr := errors.New("redirect")
	attempts := 0
	chain.Add(func(next UnaryInvoker) UnaryInvoker {
		return func(c *RPCContext) (*protos.Response, error) {
			res, err := next(c)
			if err == nil {
				return res, nil
			}
			if err != redirectErr {
				return res, err
			}
			c.Target = &Server{ID: "sv-retry"}
			return next(c)
		}
	})

	_, err := chain.Execute(testParams(), func(c *RPCContext) (*protos.Response, error) {
		attempts++
		if attempts == 1 {
			return nil, redirectErr
		}
		assert.Equal(t, "sv-retry", c.Target.ID)
		return &protos.Response{Data: []byte("ok")}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 2, attempts)
}
