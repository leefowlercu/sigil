package harness

import (
	"fmt"
	"path/filepath"

	"github.com/leefowlercu/sigil/internal/accounting"
)

type subcallAccountingArtifact struct {
	RunID        string             `json:"run_id"`
	NodeID       string             `json:"node_id"`
	StepID       string             `json:"step_id"`
	SubcallIndex int                `json:"subcall_index"`
	Accounting   accounting.Summary `json:"accounting"`
}

type stepAccountingArtifact struct {
	RunID      string            `json:"run_id"`
	NodeID     string            `json:"node_id"`
	StepID     string            `json:"step_id"`
	Accounting accounting.Rollup `json:"accounting"`
}

type nodeAccountingArtifact struct {
	RunID      string            `json:"run_id"`
	NodeID     string            `json:"node_id"`
	Accounting accounting.Rollup `json:"accounting"`
}

type runAccountingArtifact struct {
	RunID      string            `json:"run_id"`
	Accounting accounting.Rollup `json:"accounting"`
}

func (s *TurnOutputStore) PersistSubcallAccounting(runID string, nodeID string, stepID string, subcallIndex int, summary accounting.Summary) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}
	ref := fmt.Sprintf("run-output://node/%s/step/%s/subcall-%d-accounting.json", nodeID, stepID, subcallIndex)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "step", stepID, fmt.Sprintf("subcall-%d-accounting.json", subcallIndex))
	payload := subcallAccountingArtifact{
		RunID:        runID,
		NodeID:       nodeID,
		StepID:       stepID,
		SubcallIndex: subcallIndex,
		Accounting:   summary,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *TurnOutputStore) PersistStepAccounting(runID string, nodeID string, stepID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}
	ref := fmt.Sprintf("run-output://node/%s/step/%s/accounting.json", nodeID, stepID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "step", stepID, "accounting.json")
	payload := stepAccountingArtifact{
		RunID:      runID,
		NodeID:     nodeID,
		StepID:     stepID,
		Accounting: rollup,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *TurnOutputStore) PersistNodeAccounting(runID string, nodeID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}
	ref := fmt.Sprintf("run-output://node/%s/accounting.json", nodeID)
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "node", nodeID, "accounting.json")
	payload := nodeAccountingArtifact{
		RunID:      runID,
		NodeID:     nodeID,
		Accounting: rollup,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *TurnOutputStore) PersistRunAccounting(runID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("turn output store is required")
	}
	ref := "run-output://run/accounting.json"
	path := filepath.Join(s.runsBaseDir, runID, "outputs", "run", "accounting.json")
	payload := runAccountingArtifact{
		RunID:      runID,
		Accounting: rollup,
	}
	if err := writeOutputArtifact(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}
