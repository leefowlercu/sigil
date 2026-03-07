package harness

import (
	"strings"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
)

func buildInferenceAccountingConfig(runConfig config.RunConfig) inference.AccountingConfig {
	accountingConfig := inference.AccountingConfig{
		PricingVersion: strings.TrimSpace(runConfig.Accounting.PricingVersion),
	}

	if pricing, ok := runConfig.Accounting.PricingFor(runConfig.LLM.Provider, runConfig.LLM.Model); ok {
		accountingConfig.FallbackPricing = &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  pricing.InputMicrousdPerMillionTokens,
			OutputMicrousdPerMillionTokens: pricing.OutputMicrousdPerMillionTokens,
		}
		if pricing.ReasoningMicrousdPerMillionTokens != nil {
			value := *pricing.ReasoningMicrousdPerMillionTokens
			accountingConfig.FallbackPricing.ReasoningMicrousdPerMillionTokens = &value
		}
	}

	return accountingConfig
}

func zeroAccountingSummaryForRun(runConfig config.RunConfig) accounting.Summary {
	return accounting.ZeroSummary(
		runConfig.LLM.Provider,
		runConfig.LLM.Model,
		runConfig.Accounting.PricingVersion,
	)
}

func unavailableAccountingSummaryForRun(runConfig config.RunConfig) accounting.Summary {
	return accounting.UnavailableSummary(
		runConfig.LLM.Provider,
		runConfig.LLM.Model,
		runConfig.Accounting.PricingVersion,
	)
}

func unavailableAccountingRollupForRun(runConfig config.RunConfig) accounting.Rollup {
	return accounting.BuildRollup(
		runConfig.LLM.Provider,
		runConfig.LLM.Model,
		runConfig.Accounting.PricingVersion,
		unavailableAccountingSummaryForRun(runConfig),
		zeroAccountingSummaryForRun(runConfig),
	)
}
