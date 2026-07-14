package prpc

import (
	"context"

	"github.com/topfreegames/pitaya/v2/config"
	pcontext "github.com/topfreegames/pitaya/v2/context"
	"google.golang.org/protobuf/proto"
)

// CallOption is an optional configuration for an RPC call.
type CallOption func(*CallOptions)

// CallOptions holds all configurable options for an RPC call.
type CallOptions struct {
	ServerID            string                  // target server ID, empty means any
	OneWay              bool                    // fire-and-forget, no response expected
	Client              bool                    // mark as client-side RPC (Sys type)
	PropagateCtx        []pcontext.KeyValuePair // values to inject into the propagate context for RPC calls
	Reliable            bool                    // reliable RPC, retries on failure
	ReliableMetadata    map[string]interface{}  // metadata to be passed to the server
	ReliableEnqueueOpts *config.EnqueueOpts     // enqueue options
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

// WithPropagateCtx adds a key-value pair that will be propagated through the RPC context.
func WithPropagateCtx(key string, val interface{}) CallOption {
	return func(o *CallOptions) {
		o.PropagateCtx = append(o.PropagateCtx, pcontext.KeyValuePair{Key: key, Value: val})
	}
}

// WithClient marks the RPC as a client-facing call (Sys type).
func WithClient() CallOption {
	return func(o *CallOptions) {
		o.Client = true
	}
}

// WithReliable sets the call as reliable (enqueued).
func WithReliable(metadata map[string]interface{}, opts *config.EnqueueOpts) CallOption {
	return func(c *CallOptions) {
		c.Reliable = true
		c.ReliableMetadata = metadata
		c.ReliableEnqueueOpts = opts
	}
}

// Client is the interface for making RPC calls via pitaya.
// Generated code's NewXxxClient accepts this interface.
type Client interface {
	RPC(ctx context.Context, routeStr string, reply proto.Message, arg proto.Message, opts ...CallOption) error
}
