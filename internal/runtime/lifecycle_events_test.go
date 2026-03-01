package runtime

import (
	"path/filepath"
	"testing"
)

func TestLifecyclePersistsCanonicalEventsForExecutionFlow(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	appPath := "./sigil.yaml"
	runPath := "./sigil-run.yaml"

	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir:   runsDir,
		QueuedSource:  RunQueuedSourceCLIRunStart,
		AppConfigPath: &appPath,
		RunConfigPath: &runPath,
		MaxDepth:      5,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected StartExecution success, got %v", err)
	}

	root := lifecycle.Nodes()[0]
	if _, err := lifecycle.CreateChildNode(root.ID); err != nil {
		t.Fatalf("expected child creation success, got %v", err)
	}

	if err := lifecycle.Complete(); err != nil {
		t.Fatalf("expected completion success, got %v", err)
	}

	events, err := lifecycle.PersistedEvents()
	if err != nil {
		t.Fatalf("expected persisted events read success, got %v", err)
	}

	expectedTypes := []EventType{
		EventTypeRunQueued,
		EventTypeRunRunning,
		EventTypeNodeStarted,
		EventTypeNodeStarted,
		EventTypeRunCompleted,
	}
	if len(events) != len(expectedTypes) {
		t.Fatalf("expected %d events, got %d", len(expectedTypes), len(events))
	}

	for index, expectedType := range expectedTypes {
		event := events[index]
		if event.Type != expectedType {
			t.Fatalf("expected type[%d]=%q, got %q", index, expectedType, event.Type)
		}
		if event.Seq != int64(index+1) {
			t.Fatalf("expected seq[%d]=%d, got %d", index, index+1, event.Seq)
		}
		if event.RunID != lifecycle.RunID() {
			t.Fatalf("expected run_id[%d]=%q, got %q", index, lifecycle.RunID(), event.RunID)
		}
		if event.CorrelationID != lifecycle.RunID() {
			t.Fatalf("expected correlation_id[%d]=run_id, got %q", index, event.CorrelationID)
		}
		if index == 0 {
			if event.CausationID != event.EventID {
				t.Fatalf("expected first event causation_id=event_id, got %q != %q", event.CausationID, event.EventID)
			}
		} else {
			if event.CausationID != events[index-1].EventID {
				t.Fatalf("expected event[%d] causation_id to reference previous event_id", index)
			}
		}
	}

	queuedPayload, ok := events[0].Payload.(RunQueuedPayload)
	if !ok {
		t.Fatalf("expected run.queued payload type RunQueuedPayload, got %T", events[0].Payload)
	}
	if queuedPayload.Source != RunQueuedSourceCLIRunStart {
		t.Fatalf("expected queued source %q, got %q", RunQueuedSourceCLIRunStart, queuedPayload.Source)
	}
	if queuedPayload.AppConfigPath == nil || *queuedPayload.AppConfigPath != appPath {
		t.Fatalf("expected queued app_config_path %q, got %v", appPath, queuedPayload.AppConfigPath)
	}
	if queuedPayload.RunConfigPath == nil || *queuedPayload.RunConfigPath != runPath {
		t.Fatalf("expected queued run_config_path %q, got %v", runPath, queuedPayload.RunConfigPath)
	}

	runningPayload, ok := events[1].Payload.(RunRunningPayload)
	if !ok {
		t.Fatalf("expected run.running payload type RunRunningPayload, got %T", events[1].Payload)
	}
	if runningPayload.MaxDepth != 5 {
		t.Fatalf("expected max_depth 5, got %d", runningPayload.MaxDepth)
	}

	rootStartedPayload, ok := events[2].Payload.(NodeStartedPayload)
	if !ok {
		t.Fatalf("expected node.started payload type NodeStartedPayload, got %T", events[2].Payload)
	}
	if rootStartedPayload.Depth != 0 || rootStartedPayload.ParentNodeID != nil || rootStartedPayload.Role != NodeRoleRoot {
		t.Fatalf("unexpected root node.started payload: %+v", rootStartedPayload)
	}

	childStartedPayload, ok := events[3].Payload.(NodeStartedPayload)
	if !ok {
		t.Fatalf("expected node.started payload type NodeStartedPayload, got %T", events[3].Payload)
	}
	if childStartedPayload.Depth != 1 || childStartedPayload.ParentNodeID == nil || childStartedPayload.Role != NodeRoleRecursiveSubcall {
		t.Fatalf("unexpected child node.started payload: %+v", childStartedPayload)
	}
}

func TestLifecycleAppliesDefaultQueuedSourceAndMaxDepth(t *testing.T) {
	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: filepath.Join(t.TempDir(), "sigil-runs"),
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected StartExecution success, got %v", err)
	}

	events, err := lifecycle.PersistedEvents()
	if err != nil {
		t.Fatalf("expected persisted events read success, got %v", err)
	}

	queuedPayload := events[0].Payload.(RunQueuedPayload)
	if queuedPayload.Source != RunQueuedSourceInternalResume {
		t.Fatalf("expected default queued source %q, got %q", RunQueuedSourceInternalResume, queuedPayload.Source)
	}

	runningPayload := events[1].Payload.(RunRunningPayload)
	if runningPayload.MaxDepth != 0 {
		t.Fatalf("expected default max_depth 0, got %d", runningPayload.MaxDepth)
	}

	if lifecycle.EventStoreSyncCount() != len(events) {
		t.Fatalf("expected sync count %d, got %d", len(events), lifecycle.EventStoreSyncCount())
	}
}

func TestLifecycleTerminalTransitionsPersistTerminalRunEvents(t *testing.T) {
	testCases := []struct {
		name         string
		transitionFn func(*Lifecycle) error
		expectedType EventType
	}{
		{
			name: "failed transition",
			transitionFn: func(l *Lifecycle) error {
				return l.Fail()
			},
			expectedType: EventTypeRunFailed,
		},
		{
			name: "interrupted transition",
			transitionFn: func(l *Lifecycle) error {
				return l.Interrupt()
			},
			expectedType: EventTypeRunInterrupted,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
				RunsBaseDir: filepath.Join(t.TempDir(), "sigil-runs"),
			})
			if err != nil {
				t.Fatalf("expected lifecycle creation success, got %v", err)
			}
			t.Cleanup(func() {
				_ = lifecycle.Close()
			})

			if err := lifecycle.StartExecution(); err != nil {
				t.Fatalf("expected StartExecution success, got %v", err)
			}

			if err := testCase.transitionFn(lifecycle); err != nil {
				t.Fatalf("expected transition success, got %v", err)
			}

			events, err := lifecycle.PersistedEvents()
			if err != nil {
				t.Fatalf("expected persisted events read success, got %v", err)
			}

			last := events[len(events)-1]
			if last.Type != testCase.expectedType {
				t.Fatalf("expected terminal event type %q, got %q", testCase.expectedType, last.Type)
			}
		})
	}
}
