package subcommands

import (
	"fmt"
	"log/slog"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

// NewListCmd builds the run list command.
func NewListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "Show run summaries for one runs directory",
		Long: "sigil run list shows derived summaries for persisted runs.\n\n" +
			"This command scans the selected runs directory, validates each discovered run event log, and returns best-effort operator summaries ordered from newest to oldest.",
		Example: "# List runs in the default runs directory\n" +
			"  sigil run list\n\n" +
			"# List runs in a custom runs directory\n" +
			"  sigil run --run-dir /tmp/sigil-runs list\n\n" +
			"# Return machine-readable JSON output\n" +
			"  sigil run list -o json",
		PreRunE: validateListInputs,
		RunE:    runListCommand,
	}
}

func validateListInputs(cmd *cobra.Command, args []string) error {
	if err := validateInheritedRunDir(cmd); err != nil {
		return err
	}
	if err := validateNoArgs(cmd, args); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runListCommand(cmd *cobra.Command, _ []string) error {
	runsBaseDir, err := resolveRunsBaseDir(cmd)
	if err != nil {
		return err
	}

	summaries, err := runtime.ListRuns(runsBaseDir)
	if err != nil {
		runListLogger().Error("failed to list runs", "runs_base_dir", runsBaseDir, "error", err)
		return fmt.Errorf("failed to list runs; %w", err)
	}

	if clioutput.ResolveFormat(cmd) == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), summaries); err != nil {
			return fmt.Errorf("failed to write run list; %w", err)
		}
	} else {
		if err := writeRunListText(cmd.OutOrStdout(), runsBaseDir, summaries); err != nil {
			return err
		}
	}

	runListLogger().Info("run list command completed", "runs_base_dir", runsBaseDir, "run_count", len(summaries))
	return nil
}

func runListLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.list")
}
