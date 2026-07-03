package prpc

import (
	"context"

	"github.com/topfreegames/pitaya/v2/config"
)

// CallOption is an optional configuration for an RPC call.
type CallOption func(*CallOptions)

// CallOptions holds all configurable options for an RPC call.
type CallOptions struct {
	ServerID    string                 // target server ID, empty means any
	OneWay      bool                   // fire-and-forget, no response expected
	Reliable    bool                   // reliable RPC, retries on failure
	Metadata    map[string]interface{} // metadata to be passed to the server
	EnqueueOpts *config.EnqueueOpts    // enqueue options
}

// WithServerID sets the target server ID for the RPC call.
func WithServerID(id string) CallOption {
	return func(o *CallOptions) {
		o.ServerID = id
	}
}

// WithOneWay sets the call as one-way (fire-and-forget, no response).
func WithOneWay() CallOption {
	return func(o *CallOptions) {
		o.OneWay = true
	}
}

// WithReliable sets the call as reliable (enqueued).
func WithReliable(metadata map[string]interface{}, opts *config.EnqueueOpts) CallOption {
	return func(c *CallOptions) {
		c.Reliable = true
		c.Metadata = metadata
		c.EnqueueOpts = opts
	}
}

// Client is the interface for making RPC calls via pitaya.
// Generated code's NewXxxClient accepts this interface.
type Client interface {
	RPC(ctx context.Context, route string, reply, arg interface{}, opts ...CallOption) error
}
