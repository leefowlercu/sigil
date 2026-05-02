package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/accounting"
)

func TestParseEventEnvelopeStrictRejectsUnknownEnvelopeField(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := mustUUIDv7String(t)

	raw := map[string]any{
		"event_id":       eventID,
		"schema_version": SchemaVersionV1,
		"run_id":         runID,
		"seq":            1,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           EventTypeRunQueued,
		"causation_id":   eventID,
		"correlation_id": runID,
		"payload": map[string]any{
			"source": string(RunQueuedSourceInternalResume),
		},
		"unknown": "not-allowed",
	}

	serialized, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}

	_, err = ParseEventEnvelopeStrict(serialized)
	if !errors.Is(err, ErrUnknownEnvelopeField) {
		t.Fatalf("expected ErrUnknownEnvelopeField, got %v", err)
	}
}

func TestParseEventEnvelopeStrictRejectsUnknownPayloadField(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := mustUUIDv7String(t)

	raw := map[string]any{
		"event_id":       eventID,
		"schema_version": SchemaVersionV1,
		"run_id":         runID,
		"seq":            1,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           EventTypeRunQueued,
		"causation_id":   eventID,
		"correlation_id": runID,
		"payload": map[string]any{
			"source": string(RunQueuedSourceInternalResume),
			"extra":  "unexpected",
		},
	}

	serialized, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}

	_, err = ParseEventEnvelopeStrict(serialized)
	if !errors.Is(err, ErrUnknownPayloadField) {
		t.Fatalf("expected ErrUnknownPayloadField, got %v", err)
	}
}

func TestParseEventEnvelopeStrictRejectsUnknownEventType(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := mustUUIDv7String(t)

	raw := map[string]any{
		"event_id":       eventID,
		"schema_version": SchemaVersionV1,
		"run_id":         runID,
		"seq":            1,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           "run.unknown",
		"causation_id":   eventID,
		"correlation_id": runID,
		"payload": map[string]any{
			"source": string(RunQueuedSourceInternalResume),
		},
	}

	serialized, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}

	_, err = ParseEventEnvelopeStrict(serialized)
	if !errors.Is(err, ErrUnknownEventType) {
		t.Fatalf("expected ErrUnknownEventType, got %v", err)
	}
}

func TestParseEventEnvelopeStrictEnforcesCorrelationAndFirstEventCausation(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := mustUUIDv7String(t)

	testCases := []struct {
		name string
		raw  map[string]any
	}{
		{
			name: "correlation id mismatch",
			raw: map[string]any{
				"event_id":       eventID,
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            1,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           EventTypeRunQueued,
				"causation_id":   eventID,
				"correlation_id": mustUUIDv7String(t),
				"payload": map[string]any{
					"source": string(RunQueuedSourceInternalResume),
				},
			},
		},
		{
			name: "first event causation mismatch",
			raw: map[string]any{
				"event_id":       eventID,
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            1,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           EventTypeRunQueued,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload": map[string]any{
					"source": string(RunQueuedSourceInternalResume),
				},
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			serialized, err := json.Marshal(testCase.raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			_, err = ParseEventEnvelopeStrict(serialized)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestParseEventEnvelopeStrictEnforcesUUIDv7Identity(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := uuid.New().String()

	raw := map[string]any{
		"event_id":       eventID,
		"schema_version": SchemaVersionV1,
		"run_id":         runID,
		"seq":            1,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           EventTypeRunQueued,
		"causation_id":   eventID,
		"correlation_id": runID,
		"payload": map[string]any{
			"source": string(RunQueuedSourceInternalResume),
		},
	}

	serialized, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}

	_, err = ParseEventEnvelopeStrict(serialized)
	if !errors.Is(err, ErrInvalidEvent) {
		t.Fatalf("expected ErrInvalidEvent, got %v", err)
	}
}

func TestNormalizePayloadAcceptsCanonicalV1Payloads(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	parentID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)
	failedStepID := mustUUIDv7String(t)
	contentRef := "run-artifact://node/turn/1"
	errorCode := "action.failed"
	errorMessage := "failed action execution"
	accountingSummary := testAccountingSummary()
	accountingRollup := testAccountingRollup()
	actionRef, err := BuildActionArtifactRef(nodeID, stepID, 1)
	if err != nil {
		t.Fatalf("failed to build action_ref fixture: %v", err)
	}

	testCases := []struct {
		name      string
		eventType EventType
		payload   any
	}{
		{
			name:      "run.queued",
			eventType: EventTypeRunQueued,
			payload: RunQueuedPayload{
				Source: RunQueuedSourceInternalResume,
			},
		},
		{
			name:      "run.running",
			eventType: EventTypeRunRunning,
			payload: RunRunningPayload{
				Executor: "rlm",
				MaxDepth: 1,
			},
		},
		{
			name:      "node.started",
			eventType: EventTypeNodeStarted,
			payload: NodeStartedPayload{
				Depth:        1,
				ParentNodeID: &parentID,
				Role:         NodeRoleRecursiveSubcall,
				Attempt:      1,
			},
		},
		{
			name:      "node.completed",
			eventType: EventTypeNodeCompleted,
			payload: NodeCompletedPayload{
				Status:     "completed",
				DurationMS: 10,
				Accounting: accountingRollup,
			},
		},
		{
			name:      "node.failed",
			eventType: EventTypeNodeFailed,
			payload: NodeFailedPayload{
				Status:       "failed",
				DurationMS:   10,
				ErrorCode:    "repl_child_failure",
				ErrorMessage: "child failed",
				FailedStepID: &failedStepID,
			},
		},
		{
			name:      "node.step.started",
			eventType: EventTypeNodeStepStarted,
			payload: NodeStepStartedPayload{
				StepID:    stepID,
				StepIndex: 1,
				SchemaID:  "sigil.rlm.response.v1",
			},
		},
		{
			name:      "node.step.completed continue",
			eventType: EventTypeNodeStepCompleted,
			payload: NodeStepCompletedPayload{
				StepID:        stepID,
				Decision:      StepDecisionContinue,
				ActionCount:   1,
				DurationMS:    9,
				Accounting:    accountingRollup,
				AccountingRef: testStepAccountingRef(nodeID, stepID),
			},
		},
		{
			name:      "node.turn.user",
			eventType: EventTypeNodeTurnUser,
			payload: NodeTurnPayload{
				StepID:     stepID,
				Role:       TurnRoleUser,
				ContentRef: contentRef,
			},
		},
		{
			name:      "node.turn.model",
			eventType: EventTypeNodeTurnModel,
			payload: NodeTurnPayload{
				StepID:     stepID,
				Role:       TurnRoleModel,
				ContentRef: contentRef,
			},
		},
		{
			name:      "node.subcall.executed completed",
			eventType: EventTypeNodeSubcallExecuted,
			payload: NodeSubcallExecutedPayload{
				StepID:        stepID,
				ActionIndex:   1,
				SubcallIndex:  1,
				SubcallType:   SubcallTypeLLMQuery,
				ExecutionMode: SubcallExecutionModePlain,
				Status:        ActionExecutionStatusCompleted,
				Provider:      "openai",
				Model:         "gpt-5.1",
				PromptBytes:   10,
				ContextBytes:  20,
				AnswerBytes:   12,
				DurationMS:    2,
				Accounting:    accountingSummary,
				AccountingRef: testSubcallAccountingRef(nodeID, stepID, 1),
			},
		},
		{
			name:      "node.subcall.executed failed",
			eventType: EventTypeNodeSubcallExecuted,
			payload: NodeSubcallExecutedPayload{
				StepID:        stepID,
				ActionIndex:   1,
				SubcallIndex:  2,
				SubcallType:   SubcallTypeRLMQuery,
				ExecutionMode: SubcallExecutionModeRecursive,
				Status:        ActionExecutionStatusFailed,
				Provider:      "openai",
				Model:         "gpt-5.1",
				PromptBytes:   10,
				ContextBytes:  20,
				AnswerBytes:   0,
				DurationMS:    2,
				ChildNodeID:   &parentID,
				ErrorCode:     &errorCode,
				ErrorMessage:  &errorMessage,
				Accounting:    accountingSummary,
				AccountingRef: testSubcallAccountingRef(nodeID, stepID, 2),
			},
		},
		{
			name:      "node.action.executed completed",
			eventType: EventTypeNodeActionExecuted,
			payload: NodeActionExecutedPayload{
				StepID:      stepID,
				ActionIndex: 1,
				ActionType:  "repl_code",
				Language:    "go",
				Status:      ActionExecutionStatusCompleted,
				DurationMS:  10,
				ActionRef:   actionRef,
			},
		},
		{
			name:      "node.action.executed failed",
			eventType: EventTypeNodeActionExecuted,
			payload: NodeActionExecutedPayload{
				StepID:       stepID,
				ActionIndex:  1,
				ActionType:   "repl_code",
				Language:     "go",
				Status:       ActionExecutionStatusFailed,
				DurationMS:   10,
				ActionRef:    actionRef,
				ErrorCode:    &errorCode,
				ErrorMessage: &errorMessage,
			},
		},
		{
			name:      "run.completed",
			eventType: EventTypeRunCompleted,
			payload: RunCompletedPayload{
				Status:     "completed",
				DurationMS: 11,
				Accounting: accountingRollup,
			},
		},
		{
			name:      "run.failed",
			eventType: EventTypeRunFailed,
			payload: RunFailedPayload{
				Status:       "failed",
				ErrorCode:    "runtime.failure",
				ErrorMessage: "failed",
				FailedNodeID: &nodeID,
				Retryable:    false,
				Accounting:   accountingRollup,
			},
		},
		{
			name:      "run.interrupted",
			eventType: EventTypeRunInterrupted,
			payload: RunInterruptedPayload{
				Status:        "interrupted",
				Reason:        RunInterruptedReasonUserRequest,
				InterruptedBy: stringPointer("cli.run.stop"),
				Accounting:    accountingRollup,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			eventID := mustUUIDv7String(t)
			event := EventEnvelope{
				EventID:       eventID,
				SchemaVersion: SchemaVersionV1,
				RunID:         runID,
				Seq:           1,
				Timestamp:     time.Now().UTC(),
				Type:          testCase.eventType,
				CausationID:   eventID,
				CorrelationID: runID,
				Payload:       testCase.payload,
			}
			if isNodeScopedEventType(testCase.eventType) {
				event.NodeID = &nodeID
				event.Seq = 2
				event.CausationID = mustUUIDv7String(t)
			}
			if testCase.eventType != EventTypeRunQueued {
				event.Seq = 2
				event.CausationID = mustUUIDv7String(t)
			}

			if _, err := validateAndNormalizeEvent(event); err != nil {
				t.Fatalf("expected payload normalization success, got %v", err)
			}
		})
	}
}

func TestParseEventEnvelopeStrictRejectsInvalidPayloadInvariants(t *testing.T) {
	eventID := mustUUIDv7String(t)
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	testCases := []struct {
		name    string
		payload map[string]any
	}{
		{
			name: "node started invalid role at depth 0",
			payload: map[string]any{
				"depth":          0,
				"parent_node_id": nil,
				"role":           "recursive_subcall",
				"attempt":        1,
			},
		},
		{
			name: "node started missing required attempt",
			payload: map[string]any{
				"depth":          0,
				"parent_node_id": nil,
				"role":           "root",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       eventID,
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           EventTypeNodeStarted,
				"node_id":        nodeID,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			_, err = ParseEventEnvelopeStrict(serialized)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestParseEventEnvelopeStrictRejectsInvalidStepTurnAndActionPayloadInvariants(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		eventType EventType
		payload   map[string]any
	}{
		{
			name:      "node step completed continue with action_count zero",
			eventType: EventTypeNodeStepCompleted,
			payload: map[string]any{
				"step_id":      stepID,
				"decision":     "continue",
				"action_count": 0,
				"duration_ms":  1,
			},
		},
		{
			name:      "node turn user with mismatched role",
			eventType: EventTypeNodeTurnUser,
			payload: map[string]any{
				"step_id":     stepID,
				"role":        "model",
				"content_ref": "run-artifact://turn/1",
			},
		},
		{
			name:      "node action executed failed without errors",
			eventType: EventTypeNodeActionExecuted,
			payload: map[string]any{
				"step_id":      stepID,
				"action_index": 1,
				"action_type":  "repl_code",
				"language":     "go",
				"status":       "failed",
				"duration_ms":  1,
				"action_ref":   "run-artifact://node/" + nodeID + "/step/" + stepID + "/action-1.json",
			},
		},
		{
			name:      "node action executed invalid action_ref identity mismatch",
			eventType: EventTypeNodeActionExecuted,
			payload: map[string]any{
				"step_id":      stepID,
				"action_index": 1,
				"action_type":  "repl_code",
				"language":     "go",
				"status":       "completed",
				"duration_ms":  1,
				"action_ref":   "run-artifact://node/" + mustUUIDv7String(t) + "/step/" + stepID + "/action-1.json",
			},
		},
		{
			name:      "node subcall executed failed without error fields",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   "llm_query",
				"execution_mode": "plain",
				"status":         "failed",
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   0,
				"duration_ms":    1,
			},
		},
		{
			name:      "node subcall executed recursive missing child node",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   "rlm_query",
				"execution_mode": "recursive",
				"status":         "completed",
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           testCase.eventType,
				"node_id":        nodeID,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			_, err = ParseEventEnvelopeStrict(serialized)
			if !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("expected ErrInvalidEvent, got %v", err)
			}
		})
	}
}

func TestParseEventEnvelopeStrictValidatesRunFailedGuardrailMetadataInvariants(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	testCases := []struct {
		name          string
		payload       map[string]any
		expectInvalid bool
	}{
		{
			name: "valid guardrail metadata",
			payload: map[string]any{
				"status":           "failed",
				"error_code":       "harness_limit_exceeded",
				"error_message":    "guardrail breached",
				"failed_node_id":   nodeID,
				"failed_step_id":   mustUUIDv7String(t),
				"limit_key":        "max_steps_per_node",
				"configured_value": "1",
				"observed_value":   "1",
				"retryable":        false,
				"accounting":       testAccountingRollup(),
			},
			expectInvalid: false,
		},
		{
			name: "limit key missing configured value",
			payload: map[string]any{
				"status":         "failed",
				"error_code":     "harness_limit_exceeded",
				"error_message":  "guardrail breached",
				"failed_node_id": nodeID,
				"limit_key":      "max_steps_per_node",
				"observed_value": "1",
				"retryable":      false,
				"accounting":     testAccountingRollup(),
			},
			expectInvalid: true,
		},
		{
			name: "configured value without limit key remains valid",
			payload: map[string]any{
				"status":           "failed",
				"error_code":       "harness_limit_exceeded",
				"error_message":    "guardrail breached",
				"failed_node_id":   nodeID,
				"configured_value": "1",
				"retryable":        false,
				"accounting":       testAccountingRollup(),
			},
			expectInvalid: false,
		},
		{
			name: "failed step id must be uuidv7",
			payload: map[string]any{
				"status":           "failed",
				"error_code":       "harness_limit_exceeded",
				"error_message":    "guardrail breached",
				"failed_node_id":   nodeID,
				"failed_step_id":   "not-uuidv7",
				"limit_key":        "max_steps_per_node",
				"configured_value": "1",
				"observed_value":   "1",
				"retryable":        false,
				"accounting":       testAccountingRollup(),
			},
			expectInvalid: true,
		},
		{
			name: "failed step id must not be null when present",
			payload: map[string]any{
				"status":           "failed",
				"error_code":       "harness_limit_exceeded",
				"error_message":    "guardrail breached",
				"failed_node_id":   nodeID,
				"failed_step_id":   nil,
				"limit_key":        "max_steps_per_node",
				"configured_value": "1",
				"observed_value":   "1",
				"retryable":        false,
				"accounting":       testAccountingRollup(),
			},
			expectInvalid: true,
		},
		{
			name: "guardrail metadata must not be null when present",
			payload: map[string]any{
				"status":           "failed",
				"error_code":       "harness_limit_exceeded",
				"error_message":    "guardrail breached",
				"failed_node_id":   nodeID,
				"limit_key":        nil,
				"configured_value": nil,
				"observed_value":   nil,
				"retryable":        false,
				"accounting":       testAccountingRollup(),
			},
			expectInvalid: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           EventTypeRunFailed,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			_, err = ParseEventEnvelopeStrict(serialized)
			if testCase.expectInvalid {
				if !errors.Is(err, ErrInvalidEvent) {
					t.Fatalf("expected ErrInvalidEvent, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected success, got %v", err)
			}
		})
	}
}

func TestParseEventEnvelopeStrictAcceptsLegacyV1PayloadsWithoutAccountingFieldsForOptionalRefEvents(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	errorCode := "runtime.failure"
	errorMessage := "failure"

	testCases := []struct {
		name      string
		eventType EventType
		nodeID    *string
		payload   map[string]any
	}{
		{
			name:      "node.completed",
			eventType: EventTypeNodeCompleted,
			nodeID:    &nodeID,
			payload: map[string]any{
				"status":      "completed",
				"duration_ms": 1,
			},
		},
		{
			name:      "run.completed",
			eventType: EventTypeRunCompleted,
			payload: map[string]any{
				"status":      "completed",
				"duration_ms": 1,
			},
		},
		{
			name:      "run.failed",
			eventType: EventTypeRunFailed,
			payload: map[string]any{
				"status":        "failed",
				"error_code":    errorCode,
				"error_message": errorMessage,
				"retryable":     false,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           testCase.eventType,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}
			if testCase.nodeID != nil {
				raw["node_id"] = *testCase.nodeID
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			event, err := ParseEventEnvelopeStrict(serialized)
			if err != nil {
				t.Fatalf("expected legacy v1 event to remain parseable, got %v", err)
			}

			assertLegacyPayloadAccountingDefaults(t, event.Payload)
		})
	}
}

func TestParseEventEnvelopeStrictRejectsUserRequestInterruptedWithoutInterruptedBy(t *testing.T) {
	runID := mustUUIDv7String(t)
	raw := map[string]any{
		"event_id":       mustUUIDv7String(t),
		"schema_version": SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           EventTypeRunInterrupted,
		"causation_id":   mustUUIDv7String(t),
		"correlation_id": runID,
		"payload": map[string]any{
			"status": "interrupted",
			"reason": string(RunInterruptedReasonUserRequest),
		},
	}

	serialized, err := json.Marshal(raw)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}

	_, err = ParseEventEnvelopeStrict(serialized)
	if err == nil {
		t.Fatal("expected user_request interruption without interrupted_by to fail")
	}
	if !strings.Contains(err.Error(), "interrupted_by") {
		t.Fatalf("expected interrupted_by validation error, got %v", err)
	}
}

func TestParseEventEnvelopeStrictRejectsEmptyAccountingRefsForStepAndSubcallEvents(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		eventType EventType
		payload   map[string]any
	}{
		{
			name:      "node.step.completed",
			eventType: EventTypeNodeStepCompleted,
			payload: map[string]any{
				"step_id":        stepID,
				"decision":       string(StepDecisionContinue),
				"action_count":   1,
				"duration_ms":    1,
				"accounting":     testAccountingRollup(),
				"accounting_ref": "",
			},
		},
		{
			name:      "node.subcall.executed",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   string(SubcallTypeLLMQuery),
				"execution_mode": string(SubcallExecutionModePlain),
				"status":         string(ActionExecutionStatusCompleted),
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
				"accounting":     testAccountingSummary(),
				"accounting_ref": "",
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           testCase.eventType,
				"node_id":        nodeID,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			if _, err := ParseEventEnvelopeStrict(serialized); err == nil {
				t.Fatal("expected strict parse failure for empty accounting_ref")
			}
		})
	}
}

func TestParseEventEnvelopeStrictPreservesLegacyStepAndSubcallEventsWithoutAccounting(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		eventType EventType
		payload   map[string]any
	}{
		{
			name:      "node.step.completed",
			eventType: EventTypeNodeStepCompleted,
			payload: map[string]any{
				"step_id":      stepID,
				"decision":     string(StepDecisionContinue),
				"action_count": 1,
				"duration_ms":  1,
			},
		},
		{
			name:      "node.subcall.executed",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   string(SubcallTypeLLMQuery),
				"execution_mode": string(SubcallExecutionModePlain),
				"status":         string(ActionExecutionStatusCompleted),
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           testCase.eventType,
				"node_id":        nodeID,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			event, err := ParseEventEnvelopeStrict(serialized)
			if err != nil {
				t.Fatalf("expected legacy v1 event to remain parseable, got %v", err)
			}

			assertLegacyPayloadAccountingDefaults(t, event.Payload)
		})
	}
}

func TestParseEventEnvelopeStrictRejectsHalfPresentAccountingFieldsForStepAndSubcallEvents(t *testing.T) {
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	testCases := []struct {
		name      string
		eventType EventType
		payload   map[string]any
	}{
		{
			name:      "node.step.completed missing accounting",
			eventType: EventTypeNodeStepCompleted,
			payload: map[string]any{
				"step_id":        stepID,
				"decision":       string(StepDecisionContinue),
				"action_count":   1,
				"duration_ms":    1,
				"accounting_ref": testStepAccountingRef(nodeID, stepID),
			},
		},
		{
			name:      "node.step.completed missing accounting_ref",
			eventType: EventTypeNodeStepCompleted,
			payload: map[string]any{
				"step_id":      stepID,
				"decision":     string(StepDecisionContinue),
				"action_count": 1,
				"duration_ms":  1,
				"accounting":   testAccountingRollup(),
			},
		},
		{
			name:      "node.subcall.executed missing accounting",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   string(SubcallTypeLLMQuery),
				"execution_mode": string(SubcallExecutionModePlain),
				"status":         string(ActionExecutionStatusCompleted),
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
				"accounting_ref": testSubcallAccountingRef(nodeID, stepID, 1),
			},
		},
		{
			name:      "node.subcall.executed missing accounting_ref",
			eventType: EventTypeNodeSubcallExecuted,
			payload: map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   string(SubcallTypeLLMQuery),
				"execution_mode": string(SubcallExecutionModePlain),
				"status":         string(ActionExecutionStatusCompleted),
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
				"accounting":     testAccountingSummary(),
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			raw := map[string]any{
				"event_id":       mustUUIDv7String(t),
				"schema_version": SchemaVersionV1,
				"run_id":         runID,
				"seq":            2,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           testCase.eventType,
				"node_id":        nodeID,
				"causation_id":   mustUUIDv7String(t),
				"correlation_id": runID,
				"payload":        testCase.payload,
			}

			serialized, err := json.Marshal(raw)
			if err != nil {
				t.Fatalf("expected serialization success, got %v", err)
			}

			if _, err := ParseEventEnvelopeStrict(serialized); err == nil {
				t.Fatal("expected strict parse failure for half-present accounting fields")
			}
		})
	}
}

func testAccountingSummary() accounting.Summary {
	inputTokens := int64(12)
	outputTokens := int64(5)
	totalTokens := int64(17)
	reasoningTokens := int64(2)
	inputCost := int64(1200)
	outputCost := int64(400)
	reasoningCost := int64(150)
	totalCost := int64(1750)

	return accounting.Summary{
		Currency:                   accounting.CurrencyUSD,
		InputTokens:                &inputTokens,
		OutputTokens:               &outputTokens,
		TotalTokens:                &totalTokens,
		ReasoningTokens:            &reasoningTokens,
		KnownInputCostMicrousd:     &inputCost,
		KnownOutputCostMicrousd:    &outputCost,
		KnownReasoningCostMicrousd: &reasoningCost,
		KnownTotalCostMicrousd:     &totalCost,
		TokenSource:                accounting.SourceGatewayReported,
		TokenStatus:                accounting.StatusComplete,
		CostSource:                 accounting.SourceGatewayReported,
		CostStatus:                 accounting.StatusComplete,
		PricingKey: accounting.PricingKey{
			Provider: "openai",
			Model:    "gpt-5.1",
		},
		PricingVersion:        "v1",
		MissingTokenItemCount: 0,
		MissingCostItemCount:  0,
	}
}

func testAccountingRollup() accounting.Rollup {
	return accounting.BuildRollup(
		"openai",
		"gpt-5.1",
		"v1",
		testAccountingSummary(),
		accounting.ZeroSummary("openai", "gpt-5.1", "v1"),
	)
}

func testStepAccountingRef(nodeID string, stepID string) string {
	return "run-artifact://node/" + nodeID + "/step/" + stepID + "/accounting.json"
}

func testSubcallAccountingRef(nodeID string, stepID string, subcallIndex int) string {
	return fmt.Sprintf("run-artifact://node/%s/step/%s/subcall-%d-accounting.json", nodeID, stepID, subcallIndex)
}

func assertLegacyPayloadAccountingDefaults(t *testing.T, payload any) {
	t.Helper()

	switch typed := payload.(type) {
	case NodeCompletedPayload:
		if typed.Accounting.ModelTotal.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default node.completed accounting, got %+v", typed.Accounting)
		}
	case NodeStepCompletedPayload:
		if typed.Accounting.ModelTotal.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default node.step.completed accounting, got %+v", typed.Accounting)
		}
	case NodeSubcallExecutedPayload:
		if typed.Accounting.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default node.subcall.executed accounting, got %+v", typed.Accounting)
		}
	case RunCompletedPayload:
		if typed.Accounting.ModelTotal.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default run.completed accounting, got %+v", typed.Accounting)
		}
	case RunFailedPayload:
		if typed.Accounting.ModelTotal.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default run.failed accounting, got %+v", typed.Accounting)
		}
	case RunInterruptedPayload:
		if typed.Accounting.ModelTotal.Currency != accounting.CurrencyUSD {
			t.Fatalf("expected default run.interrupted accounting, got %+v", typed.Accounting)
		}
	default:
		t.Fatalf("unexpected payload type %T", payload)
	}
}

func stringPointer(value string) *string {
	return &value
}
