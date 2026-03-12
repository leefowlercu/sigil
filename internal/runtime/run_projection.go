package runtime

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"
)

const (
	// RunStateUnknown is returned by list projections when a run cannot be decoded.
	RunStateUnknown = "unknown"

	// RunPIDStatusCurrent indicates the recorded process metadata still matches a live process.
	RunPIDStatusCurrent = "current"
	// RunPIDStatusMissing indicates there is no process metadata file for the run.
	RunPIDStatusMissing = "missing"
	// RunPIDStatusNotRunning indicates process metadata exists but the original process is no longer alive.
	RunPIDStatusNotRunning = "not_running"
	// RunPIDStatusStale indicates recorded process metadata points to a reused or mismatched PID.
	RunPIDStatusStale = "stale"
)

// RunSummary provides one compact operator-facing run summary.
type RunSummary struct {
	RunID          string     `json:"run_id"`
	State          string     `json:"state"`
	Source         string     `json:"source,omitempty"`
	QueuedAt       *time.Time `json:"queued_at,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	TerminalAt     *time.Time `json:"terminal_at,omitempty"`
	EventsPath     string     `json:"events_path"`
	PIDStatus      string     `json:"pid_status"`
	StopRequested  bool       `json:"stop_requested"`
	FinalAnswerRef *string    `json:"final_answer_ref,omitempty"`
	AccountingRef  *string    `json:"accounting_ref,omitempty"`
	Error          string     `json:"error,omitempty"`
}

// RunNodeProjection provides one derived node summary for inspect surfaces.
type RunNodeProjection struct {
	NodeID        string     `json:"node_id"`
	ParentNodeID  *string    `json:"parent_node_id,omitempty"`
	Depth         int        `json:"depth"`
	Role          string     `json:"role,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	TerminalAt    *time.Time `json:"terminal_at,omitempty"`
	StepCount     int        `json:"step_count"`
	OutputRef     *string    `json:"output_ref,omitempty"`
	AccountingRef *string    `json:"accounting_ref,omitempty"`
	ErrorCode     *string    `json:"error_code,omitempty"`
	ErrorMessage  *string    `json:"error_message,omitempty"`
}

// RunProjection provides a detailed derived run inspection payload.
type RunProjection struct {
	RunID             string               `json:"run_id"`
	State             string               `json:"state"`
	RunDir            string               `json:"run_dir"`
	EventsPath        string               `json:"events_path"`
	Source            string               `json:"source,omitempty"`
	AppConfigPath     *string              `json:"app_config_path,omitempty"`
	RunConfigPath     *string              `json:"run_config_path,omitempty"`
	QueuedAt          *time.Time           `json:"queued_at,omitempty"`
	StartedAt         *time.Time           `json:"started_at,omitempty"`
	TerminalAt        *time.Time           `json:"terminal_at,omitempty"`
	Executor          string               `json:"executor,omitempty"`
	MaxDepth          int                  `json:"max_depth"`
	PIDStatus         string               `json:"pid_status"`
	StopRequested     bool                 `json:"stop_requested"`
	ProcessMetadata   *ProcessMetadata     `json:"process_metadata,omitempty"`
	StopRequest       *StopRequestMetadata `json:"stop_request,omitempty"`
	FinalAnswerRef    *string              `json:"final_answer_ref,omitempty"`
	AccountingRef     *string              `json:"accounting_ref,omitempty"`
	ErrorCode         *string              `json:"error_code,omitempty"`
	ErrorMessage      *string              `json:"error_message,omitempty"`
	FailedNodeID      *string              `json:"failed_node_id,omitempty"`
	FailedStepID      *string              `json:"failed_step_id,omitempty"`
	InterruptedReason *string              `json:"interrupted_reason,omitempty"`
	InterruptedBy     *string              `json:"interrupted_by,omitempty"`
	InterruptedNodeID *string              `json:"interrupted_node_id,omitempty"`
	NodeCount         int                  `json:"node_count"`
	StepCount         int                  `json:"step_count"`
	ActionCount       int                  `json:"action_count"`
	SubcallCount      int                  `json:"subcall_count"`
	Nodes             []RunNodeProjection  `json:"nodes"`
}

// ReadRunEvents validates and returns persisted canonical run events for one run.
func ReadRunEvents(baseDir string, runID string) ([]EventEnvelope, error) {
	if err := validateUUIDv7String(runID); err != nil {
		return nil, fmt.Errorf("run-id must be UUIDv7; %w", err)
	}

	resolvedBaseDir, err := ResolveRunsBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	eventsPath := filepath.Join(resolvedBaseDir, runID, eventsFileName)
	events, _, err := parsePersistedEventsLocked(eventsPath, runID)
	if err != nil {
		return nil, err
	}

	return cloneEventEnvelopes(events), nil
}

// LoadRunProjection returns one derived run projection for status and inspect surfaces.
func LoadRunProjection(baseDir string, runID string) (RunProjection, error) {
	if err := validateUUIDv7String(runID); err != nil {
		return RunProjection{}, fmt.Errorf("run-id must be UUIDv7; %w", err)
	}

	resolvedBaseDir, err := ResolveRunsBaseDir(baseDir)
	if err != nil {
		return RunProjection{}, err
	}

	events, err := ReadRunEvents(resolvedBaseDir, runID)
	if err != nil {
		return RunProjection{}, err
	}

	projection := RunProjection{
		RunID:      runID,
		State:      string(deriveRunState(events)),
		RunDir:     filepath.Join(resolvedBaseDir, runID),
		EventsPath: filepath.Join(resolvedBaseDir, runID, eventsFileName),
		PIDStatus:  RunPIDStatusMissing,
	}

	pidStatus, processMetadata, err := loadProcessMetadataStatus(resolvedBaseDir, runID)
	if err != nil {
		return RunProjection{}, err
	}
	projection.PIDStatus = pidStatus
	projection.ProcessMetadata = processMetadata

	stopRequest, ok, err := loadStopRequestMetadata(resolvedBaseDir, runID)
	if err != nil {
		return RunProjection{}, err
	}
	if ok {
		projection.StopRequested = true
		projection.StopRequest = stopRequest
	}

	nodeByID := make(map[string]*RunNodeProjection)
	for _, event := range events {
		switch payload := event.Payload.(type) {
		case RunQueuedPayload:
			projection.Source = string(payload.Source)
			projection.AppConfigPath = cloneStringPointer(payload.AppConfigPath)
			projection.RunConfigPath = cloneStringPointer(payload.RunConfigPath)
			projection.QueuedAt = timePointer(event.Timestamp)
		case RunRunningPayload:
			projection.StartedAt = timePointer(event.Timestamp)
			projection.Executor = payload.Executor
			projection.MaxDepth = payload.MaxDepth
		case NodeStartedPayload:
			if event.NodeID == nil {
				continue
			}
			node := ensureProjectedNode(nodeByID, *event.NodeID)
			node.NodeID = *event.NodeID
			node.Depth = payload.Depth
			node.ParentNodeID = cloneStringPointer(payload.ParentNodeID)
			node.Role = string(payload.Role)
			node.State = string(RunStateRunning)
			node.StartedAt = timePointer(event.Timestamp)
		case NodeStepStartedPayload:
			if event.NodeID == nil {
				continue
			}
			node := ensureProjectedNode(nodeByID, *event.NodeID)
			node.StepCount++
			projection.StepCount++
		case NodeActionExecutedPayload:
			projection.ActionCount++
		case NodeSubcallExecutedPayload:
			projection.SubcallCount++
		case NodeCompletedPayload:
			if event.NodeID == nil {
				continue
			}
			node := ensureProjectedNode(nodeByID, *event.NodeID)
			node.State = payload.Status
			node.TerminalAt = timePointer(event.Timestamp)
			node.OutputRef = cloneStringPointer(payload.OutputRef)
			node.AccountingRef = cloneStringPointer(payload.AccountingRef)
		case NodeFailedPayload:
			if event.NodeID == nil {
				continue
			}
			node := ensureProjectedNode(nodeByID, *event.NodeID)
			node.State = payload.Status
			node.TerminalAt = timePointer(event.Timestamp)
			node.ErrorCode = cloneStringPointer(&payload.ErrorCode)
			node.ErrorMessage = cloneStringPointer(&payload.ErrorMessage)
		case RunCompletedPayload:
			projection.TerminalAt = timePointer(event.Timestamp)
			projection.FinalAnswerRef = cloneStringPointer(payload.FinalAnswerRef)
			projection.AccountingRef = cloneStringPointer(payload.AccountingRef)
		case RunFailedPayload:
			projection.TerminalAt = timePointer(event.Timestamp)
			projection.AccountingRef = cloneStringPointer(payload.AccountingRef)
			projection.ErrorCode = cloneStringPointer(&payload.ErrorCode)
			projection.ErrorMessage = cloneStringPointer(&payload.ErrorMessage)
			projection.FailedNodeID = cloneStringPointer(payload.FailedNodeID)
			projection.FailedStepID = cloneStringPointer(payload.FailedStepID)
		case RunInterruptedPayload:
			projection.TerminalAt = timePointer(event.Timestamp)
			projection.AccountingRef = cloneStringPointer(payload.AccountingRef)
			reason := string(payload.Reason)
			projection.InterruptedReason = &reason
			projection.InterruptedBy = cloneStringPointer(payload.InterruptedBy)
			projection.InterruptedNodeID = cloneStringPointer(payload.InterruptedNodeID)
			if payload.InterruptedNodeID != nil {
				node := ensureProjectedNode(nodeByID, *payload.InterruptedNodeID)
				node.State = string(RunStateInterrupted)
				node.TerminalAt = timePointer(event.Timestamp)
			}
		}
	}

	projection.Nodes = flattenProjectedNodes(nodeByID)
	projection.NodeCount = len(projection.Nodes)
	return projection, nil
}

// LoadRunSummary returns one compact derived run summary.
func LoadRunSummary(baseDir string, runID string) (RunSummary, error) {
	projection, err := LoadRunProjection(baseDir, runID)
	if err != nil {
		return RunSummary{}, err
	}

	return summarizeProjection(projection), nil
}

// ListRuns returns best-effort summaries for runs under one base directory.
func ListRuns(baseDir string) ([]RunSummary, error) {
	resolvedBaseDir, err := ResolveRunsBaseDir(baseDir)
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(resolvedBaseDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return []RunSummary{}, nil
		}
		return nil, err
	}

	summaries := make([]RunSummary, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}

		runID := entry.Name()
		if err := validateUUIDv7String(runID); err != nil {
			continue
		}

		projection, err := LoadRunProjection(resolvedBaseDir, runID)
		if err != nil {
			summaries = append(summaries, RunSummary{
				RunID:      runID,
				State:      RunStateUnknown,
				EventsPath: filepath.Join(resolvedBaseDir, runID, eventsFileName),
				PIDStatus:  RunPIDStatusMissing,
				Error:      err.Error(),
			})
			continue
		}

		summaries = append(summaries, summarizeProjection(projection))
	}

	sort.SliceStable(summaries, func(i int, j int) bool {
		left := summarySortTime(summaries[i])
		right := summarySortTime(summaries[j])
		if left.Equal(right) {
			return summaries[i].RunID > summaries[j].RunID
		}
		return left.After(right)
	})

	return summaries, nil
}

func summarizeProjection(projection RunProjection) RunSummary {
	return RunSummary{
		RunID:          projection.RunID,
		State:          projection.State,
		Source:         projection.Source,
		QueuedAt:       cloneTimePointer(projection.QueuedAt),
		StartedAt:      cloneTimePointer(projection.StartedAt),
		TerminalAt:     cloneTimePointer(projection.TerminalAt),
		EventsPath:     projection.EventsPath,
		PIDStatus:      projection.PIDStatus,
		StopRequested:  projection.StopRequested,
		FinalAnswerRef: cloneStringPointer(projection.FinalAnswerRef),
		AccountingRef:  cloneStringPointer(projection.AccountingRef),
	}
}

func summarySortTime(summary RunSummary) time.Time {
	switch {
	case summary.QueuedAt != nil:
		return summary.QueuedAt.UTC()
	case summary.StartedAt != nil:
		return summary.StartedAt.UTC()
	case summary.TerminalAt != nil:
		return summary.TerminalAt.UTC()
	default:
		return time.Time{}
	}
}

func loadProcessMetadataStatus(baseDir string, runID string) (string, *ProcessMetadata, error) {
	path, err := ProcessMetadataPath(baseDir, runID)
	if err != nil {
		return "", nil, err
	}
	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return RunPIDStatusMissing, nil, nil
	}

	var metadata ProcessMetadata
	if err := decodeStrictJSONObject(raw, &metadata); err != nil {
		return RunPIDStatusMissing, nil, nil
	}
	if err := validateProcessMetadata(metadata); err != nil {
		return RunPIDStatusMissing, nil, nil
	}
	if metadata.RunID != runID {
		return RunPIDStatusMissing, nil, nil
	}

	switch err := ValidateLiveProcessMetadata(metadata); {
	case err == nil:
		return RunPIDStatusCurrent, &metadata, nil
	case errors.Is(err, ErrProcessNotRunning):
		return RunPIDStatusNotRunning, &metadata, nil
	case errors.Is(err, ErrStaleProcessMetadata):
		return RunPIDStatusStale, &metadata, nil
	default:
		return RunPIDStatusMissing, nil, nil
	}
}

func loadStopRequestMetadata(baseDir string, runID string) (*StopRequestMetadata, bool, error) {
	path, err := StopRequestPath(baseDir, runID)
	if err != nil {
		return nil, false, err
	}

	raw, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, false, nil
	}

	var metadata StopRequestMetadata
	if err := decodeStrictJSONObject(raw, &metadata); err != nil {
		return nil, false, nil
	}
	if err := validateStopRequestMetadata(metadata); err != nil {
		return nil, false, nil
	}
	if metadata.RunID != runID {
		return nil, false, nil
	}

	return &metadata, true, nil
}

func ensureProjectedNode(nodeByID map[string]*RunNodeProjection, nodeID string) *RunNodeProjection {
	node, ok := nodeByID[nodeID]
	if ok {
		return node
	}

	node = &RunNodeProjection{
		NodeID: nodeID,
		State:  string(RunStateQueued),
	}
	nodeByID[nodeID] = node
	return node
}

func flattenProjectedNodes(nodeByID map[string]*RunNodeProjection) []RunNodeProjection {
	projected := make([]RunNodeProjection, 0, len(nodeByID))
	for _, node := range nodeByID {
		projected = append(projected, RunNodeProjection{
			NodeID:        node.NodeID,
			ParentNodeID:  cloneStringPointer(node.ParentNodeID),
			Depth:         node.Depth,
			Role:          node.Role,
			State:         node.State,
			StartedAt:     cloneTimePointer(node.StartedAt),
			TerminalAt:    cloneTimePointer(node.TerminalAt),
			StepCount:     node.StepCount,
			OutputRef:     cloneStringPointer(node.OutputRef),
			AccountingRef: cloneStringPointer(node.AccountingRef),
			ErrorCode:     cloneStringPointer(node.ErrorCode),
			ErrorMessage:  cloneStringPointer(node.ErrorMessage),
		})
	}

	sort.SliceStable(projected, func(i int, j int) bool {
		if projected[i].Depth == projected[j].Depth {
			return projected[i].NodeID < projected[j].NodeID
		}
		return projected[i].Depth < projected[j].Depth
	})

	return projected
}

func timePointer(value time.Time) *time.Time {
	copyValue := value.UTC()
	return &copyValue
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}

	copyValue := value.UTC()
	return &copyValue
}
