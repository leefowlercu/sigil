package cmd

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/logging"
)

func TestRootCommandNoSubcommandPrintsUsage(t *testing.T) {
	stdout, _, err := executeRootCommand(t, t.TempDir(), nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("expected usage output, got %q", stdout)
	}
}

func TestRunCommandNoSubcommandPrintsUsage(t *testing.T) {
	stdout, _, err := executeRootCommand(t, t.TempDir(), nil, "run")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "sigil run") {
		t.Fatalf("expected run usage output, got %q", stdout)
	}
}

func TestRunStopPrintsUsage(t *testing.T) {
	stdout, _, err := executeRootCommand(t, t.TempDir(), nil, "run", "stop")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "sigil run stop") {
		t.Fatalf("expected stop usage output, got %q", stdout)
	}
}

func TestUnknownSubcommandUsesCobraDefaultError(t *testing.T) {
	_, stderr, err := executeRootCommand(t, t.TempDir(), nil, "unknown")
	if err == nil {
		t.Fatal("expected non-nil error for unknown subcommand")
	}

	if !strings.Contains(err.Error(), "unknown command") {
		t.Fatalf("expected unknown command error, got %v", err)
	}

	if !strings.Contains(stderr, "unknown command") {
		t.Fatalf("expected stderr to include unknown command, got %q", stderr)
	}
}

func TestRunStartUsesDefaultPathsWhenFlagsOmitted(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRootBootstrapInitializesLoggingAtDefaultPath(t *testing.T) {
	workDir := t.TempDir()

	_, _, err := executeRootCommand(t, workDir, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	activeLogPath, err := logging.ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path, got %v", err)
	}

	expectedPath, err := config.ExpandPath("./sigil/logs/sigil.log")
	if err != nil {
		t.Fatalf("expected expected-path expansion success, got %v", err)
	}
	if activeLogPath != expectedPath {
		t.Fatalf("expected active log path %q, got %q", expectedPath, activeLogPath)
	}
}

func TestRootBootstrapUsesConfigOverrideForLoggingPath(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), "log_level: info\nlog_dir: ./override-logs\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--config", "./custom-sigil.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	activeLogPath, err := logging.ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path, got %v", err)
	}

	expectedPath, err := config.ExpandPath("./override-logs/sigil.log")
	if err != nil {
		t.Fatalf("expected expected-path expansion success, got %v", err)
	}
	if activeLogPath != expectedPath {
		t.Fatalf("expected active log path %q, got %q", expectedPath, activeLogPath)
	}
}

func TestRootBootstrapFailsWhenLogSinkCannotBeInitialized(t *testing.T) {
	workDir := t.TempDir()
	blockedTarget := filepath.Join(workDir, "blocked-log-target")
	writeFile(t, blockedTarget, "blocked")
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\nlog_dir: ./blocked-log-target\n")

	_, stderr, err := executeRootCommand(t, workDir, nil, "run")
	if err == nil {
		t.Fatal("expected logging initialization failure")
	}

	if !strings.Contains(err.Error(), "failed to initialize application logging;") {
		t.Fatalf("expected wrapped logging init error, got %v", err)
	}

	if !strings.Contains(stderr, "failed to initialize application logging;") {
		t.Fatalf("expected stderr to include logging init error, got %q", stderr)
	}
}

func TestRunStartOverridesConfigPath(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), "log_level: warn\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--config", "./custom-sigil.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartOverridesRunConfigPath(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")
	writeFile(t, filepath.Join(workDir, "custom-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--run-config", "./custom-run.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartOverridesBothPaths(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), "log_level: debug\n")
	writeFile(t, filepath.Join(workDir, "custom-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(
		t,
		workDir,
		nil,
		"run",
		"start",
		"--config",
		"./custom-sigil.yaml",
		"--run-config",
		"./custom-run.yaml",
	)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartFailsWhenResolvedPathIsMissing(t *testing.T) {
	_, _, err := executeRootCommand(t, t.TempDir(), nil, "run", "start")
	if err == nil {
		t.Fatal("expected error for missing default paths")
	}

	if !strings.Contains(err.Error(), "invalid --config value") {
		t.Fatalf("expected config validation error, got %v", err)
	}
}

func TestRunStartFailsWhenConfigFlagIsExplicitlyEmpty(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--config", "")
	if err == nil {
		t.Fatal("expected error for empty --config value")
	}

	if !strings.Contains(err.Error(), "invalid --config value") {
		t.Fatalf("expected invalid --config error, got %v", err)
	}
}

func TestRunStartFailsWhenRunConfigFlagIsExplicitlyEmpty(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--run-config", "")
	if err == nil {
		t.Fatal("expected error for empty --run-config value")
	}

	if !strings.Contains(err.Error(), "invalid --run-config value") {
		t.Fatalf("expected invalid --run-config error, got %v", err)
	}
}

func TestRunStartFailsWhenResolvedPathIsNotRegularFile(t *testing.T) {
	workDir := t.TempDir()
	dirPath := filepath.Join(workDir, "not-a-file")
	if err := os.Mkdir(dirPath, 0o755); err != nil {
		t.Fatalf("failed to create directory: %v", err)
	}
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--config", "./not-a-file")
	if err == nil {
		t.Fatal("expected error for non-regular config path")
	}

	if !strings.Contains(err.Error(), "invalid --config value") {
		t.Fatalf("expected invalid --config error, got %v", err)
	}
}

func TestRunStartUsesFrameworkDefaultUnknownFlagError(t *testing.T) {
	_, stderr, err := executeRootCommand(t, t.TempDir(), nil, "run", "start", "--unknown-flag")
	if err == nil {
		t.Fatal("expected unknown flag error")
	}

	if !strings.Contains(err.Error(), "unknown flag") {
		t.Fatalf("expected unknown flag error, got %v", err)
	}

	if !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("expected stderr to include unknown flag, got %q", stderr)
	}
}

func TestRunStartReturnsRuntimeErrorForInvalidAppConfig(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: trace\n")
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start")
	if err == nil {
		t.Fatal("expected config initialization error")
	}

	if !strings.Contains(err.Error(), "failed to initialize config;") {
		t.Fatalf("expected wrapped runtime error, got %v", err)
	}
}

func TestRunStartAllowsMissingDefaultRunConfigWhenEnvironmentSuppliesRequiredValues(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")

	env := map[string]string{
		"SIGIL_RUN_PROMPT":       "env prompt",
		"SIGIL_RUN_CONTEXT":      "env context",
		"SIGIL_RUN_LLM_PROVIDER": "openai",
		"SIGIL_RUN_LLM_MODEL":    "gpt-5.1",
	}

	_, _, err := executeRootCommand(t, workDir, env, "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartFailsWhenExplicitRunConfigPathIsMissing(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--run-config", "./missing-run.yaml")
	if err == nil {
		t.Fatal("expected missing run config path error")
	}

	if !strings.Contains(err.Error(), "invalid --run-config value") {
		t.Fatalf("expected invalid --run-config error, got %v", err)
	}
}

func executeRootCommand(t *testing.T, workingDir string, env map[string]string, args ...string) (string, string, error) {
	t.Helper()

	rootCmd := NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)

	for key, value := range env {
		t.Setenv(key, value)
	}

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get current working directory: %v", err)
	}

	if workingDir != "" {
		if err := os.Chdir(workingDir); err != nil {
			t.Fatalf("failed to change working directory: %v", err)
		}
		t.Cleanup(func() {
			_ = os.Chdir(originalDir)
		})
	}

	err = rootCmd.Execute()
	return stdout.String(), stderr.String(), err
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
