package subcommands

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/control"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

const (
	stopPollInterval     = control.DefaultStopPollInterval
	stopProcessExitGrace = control.DefaultStopProcessExitGrace
)

var stopRunID string

type stopResult = control.StopRunResult

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

	result, err := control.StopRun(control.StopRunRequest{
		RunsBaseDir:      runsBaseDir,
		RunID:            stopRunID,
		RequestedBy:      runtime.StopRequesterCLIRunStop,
		Signal:           runtime.StopSignalSIGTERM,
		PollInterval:     stopPollInterval,
		ProcessExitGrace: stopProcessExitGrace,
	})
	if err != nil {
		stopLogger().Error("run stop command failed", "run_id", stopRunID, "error", err)
		return err
	}
	return writeStopResult(cmd, result)
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
func stopLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.stop")
}
