package inference

import (
	"context"
	"testing"
)

type noopGateway struct{}

func (noopGateway) Infer(_ context.Context, _ GatewayRequest) (GatewayResponse, error) {
	return GatewayResponse{}, nil
}

func TestRegistryRegisterAndResolve(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("openrouter", func(_ Request) (Gateway, error) {
		return noopGateway{}, nil
	}); err != nil {
		t.Fatalf("expected register success, got %v", err)
	}

	gateway, err := registry.Resolve(Request{Gateway: "openrouter"})
	if err != nil {
		t.Fatalf("expected resolve success, got %v", err)
	}
	if gateway == nil {
		t.Fatal("expected resolved gateway instance")
	}
}

func TestRegistryResolveUnknownGateway(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Resolve(Request{Gateway: "unknown"}); err == nil {
		t.Fatal("expected unknown gateway resolution error")
	}
}

func TestRegistryRegisterRejectsInvalidInput(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register("", func(_ Request) (Gateway, error) { return noopGateway{}, nil }); err == nil {
		t.Fatal("expected empty key registration error")
	}
	if err := registry.Register("openrouter", nil); err == nil {
		t.Fatal("expected nil factory registration error")
	}
}
