package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewDefaultConfig(t *testing.T) {
	defaults := NewDefaultConfig()
	if defaults.LogLevel != DefaultLogLevel {
		t.Fatalf("expected default log level %q, got %q", DefaultLogLevel, defaults.LogLevel)
	}

	if defaults.LogDir != DefaultLogDir {
		t.Fatalf("expected default log dir %q, got %q", DefaultLogDir, defaults.LogDir)
	}
}

func TestInitUsesDefaultsWhenConfigFileIsMissing(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	chdir(t, t.TempDir())

	if err := Init(); err != nil {
		t.Fatalf("expected defaults-only init success, got %v", err)
	}

	cfg, err := Get()
	if err != nil {
		t.Fatalf("expected active config, got %v", err)
	}

	if cfg.LogLevel != DefaultLogLevel {
		t.Fatalf("expected log level %q, got %q", DefaultLogLevel, cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, DefaultLogDir)
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected log dir %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitAppliesEnvironmentOverrides(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	chdir(t, t.TempDir())
	t.Setenv("SIGIL_LOG_LEVEL", "warn")
	t.Setenv("SIGIL_LOG_DIR", "./env-logs")

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.LogLevel != "warn" {
		t.Fatalf("expected log level override warn, got %q", cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected log dir override %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitFromPathUsesEnvironmentPrecedenceOverFile(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("log_level: debug\nlog_dir: ./file-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	t.Setenv("SIGIL_LOG_DIR", "./env-logs")

	if err := InitFromPath(configPath); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	if cfg.LogLevel != "debug" {
		t.Fatalf("expected file log level debug, got %q", cfg.LogLevel)
	}

	expectedLogDir := mustExpandPath(t, "./env-logs")
	if cfg.LogDir != expectedLogDir {
		t.Fatalf("expected env log dir override %q, got %q", expectedLogDir, cfg.LogDir)
	}
}

func TestInitResolvesRelativeLogDirFromWorkingDirectory(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	if err := Init(); err != nil {
		t.Fatalf("expected init success, got %v", err)
	}

	cfg := MustGet()
	expected := mustExpandPath(t, DefaultLogDir)
	if cfg.LogDir != expected {
		t.Fatalf("expected resolved log dir %q, got %q", expected, cfg.LogDir)
	}
}

func TestInitRejectsUnsupportedLogLevel(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil.yaml")
	if err := os.WriteFile(configPath, []byte("log_level: trace\n"), 0o644); err != nil {
		t.Fatalf("failed to write config file: %v", err)
	}

	if err := InitFromPath(configPath); err == nil {
		t.Fatal("expected unsupported log_level validation error")
	}
}

func TestInitFromPathClearsActiveConfigOnValidationFailure(t *testing.T) {
	clearActiveConfig()
	unsetEnv(t, "SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR")
	workDir := t.TempDir()
	chdir(t, workDir)

	validPath := filepath.Join(workDir, "valid.yaml")
	if err := os.WriteFile(validPath, []byte("log_level: info\nlog_dir: ./valid-logs\n"), 0o644); err != nil {
		t.Fatalf("failed to write valid config file: %v", err)
	}

	if err := InitFromPath(validPath); err != nil {
		t.Fatalf("expected initial init success, got %v", err)
	}

	invalidPath := filepath.Join(workDir, "invalid.yaml")
	if err := os.WriteFile(invalidPath, []byte("log_level: trace\n"), 0o644); err != nil {
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

	expanded, err := ExpandPath("./sigil/logs")
	if err != nil {
		t.Fatalf("expected expansion success, got %v", err)
	}

	currentDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	expected := filepath.Clean(filepath.Join(currentDir, "./sigil/logs"))
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
}

func TestInitRunUsesEnvironmentWhenDefaultRunConfigFileIsMissing(t *testing.T) {
	clearActiveRunConfig()
	unsetEnv(
		t,
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_LLM_PROVIDER",
		"SIGIL_RUN_LLM_MODEL",
	)
	chdir(t, t.TempDir())

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
		"prompt: file prompt\ncontext: file context\nllm:\n  provider: anthropic\n  model: claude-sonnet-4\n",
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
			content: "prompt: prompt\ncontext: context\nllm:\n  model: gpt-5.1\n",
		},
		{
			name:    "missing model",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n",
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
		"prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
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
}

func TestInitRunFromPathRejectsUnsupportedGateway(t *testing.T) {
	clearActiveRunConfig()
	workDir := t.TempDir()
	chdir(t, workDir)

	configPath := filepath.Join(workDir, "sigil-run.yaml")
	writeRunTestFile(
		t,
		configPath,
		"prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  gateway: unsupported\n",
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
		"prompt: prompt\ncontext: context\nllm:\n  provider: unsupported\n  model: gpt-5.1\n",
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
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
			wantErr: false,
		},
		{
			name:    "anthropic allowed",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: anthropic\n  model: claude-sonnet-4\n",
			wantErr: false,
		},
		{
			name:    "openai with anthropic model",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: claude-sonnet-4\n",
			wantErr: true,
		},
		{
			name:    "anthropic with openai model",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: anthropic\n  model: gpt-5.1\n",
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
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: minimal\n",
			wantErr: false,
		},
		{
			name:    "low",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: low\n",
			wantErr: false,
		},
		{
			name:    "medium",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: medium\n",
			wantErr: false,
		},
		{
			name:    "high",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: high\n",
			wantErr: false,
		},
		{
			name:    "invalid",
			content: "prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    effort: extreme\n",
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
		"prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n  reasoning:\n    enabled: false\n    effort: high\n",
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
		"prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: gpt-5.1\n",
	)

	if err := InitRunFromPath(validPath); err != nil {
		t.Fatalf("expected initial run config init success, got %v", err)
	}

	invalidPath := filepath.Join(workDir, "invalid-run.yaml")
	writeRunTestFile(
		t,
		invalidPath,
		"prompt: prompt\ncontext: context\nllm:\n  provider: openai\n  model: claude-sonnet-4\n",
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
