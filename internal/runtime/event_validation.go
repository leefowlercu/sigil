package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var (
	canonicalEventTypes = map[EventType]struct{}{
		EventTypeRunQueued:           {},
		EventTypeRunRunning:          {},
		EventTypeNodeStarted:         {},
		EventTypeNodeCompleted:       {},
		EventTypeNodeFailed:          {},
		EventTypeNodeStepStarted:     {},
		EventTypeNodeStepCompleted:   {},
		EventTypeNodeTurnUser:        {},
		EventTypeNodeTurnModel:       {},
		EventTypeNodeSubcallExecuted: {},
		EventTypeNodeActionExecuted:  {},
		EventTypeRunCompleted:        {},
		EventTypeRunFailed:           {},
		EventTypeRunInterrupted:      {},
	}

	runQueuedSources = map[RunQueuedSource]struct{}{
		RunQueuedSourceCLIRunStart:    {},
		RunQueuedSourceAppServerStart: {},
		RunQueuedSourceInternalResume: {},
	}

	runInterruptedReasons = map[RunInterruptedReason]struct{}{
		RunInterruptedReasonUserRequest:    {},
		RunInterruptedReasonPolicyStop:     {},
		RunInterruptedReasonSystemShutdown: {},
	}

	stepDecisions = map[StepDecision]struct{}{
		StepDecisionContinue: {},
	}

	actionExecutionStatuses = map[ActionExecutionStatus]struct{}{
		ActionExecutionStatusCompleted: {},
		ActionExecutionStatusFailed:    {},
	}

	subcallTypes = map[SubcallType]struct{}{
		SubcallTypeLLMQuery:        {},
		SubcallTypeLLMQueryBatched: {},
		SubcallTypeRLMQuery:        {},
		SubcallTypeRLMQueryBatched: {},
	}

	subcallExecutionModes = map[SubcallExecutionMode]struct{}{
		SubcallExecutionModePlain:     {},
		SubcallExecutionModeRecursive: {},
		SubcallExecutionModeFallback:  {},
	}
)

const (
	nodeStepSchemaIDV1     = "sigil.rlm.response.v1"
	nodeActionTypeReplCode = "repl_code"
	nodeActionLanguageGo   = "go"
)

type eventEnvelopeWire struct {
	EventID       string          `json:"event_id"`
	SchemaVersion string          `json:"schema_version"`
	RunID         string          `json:"run_id"`
	Seq           int64           `json:"seq"`
	Timestamp     string          `json:"ts"`
	Type          EventType       `json:"type"`
	NodeID        *string         `json:"node_id"`
	CausationID   string          `json:"causation_id"`
	CorrelationID string          `json:"correlation_id"`
	Payload       json.RawMessage `json:"payload"`
}

type eventValidationState struct {
	runID      string
	latestSeq  int64
	lastEvent  string
	eventByID  map[string]int64
	eventCount int
}

func newEventValidationState(runID string) eventValidationState {
	return eventValidationState{
		runID:     runID,
		eventByID: make(map[string]int64),
	}
}

func (s *eventValidationState) consume(event EventEnvelope) {
	s.latestSeq = event.Seq
	s.lastEvent = event.EventID
	s.eventByID[event.EventID] = event.Seq
	s.eventCount++
}

// ParseEventEnvelopeStrict decodes and validates a raw JSON envelope with strict v1 rules.
func ParseEventEnvelopeStrict(raw []byte) (EventEnvelope, error) {
	envelopeObj, err := decodeJSONObjectStrict(raw)
	if err != nil {
		return EventEnvelope{}, err
	}

	envelopeAllowed := map[string]struct{}{
		"event_id":       {},
		"schema_version": {},
		"run_id":         {},
		"seq":            {},
		"ts":             {},
		"type":           {},
		"node_id":        {},
		"causation_id":   {},
		"correlation_id": {},
		"payload":        {},
	}
	envelopeRequired := map[string]struct{}{
		"event_id":       {},
		"schema_version": {},
		"run_id":         {},
		"seq":            {},
		"ts":             {},
		"type":           {},
		"causation_id":   {},
		"correlation_id": {},
		"payload":        {},
	}
	if err := validateObjectKeys(envelopeObj, envelopeAllowed, envelopeRequired, ErrUnknownEnvelopeField); err != nil {
		return EventEnvelope{}, err
	}

	var wire eventEnvelopeWire
	if err := decodeJSONStrict(raw, &wire); err != nil {
		if isUnknownFieldDecodeError(err) {
			return EventEnvelope{}, fmt.Errorf("event envelope contains unknown field; %w", ErrUnknownEnvelopeField)
		}
		return EventEnvelope{}, fmt.Errorf("failed to decode event envelope; %w", ErrInvalidEvent)
	}

	timestamp, err := time.Parse(time.RFC3339Nano, wire.Timestamp)
	if err != nil {
		return EventEnvelope{}, fmt.Errorf("invalid ts %q; %w", wire.Timestamp, ErrInvalidEvent)
	}

	payload, err := decodeAndValidatePayload(wire.Type, wire.Payload)
	if err != nil {
		return EventEnvelope{}, err
	}

	event := EventEnvelope{
		EventID:       wire.EventID,
		SchemaVersion: wire.SchemaVersion,
		RunID:         wire.RunID,
		Seq:           wire.Seq,
		Timestamp:     timestamp,
		Type:          wire.Type,
		NodeID:        cloneStringPointer(wire.NodeID),
		CausationID:   wire.CausationID,
		CorrelationID: wire.CorrelationID,
		Payload:       payload,
	}

	event.Timestamp = event.Timestamp.UTC()
	if err := validateEventShape(event); err != nil {
		return EventEnvelope{}, err
	}

	return event, nil
}

func validateAndNormalizeEvent(event EventEnvelope) (EventEnvelope, error) {
	payload, err := normalizePayload(event.Type, event.Payload)
	if err != nil {
		return EventEnvelope{}, err
	}

	event.Payload = payload
	event.Timestamp = event.Timestamp.UTC()

	if err := validateEventShape(event); err != nil {
		return EventEnvelope{}, err
	}

	return event, nil
}

func validateEventForAppend(event EventEnvelope, state eventValidationState) (EventEnvelope, error) {
	validated, err := validateAndNormalizeEvent(event)
	if err != nil {
		return EventEnvelope{}, err
	}

	if strings.TrimSpace(state.runID) != "" && validated.RunID != state.runID {
		return EventEnvelope{}, fmt.Errorf("event run_id %q does not match store run_id %q; %w", validated.RunID, state.runID, ErrInvalidEvent)
	}

	nextSeq := state.latestSeq + 1
	if validated.Seq <= state.latestSeq {
		return EventEnvelope{}, fmt.Errorf("event seq %d rewrites prior sequence <= %d; %w", validated.Seq, state.latestSeq, ErrImmutableEventLog)
	}
	if validated.Seq != nextSeq {
		return EventEnvelope{}, fmt.Errorf("event seq %d must equal next contiguous seq %d; %w", validated.Seq, nextSeq, ErrNonContiguousSequence)
	}

	if _, exists := state.eventByID[validated.EventID]; exists {
		return EventEnvelope{}, fmt.Errorf("event_id %q already exists in run; %w", validated.EventID, ErrInvalidEvent)
	}

	if validated.Seq == 1 {
		if validated.Type != EventTypeRunQueued {
			return EventEnvelope{}, fmt.Errorf("first event type must be %q, got %q; %w", EventTypeRunQueued, validated.Type, ErrInvalidEvent)
		}
		if validated.CausationID != validated.EventID {
			return EventEnvelope{}, fmt.Errorf("first event causation_id must equal event_id; %w", ErrInvalidEvent)
		}
		return validated, nil
	}

	priorSeq, ok := state.eventByID[validated.CausationID]
	if !ok {
		return EventEnvelope{}, fmt.Errorf("causation_id %q does not reference a prior event in run; %w", validated.CausationID, ErrInvalidEvent)
	}
	if priorSeq >= validated.Seq {
		return EventEnvelope{}, fmt.Errorf("causation_id %q must reference lower seq than %d; %w", validated.CausationID, validated.Seq, ErrInvalidEvent)
	}

	return validated, nil
}

func normalizePayload(eventType EventType, payload any) (any, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload for %q; %w", eventType, ErrInvalidEvent)
	}
	return decodeAndValidatePayload(eventType, raw)
}

func decodeAndValidatePayload(eventType EventType, payloadRaw []byte) (any, error) {
	switch eventType {
	case EventTypeRunQueued:
		allowed := map[string]struct{}{
			"name":            {},
			"source":          {},
			"app_config_path": {},
			"run_config_path": {},
		}
		required := map[string]struct{}{
			"name":   {},
			"source": {},
		}
		var payload RunQueuedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if _, ok := runQueuedSources[payload.Source]; !ok {
			return nil, fmt.Errorf("invalid run.queued source %q; %w", payload.Source, ErrInvalidEvent)
		}
		if payload.AppConfigPath != nil && strings.TrimSpace(*payload.AppConfigPath) == "" {
			return nil, fmt.Errorf("run.queued app_config_path must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.RunConfigPath != nil && strings.TrimSpace(*payload.RunConfigPath) == "" {
			return nil, fmt.Errorf("run.queued run_config_path must be non-empty when present; %w", ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeRunRunning:
		allowed := map[string]struct{}{
			"executor":  {},
			"max_depth": {},
		}
		required := map[string]struct{}{
			"executor":  {},
			"max_depth": {},
		}
		var payload RunRunningPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Executor != "rlm" {
			return nil, fmt.Errorf("run.running executor must be rlm; %w", ErrInvalidEvent)
		}
		if payload.MaxDepth < 0 {
			return nil, fmt.Errorf("run.running max_depth must be >= 0; %w", ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeStarted:
		allowed := map[string]struct{}{
			"depth":          {},
			"parent_node_id": {},
			"role":           {},
			"attempt":        {},
		}
		required := map[string]struct{}{
			"depth":          {},
			"parent_node_id": {},
			"role":           {},
			"attempt":        {},
		}
		var payload NodeStartedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Depth < 0 {
			return nil, fmt.Errorf("node.started depth must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.Attempt < 1 {
			return nil, fmt.Errorf("node.started attempt must be >= 1; %w", ErrInvalidEvent)
		}
		if payload.Depth == 0 {
			if payload.ParentNodeID != nil {
				return nil, fmt.Errorf("node.started root payload requires parent_node_id=null; %w", ErrInvalidEvent)
			}
			if payload.Role != NodeRoleRoot {
				return nil, fmt.Errorf("node.started root payload requires role=%q; %w", NodeRoleRoot, ErrInvalidEvent)
			}
			return payload, nil
		}
		if payload.ParentNodeID == nil {
			return nil, fmt.Errorf("node.started non-root payload requires parent_node_id; %w", ErrInvalidEvent)
		}
		if err := validateUUIDv7String(*payload.ParentNodeID); err != nil {
			return nil, fmt.Errorf("node.started parent_node_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		if payload.Role != NodeRoleRecursiveSubcall {
			return nil, fmt.Errorf("node.started non-root payload requires role=%q; %w", NodeRoleRecursiveSubcall, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeCompleted:
		allowed := map[string]struct{}{
			"status":         {},
			"duration_ms":    {},
			"result_ref":     {},
			"accounting":     {},
			"accounting_ref": {},
		}
		required := map[string]struct{}{
			"status":      {},
			"duration_ms": {},
		}
		var payload NodeCompletedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Status != "completed" {
			return nil, fmt.Errorf("node.completed status must be completed; %w", ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("node.completed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.ResultRef != nil && strings.TrimSpace(*payload.ResultRef) == "" {
			return nil, fmt.Errorf("node.completed result_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.AccountingRef != nil && strings.TrimSpace(*payload.AccountingRef) == "" {
			return nil, fmt.Errorf("node.completed accounting_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
			return nil, fmt.Errorf("node.completed accounting %w; %w", err, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeFailed:
		allowed := map[string]struct{}{
			"status":         {},
			"duration_ms":    {},
			"error_code":     {},
			"error_message":  {},
			"failed_step_id": {},
		}
		required := map[string]struct{}{
			"status":        {},
			"duration_ms":   {},
			"error_code":    {},
			"error_message": {},
		}
		var payload NodeFailedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Status != "failed" {
			return nil, fmt.Errorf("node.failed status must be failed; %w", ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("node.failed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ErrorCode) == "" {
			return nil, fmt.Errorf("node.failed error_code must be non-empty; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ErrorMessage) == "" {
			return nil, fmt.Errorf("node.failed error_message must be non-empty; %w", ErrInvalidEvent)
		}
		if payload.FailedStepID != nil {
			if err := validateUUIDv7String(*payload.FailedStepID); err != nil {
				return nil, fmt.Errorf("node.failed failed_step_id must be UUIDv7 when present; %w", ErrInvalidEvent)
			}
		}
		return payload, nil
	case EventTypeNodeStepStarted:
		allowed := map[string]struct{}{
			"step_id":    {},
			"step_index": {},
			"schema_id":  {},
		}
		required := map[string]struct{}{
			"step_id":    {},
			"step_index": {},
			"schema_id":  {},
		}
		var payload NodeStepStartedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if err := validateUUIDv7String(payload.StepID); err != nil {
			return nil, fmt.Errorf("node.step.started step_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		if payload.StepIndex < 1 {
			return nil, fmt.Errorf("node.step.started step_index must be >= 1; %w", ErrInvalidEvent)
		}
		if payload.SchemaID != nodeStepSchemaIDV1 {
			return nil, fmt.Errorf("node.step.started schema_id must be %q; %w", nodeStepSchemaIDV1, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeStepCompleted:
		allowed := map[string]struct{}{
			"step_id":        {},
			"decision":       {},
			"action_count":   {},
			"duration_ms":    {},
			"accounting":     {},
			"accounting_ref": {},
		}
		required := map[string]struct{}{
			"step_id":      {},
			"decision":     {},
			"action_count": {},
			"duration_ms":  {},
		}
		var payload NodeStepCompletedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		payloadObj, err := decodeJSONObjectStrict(payloadRaw)
		if err != nil {
			return nil, fmt.Errorf("payload must be a JSON object; %w", ErrInvalidEvent)
		}
		if err := validateUUIDv7String(payload.StepID); err != nil {
			return nil, fmt.Errorf("node.step.completed step_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		if _, ok := stepDecisions[payload.Decision]; !ok {
			return nil, fmt.Errorf("node.step.completed decision %q is not supported; %w", payload.Decision, ErrInvalidEvent)
		}
		if payload.ActionCount != 1 {
			return nil, fmt.Errorf("node.step.completed action_count must be 1; %w", ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("node.step.completed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if err := validatePairedAccountingFields(payloadObj, "accounting", "accounting_ref", "node.step.completed", payload.AccountingRef); err != nil {
			return nil, err
		}
		if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
			return nil, fmt.Errorf("node.step.completed accounting %w; %w", err, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeTurnUser, EventTypeNodeTurnModel:
		allowed := map[string]struct{}{
			"step_id":          {},
			"role":             {},
			"content_ref":      {},
			"input_tokens":     {},
			"output_tokens":    {},
			"total_tokens":     {},
			"reasoning_tokens": {},
		}
		required := map[string]struct{}{
			"step_id":     {},
			"role":        {},
			"content_ref": {},
		}
		var payload NodeTurnPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if err := validateUUIDv7String(payload.StepID); err != nil {
			return nil, fmt.Errorf("node.turn step_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		expectedRole := TurnRoleUser
		if eventType == EventTypeNodeTurnModel {
			expectedRole = TurnRoleModel
		}
		if payload.Role != expectedRole {
			return nil, fmt.Errorf("node.turn payload role must be %q for event type %q; %w", expectedRole, eventType, ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ContentRef) == "" {
			return nil, fmt.Errorf("node.turn content_ref must be non-empty; %w", ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeSubcallExecuted:
		allowed := map[string]struct{}{
			"step_id":        {},
			"action_index":   {},
			"subcall_index":  {},
			"subcall_type":   {},
			"execution_mode": {},
			"status":         {},
			"provider":       {},
			"model":          {},
			"prompt_bytes":   {},
			"context_bytes":  {},
			"answer_bytes":   {},
			"duration_ms":    {},
			"child_node_id":  {},
			"accounting":     {},
			"accounting_ref": {},
			"error_code":     {},
			"error_message":  {},
		}
		required := map[string]struct{}{
			"step_id":        {},
			"action_index":   {},
			"subcall_index":  {},
			"subcall_type":   {},
			"execution_mode": {},
			"status":         {},
			"provider":       {},
			"model":          {},
			"prompt_bytes":   {},
			"context_bytes":  {},
			"answer_bytes":   {},
			"duration_ms":    {},
		}
		var payload NodeSubcallExecutedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		payloadObj, err := decodeJSONObjectStrict(payloadRaw)
		if err != nil {
			return nil, fmt.Errorf("payload must be a JSON object; %w", ErrInvalidEvent)
		}
		if err := validateUUIDv7String(payload.StepID); err != nil {
			return nil, fmt.Errorf("node.subcall.executed step_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		if payload.ActionIndex != 1 {
			return nil, fmt.Errorf("node.subcall.executed action_index must be 1 in v1; %w", ErrInvalidEvent)
		}
		if payload.SubcallIndex < 1 {
			return nil, fmt.Errorf("node.subcall.executed subcall_index must be >= 1; %w", ErrInvalidEvent)
		}
		if _, ok := subcallTypes[payload.SubcallType]; !ok {
			return nil, fmt.Errorf("node.subcall.executed subcall_type %q is not supported; %w", payload.SubcallType, ErrInvalidEvent)
		}
		if _, ok := subcallExecutionModes[payload.ExecutionMode]; !ok {
			return nil, fmt.Errorf("node.subcall.executed execution_mode %q is not supported; %w", payload.ExecutionMode, ErrInvalidEvent)
		}
		if _, ok := actionExecutionStatuses[payload.Status]; !ok {
			return nil, fmt.Errorf("node.subcall.executed status %q is not supported; %w", payload.Status, ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.Provider) == "" {
			return nil, fmt.Errorf("node.subcall.executed provider must be non-empty; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.Model) == "" {
			return nil, fmt.Errorf("node.subcall.executed model must be non-empty; %w", ErrInvalidEvent)
		}
		if payload.PromptBytes < 0 {
			return nil, fmt.Errorf("node.subcall.executed prompt_bytes must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.ContextBytes < 0 {
			return nil, fmt.Errorf("node.subcall.executed context_bytes must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.AnswerBytes < 0 {
			return nil, fmt.Errorf("node.subcall.executed answer_bytes must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("node.subcall.executed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if err := validatePairedAccountingFields(payloadObj, "accounting", "accounting_ref", "node.subcall.executed", payload.AccountingRef); err != nil {
			return nil, err
		}
		if err := ensureSummaryOrDefault(&payload.Accounting, payload.Provider, payload.Model); err != nil {
			return nil, fmt.Errorf("node.subcall.executed accounting %w; %w", err, ErrInvalidEvent)
		}
		if payload.ExecutionMode == SubcallExecutionModeRecursive {
			if payload.ChildNodeID == nil {
				return nil, fmt.Errorf("node.subcall.executed recursive mode requires child_node_id; %w", ErrInvalidEvent)
			}
			if err := validateUUIDv7String(*payload.ChildNodeID); err != nil {
				return nil, fmt.Errorf("node.subcall.executed child_node_id must be UUIDv7 when present; %w", ErrInvalidEvent)
			}
		} else if payload.ChildNodeID != nil {
			return nil, fmt.Errorf("node.subcall.executed child_node_id is only allowed for recursive mode; %w", ErrInvalidEvent)
		}
		if payload.Status == ActionExecutionStatusCompleted {
			if payload.ErrorCode != nil || payload.ErrorMessage != nil {
				return nil, fmt.Errorf("node.subcall.executed completed status forbids error_code and error_message; %w", ErrInvalidEvent)
			}
			return payload, nil
		}
		if payload.ErrorCode == nil || strings.TrimSpace(*payload.ErrorCode) == "" {
			return nil, fmt.Errorf("node.subcall.executed failed status requires non-empty error_code; %w", ErrInvalidEvent)
		}
		if payload.ErrorMessage == nil || strings.TrimSpace(*payload.ErrorMessage) == "" {
			return nil, fmt.Errorf("node.subcall.executed failed status requires non-empty error_message; %w", ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeNodeActionExecuted:
		allowed := map[string]struct{}{
			"step_id":       {},
			"action_index":  {},
			"action_type":   {},
			"language":      {},
			"status":        {},
			"duration_ms":   {},
			"action_ref":    {},
			"error_code":    {},
			"error_message": {},
		}
		required := map[string]struct{}{
			"step_id":      {},
			"action_index": {},
			"action_type":  {},
			"language":     {},
			"status":       {},
			"duration_ms":  {},
			"action_ref":   {},
		}
		var payload NodeActionExecutedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if err := validateUUIDv7String(payload.StepID); err != nil {
			return nil, fmt.Errorf("node.action.executed step_id must be UUIDv7; %w", ErrInvalidEvent)
		}
		if payload.ActionIndex != 1 {
			return nil, fmt.Errorf("node.action.executed action_index must be 1 in v1; %w", ErrInvalidEvent)
		}
		if payload.ActionType != nodeActionTypeReplCode {
			return nil, fmt.Errorf("node.action.executed action_type must be %q; %w", nodeActionTypeReplCode, ErrInvalidEvent)
		}
		if payload.Language != nodeActionLanguageGo {
			return nil, fmt.Errorf("node.action.executed language must be %q; %w", nodeActionLanguageGo, ErrInvalidEvent)
		}
		if _, ok := actionExecutionStatuses[payload.Status]; !ok {
			return nil, fmt.Errorf("node.action.executed status %q is not supported; %w", payload.Status, ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("node.action.executed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ActionRef) == "" {
			return nil, fmt.Errorf("node.action.executed action_ref must be non-empty; %w", ErrInvalidEvent)
		}
		if _, err := ParseActionArtifactRef(payload.ActionRef); err != nil {
			return nil, fmt.Errorf("node.action.executed action_ref is invalid; %w", err)
		}
		if payload.Status == ActionExecutionStatusCompleted {
			if payload.ErrorCode != nil || payload.ErrorMessage != nil {
				return nil, fmt.Errorf("node.action.executed completed status forbids error_code and error_message; %w", ErrInvalidEvent)
			}
			return payload, nil
		}
		if payload.ErrorCode == nil || strings.TrimSpace(*payload.ErrorCode) == "" {
			return nil, fmt.Errorf("node.action.executed failed status requires non-empty error_code; %w", ErrInvalidEvent)
		}
		if payload.ErrorMessage == nil || strings.TrimSpace(*payload.ErrorMessage) == "" {
			return nil, fmt.Errorf("node.action.executed failed status requires non-empty error_message; %w", ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeRunCompleted:
		allowed := map[string]struct{}{
			"status":           {},
			"duration_ms":      {},
			"final_answer_ref": {},
			"accounting":       {},
			"accounting_ref":   {},
		}
		required := map[string]struct{}{
			"status":      {},
			"duration_ms": {},
		}
		var payload RunCompletedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Status != "completed" {
			return nil, fmt.Errorf("run.completed status must be completed; %w", ErrInvalidEvent)
		}
		if payload.DurationMS < 0 {
			return nil, fmt.Errorf("run.completed duration_ms must be >= 0; %w", ErrInvalidEvent)
		}
		if payload.FinalAnswerRef != nil && strings.TrimSpace(*payload.FinalAnswerRef) == "" {
			return nil, fmt.Errorf("run.completed final_answer_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.AccountingRef != nil && strings.TrimSpace(*payload.AccountingRef) == "" {
			return nil, fmt.Errorf("run.completed accounting_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
			return nil, fmt.Errorf("run.completed accounting %w; %w", err, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeRunFailed:
		allowed := map[string]struct{}{
			"status":           {},
			"error_code":       {},
			"error_message":    {},
			"failed_node_id":   {},
			"failed_step_id":   {},
			"limit_key":        {},
			"configured_value": {},
			"observed_value":   {},
			"retryable":        {},
			"accounting":       {},
			"accounting_ref":   {},
		}
		required := map[string]struct{}{
			"status":        {},
			"error_code":    {},
			"error_message": {},
			"retryable":     {},
		}
		var payload RunFailedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		payloadObj, err := decodeJSONObjectStrict(payloadRaw)
		if err != nil {
			return nil, fmt.Errorf("payload must be a JSON object; %w", ErrInvalidEvent)
		}
		if payload.Status != "failed" {
			return nil, fmt.Errorf("run.failed status must be failed; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ErrorCode) == "" {
			return nil, fmt.Errorf("run.failed error_code must be non-empty; %w", ErrInvalidEvent)
		}
		if strings.TrimSpace(payload.ErrorMessage) == "" {
			return nil, fmt.Errorf("run.failed error_message must be non-empty; %w", ErrInvalidEvent)
		}
		if payload.FailedNodeID != nil {
			if err := validateUUIDv7String(*payload.FailedNodeID); err != nil {
				return nil, fmt.Errorf("run.failed failed_node_id must be UUIDv7 when present; %w", ErrInvalidEvent)
			}
		}
		if fieldIsExplicitNull(payloadObj, "failed_step_id") {
			return nil, fmt.Errorf("run.failed failed_step_id must not be null when present; %w", ErrInvalidEvent)
		}
		if payload.FailedStepID != nil {
			if err := validateUUIDv7String(*payload.FailedStepID); err != nil {
				return nil, fmt.Errorf("run.failed failed_step_id must be UUIDv7 when present; %w", ErrInvalidEvent)
			}
		}
		if fieldIsExplicitNull(payloadObj, "limit_key") {
			return nil, fmt.Errorf("run.failed limit_key must not be null when present; %w", ErrInvalidEvent)
		}
		if fieldIsExplicitNull(payloadObj, "configured_value") {
			return nil, fmt.Errorf("run.failed configured_value must not be null when present; %w", ErrInvalidEvent)
		}
		if fieldIsExplicitNull(payloadObj, "observed_value") {
			return nil, fmt.Errorf("run.failed observed_value must not be null when present; %w", ErrInvalidEvent)
		}
		if payload.LimitKey != nil {
			if strings.TrimSpace(*payload.LimitKey) == "" {
				return nil, fmt.Errorf("run.failed limit_key must be non-empty when present; %w", ErrInvalidEvent)
			}
			if payload.ConfiguredValue == nil || strings.TrimSpace(*payload.ConfiguredValue) == "" {
				return nil, fmt.Errorf("run.failed configured_value must be present and non-empty when limit_key is present; %w", ErrInvalidEvent)
			}
			if payload.ObservedValue == nil || strings.TrimSpace(*payload.ObservedValue) == "" {
				return nil, fmt.Errorf("run.failed observed_value must be present and non-empty when limit_key is present; %w", ErrInvalidEvent)
			}
		}
		if payload.ConfiguredValue != nil && strings.TrimSpace(*payload.ConfiguredValue) == "" {
			return nil, fmt.Errorf("run.failed configured_value must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.ObservedValue != nil && strings.TrimSpace(*payload.ObservedValue) == "" {
			return nil, fmt.Errorf("run.failed observed_value must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.AccountingRef != nil && strings.TrimSpace(*payload.AccountingRef) == "" {
			return nil, fmt.Errorf("run.failed accounting_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
			return nil, fmt.Errorf("run.failed accounting %w; %w", err, ErrInvalidEvent)
		}
		return payload, nil
	case EventTypeRunInterrupted:
		allowed := map[string]struct{}{
			"status":              {},
			"reason":              {},
			"interrupted_by":      {},
			"interrupted_node_id": {},
			"accounting":          {},
			"accounting_ref":      {},
		}
		required := map[string]struct{}{
			"status": {},
			"reason": {},
		}
		var payload RunInterruptedPayload
		if err := decodePayloadStrict(payloadRaw, allowed, required, &payload); err != nil {
			return nil, err
		}
		if payload.Status != "interrupted" {
			return nil, fmt.Errorf("run.interrupted status must be interrupted; %w", ErrInvalidEvent)
		}
		if _, ok := runInterruptedReasons[payload.Reason]; !ok {
			return nil, fmt.Errorf("run.interrupted reason %q is not supported; %w", payload.Reason, ErrInvalidEvent)
		}
		if payload.Reason == RunInterruptedReasonUserRequest {
			if payload.InterruptedBy == nil || strings.TrimSpace(*payload.InterruptedBy) == "" {
				return nil, fmt.Errorf("run.interrupted interrupted_by is required when reason=user_request; %w", ErrInvalidEvent)
			}
		} else if payload.InterruptedBy != nil && strings.TrimSpace(*payload.InterruptedBy) == "" {
			return nil, fmt.Errorf("run.interrupted interrupted_by must be non-empty when present; %w", ErrInvalidEvent)
		}
		if payload.InterruptedNodeID != nil {
			if err := validateUUIDv7String(*payload.InterruptedNodeID); err != nil {
				return nil, fmt.Errorf("run.interrupted interrupted_node_id must be UUIDv7 when present; %w", ErrInvalidEvent)
			}
		}
		if payload.AccountingRef != nil && strings.TrimSpace(*payload.AccountingRef) == "" {
			return nil, fmt.Errorf("run.interrupted accounting_ref must be non-empty when present; %w", ErrInvalidEvent)
		}
		if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
			return nil, fmt.Errorf("run.interrupted accounting %w; %w", err, ErrInvalidEvent)
		}
		return payload, nil
	default:
		return nil, fmt.Errorf("event type %q is not canonical v1; %w", eventType, ErrUnknownEventType)
	}
}

func decodePayloadStrict(raw []byte, allowed map[string]struct{}, required map[string]struct{}, out any) error {
	obj, err := decodeJSONObjectStrict(raw)
	if err != nil {
		return fmt.Errorf("payload must be a JSON object; %w", ErrInvalidEvent)
	}
	if err := validateObjectKeys(obj, allowed, required, ErrUnknownPayloadField); err != nil {
		return err
	}

	if err := decodeJSONStrict(raw, out); err != nil {
		if isUnknownFieldDecodeError(err) {
			return fmt.Errorf("payload contains unknown field; %w", ErrUnknownPayloadField)
		}
		return fmt.Errorf("failed to decode payload; %w", ErrInvalidEvent)
	}

	return nil
}

func fieldIsExplicitNull(obj map[string]json.RawMessage, field string) bool {
	value, ok := obj[field]
	if !ok {
		return false
	}
	return bytes.Equal(bytes.TrimSpace(value), []byte("null"))
}

func validatePairedAccountingFields(obj map[string]json.RawMessage, accountingField string, accountingRefField string, eventType string, accountingRef string) error {
	_, accountingPresent := obj[accountingField]
	_, accountingRefPresent := obj[accountingRefField]
	if accountingPresent != accountingRefPresent {
		return fmt.Errorf("%s accounting and accounting_ref must both be present or both be omitted; %w", eventType, ErrInvalidEvent)
	}
	if !accountingPresent {
		return nil
	}
	if fieldIsExplicitNull(obj, accountingField) {
		return fmt.Errorf("%s accounting must not be null when present; %w", eventType, ErrInvalidEvent)
	}
	if fieldIsExplicitNull(obj, accountingRefField) || strings.TrimSpace(accountingRef) == "" {
		return fmt.Errorf("%s accounting_ref must be non-empty; %w", eventType, ErrInvalidEvent)
	}
	return nil
}

func validateEventShape(event EventEnvelope) error {
	if event.SchemaVersion != SchemaVersionV1 {
		return fmt.Errorf("schema_version %q is not supported; %w", event.SchemaVersion, ErrInvalidSchemaVersion)
	}

	if event.Seq < 1 {
		return fmt.Errorf("seq must be >= 1; %w", ErrInvalidEvent)
	}

	if _, ok := canonicalEventTypes[event.Type]; !ok {
		return fmt.Errorf("event type %q is not canonical v1; %w", event.Type, ErrUnknownEventType)
	}

	if err := validateUUIDv7String(event.EventID); err != nil {
		return fmt.Errorf("event_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(event.RunID); err != nil {
		return fmt.Errorf("run_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(event.CausationID); err != nil {
		return fmt.Errorf("causation_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if err := validateUUIDv7String(event.CorrelationID); err != nil {
		return fmt.Errorf("correlation_id must be UUIDv7; %w", ErrInvalidEvent)
	}
	if event.NodeID != nil {
		if err := validateUUIDv7String(*event.NodeID); err != nil {
			return fmt.Errorf("node_id must be UUIDv7; %w", ErrInvalidEvent)
		}
	}

	if event.CorrelationID != event.RunID {
		return fmt.Errorf("correlation_id must equal run_id; %w", ErrInvalidEvent)
	}

	if event.Seq == 1 {
		if event.Type != EventTypeRunQueued {
			return fmt.Errorf("first event type must be %q; %w", EventTypeRunQueued, ErrInvalidEvent)
		}
		if event.CausationID != event.EventID {
			return fmt.Errorf("first event causation_id must equal event_id; %w", ErrInvalidEvent)
		}
	}

	if event.Seq > 1 && event.CausationID == event.EventID {
		return fmt.Errorf("causation_id cannot self-reference after first event; %w", ErrInvalidEvent)
	}

	if event.Timestamp.IsZero() {
		return fmt.Errorf("ts is required; %w", ErrInvalidEvent)
	}
	if offsetSeconds := timestampUTCOffsetSeconds(event.Timestamp); offsetSeconds != 0 {
		return fmt.Errorf("ts must be UTC; %w", ErrInvalidEvent)
	}

	if isRunScopedEventType(event.Type) {
		if event.NodeID != nil {
			return fmt.Errorf("run-scoped event type %q must not include node_id; %w", event.Type, ErrInvalidEvent)
		}
	}
	if isNodeScopedEventType(event.Type) {
		if event.NodeID == nil {
			return fmt.Errorf("node-scoped event type %q requires node_id; %w", event.Type, ErrInvalidEvent)
		}
	}
	if event.Type == EventTypeNodeActionExecuted {
		payload, ok := event.Payload.(NodeActionExecutedPayload)
		if !ok {
			return fmt.Errorf("node.action.executed payload type is invalid; %w", ErrInvalidEvent)
		}
		parsedRef, err := ParseActionArtifactRef(payload.ActionRef)
		if err != nil {
			return fmt.Errorf("node.action.executed action_ref is invalid; %w", err)
		}
		if event.NodeID == nil || parsedRef.NodeID != *event.NodeID {
			return fmt.Errorf("node.action.executed action_ref node id must match event node_id; %w", ErrInvalidEvent)
		}
		if parsedRef.StepID != payload.StepID {
			return fmt.Errorf("node.action.executed action_ref step id must match payload step_id; %w", ErrInvalidEvent)
		}
		if parsedRef.ActionIndex != payload.ActionIndex {
			return fmt.Errorf("node.action.executed action_ref action index must match payload action_index; %w", ErrInvalidEvent)
		}
	}

	return nil
}

func validateUUIDv7String(raw string) error {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return err
	}
	if parsed.Version() != uuid.Version(7) {
		return fmt.Errorf("expected UUIDv7, got UUIDv%d", parsed.Version())
	}
	return nil
}

func timestampUTCOffsetSeconds(ts time.Time) int {
	_, offset := ts.Zone()
	return offset
}

func isRunScopedEventType(eventType EventType) bool {
	return strings.HasPrefix(string(eventType), "run.")
}

func isNodeScopedEventType(eventType EventType) bool {
	return strings.HasPrefix(string(eventType), "node.")
}

func decodeJSONObjectStrict(raw []byte) (map[string]json.RawMessage, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	var object map[string]json.RawMessage
	if err := dec.Decode(&object); err != nil {
		return nil, err
	}
	if object == nil {
		return nil, fmt.Errorf("json object is required")
	}
	if dec.More() {
		return nil, fmt.Errorf("unexpected trailing JSON content")
	}
	return object, nil
}

func validateObjectKeys(object map[string]json.RawMessage, allowed map[string]struct{}, required map[string]struct{}, unknownSentinel error) error {
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return fmt.Errorf("field %q is not allowed; %w", key, unknownSentinel)
		}
	}
	for key := range required {
		if _, ok := object[key]; !ok {
			return fmt.Errorf("required field %q is missing; %w", key, ErrInvalidEvent)
		}
	}
	return nil
}

func decodeJSONStrict(raw []byte, out any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

func isUnknownFieldDecodeError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "unknown field")
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}
