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
	stepInputPreviewCapBytes = 2048
)

// StepContextMetadata captures deterministic context identity and sizing details.
type StepContextMetadata struct {
	ContextType      string `json:"context_type"`
	ContextBytes     int    `json:"context_bytes"`
	ContextLineCount int    `json:"context_line_count"`
	ContextSHA256    string `json:"context_sha256"`
	ContextRef       string `json:"context_ref"`
}

// PreviousActionFeedback is bounded feedback included in subsequent model-step input.
type PreviousActionFeedback struct {
	OutputRef       string  `json:"output_ref"`
	Status          string  `json:"status"`
	ErrorCode       *string `json:"error_code,omitempty"`
	ErrorMessage    *string `json:"error_message,omitempty"`
	StdoutPreview   string  `json:"stdout_preview"`
	StdoutBytes     int     `json:"stdout_bytes"`
	StdoutTruncated bool    `json:"stdout_truncated"`
	StderrPreview   string  `json:"stderr_preview"`
	StderrBytes     int     `json:"stderr_bytes"`
	StderrTruncated bool    `json:"stderr_truncated"`
}

// StepInputEnvelope is the deterministic user message payload sent to model-step inference.
type StepInputEnvelope struct {
	Query                  string                  `json:"query"`
	StepIndex              int                     `json:"step_index"`
	ContextMetadata        StepContextMetadata     `json:"context_metadata"`
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

func buildStepInputEnvelope(query string, stepIndex int, metadata StepContextMetadata, feedback *PreviousActionFeedback) (StepInputEnvelope, error) {
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

	return StepInputEnvelope{
		Query:                  query,
		StepIndex:              stepIndex,
		ContextMetadata:        metadata,
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
		StdoutPreview:   stdoutPreview,
		StdoutBytes:     stdoutBytes,
		StdoutTruncated: stdoutTruncated,
		StderrPreview:   stderrPreview,
		StderrBytes:     stderrBytes,
		StderrTruncated: stderrTruncated,
	}, nil
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
