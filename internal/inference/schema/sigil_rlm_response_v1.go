package schema

import (
	"fmt"
	"strings"
)

func newSigilRLMResponseV1Definition() Definition {
	return Definition{
		ID:         SigilRLMResponseV1SchemaID,
		Name:       "sigil_rlm_response_v1",
		JSONSchema: sigilRLMResponseV1JSONSchema(),
		Validate:   validateSigilRLMResponseV1,
	}
}

func sigilRLMResponseV1JSONSchema() map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"required":             []any{"decision", "continuation"},
		"properties": map[string]any{
			"decision": map[string]any{
				"type": "string",
				"enum": []any{"continue"},
			},
			"continuation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"repl_code", "intent", "expected_observation"},
				"properties": map[string]any{
					"repl_code": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"intent": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"expected_observation": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
				},
			},
		},
	}
}

func validateSigilRLMResponseV1(payload map[string]any) error {
	if payload == nil {
		return fmt.Errorf("payload is required")
	}

	allowedTopLevel := map[string]struct{}{
		"decision":     {},
		"continuation": {},
		"final":        {},
	}
	for key := range payload {
		if _, ok := allowedTopLevel[key]; !ok {
			return fmt.Errorf("unknown top-level field %q", key)
		}
	}

	decisionRaw, ok := payload["decision"]
	if !ok {
		return fmt.Errorf("decision is required")
	}
	decision, ok := decisionRaw.(string)
	if !ok {
		return fmt.Errorf("decision must be a string")
	}

	if decision != "continue" {
		return fmt.Errorf("decision must be continue")
	}
	if _, hasFinal := payload["final"]; hasFinal {
		return fmt.Errorf("final must be absent when decision=continue")
	}
	continuationRaw, hasContinuation := payload["continuation"]
	if !hasContinuation {
		return fmt.Errorf("continuation is required when decision=continue")
	}

	continuation, ok := continuationRaw.(map[string]any)
	if !ok {
		return fmt.Errorf("continuation must be an object when decision=continue")
	}
	if err := validateContinuation(continuation); err != nil {
		return err
	}

	return nil
}

func validateContinuation(continuation map[string]any) error {
	allowed := map[string]struct{}{
		"repl_code":            {},
		"intent":               {},
		"expected_observation": {},
	}
	for key := range continuation {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown continuation field %q", key)
		}
	}

	codeRaw, ok := continuation["repl_code"]
	if !ok {
		return fmt.Errorf("continuation.repl_code is required")
	}
	replCode, ok := codeRaw.(string)
	if !ok {
		return fmt.Errorf("continuation.repl_code must be a string")
	}
	if strings.TrimSpace(replCode) == "" {
		return fmt.Errorf("continuation.repl_code must be non-empty")
	}

	intentRaw, ok := continuation["intent"]
	if !ok {
		return fmt.Errorf("continuation.intent is required")
	}
	intent, ok := intentRaw.(string)
	if !ok {
		return fmt.Errorf("continuation.intent must be a string")
	}
	if strings.TrimSpace(intent) == "" {
		return fmt.Errorf("continuation.intent must be non-empty")
	}

	expectedObservationRaw, ok := continuation["expected_observation"]
	if !ok {
		return fmt.Errorf("continuation.expected_observation is required")
	}
	expectedObservation, ok := expectedObservationRaw.(string)
	if !ok {
		return fmt.Errorf("continuation.expected_observation must be a string")
	}
	if strings.TrimSpace(expectedObservation) == "" {
		return fmt.Errorf("continuation.expected_observation must be non-empty")
	}

	return nil
}
