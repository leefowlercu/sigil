package harness

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type modelTurnPersistFailureInference struct {
	runsBaseDir string
	result      inference.Result
}

func (m *modelTurnPersistFailureInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	match, err := findSingleStepArtifact(m.runsBaseDir, "turn-user.json")
	if err != nil {
		return inference.Result{}, err
	}
	modelTurnPath := filepath.Join(m.runsBaseDir, match.runID, "artifacts", "node", match.nodeID, "step", match.stepID, "turn-model.json")
	if err := os.MkdirAll(modelTurnPath, 0o755); err != nil {
		return inference.Result{}, err
	}
	return hydrateFinalEvidenceRef(m.result, request), nil
}

type stepArtifactMatch struct {
	runID  string
	nodeID string
	stepID string
}

func findSingleStepArtifact(runsBaseDir string, artifactName string) (stepArtifactMatch, error) {
	pattern := filepath.Join(runsBaseDir, "*", "artifacts", "node", "*", "step", "*", artifactName)
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return stepArtifactMatch{}, err
	}
	if len(matches) != 1 {
		return stepArtifactMatch{}, fmt.Errorf("expected one %s artifact, got %d", artifactName, len(matches))
	}

	relative, err := filepath.Rel(runsBaseDir, matches[0])
	if err != nil {
		return stepArtifactMatch{}, err
	}
	parts := strings.Split(filepath.ToSlash(relative), "/")
	if len(parts) != 7 {
		return stepArtifactMatch{}, fmt.Errorf("unexpected artifact path %q", relative)
	}

	return stepArtifactMatch{
		runID:  parts[0],
		nodeID: parts[3],
		stepID: parts[5],
	}, nil
}

func TestRunnerRunPreservesModelTurnAccountingWhenPersistModelTurnFails(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	inputTokens := int64(12)
	outputTokens := int64(5)
	totalTokens := int64(17)
	totalCost := int64(1750)

	inferenceClient := &modelTurnPersistFailureInference{
		runsBaseDir: baseDir,
		result: inference.Result{
			SchemaID:          schema.SigilRLMResponseV1SchemaID,
			ValidatedPayload:  map[string]any{"decision": "final", "final": map[string]any{"answer": "done", "evidence": []any{map[string]any{"ref": "__context_ref__"}}}},
			Gateway:           "openrouter",
			Provider:          "openai",
			Model:             "gpt-5.1",
			GatewayResponseID: "resp_final",
			FinishStatus:      "completed",
			RawMetadata:       map[string]any{},
			Accounting: accounting.BuildLeafSummary(accounting.LeafInput{
				Provider:                 "openai",
				Model:                    "gpt-5.1",
				PricingVersion:           "v1",
				InputTokens:              &inputTokens,
				OutputTokens:             &outputTokens,
				TotalTokens:              &totalTokens,
				GatewayTotalCostMicrousd: &totalCost,
			}),
		},
	}

	runner := NewRunner(
		WithRunsBaseDir(baseDir),
		WithInferenceFactory(func(_ config.RunConfig) (InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	_, err := runner.Run(context.Background(), RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     testRunConfig("root prompt", "", "root context", ""),
	})
	if err == nil {
		t.Fatal("expected runner failure when model-turn path is sabotaged")
	}

	payload := mustReadRunFailedPayload(t, baseDir)
	if payload.Accounting.TreeTotal.TotalTokens == nil || *payload.Accounting.TreeTotal.TotalTokens != totalTokens {
		t.Fatalf("expected run.failed accounting total_tokens=%d, got %+v", totalTokens, payload.Accounting.TreeTotal.TotalTokens)
	}
	if payload.Accounting.TreeTotal.KnownTotalCostMicrousd == nil || *payload.Accounting.TreeTotal.KnownTotalCostMicrousd != totalCost {
		t.Fatalf("expected run.failed accounting known_total_cost_microusd=%d, got %+v", totalCost, payload.Accounting.TreeTotal.KnownTotalCostMicrousd)
	}
}

func TestSubcallRouterPreservesLedgerAccountingWhenAppendFails(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir: baseDir,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})
	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected lifecycle start success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node, got %v", err)
	}
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected step start success, got %v", err)
	}

	runArtifacts, err := NewRunArtifactStore(baseDir)
	if err != nil {
		t.Fatalf("expected run artifact store creation success, got %v", err)
	}
	ledger := accounting.NewLedger("v1")
	ledger.SetRootNodeID(rootNode.ID)

	runConfig := testRunConfig("root prompt", "", "root context", "")
	runConfig.LLM.Provider = ""

	router, err := NewSubcallRouter(SubcallRouterInput{
		Lifecycle:    lifecycle,
		Inference:    &queuedInference{},
		RunConfig:    runConfig,
		Node:         rootNode,
		StepID:       stepStarted.StepID,
		ActionIndex:  1,
		NonRecursive: true,
		RunArtifacts: runArtifacts,
		Ledger:       ledger,
		ExecuteChild: func(context.Context, runtime.Node, string, string) (nodeExecutionResult, error) {
			return nodeExecutionResult{}, nil
		},
	})
	if err != nil {
		t.Fatalf("expected subcall router creation success, got %v", err)
	}

	subcallTokens := int64(7)
	subcallTotal := int64(7)
	subcallCost := int64(700)
	record := subcallRecord{
		answer:     "done",
		durationMS: 1,
		mode:       runtime.SubcallExecutionModePlain,
		accounting: accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:                 "openai",
			Model:                    "gpt-5.1",
			PricingVersion:           "v1",
			InputTokens:              &subcallTokens,
			TotalTokens:              &subcallTotal,
			GatewayTotalCostMicrousd: &subcallCost,
		}),
	}

	err = router.persistRecord(runtime.SubcallTypeLLMQuery, repl.QueryRequest{Prompt: "subcall prompt", Context: "subcall context"}, record)
	if err == nil {
		t.Fatal("expected node.subcall.executed append failure")
	}

	rollup := ledger.StepRollup(rootNode.ID, stepStarted.StepID)
	if rollup.DirectSubcallsTotal.TotalTokens == nil || *rollup.DirectSubcallsTotal.TotalTokens != subcallTotal {
		t.Fatalf("expected direct_subcalls_total.total_tokens=%d, got %+v", subcallTotal, rollup.DirectSubcallsTotal.TotalTokens)
	}
	if rollup.DirectSubcallsTotal.KnownTotalCostMicrousd == nil || *rollup.DirectSubcallsTotal.KnownTotalCostMicrousd != subcallCost {
		t.Fatalf("expected direct_subcalls_total.known_total_cost_microusd=%d, got %+v", subcallCost, rollup.DirectSubcallsTotal.KnownTotalCostMicrousd)
	}
}
