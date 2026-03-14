package subcommands

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/query"
	"github.com/spf13/cobra"
)

var inspectRunID string

// NewInspectCmd builds the run inspect command.
func NewInspectCmd() *cobra.Command {
	inspectRunID = ""

	return &cobra.Command{
		Use:   "inspect <run-id>",
		Short: "Show a detailed run inspection payload by run ID",
		Long: "sigil run inspect shows a detailed derived run inspection payload.\n\n" +
			"This command validates the requested run identifier, loads the authoritative event log and auxiliary control metadata for that run, and prints a detailed projection including node summaries and control refs.",
		Example: "# Inspect one run in text mode\n" +
			"  sigil run inspect 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Inspect one run from a custom runs directory\n" +
			"  sigil run --run-dir /tmp/sigil-runs inspect 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Return machine-readable JSON output\n" +
			"  sigil run inspect -o json 019c7714-3b77-74d1-9866-e1f484aae2ab",
		PreRunE: validateInspectInputs,
		RunE:    runInspectCommand,
	}
}

func validateInspectInputs(cmd *cobra.Command, args []string) error {
	if err := validateInheritedRunDir(cmd); err != nil {
		return err
	}
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	inspectRunID = strings.TrimSpace(args[0])
	if err := validateUUIDv7RunID("run-id", inspectRunID); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runInspectCommand(cmd *cobra.Command, _ []string) error {
	runsBaseDir, err := resolveRunsBaseDir(cmd)
	if err != nil {
		return err
	}

	projection, err := query.ReadRun(query.ReadRunRequest{
		RunsBaseDir: runsBaseDir,
		RunID:       inspectRunID,
	})
	if err != nil {
		runInspectLogger().Error("failed to inspect run", "runs_base_dir", runsBaseDir, "run_id", inspectRunID, "error", err)
		return fmt.Errorf("failed to inspect run; %w", err)
	}

	if clioutput.ResolveFormat(cmd) == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), projection); err != nil {
			return fmt.Errorf("failed to write run inspection; %w", err)
		}
	} else {
		if err := writeRunProjectionText(cmd.OutOrStdout(), "Run inspect", projection, true); err != nil {
			return err
		}
	}

	runInspectLogger().Info("run inspect command completed", "runs_base_dir", runsBaseDir, "run_id", inspectRunID, "state", projection.State)
	return nil
}

func runInspectLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.inspect")
}
