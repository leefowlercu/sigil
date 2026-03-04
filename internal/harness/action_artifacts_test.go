package harness

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/google/uuid"
)

func TestActionArtifactStorePersistWritesArtifactAndReturnsOutputRef(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewActionArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected artifact store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)
	outputRef, err := store.Persist(ActionArtifact{
		RunID:       runID,
		NodeID:      nodeID,
		StepID:      stepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      "completed",
		Code:        "fmt.Println(\"hello\")",
		Stdout:      "hello",
		Stderr:      "",
		DurationMS:  1,
	})
	if err != nil {
		t.Fatalf("expected artifact persist success, got %v", err)
	}
	if !strings.HasPrefix(outputRef, "run-artifact://node/") {
		t.Fatalf("expected canonical output_ref, got %q", outputRef)
	}

	artifactPath := filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "step", stepID, "action-1.json")
	if _, err := os.Stat(artifactPath); err != nil {
		t.Fatalf("expected persisted artifact %q, got %v", artifactPath, err)
	}

	loaded, err := store.Read(runID, outputRef)
	if err != nil {
		t.Fatalf("expected artifact read success, got %v", err)
	}
	if loaded.Stdout != "hello" {
		t.Fatalf("expected loaded stdout %q, got %q", "hello", loaded.Stdout)
	}
}

func mustUUIDv7String(t *testing.T) string {
	t.Helper()

	identifier, err := uuid.NewV7()
	if err != nil {
		t.Fatalf("expected UUIDv7 generation success, got %v", err)
	}
	return identifier.String()
}
