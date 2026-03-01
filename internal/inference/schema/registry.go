package schema

import (
	"fmt"
	"strings"
	"sync"
)

const (
	// SigilRLMResponseV1SchemaID is the required v1 structured inference schema.
	SigilRLMResponseV1SchemaID = "sigil.rlm.response.v1"
)

// ValidateFunc validates a decoded structured payload map.
type ValidateFunc func(payload map[string]any) error

// Definition stores schema metadata, JSON schema payload, and strict validator.
type Definition struct {
	ID         string
	Name       string
	JSONSchema map[string]any
	Validate   ValidateFunc
}

// Registry stores central structured response schemas by schema_id.
type Registry struct {
	mu          sync.RWMutex
	definitions map[string]Definition
}

// NewRegistry builds a registry preloaded with required v1 schemas.
func NewRegistry() *Registry {
	registry := NewEmptyRegistry()
	_ = registry.Register(newSigilRLMResponseV1Definition())
	return registry
}

// NewEmptyRegistry builds an empty schema registry.
func NewEmptyRegistry() *Registry {
	return &Registry{
		definitions: make(map[string]Definition),
	}
}

// Register adds or replaces a schema definition.
func (r *Registry) Register(definition Definition) error {
	schemaID := strings.TrimSpace(definition.ID)
	if schemaID == "" {
		return fmt.Errorf("schema id is required")
	}
	if strings.TrimSpace(definition.Name) == "" {
		return fmt.Errorf("schema %q is missing schema name", schemaID)
	}
	if definition.JSONSchema == nil {
		return fmt.Errorf("schema %q is missing json schema payload", schemaID)
	}
	if definition.Validate == nil {
		return fmt.Errorf("schema %q is missing validator", schemaID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	r.definitions[schemaID] = definition
	return nil
}

// Resolve returns a schema definition by schema_id.
func (r *Registry) Resolve(schemaID string) (Definition, error) {
	key := strings.TrimSpace(schemaID)
	if key == "" {
		return Definition{}, fmt.Errorf("schema id is required")
	}

	r.mu.RLock()
	definition, ok := r.definitions[key]
	r.mu.RUnlock()
	if !ok {
		return Definition{}, fmt.Errorf("schema id %q is not registered", key)
	}

	return definition, nil
}
