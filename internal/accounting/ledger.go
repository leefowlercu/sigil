package accounting

import (
	"strings"
	"sync"
)

type stepKey struct {
	nodeID string
	stepID string
}

type stepState struct {
	modelTotal Summary
	subcalls   []Summary
}

type nodeState struct {
	provider string
	model    string
	steps    []string
}

// Ledger maintains incremental run-scoped accounting state.
type Ledger struct {
	mu             sync.Mutex
	pricingVersion string
	rootNodeID     string
	steps          map[stepKey]*stepState
	nodes          map[string]*nodeState
}

// NewLedger constructs an empty run-scoped accounting ledger.
func NewLedger(pricingVersion string) *Ledger {
	return &Ledger{
		pricingVersion: strings.TrimSpace(pricingVersion),
		steps:          make(map[stepKey]*stepState),
		nodes:          make(map[string]*nodeState),
	}
}

// SetRootNodeID records the root node for run-total rollups.
func (l *Ledger) SetRootNodeID(nodeID string) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.rootNodeID = strings.TrimSpace(nodeID)
}

// RecordModelTurn records one model-turn accounting summary for a step.
func (l *Ledger) RecordModelTurn(nodeID string, stepID string, summary Summary) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.ensureStep(nodeID, stepID, summary.PricingKey.Provider, summary.PricingKey.Model)
	state.modelTotal = summary
}

// RecordSubcall records one subcall accounting summary for a step.
func (l *Ledger) RecordSubcall(nodeID string, stepID string, summary Summary) {
	if l == nil {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.ensureStep(nodeID, stepID, summary.PricingKey.Provider, summary.PricingKey.Model)
	state.subcalls = append(state.subcalls, summary)
}

// StepRollup returns one rollup for a step.
func (l *Ledger) StepRollup(nodeID string, stepID string) Rollup {
	if l == nil {
		return BuildRollup("", "", "", UnavailableSummary("", "", ""), ZeroSummary("", "", ""))
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	return l.stepRollupLocked(nodeID, stepID)
}

// NodeRollup returns one rollup for a node.
func (l *Ledger) NodeRollup(nodeID string) Rollup {
	if l == nil {
		return BuildRollup("", "", "", UnavailableSummary("", "", ""), ZeroSummary("", "", ""))
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	state := l.nodes[strings.TrimSpace(nodeID)]
	if state == nil {
		return BuildRollup("", "", l.pricingVersion, UnavailableSummary("", "", l.pricingVersion), ZeroSummary("", "", l.pricingVersion))
	}

	modelTotals := make([]Summary, 0, len(state.steps))
	subcallTotals := make([]Summary, 0, len(state.steps))
	for _, stepID := range state.steps {
		rollup := l.stepRollupLocked(nodeID, stepID)
		modelTotals = append(modelTotals, rollup.ModelTotal)
		subcallTotals = append(subcallTotals, rollup.DirectSubcallsTotal)
	}

	modelTotal := Aggregate(state.provider, state.model, l.pricingVersion, modelTotals...)
	directSubcallsTotal := Aggregate(state.provider, state.model, l.pricingVersion, subcallTotals...)
	return BuildRollup(state.provider, state.model, l.pricingVersion, modelTotal, directSubcallsTotal)
}

// RunRollup returns one rollup for the root-node tree.
func (l *Ledger) RunRollup() Rollup {
	if l == nil {
		return BuildRollup("", "", "", UnavailableSummary("", "", ""), ZeroSummary("", "", ""))
	}
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.rootNodeID == "" {
		return BuildRollup("", "", l.pricingVersion, UnavailableSummary("", "", l.pricingVersion), ZeroSummary("", "", l.pricingVersion))
	}

	return l.nodeRollupLocked(l.rootNodeID)
}

func (l *Ledger) stepRollupLocked(nodeID string, stepID string) Rollup {
	key := stepKey{nodeID: strings.TrimSpace(nodeID), stepID: strings.TrimSpace(stepID)}
	state := l.steps[key]
	if state == nil {
		return BuildRollup("", "", l.pricingVersion, UnavailableSummary("", "", l.pricingVersion), ZeroSummary("", "", l.pricingVersion))
	}

	provider := state.modelTotal.PricingKey.Provider
	model := state.modelTotal.PricingKey.Model
	if provider == "" || model == "" {
		nodeState := l.nodes[strings.TrimSpace(nodeID)]
		if nodeState != nil {
			if provider == "" {
				provider = nodeState.provider
			}
			if model == "" {
				model = nodeState.model
			}
		}
	}

	modelTotal := state.modelTotal
	if modelTotal.Currency == "" {
		modelTotal = UnavailableSummary(provider, model, l.pricingVersion)
	}

	directSubcallsTotal := Aggregate(provider, model, l.pricingVersion, state.subcalls...)
	return BuildRollup(provider, model, l.pricingVersion, modelTotal, directSubcallsTotal)
}

func (l *Ledger) nodeRollupLocked(nodeID string) Rollup {
	state := l.nodes[strings.TrimSpace(nodeID)]
	if state == nil {
		return BuildRollup("", "", l.pricingVersion, UnavailableSummary("", "", l.pricingVersion), ZeroSummary("", "", l.pricingVersion))
	}

	modelTotals := make([]Summary, 0, len(state.steps))
	subcallTotals := make([]Summary, 0, len(state.steps))
	for _, stepID := range state.steps {
		rollup := l.stepRollupLocked(nodeID, stepID)
		modelTotals = append(modelTotals, rollup.ModelTotal)
		subcallTotals = append(subcallTotals, rollup.DirectSubcallsTotal)
	}

	modelTotal := Aggregate(state.provider, state.model, l.pricingVersion, modelTotals...)
	directSubcallsTotal := Aggregate(state.provider, state.model, l.pricingVersion, subcallTotals...)
	return BuildRollup(state.provider, state.model, l.pricingVersion, modelTotal, directSubcallsTotal)
}

func (l *Ledger) ensureStep(nodeID string, stepID string, provider string, model string) *stepState {
	key := stepKey{nodeID: strings.TrimSpace(nodeID), stepID: strings.TrimSpace(stepID)}
	state, ok := l.steps[key]
	if !ok {
		state = &stepState{}
		l.steps[key] = state
	}

	nodeState := l.ensureNode(nodeID, provider, model)
	exists := false
	for _, existingStepID := range nodeState.steps {
		if existingStepID == key.stepID {
			exists = true
			break
		}
	}
	if !exists {
		nodeState.steps = append(nodeState.steps, key.stepID)
	}
	return state
}

func (l *Ledger) ensureNode(nodeID string, provider string, model string) *nodeState {
	cleanNodeID := strings.TrimSpace(nodeID)
	state, ok := l.nodes[cleanNodeID]
	if !ok {
		state = &nodeState{}
		l.nodes[cleanNodeID] = state
	}
	if state.provider == "" {
		state.provider = strings.TrimSpace(provider)
	}
	if state.model == "" {
		state.model = strings.TrimSpace(model)
	}
	return state
}
