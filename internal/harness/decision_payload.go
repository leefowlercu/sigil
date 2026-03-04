package harness

import (
	"fmt"
	"math"
	"strings"

	"github.com/leefowlercu/sigil/internal/runtime"
)

// ContinuationPayload is the parsed continuation branch payload.
type ContinuationPayload struct {
	ReplCode            string
	Intent              string
	ExpectedObservation string
}

// FinalEvidence is one parsed final evidence item.
type FinalEvidence struct {
	Ref       string
	ChunkID   *string
	SpanStart *int
	SpanEnd   *int
}

// FinalPayload is the parsed final branch payload.
type FinalPayload struct {
	Answer     string
	Evidence   []FinalEvidence
	Confidence *string
}

// DecisionPayload is the parsed decision payload from model output.
type DecisionPayload struct {
	Decision     runtime.StepDecision
	Continuation *ContinuationPayload
	Final        *FinalPayload
}

func parseDecisionPayload(payload map[string]any) (DecisionPayload, error) {
	decisionRaw, ok := payload["decision"]
	if !ok {
		return DecisionPayload{}, fmt.Errorf("decision is required")
	}
	decision, ok := decisionRaw.(string)
	if !ok {
		return DecisionPayload{}, fmt.Errorf("decision must be string")
	}

	switch decision {
	case string(runtime.StepDecisionContinue):
		continuationRaw, ok := payload["continuation"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation is required when decision=continue")
		}
		continuation, ok := continuationRaw.(map[string]any)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation must be object when decision=continue")
		}

		codeRaw, ok := continuation["repl_code"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.repl_code is required")
		}
		code, ok := codeRaw.(string)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.repl_code must be string")
		}
		if strings.TrimSpace(code) == "" {
			return DecisionPayload{}, fmt.Errorf("continuation.repl_code must be non-empty")
		}

		intentRaw, ok := continuation["intent"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.intent is required")
		}
		intent, ok := intentRaw.(string)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.intent must be string")
		}
		if strings.TrimSpace(intent) == "" {
			return DecisionPayload{}, fmt.Errorf("continuation.intent must be non-empty")
		}

		expectedObservationRaw, ok := continuation["expected_observation"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.expected_observation is required")
		}
		expectedObservation, ok := expectedObservationRaw.(string)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("continuation.expected_observation must be string")
		}
		if strings.TrimSpace(expectedObservation) == "" {
			return DecisionPayload{}, fmt.Errorf("continuation.expected_observation must be non-empty")
		}

		return DecisionPayload{
			Decision: runtime.StepDecisionContinue,
			Continuation: &ContinuationPayload{
				ReplCode:            code,
				Intent:              intent,
				ExpectedObservation: expectedObservation,
			},
		}, nil
	case string(runtime.StepDecisionFinal):
		finalRaw, ok := payload["final"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final is required when decision=final")
		}
		final, ok := finalRaw.(map[string]any)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final must be object when decision=final")
		}

		answerRaw, ok := final["answer"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final.answer is required")
		}
		answer, ok := answerRaw.(string)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final.answer must be string")
		}
		if strings.TrimSpace(answer) == "" {
			return DecisionPayload{}, fmt.Errorf("final.answer must be non-empty")
		}

		evidenceRaw, ok := final["evidence"]
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final.evidence is required")
		}
		evidenceArray, ok := evidenceRaw.([]any)
		if !ok {
			return DecisionPayload{}, fmt.Errorf("final.evidence must be array")
		}
		if len(evidenceArray) == 0 {
			return DecisionPayload{}, fmt.Errorf("final.evidence must be non-empty")
		}

		evidence := make([]FinalEvidence, 0, len(evidenceArray))
		for index, item := range evidenceArray {
			itemMap, ok := item.(map[string]any)
			if !ok {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d] must be object", index)
			}

			refRaw, ok := itemMap["ref"]
			if !ok {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].ref is required", index)
			}
			ref, ok := refRaw.(string)
			if !ok {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].ref must be string", index)
			}
			if strings.TrimSpace(ref) == "" {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].ref must be non-empty", index)
			}

			var chunkID *string
			if chunkIDRaw, exists := itemMap["chunk_id"]; exists {
				value, ok := chunkIDRaw.(string)
				if !ok {
					return DecisionPayload{}, fmt.Errorf("final.evidence[%d].chunk_id must be string", index)
				}
				trimmed := strings.TrimSpace(value)
				if trimmed == "" {
					return DecisionPayload{}, fmt.Errorf("final.evidence[%d].chunk_id must be non-empty when present", index)
				}
				chunkID = &trimmed
			}

			spanStart, hasSpanStart, err := parseEvidenceInteger(itemMap["span_start"])
			if err != nil {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].span_start %w", index, err)
			}
			spanEnd, hasSpanEnd, err := parseEvidenceInteger(itemMap["span_end"])
			if err != nil {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].span_end %w", index, err)
			}
			if hasSpanStart && hasSpanEnd && spanEnd < spanStart {
				return DecisionPayload{}, fmt.Errorf("final.evidence[%d].span_end must be >= span_start", index)
			}

			evidenceItem := FinalEvidence{
				Ref:     ref,
				ChunkID: chunkID,
			}
			if hasSpanStart {
				evidenceItem.SpanStart = &spanStart
			}
			if hasSpanEnd {
				evidenceItem.SpanEnd = &spanEnd
			}
			evidence = append(evidence, evidenceItem)
		}

		var confidence *string
		if confidenceRaw, exists := final["confidence"]; exists {
			value, ok := confidenceRaw.(string)
			if !ok {
				return DecisionPayload{}, fmt.Errorf("final.confidence must be string")
			}
			trimmed := strings.TrimSpace(value)
			switch trimmed {
			case "low", "medium", "high":
				confidence = &trimmed
			default:
				return DecisionPayload{}, fmt.Errorf("final.confidence must be one of low, medium, high")
			}
		}

		return DecisionPayload{
			Decision: runtime.StepDecisionFinal,
			Final: &FinalPayload{
				Answer:     answer,
				Evidence:   evidence,
				Confidence: confidence,
			},
		}, nil
	default:
		return DecisionPayload{}, fmt.Errorf("decision %q is not supported", decision)
	}
}

func parseEvidenceInteger(value any) (int, bool, error) {
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
			return 0, false, fmt.Errorf("must be integer")
		}
		return int(typed), true, nil
	default:
		return 0, false, fmt.Errorf("must be integer")
	}
}
