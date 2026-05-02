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
		return DecisionPayload{}, fmt.Errorf("decision=final is not supported; emit FINAL_ANSWER_START and FINAL_ANSWER_END from continuation.repl_code")
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
