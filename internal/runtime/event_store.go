package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const eventsFileName = "events.jsonl"

// EventStore persists append-only run events to durable JSONL files.
type EventStore struct {
	mu         sync.Mutex
	runID      string
	baseDir    string
	runDir     string
	eventsPath string
	file       *os.File
	state      eventValidationState
	syncCount  int
	fsyncFn    func(*os.File) error
	nowFn      func() time.Time
	observers  []EventObserver
}

// NewEventStore creates or opens an append-only per-run event store.
func NewEventStore(baseDir string, runID string) (*EventStore, error) {
	return NewEventStoreWithOptions(baseDir, runID, nil)
}

// NewEventStoreWithOptions creates or opens an append-only per-run event store with optional observers.
func NewEventStoreWithOptions(baseDir string, runID string, observers []EventObserver) (*EventStore, error) {
	if err := validateUUIDv7String(runID); err != nil {
		return nil, fmt.Errorf("run_id must be UUIDv7; %w", ErrInvalidEvent)
	}

	resolvedBaseDir, err := resolveBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	runDir := filepath.Join(resolvedBaseDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		return nil, fmt.Errorf("failed to create run directory %q; %w", runDir, err)
	}

	eventsPath := filepath.Join(runDir, eventsFileName)
	file, err := os.OpenFile(eventsPath, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("failed to open events file %q; %w", eventsPath, err)
	}

	store := &EventStore{
		runID:      runID,
		baseDir:    resolvedBaseDir,
		runDir:     runDir,
		eventsPath: eventsPath,
		file:       file,
		state:      newEventValidationState(runID),
		fsyncFn: func(file *os.File) error {
			return file.Sync()
		},
		nowFn: func() time.Time {
			return time.Now().UTC()
		},
		observers: cloneEventObservers(observers),
	}

	if _, err := store.reloadStateLocked(); err != nil {
		_ = file.Close()
		return nil, err
	}

	eventStoreLogger().Info("initialized event store",
		"run_id", runID,
		"base_dir", resolvedBaseDir,
		"events_path", eventsPath,
	)

	return store, nil
}

// Append appends a caller-provided event envelope with explicit sequence control.
func (s *EventStore) Append(event EventEnvelope) error {
	s.mu.Lock()
	validated, observers, err := s.appendLocked(event)
	s.mu.Unlock()
	if err != nil {
		return err
	}

	notifyEventObservers(observers, validated)
	return nil
}

// AppendNext appends a new event with auto-assigned sequence, IDs, and timestamp.
func (s *EventStore) AppendNext(eventType EventType, nodeID *string, payload any) (EventEnvelope, error) {
	s.mu.Lock()

	eventID, err := newUUIDv7String()
	if err != nil {
		s.mu.Unlock()
		return EventEnvelope{}, fmt.Errorf("failed to generate event_id; %w", err)
	}

	causationID := eventID
	if s.state.latestSeq > 0 {
		causationID = s.state.lastEvent
	}

	event := EventEnvelope{
		EventID:       eventID,
		SchemaVersion: SchemaVersionV1,
		RunID:         s.runID,
		Seq:           s.state.latestSeq + 1,
		Timestamp:     s.now(),
		Type:          eventType,
		NodeID:        cloneStringPointer(nodeID),
		CausationID:   causationID,
		CorrelationID: s.runID,
		Payload:       payload,
	}

	validated, observers, err := s.appendLocked(event)
	s.mu.Unlock()
	if err != nil {
		return EventEnvelope{}, err
	}

	notifyEventObservers(observers, validated)
	return validated, nil
}

// EventsFilePath returns the fully resolved events.jsonl path for the run.
func (s *EventStore) EventsFilePath() string {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.eventsPath
}

// ReadAll reads and validates all persisted events in append order.
func (s *EventStore) ReadAll() ([]EventEnvelope, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	events, _, err := parsePersistedEventsLocked(s.eventsPath, s.runID)
	if err != nil {
		return nil, err
	}

	return cloneEventEnvelopes(events), nil
}

// ValidateIntegrity validates the persisted event log for malformed content and sequence integrity.
func (s *EventStore) ValidateIntegrity() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, _, err := parsePersistedEventsLocked(s.eventsPath, s.runID)
	return err
}

// SyncCount returns the number of successful fsync operations acknowledged by this store.
func (s *EventStore) SyncCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.syncCount
}

// Close closes the underlying file sink for the event store.
func (s *EventStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.file == nil {
		return nil
	}

	if err := s.file.Close(); err != nil {
		return fmt.Errorf("failed to close events file %q; %w", s.eventsPath, err)
	}
	s.file = nil
	eventStoreLogger().Debug("closed events file",
		"run_id", s.runID,
		"events_path", s.eventsPath,
	)
	return nil
}

func (s *EventStore) appendLocked(event EventEnvelope) (EventEnvelope, []EventObserver, error) {
	if s.file == nil {
		return EventEnvelope{}, nil, fmt.Errorf("event store is closed")
	}

	validated, err := validateEventForAppend(event, s.state)
	if err != nil {
		return EventEnvelope{}, nil, err
	}

	serialized, err := json.Marshal(validated)
	if err != nil {
		return EventEnvelope{}, nil, fmt.Errorf("failed to serialize event envelope; %w", err)
	}

	if _, err := s.file.Write(append(serialized, '\n')); err != nil {
		return EventEnvelope{}, nil, fmt.Errorf("failed to append event to %q; %w", s.eventsPath, err)
	}

	if err := s.fsyncFn(s.file); err != nil {
		_, _ = s.reloadStateLocked()
		return EventEnvelope{}, nil, fmt.Errorf("failed to fsync events file %q; %w", s.eventsPath, err)
	}

	s.syncCount++
	s.state.consume(validated)
	eventStoreLogger().Debug("appended event envelope",
		"run_id", s.runID,
		"seq", validated.Seq,
		"type", validated.Type,
		"node_id", valueOrEmptyString(validated.NodeID),
	)
	return validated, cloneEventObservers(s.observers), nil
}

func (s *EventStore) reloadStateLocked() ([]EventEnvelope, error) {
	events, state, err := parsePersistedEventsLocked(s.eventsPath, s.runID)
	if err != nil {
		return nil, err
	}

	s.state = state
	s.syncCount = len(events)
	return events, nil
}

func parsePersistedEventsLocked(path string, runID string) ([]EventEnvelope, eventValidationState, error) {
	content, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, eventValidationState{}, fmt.Errorf("failed to read events file %q; %w", path, err)
	}

	state := newEventValidationState(runID)
	events := make([]EventEnvelope, 0, 8)

	if len(content) == 0 {
		return events, state, nil
	}

	if content[len(content)-1] != '\n' {
		return nil, eventValidationState{}, fmt.Errorf("events file %q ends with partial/truncated line; %w", path, ErrIntegrityFailure)
	}

	lines := bytes.Split(content, []byte("\n"))
	for index, line := range lines {
		trimmed := strings.TrimSpace(string(line))
		if trimmed == "" {
			continue
		}

		event, err := ParseEventEnvelopeStrict(line)
		if err != nil {
			return nil, eventValidationState{}, fmt.Errorf("events line %d failed strict parse; %w", index+1, ErrIntegrityFailure)
		}

		validated, err := validateEventForAppend(event, state)
		if err != nil {
			return nil, eventValidationState{}, fmt.Errorf("events line %d failed integrity checks; %w", index+1, ErrIntegrityFailure)
		}

		events = append(events, validated)
		state.consume(validated)
	}

	return events, state, nil
}

func resolveBaseDir(baseDir string) (string, error) {
	clean := strings.TrimSpace(baseDir)
	if clean == "" {
		return "", fmt.Errorf("baseDir is empty; %w", ErrInvalidEvent)
	}

	if filepath.IsAbs(clean) {
		return filepath.Clean(clean), nil
	}

	cwd, err := os.Getwd()
	if err != nil {
		return "", fmt.Errorf("failed to resolve working directory; %w", err)
	}

	return filepath.Clean(filepath.Join(cwd, clean)), nil
}

// ResolveRunsBaseDir returns the normalized absolute runs base directory path.
func ResolveRunsBaseDir(baseDir string) (string, error) {
	return resolveBaseDir(baseDir)
}

func (s *EventStore) now() time.Time {
	if s == nil || s.nowFn == nil {
		return time.Now().UTC()
	}

	return s.nowFn().UTC()
}

func cloneEventEnvelopes(events []EventEnvelope) []EventEnvelope {
	cloned := make([]EventEnvelope, 0, len(events))
	for _, event := range events {
		eventClone := event
		eventClone.NodeID = cloneStringPointer(event.NodeID)
		cloned = append(cloned, eventClone)
	}
	return cloned
}

func cloneEventObservers(observers []EventObserver) []EventObserver {
	if len(observers) == 0 {
		return nil
	}

	cloned := make([]EventObserver, len(observers))
	copy(cloned, observers)
	return cloned
}

func notifyEventObservers(observers []EventObserver, event EventEnvelope) {
	for _, observer := range observers {
		if observer == nil {
			continue
		}
		observer(event)
	}
}

func eventStoreLogger() *slog.Logger {
	return slog.Default().With("component", "runtime.event_store")
}
