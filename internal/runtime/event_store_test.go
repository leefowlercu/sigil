package runtime

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNewEventStoreDerivesPerRunEventsJSONLPath(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustUUIDv7String(t)

	store, err := NewEventStore(baseDir, runID)
	if err != nil {
		t.Fatalf("expected event store creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	expectedPath := filepath.Join(baseDir, runID, "events.jsonl")
	if store.EventsFilePath() != expectedPath {
		t.Fatalf("expected events path %q, got %q", expectedPath, store.EventsFilePath())
	}

	info, err := os.Stat(expectedPath)
	if err != nil {
		t.Fatalf("expected events file to exist, got %v", err)
	}
	if !info.Mode().IsRegular() {
		t.Fatalf("expected events path to be regular file, got %s", info.Mode())
	}
}

func TestEventStoreAppendNextWritesJSONLinesAndFsyncs(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustUUIDv7String(t)

	store, err := NewEventStore(baseDir, runID)
	if err != nil {
		t.Fatalf("expected event store creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = store.Close()
	})

	if _, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	}); err != nil {
		t.Fatalf("expected run.queued append success, got %v", err)
	}

	if _, err := store.AppendNext(EventTypeRunRunning, nil, RunRunningPayload{
		Executor: "rlm",
		MaxDepth: 0,
	}); err != nil {
		t.Fatalf("expected run.running append success, got %v", err)
	}

	if store.SyncCount() != 2 {
		t.Fatalf("expected sync count 2, got %d", store.SyncCount())
	}

	content, err := os.ReadFile(filepath.Clean(store.EventsFilePath()))
	if err != nil {
		t.Fatalf("expected events file read success, got %v", err)
	}
	if len(content) == 0 {
		t.Fatal("expected non-empty events file")
	}
	if content[len(content)-1] != '\n' {
		t.Fatal("expected newline-terminated JSONL file")
	}

	lines := strings.Split(strings.TrimSpace(string(content)), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 JSONL records, got %d", len(lines))
	}
	for _, line := range lines {
		if _, err := ParseEventEnvelopeStrict([]byte(line)); err != nil {
			t.Fatalf("expected strict parse success for line %q, got %v", line, err)
		}
	}
}

func TestEventStoreNotifiesObserversAfterFsync(t *testing.T) {
	baseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustUUIDv7String(t)

	fsynced := false
	observed := make([]EventEnvelope, 0, 1)
	var store *EventStore
	var observerErr error

	store, observerErr = NewEventStoreWithOptions(baseDir, runID, []EventObserver{
		func(event EventEnvelope) {
			if !fsynced {
				observerErr = fmt.Errorf("observer ran before fsync completed")
				return
			}
			if store.SyncCount() != 1 {
				observerErr = fmt.Errorf("observer ran before sync count updated")
				return
			}
			observed = append(observed, event)
		},
	})
	if observerErr != nil {
		t.Fatalf("expected event store creation success, got %v", observerErr)
	}
	if store == nil {
		t.Fatal("expected non-nil event store")
	}
	t.Cleanup(func() {
		_ = store.Close()
	})
	store.fsyncFn = func(file *os.File) error {
		fsynced = true
		return nil
	}

	event, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	})
	if err != nil {
		t.Fatalf("expected append success, got %v", err)
	}
	if observerErr != nil {
		t.Fatalf("expected observer success, got %v", observerErr)
	}
	if len(observed) != 1 {
		t.Fatalf("expected one observed event, got %d", len(observed))
	}
	if observed[0].EventID != event.EventID {
		t.Fatalf("expected observed event_id %q, got %q", event.EventID, observed[0].EventID)
	}
}

func TestEventStoreAppendRejectsNonContiguousSequence(t *testing.T) {
	store := mustNewEventStore(t)

	firstEvent, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	})
	if err != nil {
		t.Fatalf("expected first append success, got %v", err)
	}

	event := EventEnvelope{
		EventID:       mustUUIDv7String(t),
		SchemaVersion: SchemaVersionV1,
		RunID:         store.runID,
		Seq:           firstEvent.Seq + 2,
		Timestamp:     time.Now().UTC(),
		Type:          EventTypeRunRunning,
		CausationID:   firstEvent.EventID,
		CorrelationID: store.runID,
		Payload: RunRunningPayload{
			Executor: "rlm",
			MaxDepth: 1,
		},
	}

	err = store.Append(event)
	if !errors.Is(err, ErrNonContiguousSequence) {
		t.Fatalf("expected ErrNonContiguousSequence, got %v", err)
	}
}

func TestEventStoreAppendRejectsImmutableRewriteSequence(t *testing.T) {
	store := mustNewEventStore(t)

	firstEvent, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	})
	if err != nil {
		t.Fatalf("expected first append success, got %v", err)
	}

	event := EventEnvelope{
		EventID:       mustUUIDv7String(t),
		SchemaVersion: SchemaVersionV1,
		RunID:         store.runID,
		Seq:           firstEvent.Seq,
		Timestamp:     time.Now().UTC(),
		Type:          EventTypeRunQueued,
		CausationID:   firstEvent.EventID,
		CorrelationID: store.runID,
		Payload: RunQueuedPayload{
			Source: RunQueuedSourceInternalResume,
		},
	}
	event.CausationID = event.EventID

	err = store.Append(event)
	if !errors.Is(err, ErrImmutableEventLog) {
		t.Fatalf("expected ErrImmutableEventLog, got %v", err)
	}
}

func TestEventStoreValidateIntegrityFailsOnMalformedJSONLine(t *testing.T) {
	store := mustNewEventStore(t)

	if _, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	}); err != nil {
		t.Fatalf("expected append success, got %v", err)
	}

	appendRaw(t, store.EventsFilePath(), []byte(`not-json`+"\n"))

	if err := store.ValidateIntegrity(); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("expected ErrIntegrityFailure, got %v", err)
	}
}

func TestEventStoreValidateIntegrityFailsOnPartialTrailingLine(t *testing.T) {
	store := mustNewEventStore(t)

	if _, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	}); err != nil {
		t.Fatalf("expected append success, got %v", err)
	}

	appendRaw(t, store.EventsFilePath(), []byte(`{"truncated":true`))

	if err := store.ValidateIntegrity(); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("expected ErrIntegrityFailure, got %v", err)
	}
}

func TestEventStoreValidateIntegrityFailsOnSequenceGap(t *testing.T) {
	store := mustNewEventStore(t)

	firstEvent, err := store.AppendNext(EventTypeRunQueued, nil, RunQueuedPayload{
		Source: RunQueuedSourceInternalResume,
	})
	if err != nil {
		t.Fatalf("expected append success, got %v", err)
	}

	gapEvent := EventEnvelope{
		EventID:       mustUUIDv7String(t),
		SchemaVersion: SchemaVersionV1,
		RunID:         store.runID,
		Seq:           3,
		Timestamp:     time.Now().UTC(),
		Type:          EventTypeRunRunning,
		CausationID:   firstEvent.EventID,
		CorrelationID: store.runID,
		Payload: RunRunningPayload{
			Executor: "rlm",
			MaxDepth: 0,
		},
	}

	serialized, err := json.Marshal(gapEvent)
	if err != nil {
		t.Fatalf("expected serialization success, got %v", err)
	}
	appendRaw(t, store.EventsFilePath(), append(serialized, '\n'))

	if err := store.ValidateIntegrity(); !errors.Is(err, ErrIntegrityFailure) {
		t.Fatalf("expected ErrIntegrityFailure, got %v", err)
	}
}

func mustNewEventStore(t *testing.T) *EventStore {
	t.Helper()

	store, err := NewEventStore(filepath.Join(t.TempDir(), "sigil-runs"), mustUUIDv7String(t))
	if err != nil {
		t.Fatalf("expected event store creation success, got %v", err)
	}

	t.Cleanup(func() {
		_ = store.Close()
	})

	return store
}

func mustUUIDv7String(t *testing.T) string {
	t.Helper()

	value, err := newUUIDv7String()
	if err != nil {
		t.Fatalf("expected UUIDv7 generation success, got %v", err)
	}
	return value
}

func appendRaw(t *testing.T, path string, payload []byte) {
	t.Helper()

	file, err := os.OpenFile(filepath.Clean(path), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("expected file open success, got %v", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.Write(payload); err != nil {
		t.Fatalf("expected append success, got %v", err)
	}
}
