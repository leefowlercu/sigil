package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	// ProcessMetadataFileName is the auxiliary live-process metadata file name.
	ProcessMetadataFileName = "process.json"
	// StopRequestFileName is the auxiliary stop-request metadata file name.
	StopRequestFileName = "stop-request.json"
	// StopRequesterCLIRunStop is the canonical stop requester identifier.
	StopRequesterCLIRunStop = "cli.run.stop"
	// StopSignalSIGTERM is the canonical external graceful-stop signal.
	StopSignalSIGTERM = "SIGTERM"
	// RunSourceCLIRunStart identifies a run created by the CLI start command.
	RunSourceCLIRunStart = "cli.run.start"
)

var (
	// ErrExternalStopRequested marks context cancellation caused by a graceful external stop request.
	ErrExternalStopRequested = errors.New("external stop requested")
	// ErrProcessNotRunning indicates that the target process no longer exists.
	ErrProcessNotRunning = errors.New("process not running")
	// ErrStaleProcessMetadata indicates that the stored process identity no longer matches the live PID.
	ErrStaleProcessMetadata = errors.New("stale process metadata")
)

// ProcessMetadata persists auxiliary live-process identity for one run.
type ProcessMetadata struct {
	RunID      string    `json:"run_id"`
	PID        int       `json:"pid"`
	RecordedAt time.Time `json:"recorded_at"`
	StartedAt  time.Time `json:"started_at"`
	Source     string    `json:"source"`
}

// StopRequestMetadata persists auxiliary stop-request metadata for one run.
type StopRequestMetadata struct {
	RunID       string    `json:"run_id"`
	RequestedAt time.Time `json:"requested_at"`
	RequestedBy string    `json:"requested_by"`
	Signal      string    `json:"signal"`
}

// RunStatus summarizes current run state resolved from events.jsonl.
type RunStatus struct {
	RunID      string
	RunDir     string
	EventsPath string
	State      RunState
}

type processIdentityStatus int

const (
	processIdentityMissing processIdentityStatus = iota
	processIdentityCurrent
	processIdentityStale
)

// IsExternalStopContext reports whether the context was canceled for graceful external stop handling.
func IsExternalStopContext(ctx context.Context) bool {
	if ctx == nil {
		return false
	}
	return errors.Is(context.Cause(ctx), ErrExternalStopRequested)
}

// IsTerminalRunState reports whether a run state is terminal.
func IsTerminalRunState(state RunState) bool {
	switch state {
	case RunStateCompleted, RunStateFailed, RunStateInterrupted:
		return true
	default:
		return false
	}
}

// CurrentProcessMetadata captures normalized control metadata for the current process.
func CurrentProcessMetadata(source string) (ProcessMetadata, error) {
	pid := os.Getpid()
	startedAt, err := readProcessStartedAt(pid)
	if err != nil {
		return ProcessMetadata{}, fmt.Errorf("failed to resolve current process start time; %w", err)
	}
	return ProcessMetadata{
		PID:        pid,
		RecordedAt: time.Now().UTC(),
		StartedAt:  startedAt,
		Source:     source,
	}, nil
}

// ValidateLiveProcessMetadata verifies that the stored PID still belongs to the original process.
func ValidateLiveProcessMetadata(metadata ProcessMetadata) error {
	status, err := inspectProcessIdentity(metadata)
	if err != nil {
		return err
	}
	switch status {
	case processIdentityCurrent:
		return nil
	case processIdentityMissing:
		return ErrProcessNotRunning
	case processIdentityStale:
		return ErrStaleProcessMetadata
	default:
		return fmt.Errorf("unknown process identity status")
	}
}

// IsOriginalProcessRunning reports whether the original process identified by process metadata is still alive.
func IsOriginalProcessRunning(metadata ProcessMetadata) (bool, error) {
	status, err := inspectProcessIdentity(metadata)
	if err != nil {
		return false, err
	}
	return status == processIdentityCurrent, nil
}

// ResolveRunStatus loads and validates events.jsonl to derive authoritative run state.
func ResolveRunStatus(baseDir string, runID string) (RunStatus, error) {
	if err := validateUUIDv7String(runID); err != nil {
		return RunStatus{}, fmt.Errorf("run-id must be UUIDv7; %w", err)
	}

	resolvedBaseDir, err := resolveBaseDir(baseDir)
	if err != nil {
		return RunStatus{}, err
	}
	runDir := filepath.Join(resolvedBaseDir, runID)
	eventsPath := filepath.Join(runDir, eventsFileName)
	events, _, err := parsePersistedEventsLocked(eventsPath, runID)
	if err != nil {
		return RunStatus{}, err
	}
	if len(events) == 0 {
		return RunStatus{}, fmt.Errorf("run %q has no persisted events; %w", runID, ErrIntegrityFailure)
	}

	state := deriveRunState(events)
	return RunStatus{
		RunID:      runID,
		RunDir:     runDir,
		EventsPath: eventsPath,
		State:      state,
	}, nil
}

// ProcessMetadataPath resolves the live-process metadata path for one run.
func ProcessMetadataPath(baseDir string, runID string) (string, error) {
	return auxiliaryRunPath(baseDir, runID, ProcessMetadataFileName)
}

// StopRequestPath resolves the stop-request metadata path for one run.
func StopRequestPath(baseDir string, runID string) (string, error) {
	return auxiliaryRunPath(baseDir, runID, StopRequestFileName)
}

// WriteProcessMetadata writes auxiliary live-process metadata for one run.
func WriteProcessMetadata(baseDir string, metadata ProcessMetadata) error {
	if err := validateProcessMetadata(metadata); err != nil {
		return err
	}
	path, err := ProcessMetadataPath(baseDir, metadata.RunID)
	if err != nil {
		return err
	}
	return writeJSONFile(path, metadata)
}

// ReadProcessMetadata loads auxiliary live-process metadata for one run.
func ReadProcessMetadata(baseDir string, runID string) (ProcessMetadata, error) {
	path, err := ProcessMetadataPath(baseDir, runID)
	if err != nil {
		return ProcessMetadata{}, err
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return ProcessMetadata{}, err
	}

	var metadata ProcessMetadata
	if err := decodeStrictJSONObject(raw, &metadata); err != nil {
		return ProcessMetadata{}, fmt.Errorf("failed to decode process metadata %q; %w", path, err)
	}
	if err := validateProcessMetadata(metadata); err != nil {
		return ProcessMetadata{}, err
	}
	if metadata.RunID != runID {
		return ProcessMetadata{}, fmt.Errorf("process metadata run_id %q does not match requested run %q", metadata.RunID, runID)
	}
	return metadata, nil
}

// RemoveProcessMetadata removes the auxiliary live-process metadata file for one run.
func RemoveProcessMetadata(baseDir string, runID string) error {
	path, err := ProcessMetadataPath(baseDir, runID)
	if err != nil {
		return err
	}
	if err := os.Remove(filepath.Clean(path)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// WriteStopRequestMetadata writes auxiliary stop-request metadata for one run.
func WriteStopRequestMetadata(baseDir string, metadata StopRequestMetadata) error {
	if err := validateStopRequestMetadata(metadata); err != nil {
		return err
	}
	path, err := StopRequestPath(baseDir, metadata.RunID)
	if err != nil {
		return err
	}
	return writeJSONFile(path, metadata)
}

// ReadStopRequestMetadata loads auxiliary stop-request metadata for one run.
func ReadStopRequestMetadata(baseDir string, runID string) (StopRequestMetadata, bool, error) {
	path, err := StopRequestPath(baseDir, runID)
	if err != nil {
		return StopRequestMetadata{}, false, err
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return StopRequestMetadata{}, false, nil
		}
		return StopRequestMetadata{}, false, err
	}

	var metadata StopRequestMetadata
	if err := decodeStrictJSONObject(raw, &metadata); err != nil {
		return StopRequestMetadata{}, false, fmt.Errorf("failed to decode stop request metadata %q; %w", path, err)
	}
	if err := validateStopRequestMetadata(metadata); err != nil {
		return StopRequestMetadata{}, false, err
	}
	return metadata, true, nil
}

func auxiliaryRunPath(baseDir string, runID string, fileName string) (string, error) {
	if err := validateUUIDv7String(runID); err != nil {
		return "", fmt.Errorf("run-id must be UUIDv7; %w", err)
	}
	resolvedBaseDir, err := resolveBaseDir(baseDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(resolvedBaseDir, runID, fileName), nil
}

func deriveRunState(events []EventEnvelope) RunState {
	state := RunStateQueued
	for _, event := range events {
		switch event.Type {
		case EventTypeRunQueued:
			state = RunStateQueued
		case EventTypeRunRunning:
			state = RunStateRunning
		case EventTypeRunCompleted:
			state = RunStateCompleted
		case EventTypeRunFailed:
			state = RunStateFailed
		case EventTypeRunInterrupted:
			state = RunStateInterrupted
		}
	}
	return state
}

func validateProcessMetadata(metadata ProcessMetadata) error {
	if err := validateUUIDv7String(metadata.RunID); err != nil {
		return fmt.Errorf("process metadata run_id must be UUIDv7; %w", err)
	}
	if metadata.PID < 1 {
		return fmt.Errorf("process metadata pid must be >= 1")
	}
	if metadata.RecordedAt.IsZero() {
		return fmt.Errorf("process metadata recorded_at is required")
	}
	if metadata.StartedAt.IsZero() {
		return fmt.Errorf("process metadata started_at is required")
	}
	if metadata.Source != RunSourceCLIRunStart {
		return fmt.Errorf("process metadata must use source %q", RunSourceCLIRunStart)
	}
	return nil
}

func validateStopRequestMetadata(metadata StopRequestMetadata) error {
	if err := validateUUIDv7String(metadata.RunID); err != nil {
		return fmt.Errorf("stop request run_id must be UUIDv7; %w", err)
	}
	if metadata.RequestedAt.IsZero() {
		return fmt.Errorf("stop request requested_at is required")
	}
	if strings.TrimSpace(metadata.RequestedBy) == "" {
		return fmt.Errorf("stop request requested_by is required")
	}
	if strings.TrimSpace(metadata.Signal) == "" {
		return fmt.Errorf("stop request signal is required")
	}
	return nil
}

func writeJSONFile(path string, payload any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Clean(path), append(encoded, '\n'), 0o644); err != nil {
		return err
	}
	return nil
}

func decodeStrictJSONObject(raw []byte, target any) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.DisallowUnknownFields()
	if err := dec.Decode(target); err != nil {
		return err
	}
	if dec.More() {
		return fmt.Errorf("unexpected trailing JSON content")
	}
	return nil
}

func inspectProcessIdentity(metadata ProcessMetadata) (processIdentityStatus, error) {
	if err := validateProcessMetadata(metadata); err != nil {
		return processIdentityMissing, err
	}

	startedAt, err := readProcessStartedAt(metadata.PID)
	if err != nil {
		if errors.Is(err, ErrProcessNotRunning) {
			return processIdentityMissing, nil
		}
		return processIdentityMissing, err
	}
	if !startedAt.Equal(metadata.StartedAt.UTC()) {
		return processIdentityStale, nil
	}
	return processIdentityCurrent, nil
}

func readProcessStartedAt(pid int) (time.Time, error) {
	if pid < 1 {
		return time.Time{}, fmt.Errorf("pid must be >= 1")
	}

	cmd := exec.Command("ps", "-o", "lstart=", "-p", strconv.Itoa(pid))
	output, err := cmd.CombinedOutput()
	if err != nil {
		if len(strings.TrimSpace(string(output))) == 0 {
			return time.Time{}, ErrProcessNotRunning
		}
		return time.Time{}, fmt.Errorf("failed to inspect process %d start time; %w", pid, err)
	}

	trimmed := strings.TrimSpace(string(output))
	if trimmed == "" {
		return time.Time{}, ErrProcessNotRunning
	}

	normalized := strings.Join(strings.Fields(trimmed), " ")
	startedAt, err := time.ParseInLocation("Mon Jan 2 15:04:05 2006", normalized, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("failed to parse process %d start time %q; %w", pid, trimmed, err)
	}
	return startedAt.UTC(), nil
}
