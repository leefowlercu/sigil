package subcommands

import (
	"fmt"
	"io"
	"log/slog"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/spf13/cobra"
)

var appServerGenerateTSOutputFile string

// NewGenerateTSCmd builds the app-server generate-ts command.
func NewGenerateTSCmd() *cobra.Command {
	appServerGenerateTSOutputFile = ""

	cmd := &cobra.Command{
		Use:   "generate-ts",
		Short: "Generate TypeScript app-server protocol bindings",
		Long: "sigil app-server generate-ts renders TypeScript bindings for the typed Sigil app-server protocol.\n\n" +
			"This command emits deterministic bindings from the same typed method definitions used by the server so rich clients can track protocol changes without hand-maintained DTOs.",
		Example: "# Write TypeScript bindings to stdout\n" +
			"  sigil app-server generate-ts\n\n" +
			"# Write TypeScript bindings to a file\n" +
			"  sigil app-server generate-ts --output-file ./generated/app_server.ts",
		PreRunE: validateGenerateTSInputs,
		RunE:    runGenerateTSCommand,
	}

	cmd.Flags().StringVar(&appServerGenerateTSOutputFile, "output-file", "", "Optional file path for generated TypeScript bindings")
	return cmd
}

func validateGenerateTSInputs(cmd *cobra.Command, args []string) error {
	if err := cobra.NoArgs(cmd, args); err != nil {
		return err
	}
	if err := validateOptionalOutputFile(appServerGenerateTSOutputFile); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runGenerateTSCommand(cmd *cobra.Command, _ []string) error {
	rendered, err := protocol.GenerateTypeScriptBindings()
	if err != nil {
		return fmt.Errorf("failed to generate TypeScript bindings; %w", err)
	}
	if err := writeOutput(cmd.OutOrStdout(), appServerGenerateTSOutputFile, []byte(rendered)); err != nil {
		return fmt.Errorf("failed to write TypeScript bindings; %w", err)
	}
	appServerGenerateTSLogger().Info("generated TypeScript bindings", "output_file", appServerGenerateTSOutputFile)
	return nil
}

func appServerGenerateTSLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.app_server.generate_ts")
}

func writeOutput(writer io.Writer, target string, content []byte) error {
	if target == "" {
		if writer == nil {
			return fmt.Errorf("writer is required")
		}
		_, err := writer.Write(content)
		return err
	}
	return writeOutputFile(target, content)
}
