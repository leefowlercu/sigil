package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	defaults := NewDefaultConfig()
	if defaults.Logs.Level != DefaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", DefaultLogLevel, defaults.Logs.Level)
	}

	if defaults.Logs.Dir != DefaultLogDir {
		t.Fatalf("expected default log dir %q, got %q", DefaultLogDir, defaults.Logs.Dir)
	}
}

func TestInitUsesDefaultsWhenConfigFileIsMissing(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	chdir(t, t.TempDir())

	if err := Init(); err != nil {
		t.Fatalf("expected defaults-only init success, got %v", err)
	}

	cfg, err := Get()
	if err != nil {
		t.Fatalf("expected active config, got %v", err)
	}

	if cfg.Logs.Level != DefaultLogLevel {
		t.Fatalf("expected log level %q, got %q", DefaultLogLevel, cfg.Logs.Level)
	}

	expectedLogDir := mustExpandPath(t, DefaultLogDir)
	if cfg.Logs.Dir != expectedLogDir {
		t.Fatalf("expected log dir %q, got %q", expectedLogDir, cfg.Logs.Dir)
	}
}

func TestInitUsesDefaultsWhenMissingDefaultConfigNameCollidesWithBinary(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	if err := os.WriteFile(filepath.Join(workDir, "sigil"), []byte{0xff, 0xfe, 0xfd}, 0o755); err != nil {
		t.Fatalf("failed to write binary collision file: %v", err)
	}

	if err := Init(); err != nil {
		t.Fatalf("expected defaults-only init success despite binary collision, got %v", err)
	}

	cfg := MustGet()
	if cfg.Logs.Level != DefaultLogLevel {
		t.Fatalf("expected log level %q, got %q", DefaultLogLevel, cfg.Logs.Level)
	}

	expectedLogDir := mustExpandPath(t, DefaultLogDir)
	if cfg.Logs.Dir != expectedLogDir {
		t.Fatalf("expected log dir %q, got %q", expectedLogDir, cfg.Logs.Dir)
	}
}

func TestInitAppliesEnvironmentOverrides(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	chdir(t, t.TempDir())
	t.Setenv("SIGIL_LOGS_LEVEL", "warn")
	t.Setenv("SIGIL_LOGS_DIR", "./env-logs")

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.Logs.Level != "warn" {
		t.Fatalf("expected log level override warn, got %q", cfg.Logs.Level)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.Logs.Dir != expectedLogDir {
		t.Fatalf("expected log dir override %q, got %q", expectedLogDir, cfg.Logs.Dir)
	}
}

func TestInitFromPathUsesEnvironmentPrecedenceOverFile(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("logs:\n  level: debug\n  dir: ./file-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("SIGIL_LOGS_DIR", "./env-logs")

	if err := InitFromPath(configPath); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.Logs.Level != "debug" {
		t.Fatalf("expected file log level debug, got %q", cfg.Logs.Level)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.Logs.Dir != expectedLogDir {
		t.Fatalf("expected env log dir override %q, got %q", expectedLogDir, cfg.Logs.Dir)
	}
}

func TestInitResolvesRelativeLogDirFromWorkingDirectory(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	expected := mustExpandPath(t, DefaultLogDir)
	if cfg.Logs.Dir != expected {
		t.Fatalf("expected resolved log dir %q, got %q", expected, cfg.Logs.Dir)
	}
}

func TestInitRejectsUnsupportedLogLevel(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("logs:\n  level: trace\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := InitFromPath(configPath); err == nil {
		t.Fatal("expected unsupported logs.level validation error")
	}
}

func TestInitFromPathClearsActiveConfigOnValidationFailure(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOGS_LEVEL", "SIGIL_LOGS_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	validPath := filepath.Join(workDir, "valid.yaml")
	if err := os.WriteFile(validPath, []byte("logs:\n  level: info\n  dir: ./valid-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write valid config file: %v", err)
	}

	if err := InitFromPath(validPath); err != nil {
		t.Fatalf("expected initial init success, got %v", err)
	}

	invalidPath := filepath.Join(workDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("logs:\n  level: trace\n"), 0o644); err != nil {
		t.Fatalf("failed to write invalid config file: %v", err)
	}

	if err := InitFromPath(invalidPath); err == nil {
		t.Fatal("expected validation failure for invalid config")
	}

	if _, err := Get(); err == nil {
		t.Fatal("expected config to be cleared after failed initialization")
	}
}

func TestExpandPathResolvesRelativePathsFromWorkingDirectory(t *testing.T) {
	workDir := t.TempDir()
	chdir(t, workDir)

	expanded, err := ExpandPath("./.sigil/logs")
	if err != nil {
		t.Fatalf("expected expansion success, got %v", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	expected := filepath.Clean(filepath.Join(currentDir, "./.sigil/logs"))
	if expanded != expected {
		t.Fatalf("expected expanded path %q, got %q", expected, expanded)
	}
}

func TestExpandPathReturnsAbsolutePathAsIs(t *testing.T) {
	absolutePath := filepath.Join(t.TempDir(), "logs")

	expanded, err := ExpandPath(absolutePath)
	if err != nil {
		t.Fatalf("expected expansion success, got %v", err)
	}

	if expanded != filepath.Clean(absolutePath) {
		t.Fatalf("expected absolute path %q, got %q", absolutePath, expanded)
	}
}

func TestNewDefaultRunConfig(t *testing.T) {
	defaults := NewDefaultRunConfig()

	if defaults.SystemPromptAppend != "" {
		t.Fatalf("expected empty system_prompt_append, got %q", defaults.SystemPromptAppend)
	}

	if defaults.LLM.Gateway != DefaultRunGateway {
		t.Fatalf("expected llm.gateway %q, got %q", DefaultRunGateway, defaults.LLM.Gateway)
	}

	if defaults.LLM.Reasoning.Enabled != DefaultRunReasoningEnabled {
		t.Fatalf("expected llm.reasoning.enabled %t, got %t", DefaultRunReasoningEnabled, defaults.LLM.Reasoning.Enabled)
	}

	if defaults.LLM.Reasoning.Effort != DefaultRunReasoningEffort {
		t.Fatalf("expected llm.reasoning.effort %q, got %q", DefaultRunReasoningEffort, defaults.LLM.Reasoning.Effort)
	}

	if defaults.LLM.OpenRouter.BaseURL != DefaultRunOpenRouterBaseURL {
		t.Fatalf("expected openrouter base_url %q, got %q", DefaultRunOpenRouterBaseURL, defaults.LLM.OpenRouter.BaseURL)
	}

	if defaults.LLM.OpenRouter.RequestTimeoutMS != DefaultRunOpenRouterRequestTimeoutMS {
		t.Fatalf("expected openrouter request_timeout_ms %d, got %d", DefaultRunOpenRouterRequestTimeoutMS, defaults.LLM.OpenRouter.RequestTimeoutMS)
	}

	if defaults.LLM.OpenRouter.APIKeyEnv != DefaultRunOpenRouterAPIKeyEnv {
		t.Fatalf("expected openrouter api_key_env %q, got %q", DefaultRunOpenRouterAPIKeyEnv, defaults.LLM.OpenRouter.APIKeyEnv)
	}

	if defaults.RLM.Enabled != DefaultRunRLMEnabled {
		t.Fatalf("expected rlm.enabled %t, got %t", DefaultRunRLMEnabled, defaults.RLM.Enabled)
	}

	if defaults.RLM.MaxDepth != DefaultRunRLMMaxDepth {
		t.Fatalf("expected rlm.max_depth %d, got %d", DefaultRunRLMMaxDepth, defaults.RLM.MaxDepth)
	}

	if defaults.Guardrails.MaxStepsPerNode != DefaultRunGuardrailsMaxStepsPerNode {
		t.Fatalf("expected guardrails.max_steps_per_node %d, got %d", DefaultRunGuardrailsMaxStepsPerNode, defaults.Guardrails.MaxStepsPerNode)
	}

	if defaults.Guardrails.MaxTotalStepsPerRun != DefaultRunGuardrailsMaxTotalStepsPerRun {
		t.Fatalf("expected guardrails.max_total_steps_per_run %d, got %d", DefaultRunGuardrailsMaxTotalStepsPerRun, defaults.Guardrails.MaxTotalStepsPerRun)
	}

	if defaults.Guardrails.MaxRunDurationMS != DefaultRunGuardrailsMaxRunDurationMS {
		t.Fatalf("expected guardrails.max_run_duration_ms %d, got %d", DefaultRunGuardrailsMaxRunDurationMS, defaults.Guardrails.MaxRunDurationMS)
	}

	if defaults.Guardrails.MaxConsecutiveStepFailures != DefaultRunGuardrailsMaxConsecutiveStepFailures {
		t.Fatalf("expected guardrails.max_consecutive_step_failures %d, got %d", DefaultRunGuardrailsMaxConsecutiveStepFailures, defaults.Guardrails.MaxConsecutiveStepFailures)
	}
	if defaults.Guardrails.MaxTotalTokens != nil {
		t.Fatalf("expected guardrails.max_total_tokens to be unset by default, got %v", defaults.Guardrails.MaxTotalTokens)
	}
	if defaults.Guardrails.MaxTotalCostUSD != nil {
		t.Fatalf("expected guardrails.max_total_cost_usd to be unset by default, got %v", defaults.Guardrails.MaxTotalCostUSD)
	}

	if defaults.Accounting.PricingVersion != DefaultRunAccountingPricingVersion {
		t.Fatalf("expected accounting.pricing_version %q, got %q", DefaultRunAccountingPricingVersion, defaults.Accounting.PricingVersion)
	}
}

func TestInitRunUsesEnvironmentWhenDefaultRunConfigFileIsMissing(t *testing.T) {
	clearActiveRunConfig()
	unsetEnv(
		t,
		"SIGIL_RUN_NAME",
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_LLM_PROVIDER",
		"SIGIL_RUN_LLM_MODEL",
	)
	chdir(t, t.TempDir())

	t.Setenv("SIGIL_RUN_NAME", "env-test-run")
	t.Setenv("SIGIL_RUN_PROMPT", "env prompt")
	t.Setenv("SIGIL_RUN_CONTEXT", "env context")
	t.Setenv("SIGIL_RUN_LLM_PROVIDER", "openai")
	t.Setenv("SIGIL_RUN_LLM_MODEL", "gpt-5.1")

	if err := InitRun(); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg, err := GetRun()
	if err != nil {
		t.Fatalf("expected active run config, got %v", err)
	}

	if cfg.Prompt != "env prompt" {
		t.Fatalf("expected prompt from env, got %q", cfg.Prompt)
	}

	if cfg.Context != "env context" {
		t.Fatalf("expected context from env, got %q", cfg.Context)
	}

	if cfg.LLM.Provider != "openai" {
		t.Fatalf("expected provider openai, got %q", cfg.LLM.Provider)
	}

	if cfg.LLM.Model != "gpt-5.1" {
		t.Fatalf("expected model gpt-5.1, got %q", cfg.LLM.Model)
	}
}

func TestInitRunFromPathAppliesEnvironmentOverridesOverFile(t *testing.T) {
	clearActiveRunConfig()
	unsetEnv(
		t,
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_LLM_PROVIDER",
		"SIGIL_RUN_LLM_MODEL",
	)
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: file prompt\ncontext: file context\nllm:\n  provider: anthropic\n  model: claude-sonnet-4\n",
	)

	t.Setenv("SIGIL_RUN_LLM_PROVIDER", "openai")
	t.Setenv("SIGIL_RUN_LLM_MODEL", "gpt-5.1")

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.LLM.Provider != "openai" {
		t.Fatalf("expected provider override openai, got %q", cfg.LLM.Provider)
	}

	if cfg.LLM.Model != "gpt-5.1" {
		t.Fatalf("expected model override gpt-5.1, got %q", cfg.LLM.Model)
	}
}

func TestInitRunFromPathRejectsMissingProviderOrModel(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "missing provider",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  model: gpt-5.1\n",
		},
		{
			name:    "missing model",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)

			if err := InitRunFromPath(configPath); err == nil {
				t.Fatal("expected run config required-field validation error")
			}
		})
	}
}

func TestInitRunFromPathRejectsPromptAndTemplateExclusivityViolations(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "both prompt fields",
			content: "prompt: prompt\nprompt_template: prompt-template\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
		},
		{
			name:    "neither prompt field",
			content: "context: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)

			if err := InitRunFromPath(configPath); err == nil {
				t.Fatal("expected prompt/prompt_template exclusivity validation error")
			}
		})
	}
}

func TestInitRunFromPathRejectsContextAndTemplateExclusivityViolations(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "both context fields",
			content: "prompt: prompt\ncontext: context\ncontext_template: context-template\nllm:\n  provider: openai\n  model: gpt-5.1\n",
		},
		{
			name:    "neither context field",
			content: "prompt: prompt\nllm:\n  provider: openai\n  model: gpt-5.1\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)

			if err := InitRunFromPath(configPath); err == nil {
				t.Fatal("expected context/context_template exclusivity validation error")
			}
		})
	}
}

func TestInitRunFromPathAppliesGatewayOpenRouterAndRLMDefaults(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
	)

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.LLM.Gateway != DefaultRunGateway {
		t.Fatalf("expected gateway %q, got %q", DefaultRunGateway, cfg.LLM.Gateway)
	}

	if cfg.LLM.OpenRouter.BaseURL != DefaultRunOpenRouterBaseURL {
		t.Fatalf("expected openrouter base_url %q, got %q", DefaultRunOpenRouterBaseURL, cfg.LLM.OpenRouter.BaseURL)
	}

	if cfg.LLM.OpenRouter.RequestTimeoutMS != DefaultRunOpenRouterRequestTimeoutMS {
		t.Fatalf("expected openrouter request_timeout_ms %d, got %d", DefaultRunOpenRouterRequestTimeoutMS, cfg.LLM.OpenRouter.RequestTimeoutMS)
	}

	if cfg.LLM.OpenRouter.APIKeyEnv != DefaultRunOpenRouterAPIKeyEnv {
		t.Fatalf("expected openrouter api_key_env %q, got %q", DefaultRunOpenRouterAPIKeyEnv, cfg.LLM.OpenRouter.APIKeyEnv)
	}

	if cfg.RLM.Enabled != DefaultRunRLMEnabled {
		t.Fatalf("expected rlm.enabled %t, got %t", DefaultRunRLMEnabled, cfg.RLM.Enabled)
	}

	if cfg.RLM.MaxDepth != DefaultRunRLMMaxDepth {
		t.Fatalf("expected rlm.max_depth %d, got %d", DefaultRunRLMMaxDepth, cfg.RLM.MaxDepth)
	}

	if cfg.Guardrails.MaxStepsPerNode != DefaultRunGuardrailsMaxStepsPerNode {
		t.Fatalf("expected guardrails.max_steps_per_node %d, got %d", DefaultRunGuardrailsMaxStepsPerNode, cfg.Guardrails.MaxStepsPerNode)
	}

	if cfg.Guardrails.MaxTotalStepsPerRun != DefaultRunGuardrailsMaxTotalStepsPerRun {
		t.Fatalf("expected guardrails.max_total_steps_per_run %d, got %d", DefaultRunGuardrailsMaxTotalStepsPerRun, cfg.Guardrails.MaxTotalStepsPerRun)
	}

	if cfg.Guardrails.MaxRunDurationMS != DefaultRunGuardrailsMaxRunDurationMS {
		t.Fatalf("expected guardrails.max_run_duration_ms %d, got %d", DefaultRunGuardrailsMaxRunDurationMS, cfg.Guardrails.MaxRunDurationMS)
	}

	if cfg.Guardrails.MaxConsecutiveStepFailures != DefaultRunGuardrailsMaxConsecutiveStepFailures {
		t.Fatalf("expected guardrails.max_consecutive_step_failures %d, got %d", DefaultRunGuardrailsMaxConsecutiveStepFailures, cfg.Guardrails.MaxConsecutiveStepFailures)
	}
	if cfg.Guardrails.MaxTotalTokens != nil {
		t.Fatalf("expected guardrails.max_total_tokens to remain unset by default, got %v", cfg.Guardrails.MaxTotalTokens)
	}
	if cfg.Guardrails.MaxTotalCostUSD != nil {
		t.Fatalf("expected guardrails.max_total_cost_usd to remain unset by default, got %v", cfg.Guardrails.MaxTotalCostUSD)
	}
}

func TestInitRunFromPathRejectsUnsupportedGateway(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  gateway: unsupported\n",
	)

	if err := InitRunFromPath(configPath); err == nil {
		t.Fatal("expected unsupported llm.gateway validation error")
	}
}

func TestInitRunFromPathValidatesProviderAllowList(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: unsupported\n  model: gpt-5.1\n",
	)

	if err := InitRunFromPath(configPath); err == nil {
		t.Fatal("expected unsupported llm.provider validation error")
	}
}

func TestInitRunFromPathValidatesProviderModelAllowListMapping(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "openai allowed",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
			wantErr: false,
		},
		{
			name:    "anthropic allowed",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: anthropic\n  model: claude-sonnet-4\n",
			wantErr: false,
		},
		{
			name:    "openai with anthropic model",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: claude-sonnet-4\n",
			wantErr: true,
		},
		{
			name:    "anthropic with openai model",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: anthropic\n  model: gpt-5.1\n",
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)

			err := InitRunFromPath(configPath)
			if testCase.wantErr && err == nil {
				t.Fatal("expected provider/model allow-list validation error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("expected provider/model validation success, got %v", err)
			}
		})
	}
}

func TestInitRunFromPathValidatesReasoningEffortValues(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
		wantErr bool
	}{
		{
			name:    "minimal",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: minimal\n",
			wantErr: false,
		},
		{
			name:    "low",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: low\n",
			wantErr: false,
		},
		{
			name:    "medium",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: medium\n",
			wantErr: false,
		},
		{
			name:    "high",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: high\n",
			wantErr: false,
		},
		{
			name:    "invalid",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: extreme\n",
			wantErr: true,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)

			err := InitRunFromPath(configPath)
			if testCase.wantErr && err == nil {
				t.Fatal("expected llm.reasoning.effort validation error")
			}
			if !testCase.wantErr && err != nil {
				t.Fatalf("expected llm.reasoning.effort validation success, got %v", err)
			}
		})
	}
}

func TestInitRunFromPathAllowsReasoningEffortWhenReasoningDisabled(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    enabled: false\n    effort: high\n",
	)

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected reasoning-disabled run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.LLM.Reasoning.Enabled {
		t.Fatal("expected llm.reasoning.enabled=false")
	}

	if cfg.LLM.Reasoning.Effort != "high" {
		t.Fatalf("expected llm.reasoning.effort high, got %q", cfg.LLM.Reasoning.Effort)
	}
}

func TestInitRunFromPathClearsActiveRunConfigOnValidationFailure(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	validPath := filepath.Join(workDir, "valid-run.yaml")
	writeRunTestFile(
		t,
		validPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
	)

	if err := InitRunFromPath(validPath); err != nil {
		t.Fatalf("expected initial run config init success, got %v", err)
	}

	invalidPath := filepath.Join(workDir, "invalid-run.yaml")
	writeRunTestFile(
		t,
		invalidPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: claude-sonnet-4\n",
	)

	if err := InitRunFromPath(invalidPath); err == nil {
		t.Fatal("expected validation failure for invalid run config")
	}

	if _, err := GetRun(); err == nil {
		t.Fatal("expected run config to be cleared after failed initialization")
	}
}

func TestInitRunFromPathReturnsErrorWhenConfigFileIsMissing(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	if err := InitRunFromPath(filepath.Join(workDir, "missing-run.yaml")); err == nil {
		t.Fatal("expected missing explicit run config path error")
	}
}

func TestInitRunFromPathAppliesGuardrailEnvironmentOverrides(t *testing.T) {
	clearActiveRunConfig()
	unsetEnv(
		t,
		"SIGIL_RUN_GUARDRAILS_MAX_STEPS_PER_NODE",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_STEPS_PER_RUN",
		"SIGIL_RUN_GUARDRAILS_MAX_RUN_DURATION_MS",
		"SIGIL_RUN_GUARDRAILS_MAX_CONSECUTIVE_STEP_FAILURES",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_TOKENS",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_COST_USD",
	)
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 10\n  max_total_steps_per_run: 20\n  max_run_duration_ms: 30000\n  max_consecutive_step_failures: 2\n  max_total_tokens: 99\n  max_total_cost_usd: \"0.25\"\n",
	)

	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_STEPS_PER_NODE", "64")
	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_TOTAL_STEPS_PER_RUN", "256")
	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_RUN_DURATION_MS", "1200000")
	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_CONSECUTIVE_STEP_FAILURES", "6")
	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_TOTAL_TOKENS", "512")
	t.Setenv("SIGIL_RUN_GUARDRAILS_MAX_TOTAL_COST_USD", "001.230000")

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.Guardrails.MaxStepsPerNode != 64 {
		t.Fatalf("expected guardrails.max_steps_per_node 64, got %d", cfg.Guardrails.MaxStepsPerNode)
	}
	if cfg.Guardrails.MaxTotalStepsPerRun != 256 {
		t.Fatalf("expected guardrails.max_total_steps_per_run 256, got %d", cfg.Guardrails.MaxTotalStepsPerRun)
	}
	if cfg.Guardrails.MaxRunDurationMS != 1200000 {
		t.Fatalf("expected guardrails.max_run_duration_ms 1200000, got %d", cfg.Guardrails.MaxRunDurationMS)
	}
	if cfg.Guardrails.MaxConsecutiveStepFailures != 6 {
		t.Fatalf("expected guardrails.max_consecutive_step_failures 6, got %d", cfg.Guardrails.MaxConsecutiveStepFailures)
	}
	if cfg.Guardrails.MaxTotalTokens == nil || *cfg.Guardrails.MaxTotalTokens != 512 {
		t.Fatalf("expected guardrails.max_total_tokens 512, got %+v", cfg.Guardrails.MaxTotalTokens)
	}
	if cfg.Guardrails.MaxTotalCostUSD == nil || *cfg.Guardrails.MaxTotalCostUSD != "1.23" {
		t.Fatalf("expected guardrails.max_total_cost_usd canonicalized to %q, got %+v", "1.23", cfg.Guardrails.MaxTotalCostUSD)
	}
}

func TestInitRunFromPathRejectsInvalidGuardrailValues(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "max steps per node non-positive",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 0\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 1\n",
		},
		{
			name:    "max total steps per run non-positive",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 0\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 1\n",
		},
		{
			name:    "max run duration non-positive",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 0\n  max_consecutive_step_failures: 1\n",
		},
		{
			name:    "max consecutive failures non-positive",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 0\n",
		},
		{
			name:    "max total tokens non-positive",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 1\n  max_total_tokens: 0\n",
		},
		{
			name:    "max total cost usd malformed",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 1\n  max_total_cost_usd: \"1.2345678\"\n",
		},
		{
			name:    "max total cost usd numeric instead of string",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 1\n  max_total_steps_per_run: 1\n  max_run_duration_ms: 1\n  max_consecutive_step_failures: 1\n  max_total_cost_usd: 1.25\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			configPath := filepath.Join(workDir, testCase.name+".yaml")
			writeRunTestFile(t, configPath, testCase.content)
			if err := InitRunFromPath(configPath); err == nil {
				t.Fatal("expected guardrail validation failure")
			}
		})
	}
}

func TestInitRunFromPathCanonicalizesGuardrailCostBudgetFromFile(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 10\n  max_total_steps_per_run: 20\n  max_run_duration_ms: 30000\n  max_consecutive_step_failures: 2\n  max_total_tokens: 321\n  max_total_cost_usd: \"00012.340000\"\n",
	)

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.Guardrails.MaxTotalTokens == nil || *cfg.Guardrails.MaxTotalTokens != 321 {
		t.Fatalf("expected guardrails.max_total_tokens 321, got %+v", cfg.Guardrails.MaxTotalTokens)
	}
	if cfg.Guardrails.MaxTotalCostUSD == nil || *cfg.Guardrails.MaxTotalCostUSD != "12.34" {
		t.Fatalf("expected guardrails.max_total_cost_usd canonicalized to %q, got %+v", "12.34", cfg.Guardrails.MaxTotalCostUSD)
	}
}

func TestInitRunFromPathAllowsRunTotalStepBudgetBelowPerNodeBudget(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\nguardrails:\n  max_steps_per_node: 10\n  max_total_steps_per_run: 5\n  max_run_duration_ms: 1000\n  max_consecutive_step_failures: 2\n",
	)

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.Guardrails.MaxStepsPerNode != 10 {
		t.Fatalf("expected guardrails.max_steps_per_node 10, got %d", cfg.Guardrails.MaxStepsPerNode)
	}
	if cfg.Guardrails.MaxTotalStepsPerRun != 5 {
		t.Fatalf("expected guardrails.max_total_steps_per_run 5, got %d", cfg.Guardrails.MaxTotalStepsPerRun)
	}
}

func TestInitRunFromPathAppliesAccountingFallbackPricingEnvironmentOverrides(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)
	unsetEnv(
		t,
		"SIGIL_RUN_ACCOUNTING_PRICING_VERSION",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_INPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_OUTPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_REASONING_MICROUSD_PER_MILLION_TOKENS",
	)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\naccounting:\n  pricing_version: file-v1\n",
	)

	t.Setenv("SIGIL_RUN_ACCOUNTING_PRICING_VERSION", "env-v2")
	t.Setenv("SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_INPUT_MICROUSD_PER_MILLION_TOKENS", "111")
	t.Setenv("SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_OUTPUT_MICROUSD_PER_MILLION_TOKENS", "222")
	t.Setenv("SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_REASONING_MICROUSD_PER_MILLION_TOKENS", "333")

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.Accounting.PricingVersion != "env-v2" {
		t.Fatalf("expected accounting.pricing_version override %q, got %q", "env-v2", cfg.Accounting.PricingVersion)
	}

	pricing, ok := cfg.Accounting.PricingFor("openai", "gpt-5.1")
	if !ok {
		t.Fatalf("expected fallback pricing for openai/gpt-5.1")
	}
	if pricing.InputMicrousdPerMillionTokens != 111 {
		t.Fatalf("expected input rate 111, got %d", pricing.InputMicrousdPerMillionTokens)
	}
	if pricing.OutputMicrousdPerMillionTokens != 222 {
		t.Fatalf("expected output rate 222, got %d", pricing.OutputMicrousdPerMillionTokens)
	}
	if pricing.ReasoningMicrousdPerMillionTokens == nil || *pricing.ReasoningMicrousdPerMillionTokens != 333 {
		t.Fatalf("expected reasoning rate 333, got %+v", pricing.ReasoningMicrousdPerMillionTokens)
	}
}

func TestInitRunFromPathDecodesAccountingFallbackPricingFromFileKeysWithDots(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\naccounting:\n  pricing_version: custom-v1\n  fallback_pricing:\n    openai:\n      gpt-5.1:\n        input_microusd_per_million_tokens: 111\n        output_microusd_per_million_tokens: 222\n        reasoning_microusd_per_million_tokens: 333\n",
	)

	if err := InitRunFromPath(configPath); err != nil {
		t.Fatalf("expected run config init success, got %v", err)
	}

	cfg := MustGetRun()
	if cfg.Accounting.PricingVersion != "custom-v1" {
		t.Fatalf("expected pricing version custom-v1, got %q", cfg.Accounting.PricingVersion)
	}
	pricing, ok := cfg.Accounting.PricingFor("openai", "gpt-5.1")
	if !ok {
		t.Fatalf("expected fallback pricing for openai/gpt-5.1")
	}
	if pricing.InputMicrousdPerMillionTokens != 111 || pricing.OutputMicrousdPerMillionTokens != 222 {
		t.Fatalf("unexpected fallback pricing %+v", pricing)
	}
	if pricing.ReasoningMicrousdPerMillionTokens == nil || *pricing.ReasoningMicrousdPerMillionTokens != 333 {
		t.Fatalf("expected reasoning rate 333, got %+v", pricing.ReasoningMicrousdPerMillionTokens)
	}
}

func TestInitRunFromPathRejectsInvalidAccountingFallbackPricingValues(t *testing.T) {
	testCases := []struct {
		name    string
		content string
	}{
		{
			name:    "zero input rate",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\naccounting:\n  fallback_pricing:\n    openai:\n      gpt-5.1:\n        input_microusd_per_million_tokens: 0\n        output_microusd_per_million_tokens: 1\n",
		},
		{
			name:    "negative reasoning rate",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\naccounting:\n  fallback_pricing:\n    openai:\n      gpt-5.1:\n        input_microusd_per_million_tokens: 1\n        output_microusd_per_million_tokens: 2\n        reasoning_microusd_per_million_tokens: -3\n",
		},
		{
			name:    "fractional input rate",
			content: "name: test-run\nprompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\naccounting:\n  fallback_pricing:\n    openai:\n      gpt-5.1:\n        input_microusd_per_million_tokens: 1.5\n        output_microusd_per_million_tokens: 2\n",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			clearActiveRunConfig()
			workDir := t.TempDir()
			chdir(t, workDir)

			configPath := filepath.Join(workDir, "sigil-run.yaml")
			writeRunTestFile(t, configPath, testCase.content)

			if err := InitRunFromPath(configPath); err == nil {
				t.Fatalf("expected invalid accounting fallback pricing error")
			}
		})
	}
}

func TestExampleConfigFilesRemainValid(t *testing.T) {
	clearActiveConfig()
	clearActiveRunConfig()
	unsetEnv(
		t,
		"SIGIL_LOGS_LEVEL",
		"SIGIL_LOGS_DIR",
		"SIGIL_APP_SERVER_INSTANCE_NAME",
		"SIGIL_APP_SERVER_INSTANCE_ID",
		"SIGIL_APP_SERVER_RUN_DIR",
		"SIGIL_APP_SERVER_ALLOWED_ORIGINS",
		"SIGIL_APP_SERVER_WEBSOCKET_LISTEN_ADDR",
		"SIGIL_APP_SERVER_WEBSOCKET_PATH",
		"SIGIL_APP_SERVER_HEALTH_READY_PATH",
		"SIGIL_APP_SERVER_HEALTH_LIVE_PATH",
		"SIGIL_APP_SERVER_SUBSCRIPTIONS_POLL_INTERVAL_MS",
		"SIGIL_APP_SERVER_LIMITS_MAX_CONNECTIONS",
		"SIGIL_APP_SERVER_LIMITS_MAX_FRAME_BYTES",
		"SIGIL_RUN_SYSTEM_PROMPT_APPEND",
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_PROMPT_TEMPLATE",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_CONTEXT_TEMPLATE",
		"SIGIL_RUN_LLM_PROVIDER",
		"SIGIL_RUN_LLM_MODEL",
		"SIGIL_RUN_LLM_GATEWAY",
		"SIGIL_RUN_LLM_REASONING_ENABLED",
		"SIGIL_RUN_LLM_REASONING_EFFORT",
		"SIGIL_RUN_LLM_OPENROUTER_BASE_URL",
		"SIGIL_RUN_LLM_OPENROUTER_REQUEST_TIMEOUT_MS",
		"SIGIL_RUN_LLM_OPENROUTER_API_KEY_ENV",
		"SIGIL_RUN_RLM_ENABLED",
		"SIGIL_RUN_RLM_MAX_DEPTH",
		"SIGIL_RUN_GUARDRAILS_MAX_STEPS_PER_NODE",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_STEPS_PER_RUN",
		"SIGIL_RUN_GUARDRAILS_MAX_RUN_DURATION_MS",
		"SIGIL_RUN_GUARDRAILS_MAX_CONSECUTIVE_STEP_FAILURES",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_TOKENS",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_COST_USD",
		"SIGIL_RUN_ACCOUNTING_PRICING_VERSION",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_3_CODEX_INPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_3_CODEX_OUTPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_3_CODEX_REASONING_MICROUSD_PER_MILLION_TOKENS",
	)

	repoRoot := mustAbsPath(t, filepath.Join("..", ".."))
	applicationExamplePath := filepath.Join(repoRoot, "sigil.yaml.example")
	runExamplePath := filepath.Join(repoRoot, "sigil-run.yaml.example")
	workDir := t.TempDir()
	chdir(t, workDir)

	applicationExampleContent, err := os.ReadFile(applicationExamplePath)
	if err != nil {
		t.Fatalf("failed to read application example config: %v", err)
	}

	applicationConfigPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(applicationConfigPath, applicationExampleContent, 0o644); err != nil {
		t.Fatalf("failed to write temporary application config: %v", err)
	}

	if err := InitFromPath(applicationConfigPath); err != nil {
		t.Fatalf("expected application example config to validate, got %v", err)
	}

	runExampleContent, err := os.ReadFile(runExamplePath)
	if err != nil {
		t.Fatalf("failed to read run example config: %v", err)
	}

	runConfigPath := filepath.Join(workDir, "sigil-run.yaml")
	if err := os.WriteFile(runConfigPath, runExampleContent, 0o644); err != nil {
		t.Fatalf("failed to write temporary run config: %v", err)
	}

	if err := InitRunFromPath(runConfigPath); err != nil {
		t.Fatalf("expected run example config to validate, got %v", err)
	}

	applicationConfig := MustGet()
	if applicationConfig.AppServer.WebSocket.Path != "/app-server" {
		t.Fatalf("expected websocket path %q, got %q", "/app-server", applicationConfig.AppServer.WebSocket.Path)
	}

	runConfig := MustGetRun()
	if runConfig.LLM.Provider != "openai" {
		t.Fatalf("expected llm.provider %q, got %q", "openai", runConfig.LLM.Provider)
	}
	if runConfig.LLM.Model != "gpt-5.3-codex" {
		t.Fatalf("expected llm.model %q, got %q", "gpt-5.3-codex", runConfig.LLM.Model)
	}
	if runConfig.Guardrails.MaxTotalCostUSD == nil || *runConfig.Guardrails.MaxTotalCostUSD != "5" {
		t.Fatalf("expected guardrails.max_total_cost_usd canonicalized to %q, got %+v", "5", runConfig.Guardrails.MaxTotalCostUSD)
	}
}

func chdir(t *testing.T, dir string) {
	t.Helper()

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}

	if err := os.Chdir(dir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}

	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})
}

func unsetEnv(t *testing.T, keys ...string) {
	t.Helper()

	originalValues := make(map[string]*string, len(keys))
	for _, key := range keys {
		if value, ok := os.LookupEnv(key); ok {
			copied := value
			originalValues[key] = &copied
		} else {
			originalValues[key] = nil
		}

		if err := os.Unsetenv(key); err != nil {
			t.Fatalf("failed to unset env %s: %v", key, err)
		}
	}

	t.Cleanup(func() {
		for key, value := range originalValues {
			if value == nil {
				_ = os.Unsetenv(key)
				continue
			}

			_ = os.Setenv(key, *value)
		}
	})
}

func writeRunTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func mustExpandPath(t *testing.T, path string) string {
	t.Helper()

	expanded, err := ExpandPath(path)
	if err != nil {
		t.Fatalf("failed to expand %q: %v", path, err)
	}

	return expanded
}

func mustAbsPath(t *testing.T, path string) string {
	t.Helper()

	absolutePath, err := filepath.Abs(path)
	if err != nil {
		t.Fatalf("failed to resolve absolute path for %q: %v", path, err)
	}

	return absolutePath
}
