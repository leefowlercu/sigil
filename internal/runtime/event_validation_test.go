package runtime

import (
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
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
			},
		},
		{
			name:      "run.completed",
			eventType: EventTypeRunCompleted,
			payload: RunCompletedPayload{
				Status:     "completed",
				DurationMS: 11,
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
			},
		},
		{
			name:      "run.interrupted",
			eventType: EventTypeRunInterrupted,
			payload: RunInterruptedPayload{
				Status: "interrupted",
				Reason: RunInterruptedReasonUserRequest,
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
			if testCase.eventType == EventTypeNodeStarted || testCase.eventType == EventTypeNodeCompleted {
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
