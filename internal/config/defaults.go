package config

const (
	// DefaultConfigPath is the default application config source.
	DefaultConfigPath = "./sigil.yaml"
	// DefaultRunConfigPath is the default run config source.
	DefaultRunConfigPath = "./sigil-run.yaml"

	// DefaultLogLevel is the baseline log level.
	DefaultLogLevel = "info"
	// DefaultLogDir is the baseline log directory.
	DefaultLogDir = "./sigil/logs"

	// DefaultRunGateway is the default inference gateway.
	DefaultRunGateway = "openrouter"
	// DefaultRunReasoningEnabled is the default reasoning switch.
	DefaultRunReasoningEnabled = true
	// DefaultRunReasoningEffort is the default reasoning effort.
	DefaultRunReasoningEffort = "medium"
	// DefaultRunOpenRouterBaseURL is the default OpenRouter API base URL.
	DefaultRunOpenRouterBaseURL = "https://openrouter.ai/api/v1"
	// DefaultRunOpenRouterRequestTimeoutMS is the default OpenRouter request timeout.
	DefaultRunOpenRouterRequestTimeoutMS = 30000
	// DefaultRunOpenRouterAPIKeyEnv is the default OpenRouter API key env var name.
	DefaultRunOpenRouterAPIKeyEnv = "OPENROUTER_API_KEY"
	// DefaultRunRLMEnabled is the default RLM enable switch.
	DefaultRunRLMEnabled = true
	// DefaultRunRLMMaxDepth is the default RLM max recursion depth.
	DefaultRunRLMMaxDepth = 3
	// DefaultRunGuardrailsMaxStepsPerNode is default per-node step budget.
	DefaultRunGuardrailsMaxStepsPerNode = 64
	// DefaultRunGuardrailsMaxTotalStepsPerRun is default run total step budget.
	DefaultRunGuardrailsMaxTotalStepsPerRun = 256
	// DefaultRunGuardrailsMaxRunDurationMS is default run wall-clock budget.
	DefaultRunGuardrailsMaxRunDurationMS = 1200000
	// DefaultRunGuardrailsMaxConsecutiveStepFailures is default consecutive-failure budget.
	DefaultRunGuardrailsMaxConsecutiveStepFailures = 6
)

// NewDefaultConfig returns a fully populated baseline Config.
func NewDefaultConfig() Config {
	return Config{
		LogLevel: DefaultLogLevel,
		LogDir:   DefaultLogDir,
	}
}

// NewDefaultRunConfig returns a fully populated baseline RunConfig.
func NewDefaultRunConfig() RunConfig {
	return RunConfig{
		SystemPromptAppend: "",
		LLM: RunLLMConfig{
			Gateway: DefaultRunGateway,
			Reasoning: RunReasoningConfig{
				Enabled: DefaultRunReasoningEnabled,
				Effort:  DefaultRunReasoningEffort,
			},
			OpenRouter: RunOpenRouterConfig{
				BaseURL:          DefaultRunOpenRouterBaseURL,
				RequestTimeoutMS: DefaultRunOpenRouterRequestTimeoutMS,
				APIKeyEnv:        DefaultRunOpenRouterAPIKeyEnv,
			},
		},
		RLM: RunRLMConfig{
			Enabled:  DefaultRunRLMEnabled,
			MaxDepth: DefaultRunRLMMaxDepth,
		},
		Guardrails: RunGuardrailsConfig{
			MaxStepsPerNode:            DefaultRunGuardrailsMaxStepsPerNode,
			MaxTotalStepsPerRun:        DefaultRunGuardrailsMaxTotalStepsPerRun,
			MaxRunDurationMS:           DefaultRunGuardrailsMaxRunDurationMS,
			MaxConsecutiveStepFailures: DefaultRunGuardrailsMaxConsecutiveStepFailures,
		},
	}
}
