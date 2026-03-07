package runtime

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/accounting"
)

// Lifecycle encapsulates mutable run lifecycle state and node-scoped activity.
type Lifecycle struct {
	runID            string
	state            RunState
	rootNodeID       string
	nodes            []Node
	nodeByID         map[string]Node
	nodeTerminalByID map[string]EventType
	stepByNode       map[string]int
	activities       []NodeActivity
	options          LifecycleOptions
	eventStore       *EventStore
}

// NewLifecycle creates a new lifecycle in queued state with default persistent event storage.
func NewLifecycle() (*Lifecycle, error) {
	return NewLifecycleWithOptions(LifecycleOptions{})
}

// NewLifecycleWithOptions creates a new lifecycle in queued state with configurable persistence options.
func NewLifecycleWithOptions(opts LifecycleOptions) (*Lifecycle, error) {
	logger := lifecycleLogger()

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
		runID:            runID,
		state:            RunStateQueued,
		nodes:            make([]Node, 0, 4),
		nodeByID:         make(map[string]Node),
		nodeTerminalByID: make(map[string]EventType),
		stepByNode:       make(map[string]int),
		activities:       make([]NodeActivity, 0, 8),
		options:          normalizedOpts,
		eventStore:       store,
	}

	queuedPayload := RunQueuedPayload{
		Source:        lifecycle.options.QueuedSource,
		AppConfigPath: cloneStringPointer(lifecycle.options.AppConfigPath),
		RunConfigPath: cloneStringPointer(lifecycle.options.RunConfigPath),
	}

	processMetadataWritten := false
	if normalizedOpts.ProcessMetadata != nil {
		metadata := *normalizedOpts.ProcessMetadata
		metadata.RunID = runID
		if err := WriteProcessMetadata(normalizedOpts.RunsBaseDir, metadata); err != nil {
			_ = lifecycle.eventStore.Close()
			return nil, fmt.Errorf("failed to persist process metadata; %w", err)
		}
		processMetadataWritten = true
	}
	if _, err := lifecycle.eventStore.AppendNext(EventTypeRunQueued, nil, queuedPayload); err != nil {
		if processMetadataWritten {
			_ = RemoveProcessMetadata(normalizedOpts.RunsBaseDir, runID)
		}
		_ = lifecycle.eventStore.Close()
		return nil, fmt.Errorf("failed to persist initial run.queued event; %w", err)
	}

	logger.Info("initialized lifecycle in queued state",
		"run_id", lifecycle.runID,
		"runs_base_dir", normalizedOpts.RunsBaseDir,
		"queued_source", normalizedOpts.QueuedSource,
		"max_depth", normalizedOpts.MaxDepth,
	)

	return lifecycle, nil
}

// Close closes lifecycle-owned resources, including the durable event store handle.
func (l *Lifecycle) Close() error {
	if l.eventStore == nil {
		return nil
	}
	if err := l.eventStore.Close(); err != nil {
		lifecycleLogger().Error("failed to close lifecycle event store", "run_id", l.runID, "error", err)
		return err
	}
	lifecycleLogger().Debug("closed lifecycle event store", "run_id", l.runID)
	return nil
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

	lifecycleLogger().Info("started run execution",
		"run_id", l.runID,
		"root_node_id", rootNode.ID,
		"max_depth", l.options.MaxDepth,
	)

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

	nextDepth := parentNode.Depth + 1
	if nextDepth > l.options.MaxDepth {
		lifecycleLogger().Warn("child node depth exceeds max depth",
			"run_id", l.runID,
			"parent_node_id", parentNodeID,
			"next_depth", nextDepth,
			"max_depth", l.options.MaxDepth,
		)
		return Node{}, fmt.Errorf("child depth %d exceeds max_depth %d; %w", nextDepth, l.options.MaxDepth, ErrDepthLimitExceeded)
	}

	parentID := parentNode.ID
	childNode := Node{
		ID:           childNodeID,
		RunID:        l.runID,
		Depth:        nextDepth,
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

	lifecycleLogger().Info("created child node",
		"run_id", l.runID,
		"parent_node_id", parentNodeID,
		"child_node_id", childNode.ID,
		"child_depth", childNode.Depth,
	)

	return cloneNode(childNode), nil
}

// Complete transitions running -> completed.
func (l *Lifecycle) Complete() error {
	return l.CompleteWith(nil)
}

// CompleteWith transitions running -> completed with optional final answer reference.
func (l *Lifecycle) CompleteWith(finalAnswerRef *string) error {
	return l.CompleteWithAccounting(finalAnswerRef, unavailableRollupForLifecycle(), nil)
}

// CompleteWithAccounting transitions running -> completed with accounting metadata.
func (l *Lifecycle) CompleteWithAccounting(finalAnswerRef *string, runAccounting accounting.Rollup, accountingRef *string) error {
	if err := l.validateTransition(RunStateCompleted); err != nil {
		return err
	}
	if err := ensureRollupOrDefault(&runAccounting); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunCompleted, nil, RunCompletedPayload{
		Status:         "completed",
		DurationMS:     0,
		FinalAnswerRef: cloneStringPointer(finalAnswerRef),
		Accounting:     runAccounting,
		AccountingRef:  cloneStringPointer(accountingRef),
	}); err != nil {
		return fmt.Errorf("failed to persist run.completed event; %w", err)
	}

	l.state = RunStateCompleted
	lifecycleLogger().Info("run transitioned to completed",
		"run_id", l.runID,
		"final_answer_ref", valueOrEmptyString(finalAnswerRef),
	)
	return nil
}

// Fail transitions running -> failed.
func (l *Lifecycle) Fail() error {
	return l.FailWith(RunFailedPayload{
		Status:       "failed",
		ErrorCode:    "runtime.failure",
		ErrorMessage: "unrecoverable runtime failure",
		Retryable:    false,
		Accounting:   unavailableRollupForLifecycle(),
	})
}

// FailWith transitions running -> failed with caller-provided typed failure payload.
func (l *Lifecycle) FailWith(payload RunFailedPayload) error {
	if err := l.validateTransition(RunStateFailed); err != nil {
		return err
	}
	if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunFailed, nil, payload); err != nil {
		return fmt.Errorf("failed to persist run.failed event; %w", err)
	}

	l.state = RunStateFailed
	lifecycleLogger().Error("run transitioned to failed",
		"run_id", l.runID,
		"error_code", payload.ErrorCode,
		"error_message", payload.ErrorMessage,
		"failed_node_id", valueOrEmptyString(payload.FailedNodeID),
	)
	return nil
}

// Interrupt transitions running -> interrupted.
func (l *Lifecycle) Interrupt() error {
	interruptedBy := RunInterruptedByLifecycle
	return l.InterruptWith(RunInterruptedPayload{
		Status:        "interrupted",
		Reason:        RunInterruptedReasonUserRequest,
		InterruptedBy: &interruptedBy,
		Accounting:    unavailableRollupForLifecycle(),
	})
}

// InterruptWith transitions running -> interrupted with caller-provided payload.
func (l *Lifecycle) InterruptWith(payload RunInterruptedPayload) error {
	if err := l.validateTransition(RunStateInterrupted); err != nil {
		return err
	}
	if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeRunInterrupted, nil, payload); err != nil {
		return fmt.Errorf("failed to persist run.interrupted event; %w", err)
	}

	l.state = RunStateInterrupted
	lifecycleLogger().Warn("run transitioned to interrupted",
		"run_id", l.runID,
		"reason", payload.Reason,
	)
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

// RootNode returns the run root node when execution has started.
func (l *Lifecycle) RootNode() (Node, error) {
	if stringsTrimmed(l.rootNodeID) == "" {
		return Node{}, fmt.Errorf("root node is not initialized; %w", ErrNodeNotFound)
	}

	rootNode, ok := l.nodeByID[l.rootNodeID]
	if !ok {
		return Node{}, fmt.Errorf("root node %q not found; %w", l.rootNodeID, ErrNodeNotFound)
	}

	return cloneNode(rootNode), nil
}

// NodeByID returns a defensive copy of node by id.
func (l *Lifecycle) NodeByID(nodeID string) (Node, error) {
	node, ok := l.nodeByID[nodeID]
	if !ok {
		return Node{}, fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}

	return cloneNode(node), nil
}

// MaxDepth returns configured max recursion depth for the run lifecycle.
func (l *Lifecycle) MaxDepth() int {
	return l.options.MaxDepth
}

// NodeTerminalEvent returns terminal event type for a node when terminalized.
func (l *Lifecycle) NodeTerminalEvent(nodeID string) (EventType, bool) {
	eventType, ok := l.nodeTerminalByID[nodeID]
	return eventType, ok
}

// AppendNodeStepStarted appends node.step.started and returns generated payload.
func (l *Lifecycle) AppendNodeStepStarted(nodeID string) (NodeStepStartedPayload, error) {
	if l.state != RunStateRunning {
		return NodeStepStartedPayload{}, fmt.Errorf("cannot append node.step.started while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return NodeStepStartedPayload{}, fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}

	stepID, err := newUUIDv7String()
	if err != nil {
		return NodeStepStartedPayload{}, fmt.Errorf("failed to generate step_id; %w", err)
	}
	stepIndex := l.stepByNode[nodeID] + 1

	payload := NodeStepStartedPayload{
		StepID:    stepID,
		StepIndex: stepIndex,
		SchemaID:  nodeStepSchemaIDV1,
	}
	if _, err := l.eventStore.AppendNext(EventTypeNodeStepStarted, &nodeID, payload); err != nil {
		return NodeStepStartedPayload{}, fmt.Errorf("failed to persist node.step.started event; %w", err)
	}

	l.stepByNode[nodeID] = stepIndex
	lifecycleLogger().Debug("appended node.step.started",
		"run_id", l.runID,
		"node_id", nodeID,
		"step_id", stepID,
		"step_index", stepIndex,
	)
	return payload, nil
}

// AppendNodeTurn appends node.turn.user or node.turn.model events.
func (l *Lifecycle) AppendNodeTurn(nodeID string, role TurnRole, stepID string, contentRef string) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot append node.turn while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}

	eventType := EventTypeNodeTurnUser
	if role == TurnRoleModel {
		eventType = EventTypeNodeTurnModel
	}
	payload := NodeTurnPayload{
		StepID:     stepID,
		Role:       role,
		ContentRef: contentRef,
	}
	if _, err := l.eventStore.AppendNext(eventType, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist %s event; %w", eventType, err)
	}

	lifecycleLogger().Debug("appended node turn event",
		"run_id", l.runID,
		"node_id", nodeID,
		"step_id", stepID,
		"role", role,
		"content_ref", contentRef,
	)

	return nil
}

// AppendNodeSubcallExecuted appends node.subcall.executed events.
func (l *Lifecycle) AppendNodeSubcallExecuted(nodeID string, payload NodeSubcallExecutedPayload) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot append node.subcall.executed while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}
	if err := ensureSummaryOrDefault(&payload.Accounting, payload.Provider, payload.Model); err != nil {
		return err
	}
	if strings.TrimSpace(payload.AccountingRef) == "" {
		payload.AccountingRef = defaultStepSubcallAccountingRef(nodeID, payload.StepID, payload.SubcallIndex)
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeSubcallExecuted, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist node.subcall.executed event; %w", err)
	}

	if payload.Status == ActionExecutionStatusFailed {
		lifecycleLogger().Warn("appended failed node.subcall.executed",
			"run_id", l.runID,
			"node_id", nodeID,
			"step_id", payload.StepID,
			"subcall_index", payload.SubcallIndex,
			"subcall_type", payload.SubcallType,
			"error_code", valueOrEmptyString(payload.ErrorCode),
		)
	} else {
		lifecycleLogger().Debug("appended node.subcall.executed",
			"run_id", l.runID,
			"node_id", nodeID,
			"step_id", payload.StepID,
			"subcall_index", payload.SubcallIndex,
			"subcall_type", payload.SubcallType,
		)
	}

	return nil
}

// AppendNodeActionExecuted appends node.action.executed events.
func (l *Lifecycle) AppendNodeActionExecuted(nodeID string, payload NodeActionExecutedPayload) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot append node.action.executed while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeActionExecuted, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist node.action.executed event; %w", err)
	}

	if payload.Status == ActionExecutionStatusFailed {
		lifecycleLogger().Warn("appended failed node.action.executed",
			"run_id", l.runID,
			"node_id", nodeID,
			"step_id", payload.StepID,
			"action_index", payload.ActionIndex,
			"error_code", valueOrEmptyString(payload.ErrorCode),
			"output_ref", payload.OutputRef,
		)
	} else {
		lifecycleLogger().Debug("appended node.action.executed",
			"run_id", l.runID,
			"node_id", nodeID,
			"step_id", payload.StepID,
			"action_index", payload.ActionIndex,
			"output_ref", payload.OutputRef,
		)
	}

	return nil
}

// AppendNodeStepCompleted appends node.step.completed events.
func (l *Lifecycle) AppendNodeStepCompleted(nodeID string, payload NodeStepCompletedPayload) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot append node.step.completed while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}
	if err := ensureRollupOrDefault(&payload.Accounting); err != nil {
		return err
	}
	if strings.TrimSpace(payload.AccountingRef) == "" {
		payload.AccountingRef = defaultStepAccountingRef(nodeID, payload.StepID)
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeStepCompleted, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist node.step.completed event; %w", err)
	}

	lifecycleLogger().Debug("appended node.step.completed",
		"run_id", l.runID,
		"node_id", nodeID,
		"step_id", payload.StepID,
		"decision", payload.Decision,
		"action_count", payload.ActionCount,
		"duration_ms", payload.DurationMS,
	)

	return nil
}

// CompleteNode appends node.completed for an existing node.
func (l *Lifecycle) CompleteNode(nodeID string, outputRef *string) error {
	return l.CompleteNodeWithAccounting(nodeID, outputRef, unavailableRollupForLifecycle(), nil)
}

// CompleteNodeWithAccounting appends node.completed for an existing node.
func (l *Lifecycle) CompleteNodeWithAccounting(nodeID string, outputRef *string, nodeAccounting accounting.Rollup, accountingRef *string) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot complete node while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}
	if err := l.ensureNodeTerminalAvailable(nodeID); err != nil {
		return err
	}
	if err := ensureRollupOrDefault(&nodeAccounting); err != nil {
		return err
	}

	payload := NodeCompletedPayload{
		Status:        "completed",
		DurationMS:    0,
		OutputRef:     cloneStringPointer(outputRef),
		Accounting:    nodeAccounting,
		AccountingRef: cloneStringPointer(accountingRef),
	}
	if _, err := l.eventStore.AppendNext(EventTypeNodeCompleted, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist node.completed event; %w", err)
	}

	lifecycleLogger().Info("appended node.completed",
		"run_id", l.runID,
		"node_id", nodeID,
		"output_ref", valueOrEmptyString(outputRef),
	)
	l.nodeTerminalByID[nodeID] = EventTypeNodeCompleted

	return nil
}

// AppendNodeFailed appends node.failed for an existing node.
func (l *Lifecycle) AppendNodeFailed(nodeID string, payload NodeFailedPayload) error {
	if l.state != RunStateRunning {
		return fmt.Errorf("cannot fail node while run state is %q; %w", l.state, ErrRunNotRunning)
	}
	if _, ok := l.nodeByID[nodeID]; !ok {
		return fmt.Errorf("node %q does not exist in run %q; %w", nodeID, l.runID, ErrNodeNotFound)
	}
	if err := l.ensureNodeTerminalAvailable(nodeID); err != nil {
		return err
	}

	if _, err := l.eventStore.AppendNext(EventTypeNodeFailed, &nodeID, payload); err != nil {
		return fmt.Errorf("failed to persist node.failed event; %w", err)
	}

	lifecycleLogger().Warn("appended node.failed",
		"run_id", l.runID,
		"node_id", nodeID,
		"error_code", payload.ErrorCode,
	)
	l.nodeTerminalByID[nodeID] = EventTypeNodeFailed

	return nil
}

func (l *Lifecycle) ensureNodeTerminalAvailable(nodeID string) error {
	if prior, exists := l.nodeTerminalByID[nodeID]; exists {
		return fmt.Errorf("node %q already has terminal event %q; %w", nodeID, prior, ErrInvalidTransition)
	}
	return nil
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
		return next == RunStateRunning || next == RunStateInterrupted
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

func lifecycleLogger() *slog.Logger {
	return slog.Default().With("component", "runtime.lifecycle")
}

func valueOrEmptyString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func ensureSummaryOrDefault(summary *accounting.Summary, provider string, model string) error {
	if summary == nil {
		return fmt.Errorf("accounting summary is required")
	}
	if summary.Currency == "" {
		*summary = accounting.UnavailableSummary(provider, model, "")
	}
	return accounting.ValidateSummary(*summary)
}

func ensureRollupOrDefault(rollup *accounting.Rollup) error {
	if rollup == nil {
		return fmt.Errorf("accounting rollup is required")
	}
	if rollup.ModelTotal.Currency == "" {
		*rollup = unavailableRollupForLifecycle()
	}
	return accounting.ValidateRollup(*rollup)
}

func unavailableRollupForLifecycle() accounting.Rollup {
	return accounting.BuildRollup(
		"",
		"",
		"",
		accounting.UnavailableSummary("", "", ""),
		accounting.ZeroSummary("", "", ""),
	)
}

func defaultStepAccountingRef(nodeID string, stepID string) string {
	return fmt.Sprintf("run-output://node/%s/step/%s/accounting.json", nodeID, stepID)
}

func defaultStepSubcallAccountingRef(nodeID string, stepID string, subcallIndex int) string {
	return fmt.Sprintf("run-output://node/%s/step/%s/subcall-%d-accounting.json", nodeID, stepID, subcallIndex)
}
