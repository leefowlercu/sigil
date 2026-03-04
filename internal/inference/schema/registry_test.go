package schema

import "testing"

func TestNewRegistryContainsRequiredV1Schema(t *testing.T) {
	registry := NewRegistry()
	required := []string{
		SigilRLMResponseV1SchemaID,
		SigilLLMAnswerV1SchemaID,
	}
	for _, schemaID := range required {
		definition, err := registry.Resolve(schemaID)
		if err != nil {
			t.Fatalf("expected required schema %q registration, got %v", schemaID, err)
		}
		if definition.ID != schemaID {
			t.Fatalf("expected schema id %q, got %q", schemaID, definition.ID)
		}
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
				"decision": "continue",
				"continuation": map[string]any{
					"repl_code":            "fmt.Println(\"next\")",
					"intent":               "inspect context chunk",
					"expected_observation": "needle-like match in output",
				},
			},
		},
		{
			name: "final branch",
			payload: map[string]any{
				"decision": "final",
				"final": map[string]any{
					"answer": "done",
					"evidence": []any{
						map[string]any{
							"ref":        "run-output://node/a/final-answer.json",
							"chunk_id":   "chunk-17",
							"span_start": 10,
							"span_end":   32,
						},
					},
					"confidence": "medium",
				},
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
		{name: "continue missing intent", payload: map[string]any{"decision": "continue", "continuation": map[string]any{"repl_code": "fmt.Println(\"next\")", "expected_observation": "x"}}},
		{name: "continue missing expected observation", payload: map[string]any{"decision": "continue", "continuation": map[string]any{"repl_code": "fmt.Println(\"next\")", "intent": "x"}}},
		{name: "continue forbids final", payload: map[string]any{
			"decision": "continue",
			"continuation": map[string]any{
				"repl_code":            "fmt.Println(\"next\")",
				"intent":               "inspect",
				"expected_observation": "match",
			},
			"final": map[string]any{"answer": "done"},
		}},
		{name: "final missing final branch", payload: map[string]any{"decision": "final"}},
		{name: "final missing evidence", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done"}}},
		{name: "final empty evidence array", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "evidence": []any{}}}},
		{name: "final invalid confidence", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json"}}, "confidence": "sure"}}},
		{name: "final malformed evidence span range", payload: map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json", "span_start": 9, "span_end": 3}}}}},
		{name: "final forbids continuation", payload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   "done",
				"evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json"}},
			},
			"continuation": map[string]any{"repl_code": "fmt.Println(\"next\")"},
		}},
		{name: "unknown top-level field", payload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   "done",
				"evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json"}},
			},
			"unknown": "x",
		}},
		{name: "unknown nested continuation field", payload: map[string]any{
			"decision": "continue",
			"continuation": map[string]any{
				"repl_code":            "fmt.Println(\"next\")",
				"intent":               "inspect",
				"expected_observation": "match",
				"unknown":              "x",
			},
		}},
		{name: "unknown nested final field", payload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   "done",
				"evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json"}},
				"unknown":  "x",
			},
		}},
		{name: "empty repl_code", payload: map[string]any{
			"decision": "continue",
			"continuation": map[string]any{
				"repl_code":            "   ",
				"intent":               "inspect",
				"expected_observation": "match",
			},
		}},
		{name: "empty final answer", payload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   "   ",
				"evidence": []any{map[string]any{"ref": "run-output://node/x/final-answer.json"}},
			},
		}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := definition.Validate(testCase.payload); err == nil {
				t.Fatal("expected schema validation failure")
			}
		})
	}
}

func TestValidateSigilLLMAnswerV1AcceptsValidPayload(t *testing.T) {
	definition, err := NewRegistry().Resolve(SigilLLMAnswerV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	payload := map[string]any{
		"answer": "done",
	}
	if err := definition.Validate(payload); err != nil {
		t.Fatalf("expected schema validation success, got %v", err)
	}
}

func TestValidateSigilLLMAnswerV1RejectsInvalidPayloads(t *testing.T) {
	definition, err := NewRegistry().Resolve(SigilLLMAnswerV1SchemaID)
	if err != nil {
		t.Fatalf("failed to resolve schema definition: %v", err)
	}

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{name: "missing answer", payload: map[string]any{}},
		{name: "empty answer", payload: map[string]any{"answer": "   "}},
		{name: "non-string answer", payload: map[string]any{"answer": 1}},
		{name: "unknown field", payload: map[string]any{"answer": "ok", "extra": "x"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			if err := definition.Validate(testCase.payload); err == nil {
				t.Fatal("expected schema validation failure")
			}
		})
	}
}
