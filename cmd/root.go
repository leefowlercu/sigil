package cmd

import (
	"github.com/leefowlercu/sigil/cmd/run"
	"github.com/spf13/cobra"
)

// Execute runs the root CLI command for sigil.
func Execute() error {
	return NewRootCmd().Execute()
}

// NewRootCmd builds the root command and full command tree.
func NewRootCmd() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "sigil",
		Short: "Show Sigil command usage and entrypoints",
		Long: "sigil exposes the harness command-line surface.\n\n" +
			"This root command provides top-level command discovery for run management workflows.",
		Example: "# Show root help and command tree\n" +
			"  sigil --help\n\n" +
			"# Show run command help\n" +
			"  sigil run --help",
		RunE: runRootCommand,
	}

	rootCmd.AddCommand(run.NewRunCmd())

	return rootCmd
}

func runRootCommand(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
