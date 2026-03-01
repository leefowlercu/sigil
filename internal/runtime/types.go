package runtime

// RunState is the lifecycle state for a run.
type RunState string

const (
	// RunStateQueued is the initial run state before execution begins.
	RunStateQueued RunState = "queued"
	// RunStateRunning is the state while execution is active.
	RunStateRunning RunState = "running"
	// RunStateCompleted is the terminal success state.
	RunStateCompleted RunState = "completed"
	// RunStateFailed is the terminal failure state.
	RunStateFailed RunState = "failed"
	// RunStateInterrupted is the terminal interruption state.
	RunStateInterrupted RunState = "interrupted"
)

// Node represents a recursive execution node within a run.
type Node struct {
	ID           string
	RunID        string
	Depth        int
	ParentNodeID *string
}

// NodeActivityKind identifies node-scoped runtime activity type.
type NodeActivityKind string

const (
	// NodeActivityKindToolExec represents tool execution activity.
	NodeActivityKindToolExec NodeActivityKind = "tool_exec"
	// NodeActivityKindCodeExec represents code execution activity.
	NodeActivityKindCodeExec NodeActivityKind = "code_exec"
)

// NodeActivity represents node-scoped runtime activity.
type NodeActivity struct {
	RunID  string
	NodeID string
	Kind   NodeActivityKind
}
