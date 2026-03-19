package cmd

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/logging"
	"github.com/leefowlercu/sigil/internal/runtime"
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

func TestRunStopRequiresRunID(t *testing.T) {
	_, stderr, err := executeRootCommand(t, t.TempDir(), nil, "run", "stop")
	if err == nil {
		t.Fatal("expected validation error for missing run-id")
	}

	if !strings.Contains(err.Error(), "accepts 1 arg(s), received 0") {
		t.Fatalf("expected missing run-id error, got %v", err)
	}

	if !strings.Contains(stderr, "accepts 1 arg(s), received 0") {
		t.Fatalf("expected stderr to include missing run-id error, got %q", stderr)
	}
}

func TestRunStopUsesDefaultConfigBehaviorWhenBuiltBinaryNameExists(t *testing.T) {
	workDir := t.TempDir()
	writeFileBytes(t, filepath.Join(workDir, "sigil"), []byte{0xff, 0xfe, 0xfd}, 0o755)

	previousDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to chdir: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(previousDir)
	})

	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		Name:        "test-run",
		RunsBaseDir: runtime.DefaultRunsBaseDir,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})
	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected lifecycle start success, got %v", err)
	}
	if err := lifecycle.Complete(); err != nil {
		t.Fatalf("expected lifecycle complete success, got %v", err)
	}

	stdout, stderr, err := executeRootCommand(t, workDir, nil, "run", "stop", lifecycle.RunID())
	if err != nil {
		t.Fatalf("expected run stop success without parsing built binary as config, got err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(stdout, lifecycle.RunID()) {
		t.Fatalf("expected stop output to include run id, got %q", stdout)
	}

	activeLogPath, err := logging.ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path after run stop bootstrap, got %v", err)
	}

	expectedLogPath, err := config.ExpandPath("./.sigil/logs/sigil.log")
	if err != nil {
		t.Fatalf("expected log path expansion success, got %v", err)
	}
	if activeLogPath != expectedLogPath {
		t.Fatalf("expected active log path %q, got %q", expectedLogPath, activeLogPath)
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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartEmitsOperationalLogRecords(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", "./logs"))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	logPath, err := logging.ActiveLogFilePath()
	if err != nil {
		t.Fatalf("expected active log path, got %v", err)
	}
	if err := logging.Close(); err != nil {
		t.Fatalf("expected logging close success, got %v", err)
	}

	records := readJSONLogRecords(t, logPath)
	if len(records) == 0 {
		t.Fatal("expected non-empty log file after run start execution")
	}

	expectedMessages := []string{
		"application logging initialized",
		"run start command beginning",
		"harness run completed",
		"run start command completed",
	}
	for _, expectedMessage := range expectedMessages {
		if !containsLogMessage(records, expectedMessage) {
			t.Fatalf("expected log message %q, got records: %v", expectedMessage, records)
		}
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

	expectedPath, err := config.ExpandPath("./.sigil/logs/sigil.log")
	if err != nil {
		t.Fatalf("expected expected-path expansion success, got %v", err)
	}
	if activeLogPath != expectedPath {
		t.Fatalf("expected active log path %q, got %q", expectedPath, activeLogPath)
	}
}

func TestRootBootstrapUsesConfigOverrideForLoggingPath(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), applicationConfigYAML("info", "./override-logs"))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", "./blocked-log-target"))

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
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), applicationConfigYAML("warn", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--config", "./custom-sigil.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartOverridesRunConfigPath(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "custom-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--run-config", "./custom-run.yaml")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
}

func TestRunStartOverridesBothPaths(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "custom-sigil.yaml"), applicationConfigYAML("debug", ""))
	writeFile(t, filepath.Join(workDir, "custom-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("trace", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))

	env := map[string]string{
		"SIGIL_RUN_NAME":         "test-run",
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
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))

	_, _, err := executeRootCommand(t, workDir, nil, "run", "start", "--run-config", "./missing-run.yaml")
	if err == nil {
		t.Fatal("expected missing run config path error")
	}

	if !strings.Contains(err.Error(), "invalid --run-config value") {
		t.Fatalf("expected invalid --run-config error, got %v", err)
	}
}

func TestRunStartDefaultsToHumanReadableTextOutput(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	stdout, _, err := executeRootCommand(t, workDir, nil, "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "Run start") {
		t.Fatalf("expected text preflight header, got %q", stdout)
	}
	if !strings.Contains(stdout, "Run running: run_id=") {
		t.Fatalf("expected live run progress output, got %q", stdout)
	}
	if !strings.Contains(stdout, "Run summary") {
		t.Fatalf("expected text summary footer, got %q", stdout)
	}
	if !strings.Contains(stdout, "State: completed") {
		t.Fatalf("expected completed state in text summary, got %q", stdout)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("expected default text output, got %q", stdout)
	}
}

func TestRunStartJSONOutputPreservesSummaryShape(t *testing.T) {
	workDir := t.TempDir()
	writeFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeFile(t, filepath.Join(workDir, "sigil-run.yaml"), "name: test-run\nprompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	stdout, _, err := executeRootCommand(t, workDir, nil, "--output", "json", "run", "start")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	var result struct {
		RunID          string `json:"run_id"`
		State          string `json:"state"`
		FinalAnswer    string `json:"final_answer"`
		FinalAnswerRef string `json:"final_answer_ref"`
		EventsPath     string `json:"events_path"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(stdout)), &result); err != nil {
		t.Fatalf("expected JSON summary, got %v", err)
	}
	if result.RunID == "" || result.FinalAnswerRef == "" || result.EventsPath == "" {
		t.Fatalf("expected non-empty JSON summary fields, got %+v", result)
	}
	if result.State != "completed" {
		t.Fatalf("expected completed state, got %q", result.State)
	}
	if result.FinalAnswer == "" {
		t.Fatalf("expected final answer in JSON output, got %+v", result)
	}
}

func TestRootUsageRemainsTextWhenOutputJSONIsRequested(t *testing.T) {
	stdout, _, err := executeRootCommand(t, t.TempDir(), nil, "--output", "json")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "Usage:") {
		t.Fatalf("expected usage output, got %q", stdout)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("expected text usage output, got %q", stdout)
	}
}

func TestRunUsageRemainsTextWhenOutputJSONIsRequested(t *testing.T) {
	stdout, _, err := executeRootCommand(t, t.TempDir(), nil, "--output", "json", "run")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if !strings.Contains(stdout, "sigil run") {
		t.Fatalf("expected run usage output, got %q", stdout)
	}
	if strings.HasPrefix(strings.TrimSpace(stdout), "{") {
		t.Fatalf("expected text usage output, got %q", stdout)
	}
}

func TestInvalidOutputFlagReturnsValidationError(t *testing.T) {
	_, stderr, err := executeRootCommand(t, t.TempDir(), nil, "--output", "yaml", "run")
	if err == nil {
		t.Fatal("expected output flag validation error")
	}

	if !strings.Contains(err.Error(), "output must be one of: text, json") {
		t.Fatalf("expected output validation error, got %v", err)
	}
	if !strings.Contains(stderr, "output must be one of: text, json") {
		t.Fatalf("expected stderr to include output validation error, got %q", stderr)
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

	if isRunStartCommand(args) {
		mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, request *http.Request) {
			var requestBody map[string]any
			_ = json.NewDecoder(request.Body).Decode(&requestBody)
			contextRef := extractContextRefFromOpenRouterRequest(requestBody)
			if contextRef == "" {
				contextRef = "__context_ref_missing__"
			}

			decisionPayload := map[string]any{
				"decision": "final",
				"final": map[string]any{
					"answer": "done",
					"evidence": []map[string]any{
						{"ref": contextRef},
					},
				},
			}
			decisionPayloadBytes, _ := json.Marshal(decisionPayload)
			responseBody := map[string]any{
				"id":       "resp_test",
				"status":   "completed",
				"provider": "openai",
				"model":    "gpt-5.1",
				"output": []any{
					map[string]any{
						"content": []any{
							map[string]any{
								"type": "output_text",
								"text": string(decisionPayloadBytes),
							},
						},
					},
				},
				"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
			}

			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(responseBody)
		}))
		t.Cleanup(mockServer.Close)

		t.Setenv("OPENROUTER_API_KEY", "test-key")
		t.Setenv("SIGIL_RUN_LLM_OPENROUTER_BASE_URL", mockServer.URL)
		t.Setenv("SIGIL_RUN_LLM_OPENROUTER_API_KEY_ENV", "OPENROUTER_API_KEY")
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

func isRunStartCommand(args []string) bool {
	for index := 0; index+1 < len(args); index++ {
		if args[index] == "run" && args[index+1] == "start" {
			return true
		}
	}

	return false
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func writeFileBytes(t *testing.T, path string, content []byte, perm os.FileMode) {
	t.Helper()

	if err := os.WriteFile(path, content, perm); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func readJSONLogRecords(t *testing.T, path string) []map[string]any {
	t.Helper()

	content, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read log file %q: %v", path, err)
	}

	rawLines := strings.Split(string(content), "\n")
	records := make([]map[string]any, 0, len(rawLines))
	for _, rawLine := range rawLines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		record := make(map[string]any)
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Fatalf("expected JSON log line, got parse error for %q: %v", line, err)
		}
		records = append(records, record)
	}

	return records
}

func applicationConfigYAML(level string, dir string) string {
	var builder strings.Builder
	builder.WriteString("logs:\n")
	if level != "" {
		builder.WriteString("  level: " + level + "\n")
	}
	if dir != "" {
		builder.WriteString("  dir: " + dir + "\n")
	}
	return builder.String()
}

func containsLogMessage(records []map[string]any, message string) bool {
	for _, record := range records {
		value, ok := record["msg"].(string)
		if !ok {
			continue
		}
		if value == message {
			return true
		}
	}

	return false
}

func extractContextRefFromOpenRouterRequest(requestBody map[string]any) string {
	input, ok := requestBody["input"].([]any)
	if !ok {
		return ""
	}

	for _, messageAny := range input {
		message, ok := messageAny.(map[string]any)
		if !ok {
			continue
		}
		role, _ := message["role"].(string)
		if role != "user" {
			continue
		}

		content, _ := message["content"].(string)
		if strings.TrimSpace(content) == "" {
			continue
		}

		envelope := make(map[string]any)
		if err := json.Unmarshal([]byte(content), &envelope); err != nil {
			continue
		}
		contextMetadata, ok := envelope["context_metadata"].(map[string]any)
		if !ok {
			continue
		}
		contextRef, _ := contextMetadata["context_ref"].(string)
		if strings.TrimSpace(contextRef) != "" {
			return contextRef
		}
	}

	return ""
}
