package inference

import (
	"fmt"
	"strings"
	"sync"
)

// GatewayFactory creates a gateway adapter for a request.
type GatewayFactory func(request Request) (Gateway, error)

// Registry resolves gateway adapters by llm.gateway key.
type Registry struct {
	mu        sync.RWMutex
	factories map[string]GatewayFactory
}

// NewRegistry creates an empty gateway registry.
func NewRegistry() *Registry {
	return &Registry{
		factories: make(map[string]GatewayFactory),
	}
}

// Register installs a gateway factory under a registry key.
func (r *Registry) Register(gateway string, factory GatewayFactory) error {
	key := strings.TrimSpace(strings.ToLower(gateway))
	if key == "" {
		return fmt.Errorf("gateway key is required")
	}
	if factory == nil {
		return fmt.Errorf("gateway factory for %q is required", key)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.factories[key] = factory
	return nil
}

// Resolve resolves the gateway adapter for request.Gateway.
func (r *Registry) Resolve(request Request) (Gateway, error) {
	key := strings.TrimSpace(strings.ToLower(request.Gateway))
	if key == "" {
		return nil, fmt.Errorf("llm.gateway is required")
	}

	r.mu.RLock()
	factory, ok := r.factories[key]
	r.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("unknown gateway registry key %q", key)
	}

	gateway, err := factory(request)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize gateway %q; %w", key, err)
	}

	return gateway, nil
}
