package subcommands

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/leefowlercu/sigil/internal/clioutput"
	"github.com/leefowlercu/sigil/internal/query"
	"github.com/spf13/cobra"
)

var eventsRunID string

// NewEventsCmd builds the run events command.
func NewEventsCmd() *cobra.Command {
	eventsRunID = ""

	return &cobra.Command{
		Use:   "events <run-id>",
		Short: "Show canonical persisted events for one run ID",
		Long: "sigil run events shows canonical persisted events for one run.\n\n" +
			"This command validates the requested run identifier, reads the authoritative events.jsonl file for that run, and returns canonical v1 event envelopes in persisted append order.",
		Example: "# Print canonical event envelopes as JSON lines\n" +
			"  sigil run events 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Read events from a custom runs directory\n" +
			"  sigil run --run-dir /tmp/sigil-runs events 019c7714-3b77-74d1-9866-e1f484aae2ab\n\n" +
			"# Return one JSON array of events\n" +
			"  sigil run events -o json 019c7714-3b77-74d1-9866-e1f484aae2ab",
		PreRunE: validateEventsInputs,
		RunE:    runEventsCommand,
	}
}

func validateEventsInputs(cmd *cobra.Command, args []string) error {
	if err := validateInheritedRunDir(cmd); err != nil {
		return err
	}
	if err := cobra.ExactArgs(1)(cmd, args); err != nil {
		return err
	}
	eventsRunID = strings.TrimSpace(args[0])
	if err := validateUUIDv7RunID("run-id", eventsRunID); err != nil {
		return err
	}

	cmd.SilenceUsage = true
	return nil
}

func runEventsCommand(cmd *cobra.Command, _ []string) error {
	runsBaseDir, err := resolveRunsBaseDir(cmd)
	if err != nil {
		return err
	}

	events, err := query.ReadRunEvents(query.ReadRunEventsRequest{
		RunsBaseDir: runsBaseDir,
		RunID:       eventsRunID,
	})
	if err != nil {
		runEventsLogger().Error("failed to read run events", "runs_base_dir", runsBaseDir, "run_id", eventsRunID, "error", err)
		return fmt.Errorf("failed to read run events; %w", err)
	}

	if clioutput.ResolveFormat(cmd) == clioutput.FormatJSON {
		if err := clioutput.WriteJSON(cmd.OutOrStdout(), events); err != nil {
			return fmt.Errorf("failed to write run events; %w", err)
		}
	} else {
		for _, event := range events {
			if err := clioutput.WriteJSON(cmd.OutOrStdout(), event); err != nil {
				return fmt.Errorf("failed to write run events; %w", err)
			}
		}
	}

	runEventsLogger().Info("run events command completed", "runs_base_dir", runsBaseDir, "run_id", eventsRunID, "event_count", len(events))
	return nil
}

func runEventsLogger() *slog.Logger {
	return slog.Default().With("component", "cmd.run.events")
}
