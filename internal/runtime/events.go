package runtime

import "time"

const (
	// SchemaVersionV1 is the canonical v1 event envelope schema version.
	SchemaVersionV1 = "v1"

	// DefaultRunsBaseDir is the default durable runs base directory.
	DefaultRunsBaseDir = "./sigil/runs"
)

// EventType identifies canonical v1 event types.
type EventType string

const (
	EventTypeRunQueued      EventType = "run.queued"
	EventTypeRunRunning     EventType = "run.running"
	EventTypeNodeStarted    EventType = "node.started"
	EventTypeNodeCompleted  EventType = "node.completed"
	EventTypeRunCompleted   EventType = "run.completed"
	EventTypeRunFailed      EventType = "run.failed"
	EventTypeRunInterrupted EventType = "run.interrupted"
)

// RunQueuedSource identifies run initiation source for run.queued payloads.
type RunQueuedSource string

const (
	RunQueuedSourceCLIRunStart    RunQueuedSource = "cli.run.start"
	RunQueuedSourceAppServerStart RunQueuedSource = "app_server.run.start"
	RunQueuedSourceInternalResume RunQueuedSource = "internal.resume"
)

// NodeRole identifies the role for node.started payloads.
type NodeRole string

const (
	NodeRoleRoot             NodeRole = "root"
	NodeRoleRecursiveSubcall NodeRole = "recursive_subcall"
)

// RunInterruptedReason identifies interruption reasons for run.interrupted payloads.
type RunInterruptedReason string

const (
	RunInterruptedReasonUserRequest    RunInterruptedReason = "user_request"
	RunInterruptedReasonPolicyStop     RunInterruptedReason = "policy_stop"
	RunInterruptedReasonSystemShutdown RunInterruptedReason = "system_shutdown"
)

// EventEnvelope defines the canonical persisted event envelope for v1.
type EventEnvelope struct {
	EventID       string    `json:"event_id"`
	SchemaVersion string    `json:"schema_version"`
	RunID         string    `json:"run_id"`
	Seq           int64     `json:"seq"`
	Timestamp     time.Time `json:"ts"`
	Type          EventType `json:"type"`
	NodeID        *string   `json:"node_id,omitempty"`
	CausationID   string    `json:"causation_id"`
	CorrelationID string    `json:"correlation_id"`
	Payload       any       `json:"payload"`
}

// RunQueuedPayload is the strict payload for run.queued.
type RunQueuedPayload struct {
	Source        RunQueuedSource `json:"source"`
	AppConfigPath *string         `json:"app_config_path,omitempty"`
	RunConfigPath *string         `json:"run_config_path,omitempty"`
}

// RunRunningPayload is the strict payload for run.running.
type RunRunningPayload struct {
	Executor string `json:"executor"`
	MaxDepth int    `json:"max_depth"`
}

// NodeStartedPayload is the strict payload for node.started.
type NodeStartedPayload struct {
	Depth        int      `json:"depth"`
	ParentNodeID *string  `json:"parent_node_id"`
	Role         NodeRole `json:"role"`
	Attempt      int      `json:"attempt"`
}

// NodeCompletedPayload is the strict payload for node.completed.
type NodeCompletedPayload struct {
	Status     string  `json:"status"`
	DurationMS int     `json:"duration_ms"`
	OutputRef  *string `json:"output_ref,omitempty"`
}

// RunCompletedPayload is the strict payload for run.completed.
type RunCompletedPayload struct {
	Status         string  `json:"status"`
	DurationMS     int     `json:"duration_ms"`
	FinalAnswerRef *string `json:"final_answer_ref,omitempty"`
}

// RunFailedPayload is the strict payload for run.failed.
type RunFailedPayload struct {
	Status       string  `json:"status"`
	ErrorCode    string  `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
	FailedNodeID *string `json:"failed_node_id,omitempty"`
	Retryable    bool    `json:"retryable"`
}

// RunInterruptedPayload is the strict payload for run.interrupted.
type RunInterruptedPayload struct {
	Status            string               `json:"status"`
	Reason            RunInterruptedReason `json:"reason"`
	InterruptedBy     *string              `json:"interrupted_by,omitempty"`
	InterruptedNodeID *string              `json:"interrupted_node_id,omitempty"`
}

// LifecycleOptions configures lifecycle defaults and durable event-store behavior.
type LifecycleOptions struct {
	RunsBaseDir   string
	QueuedSource  RunQueuedSource
	AppConfigPath *string
	RunConfigPath *string
	MaxDepth      int
}
