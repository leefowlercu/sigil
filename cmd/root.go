package cmd

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/cmd/run"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/logging"
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
		PersistentPreRunE: initializeRootApplication,
		RunE:              runRootCommand,
	}

	rootCmd.AddCommand(run.NewRunCmd())

	return rootCmd
}

func runRootCommand(cmd *cobra.Command, _ []string) error {
	rootLogger().Debug("rendering root usage help")
	return cmd.Help()
}

func initializeRootApplication(cmd *cobra.Command, _ []string) error {
	configPath := resolveApplicationConfigPath(cmd)
	if err := config.InitFromPath(configPath); err != nil {
		return fmt.Errorf("failed to initialize config; %w", err)
	}

	cfg := config.MustGet()
	if err := logging.Init(cfg); err != nil {
		return fmt.Errorf("failed to initialize application logging; %w", err)
	}

	logPath, pathErr := logging.ActiveLogFilePath()
	if pathErr != nil {
		rootLogger().Warn("application logging initialized without active path",
			"config_path", configPath,
			"log_level", cfg.LogLevel,
			"log_dir", cfg.LogDir,
			"error", pathErr,
		)
		return nil
	}

	rootLogger().Info("application logging initialized",
		"config_path", configPath,
		"log_level", cfg.LogLevel,
		"log_dir", cfg.LogDir,
		"log_path", logPath,
	)

	return nil
}

func resolveApplicationConfigPath(cmd *cobra.Command) string {
	for current := cmd; current != nil; current = current.Parent() {
		configFlag := current.Flags().Lookup("config")
		if configFlag == nil {
			continue
		}

		value := strings.TrimSpace(configFlag.Value.String())
		if value == "" {
			return config.DefaultConfigPath
		}

		return value
	}

	return config.DefaultConfigPath
}

func rootLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.root")
}
