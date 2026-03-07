package accounting

// CurrencyUSD is the only supported accounting currency in v1.
const CurrencyUSD = "usd"

// Source describes how known accounting values were established.
type Source string

const (
	SourceGatewayReported Source = "gateway_reported"
	SourceFallbackPricing Source = "fallback_pricing"
	SourceMixed           Source = "mixed"
	SourceUnavailable     Source = "unavailable"
)

// Status describes whether a summary is complete, partial, or unavailable.
type Status string

const (
	StatusComplete    Status = "complete"
	StatusPartial     Status = "partial"
	StatusUnavailable Status = "unavailable"
)

// PricingKey identifies the provider/model pair used for accounting.
type PricingKey struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// FallbackPricing defines configured per-million-token pricing rates.
type FallbackPricing struct {
	InputMicrousdPerMillionTokens     int64  `json:"input_microusd_per_million_tokens"`
	OutputMicrousdPerMillionTokens    int64  `json:"output_microusd_per_million_tokens"`
	ReasoningMicrousdPerMillionTokens *int64 `json:"reasoning_microusd_per_million_tokens,omitempty"`
}

// Summary captures accounting for one leaf item or one aggregate scope.
type Summary struct {
	Currency                   string     `json:"currency"`
	InputTokens                *int64     `json:"input_tokens,omitempty"`
	OutputTokens               *int64     `json:"output_tokens,omitempty"`
	TotalTokens                *int64     `json:"total_tokens,omitempty"`
	ReasoningTokens            *int64     `json:"reasoning_tokens,omitempty"`
	KnownInputCostMicrousd     *int64     `json:"known_input_cost_microusd,omitempty"`
	KnownOutputCostMicrousd    *int64     `json:"known_output_cost_microusd,omitempty"`
	KnownReasoningCostMicrousd *int64     `json:"known_reasoning_cost_microusd,omitempty"`
	KnownTotalCostMicrousd     *int64     `json:"known_total_cost_microusd,omitempty"`
	TokenSource                Source     `json:"token_source"`
	TokenStatus                Status     `json:"token_status"`
	CostSource                 Source     `json:"cost_source"`
	CostStatus                 Status     `json:"cost_status"`
	PricingKey                 PricingKey `json:"pricing_key"`
	PricingVersion             string     `json:"pricing_version,omitempty"`
	MissingTokenItemCount      int        `json:"missing_token_item_count"`
	MissingCostItemCount       int        `json:"missing_cost_item_count"`
}

// Rollup captures model, direct-subcall, and full-tree totals for one scope.
type Rollup struct {
	ModelTotal          Summary `json:"model_total"`
	DirectSubcallsTotal Summary `json:"direct_subcalls_total"`
	TreeTotal           Summary `json:"tree_total"`
}

// LeafInput defines the raw values needed to construct one leaf Summary.
type LeafInput struct {
	Provider                     string
	Model                        string
	PricingVersion               string
	ReasoningEnabled             bool
	InputTokens                  *int64
	OutputTokens                 *int64
	TotalTokens                  *int64
	ReasoningTokens              *int64
	GatewayInputCostMicrousd     *int64
	GatewayOutputCostMicrousd    *int64
	GatewayReasoningCostMicrousd *int64
	GatewayTotalCostMicrousd     *int64
	FallbackPricing              *FallbackPricing
}
