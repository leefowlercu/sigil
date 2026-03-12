package subcommands

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

var statusRunID string

// NewStatusCmd builds the run status command.
func NewStatusCmd() *cobra.Command {
	statusRunID = ""

	return &cobra.Command{
		Use:   "status <run-id>",
		Short: "Show one compact run status by run ID",
		Long: "sigil run status shows one compact derived run summary.\n\n" +
			"This command validates the requested run identifier, loads the authoritative event log for that run, derives current state and control metadata, and prints a concise operator status view.",
		Example: "# Show run status in text mode\n" +
			"  sigil run status 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Show run status from a custom runs directory\n" +
			"  sigil run --run-dir /tmp/sigil-runs status 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Return machine-readable JSON output\n" +
			"  sigil run status -o json 019c7714-3b77-74d1-9866-e1f484aae2ab",
		PreRunE: validateStatusInputs,
		RunE:    runStatusCommand,
	}
}

func validateStatusInputs(cmd *cobra.Command, args []string) error {
	if err := validateInheritedRunDir(cmd); err != nil {
		return err
	}
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	statusRunID = strings.TrimSpace(args[0])
	if err := validateUUIDv7RunID("run-id", statusRunID); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runStatusCommand(cmd *cobra.Command, _ []string) error {
	runsBaseDir, err := resolveRunsBaseDir(cmd)
	if err != nil {
		return err
	}

	summary, err := runtime.LoadRunSummary(runsBaseDir, statusRunID)
	if err != nil {
		runStatusLogger().Error("failed to load run status", "runs_base_dir", runsBaseDir, "run_id", statusRunID, "error", err)
		return fmt.Errorf("failed to load run status; %w", err)
	}

	if clioutput.ResolveFormat(cmd) == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), summary); err != nil {
			return fmt.Errorf("failed to write run status; %w", err)
		}
	} else {
		if err := writeRunSummaryText(cmd.OutOrStdout(), "Run status", summary); err != nil {
			return err
		}
	}

	runStatusLogger().Info("run status command completed", "runs_base_dir", runsBaseDir, "run_id", statusRunID, "state", summary.State)
	return nil
}

func runStatusLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.status")
}
