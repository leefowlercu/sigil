package run

import (
	"github.com/leefowlercu/sigil/cmd/run/subcommands"
	"github.com/spf13/cobra"
)

var runRunsDir string

// NewRunCmd builds the run parent command and attaches run subcommands.
func NewRunCmd() *cobra.Command {
	resetRunFlags()

	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Show run-control usage and subcommands",
		Long: "sigil run scopes run-related command entrypoints.\n\n" +
			"This parent command provides usage discovery for run execution, inspection, and control flows over the selected runs storage directory.",
		Example: "# Show run command usage\n" +
			"  sigil run\n\n" +
			"# Show runs stored in a custom directory\n" +
			"  sigil run --run-dir /tmp/sigil-runs list\n\n" +
			"# Show start command usage\n" +
			"  sigil run start --help\n\n" +
			"# Show stop command usage\n" +
			"  sigil run stop --help",
		PreRunE: validateRunInputs,
		RunE:    runRunCommand,
	}

	runCmd.PersistentFlags().StringVar(&runRunsDir, "run-dir", subcommands.DefaultRunDir(), "Path to Sigil runs base directory")
	runCmd.AddCommand(subcommands.NewStartCmd())
	runCmd.AddCommand(subcommands.NewStopCmd())
	runCmd.AddCommand(subcommands.NewListCmd())
	runCmd.AddCommand(subcommands.NewStatusCmd())
	runCmd.AddCommand(subcommands.NewInspectCmd())
	runCmd.AddCommand(subcommands.NewEventsCmd())

	return runCmd
}

func resetRunFlags() {
	runRunsDir = subcommands.DefaultRunDir()
}

func validateRunInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}
	if err := subcommands.ValidateRunDirFlag(runRunsDir); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runRunCommand(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
