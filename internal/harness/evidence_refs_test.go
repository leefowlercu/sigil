package harness

import (
	"strings"
	"testing"
)

func TestResolveFinalEvidenceRefsAcceptsRunOutputAndRunArtifactRefs(t *testing.T) {
	runsBaseDir := t.TempDir()
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	turnOutputs, err := NewTurnOutputStore(runsBaseDir)
	if err != nil {
		t.Fatalf("expected turn output store creation success, got %v", err)
	}
	artifactStore, err := NewActionArtifactStore(runsBaseDir)
	if err != nil {
		t.Fatalf("expected action artifact store creation success, got %v", err)
	}

	contextRef, err := turnOutputs.PersistContext(runID, nodeID, "context")
	if err != nil {
		t.Fatalf("expected context persistence success, got %v", err)
	}
	actionRef, err := artifactStore.Persist(ActionArtifact{
		RunID:       runID,
		NodeID:      nodeID,
		StepID:      stepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      "completed",
		Code:        `fmt.Print("ok")`,
		DurationMS:  1,
	})
	if err != nil {
		t.Fatalf("expected action artifact persistence success, got %v", err)
	}

	evidence := []FinalEvidence{
		{Ref: contextRef},
		{Ref: actionRef},
	}
	if err := resolveFinalEvidenceRefs(runID, runsBaseDir, artifactStore, evidence); err != nil {
		t.Fatalf("expected evidence ref resolution success, got %v", err)
	}
}

func TestResolveFinalEvidenceRefsRejectsUnresolvableRefs(t *testing.T) {
	runsBaseDir := t.TempDir()
	runID := mustUUIDv7String(t)
	artifactStore, err := NewActionArtifactStore(runsBaseDir)
	if err != nil {
		t.Fatalf("expected action artifact store creation success, got %v", err)
	}

	evidence := []FinalEvidence{
		{Ref: "run-output://node/missing/context.json"},
	}
	err = resolveFinalEvidenceRefs(runID, runsBaseDir, artifactStore, evidence)
	if err == nil {
		t.Fatal("expected unresolved evidence ref error")
	}
	if !strings.Contains(err.Error(), "final.evidence[0]") {
		t.Fatalf("expected indexed evidence failure in error, got %v", err)
	}
}
