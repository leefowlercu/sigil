package subcommands

import "github.com/spf13/cobra"

// NewStopCmd builds the run stop placeholder command.
func NewStopCmd() *cobra.Command {
	stopCmd := &cobra.Command{
		Use:   "stop",
		Short: "Show stop command usage for run termination",
		Long: "sigil run stop provides the run-stop command surface.\n\n" +
			"This command is a usage-only placeholder until stop-runtime behavior is defined in a follow-up PRD and implemented.",
		Example: "# Show stop command usage\n" +
			"  sigil run stop\n\n" +
			"# Show stop help output\n" +
			"  sigil run stop --help",
		PreRunE: validateStopInputs,
		RunE:    runStopCommand,
	}

	return stopCmd
}

func validateStopInputs(cmd *cobra.Command, args []string) error {
	if err := validateNoArgs(cmd, args); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runStopCommand(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
