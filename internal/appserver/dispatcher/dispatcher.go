package dispatcher

import (
	"context"
	"encoding/json"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
)

// Handler resolves one request method from raw params to a typed result.
type Handler func(context.Context, json.RawMessage) (interface{}, *protocol.ErrorObject)

// Dispatcher resolves request methods to registered handlers.
type Dispatcher struct {
	handlers map[string]Handler
}

// New creates an empty dispatcher.
func New() *Dispatcher {
	return &Dispatcher{
		handlers: map[string]Handler{},
	}
}

// Register stores one handler for a method name.
func (d *Dispatcher) Register(method string, handler Handler) {
	if d == nil || method == "" || handler == nil {
		return
	}
	d.handlers[method] = handler
}

// Dispatch routes one request by method name.
func (d *Dispatcher) Dispatch(ctx context.Context, method string, params json.RawMessage) (interface{}, *protocol.ErrorObject) {
	if d == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "dispatcher is not configured", "dispatcher_unavailable", false, nil)
	}

	handler, ok := d.handlers[method]
	if !ok {
		return nil, protocol.NewError(protocol.CodeMethodNotFound, "method not found", "method_not_found", false, map[string]interface{}{
			"method": method,
		})
	}

	return handler(ctx, params)
}
