package schema

import "testing"

func TestNewRegistryContainsRequiredV1Schema(t *testing.T) {
	registry := NewRegistry()
	definition, err := registry.Resolve(SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("expected required schema registration, got %v", err)
	}
	if definition.ID != SigilRLMResponseV1SchemaID {
		t.Fatalf("expected schema id %q, got %q", SigilRLMResponseV1SchemaID, definition.ID)
	}
}

func TestResolveRejectsUnknownSchemaID(t *testing.T) {
	registry := NewRegistry()
	if _, err := registry.Resolve("sigil.unknown.v1"); err == nil {
		t.Fatal("expected unknown schema lookup error")
	}
}

func TestValidateSigilRLMResponseV1AcceptsValidBranches(t *testing.T) {
	definition, err := NewRegistry().Resolve(SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "continue branch",
			payload: map[string]any{
				"decision":     "continue",
				"continuation": map[string]any{"assistant_output": "next"},
			},
		},
		{
			name: "final branch",
			payload: map[string]any{
				"decision": "final",
				"final":    map[string]any{"answer": "done"},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := definition.Validate(testCase.payload); err != nil {
				t.Fatalf("expected schema validation success, got %v", err)
			}
		})
	}
}

func TestValidateSigilRLMResponseV1RejectsInvalidPayloads(t *testing.T) {
	definition, err := NewRegistry().Resolve(SigilRLMResponseV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{name: "invalid decision", payload: map[string]any{"decision": "maybe"}},
		{name: "continue missing continuation", payload: map[string]any{"decision": "continue"}},
		{name: "continue forbids final", payload: map[string]any{"decision": "continue", "continuation": map[string]any{"assistant_output": "next"}, "final": map[string]any{"answer": "done"}}},
		{name: "final missing final branch", payload: map[string]any{"decision": "final"}},
		{name: "final forbids continuation", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done"}, "continuation": map[string]any{"assistant_output": "next"}}},
		{name: "unknown top-level field", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done"}, "unknown": "x"}},
		{name: "unknown nested continuation field", payload: map[string]any{"decision": "continue", "continuation": map[string]any{"assistant_output": "next", "unknown": "x"}}},
		{name: "unknown nested final field", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "unknown": "x"}}},
		{name: "empty assistant output", payload: map[string]any{"decision": "continue", "continuation": map[string]any{"assistant_output": "   "}}},
		{name: "empty final answer", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "   "}}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := definition.Validate(testCase.payload); err == nil {
				t.Fatal("expected schema validation failure")
			}
		})
	}
}
