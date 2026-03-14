package subcommands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateStartInputsSetsSilenceUsageAfterValidationSuccess(t *testing.T) {
	workDir := t.TempDir()
	writeStartTestFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	startCmd := NewStartCmd()
	if err := validateStartInputs(startCmd, nil); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}

	if !startCmd.SilenceUsage {
		t.Fatal("expected SilenceUsage to be true after successful validation")
	}
}

func TestValidateStartInputsFailsWhenExplicitRunConfigPathIsMissing(t *testing.T) {
	workDir := t.TempDir()
	writeStartTestFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	startCmd := NewStartCmd()
	if err := startCmd.Flags().Set("run-config", "./missing-run.yaml"); err != nil {
		t.Fatalf("failed to set run-config flag: %v", err)
	}

	if err := validateStartInputs(startCmd, nil); err == nil {
		t.Fatal("expected validation error for missing explicit --run-config value")
	}
}

func TestValidateStartInputsDoesNotSilenceUsageOnValidationError(t *testing.T) {
	startCmd := NewStartCmd()
	if err := validateStartInputs(startCmd, nil); err == nil {
		t.Fatal("expected validation error for missing config files")
	}

	if startCmd.SilenceUsage {
		t.Fatal("expected SilenceUsage to remain false on validation error")
	}
}

func TestValidateStartInputsRejectsInvalidVarEntry(t *testing.T) {
	workDir := t.TempDir()
	writeStartTestFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeStartTestFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	startCmd := NewStartCmd()
	if err := startCmd.Flags().Set("var", "invalid"); err != nil {
		t.Fatalf("failed to set var flag: %v", err)
	}

	if err := validateStartInputs(startCmd, nil); err == nil {
		t.Fatal("expected validation failure for invalid --var entry")
	}
}

func TestParseTemplateVarsUsesLastValueForDuplicateKey(t *testing.T) {
	resolved, err := parseTemplateVars([]string{"a=1", "a=2", "b=3"})
	if err != nil {
		t.Fatalf("expected parse success, got %v", err)
	}
	if resolved["a"] != "2" {
		t.Fatalf("expected duplicate key to keep last value %q, got %q", "2", resolved["a"])
	}
	if resolved["b"] != "3" {
		t.Fatalf("expected value %q for key b, got %q", "3", resolved["b"])
	}
}

func TestValidateStartInputsPreservesMultilineVarValues(t *testing.T) {
	workDir := t.TempDir()
	writeStartTestFile(t, filepath.Join(workDir, "sigil.yaml"), applicationConfigYAML("info", ""))
	writeStartTestFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt_template: '{{.question}}'\ncontext_template: '{{.external_context}}'\nllm:\n  provider: openai\n  model: gpt-5.1\n")

	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	contextValue := "line1\nline2,with,commas\nline3"

	startCmd := NewStartCmd()
	if err := startCmd.Flags().Set("var", "question=test"); err != nil {
		t.Fatalf("failed to set question var: %v", err)
	}
	if err := startCmd.Flags().Set("var", "external_context="+contextValue); err != nil {
		t.Fatalf("failed to set external context var: %v", err)
	}

	if err := validateStartInputs(startCmd, nil); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}

	if got := startTemplateVars["external_context"]; got != contextValue {
		t.Fatalf("expected multiline context var to be preserved, got %q", got)
	}
}

func writeStartTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}

func applicationConfigYAML(level string, dir string) string {
	content := "logs:\n"
	if level != "" {
		content += "  level: " + level + "\n"
	}
	if dir != "" {
		content += "  dir: " + dir + "\n"
	}
	return content
}
