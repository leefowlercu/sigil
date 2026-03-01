package subcommands

import (
	"fmt"
	"strings"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/spf13/cobra"
)

const (
	defaultStartConfigPath    = "./sigil.yaml"
	defaultStartRunConfigPath = "./sigil-run.yaml"
)

var (
	startConfigPath    string
	startRunConfigPath string
)

// NewStartCmd builds the run start command.
func NewStartCmd() *cobra.Command {
	resetStartFlags()

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Initialize run inputs and prepare run startup",
		Long: "sigil run start initializes command inputs for a run invocation.\n\n" +
			"This command resolves application and run configuration paths, validates file input constraints, and initializes application and run configuration before runtime execution behavior is introduced.",
		Example: "# Start using default config paths\n" +
			"  sigil run start\n\n" +
			"# Start with an explicit application config path\n" +
			"  sigil run start --config ./configs/sigil.yaml\n\n" +
			"# Start with explicit application and run config paths\n" +
			"  sigil run start --config ./configs/sigil.yaml --run-config ./configs/sigil-run.yaml",
		PreRunE: validateStartInputs,
		RunE:    runStartCommand,
	}

	startCmd.Flags().StringVar(&startConfigPath, "config", defaultStartConfigPath, "Path to Sigil application config file")
	startCmd.Flags().StringVar(&startRunConfigPath, "run-config", defaultStartRunConfigPath, "Path to Sigil run config file")

	return startCmd
}

func resetStartFlags() {
	startConfigPath = defaultStartConfigPath
	startRunConfigPath = defaultStartRunConfigPath
}

func validateStartInputs(cmd *cobra.Command, args []string) error {
	if err := validateNoArgs(cmd, args); err != nil {
		return err
	}

	if err := validateNonEmptyStartFlags(cmd); err != nil {
		return err
	}

	startConfigPath = resolvePathOrDefault(startConfigPath, defaultStartConfigPath)
	startRunConfigPath = resolvePathOrDefault(startRunConfigPath, defaultStartRunConfigPath)

	if err := validateReadableRegularFile(startConfigPath); err != nil {
		return fmt.Errorf("invalid --config value; %w", err)
	}

	if cmd.Flags().Changed("run-config") {
		if err := validateReadableRegularFile(startRunConfigPath); err != nil {
			return fmt.Errorf("invalid --run-config value; %w", err)
		}
	} else {
		if err := validateReadableRegularFileIfExists(startRunConfigPath); err != nil {
			return fmt.Errorf("invalid --run-config value; %w", err)
		}
	}

	cmd.SilenceUsage = true
	return nil
}

func validateNonEmptyStartFlags(cmd *cobra.Command) error {
	if cmd.Flags().Changed("config") && strings.TrimSpace(startConfigPath) == "" {
		return fmt.Errorf("invalid --config value; path cannot be empty")
	}

	if cmd.Flags().Changed("run-config") && strings.TrimSpace(startRunConfigPath) == "" {
		return fmt.Errorf("invalid --run-config value; path cannot be empty")
	}

	return nil
}

func runStartCommand(cmd *cobra.Command, _ []string) error {
	if err := config.InitFromPath(startConfigPath); err != nil {
		return fmt.Errorf("failed to initialize config; %w", err)
	}

	if cmd.Flags().Changed("run-config") {
		if err := config.InitRunFromPath(startRunConfigPath); err != nil {
			return fmt.Errorf("failed to initialize run config; %w", err)
		}

		return nil
	}

	if err := config.InitRun(); err != nil {
		return fmt.Errorf("failed to initialize run config; %w", err)
	}

	return nil
}
