package appserver

import (
	"github.com/leefowlercu/sigil/cmd/appserver/subcommands"
	"github.com/spf13/cobra"
)

// NewAppServerCmd builds the app-server parent command and subcommands.
func NewAppServerCmd() *cobra.Command {
	appServerCmd := &cobra.Command{
		Use:   "app-server",
		Short: "Show app-server usage and protocol tooling",
		Long: "sigil app-server scopes the Sigil app-server command surface.\n\n" +
			"This parent command provides protocol-serving and protocol-generation entrypoints for local app-server development and integration workflows.",
		Example: "# Show app-server command usage\n" +
			"  sigil app-server --help\n\n" +
			"# Serve the stdio app-server\n" +
			"  sigil app-server serve --listen stdio://\n\n" +
			"# Serve the localhost WebSocket app-server\n" +
			"  sigil app-server serve --listen ws://127.0.0.1:8765\n\n" +
			"# Generate TypeScript protocol bindings\n" +
			"  sigil app-server generate-ts",
		PreRunE: validateAppServerInputs,
		RunE:    runAppServerCommand,
	}

	appServerCmd.AddCommand(subcommands.NewServeCmd())
	appServerCmd.AddCommand(subcommands.NewGenerateTSCmd())
	appServerCmd.AddCommand(subcommands.NewGenerateJSONSchemaCmd())

	return appServerCmd
}

func validateAppServerInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runAppServerCommand(cmd *cobra.Command, _ []string) error {
	return cmd.Help()
}
