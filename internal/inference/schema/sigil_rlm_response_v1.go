package schema

import (
	"fmt"
	"math"
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
			"final": map[string]any{
				"type":                 "object",
				"additionalProperties": false,
				"required":             []any{"answer", "evidence"},
				"properties": map[string]any{
					"answer": map[string]any{
						"type":      "string",
						"minLength": 1,
					},
					"evidence": map[string]any{
						"type":     "array",
						"minItems": 1,
						"items": map[string]any{
							"type":                 "object",
							"additionalProperties": false,
							"required":             []any{"ref"},
							"properties": map[string]any{
								"ref": map[string]any{
									"type":      "string",
									"minLength": 1,
								},
								"chunk_id": map[string]any{
									"type": "string",
								},
								"span_start": map[string]any{
									"type":    "integer",
									"minimum": 0,
								},
								"span_end": map[string]any{
									"type":    "integer",
									"minimum": 0,
								},
							},
						},
					},
					"confidence": map[string]any{
						"type": "string",
						"enum": []any{"low", "medium", "high"},
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

func validateFinal(finalBranch map[string]any) error {
	allowed := map[string]struct{}{
		"answer":     {},
		"evidence":   {},
		"confidence": {},
	}
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

	evidenceRaw, ok := finalBranch["evidence"]
	if !ok {
		return fmt.Errorf("final.evidence is required")
	}
	evidence, ok := evidenceRaw.([]any)
	if !ok {
		return fmt.Errorf("final.evidence must be an array")
	}
	if len(evidence) == 0 {
		return fmt.Errorf("final.evidence must contain at least one item")
	}
	for index, item := range evidence {
		itemMap, ok := item.(map[string]any)
		if !ok {
			return fmt.Errorf("final.evidence[%d] must be an object", index)
		}
		if err := validateFinalEvidenceItem(index, itemMap); err != nil {
			return err
		}
	}

	if confidenceRaw, exists := finalBranch["confidence"]; exists {
		confidence, ok := confidenceRaw.(string)
		if !ok {
			return fmt.Errorf("final.confidence must be a string")
		}
		switch strings.TrimSpace(confidence) {
		case "low", "medium", "high":
		default:
			return fmt.Errorf("final.confidence must be one of low, medium, or high")
		}
	}

	return nil
}

func validateFinalEvidenceItem(index int, item map[string]any) error {
	allowed := map[string]struct{}{
		"ref":        {},
		"chunk_id":   {},
		"span_start": {},
		"span_end":   {},
	}
	for key := range item {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("unknown final.evidence[%d] field %q", index, key)
		}
	}

	refRaw, ok := item["ref"]
	if !ok {
		return fmt.Errorf("final.evidence[%d].ref is required", index)
	}
	ref, ok := refRaw.(string)
	if !ok {
		return fmt.Errorf("final.evidence[%d].ref must be a string", index)
	}
	if strings.TrimSpace(ref) == "" {
		return fmt.Errorf("final.evidence[%d].ref must be non-empty", index)
	}

	if chunkIDRaw, exists := item["chunk_id"]; exists {
		chunkID, ok := chunkIDRaw.(string)
		if !ok {
			return fmt.Errorf("final.evidence[%d].chunk_id must be a string", index)
		}
		if strings.TrimSpace(chunkID) == "" {
			return fmt.Errorf("final.evidence[%d].chunk_id must be non-empty when present", index)
		}
	}

	spanStart, hasSpanStart, err := parseNonNegativeInteger(item["span_start"])
	if err != nil {
		return fmt.Errorf("final.evidence[%d].span_start %w", index, err)
	}
	spanEnd, hasSpanEnd, err := parseNonNegativeInteger(item["span_end"])
	if err != nil {
		return fmt.Errorf("final.evidence[%d].span_end %w", index, err)
	}
	if hasSpanStart && hasSpanEnd && spanEnd < spanStart {
		return fmt.Errorf("final.evidence[%d].span_end must be greater than or equal to span_start", index)
	}

	return nil
}

func parseNonNegativeInteger(value any) (int, bool, error) {
	if value == nil {
		return 0, false, nil
	}

	switch typed := value.(type) {
	case int:
		if typed < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return typed, true, nil
	case int64:
		if typed < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		return int(typed), true, nil
	case float64:
		if typed < 0 {
			return 0, false, fmt.Errorf("must be >= 0")
		}
		if math.Mod(typed, 1) != 0 {
			return 0, false, fmt.Errorf("must be an integer")
		}
		return int(typed), true, nil
	default:
		return 0, false, fmt.Errorf("must be an integer")
	}
}
