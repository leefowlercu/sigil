package clioutput

import (
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/harness"
	"github.com/leefowlercu/sigil/internal/runtime"
)

// StartPreflight defines the human-readable preflight summary for one run start.
type StartPreflight struct {
	ConfigPath       string
	RunConfigPath    string
	RunsBaseDir      string
	Gateway          string
	Provider         string
	Model            string
	ReasoningEnabled bool
	RLMEnabled       bool
	RLMMaxDepth      int
}

type startNodeInfo struct {
	Depth int
	Role  runtime.NodeRole
}

type completedTerminal struct {
	DurationMS    int
	Accounting    accounting.Rollup
	AccountingRef *string
}

// StartTextRenderer renders human-readable run-start progress from canonical lifecycle events.
type StartTextRenderer struct {
	mu            sync.Mutex
	writer        io.Writer
	err           error
	nodeByID      map[string]startNodeInfo
	stepIndexByID map[string]int
	completed     *completedTerminal
}

// NewStartTextRenderer builds a renderer for text-mode run start output.
func NewStartTextRenderer(writer io.Writer) *StartTextRenderer {
	return &StartTextRenderer{
		writer:        writer,
		nodeByID:      make(map[string]startNodeInfo),
		stepIndexByID: make(map[string]int),
	}
}

// WritePreflight writes the text-mode preflight header.
func (r *StartTextRenderer) WritePreflight(preflight StartPreflight) {
	if r == nil {
		return
	}

	profile := "non-recursive"
	if preflight.RLMEnabled {
		profile = "recursive"
	}

	reasoning := "disabled"
	if preflight.ReasoningEnabled {
		reasoning = "enabled"
	}

	r.writef("Run start\n")
	r.writef("  Config path: %s\n", preflight.ConfigPath)
	r.writef("  Run config path: %s\n", preflight.RunConfigPath)
	r.writef("  Runs dir: %s\n", preflight.RunsBaseDir)
	r.writef("  Gateway: %s\n", preflight.Gateway)
	r.writef("  Provider: %s\n", preflight.Provider)
	r.writef("  Model: %s\n", preflight.Model)
	r.writef("  Reasoning: %s\n", reasoning)
	r.writef("  Profile: %s\n", profile)
	r.writef("  RLM max depth: %d\n", preflight.RLMMaxDepth)
}

// ObserveEvent renders one canonical lifecycle event.
func (r *StartTextRenderer) ObserveEvent(event runtime.EventEnvelope) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	switch payload := event.Payload.(type) {
	case runtime.RunQueuedPayload:
		r.writefLocked("Run queued: run_id=%s\n", event.RunID)
	case runtime.RunRunningPayload:
		r.writefLocked("Run running: run_id=%s executor=%s max_depth=%d\n", event.RunID, payload.Executor, payload.MaxDepth)
	case runtime.NodeStartedPayload:
		if event.NodeID == nil {
			return
		}
		r.nodeByID[*event.NodeID] = startNodeInfo{
			Depth: payload.Depth,
			Role:  payload.Role,
		}
		prefix := strings.Repeat("  ", payload.Depth)
		if payload.ParentNodeID != nil {
			r.writefLocked("%sNode started: node_id=%s depth=%d role=%s parent_node_id=%s\n", prefix, *event.NodeID, payload.Depth, payload.Role, *payload.ParentNodeID)
			return
		}
		r.writefLocked("%sNode started: node_id=%s depth=%d role=%s\n", prefix, *event.NodeID, payload.Depth, payload.Role)
	case runtime.NodeStepStartedPayload:
		if event.NodeID == nil {
			return
		}
		r.stepIndexByID[payload.StepID] = payload.StepIndex
		r.writeNodeLineLocked(*event.NodeID, "Step %d started: step_id=%s", payload.StepIndex, payload.StepID)
	case runtime.NodeSubcallExecutedPayload:
		if event.NodeID == nil {
			return
		}
		stepIndex := r.stepIndexByID[payload.StepID]
		message := fmt.Sprintf(
			"Subcall %d: step=%d type=%s mode=%s duration_ms=%d",
			payload.SubcallIndex,
			stepIndex,
			payload.SubcallType,
			payload.ExecutionMode,
			payload.DurationMS,
		)
		if payload.ChildNodeID != nil {
			message += fmt.Sprintf(" child_node_id=%s", *payload.ChildNodeID)
		}
		if payload.ErrorCode != nil {
			message += fmt.Sprintf(" error_code=%s", *payload.ErrorCode)
		}
		r.writeNodeLineLocked(*event.NodeID, "%s", message)
	case runtime.NodeActionExecutedPayload:
		if event.NodeID == nil {
			return
		}
		stepIndex := r.stepIndexByID[payload.StepID]
		if payload.ErrorCode != nil {
			r.writeNodeLineLocked(*event.NodeID, "Action %d failed: step=%d duration_ms=%d error_code=%s action_ref=%s", payload.ActionIndex, stepIndex, payload.DurationMS, *payload.ErrorCode, payload.ActionRef)
			return
		}
		r.writeNodeLineLocked(*event.NodeID, "Action %d completed: step=%d duration_ms=%d action_ref=%s", payload.ActionIndex, stepIndex, payload.DurationMS, payload.ActionRef)
	case runtime.NodeStepCompletedPayload:
		if event.NodeID == nil {
			return
		}
		stepIndex := r.stepIndexByID[payload.StepID]
		r.writeNodeLineLocked(*event.NodeID, "Step %d completed: decision=%s actions=%d duration_ms=%d", stepIndex, payload.Decision, payload.ActionCount, payload.DurationMS)
	case runtime.NodeCompletedPayload:
		if event.NodeID == nil {
			return
		}
		r.writeNodeLineLocked(*event.NodeID, "Node completed: status=%s duration_ms=%d result_ref=%s", payload.Status, payload.DurationMS, valueOrPlaceholder(payload.ResultRef))
	case runtime.NodeFailedPayload:
		if event.NodeID == nil {
			return
		}
		r.writeNodeLineLocked(*event.NodeID, "Node failed: status=%s duration_ms=%d error_code=%s", payload.Status, payload.DurationMS, payload.ErrorCode)
	case runtime.RunCompletedPayload:
		r.completed = &completedTerminal{
			DurationMS:    payload.DurationMS,
			Accounting:    payload.Accounting,
			AccountingRef: cloneOptionalString(payload.AccountingRef),
		}
		r.writefLocked("Run completed: run_id=%s duration_ms=%d\n", event.RunID, payload.DurationMS)
	case runtime.RunFailedPayload:
		r.writefLocked("Run failed: run_id=%s error_code=%s retryable=%t\n", event.RunID, payload.ErrorCode, payload.Retryable)
	case runtime.RunInterruptedPayload:
		r.writefLocked("Run interrupted: run_id=%s reason=%s\n", event.RunID, payload.Reason)
	}
}

// WriteCompletedSummary writes the detailed text-mode summary for a successful run.
func (r *StartTextRenderer) WriteCompletedSummary(result harness.RunResult) {
	if r == nil {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	duration := 0
	accountingRollup := result.Accounting
	var accountingRef *string
	if r.completed != nil {
		duration = r.completed.DurationMS
		accountingRollup = r.completed.Accounting
		accountingRef = cloneOptionalString(r.completed.AccountingRef)
	}

	r.writefLocked("Run summary\n")
	r.writefLocked("  State: %s\n", result.State)
	r.writefLocked("  Run ID: %s\n", result.RunID)
	r.writefLocked("  Duration (ms): %d\n", duration)
	r.writefLocked("  Events path: %s\n", result.EventsPath)
	r.writefLocked("  Final answer ref: %s\n", result.FinalAnswerRef)
	r.writefLocked("  Final answer:\n")
	for _, line := range strings.Split(result.FinalAnswer, "\n") {
		r.writefLocked("    %s\n", line)
	}
	r.writefLocked("  Accounting:\n")
	r.writeAccountingSummaryLocked("    Model total", accountingRollup.ModelTotal)
	r.writeAccountingSummaryLocked("    Direct subcalls total", accountingRollup.DirectSubcallsTotal)
	r.writeAccountingSummaryLocked("    Tree total", accountingRollup.TreeTotal)
	if accountingRef != nil {
		r.writefLocked("    Accounting ref: %s\n", *accountingRef)
		r.writefLocked("    Accounting path: %s\n", filepath.Join(filepath.Dir(result.EventsPath), "artifacts", "run", "accounting.json"))
	}
}

// Err returns the first write error captured by the renderer.
func (r *StartTextRenderer) Err() error {
	if r == nil {
		return nil
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	return r.err
}

func (r *StartTextRenderer) writeAccountingSummaryLocked(label string, summary accounting.Summary) {
	r.writefLocked("%s: provider=%s model=%s pricing_version=%s tokens=%s token_status=%s cost_microusd=%s cost_status=%s token_source=%s cost_source=%s\n",
		label,
		summary.PricingKey.Provider,
		summary.PricingKey.Model,
		valueOrFallback(summary.PricingVersion, "unknown"),
		valueOrUnknownInt64(summary.TotalTokens),
		summary.TokenStatus,
		valueOrUnknownInt64(summary.KnownTotalCostMicrousd),
		summary.CostStatus,
		summary.TokenSource,
		summary.CostSource,
	)
}

func (r *StartTextRenderer) writeNodeLineLocked(nodeID string, format string, args ...any) {
	info := r.nodeByID[nodeID]
	prefix := strings.Repeat("  ", info.Depth+1)
	r.writefLocked(prefix+format+"\n", args...)
}

func (r *StartTextRenderer) writef(format string, args ...any) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.writefLocked(format, args...)
}

func (r *StartTextRenderer) writefLocked(format string, args ...any) {
	if r.err != nil || r.writer == nil {
		return
	}

	if _, err := fmt.Fprintf(r.writer, format, args...); err != nil && r.err == nil {
		r.err = fmt.Errorf("failed to write text output; %w", err)
	}
}

func valueOrUnknownInt64(value *int64) string {
	if value == nil {
		return "unknown"
	}
	return fmt.Sprintf("%d", *value)
}

func valueOrPlaceholder(value *string) string {
	if value == nil || strings.TrimSpace(*value) == "" {
		return "-"
	}
	return *value
}

func valueOrFallback(value string, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

func cloneOptionalString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}
