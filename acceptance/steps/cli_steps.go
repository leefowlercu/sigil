package steps

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"

	"github.com/cucumber/godog"
	"github.com/leefowlercu/sigil/cmd"
	"github.com/leefowlercu/sigil/internal/config"
)

type harnessWorld struct {
	workingDir            string
	originalWorkingDir    string
	lastStdout            string
	lastStderr            string
	lastErr               error
	lastExitCode          int
	configInitErr         error
	resolvedAppConfigPath string
}

// InitializeScenario wires all acceptance steps for harness.feature.
func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &harnessWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
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
		world.resolvedAppConfigPath = config.DefaultConfigPath

		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
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
	ctx.Step(`^application config exists at "([^"]*)" with:$`, world.applicationConfigExistsAtWith)
	ctx.Step(`^run config file exists at "([^"]*)"$`, world.runConfigFileExistsAt)
	ctx.Step(`^a directory exists at "([^"]*)"$`, world.aDirectoryExistsAt)
	ctx.Step(`^environment override "([^"]*)" is "([^"]*)"$`, world.environmentOverrideIs)
	ctx.Step(`^application configuration is resolved$`, world.applicationConfigurationIsResolved)
	ctx.Step(`^application configuration is merged$`, world.applicationConfigurationIsMerged)
	ctx.Step(`^application configuration validation runs$`, world.applicationConfigurationValidationRuns)
	ctx.Step(`^the default application config path is "([^"]*)"$`, world.theDefaultApplicationConfigPathIs)
	ctx.Step(`^the application config format is "([^"]*)"$`, world.theApplicationConfigFormatIs)
	ctx.Step(`^baseline application config keys are "([^"]*)" and "([^"]*)"$`, world.baselineApplicationConfigKeysAreAnd)
	ctx.Step(`^effective application log_level is "([^"]*)"$`, world.effectiveApplicationLogLevelIs)
	ctx.Step(`^effective application log_dir is "([^"]*)"$`, world.effectiveApplicationLogDirIs)
	ctx.Step(`^application configuration initialization fails$`, world.applicationConfigurationInitializationFails)
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
}

func (w *harnessWorld) aCleanSigilWorkingDirectory() error {
	return nil
}

func (w *harnessWorld) sigilConfigEnvironmentVariablesAreCleared() error {
	for _, key := range []string{"SIGIL_LOG_LEVEL", "SIGIL_LOG_DIR"} {
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

func (w *harnessWorld) applicationConfigExistsAtWith(path string, body *godog.DocString) error {
	w.resolvedAppConfigPath = path
	return os.WriteFile(filepath.Clean(path), []byte(body.Content), 0o644)
}

func (w *harnessWorld) runConfigFileExistsAt(path string) error {
	return os.WriteFile(filepath.Clean(path), []byte("prompt: test\n"), 0o644)
}

func (w *harnessWorld) aDirectoryExistsAt(path string) error {
	return os.MkdirAll(filepath.Clean(path), 0o755)
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

func (w *harnessWorld) theDefaultApplicationConfigPathIs(expectedPath string) error {
	if config.DefaultConfigPath != expectedPath {
		return fmt.Errorf("expected default path %q, got %q", expectedPath, config.DefaultConfigPath)
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

func (w *harnessWorld) applicationConfigurationInitializationFails() error {
	if w.configInitErr == nil {
		return fmt.Errorf("expected config initialization failure")
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
