package subcommands

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

const (
	stopPollInterval     = 25 * time.Millisecond
	stopProcessExitGrace = 500 * time.Millisecond
)

var stopRunID string

type stopResult struct {
	RunID         string `json:"run_id"`
	StopRequested bool   `json:"stop_requested"`
	State         string `json:"state"`
	EventsPath    string `json:"events_path"`
}

// NewStopCmd builds the run stop command.
func NewStopCmd() *cobra.Command {
	stopRunID = ""

	stopCmd := &cobra.Command{
		Use:   "stop <run-id>",
		Short: "Gracefully stop an in-progress run by run ID",
		Long: "sigil run stop requests graceful interruption for one in-progress run.\n\n" +
			"This command validates the provided run ID, inspects the authoritative events.jsonl state for that run, writes stop-request metadata, sends SIGTERM to the active local CLI process, and waits until a terminal run state is observed.",
		Example: "# Gracefully stop one running CLI run\n" +
			"  sigil run stop 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Return machine-readable JSON output\n" +
			"  sigil run stop -o json 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Show stop command help\n" +
			"  sigil run stop --help",
		PreRunE: validateStopInputs,
		RunE:    runStopCommand,
	}

	return stopCmd
}

func validateStopInputs(cmd *cobra.Command, args []string) error {
	if err := validateInheritedRunDir(cmd); err != nil {
		return err
	}
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}

	trimmedRunID := strings.TrimSpace(args[0])
	if trimmedRunID == "" {
		return fmt.Errorf("run-id must be UUIDv7; value is empty")
	}
	parsed, err := uuid.Parse(trimmedRunID)
	if err != nil || parsed.Version() != uuid.Version(7) {
		if err == nil {
			err = fmt.Errorf("expected UUIDv7, got UUIDv%d", parsed.Version())
		}
		return fmt.Errorf("run-id must be UUIDv7; %w", err)
	}

	stopRunID = trimmedRunID
	cmd.SilenceUsage = true
	return nil
}

func runStopCommand(cmd *cobra.Command, _ []string) error {
	runsBaseDir, err := resolveRunsBaseDir(cmd)
	if err != nil {
		return err
	}

	status, err := runtime.ResolveRunStatus(runsBaseDir, stopRunID)
	if err != nil {
		stopLogger().Error("failed to resolve run state", "run_id", stopRunID, "error", err)
		return fmt.Errorf("failed to resolve run state; %w", err)
	}

	result := stopResult{
		RunID:      status.RunID,
		State:      string(status.State),
		EventsPath: status.EventsPath,
	}
	if runtime.IsTerminalRunState(status.State) {
		return writeStopResult(cmd, result)
	}

	processMetadata, err := runtime.ReadProcessMetadata(runsBaseDir, stopRunID)
	if err != nil {
		stopLogger().Error("failed to resolve process metadata", "run_id", stopRunID, "error", err)
		return fmt.Errorf("failed to resolve live process metadata; %w", err)
	}

	if err := runtime.WriteStopRequestMetadata(runsBaseDir, runtime.StopRequestMetadata{
		RunID:       stopRunID,
		RequestedAt: time.Now().UTC(),
		RequestedBy: runtime.StopRequesterCLIRunStop,
		Signal:      runtime.StopSignalSIGTERM,
	}); err != nil {
		stopLogger().Error("failed to persist stop request metadata", "run_id", stopRunID, "error", err)
		return fmt.Errorf("failed to persist stop request metadata; %w", err)
	}
	result.StopRequested = true

	if err := runtime.ValidateLiveProcessMetadata(processMetadata); err != nil {
		if errors.Is(err, runtime.ErrProcessNotRunning) {
			return waitForTerminalStopState(cmd, runsBaseDir, result, processMetadata, time.Now().Add(stopProcessExitGrace))
		}
		stopLogger().Error("failed to validate process identity", "run_id", stopRunID, "pid", processMetadata.PID, "error", err)
		return fmt.Errorf("failed to validate live process metadata; %w", err)
	}

	process, err := os.FindProcess(processMetadata.PID)
	if err != nil {
		stopLogger().Error("failed to resolve process handle", "run_id", stopRunID, "pid", processMetadata.PID, "error", err)
		return fmt.Errorf("failed to resolve process handle; %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if !isExitedProcessError(err) {
			stopLogger().Error("failed to signal process", "run_id", stopRunID, "pid", processMetadata.PID, "error", err)
			return fmt.Errorf("failed to signal run process; %w", err)
		}
		return waitForTerminalStopState(cmd, runsBaseDir, result, processMetadata, time.Now().Add(stopProcessExitGrace))
	}

	return waitForTerminalStopState(cmd, runsBaseDir, result, processMetadata, time.Time{})
}

func waitForTerminalStopState(cmd *cobra.Command, runsBaseDir string, result stopResult, processMetadata runtime.ProcessMetadata, processExitDeadline time.Time) error {
	for {
		status, err := runtime.ResolveRunStatus(runsBaseDir, stopRunID)
		if err != nil {
			stopLogger().Error("failed to poll run state", "run_id", stopRunID, "error", err)
			return fmt.Errorf("failed to poll run state; %w", err)
		}
		result.State = string(status.State)
		result.EventsPath = status.EventsPath
		if runtime.IsTerminalRunState(status.State) {
			return writeStopResult(cmd, result)
		}

		running, err := runtime.IsOriginalProcessRunning(processMetadata)
		if err != nil {
			stopLogger().Error("failed to poll process liveness", "run_id", stopRunID, "pid", processMetadata.PID, "error", err)
			return fmt.Errorf("failed to poll run process; %w", err)
		}
		if !running {
			if processExitDeadline.IsZero() {
				processExitDeadline = time.Now().Add(stopProcessExitGrace)
			}
			if time.Now().After(processExitDeadline) {
				return fmt.Errorf("run process exited before terminal state was observed")
			}
		}

		time.Sleep(stopPollInterval)
	}
}

func writeStopResult(cmd *cobra.Command, result stopResult) error {
	if clioutput.ResolveFormat(cmd) == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), result); err != nil {
			stopLogger().Error("failed to write stop result", "run_id", result.RunID, "error", err)
			return fmt.Errorf("failed to write stop result; %w", err)
		}
	} else {
		if err := clioutput.WriteStopText(cmd.OutOrStdout(), result.RunID, result.StopRequested, result.State, result.EventsPath); err != nil {
			stopLogger().Error("failed to write stop result", "run_id", result.RunID, "error", err)
			return fmt.Errorf("failed to write stop result; %w", err)
		}
	}
	stopLogger().Info("run stop command completed",
		"run_id", result.RunID,
		"stop_requested", result.StopRequested,
		"state", result.State,
		"events_path", result.EventsPath,
	)
	return nil
}

func isExitedProcessError(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

func stopLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.stop")
}
