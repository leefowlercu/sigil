package subcommands

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/harness"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

const (
	defaultStartConfigPath    = "./sigil.yaml"
	defaultStartRunConfigPath = "./sigil-run.yaml"
)

var (
	startConfigPath     string
	startRunConfigPath  string
	startTemplateVarRaw []string
	startTemplateVars   map[string]string
)

// NewStartCmd builds the run start command.
func NewStartCmd() *cobra.Command {
	resetStartFlags()

	startCmd := &cobra.Command{
		Use:   "start",
		Short: "Execute a blocking harness run from resolved config inputs",
		Long: "sigil run start initializes config inputs and executes the harness synchronously.\n\n" +
			"This command resolves application and run configuration paths, validates command input constraints, renders prompt/context templates when configured, and runs the harness until the run reaches a terminal state.",
		Example: "# Start using default config paths\n" +
			"  sigil run start\n\n" +
			"# Start with explicit config paths\n" +
			"  sigil run start --config ./configs/sigil.yaml --run-config ./configs/sigil-run.yaml\n\n" +
			"# Start with machine-readable JSON output\n" +
			"  sigil run start -o json\n\n" +
			"# Start with template variables\n" +
			"  sigil run start --var customer=acme --var environment=prod",
		PreRunE: validateStartInputs,
		RunE:    runStartCommand,
	}

	startCmd.Flags().StringVar(&startConfigPath, "config", defaultStartConfigPath, "Path to Sigil application config file")
	startCmd.Flags().StringVar(&startRunConfigPath, "run-config", defaultStartRunConfigPath, "Path to Sigil run config file")
	startCmd.Flags().StringArrayVar(&startTemplateVarRaw, "var", nil, "Template variable in key=value format (repeatable)")

	return startCmd
}

func resetStartFlags() {
	startConfigPath = defaultStartConfigPath
	startRunConfigPath = defaultStartRunConfigPath
	startTemplateVarRaw = nil
	startTemplateVars = make(map[string]string)
}

func validateStartInputs(cmd *cobra.Command, args []string) error {
	if err := validateNoArgs(cmd, args); err != nil {
		return err
	}

	if err := validateNonEmptyStartFlags(cmd); err != nil {
		return err
	}

	resolvedVars, err := parseTemplateVars(startTemplateVarRaw)
	if err != nil {
		return fmt.Errorf("invalid --var value; %w", err)
	}
	startTemplateVars = resolvedVars

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

	runStartLogger().Info("validated run start inputs",
		"config_path", startConfigPath,
		"run_config_path", startRunConfigPath,
		"template_var_count", len(startTemplateVars),
		"explicit_run_config", cmd.Flags().Changed("run-config"),
	)

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

func parseTemplateVars(entries []string) (map[string]string, error) {
	resolved := make(map[string]string, len(entries))
	for _, entry := range entries {
		trimmed := strings.TrimSpace(entry)
		if trimmed == "" {
			return nil, fmt.Errorf("entry cannot be empty")
		}

		separator := strings.Index(trimmed, "=")
		if separator < 1 {
			return nil, fmt.Errorf("entry %q must use key=value format with non-empty key", entry)
		}

		key := strings.TrimSpace(trimmed[:separator])
		value := ""
		if separator+1 < len(trimmed) {
			value = trimmed[separator+1:]
		}
		if key == "" {
			return nil, fmt.Errorf("entry %q must use key=value format with non-empty key", entry)
		}

		resolved[key] = value
	}

	return resolved, nil
}

func runStartCommand(cmd *cobra.Command, _ []string) error {
	runStartLogger().Info("run start command beginning",
		"config_path", startConfigPath,
		"run_config_path", startRunConfigPath,
		"template_var_count", len(startTemplateVars),
	)

	if err := config.InitFromPath(startConfigPath); err != nil {
		runStartLogger().Error("failed to initialize application config",
			"config_path", startConfigPath,
			"error", err,
		)
		return fmt.Errorf("failed to initialize config; %w", err)
	}

	if cmd.Flags().Changed("run-config") {
		if err := config.InitRunFromPath(startRunConfigPath); err != nil {
			runStartLogger().Error("failed to initialize run config from explicit path",
				"run_config_path", startRunConfigPath,
				"error", err,
			)
			return fmt.Errorf("failed to initialize run config; %w", err)
		}
	} else {
		if err := config.InitRun(); err != nil {
			runStartLogger().Error("failed to initialize run config from default path",
				"run_config_path", startRunConfigPath,
				"error", err,
			)
			return fmt.Errorf("failed to initialize run config; %w", err)
		}
	}

	runCfg := config.MustGetRun()
	runStartLogger().Info("resolved run configuration",
		"llm_gateway", runCfg.LLM.Gateway,
		"llm_provider", runCfg.LLM.Provider,
		"llm_model", runCfg.LLM.Model,
		"reasoning_enabled", runCfg.LLM.Reasoning.Enabled,
		"rlm_enabled", runCfg.RLM.Enabled,
		"rlm_max_depth", runCfg.RLM.MaxDepth,
	)

	format := clioutput.ResolveFormat(cmd)
	var textRenderer *clioutput.StartTextRenderer
	runnerOptions := make([]harness.RunnerOption, 0, 1)
	if format == clioutput.FormatText {
		textRenderer = clioutput.NewStartTextRenderer(cmd.OutOrStdout())
		textRenderer.WritePreflight(clioutput.StartPreflight{
			ConfigPath:       startConfigPath,
			RunConfigPath:    startRunConfigPath,
			Gateway:          runCfg.LLM.Gateway,
			Provider:         runCfg.LLM.Provider,
			Model:            runCfg.LLM.Model,
			ReasoningEnabled: runCfg.LLM.Reasoning.Enabled,
			RLMEnabled:       runCfg.RLM.Enabled,
			RLMMaxDepth:      runCfg.RLM.MaxDepth,
		})
		runnerOptions = append(runnerOptions, harness.WithEventObserver(textRenderer.ObserveEvent))
	}

	runner := harness.NewRunner(runnerOptions...)
	runContext, cancel := context.WithCancelCause(cmd.Context())
	defer cancel(nil)
	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM)
	defer signal.Stop(signalCh)
	go func() {
		select {
		case <-signalCh:
			cancel(runtime.ErrExternalStopRequested)
		case <-runContext.Done():
		}
	}()

	result, err := runner.Run(runContext, harness.RunInput{
		AppConfigPath: startConfigPath,
		RunConfigPath: startRunConfigPath,
		RunConfig:     runCfg,
		TemplateVars:  cloneStringMap(startTemplateVars),
	})
	if err != nil {
		logAttrs := []any{"error", err}
		if code, ok := harness.CodeOf(err); ok {
			logAttrs = append(logAttrs, "error_code", code)
		}
		runStartLogger().Error("run start command failed", logAttrs...)
		if textRenderer != nil && textRenderer.Err() != nil {
			return fmt.Errorf("failed to write run progress output; %w", textRenderer.Err())
		}
		return fmt.Errorf("failed to execute harness run; %w", err)
	}

	if format == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), result); err != nil {
			runStartLogger().Error("failed to write run summary output",
				"run_id", result.RunID,
				"error", err,
			)
			return fmt.Errorf("failed to write run summary; %w", err)
		}
	} else if textRenderer != nil {
		textRenderer.WriteCompletedSummary(result)
		if err := textRenderer.Err(); err != nil {
			runStartLogger().Error("failed to write run summary output",
				"run_id", result.RunID,
				"error", err,
			)
			return fmt.Errorf("failed to write run summary; %w", err)
		}
	}

	runStartLogger().Info("run start command completed",
		"run_id", result.RunID,
		"state", result.State,
		"events_path", result.EventsPath,
		"final_answer_ref", result.FinalAnswerRef,
	)

	return nil
}

func cloneStringMap(input map[string]string) map[string]string {
	if input == nil {
		return map[string]string{}
	}

	cloned := make(map[string]string, len(input))
	for key, value := range input {
		cloned[key] = value
	}

	return cloned
}

func runStartLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.start")
}
