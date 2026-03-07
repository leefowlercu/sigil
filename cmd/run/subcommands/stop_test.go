package subcommands

import (
	"bytes"
	"encoding/json"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

const testStopRunID = "019c7714-3b77-74d1-9866-e1f484aae2ab"

func TestValidateStopInputsSetsSilenceUsageAfterValidationSuccess(t *testing.T) {
	stopCmd := NewStopCmd()
	if err := validateStopInputs(stopCmd, []string{testStopRunID}); err != nil {
		t.Fatalf("expected validation success, got %v", err)
	}

	if stopRunID != testStopRunID {
		t.Fatalf("expected stopRunID %q, got %q", testStopRunID, stopRunID)
	}
	if !stopCmd.SilenceUsage {
		t.Fatal("expected SilenceUsage to be true after successful validation")
	}
}

func TestValidateStopInputsRejectsInvalidRunID(t *testing.T) {
	testCases := []struct {
		name string
		args []string
	}{
		{name: "empty arg list", args: nil},
		{name: "non uuid", args: []string{"not-a-uuid"}},
		{name: "uuidv4", args: []string{uuid.NewString()}},
		{name: "extra arg", args: []string{testStopRunID, "extra"}},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			stopCmd := NewStopCmd()
			if err := validateStopInputs(stopCmd, testCase.args); err == nil {
				t.Fatal("expected validation error")
			}
			if stopCmd.SilenceUsage {
				t.Fatal("expected SilenceUsage to remain false on validation error")
			}
		})
	}
}

func TestRunStopCommandWritesTerminalNoOpJSONResult(t *testing.T) {
	testCases := []struct {
		name          string
		transition    func(*runtime.Lifecycle) error
		expectedState string
	}{
		{
			name: "completed",
			transition: func(lifecycle *runtime.Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Complete()
			},
			expectedState: string(runtime.RunStateCompleted),
		},
		{
			name: "failed",
			transition: func(lifecycle *runtime.Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Fail()
			},
			expectedState: string(runtime.RunStateFailed),
		},
		{
			name: "interrupted",
			transition: func(lifecycle *runtime.Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Interrupt()
			},
			expectedState: string(runtime.RunStateInterrupted),
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			withStopTestWorkingDir(t, func() {
				lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
					RunsBaseDir: runtime.DefaultRunsBaseDir,
					MaxDepth:    3,
				})
				if err != nil {
					t.Fatalf("expected lifecycle creation success, got %v", err)
				}
				t.Cleanup(func() {
					_ = lifecycle.Close()
				})
				if err := testCase.transition(lifecycle); err != nil {
					t.Fatalf("expected lifecycle transition success, got %v", err)
				}

				stopCmd := NewStopCmd()
				var stdout bytes.Buffer
				stopCmd.SetOut(&stdout)
				setStopTestOutputFormat(t, stopCmd, clioutput.FormatJSON)
				if err := validateStopInputs(stopCmd, []string{lifecycle.RunID()}); err != nil {
					t.Fatalf("expected validation success, got %v", err)
				}
				if err := runStopCommand(stopCmd, nil); err != nil {
					t.Fatalf("expected terminal no-op success, got %v", err)
				}

				var result stopResult
				if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
					t.Fatalf("expected JSON result, got %v", err)
				}
				if result.RunID != lifecycle.RunID() {
					t.Fatalf("expected run_id %q, got %q", lifecycle.RunID(), result.RunID)
				}
				if result.StopRequested {
					t.Fatal("expected stop_requested=false for terminal run")
				}
				if result.State != testCase.expectedState {
					t.Fatalf("expected state %q, got %q", testCase.expectedState, result.State)
				}
				if !strings.HasSuffix(result.EventsPath, "/"+lifecycle.RunID()+"/events.jsonl") {
					t.Fatalf("expected events_path to end with run events path, got %q", result.EventsPath)
				}
			})
		})
	}
}

func TestRunStopCommandFailsWithoutProcessMetadataForNonTerminalRun(t *testing.T) {
	withStopTestWorkingDir(t, func() {
		lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
			RunsBaseDir: runtime.DefaultRunsBaseDir,
			MaxDepth:    3,
		})
		if err != nil {
			t.Fatalf("expected lifecycle creation success, got %v", err)
		}
		t.Cleanup(func() {
			_ = lifecycle.Close()
		})

		stopCmd := NewStopCmd()
		if err := validateStopInputs(stopCmd, []string{lifecycle.RunID()}); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
		err = runStopCommand(stopCmd, nil)
		if err == nil {
			t.Fatal("expected non-terminal run stop failure without process metadata")
		}
		if !strings.Contains(err.Error(), "failed to resolve live process metadata") {
			t.Fatalf("expected process metadata error, got %v", err)
		}
	})
}

func TestRunStopCommandRejectsStaleProcessMetadataBeforeSignaling(t *testing.T) {
	withStopTestWorkingDir(t, func() {
		lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
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

		liveProcess := mustCreateLongRunningProcess(t)
		t.Cleanup(func() {
			_ = liveProcess.Process.Kill()
			_ = liveProcess.Wait()
		})

		if err := runtime.WriteProcessMetadata(runtime.DefaultRunsBaseDir, runtime.ProcessMetadata{
			RunID:      lifecycle.RunID(),
			PID:        liveProcess.Process.Pid,
			RecordedAt: time.Now().UTC(),
			StartedAt:  time.Unix(1_700_000_000, 0).UTC(),
			Source:     runtime.RunSourceCLIRunStart,
		}); err != nil {
			t.Fatalf("expected process metadata write success, got %v", err)
		}

		stopCmd := NewStopCmd()
		if err := validateStopInputs(stopCmd, []string{lifecycle.RunID()}); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
		err = runStopCommand(stopCmd, nil)
		if err == nil {
			t.Fatal("expected stale process metadata error")
		}
		if !strings.Contains(err.Error(), "stale process metadata") {
			t.Fatalf("expected stale process metadata error, got %v", err)
		}
		if signalErr := liveProcess.Process.Signal(syscall.Signal(0)); signalErr != nil {
			t.Fatalf("expected helper process to remain alive, got %v", signalErr)
		}
	})
}

func TestRunStopCommandSucceedsWhenTerminalStateWinsAfterStopRequest(t *testing.T) {
	withStopTestWorkingDir(t, func() {
		lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
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

		pid := mustCreateExitedProcessPID(t)
		if err := runtime.WriteProcessMetadata(runtime.DefaultRunsBaseDir, runtime.ProcessMetadata{
			RunID:      lifecycle.RunID(),
			PID:        pid,
			RecordedAt: time.Now().UTC(),
			StartedAt:  time.Unix(1_700_000_000, 0).UTC(),
			Source:     runtime.RunSourceCLIRunStart,
		}); err != nil {
			t.Fatalf("expected process metadata write success, got %v", err)
		}

		stopRequested := make(chan struct{})
		go func() {
			for {
				if _, ok, readErr := runtime.ReadStopRequestMetadata(runtime.DefaultRunsBaseDir, lifecycle.RunID()); readErr == nil && ok {
					_ = lifecycle.Complete()
					close(stopRequested)
					return
				}
				time.Sleep(stopPollInterval)
			}
		}()

		stopCmd := NewStopCmd()
		var stdout bytes.Buffer
		stopCmd.SetOut(&stdout)
		setStopTestOutputFormat(t, stopCmd, clioutput.FormatJSON)
		if err := validateStopInputs(stopCmd, []string{lifecycle.RunID()}); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
		if err := runStopCommand(stopCmd, nil); err != nil {
			t.Fatalf("expected terminal-race success, got %v", err)
		}
		<-stopRequested

		var result stopResult
		if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
			t.Fatalf("expected JSON result, got %v", err)
		}
		if !result.StopRequested {
			t.Fatal("expected stop_requested=true when stop request was written")
		}
		if result.State != string(runtime.RunStateCompleted) {
			t.Fatalf("expected completed state after race, got %q", result.State)
		}
	})
}

func TestRunStopCommandWritesTextResultByDefault(t *testing.T) {
	withStopTestWorkingDir(t, func() {
		lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
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
			t.Fatalf("expected lifecycle completion success, got %v", err)
		}

		stopCmd := NewStopCmd()
		var stdout bytes.Buffer
		stopCmd.SetOut(&stdout)
		if err := validateStopInputs(stopCmd, []string{lifecycle.RunID()}); err != nil {
			t.Fatalf("expected validation success, got %v", err)
		}
		if err := runStopCommand(stopCmd, nil); err != nil {
			t.Fatalf("expected text stop result success, got %v", err)
		}

		rendered := stdout.String()
		if !strings.Contains(rendered, "Run stop result") {
			t.Fatalf("expected text stop summary, got %q", rendered)
		}
		if !strings.Contains(rendered, "Stop requested: false") {
			t.Fatalf("expected stop_requested text field, got %q", rendered)
		}
		if !strings.Contains(rendered, "State: completed") {
			t.Fatalf("expected completed state text field, got %q", rendered)
		}
	})
}

func mustCreateExitedProcessPID(t *testing.T) int {
	t.Helper()

	cmd := exec.Command("sh", "-c", "exit 0")
	if err := cmd.Start(); err != nil {
		t.Fatalf("expected process start success, got %v", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		t.Fatalf("expected process wait success, got %v", err)
	}
	return pid
}

func mustCreateLongRunningProcess(t *testing.T) *exec.Cmd {
	t.Helper()

	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("expected process start success, got %v", err)
	}
	return cmd
}

func withStopTestWorkingDir(t *testing.T, fn func()) {
	t.Helper()

	workingDir := t.TempDir()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatalf("failed to get working directory: %v", err)
	}
	if err := os.Chdir(workingDir); err != nil {
		t.Fatalf("failed to change working directory: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(originalDir)
	})

	fn()
}

func setStopTestOutputFormat(t *testing.T, cmd *cobra.Command, format clioutput.Format) {
	t.Helper()

	var boundFormat clioutput.Format
	clioutput.AddOutputFlag(cmd, &boundFormat)
	if err := cmd.PersistentFlags().Set("output", string(format)); err != nil {
		t.Fatalf("failed to set output flag: %v", err)
	}
}
