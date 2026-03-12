package subcommands

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

func TestResolveRunsBaseDirUsesInheritedRunDirFlag(t *testing.T) {
	workDir := t.TempDir()

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

	parentCmd := newInspectionParentCommand(t)
	if err := parentCmd.PersistentFlags().Set(runDirFlagName, "./custom-runs"); err != nil {
		t.Fatalf("failed to set run-dir flag: %v", err)
	}

	childCmd := &cobra.Command{Use: "child"}
	parentCmd.AddCommand(childCmd)

	resolved, err := resolveRunsBaseDir(childCmd)
	if err != nil {
		t.Fatalf("expected run-dir resolution success, got %v", err)
	}

	expected, err := runtime.ResolveRunsBaseDir("./custom-runs")
	if err != nil {
		t.Fatalf("expected run-dir normalization success, got %v", err)
	}
	if resolved != expected {
		t.Fatalf("expected resolved run dir %q, got %q", expected, resolved)
	}
}

func TestRunListCommandUsesCustomRunDir(t *testing.T) {
	workDir := t.TempDir()
	parentCmd := newInspectionParentCommand(t)
	listCmd := NewListCmd()
	parentCmd.AddCommand(listCmd)

	customRunsDir := filepath.Join(workDir, "runs")
	if err := parentCmd.PersistentFlags().Set(runDirFlagName, customRunsDir); err != nil {
		t.Fatalf("failed to set run-dir flag: %v", err)
	}

	var stdout bytes.Buffer
	listCmd.SetOut(&stdout)
	if err := validateListInputs(listCmd, nil); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
	if err := runListCommand(listCmd, nil); err != nil {
		t.Fatalf("expected run list success, got %v", err)
	}

	if !strings.Contains(stdout.String(), "Runs dir: "+customRunsDir) || !strings.Contains(stdout.String(), "Runs: 0") {
		t.Fatalf("expected custom run-dir text output, got %q", stdout.String())
	}
}

func TestRunStatusCommandReadsRunFromCustomRunDir(t *testing.T) {
	workDir := t.TempDir()
	customRunsDir := filepath.Join(workDir, "runs")
	runID := mustCreateCompletedRun(t, customRunsDir)

	parentCmd := newInspectionParentCommand(t)
	statusCmd := NewStatusCmd()
	parentCmd.AddCommand(statusCmd)

	if err := parentCmd.PersistentFlags().Set(runDirFlagName, customRunsDir); err != nil {
		t.Fatalf("failed to set run-dir flag: %v", err)
	}
	setInspectionOutputFormat(t, parentCmd, clioutput.FormatJSON)

	var stdout bytes.Buffer
	statusCmd.SetOut(&stdout)
	if err := validateStatusInputs(statusCmd, []string{runID}); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
	if err := runStatusCommand(statusCmd, nil); err != nil {
		t.Fatalf("expected run status success, got %v", err)
	}

	var summary runtime.RunSummary
	if err := json.Unmarshal(stdout.Bytes(), &summary); err != nil {
		t.Fatalf("expected JSON summary, got %v", err)
	}
	if summary.RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, summary.RunID)
	}
	if summary.EventsPath != filepath.Join(customRunsDir, runID, "events.jsonl") {
		t.Fatalf("expected events_path %q, got %q", filepath.Join(customRunsDir, runID, "events.jsonl"), summary.EventsPath)
	}

	var raw map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &raw); err != nil {
		t.Fatalf("expected raw JSON decode success, got %v", err)
	}
	if _, ok := raw["nodes"]; ok {
		t.Fatalf("expected status JSON to omit inspect-only nodes payload, got %+v", raw["nodes"])
	}
	if _, ok := raw["process_metadata"]; ok {
		t.Fatalf("expected status JSON to omit inspect-only process_metadata payload, got %+v", raw["process_metadata"])
	}
}

func TestRunEventsCommandReadsEventsFromCustomRunDir(t *testing.T) {
	workDir := t.TempDir()
	customRunsDir := filepath.Join(workDir, "runs")
	runID := mustCreateCompletedRun(t, customRunsDir)

	parentCmd := newInspectionParentCommand(t)
	eventsCmd := NewEventsCmd()
	parentCmd.AddCommand(eventsCmd)

	if err := parentCmd.PersistentFlags().Set(runDirFlagName, customRunsDir); err != nil {
		t.Fatalf("failed to set run-dir flag: %v", err)
	}
	setInspectionOutputFormat(t, parentCmd, clioutput.FormatJSON)

	var stdout bytes.Buffer
	eventsCmd.SetOut(&stdout)
	if err := validateEventsInputs(eventsCmd, []string{runID}); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}
	if err := runEventsCommand(eventsCmd, nil); err != nil {
		t.Fatalf("expected run events success, got %v", err)
	}

	var events []runtime.EventEnvelope
	if err := json.Unmarshal(stdout.Bytes(), &events); err != nil {
		t.Fatalf("expected JSON events, got %v", err)
	}
	if len(events) == 0 {
		t.Fatal("expected at least one event")
	}
	if events[0].RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, events[0].RunID)
	}
}

func TestWriteRunProjectionTextIncludesTerminalIdentifiers(t *testing.T) {
	t.Run("failed", func(t *testing.T) {
		failedNodeID := "019ce40d-aaaa-7aaa-8aaa-aaaaaaaaaaaa"
		failedStepID := "019ce40d-bbbb-7bbb-8bbb-bbbbbbbbbbbb"
		errorCode := "runtime.failure"
		errorMessage := "unrecoverable runtime failure"
		projection := runtime.RunProjection{
			RunID:        "019ce40d-cccc-7ccc-8ccc-cccccccccccc",
			State:        string(runtime.RunStateFailed),
			RunDir:       "/tmp/custom-runs/019ce40d-cccc-7ccc-8ccc-cccccccccccc",
			EventsPath:   "/tmp/custom-runs/019ce40d-cccc-7ccc-8ccc-cccccccccccc/events.jsonl",
			PIDStatus:    runtime.RunPIDStatusMissing,
			ErrorCode:    &errorCode,
			ErrorMessage: &errorMessage,
			FailedNodeID: &failedNodeID,
			FailedStepID: &failedStepID,
		}

		var stdout bytes.Buffer
		if err := writeRunProjectionText(&stdout, "Run inspect", projection, false); err != nil {
			t.Fatalf("expected failed projection text output success, got %v", err)
		}

		rendered := stdout.String()
		if !strings.Contains(rendered, "Failed node ID: "+failedNodeID) {
			t.Fatalf("expected failed node ID in text output, got %q", rendered)
		}
		if !strings.Contains(rendered, "Failed step ID: "+failedStepID) {
			t.Fatalf("expected failed step ID in text output, got %q", rendered)
		}
	})

	t.Run("interrupted", func(t *testing.T) {
		interruptedBy := "cli.run.stop"
		interruptedNodeID := "019ce40d-dddd-7ddd-8ddd-dddddddddddd"
		reason := "user_request"
		projection := runtime.RunProjection{
			RunID:             "019ce40d-eeee-7eee-8eee-eeeeeeeeeeee",
			State:             string(runtime.RunStateInterrupted),
			RunDir:            "/tmp/custom-runs/019ce40d-eeee-7eee-8eee-eeeeeeeeeeee",
			EventsPath:        "/tmp/custom-runs/019ce40d-eeee-7eee-8eee-eeeeeeeeeeee/events.jsonl",
			PIDStatus:         runtime.RunPIDStatusMissing,
			InterruptedReason: &reason,
			InterruptedBy:     &interruptedBy,
			InterruptedNodeID: &interruptedNodeID,
		}

		var stdout bytes.Buffer
		if err := writeRunProjectionText(&stdout, "Run inspect", projection, false); err != nil {
			t.Fatalf("expected interrupted projection text output success, got %v", err)
		}

		rendered := stdout.String()
		if !strings.Contains(rendered, "Interrupted node ID: "+interruptedNodeID) {
			t.Fatalf("expected interrupted node ID in text output, got %q", rendered)
		}
	})
}

func newInspectionParentCommand(t *testing.T) *cobra.Command {
	t.Helper()

	parentCmd := &cobra.Command{Use: "run"}
	parentCmd.PersistentFlags().String(runDirFlagName, DefaultRunDir(), "runs dir")

	var format clioutput.Format
	clioutput.AddOutputFlag(parentCmd, &format)

	return parentCmd
}

func setInspectionOutputFormat(t *testing.T, parentCmd *cobra.Command, format clioutput.Format) {
	t.Helper()

	if err := parentCmd.PersistentFlags().Set("output", string(format)); err != nil {
		t.Fatalf("failed to set output flag: %v", err)
	}
}

func mustCreateCompletedRun(t *testing.T, runsBaseDir string) string {
	t.Helper()

	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir: runsBaseDir,
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

	return lifecycle.RunID()
}
