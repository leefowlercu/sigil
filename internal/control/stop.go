package control

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	// DefaultStopPollInterval is the shared poll interval for graceful stop waits.
	DefaultStopPollInterval = 25 * time.Millisecond
	// DefaultStopProcessExitGrace is the shared grace window after an observed process exit.
	DefaultStopProcessExitGrace = 500 * time.Millisecond
)

var (
	// ErrInvalidRunID indicates the supplied run identifier is not UUIDv7.
	ErrInvalidRunID = errors.New("invalid run id")
	// ErrRunNotFound indicates the selected run does not exist in the chosen corpus.
	ErrRunNotFound = errors.New("run not found")
	// ErrProcessMetadataUnavailable indicates non-terminal stop control metadata is missing.
	ErrProcessMetadataUnavailable = errors.New("process metadata unavailable")
	// ErrStaleProcessMetadata indicates the stored process metadata no longer identifies the original process.
	ErrStaleProcessMetadata = errors.New("stale process metadata")
	// ErrCurrentProcessFastPathRequired indicates the target resolves to the current process and must use a fast path.
	ErrCurrentProcessFastPathRequired = errors.New("current process stop requires fast path")
	// ErrTerminalStateNotObserved indicates the target process exited before a terminal run state was observed.
	ErrTerminalStateNotObserved = errors.New("terminal state not observed")
)

// StopRunRequest defines one shared graceful-stop request.
type StopRunRequest struct {
	RunsBaseDir      string
	RunID            string
	RequestedBy      string
	Signal           string
	Now              func() time.Time
	PollInterval     time.Duration
	ProcessExitGrace time.Duration
	InProcessStop    func(runID string, metadata runtime.ProcessMetadata) bool
}

// StopRunResult defines one transport-neutral graceful-stop result.
type StopRunResult struct {
	RunID                 string `json:"run_id"`
	StopRequested         bool   `json:"stop_requested"`
	State                 string `json:"state"`
	EventsPath            string `json:"events_path"`
	UsedInProcessFastPath bool   `json:"-"`
}

type normalizedStopRunRequest struct {
	StopRunRequest
	now func() time.Time
}

// StopRun executes the shared graceful-stop contract used by the CLI and app-server.
func StopRun(request StopRunRequest) (StopRunResult, error) {
	normalized, err := normalizeStopRunRequest(request)
	if err != nil {
		return StopRunResult{}, err
	}

	logger := stopLogger(normalized.RunID)
	logger.Info("starting shared run stop request",
		"runs_base_dir", normalized.RunsBaseDir,
		"requested_by", normalized.RequestedBy,
		"signal", normalized.Signal,
	)

	status, err := runtime.ResolveRunStatus(normalized.RunsBaseDir, normalized.RunID)
	if err != nil {
		logger.Error("failed to resolve run state", "error", err)
		return StopRunResult{}, wrapResolveRunStatusError(err)
	}

	result := StopRunResult{
		RunID:      status.RunID,
		State:      string(status.State),
		EventsPath: status.EventsPath,
	}
	if runtime.IsTerminalRunState(status.State) {
		logger.Info("run stop target is already terminal", "state", status.State)
		return result, nil
	}

	processMetadata, err := runtime.ReadProcessMetadata(normalized.RunsBaseDir, normalized.RunID)
	if err != nil {
		logger.Error("failed to resolve process metadata", "error", err)
		return StopRunResult{}, wrapProcessMetadataReadError(err)
	}

	if err := runtime.WriteStopRequestMetadata(normalized.RunsBaseDir, runtime.StopRequestMetadata{
		RunID:       normalized.RunID,
		RequestedAt: normalized.now(),
		RequestedBy: normalized.RequestedBy,
		Signal:      normalized.Signal,
	}); err != nil {
		logger.Error("failed to persist stop request metadata", "error", err)
		return StopRunResult{}, fmt.Errorf("failed to persist stop request metadata; %w", err)
	}
	result.StopRequested = true

	if err := runtime.ValidateLiveProcessMetadata(processMetadata); err != nil {
		if errors.Is(err, runtime.ErrProcessNotRunning) {
			logger.Info("run process was not running when stop request was issued",
				"pid", processMetadata.PID,
			)
			return waitForTerminalStopState(normalized, result, processMetadata, normalized.now().Add(normalized.ProcessExitGrace), logger)
		}
		logger.Error("failed to validate process identity",
			"pid", processMetadata.PID,
			"error", err,
		)
		return StopRunResult{}, wrapProcessMetadataValidationError(err)
	}

	if processMetadata.PID == os.Getpid() {
		if normalized.InProcessStop != nil && normalized.InProcessStop(normalized.RunID, processMetadata) {
			result.UsedInProcessFastPath = true
			logger.Info("applied in-process run stop fast path",
				"pid", processMetadata.PID,
				"source", processMetadata.Source,
			)
			return waitForTerminalStopState(normalized, result, processMetadata, time.Time{}, logger)
		}
		logger.Error("current-process stop target requires an in-process fast path",
			"pid", processMetadata.PID,
			"source", processMetadata.Source,
		)
		return StopRunResult{}, fmt.Errorf("failed to stop current-process run without fast path; %w", ErrCurrentProcessFastPathRequired)
	}

	process, err := os.FindProcess(processMetadata.PID)
	if err != nil {
		logger.Error("failed to resolve process handle", "pid", processMetadata.PID, "error", err)
		return StopRunResult{}, fmt.Errorf("failed to resolve process handle; %w", err)
	}
	if err := process.Signal(syscall.SIGTERM); err != nil {
		if !isExitedProcessError(err) {
			logger.Error("failed to signal process", "pid", processMetadata.PID, "error", err)
			return StopRunResult{}, fmt.Errorf("failed to signal run process; %w", err)
		}
		logger.Info("run process had already exited before signal delivery",
			"pid", processMetadata.PID,
		)
		return waitForTerminalStopState(normalized, result, processMetadata, normalized.now().Add(normalized.ProcessExitGrace), logger)
	}

	logger.Info("signaled run process",
		"pid", processMetadata.PID,
		"signal", normalized.Signal,
	)
	return waitForTerminalStopState(normalized, result, processMetadata, time.Time{}, logger)
}

func normalizeStopRunRequest(request StopRunRequest) (normalizedStopRunRequest, error) {
	if err := validateRunID(request.RunID); err != nil {
		return normalizedStopRunRequest{}, err
	}
	if request.Now == nil {
		request.Now = func() time.Time {
			return time.Now().UTC()
		}
	}
	if request.PollInterval <= 0 {
		request.PollInterval = DefaultStopPollInterval
	}
	if request.ProcessExitGrace <= 0 {
		request.ProcessExitGrace = DefaultStopProcessExitGrace
	}
	if request.Signal == "" {
		request.Signal = runtime.StopSignalSIGTERM
	}
	if request.Signal != runtime.StopSignalSIGTERM {
		return normalizedStopRunRequest{}, fmt.Errorf("unsupported stop signal %q", request.Signal)
	}
	if request.RequestedBy == "" {
		return normalizedStopRunRequest{}, fmt.Errorf("requested_by is required")
	}
	return normalizedStopRunRequest{
		StopRunRequest: request,
		now:            request.Now,
	}, nil
}

func waitForTerminalStopState(
	request normalizedStopRunRequest,
	result StopRunResult,
	processMetadata runtime.ProcessMetadata,
	processExitDeadline time.Time,
	logger *slog.Logger,
) (StopRunResult, error) {
	for {
		status, err := runtime.ResolveRunStatus(request.RunsBaseDir, request.RunID)
		if err != nil {
			logger.Error("failed to poll run state", "error", err)
			return StopRunResult{}, fmt.Errorf("failed to poll run state; %w", err)
		}
		result.State = string(status.State)
		result.EventsPath = status.EventsPath
		if runtime.IsTerminalRunState(status.State) {
			logger.Info("observed terminal run state after stop request",
				"state", status.State,
				"stop_requested", result.StopRequested,
				"used_in_process_fast_path", result.UsedInProcessFastPath,
			)
			return result, nil
		}

		if !result.UsedInProcessFastPath {
			running, err := runtime.IsOriginalProcessRunning(processMetadata)
			if err != nil {
				logger.Error("failed to poll process liveness", "pid", processMetadata.PID, "error", err)
				return StopRunResult{}, fmt.Errorf("failed to poll run process; %w", err)
			}
			if !running {
				if processExitDeadline.IsZero() {
					processExitDeadline = request.now().Add(request.ProcessExitGrace)
				}
				if request.now().After(processExitDeadline) {
					logger.Error("run process exited before a terminal state was observed",
						"pid", processMetadata.PID,
					)
					return StopRunResult{}, fmt.Errorf("run process exited before terminal state was observed; %w", ErrTerminalStateNotObserved)
				}
			}
		}

		time.Sleep(request.PollInterval)
	}
}

func validateRunID(runID string) error {
	trimmed := strings.TrimSpace(runID)
	parsed, err := uuid.Parse(trimmed)
	if err != nil || parsed.Version() != uuid.Version(7) {
		if err == nil {
			err = fmt.Errorf("expected UUIDv7, got UUIDv%d", parsed.Version())
		}
		return fmt.Errorf("run-id must be UUIDv7; %w; %w", err, ErrInvalidRunID)
	}
	return nil
}

func wrapResolveRunStatusError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to resolve run state; %w", ErrRunNotFound)
	}
	return fmt.Errorf("failed to resolve run state; %w", err)
}

func wrapProcessMetadataReadError(err error) error {
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("failed to resolve live process metadata; %w", ErrProcessMetadataUnavailable)
	}
	return fmt.Errorf("failed to resolve live process metadata; %w", err)
}

func wrapProcessMetadataValidationError(err error) error {
	if errors.Is(err, runtime.ErrStaleProcessMetadata) {
		return fmt.Errorf("failed to validate live process metadata; %w; %w", err, ErrStaleProcessMetadata)
	}
	return fmt.Errorf("failed to validate live process metadata; %w", err)
}

func isExitedProcessError(err error) bool {
	return errors.Is(err, os.ErrProcessDone) || errors.Is(err, syscall.ESRCH)
}

func stopLogger(runID string) *slog.Logger {
	return slog.Default().With("component", "control.run.stop", "run_id", runID)
}
