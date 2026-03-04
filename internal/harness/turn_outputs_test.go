package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leefowlercu/sigil/internal/inference"
)

func TestTurnOutputStorePersistsUserAndModelTurns(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewTurnOutputStore(baseDir)
	if err != nil {
		t.Fatalf("expected turn output store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)

	envelope := StepInputEnvelope{
		Query:     "prompt",
		StepIndex: 1,
		ContextMetadata: StepContextMetadata{
			ContextType:      "string",
			ContextBytes:     123,
			ContextLineCount: 4,
			ContextSHA256:    "abc123",
			ContextRef:       "run-output://node/" + nodeID + "/context.json",
		},
	}
	messages := []inference.Message{
		{Role: inference.MessageRoleSystem, Content: "system"},
		{Role: inference.MessageRoleUser, Content: "{\"query\":\"prompt\"}"},
	}
	userRef, err := store.PersistUserTurn(runID, nodeID, stepID, envelope, messages)
	if err != nil {
		t.Fatalf("expected user turn persist success, got %v", err)
	}
	if userRef == "" {
		t.Fatal("expected non-empty user turn ref")
	}

	modelRef, err := store.PersistModelTurn(runID, nodeID, stepID, inference.Result{
		SchemaID:          "sigil.rlm.response.v1",
		ValidatedPayload:  map[string]any{"decision": "final", "final": map[string]any{"answer": "done"}},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_1",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{"k": "v"},
	})
	if err != nil {
		t.Fatalf("expected model turn persist success, got %v", err)
	}
	if modelRef == "" {
		t.Fatal("expected non-empty model turn ref")
	}

	userPath := filepath.Join(baseDir, runID, "outputs", "node", nodeID, "step", stepID, "turn-user.json")
	if _, err := os.Stat(userPath); err != nil {
		t.Fatalf("expected persisted user turn artifact %q, got %v", userPath, err)
	}
	userBytes, err := os.ReadFile(userPath)
	if err != nil {
		t.Fatalf("expected user turn artifact read success, got %v", err)
	}
	var userArtifact map[string]any
	if err := json.Unmarshal(userBytes, &userArtifact); err != nil {
		t.Fatalf("expected user turn artifact decode success, got %v", err)
	}
	if _, hasContext := userArtifact["context"]; hasContext {
		t.Fatalf("expected compact user-turn artifact to exclude raw context body")
	}
	envelopeValue, ok := userArtifact["model_input_envelope"].(map[string]any)
	if !ok {
		t.Fatalf("expected model_input_envelope object, got %T", userArtifact["model_input_envelope"])
	}
	if gotQuery, _ := envelopeValue["query"].(string); gotQuery != "prompt" {
		t.Fatalf("expected model_input_envelope.query=%q, got %q", "prompt", gotQuery)
	}
	msgValues, ok := userArtifact["model_input_messages"].([]any)
	if !ok || len(msgValues) != 2 {
		t.Fatalf("expected model_input_messages length 2, got %T len=%d", userArtifact["model_input_messages"], len(msgValues))
	}

	modelPath := filepath.Join(baseDir, runID, "outputs", "node", nodeID, "step", stepID, "turn-model.json")
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("expected persisted model turn artifact %q, got %v", modelPath, err)
	}
}

func TestTurnOutputStorePersistsFinalAnswer(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewTurnOutputStore(baseDir)
	if err != nil {
		t.Fatalf("expected turn output store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	evidence := []FinalEvidence{{Ref: "run-output://node/" + nodeID + "/context.json"}}
	confidence := "medium"

	ref, err := store.PersistFinalAnswer(runID, nodeID, "done", evidence, &confidence)
	if err != nil {
		t.Fatalf("expected final-answer persist success, got %v", err)
	}
	if ref == "" {
		t.Fatal("expected non-empty final-answer ref")
	}

	path := filepath.Join(baseDir, runID, "outputs", "node", nodeID, "final-answer.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted final-answer artifact %q, got %v", path, err)
	}
}

func TestTurnOutputStorePersistsContext(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewTurnOutputStore(baseDir)
	if err != nil {
		t.Fatalf("expected turn output store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	ref, err := store.PersistContext(runID, nodeID, "context-body")
	if err != nil {
		t.Fatalf("expected context persist success, got %v", err)
	}
	if ref != "run-output://node/"+nodeID+"/context.json" {
		t.Fatalf("expected context ref for node %q, got %q", nodeID, ref)
	}

	path := filepath.Join(baseDir, runID, "outputs", "node", nodeID, "context.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted context artifact %q, got %v", path, err)
	}
}
