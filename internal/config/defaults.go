package config

const (
	// DefaultConfigPath is the default application config source.
	DefaultConfigPath = "./sigil.yaml"
	// DefaultRunConfigPath is the default run config source.
	DefaultRunConfigPath = "./sigil-run.yaml"

	// DefaultLogLevel is the baseline log level.
	DefaultLogLevel = "info"
	// DefaultLogDir is the baseline log directory.
	DefaultLogDir = "./.sigil/logs"
	// DefaultAppServerInstanceName is the default operator-facing app-server name.
	DefaultAppServerInstanceName = "sigil-local"
	// DefaultAppServerInstanceID is the default machine-readable app-server identity.
	DefaultAppServerInstanceID = "sigil-local"
	// DefaultAppServerRunDir is the default app-server run corpus directory.
	DefaultAppServerRunDir = "./.sigil/runs"
	// DefaultAppServerWebSocketListenAddr is the default WebSocket listen address.
	DefaultAppServerWebSocketListenAddr = "127.0.0.1:8765"
	// DefaultAppServerWebSocketPath is the default WebSocket path.
	DefaultAppServerWebSocketPath = "/app-server"
	// DefaultAppServerHealthReadyPath is the default readiness path.
	DefaultAppServerHealthReadyPath = "/readyz"
	// DefaultAppServerHealthLivePath is the default liveness path.
	DefaultAppServerHealthLivePath = "/healthz"
	// DefaultAppServerSubscriptionsPollIntervalMS is the default polling cadence.
	DefaultAppServerSubscriptionsPollIntervalMS = 500
	// DefaultAppServerLimitsMaxConnections is the default soft connection limit.
	DefaultAppServerLimitsMaxConnections = 50
	// DefaultAppServerLimitsMaxFrameBytes is the default inbound payload limit.
	DefaultAppServerLimitsMaxFrameBytes = 1048576

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
	// DefaultRunAccountingPricingVersion is the default accounting pricing version.
	DefaultRunAccountingPricingVersion = "v1"
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
		Logs: LogsConfig{
			Level: DefaultLogLevel,
			Dir:   DefaultLogDir,
		},
		AppServer: AppServerConfig{
			InstanceName:   DefaultAppServerInstanceName,
			InstanceID:     DefaultAppServerInstanceID,
			RunDir:         DefaultAppServerRunDir,
			AllowedOrigins: []string{},
			WebSocket: AppServerWebSocketConfig{
				ListenAddr: DefaultAppServerWebSocketListenAddr,
				Path:       DefaultAppServerWebSocketPath,
			},
			Health: AppServerHealthConfig{
				ReadyPath: DefaultAppServerHealthReadyPath,
				LivePath:  DefaultAppServerHealthLivePath,
			},
			Subscriptions: AppServerSubscriptionsConfig{
				PollIntervalMS: DefaultAppServerSubscriptionsPollIntervalMS,
			},
			Limits: AppServerLimitsConfig{
				MaxConnections: DefaultAppServerLimitsMaxConnections,
				MaxFrameBytes:  DefaultAppServerLimitsMaxFrameBytes,
			},
		},
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
		Accounting: RunAccountingConfig{
			PricingVersion:  DefaultRunAccountingPricingVersion,
			FallbackPricing: map[string]map[string]RunAccountingFallbackModelConfig{},
		},
		Guardrails: RunGuardrailsConfig{
			MaxStepsPerNode:            DefaultRunGuardrailsMaxStepsPerNode,
			MaxTotalStepsPerRun:        DefaultRunGuardrailsMaxTotalStepsPerRun,
			MaxRunDurationMS:           DefaultRunGuardrailsMaxRunDurationMS,
			MaxConsecutiveStepFailures: DefaultRunGuardrailsMaxConsecutiveStepFailures,
		},
	}
}
