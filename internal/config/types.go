package config

import "strings"

// Config is the typed application configuration contract for sigil.
type Config struct {
	Logs      LogsConfig      `yaml:"logs" mapstructure:"logs"`
	AppServer AppServerConfig `yaml:"app_server" mapstructure:"app_server"`
}

// LogsConfig defines application logging configuration.
type LogsConfig struct {
	Level string `yaml:"level" mapstructure:"level"`
	Dir   string `yaml:"dir" mapstructure:"dir"`
}

// AppServerConfig defines application-level app-server behavior.
type AppServerConfig struct {
	InstanceName   string                       `yaml:"instance_name" mapstructure:"instance_name"`
	InstanceID     string                       `yaml:"instance_id" mapstructure:"instance_id"`
	RunDir         string                       `yaml:"run_dir" mapstructure:"run_dir"`
	AllowedOrigins []string                     `yaml:"allowed_origins" mapstructure:"allowed_origins"`
	WebSocket      AppServerWebSocketConfig     `yaml:"websocket" mapstructure:"websocket"`
	Health         AppServerHealthConfig        `yaml:"health" mapstructure:"health"`
	Subscriptions  AppServerSubscriptionsConfig `yaml:"subscriptions" mapstructure:"subscriptions"`
	Limits         AppServerLimitsConfig        `yaml:"limits" mapstructure:"limits"`
}

// AppServerWebSocketConfig defines WebSocket listener defaults.
type AppServerWebSocketConfig struct {
	ListenAddr string `yaml:"listen_addr" mapstructure:"listen_addr"`
	Path       string `yaml:"path" mapstructure:"path"`
}

// AppServerHealthConfig defines HTTP health endpoint paths.
type AppServerHealthConfig struct {
	ReadyPath string `yaml:"ready_path" mapstructure:"ready_path"`
	LivePath  string `yaml:"live_path" mapstructure:"live_path"`
}

// AppServerSubscriptionsConfig defines subscription polling defaults.
type AppServerSubscriptionsConfig struct {
	PollIntervalMS int `yaml:"poll_interval_ms" mapstructure:"poll_interval_ms"`
}

// AppServerLimitsConfig defines transport safety limits.
type AppServerLimitsConfig struct {
	MaxConnections int `yaml:"max_connections" mapstructure:"max_connections"`
	MaxFrameBytes  int `yaml:"max_frame_bytes" mapstructure:"max_frame_bytes"`
}

// RunConfig is the typed run configuration contract for sigil.
type RunConfig struct {
	Name               string              `yaml:"name" mapstructure:"name"`
	SystemPromptAppend string              `yaml:"system_prompt_append" mapstructure:"system_prompt_append"`
	Prompt             string              `yaml:"prompt" mapstructure:"prompt"`
	PromptTemplate     string              `yaml:"prompt_template" mapstructure:"prompt_template"`
	Context            string              `yaml:"context" mapstructure:"context"`
	ContextTemplate    string              `yaml:"context_template" mapstructure:"context_template"`
	LLM                RunLLMConfig        `yaml:"llm" mapstructure:"llm"`
	RLM                RunRLMConfig        `yaml:"rlm" mapstructure:"rlm"`
	Guardrails         RunGuardrailsConfig `yaml:"guardrails" mapstructure:"guardrails"`
	Accounting         RunAccountingConfig `yaml:"accounting" mapstructure:"accounting"`
}

// RunLLMConfig defines LLM-related run configuration.
type RunLLMConfig struct {
	Provider   string              `yaml:"provider" mapstructure:"provider"`
	Model      string              `yaml:"model" mapstructure:"model"`
	Gateway    string              `yaml:"gateway" mapstructure:"gateway"`
	Reasoning  RunReasoningConfig  `yaml:"reasoning" mapstructure:"reasoning"`
	OpenRouter RunOpenRouterConfig `yaml:"openrouter" mapstructure:"openrouter"`
}

// RunReasoningConfig defines reasoning controls for inference requests.
type RunReasoningConfig struct {
	Enabled bool   `yaml:"enabled" mapstructure:"enabled"`
	Effort  string `yaml:"effort" mapstructure:"effort"`
}

// RunOpenRouterConfig defines OpenRouter transport settings.
type RunOpenRouterConfig struct {
	BaseURL          string `yaml:"base_url" mapstructure:"base_url"`
	RequestTimeoutMS int    `yaml:"request_timeout_ms" mapstructure:"request_timeout_ms"`
	APIKeyEnv        string `yaml:"api_key_env" mapstructure:"api_key_env"`
}

// RunRLMConfig defines recursive language model controls.
type RunRLMConfig struct {
	Enabled  bool `yaml:"enabled" mapstructure:"enabled"`
	MaxDepth int  `yaml:"max_depth" mapstructure:"max_depth"`
}

// RunGuardrailsConfig defines deterministic harness runtime guardrails.
type RunGuardrailsConfig struct {
	MaxStepsPerNode            int     `yaml:"max_steps_per_node" mapstructure:"max_steps_per_node"`
	MaxTotalStepsPerRun        int     `yaml:"max_total_steps_per_run" mapstructure:"max_total_steps_per_run"`
	MaxRunDurationMS           int     `yaml:"max_run_duration_ms" mapstructure:"max_run_duration_ms"`
	MaxConsecutiveStepFailures int     `yaml:"max_consecutive_step_failures" mapstructure:"max_consecutive_step_failures"`
	MaxTotalTokens             *int64  `yaml:"max_total_tokens" mapstructure:"max_total_tokens"`
	MaxTotalCostUSD            *string `yaml:"max_total_cost_usd" mapstructure:"max_total_cost_usd"`
}

// RunAccountingConfig defines accounting policy and fallback pricing.
type RunAccountingConfig struct {
	PricingVersion  string                                                 `yaml:"pricing_version" mapstructure:"pricing_version"`
	FallbackPricing map[string]map[string]RunAccountingFallbackModelConfig `yaml:"fallback_pricing" mapstructure:"fallback_pricing"`
}

// RunAccountingFallbackModelConfig defines fallback pricing for one provider/model pair.
type RunAccountingFallbackModelConfig struct {
	InputMicrousdPerMillionTokens     int64  `yaml:"input_microusd_per_million_tokens" mapstructure:"input_microusd_per_million_tokens"`
	OutputMicrousdPerMillionTokens    int64  `yaml:"output_microusd_per_million_tokens" mapstructure:"output_microusd_per_million_tokens"`
	ReasoningMicrousdPerMillionTokens *int64 `yaml:"reasoning_microusd_per_million_tokens" mapstructure:"reasoning_microusd_per_million_tokens"`
}

// PricingFor returns fallback pricing for one provider/model pair when configured.
func (c RunAccountingConfig) PricingFor(provider string, model string) (*RunAccountingFallbackModelConfig, bool) {
	providerPricing, ok := c.FallbackPricing[strings.TrimSpace(provider)]
	if !ok {
		return nil, false
	}
	modelPricing, ok := providerPricing[strings.TrimSpace(model)]
	if !ok {
		return nil, false
	}
	cloned := modelPricing
	if modelPricing.ReasoningMicrousdPerMillionTokens != nil {
		value := *modelPricing.ReasoningMicrousdPerMillionTokens
		cloned.ReasoningMicrousdPerMillionTokens = &value
	}
	return &cloned, true
}
