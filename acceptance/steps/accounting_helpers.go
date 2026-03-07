package steps

import "github.com/leefowlercu/sigil/internal/accounting"

func acceptanceAccountingSummary(provider string, model string) accounting.Summary {
	inputTokens := int64(12)
	outputTokens := int64(5)
	totalTokens := int64(17)
	reasoningTokens := int64(2)
	totalCost := int64(1750)

	return accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:                 provider,
		Model:                    model,
		PricingVersion:           "v1",
		InputTokens:              &inputTokens,
		OutputTokens:             &outputTokens,
		TotalTokens:              &totalTokens,
		ReasoningTokens:          &reasoningTokens,
		GatewayTotalCostMicrousd: &totalCost,
	})
}

func acceptancePartialAccountingSummary(provider string, model string) accounting.Summary {
	inputTokens := int64(9)
	outputTokens := int64(4)
	totalTokens := int64(13)
	inputCost := int64(900)

	return accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:                 provider,
		Model:                    model,
		PricingVersion:           "v1",
		InputTokens:              &inputTokens,
		OutputTokens:             &outputTokens,
		TotalTokens:              &totalTokens,
		GatewayInputCostMicrousd: &inputCost,
	})
}

func acceptanceFallbackAccountingSummary(provider string, model string) accounting.Summary {
	inputTokens := int64(10_000)
	outputTokens := int64(8_000)
	totalTokens := int64(18_000)
	reasoningTokens := int64(3_000)
	reasoningRate := int64(300000)

	return accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:        provider,
		Model:           model,
		PricingVersion:  "v1",
		InputTokens:     &inputTokens,
		OutputTokens:    &outputTokens,
		TotalTokens:     &totalTokens,
		ReasoningTokens: &reasoningTokens,
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:     100000,
			OutputMicrousdPerMillionTokens:    200000,
			ReasoningMicrousdPerMillionTokens: &reasoningRate,
		},
	})
}

func acceptanceAccountingRollup(provider string, model string) accounting.Rollup {
	return accounting.BuildRollup(
		provider,
		model,
		"v1",
		acceptanceAccountingSummary(provider, model),
		accounting.ZeroSummary(provider, model, "v1"),
	)
}

func acceptancePartialAccountingRollup(provider string, model string) accounting.Rollup {
	return accounting.BuildRollup(
		provider,
		model,
		"v1",
		acceptancePartialAccountingSummary(provider, model),
		accounting.ZeroSummary(provider, model, "v1"),
	)
}
