package protocol

import "time"

const (
	// JSONRPCVersion is the JSON-RPC version used on the wire.
	JSONRPCVersion = "2.0"

	// VersionV1Alpha1 is the initial Sigil app-server protocol version.
	VersionV1Alpha1 = "sigil.appserver.v1alpha1"
)

const (
	// CodeParseError is the JSON-RPC parse error code.
	CodeParseError = -32700
	// CodeInvalidRequest is the JSON-RPC invalid request code.
	CodeInvalidRequest = -32600
	// CodeMethodNotFound is the JSON-RPC method not found code.
	CodeMethodNotFound = -32601
	// CodeInvalidParams is the JSON-RPC invalid params code.
	CodeInvalidParams = -32602
	// CodeInternalError is the JSON-RPC internal error code.
	CodeInternalError = -32603
)

// MethodKind differentiates requests from notifications.
type MethodKind string

const (
	// MethodKindRequest identifies a JSON-RPC request method.
	MethodKindRequest MethodKind = "request"
	// MethodKindNotification identifies a JSON-RPC notification method.
	MethodKindNotification MethodKind = "notification"
)

// ClientInfo identifies one connecting client.
type ClientInfo struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
}

// ConfigCapability describes supported client-config contract versions.
type ConfigCapability struct {
	DefaultVersion    string   `json:"defaultVersion"`
	SupportedVersions []string `json:"supportedVersions"`
}

// ViewCapabilities describes read-model surfaces the server can support.
type ViewCapabilities struct {
	RunTree           bool `json:"runTree"`
	StepDetail        bool `json:"stepDetail"`
	LiveSubscriptions bool `json:"liveSubscriptions"`
}

// ServerCapabilities describes rich-client capabilities advertised at initialize time.
type ServerCapabilities struct {
	Config            ConfigCapability `json:"config"`
	Views             ViewCapabilities `json:"views"`
	ProtocolArtifacts bool             `json:"protocolArtifacts"`
}

// InitializeParams defines the initialize request payload.
type InitializeParams struct {
	ProtocolVersions []string   `json:"protocolVersions"`
	Client           ClientInfo `json:"client"`
}

// InitializeResult defines the initialize success payload.
type InitializeResult struct {
	ServerName      string             `json:"serverName"`
	ServerVersion   string             `json:"serverVersion"`
	InstanceID      string             `json:"instanceId"`
	InstanceName    string             `json:"instanceName"`
	ProtocolVersion string             `json:"protocolVersion"`
	MethodFamilies  []string           `json:"methodFamilies"`
	Capabilities    ServerCapabilities `json:"capabilities"`
}

// InitializedParams defines the initialized notification payload.
type InitializedParams struct{}

// ServerPingParams defines the server/ping request payload.
type ServerPingParams struct{}

// ServerPingResult defines the server/ping success payload.
type ServerPingResult struct {
	ServerName      string    `json:"serverName"`
	ServerVersion   string    `json:"serverVersion"`
	InstanceID      string    `json:"instanceId"`
	ProtocolVersion string    `json:"protocolVersion"`
	ServerTime      time.Time `json:"serverTime"`
}

// ServerHeartbeatParams defines the server/heartbeat notification payload.
type ServerHeartbeatParams struct {
	InstanceID          string    `json:"instanceId"`
	ServerTime          time.Time `json:"serverTime"`
	HeartbeatIntervalMS int       `json:"heartbeatIntervalMs"`
}

// ProcessMetadataView defines one app-server process metadata payload.
type ProcessMetadataView struct {
	RunID      string    `json:"runId"`
	PID        int       `json:"pid"`
	RecordedAt time.Time `json:"recordedAt"`
	StartedAt  time.Time `json:"startedAt"`
	Source     string    `json:"source"`
}

// StopRequestMetadataView defines one app-server stop-request metadata payload.
type StopRequestMetadataView struct {
	RunID       string    `json:"runId"`
	RequestedAt time.Time `json:"requestedAt"`
	RequestedBy string    `json:"requestedBy"`
	Signal      string    `json:"signal"`
}

// RunSummaryView defines one app-server run summary payload.
type RunSummaryView struct {
	RunID          string     `json:"runId"`
	Name           string     `json:"name"`
	State          string     `json:"state"`
	Source         string     `json:"source,omitempty"`
	QueuedAt       *time.Time `json:"queuedAt,omitempty"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	TerminalAt     *time.Time `json:"terminalAt,omitempty"`
	EventsPath     string     `json:"eventsPath"`
	PIDStatus      string     `json:"pidStatus"`
	StopRequested  bool       `json:"stopRequested"`
	FinalAnswerRef *string    `json:"finalAnswerRef,omitempty"`
	AccountingRef  *string    `json:"accountingRef,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// RunNodeProjectionView defines one app-server run-node projection payload.
type RunNodeProjectionView struct {
	NodeID        string     `json:"nodeId"`
	ParentNodeID  *string    `json:"parentNodeId,omitempty"`
	Depth         int        `json:"depth"`
	Role          string     `json:"role,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	TerminalAt    *time.Time `json:"terminalAt,omitempty"`
	StepCount     int        `json:"stepCount"`
	ResultRef     *string    `json:"resultRef,omitempty"`
	AccountingRef *string    `json:"accountingRef,omitempty"`
	ErrorCode     *string    `json:"errorCode,omitempty"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
}

// RunProjectionView defines one app-server detailed run projection payload.
type RunProjectionView struct {
	RunID             string                   `json:"runId"`
	Name              string                   `json:"name"`
	State             string                   `json:"state"`
	RunDir            string                   `json:"runDir"`
	EventsPath        string                   `json:"eventsPath"`
	Source            string                   `json:"source,omitempty"`
	AppConfigPath     *string                  `json:"appConfigPath,omitempty"`
	RunConfigPath     *string                  `json:"runConfigPath,omitempty"`
	QueuedAt          *time.Time               `json:"queuedAt,omitempty"`
	StartedAt         *time.Time               `json:"startedAt,omitempty"`
	TerminalAt        *time.Time               `json:"terminalAt,omitempty"`
	Executor          string                   `json:"executor,omitempty"`
	MaxDepth          int                      `json:"maxDepth"`
	PIDStatus         string                   `json:"pidStatus"`
	StopRequested     bool                     `json:"stopRequested"`
	ProcessMetadata   *ProcessMetadataView     `json:"processMetadata,omitempty"`
	StopRequest       *StopRequestMetadataView `json:"stopRequest,omitempty"`
	FinalAnswerRef    *string                  `json:"finalAnswerRef,omitempty"`
	AccountingRef     *string                  `json:"accountingRef,omitempty"`
	ErrorCode         *string                  `json:"errorCode,omitempty"`
	ErrorMessage      *string                  `json:"errorMessage,omitempty"`
	FailedNodeID      *string                  `json:"failedNodeId,omitempty"`
	FailedStepID      *string                  `json:"failedStepId,omitempty"`
	InterruptedReason *string                  `json:"interruptedReason,omitempty"`
	InterruptedBy     *string                  `json:"interruptedBy,omitempty"`
	InterruptedNodeID *string                  `json:"interruptedNodeId,omitempty"`
	NodeCount         int                      `json:"nodeCount"`
	StepCount         int                      `json:"stepCount"`
	ActionCount       int                      `json:"actionCount"`
	SubcallCount      int                      `json:"subcallCount"`
	Nodes             []RunNodeProjectionView  `json:"nodes"`
}

// EventEnvelopeView defines one app-server canonical event payload.
type EventEnvelopeView struct {
	EventID       string         `json:"eventId"`
	SchemaVersion string         `json:"schemaVersion"`
	RunID         string         `json:"runId"`
	Seq           int64          `json:"seq"`
	Timestamp     time.Time      `json:"ts"`
	Type          string         `json:"type"`
	NodeID        *string        `json:"nodeId,omitempty"`
	CausationID   string         `json:"causationId"`
	CorrelationID string         `json:"correlationId"`
	Payload       map[string]any `json:"payload"`
}

// RunTreeNodeView defines one app-server node tree payload.
type RunTreeNodeView struct {
	NodeID        string     `json:"nodeId"`
	ParentNodeID  *string    `json:"parentNodeId,omitempty"`
	Depth         int        `json:"depth"`
	Role          string     `json:"role,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	TerminalAt    *time.Time `json:"terminalAt,omitempty"`
	StepCount     int        `json:"stepCount"`
	ResultRef     *string    `json:"resultRef,omitempty"`
	AccountingRef *string    `json:"accountingRef,omitempty"`
	ErrorCode     *string    `json:"errorCode,omitempty"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
	ChildNodeIDs  []string   `json:"childNodeIds,omitempty"`
}

// RunStepSummaryView defines one app-server step timeline payload.
type RunStepSummaryView struct {
	NodeID         string     `json:"nodeId"`
	StepID         string     `json:"stepId"`
	NodeStepIndex  int        `json:"nodeStepIndex"`
	StepStartedSeq int64      `json:"stepStartedSeq"`
	SchemaID       string     `json:"schemaId"`
	Decision       *string    `json:"decision,omitempty"`
	State          string     `json:"state"`
	StartedAt      *time.Time `json:"startedAt,omitempty"`
	CompletedAt    *time.Time `json:"completedAt,omitempty"`
	DurationMS     *int       `json:"durationMs,omitempty"`
	ActionCount    int        `json:"actionCount"`
	SubcallCount   int        `json:"subcallCount"`
	UserTurnRef    *string    `json:"userTurnRef,omitempty"`
	ModelTurnRef   *string    `json:"modelTurnRef,omitempty"`
	AccountingRef  *string    `json:"accountingRef,omitempty"`
	ErrorCode      *string    `json:"errorCode,omitempty"`
	ErrorMessage   *string    `json:"errorMessage,omitempty"`
}

// RunNodeDetailView defines one app-server node detail payload.
type RunNodeDetailView struct {
	NodeID        string     `json:"nodeId"`
	ParentNodeID  *string    `json:"parentNodeId,omitempty"`
	Depth         int        `json:"depth"`
	Role          string     `json:"role,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"startedAt,omitempty"`
	TerminalAt    *time.Time `json:"terminalAt,omitempty"`
	StepCount     int        `json:"stepCount"`
	ResultRef     *string    `json:"resultRef,omitempty"`
	AccountingRef *string    `json:"accountingRef,omitempty"`
	ErrorCode     *string    `json:"errorCode,omitempty"`
	ErrorMessage  *string    `json:"errorMessage,omitempty"`
	StepIDs       []string   `json:"stepIds,omitempty"`
	ActiveStepID  *string    `json:"activeStepId,omitempty"`
	ChildNodeIDs  []string   `json:"childNodeIds,omitempty"`
}

// RunStepDetailView defines one app-server step detail payload.
type RunStepDetailView struct {
	NodeID                string     `json:"nodeId"`
	StepID                string     `json:"stepId"`
	NodeStepIndex         int        `json:"nodeStepIndex"`
	StepStartedSeq        int64      `json:"stepStartedSeq"`
	SchemaID              string     `json:"schemaId"`
	Decision              *string    `json:"decision,omitempty"`
	State                 string     `json:"state"`
	StartedAt             *time.Time `json:"startedAt,omitempty"`
	CompletedAt           *time.Time `json:"completedAt,omitempty"`
	DurationMS            *int       `json:"durationMs,omitempty"`
	ActionCount           int        `json:"actionCount"`
	SubcallCount          int        `json:"subcallCount"`
	UserTurnRef           *string    `json:"userTurnRef,omitempty"`
	ModelTurnRef          *string    `json:"modelTurnRef,omitempty"`
	AccountingRef         *string    `json:"accountingRef,omitempty"`
	ErrorCode             *string    `json:"errorCode,omitempty"`
	ErrorMessage          *string    `json:"errorMessage,omitempty"`
	ActionRefs            []string   `json:"actionRefs,omitempty"`
	SubcallAccountingRefs []string   `json:"subcallAccountingRefs,omitempty"`
}

// ArtifactIdentityView defines sparse identity fields parsed from canonical artifact refs.
type ArtifactIdentityView struct {
	NodeID       *string `json:"nodeId,omitempty"`
	StepID       *string `json:"stepId,omitempty"`
	ActionIndex  *int    `json:"actionIndex,omitempty"`
	SubcallIndex *int    `json:"subcallIndex,omitempty"`
}

// RunsListParams defines the runs/list request payload.
type RunsListParams struct {
	Limit  int     `json:"limit,omitempty"`
	Cursor *string `json:"cursor,omitempty"`
}

// RunsListPayload defines the runs/list response body.
type RunsListPayload struct {
	Items      []RunSummaryView `json:"items"`
	NextCursor *string          `json:"nextCursor,omitempty"`
}

// RunsListResult defines the runs/list success payload.
type RunsListResult struct {
	Payload RunsListPayload `json:"payload"`
}

// RunReadParams defines the run/read request payload.
type RunReadParams struct {
	RunID string `json:"runId"`
}

// RunReadPayload defines the run/read response body.
type RunReadPayload struct {
	Run RunProjectionView `json:"run"`
}

// RunReadResult defines the run/read success payload.
type RunReadResult struct {
	RunID   string         `json:"runId"`
	AsOfSeq int64          `json:"asOfSeq"`
	Payload RunReadPayload `json:"payload"`
}

// RunEventsReadParams defines the run/events/read request payload.
type RunEventsReadParams struct {
	RunID string `json:"runId"`
}

// RunEventsReadPayload defines the run/events/read response body.
type RunEventsReadPayload struct {
	FirstSeq int64               `json:"firstSeq,omitempty"`
	LastSeq  int64               `json:"lastSeq,omitempty"`
	Events   []EventEnvelopeView `json:"events"`
}

// RunEventsReadResult defines the run/events/read success payload.
type RunEventsReadResult struct {
	RunID   string               `json:"runId"`
	Payload RunEventsReadPayload `json:"payload"`
}

// RunTreeReadParams defines the run/tree/read request payload.
type RunTreeReadParams struct {
	RunID string `json:"runId"`
}

// RunTreeReadPayload defines the run/tree/read response body.
type RunTreeReadPayload struct {
	RootNodeID *string           `json:"rootNodeId,omitempty"`
	Nodes      []RunTreeNodeView `json:"nodes"`
}

// RunTreeReadResult defines the run/tree/read success payload.
type RunTreeReadResult struct {
	RunID   string             `json:"runId"`
	AsOfSeq int64              `json:"asOfSeq"`
	Payload RunTreeReadPayload `json:"payload"`
}

// RunStepsListParams defines the run/steps/list request payload.
type RunStepsListParams struct {
	RunID string `json:"runId"`
}

// RunStepsListPayload defines the run/steps/list response body.
type RunStepsListPayload struct {
	Steps []RunStepSummaryView `json:"steps"`
}

// RunStepsListResult defines the run/steps/list success payload.
type RunStepsListResult struct {
	RunID   string              `json:"runId"`
	AsOfSeq int64               `json:"asOfSeq"`
	Payload RunStepsListPayload `json:"payload"`
}

// RunNodeReadParams defines the run/node/read request payload.
type RunNodeReadParams struct {
	RunID  string `json:"runId"`
	NodeID string `json:"nodeId"`
}

// RunNodeReadPayload defines the run/node/read response body.
type RunNodeReadPayload struct {
	Node RunNodeDetailView `json:"node"`
}

// RunNodeReadResult defines the run/node/read success payload.
type RunNodeReadResult struct {
	RunID   string             `json:"runId"`
	AsOfSeq int64              `json:"asOfSeq"`
	Payload RunNodeReadPayload `json:"payload"`
}

// RunStepReadParams defines the run/step/read request payload.
type RunStepReadParams struct {
	RunID  string `json:"runId"`
	NodeID string `json:"nodeId"`
	StepID string `json:"stepId"`
}

// RunStepReadPayload defines the run/step/read response body.
type RunStepReadPayload struct {
	Step RunStepDetailView `json:"step"`
}

// RunStepReadResult defines the run/step/read success payload.
type RunStepReadResult struct {
	RunID   string             `json:"runId"`
	AsOfSeq int64              `json:"asOfSeq"`
	Payload RunStepReadPayload `json:"payload"`
}

// RunArtifactReadParams defines the run/artifact/read request payload.
type RunArtifactReadParams struct {
	RunID       string `json:"runId"`
	ArtifactRef string `json:"artifactRef"`
}

// RunArtifactReadPayload defines the run/artifact/read response body.
type RunArtifactReadPayload struct {
	ArtifactRef  string               `json:"artifactRef"`
	ArtifactKind string               `json:"artifactKind"`
	Identity     ArtifactIdentityView `json:"identity"`
	Artifact     map[string]any       `json:"artifact"`
}

// RunArtifactReadResult defines the run/artifact/read success payload.
type RunArtifactReadResult struct {
	RunID   string                 `json:"runId"`
	Payload RunArtifactReadPayload `json:"payload"`
}

// RunStartParams defines the run/start request payload.
type RunStartParams struct {
	RunConfigYAML string            `json:"runConfigYaml"`
	TemplateVars  map[string]string `json:"templateVars,omitempty"`
}

// RunStartPayload defines the run/start response body.
type RunStartPayload struct {
	Run RunProjectionView `json:"run"`
}

// RunStartResult defines the run/start success payload.
type RunStartResult struct {
	RunID   string          `json:"runId"`
	AsOfSeq int64           `json:"asOfSeq"`
	Payload RunStartPayload `json:"payload"`
}

// RunSubscriptionSnapshotView defines one fresh-attach subscription snapshot payload.
type RunSubscriptionSnapshotView struct {
	Run          RunProjectionView  `json:"run"`
	Tree         RunTreeReadPayload `json:"tree"`
	ActiveNodeID *string            `json:"activeNodeId,omitempty"`
	ActiveStepID *string            `json:"activeStepId,omitempty"`
	Terminal     bool               `json:"terminal"`
}

// RunSubscribeParams defines the run/subscribe request payload.
type RunSubscribeParams struct {
	RunID    string `json:"runId"`
	AfterSeq *int64 `json:"afterSeq,omitempty"`
}

// RunSubscribePayload defines the run/subscribe response body.
type RunSubscribePayload struct {
	Snapshot     *RunSubscriptionSnapshotView `json:"snapshot,omitempty"`
	ReplayEvents []EventEnvelopeView          `json:"replayEvents,omitempty"`
	Terminal     bool                         `json:"terminal"`
}

// RunSubscribeResult defines the run/subscribe success payload.
type RunSubscribeResult struct {
	RunID           string              `json:"runId"`
	SnapshotAsOfSeq int64               `json:"snapshotAsOfSeq"`
	Payload         RunSubscribePayload `json:"payload"`
}

// RunsSubscribeParams defines the runs/subscribe request payload.
type RunsSubscribeParams struct{}

// RunsSubscribePayload defines the runs/subscribe response body.
type RunsSubscribePayload struct {
	Items    []RunSummaryView `json:"items"`
	Revision int64            `json:"revision"`
}

// RunsSubscribeResult defines the runs/subscribe success payload.
type RunsSubscribeResult struct {
	Payload RunsSubscribePayload `json:"payload"`
}

// RunUnsubscribeParams defines the run/unsubscribe request payload.
type RunUnsubscribeParams struct {
	RunID string `json:"runId"`
}

// RunUnsubscribePayload defines the run/unsubscribe response body.
type RunUnsubscribePayload struct {
	Unsubscribed bool `json:"unsubscribed"`
}

// RunUnsubscribeResult defines the run/unsubscribe success payload.
type RunUnsubscribeResult struct {
	RunID   string                `json:"runId"`
	Payload RunUnsubscribePayload `json:"payload"`
}

// RunsUnsubscribeParams defines the runs/unsubscribe request payload.
type RunsUnsubscribeParams struct{}

// RunsUnsubscribePayload defines the runs/unsubscribe response body.
type RunsUnsubscribePayload struct {
	Unsubscribed bool `json:"unsubscribed"`
}

// RunsUnsubscribeResult defines the runs/unsubscribe success payload.
type RunsUnsubscribeResult struct {
	Payload RunsUnsubscribePayload `json:"payload"`
}

// RunStopParams defines the run/stop request payload.
type RunStopParams struct {
	RunID string `json:"runId"`
}

// RunStopPayload defines the run/stop response body.
type RunStopPayload struct {
	StopRequested bool   `json:"stopRequested"`
	State         string `json:"state"`
	EventsPath    string `json:"eventsPath"`
}

// RunStopResult defines the run/stop success payload.
type RunStopResult struct {
	RunID   string         `json:"runId"`
	Payload RunStopPayload `json:"payload"`
}

// RunStartedPayload defines one run/started notification body.
type RunStartedPayload struct {
	Run RunProjectionView `json:"run"`
}

// RunStartedParams defines the run/started notification payload.
type RunStartedParams struct {
	RunID   string            `json:"runId"`
	Seq     int64             `json:"seq"`
	Payload RunStartedPayload `json:"payload"`
}

// RunEventAppendedPayload defines one run/eventAppended notification body.
type RunEventAppendedPayload struct {
	Event EventEnvelopeView `json:"event"`
}

// RunEventAppendedParams defines the run/eventAppended notification payload.
type RunEventAppendedParams struct {
	RunID   string                  `json:"runId"`
	Seq     int64                   `json:"seq"`
	Payload RunEventAppendedPayload `json:"payload"`
}

// RunStatusChangedPayload defines one run/statusChanged notification body.
type RunStatusChangedPayload struct {
	State    string `json:"state"`
	Terminal bool   `json:"terminal"`
}

// RunStatusChangedParams defines the run/statusChanged notification payload.
type RunStatusChangedParams struct {
	RunID   string                  `json:"runId"`
	Seq     int64                   `json:"seq"`
	Payload RunStatusChangedPayload `json:"payload"`
}

// RunCompletedPayload defines one run/completed notification body.
type RunCompletedPayload struct {
	State    string `json:"state"`
	Terminal bool   `json:"terminal"`
}

// RunCompletedParams defines the run/completed notification payload.
type RunCompletedParams struct {
	RunID   string              `json:"runId"`
	Seq     int64               `json:"seq"`
	Payload RunCompletedPayload `json:"payload"`
}

// RunsChangedKind defines the payload kind for one runs/changed notification.
type RunsChangedKind string

const (
	// RunsChangedKindUpsert carries one changed or newly observed run summary.
	RunsChangedKindUpsert RunsChangedKind = "upsert"
	// RunsChangedKindRemove removes one run summary from the collection view.
	RunsChangedKindRemove RunsChangedKind = "remove"
	// RunsChangedKindReset indicates the client should fetch a fresh authoritative snapshot.
	RunsChangedKindReset RunsChangedKind = "reset"
)

// RunsChangedPayload defines one runs/changed notification body.
type RunsChangedPayload struct {
	Kind  RunsChangedKind `json:"kind"`
	Run   *RunSummaryView `json:"run,omitempty"`
	RunID *string         `json:"runId,omitempty"`
}

// RunsChangedParams defines the runs/changed notification payload.
type RunsChangedParams struct {
	Revision int64              `json:"revision"`
	Payload  RunsChangedPayload `json:"payload"`
}

// ErrorData carries machine-actionable Sigil domain error data.
type ErrorData struct {
	Code      string                 `json:"code"`
	Retryable bool                   `json:"retryable,omitempty"`
	Details   map[string]interface{} `json:"details,omitempty"`
}

// ErrorObject is the JSON-RPC error payload.
type ErrorObject struct {
	Code    int        `json:"code"`
	Message string     `json:"message"`
	Data    *ErrorData `json:"data,omitempty"`
}

// MethodDefinition describes one protocol method for generation and dispatch.
type MethodDefinition struct {
	Name   string
	Kind   MethodKind
	Params interface{}
	Result interface{}
}

// SupportedProtocolVersions returns the server-supported protocol versions.
func SupportedProtocolVersions() []string {
	return []string{VersionV1Alpha1}
}

// DefaultProtocolVersion returns the default negotiated protocol version.
func DefaultProtocolVersion() string {
	return VersionV1Alpha1
}

// Definitions returns the current typed protocol method set.
func Definitions() []MethodDefinition {
	return []MethodDefinition{
		{
			Name:   "initialize",
			Kind:   MethodKindRequest,
			Params: InitializeParams{},
			Result: InitializeResult{},
		},
		{
			Name:   "initialized",
			Kind:   MethodKindNotification,
			Params: InitializedParams{},
		},
		{
			Name:   "run/artifact/read",
			Kind:   MethodKindRequest,
			Params: RunArtifactReadParams{},
			Result: RunArtifactReadResult{},
		},
		{
			Name:   "run/events/read",
			Kind:   MethodKindRequest,
			Params: RunEventsReadParams{},
			Result: RunEventsReadResult{},
		},
		{
			Name:   "runs/list",
			Kind:   MethodKindRequest,
			Params: RunsListParams{},
			Result: RunsListResult{},
		},
		{
			Name:   "run/node/read",
			Kind:   MethodKindRequest,
			Params: RunNodeReadParams{},
			Result: RunNodeReadResult{},
		},
		{
			Name:   "run/read",
			Kind:   MethodKindRequest,
			Params: RunReadParams{},
			Result: RunReadResult{},
		},
		{
			Name:   "run/start",
			Kind:   MethodKindRequest,
			Params: RunStartParams{},
			Result: RunStartResult{},
		},
		{
			Name:   "run/step/read",
			Kind:   MethodKindRequest,
			Params: RunStepReadParams{},
			Result: RunStepReadResult{},
		},
		{
			Name:   "run/steps/list",
			Kind:   MethodKindRequest,
			Params: RunStepsListParams{},
			Result: RunStepsListResult{},
		},
		{
			Name:   "run/tree/read",
			Kind:   MethodKindRequest,
			Params: RunTreeReadParams{},
			Result: RunTreeReadResult{},
		},
		{
			Name:   "run/subscribe",
			Kind:   MethodKindRequest,
			Params: RunSubscribeParams{},
			Result: RunSubscribeResult{},
		},
		{
			Name:   "run/stop",
			Kind:   MethodKindRequest,
			Params: RunStopParams{},
			Result: RunStopResult{},
		},
		{
			Name:   "run/unsubscribe",
			Kind:   MethodKindRequest,
			Params: RunUnsubscribeParams{},
			Result: RunUnsubscribeResult{},
		},
		{
			Name:   "runs/subscribe",
			Kind:   MethodKindRequest,
			Params: RunsSubscribeParams{},
			Result: RunsSubscribeResult{},
		},
		{
			Name:   "runs/unsubscribe",
			Kind:   MethodKindRequest,
			Params: RunsUnsubscribeParams{},
			Result: RunsUnsubscribeResult{},
		},
		{
			Name:   "run/completed",
			Kind:   MethodKindNotification,
			Params: RunCompletedParams{},
		},
		{
			Name:   "run/eventAppended",
			Kind:   MethodKindNotification,
			Params: RunEventAppendedParams{},
		},
		{
			Name:   "run/started",
			Kind:   MethodKindNotification,
			Params: RunStartedParams{},
		},
		{
			Name:   "run/statusChanged",
			Kind:   MethodKindNotification,
			Params: RunStatusChangedParams{},
		},
		{
			Name:   "runs/changed",
			Kind:   MethodKindNotification,
			Params: RunsChangedParams{},
		},
		{
			Name:   "server/heartbeat",
			Kind:   MethodKindNotification,
			Params: ServerHeartbeatParams{},
		},
		{
			Name:   "server/ping",
			Kind:   MethodKindRequest,
			Params: ServerPingParams{},
			Result: ServerPingResult{},
		},
	}
}

// NewError constructs one JSON-RPC error object with stable Sigil domain data.
func NewError(code int, message string, dataCode string, retryable bool, details map[string]interface{}) *ErrorObject {
	errData := &ErrorData{
		Code:      dataCode,
		Retryable: retryable,
	}
	if len(details) > 0 {
		errData.Details = details
	}
	return &ErrorObject{
		Code:    code,
		Message: message,
		Data:    errData,
	}
}
