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
		"required":             []any{"decision"},
		"properties": map[string]any{
			"decision": map[string]any{
				"type": "string",
				"enum": []any{"continue", "final"},
			},
			"continuation": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"assistant_output"},
				"properties": map[string]any{
					"assistant_output": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
				},
			},
			"final": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"answer"},
				"properties": map[string]any{
					"answer": map[string]any{
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

	switch decision {
	case "continue":
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
	case "final":
		if _, hasContinuation := payload["continuation"]; hasContinuation {
			return fmt.Errorf("continuation must be absent when decision=final")
		}
		finalRaw, hasFinal := payload["final"]
		if !hasFinal {
			return fmt.Errorf("final is required when decision=final")
		}

		finalBranch, ok := finalRaw.(map[string]any)
		if !ok {
			return fmt.Errorf("final must be an object when decision=final")
		}
		if err := validateFinal(finalBranch); err != nil {
			return err
		}
	default:
		return fmt.Errorf("decision must be one of continue or final")
	}

	return nil
}

func validateContinuation(continuation map[string]any) error {
	allowed := map[string]struct{}{"assistant_output": {}}
	for key := range continuation {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown continuation field %q", key)
		}
	}

	assistantRaw, ok := continuation["assistant_output"]
	if !ok {
		return fmt.Errorf("continuation.assistant_output is required")
	}
	assistantOutput, ok := assistantRaw.(string)
	if !ok {
		return fmt.Errorf("continuation.assistant_output must be a string")
	}
	if strings.TrimSpace(assistantOutput) == "" {
		return fmt.Errorf("continuation.assistant_output must be non-empty")
	}

	return nil
}

func validateFinal(finalBranch map[string]any) error {
	allowed := map[string]struct{}{"answer": {}}
	for key := range finalBranch {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown final field %q", key)
		}
	}

	answerRaw, ok := finalBranch["answer"]
	if !ok {
		return fmt.Errorf("final.answer is required")
	}
	answer, ok := answerRaw.(string)
	if !ok {
		return fmt.Errorf("final.answer must be a string")
	}
	if strings.TrimSpace(answer) == "" {
		return fmt.Errorf("final.answer must be non-empty")
	}

	return nil
}
