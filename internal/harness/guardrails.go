package harness

import (
	"fmt"
	"strconv"
	"time"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	limitKeyMaxStepsPerNode          = "max_steps_per_node"
	limitKeyMaxTotalStepsPerRun      = "max_total_steps_per_run"
	limitKeyMaxRunDurationMS         = "max_run_duration_ms"
	limitKeyMaxConsecutiveStepErrors = "max_consecutive_step_failures"
)

type deterministicGuardrails struct {
	cfg                    config.RunGuardrailsConfig
	startTime              time.Time
	deadline               time.Time
	stepsByNode            map[string]int
	totalSteps             int
	consecutiveStepFailure int
}

func newDeterministicGuardrails(cfg config.RunGuardrailsConfig, startTime time.Time) *deterministicGuardrails {
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}
	return &deterministicGuardrails{
		cfg:         cfg,
		startTime:   startTime,
		deadline:    startTime.Add(time.Duration(cfg.MaxRunDurationMS) * time.Millisecond),
		stepsByNode: make(map[string]int),
	}
}

func (g *deterministicGuardrails) CheckBeforeStep(nodeID string, now time.Time) error {
	if err := g.checkRunDuration(now, nodeID, ""); err != nil {
		return err
	}

	nodeSteps := g.stepsByNode[nodeID]
	if nodeSteps >= g.cfg.MaxStepsPerNode {
		return g.limitError(limitKeyMaxStepsPerNode, nodeID, "", g.cfg.MaxStepsPerNode, nodeSteps)
	}
	if g.totalSteps >= g.cfg.MaxTotalStepsPerRun {
		return g.limitError(limitKeyMaxTotalStepsPerRun, nodeID, "", g.cfg.MaxTotalStepsPerRun, g.totalSteps)
	}

	return nil
}

func (g *deterministicGuardrails) RecordStepStarted(nodeID string) {
	g.stepsByNode[nodeID] = g.stepsByNode[nodeID] + 1
	g.totalSteps++
}

func (g *deterministicGuardrails) RecordContinueAction(nodeID string, stepID string, status runtime.ActionExecutionStatus) error {
	if status == runtime.ActionExecutionStatusFailed {
		g.consecutiveStepFailure++
		if g.consecutiveStepFailure >= g.cfg.MaxConsecutiveStepFailures {
			return g.limitError(
				limitKeyMaxConsecutiveStepErrors,
				nodeID,
				stepID,
				g.cfg.MaxConsecutiveStepFailures,
				g.consecutiveStepFailure,
			)
		}
		return nil
	}

	g.ResetConsecutiveFailures()
	return nil
}

func (g *deterministicGuardrails) RecordFinalDecision() {
	g.ResetConsecutiveFailures()
}

func (g *deterministicGuardrails) CheckRunDuration(nodeID string, stepID string, now time.Time) error {
	return g.checkRunDuration(now, nodeID, stepID)
}

func (g *deterministicGuardrails) ResetConsecutiveFailures() {
	g.consecutiveStepFailure = 0
}

func (g *deterministicGuardrails) Deadline() time.Time {
	return g.deadline
}

func (g *deterministicGuardrails) checkRunDuration(now time.Time, nodeID string, stepID string) error {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	elapsedMS := int(now.Sub(g.startTime).Milliseconds())
	if elapsedMS < 0 {
		elapsedMS = 0
	}
	if !now.Before(g.deadline) {
		return g.limitError(limitKeyMaxRunDurationMS, nodeID, stepID, g.cfg.MaxRunDurationMS, elapsedMS)
	}
	return nil
}

func (g *deterministicGuardrails) limitError(limitKey string, nodeID string, stepID string, configured int, observed int) error {
	message := fmt.Sprintf("deterministic runtime guardrail breached; %s configured=%d observed=%d", limitKey, configured, observed)
	return NewLimitError(message, LimitMetadata{
		LimitKey:        limitKey,
		ConfiguredValue: strconv.Itoa(configured),
		ObservedValue:   strconv.Itoa(observed),
		NodeID:          nodeID,
		StepID:          stepID,
	})
}
