package accounting

import (
	"fmt"
	"strings"
)

// BuildLeafSummary constructs one leaf accounting summary from normalized
// tokens plus gateway-reported and/or fallback-derived costs.
func BuildLeafSummary(input LeafInput) Summary {
	summary := Summary{
		Currency:        CurrencyUSD,
		PricingKey:      PricingKey{Provider: strings.TrimSpace(input.Provider), Model: strings.TrimSpace(input.Model)},
		PricingVersion:  strings.TrimSpace(input.PricingVersion),
		InputTokens:     cloneInt64Pointer(input.InputTokens),
		OutputTokens:    cloneInt64Pointer(input.OutputTokens),
		TotalTokens:     cloneInt64Pointer(input.TotalTokens),
		ReasoningTokens: cloneInt64Pointer(input.ReasoningTokens),
		TokenSource:     SourceUnavailable,
		TokenStatus:     StatusUnavailable,
		CostSource:      SourceUnavailable,
		CostStatus:      StatusUnavailable,
	}

	tokenFieldCount := 0
	if summary.InputTokens != nil {
		tokenFieldCount++
	}
	if summary.OutputTokens != nil {
		tokenFieldCount++
	}
	if summary.TotalTokens != nil {
		tokenFieldCount++
	}
	hasKnownTokenSubtotal := tokenFieldCount > 0 || summary.ReasoningTokens != nil
	switch {
	case tokenFieldCount == 3:
		summary.TokenSource = SourceGatewayReported
		summary.TokenStatus = StatusComplete
	case hasKnownTokenSubtotal:
		summary.TokenSource = SourceGatewayReported
		summary.TokenStatus = StatusPartial
		summary.MissingTokenItemCount = 1
	default:
		summary.MissingTokenItemCount = 1
	}

	usedGatewayCost := false
	usedFallbackCost := false
	exactTotalCostKnown := false

	if input.GatewayInputCostMicrousd != nil {
		summary.KnownInputCostMicrousd = cloneInt64Pointer(input.GatewayInputCostMicrousd)
		usedGatewayCost = true
	}
	if input.GatewayOutputCostMicrousd != nil {
		summary.KnownOutputCostMicrousd = cloneInt64Pointer(input.GatewayOutputCostMicrousd)
		usedGatewayCost = true
	}
	if input.GatewayReasoningCostMicrousd != nil {
		summary.KnownReasoningCostMicrousd = cloneInt64Pointer(input.GatewayReasoningCostMicrousd)
		usedGatewayCost = true
	}
	if input.GatewayTotalCostMicrousd != nil {
		summary.KnownTotalCostMicrousd = cloneInt64Pointer(input.GatewayTotalCostMicrousd)
		usedGatewayCost = true
		exactTotalCostKnown = true
	}

	if summary.KnownTotalCostMicrousd == nil && input.FallbackPricing != nil {
		if applied := applyFallbackPricing(&summary, input); applied {
			usedFallbackCost = true
		}
		if summary.KnownTotalCostMicrousd != nil {
			exactTotalCostKnown = true
		}
	}

	if !exactTotalCostKnown && summary.KnownTotalCostMicrousd == nil {
		summary.KnownTotalCostMicrousd = subtotalKnownCostMicrousd(summary)
	}

	switch {
	case exactTotalCostKnown:
		summary.CostStatus = StatusComplete
	case summary.KnownTotalCostMicrousd != nil:
		summary.CostStatus = StatusPartial
		summary.MissingCostItemCount = 1
	default:
		summary.CostStatus = StatusUnavailable
		summary.MissingCostItemCount = 1
	}

	switch {
	case usedGatewayCost && usedFallbackCost:
		summary.CostSource = SourceMixed
	case usedGatewayCost:
		summary.CostSource = SourceGatewayReported
	case usedFallbackCost:
		summary.CostSource = SourceFallbackPricing
	default:
		summary.CostSource = SourceUnavailable
	}

	return summary
}

// ZeroSummary returns an exact zero summary for scopes with no contributing
// accounting items.
func ZeroSummary(provider string, model string, pricingVersion string) Summary {
	zero := int64(0)
	return Summary{
		Currency:                   CurrencyUSD,
		InputTokens:                int64Pointer(zero),
		OutputTokens:               int64Pointer(zero),
		TotalTokens:                int64Pointer(zero),
		ReasoningTokens:            int64Pointer(zero),
		KnownInputCostMicrousd:     int64Pointer(zero),
		KnownOutputCostMicrousd:    int64Pointer(zero),
		KnownReasoningCostMicrousd: int64Pointer(zero),
		KnownTotalCostMicrousd:     int64Pointer(zero),
		TokenSource:                SourceUnavailable,
		TokenStatus:                StatusComplete,
		CostSource:                 SourceUnavailable,
		CostStatus:                 StatusComplete,
		PricingKey: PricingKey{
			Provider: strings.TrimSpace(provider),
			Model:    strings.TrimSpace(model),
		},
		PricingVersion:        strings.TrimSpace(pricingVersion),
		MissingTokenItemCount: 0,
		MissingCostItemCount:  0,
	}
}

// UnavailableSummary returns a summary for one item whose accounting could not
// be established.
func UnavailableSummary(provider string, model string, pricingVersion string) Summary {
	return Summary{
		Currency:              CurrencyUSD,
		TokenSource:           SourceUnavailable,
		TokenStatus:           StatusUnavailable,
		CostSource:            SourceUnavailable,
		CostStatus:            StatusUnavailable,
		PricingKey:            PricingKey{Provider: strings.TrimSpace(provider), Model: strings.TrimSpace(model)},
		PricingVersion:        strings.TrimSpace(pricingVersion),
		MissingTokenItemCount: 1,
		MissingCostItemCount:  1,
	}
}

// Aggregate returns one summary over one or more child summaries.
func Aggregate(provider string, model string, pricingVersion string, summaries ...Summary) Summary {
	if len(summaries) == 0 {
		return ZeroSummary(provider, model, pricingVersion)
	}

	contributors := make([]Summary, 0, len(summaries))
	for _, summary := range summaries {
		if isNeutralZeroSummary(summary) {
			continue
		}
		contributors = append(contributors, summary)
	}
	if len(contributors) == 0 {
		return ZeroSummary(provider, model, pricingVersion)
	}

	aggregate := Summary{
		Currency:       CurrencyUSD,
		PricingKey:     PricingKey{Provider: strings.TrimSpace(provider), Model: strings.TrimSpace(model)},
		PricingVersion: strings.TrimSpace(pricingVersion),
		TokenSource:    selectAggregateSource(SourceUnavailable, contributors, true),
		TokenStatus:    StatusComplete,
		CostSource:     selectAggregateSource(SourceUnavailable, contributors, false),
		CostStatus:     StatusComplete,
	}
	unavailableTokenContributors := 0
	unavailableCostContributors := 0

	for _, summary := range contributors {
		aggregate.MissingTokenItemCount += summary.MissingTokenItemCount
		aggregate.MissingCostItemCount += summary.MissingCostItemCount
		aggregate.InputTokens = addKnownValue(aggregate.InputTokens, summary.InputTokens)
		aggregate.OutputTokens = addKnownValue(aggregate.OutputTokens, summary.OutputTokens)
		aggregate.TotalTokens = addKnownValue(aggregate.TotalTokens, summary.TotalTokens)
		aggregate.ReasoningTokens = addKnownValue(aggregate.ReasoningTokens, summary.ReasoningTokens)
		aggregate.KnownInputCostMicrousd = addKnownValue(aggregate.KnownInputCostMicrousd, summary.KnownInputCostMicrousd)
		aggregate.KnownOutputCostMicrousd = addKnownValue(aggregate.KnownOutputCostMicrousd, summary.KnownOutputCostMicrousd)
		aggregate.KnownReasoningCostMicrousd = addKnownValue(aggregate.KnownReasoningCostMicrousd, summary.KnownReasoningCostMicrousd)
		aggregate.KnownTotalCostMicrousd = addKnownValue(aggregate.KnownTotalCostMicrousd, summary.KnownTotalCostMicrousd)
		if aggregate.PricingVersion == "" && summary.PricingVersion != "" {
			aggregate.PricingVersion = summary.PricingVersion
		}
		if aggregate.PricingKey.Provider == "" && summary.PricingKey.Provider != "" {
			aggregate.PricingKey.Provider = summary.PricingKey.Provider
		}
		if aggregate.PricingKey.Model == "" && summary.PricingKey.Model != "" {
			aggregate.PricingKey.Model = summary.PricingKey.Model
		}
		if summary.TokenStatus != StatusComplete {
			aggregate.TokenStatus = statusWithMissing(summary.TokenStatus, aggregate.TokenStatus)
		}
		if summary.CostStatus != StatusComplete {
			aggregate.CostStatus = statusWithMissing(summary.CostStatus, aggregate.CostStatus)
		}
		if summary.TokenStatus == StatusUnavailable {
			unavailableTokenContributors++
		}
		if summary.CostStatus == StatusUnavailable {
			unavailableCostContributors++
		}
	}

	if aggregate.MissingTokenItemCount > 0 && aggregate.TokenStatus == StatusComplete {
		aggregate.TokenStatus = StatusPartial
	}
	if aggregate.MissingCostItemCount > 0 && aggregate.CostStatus == StatusComplete {
		aggregate.CostStatus = StatusPartial
	}
	if unavailableTokenContributors == len(contributors) && noTokenValues(aggregate) {
		aggregate.TokenStatus = StatusUnavailable
	}
	if unavailableCostContributors == len(contributors) && noCostValues(aggregate) {
		aggregate.CostStatus = StatusUnavailable
	}

	return aggregate
}

// BuildRollup combines model and direct-subcall totals into one tree total.
func BuildRollup(provider string, model string, pricingVersion string, modelTotal Summary, directSubcallsTotal Summary) Rollup {
	return Rollup{
		ModelTotal:          modelTotal,
		DirectSubcallsTotal: directSubcallsTotal,
		TreeTotal:           Aggregate(provider, model, pricingVersion, modelTotal, directSubcallsTotal),
	}
}

// ValidateSummary validates one accounting summary.
func ValidateSummary(summary Summary) error {
	if strings.TrimSpace(summary.Currency) != CurrencyUSD {
		return fmt.Errorf("currency %q is not supported", summary.Currency)
	}
	if err := validateTokenSource(summary.TokenSource); err != nil {
		return fmt.Errorf("token_source %w", err)
	}
	if err := validateCostSource(summary.CostSource); err != nil {
		return fmt.Errorf("cost_source %w", err)
	}
	if err := validateStatus(summary.TokenStatus); err != nil {
		return fmt.Errorf("token_status %w", err)
	}
	if err := validateStatus(summary.CostStatus); err != nil {
		return fmt.Errorf("cost_status %w", err)
	}
	if err := validateKnownNonNegative(summary.InputTokens); err != nil {
		return fmt.Errorf("input_tokens %w", err)
	}
	if err := validateKnownNonNegative(summary.OutputTokens); err != nil {
		return fmt.Errorf("output_tokens %w", err)
	}
	if err := validateKnownNonNegative(summary.TotalTokens); err != nil {
		return fmt.Errorf("total_tokens %w", err)
	}
	if err := validateKnownNonNegative(summary.ReasoningTokens); err != nil {
		return fmt.Errorf("reasoning_tokens %w", err)
	}
	if err := validateKnownNonNegative(summary.KnownInputCostMicrousd); err != nil {
		return fmt.Errorf("known_input_cost_microusd %w", err)
	}
	if err := validateKnownNonNegative(summary.KnownOutputCostMicrousd); err != nil {
		return fmt.Errorf("known_output_cost_microusd %w", err)
	}
	if err := validateKnownNonNegative(summary.KnownReasoningCostMicrousd); err != nil {
		return fmt.Errorf("known_reasoning_cost_microusd %w", err)
	}
	if err := validateKnownNonNegative(summary.KnownTotalCostMicrousd); err != nil {
		return fmt.Errorf("known_total_cost_microusd %w", err)
	}
	if summary.MissingTokenItemCount < 0 {
		return fmt.Errorf("missing_token_item_count must be >= 0")
	}
	if summary.MissingCostItemCount < 0 {
		return fmt.Errorf("missing_cost_item_count must be >= 0")
	}
	if summary.TokenStatus == StatusComplete && summary.TotalTokens == nil {
		return fmt.Errorf("complete token_status requires total_tokens")
	}
	if summary.CostStatus == StatusComplete && summary.KnownTotalCostMicrousd == nil {
		return fmt.Errorf("complete cost_status requires known_total_cost_microusd")
	}
	if summary.TokenStatus == StatusUnavailable && summary.MissingTokenItemCount == 0 {
		return fmt.Errorf("unavailable token_status requires missing_token_item_count >= 1")
	}
	if summary.CostStatus == StatusUnavailable && summary.MissingCostItemCount == 0 {
		return fmt.Errorf("unavailable cost_status requires missing_cost_item_count >= 1")
	}
	return nil
}

// ValidateRollup validates one accounting rollup.
func ValidateRollup(rollup Rollup) error {
	if err := ValidateSummary(rollup.ModelTotal); err != nil {
		return fmt.Errorf("model_total %w", err)
	}
	if err := ValidateSummary(rollup.DirectSubcallsTotal); err != nil {
		return fmt.Errorf("direct_subcalls_total %w", err)
	}
	if err := ValidateSummary(rollup.TreeTotal); err != nil {
		return fmt.Errorf("tree_total %w", err)
	}
	return nil
}

func applyFallbackPricing(summary *Summary, input LeafInput) bool {
	if summary == nil || input.FallbackPricing == nil {
		return false
	}

	applied := false
	if summary.KnownInputCostMicrousd == nil && input.InputTokens != nil {
		summary.KnownInputCostMicrousd = int64Pointer(mulMicrousd(*input.InputTokens, input.FallbackPricing.InputMicrousdPerMillionTokens))
		applied = true
	}

	outputRate := input.FallbackPricing.OutputMicrousdPerMillionTokens
	reasoningRate := outputRate
	if input.FallbackPricing.ReasoningMicrousdPerMillionTokens != nil {
		reasoningRate = *input.FallbackPricing.ReasoningMicrousdPerMillionTokens
	}

	if summary.KnownOutputCostMicrousd == nil || summary.KnownReasoningCostMicrousd == nil || summary.KnownTotalCostMicrousd == nil {
		knownOutputCost, knownReasoningCost, ok := fallbackOutputCosts(input, outputRate, reasoningRate)
		if ok {
			if summary.KnownOutputCostMicrousd == nil {
				summary.KnownOutputCostMicrousd = knownOutputCost
				applied = true
			}
			if knownReasoningCost != nil && summary.KnownReasoningCostMicrousd == nil {
				summary.KnownReasoningCostMicrousd = knownReasoningCost
				applied = true
			}
		}
	}
	if summary.KnownTotalCostMicrousd == nil {
		if total, ok := exactFallbackTotal(input, *summary); ok {
			summary.KnownTotalCostMicrousd = int64Pointer(total)
			applied = true
		}
	}

	return applied
}

func fallbackOutputCosts(input LeafInput, outputRate int64, reasoningRate int64) (*int64, *int64, bool) {
	if input.OutputTokens == nil {
		return nil, nil, false
	}

	if input.FallbackPricing == nil {
		return nil, nil, false
	}

	if input.FallbackPricing.ReasoningMicrousdPerMillionTokens == nil {
		if input.ReasoningTokens != nil {
			reasoningCost := int64Pointer(mulMicrousd(*input.ReasoningTokens, outputRate))
			outputOnlyTokens := *input.OutputTokens - *input.ReasoningTokens
			if outputOnlyTokens < 0 {
				outputOnlyTokens = 0
			}
			outputCost := int64Pointer(mulMicrousd(outputOnlyTokens, outputRate))
			return outputCost, reasoningCost, true
		}
		return int64Pointer(mulMicrousd(*input.OutputTokens, outputRate)), nil, true
	}

	if input.ReasoningTokens == nil {
		if !input.ReasoningEnabled {
			return int64Pointer(mulMicrousd(*input.OutputTokens, outputRate)), int64Pointer(0), true
		}
		return nil, nil, false
	}

	outputOnlyTokens := *input.OutputTokens - *input.ReasoningTokens
	if outputOnlyTokens < 0 {
		outputOnlyTokens = 0
	}
	outputCost := int64Pointer(mulMicrousd(outputOnlyTokens, outputRate))
	reasoningCost := int64Pointer(mulMicrousd(*input.ReasoningTokens, reasoningRate))
	return outputCost, reasoningCost, true
}

func mulMicrousd(tokens int64, microusdPerMillion int64) int64 {
	if tokens <= 0 || microusdPerMillion <= 0 {
		return 0
	}
	return (tokens * microusdPerMillion) / 1_000_000
}

func selectAggregateSource(defaultSource Source, summaries []Summary, useToken bool) Source {
	source := defaultSource
	for _, summary := range summaries {
		next := summary.CostSource
		if useToken {
			next = summary.TokenSource
		}
		if next == SourceUnavailable {
			continue
		}
		if source == SourceUnavailable {
			source = next
			continue
		}
		if source != next {
			return SourceMixed
		}
	}
	return source
}

func statusWithMissing(next Status, current Status) Status {
	if next == StatusUnavailable && current == StatusComplete {
		return StatusPartial
	}
	if next == StatusPartial {
		return StatusPartial
	}
	return current
}

func addKnownValue(base *int64, next *int64) *int64 {
	if base == nil && next == nil {
		return nil
	}
	total := int64(0)
	if base != nil {
		total += *base
	}
	if next != nil {
		total += *next
	}
	return &total
}

func subtotalKnownCostMicrousd(summary Summary) *int64 {
	subtotal := addKnownValue(nil, summary.KnownInputCostMicrousd)
	subtotal = addKnownValue(subtotal, summary.KnownOutputCostMicrousd)
	subtotal = addKnownValue(subtotal, summary.KnownReasoningCostMicrousd)
	return subtotal
}

func isNeutralZeroSummary(summary Summary) bool {
	if strings.TrimSpace(summary.Currency) != CurrencyUSD {
		return false
	}
	if summary.TokenSource != SourceUnavailable || summary.CostSource != SourceUnavailable {
		return false
	}
	if summary.TokenStatus != StatusComplete || summary.CostStatus != StatusComplete {
		return false
	}
	if summary.MissingTokenItemCount != 0 || summary.MissingCostItemCount != 0 {
		return false
	}

	return pointerIsZero(summary.InputTokens) &&
		pointerIsZero(summary.OutputTokens) &&
		pointerIsZero(summary.TotalTokens) &&
		pointerIsZero(summary.ReasoningTokens) &&
		pointerIsZero(summary.KnownInputCostMicrousd) &&
		pointerIsZero(summary.KnownOutputCostMicrousd) &&
		pointerIsZero(summary.KnownReasoningCostMicrousd) &&
		pointerIsZero(summary.KnownTotalCostMicrousd)
}

func pointerIsZero(value *int64) bool {
	return value != nil && *value == 0
}

func validateTokenSource(source Source) error {
	switch source {
	case SourceGatewayReported, SourceMixed, SourceUnavailable:
		return nil
	default:
		return fmt.Errorf("value %q is not supported", source)
	}
}

func validateCostSource(source Source) error {
	switch source {
	case SourceGatewayReported, SourceFallbackPricing, SourceMixed, SourceUnavailable:
		return nil
	default:
		return fmt.Errorf("value %q is not supported", source)
	}
}

func validateStatus(status Status) error {
	switch status {
	case StatusComplete, StatusPartial, StatusUnavailable:
		return nil
	default:
		return fmt.Errorf("value %q is not supported", status)
	}
}

func validateKnownNonNegative(value *int64) error {
	if value == nil {
		return nil
	}
	if *value < 0 {
		return fmt.Errorf("must be >= 0")
	}
	return nil
}

func noTokenValues(summary Summary) bool {
	return summary.InputTokens == nil && summary.OutputTokens == nil && summary.TotalTokens == nil && summary.ReasoningTokens == nil
}

func noCostValues(summary Summary) bool {
	return summary.KnownInputCostMicrousd == nil && summary.KnownOutputCostMicrousd == nil &&
		summary.KnownReasoningCostMicrousd == nil && summary.KnownTotalCostMicrousd == nil
}

func sumKnownCosts(values ...*int64) (int64, bool) {
	total := int64(0)
	known := false
	for _, value := range values {
		if value == nil {
			continue
		}
		total += *value
		known = true
	}
	return total, known
}

func exactFallbackTotal(input LeafInput, summary Summary) (int64, bool) {
	if summary.TokenStatus != StatusComplete {
		return 0, false
	}
	total := int64(0)
	if input.InputTokens != nil {
		if summary.KnownInputCostMicrousd == nil {
			return 0, false
		}
		total += *summary.KnownInputCostMicrousd
	}
	if input.OutputTokens != nil {
		if summary.KnownOutputCostMicrousd == nil {
			return 0, false
		}
		total += *summary.KnownOutputCostMicrousd
	}
	if input.ReasoningTokens != nil && summary.KnownReasoningCostMicrousd != nil {
		total += *summary.KnownReasoningCostMicrousd
	} else if input.ReasoningEnabled && input.FallbackPricing != nil && input.FallbackPricing.ReasoningMicrousdPerMillionTokens != nil && input.ReasoningTokens == nil {
		return 0, false
	}
	return total, true
}

func cloneInt64Pointer(value *int64) *int64 {
	if value == nil {
		return nil
	}
	cloned := *value
	return &cloned
}

func int64Pointer(value int64) *int64 {
	cloned := value
	return &cloned
}
