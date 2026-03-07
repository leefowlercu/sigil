package harness

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	stepInputPreviewCapBytes   = 2048
	smallContextBytesThreshold = 2000
	smallContextLineThreshold  = 20
)

// StepContextMetadata captures deterministic context identity and sizing details.
type StepContextMetadata struct {
	ContextType      string `json:"context_type"`
	ContextBytes     int    `json:"context_bytes"`
	ContextLineCount int    `json:"context_line_count"`
	ContextSHA256    string `json:"context_sha256"`
	ContextRef       string `json:"context_ref"`
}

// StepExecutionState captures deterministic node progress and recursion policy metadata.
type StepExecutionState struct {
	NodeDepth                 int     `json:"node_depth"`
	MaxDepth                  int     `json:"max_depth"`
	RemainingDepth            int     `json:"remaining_depth"`
	NodeStepsUsed             int     `json:"node_steps_used"`
	NodeStepsRemaining        int     `json:"node_steps_remaining"`
	RunStepsUsed              int     `json:"run_steps_used"`
	RunStepsRemaining         int     `json:"run_steps_remaining"`
	SameContextAsPreviousStep bool    `json:"same_context_as_previous_step"`
	SmallContext              bool    `json:"small_context"`
	RecursiveSubcallsAllowed  bool    `json:"recursive_subcalls_allowed"`
	RecursiveSubcallsReason   *string `json:"recursive_subcalls_reason,omitempty"`
}

// PreviousActionSubcallSummary captures deterministic counts for prior-step subcall execution.
type PreviousActionSubcallSummary struct {
	TotalCount     int `json:"total_count"`
	PlainCount     int `json:"plain_count"`
	RecursiveCount int `json:"recursive_count"`
	FallbackCount  int `json:"fallback_count"`
	CompletedCount int `json:"completed_count"`
	FailedCount    int `json:"failed_count"`
}

// PreviousActionFeedback is bounded feedback included in subsequent model-step input.
type PreviousActionFeedback struct {
	OutputRef       string                        `json:"output_ref"`
	Status          string                        `json:"status"`
	ErrorCode       *string                       `json:"error_code,omitempty"`
	ErrorMessage    *string                       `json:"error_message,omitempty"`
	ErrorDetail     *ActionErrorDetail            `json:"error_detail,omitempty"`
	SubcallSummary  *PreviousActionSubcallSummary `json:"subcall_summary,omitempty"`
	StdoutPreview   string                        `json:"stdout_preview"`
	StdoutBytes     int                           `json:"stdout_bytes"`
	StdoutTruncated bool                          `json:"stdout_truncated"`
	StderrPreview   string                        `json:"stderr_preview"`
	StderrBytes     int                           `json:"stderr_bytes"`
	StderrTruncated bool                          `json:"stderr_truncated"`
}

// StepInputEnvelope is the deterministic user message payload sent to model-step inference.
type StepInputEnvelope struct {
	Query                  string                  `json:"query"`
	StepIndex              int                     `json:"step_index"`
	ContextMetadata        StepContextMetadata     `json:"context_metadata"`
	ExecutionState         StepExecutionState      `json:"execution_state"`
	PreviousActionFeedback *PreviousActionFeedback `json:"previous_action_feedback,omitempty"`
}

func buildContextMetadata(rawContext string, contextRef string) StepContextMetadata {
	normalized := strings.ReplaceAll(rawContext, "\r\n", "\n")
	lineCount := 0
	if normalized != "" {
		lineCount = strings.Count(normalized, "\n") + 1
	}

	sum := sha256.Sum256([]byte(rawContext))
	return StepContextMetadata{
		ContextType:      "string",
		ContextBytes:     len(rawContext),
		ContextLineCount: lineCount,
		ContextSHA256:    hex.EncodeToString(sum[:]),
		ContextRef:       strings.TrimSpace(contextRef),
	}
}

func buildStepInputEnvelope(query string, stepIndex int, metadata StepContextMetadata, executionState StepExecutionState, feedback *PreviousActionFeedback) (StepInputEnvelope, error) {
	if strings.TrimSpace(query) == "" {
		return StepInputEnvelope{}, fmt.Errorf("query is required")
	}
	if stepIndex < 1 {
		return StepInputEnvelope{}, fmt.Errorf("step_index must be >= 1")
	}
	if strings.TrimSpace(metadata.ContextType) == "" {
		return StepInputEnvelope{}, fmt.Errorf("context_type is required")
	}
	if metadata.ContextBytes < 0 {
		return StepInputEnvelope{}, fmt.Errorf("context_bytes must be >= 0")
	}
	if metadata.ContextLineCount < 0 {
		return StepInputEnvelope{}, fmt.Errorf("context_line_count must be >= 0")
	}
	if strings.TrimSpace(metadata.ContextSHA256) == "" {
		return StepInputEnvelope{}, fmt.Errorf("context_sha256 is required")
	}
	if strings.TrimSpace(metadata.ContextRef) == "" {
		return StepInputEnvelope{}, fmt.Errorf("context_ref is required")
	}
	if executionState.NodeDepth < 0 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.node_depth must be >= 0")
	}
	if executionState.MaxDepth < 0 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.max_depth must be >= 0")
	}
	if executionState.RemainingDepth < 0 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.remaining_depth must be >= 0")
	}
	if executionState.NodeStepsUsed < 1 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.node_steps_used must be >= 1")
	}
	if executionState.NodeStepsRemaining < 0 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.node_steps_remaining must be >= 0")
	}
	if executionState.RunStepsUsed < 1 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.run_steps_used must be >= 1")
	}
	if executionState.RunStepsRemaining < 0 {
		return StepInputEnvelope{}, fmt.Errorf("execution_state.run_steps_remaining must be >= 0")
	}

	return StepInputEnvelope{
		Query:                  query,
		StepIndex:              stepIndex,
		ContextMetadata:        metadata,
		ExecutionState:         executionState,
		PreviousActionFeedback: feedback,
	}, nil
}

func encodeStepInputEnvelope(envelope StepInputEnvelope) (string, error) {
	encoded, err := json.Marshal(envelope)
	if err != nil {
		return "", fmt.Errorf("failed to encode step input envelope; %w", err)
	}
	return string(encoded), nil
}

func buildPreviousActionFeedback(runID string, artifacts *ActionArtifactStore, payload runtime.NodeActionExecutedPayload) (*PreviousActionFeedback, error) {
	if strings.TrimSpace(runID) == "" {
		return nil, fmt.Errorf("run id is required")
	}
	if artifacts == nil {
		return nil, fmt.Errorf("artifact store is required")
	}
	if strings.TrimSpace(payload.OutputRef) == "" {
		return nil, fmt.Errorf("output_ref is required")
	}

	artifact, err := artifacts.Read(runID, payload.OutputRef)
	if err != nil {
		return nil, err
	}

	stdoutPreview, stdoutBytes, stdoutTruncated := boundedPreview(artifact.Stdout, stepInputPreviewCapBytes)
	stderrPreview, stderrBytes, stderrTruncated := boundedPreview(artifact.Stderr, stepInputPreviewCapBytes)

	return &PreviousActionFeedback{
		OutputRef:       payload.OutputRef,
		Status:          string(payload.Status),
		ErrorCode:       cloneOptional(payload.ErrorCode),
		ErrorMessage:    cloneOptional(payload.ErrorMessage),
		ErrorDetail:     cloneActionErrorDetail(artifact.ErrorDetail),
		SubcallSummary:  buildPreviousActionSubcallSummary(artifact.Subcalls),
		StdoutPreview:   stdoutPreview,
		StdoutBytes:     stdoutBytes,
		StdoutTruncated: stdoutTruncated,
		StderrPreview:   stderrPreview,
		StderrBytes:     stderrBytes,
		StderrTruncated: stderrTruncated,
	}, nil
}

func buildStepExecutionState(node runtime.Node, maxDepth int, nodeStepsUsed int, nodeStepsRemaining int, runStepsUsed int, runStepsRemaining int, metadata StepContextMetadata, previousFeedback *PreviousActionFeedback, recursiveSubcallsAllowed bool, recursiveSubcallsReason *string) StepExecutionState {
	remainingDepth := maxDepth - node.Depth
	if remainingDepth < 0 {
		remainingDepth = 0
	}
	return StepExecutionState{
		NodeDepth:                 node.Depth,
		MaxDepth:                  maxDepth,
		RemainingDepth:            remainingDepth,
		NodeStepsUsed:             nodeStepsUsed,
		NodeStepsRemaining:        nodeStepsRemaining,
		RunStepsUsed:              runStepsUsed,
		RunStepsRemaining:         runStepsRemaining,
		SameContextAsPreviousStep: previousFeedback != nil,
		SmallContext:              isSmallContext(metadata),
		RecursiveSubcallsAllowed:  recursiveSubcallsAllowed,
		RecursiveSubcallsReason:   cloneOptional(recursiveSubcallsReason),
	}
}

func isSmallContext(metadata StepContextMetadata) bool {
	return metadata.ContextBytes <= smallContextBytesThreshold || metadata.ContextLineCount <= smallContextLineThreshold
}

func buildPreviousActionSubcallSummary(subcalls []ActionSubcallTrace) *PreviousActionSubcallSummary {
	if len(subcalls) == 0 {
		return nil
	}
	summary := &PreviousActionSubcallSummary{TotalCount: len(subcalls)}
	for _, subcall := range subcalls {
		switch subcall.ExecutionMode {
		case string(runtime.SubcallExecutionModePlain):
			summary.PlainCount++
		case string(runtime.SubcallExecutionModeRecursive):
			summary.RecursiveCount++
		case string(runtime.SubcallExecutionModeFallback):
			summary.FallbackCount++
		}
		if subcall.Status == string(runtime.ActionExecutionStatusFailed) {
			summary.FailedCount++
			continue
		}
		summary.CompletedCount++
	}
	return summary
}

func boundedPreview(value string, capBytes int) (string, int, bool) {
	raw := []byte(value)
	size := len(raw)
	if capBytes <= 0 || size <= capBytes {
		return value, size, false
	}
	return string(raw[:capBytes]), size, true
}

func cloneOptional(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneActionErrorDetail(detail *ActionErrorDetail) *ActionErrorDetail {
	if detail == nil {
		return nil
	}

	cloned := &ActionErrorDetail{
		Stage:      detail.Stage,
		Message:    detail.Message,
		Line:       cloneOptionalIntValue(detail.Line),
		Column:     cloneOptionalIntValue(detail.Column),
		Symbol:     cloneOptional(detail.Symbol),
		SourceLine: cloneOptional(detail.SourceLine),
	}
	return cloned
}

func cloneOptionalIntValue(value *int) *int {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
