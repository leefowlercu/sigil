package clioutput

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/harness"
	"github.com/leefowlercu/sigil/internal/runtime"
)

func TestStartTextRendererRendersProgressTreeAndSummary(t *testing.T) {
	var buffer bytes.Buffer
	renderer := NewStartTextRenderer(&buffer)

	renderer.WritePreflight(StartPreflight{
		ConfigPath:       "./sigil.yaml",
		RunConfigPath:    "./sigil-run.yaml",
		RunsBaseDir:      "/tmp/custom-runs",
		Gateway:          "openrouter",
		Provider:         "openai",
		Model:            "gpt-5.1",
		ReasoningEnabled: true,
		RLMEnabled:       true,
		RLMMaxDepth:      2,
	})

	runID := "019cc706-1111-7111-8111-111111111111"
	rootNodeID := "019cc706-2222-7222-8222-222222222222"
	childNodeID := "019cc706-3333-7333-8333-333333333333"
	stepID := "019cc706-4444-7444-8444-444444444444"
	accountingRef := "run-output://run/accounting.json"
	rollup := testRollup()

	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeRunQueued,
		Timestamp: time.Now().UTC(),
		Payload: runtime.RunQueuedPayload{
			Source: runtime.RunQueuedSourceCLIRunStart,
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeRunRunning,
		Timestamp: time.Now().UTC(),
		Payload: runtime.RunRunningPayload{
			Executor: "rlm",
			MaxDepth: 2,
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeStarted,
		NodeID:    &rootNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeStartedPayload{
			Depth: 0,
			Role:  runtime.NodeRoleRoot,
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeStepStarted,
		NodeID:    &rootNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeStepStartedPayload{
			StepID:    stepID,
			StepIndex: 1,
			SchemaID:  "sigil.rlm.response.v1",
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeStarted,
		NodeID:    &childNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeStartedPayload{
			Depth:        1,
			ParentNodeID: &rootNodeID,
			Role:         runtime.NodeRoleRecursiveSubcall,
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeSubcallExecuted,
		NodeID:    &rootNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeSubcallExecutedPayload{
			StepID:        stepID,
			ActionIndex:   1,
			SubcallIndex:  1,
			SubcallType:   runtime.SubcallTypeRLMQuery,
			ExecutionMode: runtime.SubcallExecutionModeRecursive,
			Status:        runtime.ActionExecutionStatusCompleted,
			Provider:      "openai",
			Model:         "gpt-5.1",
			DurationMS:    12,
			ChildNodeID:   &childNodeID,
			Accounting:    accounting.ZeroSummary("openai", "gpt-5.1", "v1"),
			AccountingRef: "run-output://node/root/step/1/subcall-1-accounting.json",
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeActionExecuted,
		NodeID:    &rootNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeActionExecutedPayload{
			StepID:      stepID,
			ActionIndex: 1,
			Status:      runtime.ActionExecutionStatusCompleted,
			DurationMS:  15,
			OutputRef:   "run-artifact://node/root/step/1/action-1.json",
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeNodeStepCompleted,
		NodeID:    &rootNodeID,
		Timestamp: time.Now().UTC(),
		Payload: runtime.NodeStepCompletedPayload{
			StepID:      stepID,
			Decision:    runtime.StepDecisionContinue,
			ActionCount: 1,
			DurationMS:  18,
			Accounting:  rollup,
		},
	})
	renderer.ObserveEvent(runtime.EventEnvelope{
		RunID:     runID,
		Type:      runtime.EventTypeRunCompleted,
		Timestamp: time.Now().UTC(),
		Payload: runtime.RunCompletedPayload{
			Status:         "completed",
			DurationMS:     42,
			FinalAnswerRef: stringPointer("run-output://node/root/final-answer.json"),
			Accounting:     rollup,
			AccountingRef:  &accountingRef,
		},
	})

	renderer.WriteCompletedSummary(harness.RunResult{
		RunID:          runID,
		State:          "completed",
		FinalAnswer:    "done",
		FinalAnswerRef: "run-output://node/root/final-answer.json",
		EventsPath:     "/tmp/.sigil/runs/" + runID + "/events.jsonl",
		Accounting:     rollup,
	})

	if err := renderer.Err(); err != nil {
		t.Fatalf("expected no write error, got %v", err)
	}

	rendered := buffer.String()
	expectedSnippets := []string{
		"Run start",
		"Runs dir: /tmp/custom-runs",
		"Profile: recursive",
		"Run queued: run_id=" + runID,
		"Run running: run_id=" + runID,
		"Node started: node_id=" + rootNodeID,
		"  Node started: node_id=" + childNodeID,
		"  Step 1 started: step_id=" + stepID,
		"  Subcall 1: step=1 type=rlm_query mode=recursive duration_ms=12 child_node_id=" + childNodeID,
		"  Action 1 completed: step=1 duration_ms=15",
		"  Step 1 completed: decision=continue actions=1 duration_ms=18",
		"Run completed: run_id=" + runID + " duration_ms=42",
		"Run summary",
		"Duration (ms): 42",
		"Final answer:",
		"Accounting ref: " + accountingRef,
		"Accounting path: /tmp/.sigil/runs/" + runID + "/outputs/run/accounting.json",
	}
	for _, snippet := range expectedSnippets {
		if !strings.Contains(rendered, snippet) {
			t.Fatalf("expected rendered output to contain %q, got %q", snippet, rendered)
		}
	}
}

func testRollup() accounting.Rollup {
	totalTokens := int64(15)
	totalCost := int64(123)
	modelTotal := accounting.ZeroSummary("openai", "gpt-5.1", "v1")
	modelTotal.TotalTokens = &totalTokens
	modelTotal.KnownTotalCostMicrousd = &totalCost
	modelTotal.PricingKey = accounting.PricingKey{
		Provider: "openai",
		Model:    "gpt-5.1",
	}
	modelTotal.PricingVersion = "v1"
	modelTotal.TokenStatus = accounting.StatusComplete
	modelTotal.CostStatus = accounting.StatusComplete
	return accounting.BuildRollup("openai", "gpt-5.1", "v1", modelTotal, accounting.ZeroSummary("openai", "gpt-5.1", "v1"))
}

func stringPointer(value string) *string {
	return &value
}
