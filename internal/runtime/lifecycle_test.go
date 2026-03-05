package runtime

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/google/uuid"
)

func TestNewLifecycleWithOptionsInitializesQueuedStateWithUUIDv7RunID(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsDir,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if lifecycle.State() != RunStateQueued {
		t.Fatalf("expected initial state %q, got %q", RunStateQueued, lifecycle.State())
	}

	assertUUIDv7String(t, lifecycle.RunID())

	eventsPath, err := lifecycle.EventsFilePath()
	if err != nil {
		t.Fatalf("expected events file path retrieval success, got %v", err)
	}
	expectedPath := filepath.Join(runsDir, lifecycle.RunID(), "events.jsonl")
	if eventsPath != expectedPath {
		t.Fatalf("expected events path %q, got %q", expectedPath, eventsPath)
	}
}

func TestStartExecutionTransitionsToRunningAndCreatesSingleRootNode(t *testing.T) {
	lifecycle := mustNewLifecycle(t)

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	if lifecycle.State() != RunStateRunning {
		t.Fatalf("expected state %q, got %q", RunStateRunning, lifecycle.State())
	}

	nodes := lifecycle.Nodes()
	if len(nodes) != 1 {
		t.Fatalf("expected exactly one root node, got %d", len(nodes))
	}

	rootNode := nodes[0]
	if rootNode.Depth != 0 {
		t.Fatalf("expected root depth 0, got %d", rootNode.Depth)
	}
	if rootNode.ParentNodeID != nil {
		t.Fatalf("expected nil parent for root node, got %v", rootNode.ParentNodeID)
	}
	if rootNode.RunID != lifecycle.RunID() {
		t.Fatalf("expected root node run_id %q, got %q", lifecycle.RunID(), rootNode.RunID)
	}
	assertUUIDv7String(t, rootNode.ID)
}

func TestCreateChildNodeRequiresRunningState(t *testing.T) {
	lifecycle := mustNewLifecycle(t)

	if _, err := lifecycle.CreateChildNode("missing"); !errors.Is(err, ErrRunNotRunning) {
		t.Fatalf("expected ErrRunNotRunning, got %v", err)
	}
}

func TestCreateChildNodeRequiresExistingParentInSameRun(t *testing.T) {
	lifecycle := mustNewLifecycle(t)
	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	if _, err := lifecycle.CreateChildNode("missing-parent"); !errors.Is(err, ErrParentNodeNotFound) {
		t.Fatalf("expected ErrParentNodeNotFound, got %v", err)
	}
}

func TestCreateChildNodeUsesParentDepthPlusOne(t *testing.T) {
	lifecycle := mustRunningLifecycle(t)
	rootNode := lifecycle.Nodes()[0]

	childNode, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		t.Fatalf("expected create child success, got %v", err)
	}

	if childNode.Depth != 1 {
		t.Fatalf("expected child depth 1, got %d", childNode.Depth)
	}
	if childNode.ParentNodeID == nil {
		t.Fatal("expected non-nil parent_node_id for child")
	}
	if *childNode.ParentNodeID != rootNode.ID {
		t.Fatalf("expected parent_node_id %q, got %q", rootNode.ID, *childNode.ParentNodeID)
	}
	if childNode.RunID != lifecycle.RunID() {
		t.Fatalf("expected child run_id %q, got %q", lifecycle.RunID(), childNode.RunID)
	}
	assertUUIDv7String(t, childNode.ID)
}

func TestCreateChildNodeRejectsWhenDepthWouldExceedMaxDepth(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsDir,
		MaxDepth:    0,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode := lifecycle.Nodes()[0]

	if _, err := lifecycle.CreateChildNode(rootNode.ID); !errors.Is(err, ErrDepthLimitExceeded) {
		t.Fatalf("expected ErrDepthLimitExceeded, got %v", err)
	}
}

func TestTransitionRulesFromQueuedAndRunning(t *testing.T) {
	testCases := []struct {
		name          string
		setup         func(*Lifecycle) error
		action        func(*Lifecycle) error
		expectedErr   error
		expectedState RunState
	}{
		{
			name: "queued to completed invalid",
			action: func(l *Lifecycle) error {
				return l.Complete()
			},
			expectedErr: ErrInvalidTransition,
		},
		{
			name: "queued to failed invalid",
			action: func(l *Lifecycle) error {
				return l.Fail()
			},
			expectedErr: ErrInvalidTransition,
		},
		{
			name: "queued to interrupted invalid",
			action: func(l *Lifecycle) error {
				return l.Interrupt()
			},
			expectedErr: ErrInvalidTransition,
		},
		{
			name: "queued to running valid",
			action: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			expectedState: RunStateRunning,
		},
		{
			name: "running to completed valid",
			setup: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			action: func(l *Lifecycle) error {
				return l.Complete()
			},
			expectedState: RunStateCompleted,
		},
		{
			name: "running to failed valid",
			setup: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			action: func(l *Lifecycle) error {
				return l.Fail()
			},
			expectedState: RunStateFailed,
		},
		{
			name: "running to interrupted valid",
			setup: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			action: func(l *Lifecycle) error {
				return l.Interrupt()
			},
			expectedState: RunStateInterrupted,
		},
		{
			name: "running to running invalid",
			setup: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			action: func(l *Lifecycle) error {
				return l.StartExecution()
			},
			expectedErr: ErrInvalidTransition,
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			lifecycle := mustNewLifecycle(t)

			if testCase.setup != nil {
				if err := testCase.setup(lifecycle); err != nil {
					t.Fatalf("setup failed: %v", err)
				}
			}

			err := testCase.action(lifecycle)
			if testCase.expectedErr != nil {
				if !errors.Is(err, testCase.expectedErr) {
					t.Fatalf("expected error %v, got %v", testCase.expectedErr, err)
				}
				return
			}

			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}

			if lifecycle.State() != testCase.expectedState {
				t.Fatalf("expected state %q, got %q", testCase.expectedState, lifecycle.State())
			}
		})
	}
}

func TestTerminalStatesRejectFurtherTransitions(t *testing.T) {
	testCases := []struct {
		name           string
		enterTerminal  func(*Lifecycle) error
		invalidRequest func(*Lifecycle) error
	}{
		{
			name: "completed",
			enterTerminal: func(l *Lifecycle) error {
				if err := l.StartExecution(); err != nil {
					return err
				}
				return l.Complete()
			},
			invalidRequest: func(l *Lifecycle) error {
				return l.Interrupt()
			},
		},
		{
			name: "failed",
			enterTerminal: func(l *Lifecycle) error {
				if err := l.StartExecution(); err != nil {
					return err
				}
				return l.Fail()
			},
			invalidRequest: func(l *Lifecycle) error {
				return l.Complete()
			},
		},
		{
			name: "interrupted",
			enterTerminal: func(l *Lifecycle) error {
				if err := l.StartExecution(); err != nil {
					return err
				}
				return l.Interrupt()
			},
			invalidRequest: func(l *Lifecycle) error {
				return l.Fail()
			},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			lifecycle := mustNewLifecycle(t)
			if err := testCase.enterTerminal(lifecycle); err != nil {
				t.Fatalf("failed to enter terminal state: %v", err)
			}

			err := testCase.invalidRequest(lifecycle)
			if !errors.Is(err, ErrTerminalState) {
				t.Fatalf("expected ErrTerminalState, got %v", err)
			}
		})
	}
}

func TestRecordNodeActivityRequiresRunningAndExistingNode(t *testing.T) {
	lifecycle := mustNewLifecycle(t)
	if err := lifecycle.RecordNodeActivity("missing", NodeActivityKindToolExec); !errors.Is(err, ErrRunNotRunning) {
		t.Fatalf("expected ErrRunNotRunning, got %v", err)
	}

	runningLifecycle := mustRunningLifecycle(t)
	if err := runningLifecycle.RecordNodeActivity("missing", NodeActivityKindToolExec); !errors.Is(err, ErrNodeNotFound) {
		t.Fatalf("expected ErrNodeNotFound, got %v", err)
	}
}

func TestRecordNodeActivityDoesNotCreateAdditionalNodes(t *testing.T) {
	lifecycle := mustRunningLifecycle(t)
	rootNode := lifecycle.Nodes()[0]
	childNode, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		t.Fatalf("expected child creation success, got %v", err)
	}

	nodeCountBefore := len(lifecycle.Nodes())
	if err := lifecycle.RecordNodeActivity(childNode.ID, NodeActivityKindToolExec); err != nil {
		t.Fatalf("expected tool activity record success, got %v", err)
	}
	if err := lifecycle.RecordNodeActivity(childNode.ID, NodeActivityKindCodeExec); err != nil {
		t.Fatalf("expected code activity record success, got %v", err)
	}
	nodeCountAfter := len(lifecycle.Nodes())

	if nodeCountBefore != nodeCountAfter {
		t.Fatalf("expected node count unchanged, before=%d after=%d", nodeCountBefore, nodeCountAfter)
	}

	activities := lifecycle.Activities()
	if len(activities) != 2 {
		t.Fatalf("expected 2 activities, got %d", len(activities))
	}
	if activities[0].Kind != NodeActivityKindToolExec || activities[1].Kind != NodeActivityKindCodeExec {
		t.Fatalf("unexpected activity kinds: %+v", activities)
	}
}

func TestNodesAndActivitiesReturnDefensiveCopies(t *testing.T) {
	lifecycle := mustRunningLifecycle(t)
	rootNode := lifecycle.Nodes()[0]
	_, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		t.Fatalf("expected child creation success, got %v", err)
	}

	nodes := lifecycle.Nodes()
	if len(nodes) < 2 {
		t.Fatalf("expected at least 2 nodes, got %d", len(nodes))
	}

	nodes[1].Depth = 99
	if nodes[1].ParentNodeID != nil {
		*nodes[1].ParentNodeID = "tampered-parent"
	}

	restoredNodes := lifecycle.Nodes()
	if restoredNodes[1].Depth == 99 {
		t.Fatalf("expected defensive node copy, got mutable depth %d", restoredNodes[1].Depth)
	}
	if restoredNodes[1].ParentNodeID != nil && *restoredNodes[1].ParentNodeID == "tampered-parent" {
		t.Fatal("expected defensive node parent copy, observed tampered parent")
	}

	if err := lifecycle.RecordNodeActivity(restoredNodes[1].ID, NodeActivityKindToolExec); err != nil {
		t.Fatalf("expected activity record success, got %v", err)
	}
	activities := lifecycle.Activities()
	activities[0].Kind = NodeActivityKindCodeExec

	refetchedActivities := lifecycle.Activities()
	if refetchedActivities[0].Kind != NodeActivityKindToolExec {
		t.Fatalf("expected defensive activity copy, got %q", refetchedActivities[0].Kind)
	}
}

func TestRootNodeAndNodeByIDAccessors(t *testing.T) {
	lifecycle := mustRunningLifecycle(t)
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node lookup success, got %v", err)
	}

	lookupNode, err := lifecycle.NodeByID(rootNode.ID)
	if err != nil {
		t.Fatalf("expected node-by-id lookup success, got %v", err)
	}
	if lookupNode.ID != rootNode.ID {
		t.Fatalf("expected node id %q, got %q", rootNode.ID, lookupNode.ID)
	}
}

func TestNodeTerminalEventExclusivityBetweenCompletedAndFailed(t *testing.T) {
	lifecycle := mustRunningLifecycle(t)
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node lookup success, got %v", err)
	}

	if err := lifecycle.CompleteNode(rootNode.ID, nil); err != nil {
		t.Fatalf("expected node completion success, got %v", err)
	}

	err = lifecycle.AppendNodeFailed(rootNode.ID, NodeFailedPayload{
		Status:       "failed",
		DurationMS:   1,
		ErrorCode:    "harness_inference",
		ErrorMessage: "failed",
	})
	if !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("expected ErrInvalidTransition, got %v", err)
	}
}

func mustNewLifecycle(t *testing.T) *Lifecycle {
	t.Helper()

	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsDir,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}

	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	return lifecycle
}

func mustRunningLifecycle(t *testing.T) *Lifecycle {
	t.Helper()

	lifecycle := mustNewLifecycle(t)
	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	return lifecycle
}

func assertUUIDv7String(t *testing.T, raw string) {
	t.Helper()

	parsed, err := uuid.Parse(raw)
	if err != nil {
		t.Fatalf("expected valid UUID, got %q with error %v", raw, err)
	}

	if parsed.Version() != uuid.Version(7) {
		t.Fatalf("expected UUIDv7, got version %d for %q", parsed.Version(), raw)
	}
}
