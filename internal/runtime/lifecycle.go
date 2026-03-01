package runtime

import (
	"fmt"
	"strings"

	"github.com/google/uuid"
)

// Lifecycle encapsulates mutable run lifecycle state and node-scoped activity.
type Lifecycle struct {
	runID      string
	state      RunState
	rootNodeID string
	nodes      []Node
	nodeByID   map[string]Node
	activities []NodeActivity
	options    LifecycleOptions
	eventStore *EventStore
}

// NewLifecycle creates a new lifecycle in queued state with default persistent event storage.
func NewLifecycle() (*Lifecycle, error) {
	return NewLifecycleWithOptions(LifecycleOptions{})
}

// NewLifecycleWithOptions creates a new lifecycle in queued state with configurable persistence options.
func NewLifecycleWithOptions(opts LifecycleOptions) (*Lifecycle, error) {
	normalizedOpts, err := normalizeLifecycleOptions(opts)
	if err != nil {
		return nil, err
	}

	runID, err := newUUIDv7String()
	if err != nil {
		return nil, fmt.Errorf("failed to generate run_id; %w", err)
	}

	store, err := NewEventStore(normalizedOpts.RunsBaseDir, runID)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize run event store; %w", err)
	}

	lifecycle := &Lifecycle{
		runID:      runID,
		state:      RunStateQueued,
		nodes:      make([]Node, 0, 4),
		nodeByID:   make(map[string]Node),
		activities: make([]NodeActivity, 0, 8),
		options:    normalizedOpts,
		eventStore: store,
	}

	queuedPayload := RunQueuedPayload{
		Source:        lifecycle.options.QueuedSource,
		AppConfigPath: cloneStringPointer(lifecycle.options.AppConfigPath),
		RunConfigPath: cloneStringPointer(lifecycle.options.RunConfigPath),
	}

	if _, err := lifecycle.eventStore.AppendNext(EventTypeRunQueued, nil, queuedPayload); err != nil {
		_ = lifecycle.eventStore.Close()
		return nil, fmt.Errorf("failed to persist initial run.queued event; %w", err)
	}

	return lifecycle, nil
}

// Close closes lifecycle-owned resources, including the durable event store handle.
func (l *Lifecycle) Close() error {
	if l.eventStore == nil {
		return nil
	}
	return l.eventStore.Close()
}

// RunID returns the lifecycle run identifier.
func (l *Lifecycle) RunID() string {
	return l.runID
}

// State returns the current lifecycle run state.
func (l *Lifecycle) State() RunState {
	return l.state
}

// EventStore returns the lifecycle event store instance.
func (l *Lifecycle) EventStore() *EventStore {
	return l.eventStore
}

// EventsFilePath returns the active lifecycle events.jsonl path.
func (l *Lifecycle) EventsFilePath() (string, error) {
	if l.eventStore == nil {
		return "", fmt.Errorf("event store is not initialized")
	}
	return l.eventStore.EventsFilePath(), nil
}

// PersistedEvents reads and validates currently persisted lifecycle events.
func (l *Lifecycle) PersistedEvents() ([]EventEnvelope, error) {
	if l.eventStore == nil {
		return nil, fmt.Errorf("event store is not initialized")
	}
	return l.eventStore.ReadAll()
}

// ValidateEventLogIntegrity validates lifecycle event persistence integrity.
func (l *Lifecycle) ValidateEventLogIntegrity() error {
	if l.eventStore == nil {
		return fmt.Errorf("event store is not initialized")
	}
	return l.eventStore.ValidateIntegrity()
}

// EventStoreSyncCount returns successful fsync count acknowledged by the event store.
func (l *Lifecycle) EventStoreSyncCount() int {
	if l.eventStore == nil {
		return 0
	}
	return l.eventStore.SyncCount()
}

// StartExecution transitions queued -> running and creates the single root node.
func (l *Lifecycle) StartExecution() error {
	if err := l.validateTransition(RunStateRunning); err != nil {
		return err
	}

	rootNodeID, err := newUUIDv7String()
	if err != nil {
		return fmt.Errorf("failed to generate root node id; %w", err)
	}

	rootNode := Node{
		ID:           rootNodeID,
		RunID:        l.runID,
		Depth:        0,
		ParentNodeID: nil,
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunRunning, nil, RunRunningPayload{
		Executor: "rlm",
		MaxDepth: l.options.MaxDepth,
	}); err != nil {
		return fmt.Errorf("failed to persist run.running event; %w", err)
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeStarted, &rootNode.ID, NodeStartedPayload{
		Depth:        rootNode.Depth,
		ParentNodeID: rootNode.ParentNodeID,
		Role:         NodeRoleRoot,
		Attempt:      1,
	}); err != nil {
		return fmt.Errorf("failed to persist root node.started event; %w", err)
	}

	l.state = RunStateRunning
	l.nodes = append(l.nodes, rootNode)
	l.nodeByID[rootNode.ID] = rootNode
	l.rootNodeID = rootNode.ID

	return nil
}

// CreateChildNode creates a child node under an existing parent while running.
func (l *Lifecycle) CreateChildNode(parentNodeID string) (Node, error) {
	if l.state != RunStateRunning {
		return Node{}, fmt.Errorf("cannot create child node while run state is %q; %w", l.state, ErrRunNotRunning)
	}

	parentNode, ok := l.nodeByID[parentNodeID]
	if !ok {
		return Node{}, fmt.Errorf("parent node %q does not exist in run %q; %w", parentNodeID, l.runID, ErrParentNodeNotFound)
	}

	childNodeID, err := newUUIDv7String()
	if err != nil {
		return Node{}, fmt.Errorf("failed to generate child node id; %w", err)
	}

	parentID := parentNode.ID
	childNode := Node{
		ID:           childNodeID,
		RunID:        l.runID,
		Depth:        parentNode.Depth + 1,
		ParentNodeID: &parentID,
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeStarted, &childNode.ID, NodeStartedPayload{
		Depth:        childNode.Depth,
		ParentNodeID: cloneStringPointer(childNode.ParentNodeID),
		Role:         NodeRoleRecursiveSubcall,
		Attempt:      1,
	}); err != nil {
		return Node{}, fmt.Errorf("failed to persist child node.started event; %w", err)
	}

	l.nodes = append(l.nodes, childNode)
	l.nodeByID[childNode.ID] = childNode

	return cloneNode(childNode), nil
}

// Complete transitions running -> completed.
func (l *Lifecycle) Complete() error {
	if err := l.validateTransition(RunStateCompleted); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunCompleted, nil, RunCompletedPayload{
		Status:     "completed",
		DurationMS: 0,
	}); err != nil {
		return fmt.Errorf("failed to persist run.completed event; %w", err)
	}

	l.state = RunStateCompleted
	return nil
}

// Fail transitions running -> failed.
func (l *Lifecycle) Fail() error {
	if err := l.validateTransition(RunStateFailed); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunFailed, nil, RunFailedPayload{
		Status:       "failed",
		ErrorCode:    "runtime.failure",
		ErrorMessage: "unrecoverable runtime failure",
		Retryable:    false,
	}); err != nil {
		return fmt.Errorf("failed to persist run.failed event; %w", err)
	}

	l.state = RunStateFailed
	return nil
}

// Interrupt transitions running -> interrupted.
func (l *Lifecycle) Interrupt() error {
	if err := l.validateTransition(RunStateInterrupted); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunInterrupted, nil, RunInterruptedPayload{
		Status: "interrupted",
		Reason: RunInterruptedReasonUserRequest,
	}); err != nil {
		return fmt.Errorf("failed to persist run.interrupted event; %w", err)
	}

	l.state = RunStateInterrupted
	return nil
}

// RecordNodeActivity appends node-scoped activity without creating node entities.
func (l *Lifecycle) RecordNodeActivity(nodeID string, kind NodeActivityKind) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot record node activity while run state is %q; %w", l.state, ErrRunNotRunning)
	}

	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}

	switch kind {
	case NodeActivityKindToolExec, NodeActivityKindCodeExec:
	default:
		return fmt.Errorf("unsupported node activity kind %q", kind)
	}

	l.activities = append(l.activities, NodeActivity{
		RunID:  l.runID,
		NodeID: nodeID,
		Kind:   kind,
	})

	return nil
}

// Nodes returns a defensive copy of nodes in creation order.
func (l *Lifecycle) Nodes() []Node {
	nodes := make([]Node, 0, len(l.nodes))
	for _, node := range l.nodes {
		nodes = append(nodes, cloneNode(node))
	}

	return nodes
}

// Activities returns a defensive copy of node-scoped activity records.
func (l *Lifecycle) Activities() []NodeActivity {
	activities := make([]NodeActivity, len(l.activities))
	copy(activities, l.activities)
	return activities
}

func (l *Lifecycle) validateTransition(next RunState) error {
	switch l.state {
	case RunStateCompleted, RunStateFailed, RunStateInterrupted:
		return fmt.Errorf("cannot transition from terminal state %q to %q; %w", l.state, next, ErrTerminalState)
	}

	if !isAllowedTransition(l.state, next) {
		return fmt.Errorf("transition %q -> %q is not allowed; %w", l.state, next, ErrInvalidTransition)
	}

	return nil
}

func isAllowedTransition(current RunState, next RunState) bool {
	switch current {
	case RunStateQueued:
		return next == RunStateRunning
	case RunStateRunning:
		return next == RunStateCompleted || next == RunStateFailed || next == RunStateInterrupted
	default:
		return false
	}
}

func cloneNode(node Node) Node {
	cloned := node
	if node.ParentNodeID != nil {
		parentID := *node.ParentNodeID
		cloned.ParentNodeID = &parentID
	}

	return cloned
}

func newUUIDv7String() (string, error) {
	identifier, err := uuid.NewV7()
	if err != nil {
		return "", err
	}

	return identifier.String(), nil
}

func normalizeLifecycleOptions(options LifecycleOptions) (LifecycleOptions, error) {
	normalized := options

	if stringsTrimmed(options.RunsBaseDir) == "" {
		normalized.RunsBaseDir = DefaultRunsBaseDir
	}
	if stringsTrimmed(string(options.QueuedSource)) == "" {
		normalized.QueuedSource = RunQueuedSourceInternalResume
	}
	if normalized.MaxDepth < 0 {
		return LifecycleOptions{}, fmt.Errorf("max depth must be >= 0; %w", ErrInvalidEvent)
	}

	return normalized, nil
}

func stringsTrimmed(raw string) string {
	return strings.TrimSpace(raw)
}
