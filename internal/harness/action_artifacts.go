package harness

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/runtime"
)

// ActionArtifact captures persisted execution records for node action execution.
type ActionArtifact struct {
	RunID        string               `json:"run_id"`
	NodeID       string               `json:"node_id"`
	StepID       string               `json:"step_id"`
	ActionIndex  int                  `json:"action_index"`
	ActionType   string               `json:"action_type"`
	Language     string               `json:"language"`
	Status       string               `json:"status"`
	Code         string               `json:"code"`
	Stdout       string               `json:"stdout"`
	Stderr       string               `json:"stderr"`
	DurationMS   int                  `json:"duration_ms"`
	ErrorCode    *string              `json:"error_code,omitempty"`
	ErrorMessage *string              `json:"error_message,omitempty"`
	ErrorDetail  *ActionErrorDetail   `json:"error_detail,omitempty"`
	Subcalls     []ActionSubcallTrace `json:"subcalls,omitempty"`
}

// ActionErrorDetail captures structured stage-specific diagnostics.
type ActionErrorDetail struct {
	Stage      string  `json:"stage"`
	Message    string  `json:"message"`
	Line       *int    `json:"line,omitempty"`
	Column     *int    `json:"column,omitempty"`
	Symbol     *string `json:"symbol,omitempty"`
	SourceLine *string `json:"source_line,omitempty"`
}

// ActionSubcallTrace captures one subcall execution observed within a continue action.
type ActionSubcallTrace struct {
	SubcallIndex  int                 `json:"subcall_index"`
	SubcallType   string              `json:"subcall_type"`
	ExecutionMode string              `json:"execution_mode"`
	Status        string              `json:"status"`
	Provider      string              `json:"provider"`
	Model         string              `json:"model"`
	PromptBytes   int                 `json:"prompt_bytes"`
	ContextBytes  int                 `json:"context_bytes"`
	AnswerBytes   int                 `json:"answer_bytes"`
	DurationMS    int                 `json:"duration_ms"`
	ChildNodeID   *string             `json:"child_node_id,omitempty"`
	ErrorCode     *string             `json:"error_code,omitempty"`
	ErrorMessage  *string             `json:"error_message,omitempty"`
	Accounting    *accounting.Summary `json:"accounting,omitempty"`
}

// ActionArtifactStore persists node action artifacts under run-local storage.
type ActionArtifactStore struct {
	runsBaseDir string
}

// NewActionArtifactStore constructs artifact store for a runs base directory.
func NewActionArtifactStore(runsBaseDir string) (*ActionArtifactStore, error) {
	clean := strings.TrimSpace(runsBaseDir)
	if clean == "" {
		return nil, fmt.Errorf("runs base directory is required")
	}

	return &ActionArtifactStore{runsBaseDir: clean}, nil
}

// Persist writes an action artifact and returns its canonical action_ref.
func (s *ActionArtifactStore) Persist(artifact ActionArtifact) (string, error) {
	if s == nil {
		return "", fmt.Errorf("artifact store is required")
	}

	actionRef, err := runtime.BuildActionArtifactRef(artifact.NodeID, artifact.StepID, artifact.ActionIndex)
	if err != nil {
		return "", err
	}

	artifactDir := filepath.Join(s.runsBaseDir, artifact.RunID, "artifacts", "node", artifact.NodeID, "step", artifact.StepID)
	if err := os.MkdirAll(artifactDir, 0o755); err != nil {
		return "", fmt.Errorf("failed to create artifact directory %q; %w", artifactDir, err)
	}

	artifactPath := filepath.Join(artifactDir, fmt.Sprintf("action-%d.json", artifact.ActionIndex))
	encoded, err := json.Marshal(artifact)
	if err != nil {
		return "", fmt.Errorf("failed to serialize action artifact; %w", err)
	}
	if err := os.WriteFile(artifactPath, encoded, 0o644); err != nil {
		return "", fmt.Errorf("failed to persist action artifact %q; %w", artifactPath, err)
	}

	return actionRef, nil
}

// Read loads a previously persisted action artifact from a canonical action_ref.
func (s *ActionArtifactStore) Read(runID string, actionRef string) (ActionArtifact, error) {
	if s == nil {
		return ActionArtifact{}, fmt.Errorf("artifact store is required")
	}
	if strings.TrimSpace(runID) == "" {
		return ActionArtifact{}, fmt.Errorf("run id is required")
	}

	parsed, err := runtime.ParseActionArtifactRef(actionRef)
	if err != nil {
		return ActionArtifact{}, err
	}

	path := filepath.Join(
		s.runsBaseDir,
		runID,
		"artifacts",
		"node",
		parsed.NodeID,
		"step",
		parsed.StepID,
		fmt.Sprintf("action-%d.json", parsed.ActionIndex),
	)

	encoded, err := os.ReadFile(path)
	if err != nil {
		return ActionArtifact{}, fmt.Errorf("failed to read action artifact %q; %w", path, err)
	}

	var artifact ActionArtifact
	if err := json.Unmarshal(encoded, &artifact); err != nil {
		return ActionArtifact{}, fmt.Errorf("failed to decode action artifact %q; %w", path, err)
	}

	return artifact, nil
}
