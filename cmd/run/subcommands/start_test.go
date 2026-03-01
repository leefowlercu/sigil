package subcommands

import (
	"os"
	"path/filepath"
	"testing"
)

func TestValidateStartInputsSetsSilenceUsageAfterValidationSuccess(t *testing.T) {
	workDir := t.TempDir()
	writeStartTestFile(t, filepath.Join(workDir, "sigil.yaml"), "log_level: info\n")
	writeStartTestFile(t, filepath.Join(workDir, "sigil-run.yaml"), "prompt: test\n")

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

func TestValidateStartInputsDoesNotSilenceUsageOnValidationError(t *testing.T) {
	startCmd := NewStartCmd()
	if err := validateStartInputs(startCmd, nil); err == nil {
		t.Fatal("expected validation error for missing config files")
	}

	if startCmd.SilenceUsage {
		t.Fatal("expected SilenceUsage to remain false on validation error")
	}
}

func writeStartTestFile(t *testing.T, path string, content string) {
	t.Helper()

	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %s: %v", path, err)
	}
}
