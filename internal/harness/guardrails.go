package harness

import (
	"fmt"
	"strconv"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	limitKeyMaxStepsPerNode          = "max_steps_per_node"
	limitKeyMaxTotalStepsPerRun      = "max_total_steps_per_run"
	limitKeyMaxRunDurationMS         = "max_run_duration_ms"
	limitKeyMaxConsecutiveStepErrors = "max_consecutive_step_failures"
	limitKeyMaxTotalTokens           = "max_total_tokens"
	limitKeyMaxTotalCostUSD          = "max_total_cost_usd"
)

type deterministicGuardrails struct {
	cfg                    config.RunGuardrailsConfig
	startTime              time.Time
	deadline               time.Time
	maxTotalTokens         *int64
	maxTotalCostMicrousd   *int64
	maxTotalCostUSD        *string
	stepsByNode            map[string]int
	totalSteps             int
	consecutiveStepFailure int
}

func newDeterministicGuardrails(cfg config.RunGuardrailsConfig, startTime time.Time) (*deterministicGuardrails, error) {
	if startTime.IsZero() {
		startTime = time.Now().UTC()
	}
	guardrails := &deterministicGuardrails{
		cfg:         cfg,
		startTime:   startTime,
		deadline:    startTime.Add(time.Duration(cfg.MaxRunDurationMS) * time.Millisecond),
		stepsByNode: make(map[string]int),
	}
	if cfg.MaxTotalTokens != nil {
		value := *cfg.MaxTotalTokens
		guardrails.maxTotalTokens = &value
	}
	if cfg.MaxTotalCostUSD != nil {
		canonical, microusd, err := accounting.NormalizeUSDDecimal(*cfg.MaxTotalCostUSD)
		if err != nil {
			return nil, fmt.Errorf("invalid guardrails.max_total_cost_usd; %w", err)
		}
		guardrails.maxTotalCostMicrousd = &microusd
		guardrails.maxTotalCostUSD = &canonical
	}
	return guardrails, nil
}

func (g *deterministicGuardrails) CheckBeforeStep(nodeID string, now time.Time) error {
	if err := g.checkRunDuration(now, nodeID, ""); err != nil {
		return err
	}

	nodeSteps := g.stepsByNode[nodeID]
	if nodeSteps >= g.cfg.MaxStepsPerNode {
		return g.stepStartLimitError(limitKeyMaxStepsPerNode, nodeID, g.cfg.MaxStepsPerNode, nodeSteps)
	}
	if g.totalSteps >= g.cfg.MaxTotalStepsPerRun {
		return g.stepStartLimitError(limitKeyMaxTotalStepsPerRun, nodeID, g.cfg.MaxTotalStepsPerRun, g.totalSteps)
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

func (g *deterministicGuardrails) CheckRunAccounting(nodeID string, stepID string, treeTotal accounting.Summary) error {
	if g == nil {
		return nil
	}
	if err := g.checkMaxTotalTokens(nodeID, stepID, treeTotal); err != nil {
		return err
	}
	if err := g.checkMaxTotalCostUSD(nodeID, stepID, treeTotal); err != nil {
		return err
	}
	return nil
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

func (g *deterministicGuardrails) stepStartLimitError(limitKey string, nodeID string, configured int, observed int) error {
	attempted := observed + 1
	message := fmt.Sprintf(
		"deterministic runtime guardrail breached; %s configured=%d observed=%d attempted=%d while blocking a new step start",
		limitKey,
		configured,
		observed,
		attempted,
	)
	return NewLimitError(message, LimitMetadata{
		LimitKey:        limitKey,
		ConfiguredValue: strconv.Itoa(configured),
		ObservedValue:   strconv.Itoa(observed),
		NodeID:          nodeID,
	})
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

func (g *deterministicGuardrails) checkMaxTotalTokens(nodeID string, stepID string, treeTotal accounting.Summary) error {
	if g == nil || g.maxTotalTokens == nil {
		return nil
	}

	configured := strconv.FormatInt(*g.maxTotalTokens, 10)
	observed := "unavailable"
	if treeTotal.TotalTokens != nil {
		observed = strconv.FormatInt(*treeTotal.TotalTokens, 10)
	}
	if treeTotal.TokenStatus != accounting.StatusComplete {
		return g.limitErrorWithStrings(
			limitKeyMaxTotalTokens,
			nodeID,
			stepID,
			configured,
			observed,
			fmt.Sprintf(
				"deterministic runtime guardrail breached; %s configured=%s observed=%s accounting_status=%s while requiring complete token accounting under an active budget",
				limitKeyMaxTotalTokens,
				configured,
				observed,
				treeTotal.TokenStatus,
			),
		)
	}
	if treeTotal.TotalTokens != nil && *treeTotal.TotalTokens > *g.maxTotalTokens {
		return g.limitErrorWithStrings(
			limitKeyMaxTotalTokens,
			nodeID,
			stepID,
			configured,
			observed,
			fmt.Sprintf("deterministic runtime guardrail breached; %s configured=%s observed=%s", limitKeyMaxTotalTokens, configured, observed),
		)
	}
	return nil
}

func (g *deterministicGuardrails) checkMaxTotalCostUSD(nodeID string, stepID string, treeTotal accounting.Summary) error {
	if g == nil || g.maxTotalCostMicrousd == nil || g.maxTotalCostUSD == nil {
		return nil
	}

	observed := "unavailable"
	if treeTotal.KnownTotalCostMicrousd != nil {
		observed = accounting.FormatMicrousdAsUSD(*treeTotal.KnownTotalCostMicrousd)
	}
	if treeTotal.CostStatus != accounting.StatusComplete {
		return g.limitErrorWithStrings(
			limitKeyMaxTotalCostUSD,
			nodeID,
			stepID,
			*g.maxTotalCostUSD,
			observed,
			fmt.Sprintf(
				"deterministic runtime guardrail breached; %s configured=%s observed=%s accounting_status=%s while requiring complete cost accounting under an active budget",
				limitKeyMaxTotalCostUSD,
				*g.maxTotalCostUSD,
				observed,
				treeTotal.CostStatus,
			),
		)
	}
	if treeTotal.KnownTotalCostMicrousd != nil && *treeTotal.KnownTotalCostMicrousd > *g.maxTotalCostMicrousd {
		return g.limitErrorWithStrings(
			limitKeyMaxTotalCostUSD,
			nodeID,
			stepID,
			*g.maxTotalCostUSD,
			observed,
			fmt.Sprintf("deterministic runtime guardrail breached; %s configured=%s observed=%s", limitKeyMaxTotalCostUSD, *g.maxTotalCostUSD, observed),
		)
	}
	return nil
}

func (g *deterministicGuardrails) limitErrorWithStrings(limitKey string, nodeID string, stepID string, configured string, observed string, message string) error {
	return NewLimitError(message, LimitMetadata{
		LimitKey:        limitKey,
		ConfiguredValue: configured,
		ObservedValue:   observed,
		NodeID:          nodeID,
		StepID:          stepID,
	})
}
