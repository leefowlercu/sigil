package subcommands

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/runtime"
	"github.com/spf13/cobra"
)

const runDirFlagName = "run-dir"

func validateNoArgs(cmd *cobra.Command, args []string) error {
	return cobra.NoArgs(cmd, args)
}

// DefaultRunDir returns the canonical default runs base directory.
func DefaultRunDir() string {
	return runtime.DefaultRunsBaseDir
}

// ValidateRunDirFlag validates an inherited run-dir flag value.
func ValidateRunDirFlag(path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("invalid --run-dir value; path cannot be empty")
	}

	return nil
}

func resolveRunsBaseDir(cmd *cobra.Command) (string, error) {
	runDir, changed, err := inheritedStringFlag(cmd, runDirFlagName)
	if err != nil {
		return "", err
	}
	if !changed || strings.TrimSpace(runDir) == "" {
		runDir = runtime.DefaultRunsBaseDir
	}
	if err := ValidateRunDirFlag(runDir); err != nil {
		return "", err
	}

	resolved, err := runtime.ResolveRunsBaseDir(runDir)
	if err != nil {
		return "", fmt.Errorf("failed to resolve run directory; %w", err)
	}

	return resolved, nil
}

func validateInheritedRunDir(cmd *cobra.Command) error {
	runDir, changed, err := inheritedStringFlag(cmd, runDirFlagName)
	if err != nil {
		return err
	}
	if !changed {
		return nil
	}

	return ValidateRunDirFlag(runDir)
}

func resolvePathOrDefault(path string, fallback string) string {
	if path == "" {
		return fallback
	}

	return path
}

func validateReadableRegularFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("failed to stat path %q; %w", path, err)
	}

	if !info.Mode().IsRegular() {
		return fmt.Errorf("path %q is not a regular file", path)
	}

	file, err := os.Open(filepath.Clean(path))
	if err != nil {
		return fmt.Errorf("failed to open file %q; %w", path, err)
	}

	if err := file.Close(); err != nil {
		return fmt.Errorf("failed to close file %q; %w", path, err)
	}

	return nil
}

func validateReadableRegularFileIfExists(path string) error {
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}

		return fmt.Errorf("failed to stat path %q; %w", path, err)
	}

	return validateReadableRegularFile(path)
}

func inheritedStringFlag(cmd *cobra.Command, name string) (string, bool, error) {
	for current := cmd; current != nil; current = current.Parent() {
		flag := current.Flags().Lookup(name)
		if flag == nil {
			flag = current.PersistentFlags().Lookup(name)
		}
		if flag == nil {
			continue
		}

		value := flag.Value.String()
		changed := current.Flags().Changed(name) || current.PersistentFlags().Changed(name)
		return value, changed, nil
	}

	return "", false, nil
}

func formatOptionalTime(value *time.Time) string {
	if value == nil || value.IsZero() {
		return "-"
	}

	return value.UTC().Format(time.RFC3339)
}

func validateUUIDv7RunID(name string, value string) error {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return fmt.Errorf("%s must be UUIDv7; value is empty", name)
	}

	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed.Version() != uuid.Version(7) {
		if err == nil {
			err = fmt.Errorf("expected UUIDv7, got UUIDv%d", parsed.Version())
		}
		return fmt.Errorf("%s must be UUIDv7; %w", name, err)
	}

	return nil
}

func writeRunListText(writer io.Writer, runsBaseDir string, summaries []runtime.RunSummary) error {
	if writer == nil {
		return fmt.Errorf("writer is required")
	}

	if _, err := fmt.Fprintf(writer, "Run list\n  Runs dir: %s\n  Runs: %d\n", runsBaseDir, len(summaries)); err != nil {
		return fmt.Errorf("failed to write run list header; %w", err)
	}

	for _, summary := range summaries {
		if _, err := fmt.Fprintf(
			writer,
			"  - Run ID: %s\n    State: %s\n    Source: %s\n    Queued at: %s\n    Started at: %s\n    Terminal at: %s\n    PID status: %s\n    Stop requested: %t\n    Events path: %s\n",
			summary.RunID,
			valueOrPlaceholder(summary.State),
			valueOrPlaceholder(summary.Source),
			formatOptionalTime(summary.QueuedAt),
			formatOptionalTime(summary.StartedAt),
			formatOptionalTime(summary.TerminalAt),
			valueOrPlaceholder(summary.PIDStatus),
			summary.StopRequested,
			valueOrPlaceholder(summary.EventsPath),
		); err != nil {
			return fmt.Errorf("failed to write run list item; %w", err)
		}
		if summary.FinalAnswerRef != nil {
			if _, err := fmt.Fprintf(writer, "    Final answer ref: %s\n", *summary.FinalAnswerRef); err != nil {
				return fmt.Errorf("failed to write run final answer ref; %w", err)
			}
		}
		if summary.AccountingRef != nil {
			if _, err := fmt.Fprintf(writer, "    Accounting ref: %s\n", *summary.AccountingRef); err != nil {
				return fmt.Errorf("failed to write run accounting ref; %w", err)
			}
		}
		if summary.Error != "" {
			if _, err := fmt.Fprintf(writer, "    Error: %s\n", summary.Error); err != nil {
				return fmt.Errorf("failed to write run list error; %w", err)
			}
		}
	}

	return nil
}

func writeRunSummaryText(writer io.Writer, heading string, summary runtime.RunSummary) error {
	if writer == nil {
		return fmt.Errorf("writer is required")
	}

	if _, err := fmt.Fprintf(
		writer,
		"%s\n  Run ID: %s\n  State: %s\n  Source: %s\n  Events path: %s\n  Queued at: %s\n  Started at: %s\n  Terminal at: %s\n  PID status: %s\n  Stop requested: %t\n",
		heading,
		summary.RunID,
		valueOrPlaceholder(summary.State),
		valueOrPlaceholder(summary.Source),
		valueOrPlaceholder(summary.EventsPath),
		formatOptionalTime(summary.QueuedAt),
		formatOptionalTime(summary.StartedAt),
		formatOptionalTime(summary.TerminalAt),
		valueOrPlaceholder(summary.PIDStatus),
		summary.StopRequested,
	); err != nil {
		return fmt.Errorf("failed to write run summary header; %w", err)
	}

	if summary.FinalAnswerRef != nil {
		if _, err := fmt.Fprintf(writer, "  Final answer ref: %s\n", *summary.FinalAnswerRef); err != nil {
			return fmt.Errorf("failed to write final answer ref; %w", err)
		}
	}
	if summary.AccountingRef != nil {
		if _, err := fmt.Fprintf(writer, "  Accounting ref: %s\n", *summary.AccountingRef); err != nil {
			return fmt.Errorf("failed to write accounting ref; %w", err)
		}
	}
	if summary.Error != "" {
		if _, err := fmt.Fprintf(writer, "  Error: %s\n", summary.Error); err != nil {
			return fmt.Errorf("failed to write run summary error; %w", err)
		}
	}

	return nil
}

func writeRunProjectionText(writer io.Writer, heading string, projection runtime.RunProjection, verbose bool) error {
	if writer == nil {
		return fmt.Errorf("writer is required")
	}

	if _, err := fmt.Fprintf(
		writer,
		"%s\n  Run ID: %s\n  State: %s\n  Source: %s\n  App config path: %s\n  Run config path: %s\n  Executor: %s\n  Max depth: %d\n  Runs dir: %s\n  Events path: %s\n  Queued at: %s\n  Started at: %s\n  Terminal at: %s\n  PID status: %s\n  Stop requested: %t\n  Node count: %d\n  Step count: %d\n  Action count: %d\n  Subcall count: %d\n",
		heading,
		projection.RunID,
		valueOrPlaceholder(projection.State),
		valueOrPlaceholder(projection.Source),
		valueOrPlaceholderPointer(projection.AppConfigPath),
		valueOrPlaceholderPointer(projection.RunConfigPath),
		valueOrPlaceholder(projection.Executor),
		projection.MaxDepth,
		valueOrPlaceholder(projection.RunDir),
		valueOrPlaceholder(projection.EventsPath),
		formatOptionalTime(projection.QueuedAt),
		formatOptionalTime(projection.StartedAt),
		formatOptionalTime(projection.TerminalAt),
		valueOrPlaceholder(projection.PIDStatus),
		projection.StopRequested,
		projection.NodeCount,
		projection.StepCount,
		projection.ActionCount,
		projection.SubcallCount,
	); err != nil {
		return fmt.Errorf("failed to write run projection header; %w", err)
	}

	if projection.FinalAnswerRef != nil {
		if _, err := fmt.Fprintf(writer, "  Final answer ref: %s\n", *projection.FinalAnswerRef); err != nil {
			return fmt.Errorf("failed to write final answer ref; %w", err)
		}
	}
	if projection.AccountingRef != nil {
		if _, err := fmt.Fprintf(writer, "  Accounting ref: %s\n", *projection.AccountingRef); err != nil {
			return fmt.Errorf("failed to write accounting ref; %w", err)
		}
	}
	if projection.ErrorCode != nil {
		if _, err := fmt.Fprintf(writer, "  Error code: %s\n", *projection.ErrorCode); err != nil {
			return fmt.Errorf("failed to write error code; %w", err)
		}
	}
	if projection.ErrorMessage != nil {
		if _, err := fmt.Fprintf(writer, "  Error message: %s\n", *projection.ErrorMessage); err != nil {
			return fmt.Errorf("failed to write error message; %w", err)
		}
	}
	if projection.FailedNodeID != nil {
		if _, err := fmt.Fprintf(writer, "  Failed node ID: %s\n", *projection.FailedNodeID); err != nil {
			return fmt.Errorf("failed to write failed node ID; %w", err)
		}
	}
	if projection.FailedStepID != nil {
		if _, err := fmt.Fprintf(writer, "  Failed step ID: %s\n", *projection.FailedStepID); err != nil {
			return fmt.Errorf("failed to write failed step ID; %w", err)
		}
	}
	if projection.InterruptedReason != nil {
		if _, err := fmt.Fprintf(writer, "  Interrupted reason: %s\n", *projection.InterruptedReason); err != nil {
			return fmt.Errorf("failed to write interrupted reason; %w", err)
		}
	}
	if projection.InterruptedBy != nil {
		if _, err := fmt.Fprintf(writer, "  Interrupted by: %s\n", *projection.InterruptedBy); err != nil {
			return fmt.Errorf("failed to write interrupted by; %w", err)
		}
	}
	if projection.InterruptedNodeID != nil {
		if _, err := fmt.Fprintf(writer, "  Interrupted node ID: %s\n", *projection.InterruptedNodeID); err != nil {
			return fmt.Errorf("failed to write interrupted node ID; %w", err)
		}
	}
	if !verbose {
		return nil
	}

	if projection.ProcessMetadata != nil {
		if _, err := fmt.Fprintf(
			writer,
			"  Process metadata: pid=%d recorded_at=%s started_at=%s source=%s\n",
			projection.ProcessMetadata.PID,
			projection.ProcessMetadata.RecordedAt.UTC().Format(time.RFC3339),
			projection.ProcessMetadata.StartedAt.UTC().Format(time.RFC3339),
			projection.ProcessMetadata.Source,
		); err != nil {
			return fmt.Errorf("failed to write process metadata; %w", err)
		}
	}
	if projection.StopRequest != nil {
		if _, err := fmt.Fprintf(
			writer,
			"  Stop request: requested_at=%s requested_by=%s signal=%s\n",
			projection.StopRequest.RequestedAt.UTC().Format(time.RFC3339),
			projection.StopRequest.RequestedBy,
			projection.StopRequest.Signal,
		); err != nil {
			return fmt.Errorf("failed to write stop request metadata; %w", err)
		}
	}

	for _, node := range projection.Nodes {
		if _, err := fmt.Fprintf(
			writer,
			"  Node: %s\n    State: %s\n    Depth: %d\n    Role: %s\n    Parent node ID: %s\n    Started at: %s\n    Terminal at: %s\n    Step count: %d\n",
			node.NodeID,
			valueOrPlaceholder(node.State),
			node.Depth,
			valueOrPlaceholder(node.Role),
			valueOrPlaceholderPointer(node.ParentNodeID),
			formatOptionalTime(node.StartedAt),
			formatOptionalTime(node.TerminalAt),
			node.StepCount,
		); err != nil {
			return fmt.Errorf("failed to write node projection; %w", err)
		}
		if node.ResultRef != nil {
			if _, err := fmt.Fprintf(writer, "    Result ref: %s\n", *node.ResultRef); err != nil {
				return fmt.Errorf("failed to write node result ref; %w", err)
			}
		}
		if node.AccountingRef != nil {
			if _, err := fmt.Fprintf(writer, "    Accounting ref: %s\n", *node.AccountingRef); err != nil {
				return fmt.Errorf("failed to write node accounting ref; %w", err)
			}
		}
		if node.ErrorCode != nil {
			if _, err := fmt.Fprintf(writer, "    Error code: %s\n", *node.ErrorCode); err != nil {
				return fmt.Errorf("failed to write node error code; %w", err)
			}
		}
		if node.ErrorMessage != nil {
			if _, err := fmt.Fprintf(writer, "    Error message: %s\n", *node.ErrorMessage); err != nil {
				return fmt.Errorf("failed to write node error message; %w", err)
			}
		}
	}

	return nil
}

func valueOrPlaceholder(value string) string {
	if strings.TrimSpace(value) == "" {
		return "-"
	}

	return value
}

func valueOrPlaceholderPointer(value *string) string {
	if value == nil {
		return "-"
	}

	return valueOrPlaceholder(*value)
}
