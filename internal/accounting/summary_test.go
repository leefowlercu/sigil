package accounting

import "testing"

func TestBuildLeafSummaryUsesGatewayTotalCostWithoutInventingComponents(t *testing.T) {
	inputTokens := int64(12)
	outputTokens := int64(5)
	totalTokens := int64(17)
	totalCost := int64(1234)

	summary := BuildLeafSummary(LeafInput{
		Provider:                 "openai",
		Model:                    "gpt-5.1",
		PricingVersion:           "v1",
		InputTokens:              &inputTokens,
		OutputTokens:             &outputTokens,
		TotalTokens:              &totalTokens,
		GatewayTotalCostMicrousd: &totalCost,
	})

	if summary.CostSource != SourceGatewayReported {
		t.Fatalf("expected cost_source=%q, got %q", SourceGatewayReported, summary.CostSource)
	}
	if summary.CostStatus != StatusComplete {
		t.Fatalf("expected cost_status=%q, got %q", StatusComplete, summary.CostStatus)
	}
	if summary.KnownTotalCostMicrousd == nil || *summary.KnownTotalCostMicrousd != totalCost {
		t.Fatalf("expected known_total_cost_microusd=%d, got %+v", totalCost, summary.KnownTotalCostMicrousd)
	}
	if summary.KnownInputCostMicrousd != nil || summary.KnownOutputCostMicrousd != nil || summary.KnownReasoningCostMicrousd != nil {
		t.Fatalf("expected gateway total-only accounting to preserve nil component costs, got %+v", summary)
	}
}

func TestBuildLeafSummaryDerivesFallbackCostsAndReasoningSplit(t *testing.T) {
	inputTokens := int64(1_000_000)
	outputTokens := int64(1_000_000)
	totalTokens := int64(2_000_000)
	reasoningTokens := int64(250_000)
	reasoningRate := int64(300)

	summary := BuildLeafSummary(LeafInput{
		Provider:        "openai",
		Model:           "gpt-5.1",
		PricingVersion:  "v1",
		InputTokens:     &inputTokens,
		OutputTokens:    &outputTokens,
		TotalTokens:     &totalTokens,
		ReasoningTokens: &reasoningTokens,
		FallbackPricing: &FallbackPricing{
			InputMicrousdPerMillionTokens:     100,
			OutputMicrousdPerMillionTokens:    200,
			ReasoningMicrousdPerMillionTokens: &reasoningRate,
		},
	})

	if summary.CostSource != SourceFallbackPricing {
		t.Fatalf("expected cost_source=%q, got %q", SourceFallbackPricing, summary.CostSource)
	}
	if summary.CostStatus != StatusComplete {
		t.Fatalf("expected cost_status=%q, got %q", StatusComplete, summary.CostStatus)
	}
	if summary.KnownInputCostMicrousd == nil || *summary.KnownInputCostMicrousd != 100 {
		t.Fatalf("expected input fallback cost 100, got %+v", summary.KnownInputCostMicrousd)
	}
	if summary.KnownOutputCostMicrousd == nil || *summary.KnownOutputCostMicrousd != 150 {
		t.Fatalf("expected output fallback cost 150, got %+v", summary.KnownOutputCostMicrousd)
	}
	if summary.KnownReasoningCostMicrousd == nil || *summary.KnownReasoningCostMicrousd != 75 {
		t.Fatalf("expected reasoning fallback cost 75, got %+v", summary.KnownReasoningCostMicrousd)
	}
	if summary.KnownTotalCostMicrousd == nil || *summary.KnownTotalCostMicrousd != 325 {
		t.Fatalf("expected total fallback cost 325, got %+v", summary.KnownTotalCostMicrousd)
	}
}

func TestBuildLeafSummaryPreservesFallbackCostSubtotalWhenTokenSetIsPartial(t *testing.T) {
	inputTokens := int64(1_000_000)

	summary := BuildLeafSummary(LeafInput{
		Provider:       "openai",
		Model:          "gpt-5.1",
		PricingVersion: "v1",
		InputTokens:    &inputTokens,
		FallbackPricing: &FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	})

	if summary.KnownInputCostMicrousd == nil || *summary.KnownInputCostMicrousd != 100 {
		t.Fatalf("expected input fallback cost 100, got %+v", summary.KnownInputCostMicrousd)
	}
	if summary.CostStatus != StatusPartial {
		t.Fatalf("expected partial cost status, got %q", summary.CostStatus)
	}
	if summary.KnownTotalCostMicrousd == nil || *summary.KnownTotalCostMicrousd != 100 {
		t.Fatalf("expected fallback subtotal cost 100, got %+v", summary.KnownTotalCostMicrousd)
	}
}

func TestBuildLeafSummaryPreservesKnownGatewayCostSubtotalWhenCostsArePartial(t *testing.T) {
	inputTokens := int64(9)
	outputTokens := int64(4)
	totalTokens := int64(13)
	inputCost := int64(100)

	summary := BuildLeafSummary(LeafInput{
		Provider:                 "openai",
		Model:                    "gpt-5.1",
		PricingVersion:           "v1",
		InputTokens:              &inputTokens,
		OutputTokens:             &outputTokens,
		TotalTokens:              &totalTokens,
		GatewayInputCostMicrousd: &inputCost,
	})

	if summary.CostStatus != StatusPartial {
		t.Fatalf("expected partial cost status, got %q", summary.CostStatus)
	}
	if summary.KnownTotalCostMicrousd == nil || *summary.KnownTotalCostMicrousd != inputCost {
		t.Fatalf("expected known_total_cost_microusd=%d, got %+v", inputCost, summary.KnownTotalCostMicrousd)
	}
}

func TestAggregatePreservesUnavailableCostWhenTotalsAreUnknown(t *testing.T) {
	inputTokens := int64(9)
	outputTokens := int64(4)
	totalTokens := int64(13)

	partial := BuildLeafSummary(LeafInput{
		Provider:       "openai",
		Model:          "gpt-5.1",
		PricingVersion: "v1",
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
		TotalTokens:    &totalTokens,
	})
	aggregate := Aggregate("openai", "gpt-5.1", "v1", partial, ZeroSummary("openai", "gpt-5.1", "v1"))

	if aggregate.TokenStatus != StatusComplete {
		t.Fatalf("expected complete token status, got %q", aggregate.TokenStatus)
	}
	if aggregate.CostStatus != StatusUnavailable {
		t.Fatalf("expected unavailable cost status, got %q", aggregate.CostStatus)
	}
	if aggregate.KnownTotalCostMicrousd != nil {
		t.Fatalf("expected aggregate total cost to remain unknown, got %+v", aggregate.KnownTotalCostMicrousd)
	}
}

func TestBuildRollupPreservesUnknownTotalsWhenNoSubcallsExist(t *testing.T) {
	inputTokens := int64(9)
	outputTokens := int64(4)

	modelOnly := BuildLeafSummary(LeafInput{
		Provider:       "openai",
		Model:          "gpt-5.1",
		PricingVersion: "v1",
		InputTokens:    &inputTokens,
		OutputTokens:   &outputTokens,
	})

	rollup := BuildRollup("openai", "gpt-5.1", "v1", modelOnly, ZeroSummary("openai", "gpt-5.1", "v1"))
	if rollup.TreeTotal.TokenStatus != StatusPartial {
		t.Fatalf("expected partial tree token status, got %q", rollup.TreeTotal.TokenStatus)
	}
	if rollup.TreeTotal.TotalTokens != nil {
		t.Fatalf("expected tree total_tokens to remain unknown, got %+v", rollup.TreeTotal.TotalTokens)
	}
	if rollup.TreeTotal.CostStatus != StatusUnavailable {
		t.Fatalf("expected unavailable tree cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	if rollup.TreeTotal.KnownTotalCostMicrousd != nil {
		t.Fatalf("expected tree known_total_cost_microusd to remain unknown, got %+v", rollup.TreeTotal.KnownTotalCostMicrousd)
	}
}

func TestBuildLeafSummaryTreatsReasoningOnlyUsageAsPartial(t *testing.T) {
	reasoningTokens := int64(9)

	summary := BuildLeafSummary(LeafInput{
		Provider:        "openai",
		Model:           "gpt-5.1",
		PricingVersion:  "v1",
		ReasoningTokens: &reasoningTokens,
	})

	if summary.ReasoningTokens == nil || *summary.ReasoningTokens != reasoningTokens {
		t.Fatalf("expected reasoning_tokens=%d, got %+v", reasoningTokens, summary.ReasoningTokens)
	}
	if summary.TokenStatus != StatusPartial {
		t.Fatalf("expected partial token status, got %q", summary.TokenStatus)
	}
	if summary.TokenSource != SourceGatewayReported {
		t.Fatalf("expected gateway_reported token source, got %q", summary.TokenSource)
	}
}

func TestLedgerRunRollupIsUnavailableWhenNoTurnsRecorded(t *testing.T) {
	ledger := NewLedger("v1")
	ledger.SetRootNodeID("root-node")

	rollup := ledger.RunRollup()
	if rollup.TreeTotal.TokenStatus != StatusUnavailable {
		t.Fatalf("expected unavailable token status, got %q", rollup.TreeTotal.TokenStatus)
	}
	if rollup.TreeTotal.CostStatus != StatusUnavailable {
		t.Fatalf("expected unavailable cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	if rollup.TreeTotal.TotalTokens != nil {
		t.Fatalf("expected total_tokens to remain unknown, got %+v", rollup.TreeTotal.TotalTokens)
	}
	if rollup.TreeTotal.KnownTotalCostMicrousd != nil {
		t.Fatalf("expected known_total_cost_microusd to remain unknown, got %+v", rollup.TreeTotal.KnownTotalCostMicrousd)
	}
}

func TestLedgerAggregatesStepNodeAndRunRollups(t *testing.T) {
	ledger := NewLedger("v1")
	ledger.SetRootNodeID("root-node")

	modelSummary := gatewaySummary(12, 5, 17, 1000)
	subcallSummary := gatewaySummary(3, 2, 5, 200)

	ledger.RecordModelTurn("root-node", "step-1", modelSummary)
	ledger.RecordSubcall("root-node", "step-1", subcallSummary)

	stepRollup := ledger.StepRollup("root-node", "step-1")
	if stepRollup.TreeTotal.TotalTokens == nil || *stepRollup.TreeTotal.TotalTokens != 22 {
		t.Fatalf("expected step tree total_tokens=22, got %+v", stepRollup.TreeTotal.TotalTokens)
	}
	if stepRollup.TreeTotal.KnownTotalCostMicrousd == nil || *stepRollup.TreeTotal.KnownTotalCostMicrousd != 1200 {
		t.Fatalf("expected step tree known_total_cost_microusd=1200, got %+v", stepRollup.TreeTotal.KnownTotalCostMicrousd)
	}

	nodeRollup := ledger.NodeRollup("root-node")
	if nodeRollup.TreeTotal.TotalTokens == nil || *nodeRollup.TreeTotal.TotalTokens != 22 {
		t.Fatalf("expected node tree total_tokens=22, got %+v", nodeRollup.TreeTotal.TotalTokens)
	}

	runRollup := ledger.RunRollup()
	if runRollup.TreeTotal.TotalTokens == nil || *runRollup.TreeTotal.TotalTokens != 22 {
		t.Fatalf("expected run tree total_tokens=22, got %+v", runRollup.TreeTotal.TotalTokens)
	}
}

func TestValidateSummaryRejectsFallbackPricingTokenSource(t *testing.T) {
	summary := gatewaySummary(12, 5, 17, 1000)
	summary.TokenSource = SourceFallbackPricing

	err := ValidateSummary(summary)
	if err == nil {
		t.Fatalf("expected token_source validation error")
	}
}

func gatewaySummary(input int64, output int64, total int64, cost int64) Summary {
	return BuildLeafSummary(LeafInput{
		Provider:                 "openai",
		Model:                    "gpt-5.1",
		PricingVersion:           "v1",
		InputTokens:              &input,
		OutputTokens:             &output,
		TotalTokens:              &total,
		GatewayTotalCostMicrousd: &cost,
	})
}
