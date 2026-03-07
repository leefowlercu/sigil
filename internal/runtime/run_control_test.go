package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestResolveRunStatusDerivesLifecycleState(t *testing.T) {
	testCases := []struct {
		name          string
		transition    func(*Lifecycle) error
		expectedState RunState
	}{
		{
			name:          "queued",
			transition:    func(*Lifecycle) error { return nil },
			expectedState: RunStateQueued,
		},
		{
			name: "running",
			transition: func(lifecycle *Lifecycle) error {
				return lifecycle.StartExecution()
			},
			expectedState: RunStateRunning,
		},
		{
			name: "completed",
			transition: func(lifecycle *Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Complete()
			},
			expectedState: RunStateCompleted,
		},
		{
			name: "failed",
			transition: func(lifecycle *Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Fail()
			},
			expectedState: RunStateFailed,
		},
		{
			name: "interrupted",
			transition: func(lifecycle *Lifecycle) error {
				if err := lifecycle.StartExecution(); err != nil {
					return err
				}
				return lifecycle.Interrupt()
			},
			expectedState: RunStateInterrupted,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
			lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
				RunsBaseDir: runsBaseDir,
				MaxDepth:    3,
			})
			if err != nil {
				t.Fatalf("expected lifecycle creation success, got %v", err)
			}
			t.Cleanup(func() {
				_ = lifecycle.Close()
			})
			if err := testCase.transition(lifecycle); err != nil {
				t.Fatalf("expected lifecycle transition success, got %v", err)
			}

			eventsPath, err := lifecycle.EventsFilePath()
			if err != nil {
				t.Fatalf("expected events path resolution success, got %v", err)
			}

			status, err := ResolveRunStatus(runsBaseDir, lifecycle.RunID())
			if err != nil {
				t.Fatalf("expected status resolution success, got %v", err)
			}
			if status.RunID != lifecycle.RunID() {
				t.Fatalf("expected run_id %q, got %q", lifecycle.RunID(), status.RunID)
			}
			if status.State != testCase.expectedState {
				t.Fatalf("expected state %q, got %q", testCase.expectedState, status.State)
			}
			if status.EventsPath != eventsPath {
				t.Fatalf("expected events_path %q, got %q", eventsPath, status.EventsPath)
			}
			if status.RunDir != filepath.Dir(eventsPath) {
				t.Fatalf("expected run_dir %q, got %q", filepath.Dir(eventsPath), status.RunDir)
			}
		})
	}
}

func TestResolveRunStatusRejectsCorruptEventLog(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID, err := newUUIDv7String()
	if err != nil {
		t.Fatalf("expected run_id generation success, got %v", err)
	}

	runDir := filepath.Join(runsBaseDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("expected run dir creation success, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatalf("expected corrupt event write success, got %v", err)
	}

	_, err = ResolveRunStatus(runsBaseDir, runID)
	if !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("expected ErrIntegrityFailure, got %v", err)
	}
}

func TestProcessMetadataRoundTripAndRemove(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	recordedAt := time.Unix(1_700_000_000, 0).UTC()
	startedAt := time.Unix(1_700_000_010, 0).UTC()
	metadata := ProcessMetadata{
		RunID:      testRunID(t),
		PID:        4242,
		RecordedAt: recordedAt,
		StartedAt:  startedAt,
		Source:     RunSourceCLIRunStart,
	}

	if err := WriteProcessMetadata(runsBaseDir, metadata); err != nil {
		t.Fatalf("expected process metadata write success, got %v", err)
	}
	path, err := ProcessMetadataPath(runsBaseDir, metadata.RunID)
	if err != nil {
		t.Fatalf("expected process metadata path resolution success, got %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected process metadata file to exist, got %v", err)
	}

	decoded, err := ReadProcessMetadata(runsBaseDir, metadata.RunID)
	if err != nil {
		t.Fatalf("expected process metadata read success, got %v", err)
	}
	if decoded.RunID != metadata.RunID || decoded.PID != metadata.PID || decoded.Source != metadata.Source || !decoded.RecordedAt.Equal(recordedAt) || !decoded.StartedAt.Equal(startedAt) {
		t.Fatalf("expected decoded metadata %+v, got %+v", metadata, decoded)
	}

	if err := RemoveProcessMetadata(runsBaseDir, metadata.RunID); err != nil {
		t.Fatalf("expected process metadata removal success, got %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected removed process metadata file, got %v", err)
	}
}

func TestReadProcessMetadataRejectsMismatchedRunIdentity(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	expectedRunID := testRunID(t)
	otherRunID := testRunID(t)
	recordedAt := time.Unix(1_700_000_200, 0).UTC()
	path, err := ProcessMetadataPath(runsBaseDir, expectedRunID)
	if err != nil {
		t.Fatalf("expected process metadata path resolution success, got %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("expected run dir creation success, got %v", err)
	}
	raw, err := json.Marshal(ProcessMetadata{
		RunID:      otherRunID,
		PID:        4242,
		RecordedAt: recordedAt,
		StartedAt:  recordedAt.Add(-time.Second),
		Source:     RunSourceCLIRunStart,
	})
	if err != nil {
		t.Fatalf("expected process metadata encode success, got %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("expected process metadata write success, got %v", err)
	}

	_, err = ReadProcessMetadata(runsBaseDir, expectedRunID)
	if err == nil {
		t.Fatal("expected mismatched run identity to fail")
	}
	if !strings.Contains(err.Error(), "does not match requested run") {
		t.Fatalf("expected mismatched run identity error, got %v", err)
	}
}

func TestReadProcessMetadataRejectsUnexpectedSource(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := testRunID(t)
	recordedAt := time.Unix(1_700_000_300, 0).UTC()
	path, err := ProcessMetadataPath(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected process metadata path resolution success, got %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("expected run dir creation success, got %v", err)
	}
	raw, err := json.Marshal(ProcessMetadata{
		RunID:      runID,
		PID:        4242,
		RecordedAt: recordedAt,
		StartedAt:  recordedAt.Add(-time.Second),
		Source:     "daemon.resume",
	})
	if err != nil {
		t.Fatalf("expected process metadata encode success, got %v", err)
	}
	if err := os.WriteFile(path, append(raw, '\n'), 0o644); err != nil {
		t.Fatalf("expected process metadata write success, got %v", err)
	}

	_, err = ReadProcessMetadata(runsBaseDir, runID)
	if err == nil {
		t.Fatal("expected unexpected source to fail")
	}
	if !strings.Contains(err.Error(), "must use source") {
		t.Fatalf("expected source validation error, got %v", err)
	}
}

func TestNewLifecycleWithOptionsPersistsProcessMetadataWithQueuedRun(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	recordedAt := time.Unix(1_700_000_400, 0).UTC()
	startedAt := time.Unix(1_700_000_390, 0).UTC()

	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsBaseDir,
		MaxDepth:    3,
		ProcessMetadata: &ProcessMetadata{
			PID:        4242,
			RecordedAt: recordedAt,
			StartedAt:  startedAt,
			Source:     RunSourceCLIRunStart,
		},
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	status, err := ResolveRunStatus(runsBaseDir, lifecycle.RunID())
	if err != nil {
		t.Fatalf("expected status resolution success, got %v", err)
	}
	if status.State != RunStateQueued {
		t.Fatalf("expected queued lifecycle state, got %q", status.State)
	}

	metadata, err := ReadProcessMetadata(runsBaseDir, lifecycle.RunID())
	if err != nil {
		t.Fatalf("expected process metadata read success, got %v", err)
	}
	if metadata.RunID != lifecycle.RunID() {
		t.Fatalf("expected process metadata run_id %q, got %q", lifecycle.RunID(), metadata.RunID)
	}
	if metadata.PID != 4242 {
		t.Fatalf("expected process metadata pid 4242, got %d", metadata.PID)
	}
	if !metadata.RecordedAt.Equal(recordedAt) {
		t.Fatalf("expected process metadata recorded_at %s, got %s", recordedAt, metadata.RecordedAt)
	}
	if !metadata.StartedAt.Equal(startedAt) {
		t.Fatalf("expected process metadata started_at %s, got %s", startedAt, metadata.StartedAt)
	}
}

func TestNewLifecycleWithOptionsDoesNotExposeQueuedRunWhenProcessMetadataInvalid(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")

	_, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsBaseDir,
		MaxDepth:    3,
		ProcessMetadata: &ProcessMetadata{
			PID:        4242,
			RecordedAt: time.Unix(1_700_000_500, 0).UTC(),
			Source:     RunSourceCLIRunStart,
		},
	})
	if err == nil {
		t.Fatal("expected lifecycle creation to fail for invalid process metadata")
	}

	eventsPaths, globErr := filepath.Glob(filepath.Join(runsBaseDir, "*", eventsFileName))
	if globErr != nil {
		t.Fatalf("expected events glob success, got %v", globErr)
	}
	if len(eventsPaths) != 1 {
		t.Fatalf("expected one run directory after failed init, got %d", len(eventsPaths))
	}

	content, readErr := os.ReadFile(eventsPaths[0])
	if readErr != nil {
		t.Fatalf("expected events file read success, got %v", readErr)
	}
	if len(content) != 0 {
		t.Fatalf("expected no persisted events when process metadata is invalid, got %q", string(content))
	}
}

func TestStopRequestMetadataRoundTripAndMissing(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := testRunID(t)

	decoded, ok, err := ReadStopRequestMetadata(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected missing stop request read success, got %v", err)
	}
	if ok {
		t.Fatalf("expected missing stop request metadata, got %+v", decoded)
	}

	requestedAt := time.Unix(1_700_000_100, 0).UTC()
	metadata := StopRequestMetadata{
		RunID:       runID,
		RequestedAt: requestedAt,
		RequestedBy: StopRequesterCLIRunStop,
		Signal:      StopSignalSIGTERM,
	}
	if err := WriteStopRequestMetadata(runsBaseDir, metadata); err != nil {
		t.Fatalf("expected stop request write success, got %v", err)
	}

	decoded, ok, err = ReadStopRequestMetadata(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected stop request read success, got %v", err)
	}
	if !ok {
		t.Fatal("expected stop request metadata to exist")
	}
	if decoded.RunID != metadata.RunID || decoded.RequestedBy != metadata.RequestedBy || decoded.Signal != metadata.Signal || !decoded.RequestedAt.Equal(requestedAt) {
		t.Fatalf("expected decoded metadata %+v, got %+v", metadata, decoded)
	}
}

func TestIsExternalStopContext(t *testing.T) {
	if IsExternalStopContext(nil) {
		t.Fatal("expected nil context to report false")
	}

	ctx, cancel := context.WithCancelCause(context.Background())
	cancel(ErrExternalStopRequested)
	if !IsExternalStopContext(ctx) {
		t.Fatal("expected external stop context to report true")
	}

	ctx, cancel = context.WithCancelCause(context.Background())
	cancel(context.Canceled)
	if IsExternalStopContext(ctx) {
		t.Fatal("expected generic canceled context to report false")
	}
}

func testRunID(t *testing.T) string {
	t.Helper()

	runID, err := newUUIDv7String()
	if err != nil {
		t.Fatalf("expected run_id generation success, got %v", err)
	}
	return runID
}
