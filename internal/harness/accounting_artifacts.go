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

func (s *RunArtifactStore) PersistSubcallAccounting(runID string, nodeID string, stepID string, subcallIndex int, summary accounting.Summary) (string, error) {
	if s == nil {
		return "", fmt.Errorf("run artifact store is required")
	}
	ref := fmt.Sprintf("run-artifact://node/%s/step/%s/subcall-%d-accounting.json", nodeID, stepID, subcallIndex)
	path := filepath.Join(s.runsBaseDir, runID, "artifacts", "node", nodeID, "step", stepID, fmt.Sprintf("subcall-%d-accounting.json", subcallIndex))
	payload := subcallAccountingArtifact{
		RunID:        runID,
		NodeID:       nodeID,
		StepID:       stepID,
		SubcallIndex: subcallIndex,
		Accounting:   summary,
	}
	if err := writeArtifactFile(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *RunArtifactStore) PersistStepAccounting(runID string, nodeID string, stepID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("run artifact store is required")
	}
	ref := fmt.Sprintf("run-artifact://node/%s/step/%s/accounting.json", nodeID, stepID)
	path := filepath.Join(s.runsBaseDir, runID, "artifacts", "node", nodeID, "step", stepID, "accounting.json")
	payload := stepAccountingArtifact{
		RunID:      runID,
		NodeID:     nodeID,
		StepID:     stepID,
		Accounting: rollup,
	}
	if err := writeArtifactFile(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *RunArtifactStore) PersistNodeAccounting(runID string, nodeID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("run artifact store is required")
	}
	ref := fmt.Sprintf("run-artifact://node/%s/accounting.json", nodeID)
	path := filepath.Join(s.runsBaseDir, runID, "artifacts", "node", nodeID, "accounting.json")
	payload := nodeAccountingArtifact{
		RunID:      runID,
		NodeID:     nodeID,
		Accounting: rollup,
	}
	if err := writeArtifactFile(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}

func (s *RunArtifactStore) PersistRunAccounting(runID string, rollup accounting.Rollup) (string, error) {
	if s == nil {
		return "", fmt.Errorf("run artifact store is required")
	}
	ref := "run-artifact://run/accounting.json"
	path := filepath.Join(s.runsBaseDir, runID, "artifacts", "run", "accounting.json")
	payload := runAccountingArtifact{
		RunID:      runID,
		Accounting: rollup,
	}
	if err := writeArtifactFile(path, payload); err != nil {
		return "", err
	}
	return ref, nil
}
