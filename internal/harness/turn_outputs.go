package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/inference"
)

// TurnOutputStore persists user/model turn artifacts and final-answer outputs.
type TurnOutputStore struct {
	runsBaseDir string
}

// NewTurnOutputStore constructs a turn output store for a runs base directory.
func NewTurnOutputStore(runsBaseDir string) (*TurnOutputStore, error) {
	clean := strings.TrimSpace(runsBaseDir)
	if clean == "" {
		return nil, fmt.Errorf("runs base directory is required")
	}

	return &TurnOutputStore{runsBaseDir: clean}, nil
}

type userTurnArtifact struct {
	RunID              string                    `json:"run_id"`
	NodeID             string                    `json:"node_id"`
	StepID             string                    `json:"step_id"`
	ModelInputEnvelope StepInputEnvelope         `json:"model_input_envelope"`
	ModelInputMessages []userTurnMessageArtifact `json:"model_input_messages"`
}

type userTurnMessageArtifact struct {
	Role         string `json:"role"`
	ContentBytes int    `json:"content_bytes"`
}

type modelTurnArtifact struct {
	RunID         string             `json:"run_id"`
	NodeID        string             `json:"node_id"`
	StepID        string             `json:"step_id"`
	SchemaID      string             `json:"schema_id"`
	Validated     map[string]any     `json:"validated_payload"`
	Gateway       string             `json:"gateway"`
	Provider      string             `json:"provider"`
	Model         string             `json:"model"`
	GatewayRespID string             `json:"gateway_response_id"`
	FinishStatus  string             `json:"finish_status"`
	Reasoning     any                `json:"reasoning"`
	Usage         any                `json:"usage"`
	Accounting    accounting.Summary `json:"accounting"`
	RawMetadata   map[string]any     `json:"raw_metadata"`
}

type contextArtifact struct {
	RunID   string `json:"run_id"`
	NodeID  string `json:"node_id"`
	Context string `json:"context"`
}

type finalEvidenceArtifact struct {
	Ref       string  `json:"ref"`
	ChunkID   *string `json:"chunk_id,omitempty"`
	SpanStart *int    `json:"span_start,omitempty"`
	SpanEnd   *int    `json:"span_end,omitempty"`
}

type finalAnswerArtifact struct {
	RunID       string                  `json:"run_id"`
	NodeID      string                  `json:"node_id"`
	FinalAnswer string                  `json:"final_answer"`
	Evidence    []finalEvidenceArtifact `json:"evidence"`
	Confidence  *string                 `json:"confidence,omitempty"`
}

// PersistUserTurn stores a compact user turn artifact and returns content_ref.
func (s *TurnOutputStore) PersistUserTurn(runID string, nodeID string, stepID string, envelope StepInputEnvelope, messages []inference.Message) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}

	ref := fmt.Sprintf("run-output://node/%s/step/%s/turn-user.json", nodeID, stepID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "step", stepID, "turn-user.json")
	messageArtifacts := make([]userTurnMessageArtifact, 0, len(messages))
	for _, message := range messages {
		messageArtifacts = append(messageArtifacts, userTurnMessageArtifact{
			Role:         string(message.Role),
			ContentBytes: len(message.Content),
		})
	}

	payload := userTurnArtifact{
		RunID:              runID,
		NodeID:             nodeID,
		StepID:             stepID,
		ModelInputEnvelope: envelope,
		ModelInputMessages: messageArtifacts,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}

	return ref, nil
}

// PersistContext stores node context output and returns content_ref.
func (s *TurnOutputStore) PersistContext(runID string, nodeID string, contextBody string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}

	ref := fmt.Sprintf("run-output://node/%s/context.json", nodeID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "context.json")
	payload := contextArtifact{
		RunID:   runID,
		NodeID:  nodeID,
		Context: contextBody,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}

	return ref, nil
}

// PersistModelTurn stores a model turn artifact and returns content_ref.
func (s *TurnOutputStore) PersistModelTurn(runID string, nodeID string, stepID string, result inference.Result) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}

	ref := fmt.Sprintf("run-output://node/%s/step/%s/turn-model.json", nodeID, stepID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "step", stepID, "turn-model.json")
	payload := modelTurnArtifact{
		RunID:         runID,
		NodeID:        nodeID,
		StepID:        stepID,
		SchemaID:      result.SchemaID,
		Validated:     result.ValidatedPayload,
		Gateway:       result.Gateway,
		Provider:      result.Provider,
		Model:         result.Model,
		GatewayRespID: result.GatewayResponseID,
		FinishStatus:  result.FinishStatus,
		Reasoning:     result.Reasoning,
		Usage:         result.Usage,
		Accounting:    result.Accounting,
		RawMetadata:   result.RawMetadata,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}

	return ref, nil
}

// PersistFinalAnswer stores final-answer output and returns final_answer_ref/output_ref.
func (s *TurnOutputStore) PersistFinalAnswer(runID string, nodeID string, answer string, evidence []FinalEvidence, confidence *string) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}

	ref := fmt.Sprintf("run-output://node/%s/final-answer.json", nodeID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "final-answer.json")
	evidenceArtifacts := make([]finalEvidenceArtifact, 0, len(evidence))
	for _, item := range evidence {
		evidenceArtifacts = append(evidenceArtifacts, finalEvidenceArtifact{
			Ref:       item.Ref,
			ChunkID:   cloneOptional(item.ChunkID),
			SpanStart: cloneOptionalInt(item.SpanStart),
			SpanEnd:   cloneOptionalInt(item.SpanEnd),
		})
	}
	payload := finalAnswerArtifact{
		RunID:       runID,
		NodeID:      nodeID,
		FinalAnswer: answer,
		Evidence:    evidenceArtifacts,
		Confidence:  cloneOptional(confidence),
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}

	return ref, nil
}

func writeOutputArtifact(path string, payload any) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("failed to create output artifact directory %q; %w", directory, err)
	}

	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode output artifact; %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("failed to write output artifact %q; %w", path, err)
	}

	return nil
}

func cloneOptionalInt(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
