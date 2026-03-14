package subcommands

import (
	"fmt"
	"log/slog"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/spf13/cobra"
)

var appServerGenerateJSONSchemaOutputFile string

// NewGenerateJSONSchemaCmd builds the app-server generate-json-schema command.
func NewGenerateJSONSchemaCmd() *cobra.Command {
	appServerGenerateJSONSchemaOutputFile = ""

	cmd := &cobra.Command{
		Use:   "generate-json-schema",
		Short: "Generate JSON Schema for the app-server protocol",
		Long: "sigil app-server generate-json-schema renders JSON Schema for the typed Sigil app-server protocol.\n\n" +
			"This command emits a deterministic schema bundle from the same typed method definitions used by the server so validation tooling and non-TypeScript clients can consume the protocol contract directly.",
		Example: "# Write JSON Schema to stdout\n" +
			"  sigil app-server generate-json-schema\n\n" +
			"# Write JSON Schema to a file\n" +
			"  sigil app-server generate-json-schema --output-file ./generated/app_server.schema.json",
		PreRunE: validateGenerateJSONSchemaInputs,
		RunE:    runGenerateJSONSchemaCommand,
	}

	cmd.Flags().StringVar(&appServerGenerateJSONSchemaOutputFile, "output-file", "", "Optional file path for generated JSON Schema")
	return cmd
}

func validateGenerateJSONSchemaInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}
	if err := validateOptionalOutputFile(appServerGenerateJSONSchemaOutputFile); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runGenerateJSONSchemaCommand(cmd *cobra.Command, _ []string) error {
	rendered, err := protocol.GenerateJSONSchemaBundle()
	if err != nil {
		return fmt.Errorf("failed to generate JSON Schema; %w", err)
	}
	if err := writeOutput(cmd.OutOrStdout(), appServerGenerateJSONSchemaOutputFile, rendered); err != nil {
		return fmt.Errorf("failed to write JSON Schema; %w", err)
	}
	appServerGenerateJSONSchemaLogger().Info("generated JSON Schema bundle", "output_file", appServerGenerateJSONSchemaOutputFile)
	return nil
}

func appServerGenerateJSONSchemaLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.app_server.generate_json_schema")
}
