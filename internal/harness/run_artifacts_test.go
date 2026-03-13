package harness

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/inference"
)

func TestRunArtifactStorePersistsUserAndModelTurns(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewRunArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected run artifact store creation success, got %v", err)
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
			ContextRef:       "run-artifact://node/" + nodeID + "/context.json",
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

	userPath := filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "step", stepID, "turn-user.json")
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

	modelPath := filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "step", stepID, "turn-model.json")
	if _, err := os.Stat(modelPath); err != nil {
		t.Fatalf("expected persisted model turn artifact %q, got %v", modelPath, err)
	}
}

func TestRunArtifactStorePersistsFinalAnswer(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewRunArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected run artifact store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	evidence := []FinalEvidence{{Ref: "run-artifact://node/" + nodeID + "/context.json"}}
	confidence := "medium"

	ref, err := store.PersistFinalAnswer(runID, nodeID, "done", evidence, &confidence)
	if err != nil {
		t.Fatalf("expected final-answer persist success, got %v", err)
	}
	if ref == "" {
		t.Fatal("expected non-empty final-answer ref")
	}

	path := filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "final-answer.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted final-answer artifact %q, got %v", path, err)
	}
}

func TestRunArtifactStorePersistsContext(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewRunArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected run artifact store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)

	ref, err := store.PersistContext(runID, nodeID, "context-body")
	if err != nil {
		t.Fatalf("expected context persist success, got %v", err)
	}
	if ref != "run-artifact://node/"+nodeID+"/context.json" {
		t.Fatalf("expected context ref for node %q, got %q", nodeID, ref)
	}

	path := filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "context.json")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted context artifact %q, got %v", path, err)
	}
}

func TestRunArtifactStorePersistsAccountingArtifacts(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	store, err := NewRunArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected run artifact store creation success, got %v", err)
	}

	runID := mustUUIDv7String(t)
	nodeID := mustUUIDv7String(t)
	stepID := mustUUIDv7String(t)
	inputTokens := int64(10)
	outputTokens := int64(5)
	totalTokens := int64(15)
	totalCost := int64(250)

	summary := accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:                 "openai",
		Model:                    "gpt-5.1",
		PricingVersion:           "v1",
		InputTokens:              &inputTokens,
		OutputTokens:             &outputTokens,
		TotalTokens:              &totalTokens,
		GatewayTotalCostMicrousd: &totalCost,
	})
	rollup := accounting.BuildRollup("openai", "gpt-5.1", "v1", summary, accounting.ZeroSummary("openai", "gpt-5.1", "v1"))

	subcallRef, err := store.PersistSubcallAccounting(runID, nodeID, stepID, 1, summary)
	if err != nil {
		t.Fatalf("expected subcall accounting persist success, got %v", err)
	}
	if subcallRef != "run-artifact://node/"+nodeID+"/step/"+stepID+"/subcall-1-accounting.json" {
		t.Fatalf("unexpected subcall accounting ref %q", subcallRef)
	}

	stepRef, err := store.PersistStepAccounting(runID, nodeID, stepID, rollup)
	if err != nil {
		t.Fatalf("expected step accounting persist success, got %v", err)
	}
	if stepRef != "run-artifact://node/"+nodeID+"/step/"+stepID+"/accounting.json" {
		t.Fatalf("unexpected step accounting ref %q", stepRef)
	}

	nodeRef, err := store.PersistNodeAccounting(runID, nodeID, rollup)
	if err != nil {
		t.Fatalf("expected node accounting persist success, got %v", err)
	}
	if nodeRef != "run-artifact://node/"+nodeID+"/accounting.json" {
		t.Fatalf("unexpected node accounting ref %q", nodeRef)
	}

	runRef, err := store.PersistRunAccounting(runID, rollup)
	if err != nil {
		t.Fatalf("expected run accounting persist success, got %v", err)
	}
	if runRef != "run-artifact://run/accounting.json" {
		t.Fatalf("unexpected run accounting ref %q", runRef)
	}

	paths := []string{
		filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "step", stepID, "subcall-1-accounting.json"),
		filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "step", stepID, "accounting.json"),
		filepath.Join(baseDir, runID, "artifacts", "node", nodeID, "accounting.json"),
		filepath.Join(baseDir, runID, "artifacts", "run", "accounting.json"),
	}
	for _, path := range paths {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("expected persisted accounting artifact %q, got %v", path, err)
		}
	}
}
