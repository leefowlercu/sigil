package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"

	"github.com/cucumber/godog"
	"github.com/leefowlercu/sigil/cmd"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/logging"
)

type harnessWorld struct {
	workingDir            string
	originalWorkingDir    string
	lastStdout            string
	lastStderr            string
	lastErr               error
	lastExitCode          int
	configInitErr         error
	loggingInitErr        error
	runConfigInitErr      error
	resolvedAppConfigPath string
	resolvedRunConfigPath string
}

// InitializeScenario wires all acceptance steps for harness.feature.
func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &harnessWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		_ = logging.Close()

		workDir, err := os.MkdirTemp("", "sigil-acceptance-*")
		if err != nil {
			return ctx, err
		}

		world.workingDir = workDir

		originalDir, err := os.Getwd()
		if err != nil {
			return ctx, err
		}
		world.originalWorkingDir = originalDir

		if err := os.Chdir(workDir); err != nil {
			return ctx, err
		}

		world.lastStdout = ""
		world.lastStderr = ""
		world.lastErr = nil
		world.lastExitCode = 0
		world.configInitErr = nil
		world.loggingInitErr = nil
		world.runConfigInitErr = nil
		world.resolvedAppConfigPath = config.DefaultConfigPath
		world.resolvedRunConfigPath = config.DefaultRunConfigPath

		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		_ = logging.Close()

		if world.originalWorkingDir != "" {
			_ = os.Chdir(world.originalWorkingDir)
		}
		if world.workingDir != "" {
			_ = os.RemoveAll(world.workingDir)
		}

		return ctx, nil
	})

	ctx.Step(`^a clean sigil working directory$`, world.aCleanSigilWorkingDirectory)
	ctx.Step(`^SIGIL configuration environment variables are cleared$`, world.sigilConfigEnvironmentVariablesAreCleared)
	ctx.Step(`^the sigil application starts without an explicit application config path$`, world.theSigilApplicationStartsWithoutAnExplicitApplicationConfigPath)
	ctx.Step(`^the sigil application starts without an explicit run config path$`, world.theSigilApplicationStartsWithoutAnExplicitRunConfigPath)
	ctx.Step(`^application config exists at "([^"]*)" with:$`, world.applicationConfigExistsAtWith)
	ctx.Step(`^run config file exists at "([^"]*)"$`, world.runConfigFileExistsAt)
	ctx.Step(`^run configuration exists at "([^"]*)" with:$`, world.runConfigurationExistsAtWith)
	ctx.Step(`^a directory exists at "([^"]*)"$`, world.aDirectoryExistsAt)
	ctx.Step(`^a file exists at "([^"]*)"$`, world.aFileExistsAt)
	ctx.Step(`^environment override "([^"]*)" is "([^"]*)"$`, world.environmentOverrideIs)
	ctx.Step(`^application configuration is resolved$`, world.applicationConfigurationIsResolved)
	ctx.Step(`^application configuration is merged$`, world.applicationConfigurationIsMerged)
	ctx.Step(`^application configuration validation runs$`, world.applicationConfigurationValidationRuns)
	ctx.Step(`^application logging is initialized$`, world.applicationLoggingIsInitialized)
	ctx.Step(`^application logging writes an info record with message "([^"]*)"$`, world.applicationLoggingWritesAnInfoRecordWithMessage)
	ctx.Step(`^run configuration is resolved$`, world.runConfigurationIsResolved)
	ctx.Step(`^run configuration is merged$`, world.runConfigurationIsMerged)
	ctx.Step(`^run configuration validation runs$`, world.runConfigurationValidationRuns)
	ctx.Step(`^the default application config path is "([^"]*)"$`, world.theDefaultApplicationConfigPathIs)
	ctx.Step(`^the default run config path is "([^"]*)"$`, world.theDefaultRunConfigPathIs)
	ctx.Step(`^the application config format is "([^"]*)"$`, world.theApplicationConfigFormatIs)
	ctx.Step(`^baseline application config keys are "([^"]*)" and "([^"]*)"$`, world.baselineApplicationConfigKeysAreAnd)
	ctx.Step(`^effective application log_level is "([^"]*)"$`, world.effectiveApplicationLogLevelIs)
	ctx.Step(`^effective application log_dir is "([^"]*)"$`, world.effectiveApplicationLogDirIs)
	ctx.Step(`^the effective log file path is "([^"]*)"$`, world.theEffectiveLogFilePathIs)
	ctx.Step(`^the effective log target path is "([^"]*)"$`, world.theEffectiveLogTargetPathIs)
	ctx.Step(`^log records are structured JSON$`, world.logRecordsAreStructuredJSON)
	ctx.Step(`^effective run llm.provider is "([^"]*)"$`, world.effectiveRunLLMProviderIs)
	ctx.Step(`^effective run llm.model is "([^"]*)"$`, world.effectiveRunLLMModelIs)
	ctx.Step(`^effective run llm.gateway is "([^"]*)"$`, world.effectiveRunLLMGatewayIs)
	ctx.Step(`^effective run llm.openrouter.base_url is "([^"]*)"$`, world.effectiveRunLLMOpenRouterBaseURLIs)
	ctx.Step(`^effective run llm.openrouter.request_timeout_ms is (\d+)$`, world.effectiveRunLLMOpenRouterRequestTimeoutMSIs)
	ctx.Step(`^effective run llm.openrouter.api_key_env is "([^"]*)"$`, world.effectiveRunLLMOpenRouterAPIKeyEnvIs)
	ctx.Step(`^effective run rlm.enabled is (true|false)$`, world.effectiveRunRLMEnabledIs)
	ctx.Step(`^effective run rlm.max_depth is (\d+)$`, world.effectiveRunRLMMaxDepthIs)
	ctx.Step(`^effective run llm.reasoning.enabled is (true|false)$`, world.effectiveRunLLMReasoningEnabledIs)
	ctx.Step(`^effective run llm.reasoning.effort is "([^"]*)"$`, world.effectiveRunLLMReasoningEffortIs)
	ctx.Step(`^application configuration initialization fails$`, world.applicationConfigurationInitializationFails)
	ctx.Step(`^application logging initialization fails$`, world.applicationLoggingInitializationFails)
	ctx.Step(`^run configuration initialization succeeds$`, world.runConfigurationInitializationSucceeds)
	ctx.Step(`^run configuration initialization fails$`, world.runConfigurationInitializationFails)
	ctx.Step(`^the sigil executable is available$`, world.theSigilExecutableIsAvailable)
	ctx.Step("^a user runs `([^`]*)`$", world.aUserRuns)
	ctx.Step(`^command exits with status code (\d+)$`, world.commandExitsWithStatusCode)
	ctx.Step(`^command exits non-zero$`, world.commandExitsNonZero)
	ctx.Step("^command output contains `([^`]*)`$", world.commandOutputContains)
	ctx.Step("^command error contains `([^`]*)`$", world.commandErrorContains)
	ctx.Step(`^root usage/help is printed$`, world.rootUsageHelpIsPrinted)
	ctx.Step(`^run usage/help is printed$`, world.runUsageHelpIsPrinted)
	ctx.Step(`^stop usage/help is printed$`, world.stopUsageHelpIsPrinted)
	ctx.Step(`^no default start config files exist$`, world.noDefaultStartConfigFilesExist)
	ctx.Step(`^no default run config files exist$`, world.noDefaultRunConfigFilesExist)
}

func (w *harnessWorld) aCleanSigilWorkingDirectory() error {
	return nil
}

func (w *harnessWorld) sigilConfigEnvironmentVariablesAreCleared() error {
	keys := []string{
		"SIGIL_LOG_LEVEL",
		"SIGIL_LOG_DIR",
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
		"SIGIL_RUN_SYSTEM_PROMPT_APPEND",
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_PROMPT_TEMPLATE",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_CONTEXT_TEMPLATE",
	}

	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			return err
		}
	}

	return nil
}

func (w *harnessWorld) theSigilApplicationStartsWithoutAnExplicitApplicationConfigPath() error {
	w.resolvedAppConfigPath = config.DefaultConfigPath
	return nil
}

func (w *harnessWorld) theSigilApplicationStartsWithoutAnExplicitRunConfigPath() error {
	w.resolvedRunConfigPath = config.DefaultRunConfigPath
	return nil
}

func (w *harnessWorld) applicationConfigExistsAtWith(path string, body *godog.DocString) error {
	w.resolvedAppConfigPath = path
	return os.WriteFile(filepath.Clean(path), []byte(body.Content), 0o644)
}

func (w *harnessWorld) runConfigFileExistsAt(path string) error {
	content := "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n"
	return os.WriteFile(filepath.Clean(path), []byte(content), 0o644)
}

func (w *harnessWorld) runConfigurationExistsAtWith(path string, body *godog.DocString) error {
	w.resolvedRunConfigPath = path
	return os.WriteFile(filepath.Clean(path), []byte(body.Content), 0o644)
}

func (w *harnessWorld) aDirectoryExistsAt(path string) error {
	return os.MkdirAll(filepath.Clean(path), 0o755)
}

func (w *harnessWorld) aFileExistsAt(path string) error {
	return os.WriteFile(filepath.Clean(path), []byte("fixture"), 0o644)
}

func (w *harnessWorld) environmentOverrideIs(key string, value string) error {
	return os.Setenv(key, value)
}

func (w *harnessWorld) applicationConfigurationIsResolved() error {
	w.configInitErr = config.Init()
	return nil
}

func (w *harnessWorld) applicationConfigurationIsMerged() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	return nil
}

func (w *harnessWorld) applicationConfigurationValidationRuns() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	return nil
}

func (w *harnessWorld) applicationLoggingIsInitialized() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	if w.configInitErr != nil {
		w.loggingInitErr = fmt.Errorf("config initialization failed; %w", w.configInitErr)
		return nil
	}

	w.loggingInitErr = logging.Init(config.MustGet())
	return nil
}

func (w *harnessWorld) applicationLoggingWritesAnInfoRecordWithMessage(message string) error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	slog.Info(message)
	return nil
}

func (w *harnessWorld) runConfigurationIsResolved() error {
	w.runConfigInitErr = config.InitRun()
	return nil
}

func (w *harnessWorld) runConfigurationIsMerged() error {
	w.runConfigInitErr = config.InitRunFromPath(w.resolvedRunConfigPath)
	return nil
}

func (w *harnessWorld) runConfigurationValidationRuns() error {
	w.runConfigInitErr = config.InitRunFromPath(w.resolvedRunConfigPath)
	return nil
}

func (w *harnessWorld) theDefaultApplicationConfigPathIs(expectedPath string) error {
	if config.DefaultConfigPath != expectedPath {
		return fmt.Errorf("expected default path %q, got %q", expectedPath, config.DefaultConfigPath)
	}

	return nil
}

func (w *harnessWorld) theDefaultRunConfigPathIs(expectedPath string) error {
	if config.DefaultRunConfigPath != expectedPath {
		return fmt.Errorf("expected run default path %q, got %q", expectedPath, config.DefaultRunConfigPath)
	}

	return nil
}

func (w *harnessWorld) theApplicationConfigFormatIs(expectedFormat string) error {
	if strings.ToLower(expectedFormat) != "yaml" {
		return fmt.Errorf("unsupported expected format %q", expectedFormat)
	}

	return nil
}

func (w *harnessWorld) baselineApplicationConfigKeysAreAnd(expectedKeyOne string, expectedKeyTwo string) error {
	cfgType := reflect.TypeOf(config.Config{})
	tags := make(map[string]struct{}, cfgType.NumField())
	for index := 0; index < cfgType.NumField(); index++ {
		field := cfgType.Field(index)
		tag := field.Tag.Get("mapstructure")
		tags[tag] = struct{}{}
	}

	if _, ok := tags[expectedKeyOne]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyOne)
	}
	if _, ok := tags[expectedKeyTwo]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyTwo)
	}

	return nil
}

func (w *harnessWorld) effectiveApplicationLogLevelIs(expectedLevel string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.LogLevel != expectedLevel {
		return fmt.Errorf("expected log_level %q, got %q", expectedLevel, cfg.LogLevel)
	}

	return nil
}

func (w *harnessWorld) effectiveApplicationLogDirIs(expectedDir string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	expectedPath, err := config.ExpandPath(expectedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve expected log_dir %q; %w", expectedDir, err)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.LogDir != expectedPath {
		return fmt.Errorf("expected log_dir %q, got %q", expectedPath, cfg.LogDir)
	}

	return nil
}

func (w *harnessWorld) theEffectiveLogFilePathIs(expectedPath string) error {
	return w.assertEffectiveLogPath(expectedPath)
}

func (w *harnessWorld) theEffectiveLogTargetPathIs(expectedPath string) error {
	return w.assertEffectiveLogPath(expectedPath)
}

func (w *harnessWorld) logRecordsAreStructuredJSON() error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	logPath, err := logging.ActiveLogFilePath()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filepath.Clean(logPath))
	if err != nil {
		return fmt.Errorf("failed to read log file %q; %w", logPath, err)
	}

	lines := strings.Split(string(content), "\n")
	recordCount := 0
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		record := make(map[string]any)
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("expected structured JSON record, got invalid line %q; %w", line, err)
		}

		if _, ok := record["time"]; !ok {
			return fmt.Errorf("structured JSON record missing time field: %v", record)
		}
		if _, ok := record["level"]; !ok {
			return fmt.Errorf("structured JSON record missing level field: %v", record)
		}
		if _, ok := record["msg"]; !ok {
			return fmt.Errorf("structured JSON record missing msg field: %v", record)
		}

		recordCount++
	}

	if recordCount == 0 {
		return fmt.Errorf("expected at least one structured JSON record")
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMProviderIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Provider != expected {
		return fmt.Errorf("expected llm.provider %q, got %q", expected, cfg.LLM.Provider)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMModelIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Model != expected {
		return fmt.Errorf("expected llm.model %q, got %q", expected, cfg.LLM.Model)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMGatewayIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Gateway != expected {
		return fmt.Errorf("expected llm.gateway %q, got %q", expected, cfg.LLM.Gateway)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterBaseURLIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.BaseURL != expected {
		return fmt.Errorf("expected llm.openrouter.base_url %q, got %q", expected, cfg.LLM.OpenRouter.BaseURL)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterRequestTimeoutMSIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.RequestTimeoutMS != expected {
		return fmt.Errorf("expected llm.openrouter.request_timeout_ms %d, got %d", expected, cfg.LLM.OpenRouter.RequestTimeoutMS)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterAPIKeyEnvIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.APIKeyEnv != expected {
		return fmt.Errorf("expected llm.openrouter.api_key_env %q, got %q", expected, cfg.LLM.OpenRouter.APIKeyEnv)
	}

	return nil
}

func (w *harnessWorld) effectiveRunRLMEnabledIs(expectedRaw string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	expected, err := strconv.ParseBool(expectedRaw)
	if err != nil {
		return fmt.Errorf("failed to parse bool %q; %w", expectedRaw, err)
	}

	if cfg.RLM.Enabled != expected {
		return fmt.Errorf("expected rlm.enabled %t, got %t", expected, cfg.RLM.Enabled)
	}

	return nil
}

func (w *harnessWorld) effectiveRunRLMMaxDepthIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.RLM.MaxDepth != expected {
		return fmt.Errorf("expected rlm.max_depth %d, got %d", expected, cfg.RLM.MaxDepth)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMReasoningEnabledIs(expectedRaw string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	expected, err := strconv.ParseBool(expectedRaw)
	if err != nil {
		return fmt.Errorf("failed to parse bool %q; %w", expectedRaw, err)
	}

	if cfg.LLM.Reasoning.Enabled != expected {
		return fmt.Errorf("expected llm.reasoning.enabled %t, got %t", expected, cfg.LLM.Reasoning.Enabled)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMReasoningEffortIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Reasoning.Effort != expected {
		return fmt.Errorf("expected llm.reasoning.effort %q, got %q", expected, cfg.LLM.Reasoning.Effort)
	}

	return nil
}

func (w *harnessWorld) activeRunConfig() (config.RunConfig, error) {
	if w.runConfigInitErr != nil {
		return config.RunConfig{}, fmt.Errorf("run configuration initialization failed: %w", w.runConfigInitErr)
	}

	return config.GetRun()
}

func (w *harnessWorld) applicationConfigurationInitializationFails() error {
	if w.configInitErr == nil {
		return fmt.Errorf("expected config initialization failure")
	}

	return nil
}

func (w *harnessWorld) applicationLoggingInitializationFails() error {
	if w.loggingInitErr == nil {
		return fmt.Errorf("expected application logging initialization failure")
	}

	return nil
}

func (w *harnessWorld) runConfigurationInitializationSucceeds() error {
	if w.runConfigInitErr != nil {
		return fmt.Errorf("expected run config initialization success, got: %w", w.runConfigInitErr)
	}

	if _, err := config.GetRun(); err != nil {
		return fmt.Errorf("expected active run config, got: %w", err)
	}

	return nil
}

func (w *harnessWorld) runConfigurationInitializationFails() error {
	if w.runConfigInitErr == nil {
		return fmt.Errorf("expected run config initialization failure")
	}

	return nil
}

func (w *harnessWorld) theSigilExecutableIsAvailable() error {
	rootCmd := cmd.NewRootCmd()
	if rootCmd.Use != "sigil" {
		return fmt.Errorf("expected root use sigil, got %q", rootCmd.Use)
	}

	return nil
}

func (w *harnessWorld) aUserRuns(commandLine string) error {
	tokens := strings.Fields(commandLine)
	if len(tokens) == 0 {
		return fmt.Errorf("command line is empty")
	}
	if tokens[0] != "sigil" {
		return fmt.Errorf("expected command to start with sigil, got %q", tokens[0])
	}

	rootCmd := cmd.NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(tokens[1:])

	w.lastErr = rootCmd.Execute()
	w.lastStdout = stdout.String()
	w.lastStderr = stderr.String()
	if w.lastErr != nil {
		w.lastExitCode = 1
		return nil
	}

	w.lastExitCode = 0
	return nil
}

func (w *harnessWorld) commandExitsWithStatusCode(expectedCode int) error {
	if w.lastExitCode != expectedCode {
		return fmt.Errorf("expected exit code %d, got %d", expectedCode, w.lastExitCode)
	}

	return nil
}

func (w *harnessWorld) commandExitsNonZero() error {
	if w.lastExitCode == 0 {
		return fmt.Errorf("expected non-zero exit code")
	}

	return nil
}

func (w *harnessWorld) commandOutputContains(expectedText string) error {
	combined := w.lastStdout + "\n" + w.lastStderr
	if !strings.Contains(combined, expectedText) {
		return fmt.Errorf("expected command output to contain %q, got %q", expectedText, combined)
	}

	return nil
}

func (w *harnessWorld) commandErrorContains(expectedText string) error {
	combined := w.lastStderr
	if w.lastErr != nil {
		combined += "\n" + w.lastErr.Error()
	}

	if !strings.Contains(combined, expectedText) {
		return fmt.Errorf("expected command error to contain %q, got %q", expectedText, combined)
	}

	return nil
}

func (w *harnessWorld) rootUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil") {
		return fmt.Errorf("expected root usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) runUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil run") {
		return fmt.Errorf("expected run usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) stopUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil run stop") {
		return fmt.Errorf("expected stop usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) noDefaultStartConfigFilesExist() error {
	_ = os.Remove(filepath.Clean("./sigil.yaml"))
	_ = os.Remove(filepath.Clean("./sigil-run.yaml"))
	return nil
}

func (w *harnessWorld) noDefaultRunConfigFilesExist() error {
	_ = os.Remove(filepath.Clean(config.DefaultRunConfigPath))
	return nil
}

func (w *harnessWorld) assertEffectiveLogPath(expectedPath string) error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	expected, err := config.ExpandPath(expectedPath)
	if err != nil {
		return fmt.Errorf("failed to resolve expected log path %q; %w", expectedPath, err)
	}

	actual, err := logging.ActiveLogFilePath()
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("expected effective log path %q, got %q", expected, actual)
	}

	return nil
}
