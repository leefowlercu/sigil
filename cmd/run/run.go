package run

import (
	"github.com/leefowlercu/sigil/cmd/run/subcommands"
	"github.com/spf13/cobra"
)

// NewRunCmd builds the run parent command and attaches run subcommands.
func NewRunCmd() *cobra.Command {
	runCmd := &cobra.Command{
		Use:   "run",
		Short: "Show run command usage and subcommands",
		Long: "sigil run scopes run-related command entrypoints.\n\n" +
			"This parent command provides usage discovery for start and stop run flows, including blocking harness execution through run start.",
		Example: "# Show run command usage\n" +
			"  sigil run\n\n" +
			"# Show start command usage\n" +
			"  sigil run start --help\n\n" +
			"# Show stop command usage\n" +
			"  sigil run stop --help",
		PreRunE: validateRunInputs,
		RunE:    runRunCommand,
	}

	runCmd.AddCommand(subcommands.NewStartCmd())
	runCmd.AddCommand(subcommands.NewStopCmd())

	return runCmd
}

func validateRunInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runRunCommand(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
