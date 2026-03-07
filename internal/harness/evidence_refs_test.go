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
	if err := resolveFinalEvidenceRefs(runID, runsBaseDir, artifactStore, nodeID, evidence); err != nil {
		t.Fatalf("expected evidence ref resolution success, got %v", err)
	}
}

func TestResolveFinalEvidenceRefsRejectsUnresolvableRefs(t *testing.T) {
	runsBaseDir := t.TempDir()
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	artifactStore, err := NewActionArtifactStore(runsBaseDir)
	if err != nil {
		t.Fatalf("expected action artifact store creation success, got %v", err)
	}

	evidence := []FinalEvidence{
		{Ref: "run-output://node/missing/context.json"},
	}
	err = resolveFinalEvidenceRefs(runID, runsBaseDir, artifactStore, nodeID, evidence)
	if err == nil {
		t.Fatal("expected unresolved evidence ref error")
	}
	if !strings.Contains(err.Error(), "final.evidence[0]") {
		t.Fatalf("expected indexed evidence failure in error, got %v", err)
	}
}

func TestResolveFinalEvidenceRefsNormalizesMalformedPreviousActionRefs(t *testing.T) {
	runsBaseDir := t.TempDir()
	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	artifactStore, err := NewActionArtifactStore(runsBaseDir)
	if err != nil {
		t.Fatalf("expected action artifact store creation success, got %v", err)
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

	stepParts := strings.Split(stepID, "-")
	nodeParts := strings.Split(nodeID, "-")
	if len(stepParts) != 5 || len(nodeParts) != 5 {
		t.Fatalf("expected UUIDv7 identifiers, got node=%q step=%q", nodeID, stepID)
	}
	malformedRef := "run-artifact://node/" + nodeParts[0] + "-" + nodeParts[1] + "-" +
		stepParts[2] + "-" + stepParts[3] + "-" + stepParts[4] + "/action-1.json"

	evidence := []FinalEvidence{{Ref: malformedRef}}
	if err := resolveFinalEvidenceRefs(runID, runsBaseDir, artifactStore, nodeID, evidence); err != nil {
		t.Fatalf("expected malformed previous-action ref normalization success, got %v", err)
	}
	if evidence[0].Ref != actionRef {
		t.Fatalf("expected normalized ref %q, got %q", actionRef, evidence[0].Ref)
	}
}
