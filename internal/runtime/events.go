package runtime

import (
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
)

const (
	// SchemaVersionV1 is the canonical v1 event envelope schema version.
	SchemaVersionV1 = "v1"

	// DefaultRunsBaseDir is the default durable runs base directory.
	DefaultRunsBaseDir = "./.sigil/runs"
)

// EventType identifies canonical v1 event types.
type EventType string

const (
	EventTypeRunQueued           EventType = "run.queued"
	EventTypeRunRunning          EventType = "run.running"
	EventTypeNodeStarted         EventType = "node.started"
	EventTypeNodeCompleted       EventType = "node.completed"
	EventTypeNodeFailed          EventType = "node.failed"
	EventTypeNodeStepStarted     EventType = "node.step.started"
	EventTypeNodeStepCompleted   EventType = "node.step.completed"
	EventTypeNodeTurnUser        EventType = "node.turn.user"
	EventTypeNodeTurnModel       EventType = "node.turn.model"
	EventTypeNodeSubcallExecuted EventType = "node.subcall.executed"
	EventTypeNodeActionExecuted  EventType = "node.action.executed"
	EventTypeRunCompleted        EventType = "run.completed"
	EventTypeRunFailed           EventType = "run.failed"
	EventTypeRunInterrupted      EventType = "run.interrupted"
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
	RunInterruptedByLifecycle          string               = "runtime.lifecycle"
)

// StepDecision identifies node step decisions.
type StepDecision string

const (
	StepDecisionContinue StepDecision = "continue"
	StepDecisionFinal    StepDecision = "final"
)

// TurnRole identifies node turn payload roles.
type TurnRole string

const (
	TurnRoleUser  TurnRole = "user"
	TurnRoleModel TurnRole = "model"
)

// ActionExecutionStatus identifies node.action.executed status values.
type ActionExecutionStatus string

const (
	ActionExecutionStatusCompleted ActionExecutionStatus = "completed"
	ActionExecutionStatusFailed    ActionExecutionStatus = "failed"
)

// SubcallType identifies node.subcall.executed subcall API type values.
type SubcallType string

const (
	SubcallTypeLLMQuery        SubcallType = "llm_query"
	SubcallTypeLLMQueryBatched SubcallType = "llm_query_batched"
	SubcallTypeRLMQuery        SubcallType = "rlm_query"
	SubcallTypeRLMQueryBatched SubcallType = "rlm_query_batched"
)

// SubcallExecutionMode identifies node.subcall.executed execution mode values.
type SubcallExecutionMode string

const (
	SubcallExecutionModePlain     SubcallExecutionMode = "plain"
	SubcallExecutionModeRecursive SubcallExecutionMode = "recursive"
	SubcallExecutionModeFallback  SubcallExecutionMode = "fallback"
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
	Status        string            `json:"status"`
	DurationMS    int               `json:"duration_ms"`
	OutputRef     *string           `json:"output_ref,omitempty"`
	Accounting    accounting.Rollup `json:"accounting"`
	AccountingRef *string           `json:"accounting_ref,omitempty"`
}

// NodeFailedPayload is the strict payload for node.failed.
type NodeFailedPayload struct {
	Status       string  `json:"status"`
	DurationMS   int     `json:"duration_ms"`
	ErrorCode    string  `json:"error_code"`
	ErrorMessage string  `json:"error_message"`
	FailedStepID *string `json:"failed_step_id,omitempty"`
}

// NodeStepStartedPayload is the strict payload for node.step.started.
type NodeStepStartedPayload struct {
	StepID    string `json:"step_id"`
	StepIndex int    `json:"step_index"`
	SchemaID  string `json:"schema_id"`
}

// NodeStepCompletedPayload is the strict payload for node.step.completed.
type NodeStepCompletedPayload struct {
	StepID        string            `json:"step_id"`
	Decision      StepDecision      `json:"decision"`
	ActionCount   int               `json:"action_count"`
	DurationMS    int               `json:"duration_ms"`
	Accounting    accounting.Rollup `json:"accounting"`
	AccountingRef string            `json:"accounting_ref"`
}

// NodeTurnPayload is the strict payload for node.turn.user and node.turn.model.
type NodeTurnPayload struct {
	StepID     string   `json:"step_id"`
	Role       TurnRole `json:"role"`
	ContentRef string   `json:"content_ref"`
}

// NodeSubcallExecutedPayload is the strict payload for node.subcall.executed.
type NodeSubcallExecutedPayload struct {
	StepID        string                `json:"step_id"`
	ActionIndex   int                   `json:"action_index"`
	SubcallIndex  int                   `json:"subcall_index"`
	SubcallType   SubcallType           `json:"subcall_type"`
	ExecutionMode SubcallExecutionMode  `json:"execution_mode"`
	Status        ActionExecutionStatus `json:"status"`
	Provider      string                `json:"provider"`
	Model         string                `json:"model"`
	PromptBytes   int                   `json:"prompt_bytes"`
	ContextBytes  int                   `json:"context_bytes"`
	AnswerBytes   int                   `json:"answer_bytes"`
	DurationMS    int                   `json:"duration_ms"`
	ChildNodeID   *string               `json:"child_node_id,omitempty"`
	Accounting    accounting.Summary    `json:"accounting"`
	AccountingRef string                `json:"accounting_ref"`
	ErrorCode     *string               `json:"error_code,omitempty"`
	ErrorMessage  *string               `json:"error_message,omitempty"`
}

// NodeActionExecutedPayload is the strict payload for node.action.executed.
type NodeActionExecutedPayload struct {
	StepID       string                `json:"step_id"`
	ActionIndex  int                   `json:"action_index"`
	ActionType   string                `json:"action_type"`
	Language     string                `json:"language"`
	Status       ActionExecutionStatus `json:"status"`
	DurationMS   int                   `json:"duration_ms"`
	OutputRef    string                `json:"output_ref"`
	ErrorCode    *string               `json:"error_code,omitempty"`
	ErrorMessage *string               `json:"error_message,omitempty"`
}

// RunCompletedPayload is the strict payload for run.completed.
type RunCompletedPayload struct {
	Status         string            `json:"status"`
	DurationMS     int               `json:"duration_ms"`
	FinalAnswerRef *string           `json:"final_answer_ref,omitempty"`
	Accounting     accounting.Rollup `json:"accounting"`
	AccountingRef  *string           `json:"accounting_ref,omitempty"`
}

// RunFailedPayload is the strict payload for run.failed.
type RunFailedPayload struct {
	Status          string            `json:"status"`
	ErrorCode       string            `json:"error_code"`
	ErrorMessage    string            `json:"error_message"`
	FailedNodeID    *string           `json:"failed_node_id,omitempty"`
	FailedStepID    *string           `json:"failed_step_id,omitempty"`
	LimitKey        *string           `json:"limit_key,omitempty"`
	ConfiguredValue *string           `json:"configured_value,omitempty"`
	ObservedValue   *string           `json:"observed_value,omitempty"`
	Retryable       bool              `json:"retryable"`
	Accounting      accounting.Rollup `json:"accounting"`
	AccountingRef   *string           `json:"accounting_ref,omitempty"`
}

// RunInterruptedPayload is the strict payload for run.interrupted.
type RunInterruptedPayload struct {
	Status            string               `json:"status"`
	Reason            RunInterruptedReason `json:"reason"`
	InterruptedBy     *string              `json:"interrupted_by,omitempty"`
	InterruptedNodeID *string              `json:"interrupted_node_id,omitempty"`
	Accounting        accounting.Rollup    `json:"accounting"`
	AccountingRef     *string              `json:"accounting_ref,omitempty"`
}

// LifecycleOptions configures lifecycle defaults and durable event-store behavior.
type LifecycleOptions struct {
	RunsBaseDir     string
	QueuedSource    RunQueuedSource
	AppConfigPath   *string
	RunConfigPath   *string
	MaxDepth        int
	ProcessMetadata *ProcessMetadata
}
