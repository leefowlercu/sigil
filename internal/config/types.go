package config

// Config is the typed application configuration contract for sigil.
type Config struct {
	LogLevel string `yaml:"log_level" mapstructure:"log_level"`
	LogDir   string `yaml:"log_dir" mapstructure:"log_dir"`
}

// RunConfig is the typed run configuration contract for sigil.
type RunConfig struct {
	SystemPromptAppend string       `yaml:"system_prompt_append" mapstructure:"system_prompt_append"`
	Prompt             string       `yaml:"prompt" mapstructure:"prompt"`
	PromptTemplate     string       `yaml:"prompt_template" mapstructure:"prompt_template"`
	Context            string       `yaml:"context" mapstructure:"context"`
	ContextTemplate    string       `yaml:"context_template" mapstructure:"context_template"`
	LLM                RunLLMConfig `yaml:"llm" mapstructure:"llm"`
	RLM                RunRLMConfig `yaml:"rlm" mapstructure:"rlm"`
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
