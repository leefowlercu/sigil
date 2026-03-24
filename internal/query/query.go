package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const defaultListRunsPageLimit = 50

var (
	// ErrInvalidCursor indicates the supplied paging cursor does not match the current list order.
	ErrInvalidCursor = errors.New("invalid cursor")
	// ErrInvalidListLimit indicates the supplied paging limit is out of range.
	ErrInvalidListLimit = errors.New("invalid list limit")
	// ErrInvalidIdentifier indicates one or more run-scoped identifiers are malformed.
	ErrInvalidIdentifier = errors.New("invalid identifier")
	// ErrRunNotFound indicates the requested run does not exist in the selected corpus.
	ErrRunNotFound = errors.New("run not found")
	// ErrNodeNotFound indicates the requested node is not present in the run projection.
	ErrNodeNotFound = errors.New("node not found")
	// ErrStepNotFound indicates the requested step is not present in the run history.
	ErrStepNotFound = errors.New("step not found")
	// ErrArtifactNotFound indicates the requested artifact ref does not resolve to a persisted file.
	ErrArtifactNotFound = errors.New("artifact not found")
	// ErrInvalidResumeCursor indicates the supplied resume cursor is not valid for the run history.
	ErrInvalidResumeCursor = errors.New("invalid resume cursor")

	loadRunProjectionFunc         = runtime.LoadRunProjection
	readRunEventsFunc             = runtime.ReadRunEvents
	deriveRunProjectionFromEvents = runtime.DeriveRunProjectionFromEvents
)

// ListRunsRequest defines one shared list-runs query request.
type ListRunsRequest struct {
	RunsBaseDir string
}

// ListRunsPageRequest defines one paged run-list query request.
type ListRunsPageRequest struct {
	RunsBaseDir string
	Limit       int
	Cursor      *string
}

// PaginateRunSummariesRequest defines one in-memory run-summary paging request.
type PaginateRunSummariesRequest struct {
	Summaries []runtime.RunSummary
	Limit     int
	Cursor    *string
}

// ListRunsPageResult defines one paged run-list query response.
type ListRunsPageResult struct {
	Items      []runtime.RunSummary `json:"items"`
	NextCursor *string              `json:"next_cursor,omitempty"`
}

// ReadRunSummaryRequest defines one shared run-summary query request.
type ReadRunSummaryRequest struct {
	RunsBaseDir string
	RunID       string
}

// ReadRunRequest defines one shared run-projection query request.
type ReadRunRequest struct {
	RunsBaseDir string
	RunID       string
}

// ReadRunEventsRequest defines one shared run-events query request.
type ReadRunEventsRequest struct {
	RunsBaseDir string
	RunID       string
}

// ReadRunTreeRequest defines one derived run-tree query request.
type ReadRunTreeRequest struct {
	RunsBaseDir string
	RunID       string
}

// RunTreeNode defines one derived node-tree item.
type RunTreeNode struct {
	NodeID        string     `json:"node_id"`
	ParentNodeID  *string    `json:"parent_node_id,omitempty"`
	Depth         int        `json:"depth"`
	Role          string     `json:"role,omitempty"`
	State         string     `json:"state"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	TerminalAt    *time.Time `json:"terminal_at,omitempty"`
	StepCount     int        `json:"step_count"`
	ResultRef     *string    `json:"result_ref,omitempty"`
	AccountingRef *string    `json:"accounting_ref,omitempty"`
	ErrorCode     *string    `json:"error_code,omitempty"`
	ErrorMessage  *string    `json:"error_message,omitempty"`
	ChildNodeIDs  []string   `json:"child_node_ids,omitempty"`
}

// RunTreeResult defines one derived run-tree response.
type RunTreeResult struct {
	RunID      string        `json:"run_id"`
	AsOfSeq    int64         `json:"as_of_seq"`
	RootNodeID *string       `json:"root_node_id,omitempty"`
	Nodes      []RunTreeNode `json:"nodes"`
}

// ListRunStepsRequest defines one derived step-timeline query request.
type ListRunStepsRequest struct {
	RunsBaseDir string
	RunID       string
}

// RunStepSummary defines one derived step timeline item.
type RunStepSummary struct {
	StepID         string     `json:"step_id"`
	NodeID         string     `json:"node_id"`
	NodeStepIndex  int        `json:"node_step_index"`
	StepStartedSeq int64      `json:"step_started_seq"`
	SchemaID       string     `json:"schema_id"`
	Decision       *string    `json:"decision,omitempty"`
	State          string     `json:"state"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	CompletedAt    *time.Time `json:"completed_at,omitempty"`
	DurationMS     *int       `json:"duration_ms,omitempty"`
	ActionCount    int        `json:"action_count"`
	SubcallCount   int        `json:"subcall_count"`
	UserTurnRef    *string    `json:"user_turn_ref,omitempty"`
	ModelTurnRef   *string    `json:"model_turn_ref,omitempty"`
	AccountingRef  *string    `json:"accounting_ref,omitempty"`
	ErrorCode      *string    `json:"error_code,omitempty"`
	ErrorMessage   *string    `json:"error_message,omitempty"`
}

// ListRunStepsResult defines one derived step-list response.
type ListRunStepsResult struct {
	RunID   string           `json:"run_id"`
	AsOfSeq int64            `json:"as_of_seq"`
	Steps   []RunStepSummary `json:"steps"`
}

// ReadRunNodeRequest defines one derived node-read query request.
type ReadRunNodeRequest struct {
	RunsBaseDir string
	RunID       string
	NodeID      string
}

// RunNodeDetail defines one derived node-read payload.
type RunNodeDetail struct {
	RunTreeNode
	StepIDs      []string `json:"step_ids,omitempty"`
	ActiveStepID *string  `json:"active_step_id,omitempty"`
}

// ReadRunNodeResult defines one derived node-read response.
type ReadRunNodeResult struct {
	RunID   string        `json:"run_id"`
	AsOfSeq int64         `json:"as_of_seq"`
	Node    RunNodeDetail `json:"node"`
}

// ReadRunStepRequest defines one derived step-read query request.
type ReadRunStepRequest struct {
	RunsBaseDir string
	RunID       string
	NodeID      string
	StepID      string
}

// RunStepDetail defines one derived step detail payload.
type RunStepDetail struct {
	RunStepSummary
	ActionRefs            []string `json:"action_refs,omitempty"`
	SubcallAccountingRefs []string `json:"subcall_accounting_refs,omitempty"`
}

// ReadRunStepResult defines one derived step-read response.
type ReadRunStepResult struct {
	RunID   string        `json:"run_id"`
	AsOfSeq int64         `json:"as_of_seq"`
	Step    RunStepDetail `json:"step"`
}

// ReadRunArtifactRequest defines one persisted artifact-read query request.
type ReadRunArtifactRequest struct {
	RunsBaseDir string
	RunID       string
	ArtifactRef string
}

// ArtifactIdentity defines sparse identity fields parsed from canonical refs.
type ArtifactIdentity struct {
	NodeID       *string `json:"node_id,omitempty"`
	StepID       *string `json:"step_id,omitempty"`
	ActionIndex  *int    `json:"action_index,omitempty"`
	SubcallIndex *int    `json:"subcall_index,omitempty"`
}

// ReadRunArtifactResult defines one persisted artifact-read response.
type ReadRunArtifactResult struct {
	RunID        string           `json:"run_id"`
	ArtifactRef  string           `json:"artifact_ref"`
	ArtifactKind string           `json:"artifact_kind"`
	Identity     ArtifactIdentity `json:"identity"`
	Artifact     map[string]any   `json:"artifact"`
}

// BuildRunSubscriptionSnapshotRequest defines one fresh-attach snapshot request.
type BuildRunSubscriptionSnapshotRequest struct {
	RunsBaseDir string
	RunID       string
}

// RunSubscriptionSnapshot defines one fresh-attach subscription snapshot.
type RunSubscriptionSnapshot struct {
	RunID           string                `json:"run_id"`
	SnapshotAsOfSeq int64                 `json:"snapshot_as_of_seq"`
	Run             runtime.RunProjection `json:"run"`
	Tree            RunTreeResult         `json:"tree"`
	ActiveNodeID    *string               `json:"active_node_id,omitempty"`
	ActiveStepID    *string               `json:"active_step_id,omitempty"`
	Terminal        bool                  `json:"terminal"`
}

// ReplayRunEventsRequest defines one persisted-event resume request.
type ReplayRunEventsRequest struct {
	RunsBaseDir string
	RunID       string
	AfterSeq    int64
}

// ReplayRunEventsResult defines one replay window response.
type ReplayRunEventsResult struct {
	RunID    string                  `json:"run_id"`
	AsOfSeq  int64                   `json:"as_of_seq"`
	Terminal bool                    `json:"terminal"`
	Events   []runtime.EventEnvelope `json:"events"`
}

type stepAccumulator struct {
	summary               RunStepSummary
	actionRefs            []string
	subcallAccountingRefs []string
}

type nodeAccumulator struct {
	node         RunTreeNode
	startedSeq   int64
	stepIDs      []string
	activeStepID *string
}

type derivedRunData struct {
	projection runtime.RunProjection
	events     []runtime.EventEnvelope
	asOfSeq    int64
	tree       RunTreeResult
	nodesByID  map[string]*nodeAccumulator
	stepOrder  []string
	stepsByID  map[string]*stepAccumulator
}

// ListRuns loads run summaries for one runs corpus.
func ListRuns(request ListRunsRequest) ([]runtime.RunSummary, error) {
	return runtime.ListRuns(request.RunsBaseDir)
}

// ListRunsPage loads paged run summaries for one runs corpus.
func ListRunsPage(request ListRunsPageRequest) (ListRunsPageResult, error) {
	summaries, err := ListRuns(ListRunsRequest{RunsBaseDir: request.RunsBaseDir})
	if err != nil {
		return ListRunsPageResult{}, err
	}

	return PaginateRunSummaries(PaginateRunSummariesRequest{
		Summaries: summaries,
		Limit:     request.Limit,
		Cursor:    request.Cursor,
	})
}

// PaginateRunSummaries pages one caller-supplied run summary slice with the same
// cursor semantics as the shared run-list query.
func PaginateRunSummaries(request PaginateRunSummariesRequest) (ListRunsPageResult, error) {
	summaries := cloneRunSummaries(request.Summaries)
	limit := request.Limit
	switch {
	case limit == 0:
		limit = defaultListRunsPageLimit
	case limit < 0:
		return ListRunsPageResult{}, fmt.Errorf("limit must be >= 0; %w", ErrInvalidListLimit)
	}

	startIndex := 0
	if request.Cursor != nil && strings.TrimSpace(*request.Cursor) != "" {
		if err := validateUUIDv7String("cursor", *request.Cursor); err != nil {
			return ListRunsPageResult{}, err
		}
		cursorIndex := -1
		for index, summary := range summaries {
			if summary.RunID == strings.TrimSpace(*request.Cursor) {
				cursorIndex = index
				break
			}
		}
		if cursorIndex < 0 {
			return ListRunsPageResult{}, fmt.Errorf("cursor %q was not found in the current run list; %w", *request.Cursor, ErrInvalidCursor)
		}
		startIndex = cursorIndex + 1
	}

	if startIndex >= len(summaries) {
		return ListRunsPageResult{Items: []runtime.RunSummary{}}, nil
	}

	endIndex := len(summaries)
	if limit > 0 && startIndex+limit < endIndex {
		endIndex = startIndex + limit
	}

	items := cloneRunSummaries(summaries[startIndex:endIndex])
	result := ListRunsPageResult{Items: items}
	if endIndex < len(summaries) && len(items) > 0 {
		nextCursor := items[len(items)-1].RunID
		result.NextCursor = &nextCursor
	}
	return result, nil
}

// ReadRunSummary loads one compact derived run summary.
func ReadRunSummary(request ReadRunSummaryRequest) (runtime.RunSummary, error) {
	if err := validateUUIDv7String("run_id", request.RunID); err != nil {
		return runtime.RunSummary{}, err
	}
	summary, err := runtime.LoadRunSummary(request.RunsBaseDir, request.RunID)
	if err != nil {
		return runtime.RunSummary{}, wrapRunLoadError(request.RunID, err)
	}
	return summary, nil
}

// ReadRun loads one detailed derived run projection.
func ReadRun(request ReadRunRequest) (runtime.RunProjection, error) {
	if err := validateUUIDv7String("run_id", request.RunID); err != nil {
		return runtime.RunProjection{}, err
	}
	projection, err := loadRunProjectionFunc(request.RunsBaseDir, request.RunID)
	if err != nil {
		return runtime.RunProjection{}, wrapRunLoadError(request.RunID, err)
	}
	return projection, nil
}

// ReadRunEvents loads canonical persisted events for one run.
func ReadRunEvents(request ReadRunEventsRequest) ([]runtime.EventEnvelope, error) {
	if err := validateUUIDv7String("run_id", request.RunID); err != nil {
		return nil, err
	}
	events, err := readRunEventsFunc(request.RunsBaseDir, request.RunID)
	if err != nil {
		return nil, wrapRunLoadError(request.RunID, err)
	}
	return events, nil
}

// ReadRunTree loads one derived node tree for a run.
func ReadRunTree(request ReadRunTreeRequest) (RunTreeResult, error) {
	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return RunTreeResult{}, err
	}
	return cloneRunTreeResult(data.tree), nil
}

// ListRunSteps loads one derived run-global step timeline.
func ListRunSteps(request ListRunStepsRequest) (ListRunStepsResult, error) {
	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return ListRunStepsResult{}, err
	}

	steps := make([]RunStepSummary, 0, len(data.stepOrder))
	for _, stepID := range data.stepOrder {
		accumulator := data.stepsByID[stepID]
		if accumulator == nil {
			continue
		}
		steps = append(steps, cloneRunStepSummary(accumulator.summary))
	}
	return ListRunStepsResult{
		RunID:   request.RunID,
		AsOfSeq: data.asOfSeq,
		Steps:   steps,
	}, nil
}

// ReadRunNode loads one derived node detail plus linkage metadata.
func ReadRunNode(request ReadRunNodeRequest) (ReadRunNodeResult, error) {
	if err := validateUUIDv7String("node_id", request.NodeID); err != nil {
		return ReadRunNodeResult{}, err
	}

	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return ReadRunNodeResult{}, err
	}

	node, ok := data.nodesByID[request.NodeID]
	if !ok {
		return ReadRunNodeResult{}, fmt.Errorf("node %q was not found in run %q; %w", request.NodeID, request.RunID, ErrNodeNotFound)
	}

	return ReadRunNodeResult{
		RunID:   request.RunID,
		AsOfSeq: data.asOfSeq,
		Node: RunNodeDetail{
			RunTreeNode:  cloneRunTreeNode(node.node),
			StepIDs:      append([]string(nil), node.stepIDs...),
			ActiveStepID: cloneStringPointer(node.activeStepID),
		},
	}, nil
}

// ReadRunStep loads one derived step detail.
func ReadRunStep(request ReadRunStepRequest) (ReadRunStepResult, error) {
	if strings.TrimSpace(request.NodeID) != "" {
		if err := validateUUIDv7String("node_id", request.NodeID); err != nil {
			return ReadRunStepResult{}, err
		}
	}
	if err := validateUUIDv7String("step_id", request.StepID); err != nil {
		return ReadRunStepResult{}, err
	}

	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return ReadRunStepResult{}, err
	}

	accumulator, ok := data.stepsByID[request.StepID]
	if !ok {
		return ReadRunStepResult{}, fmt.Errorf("step %q was not found in run %q; %w", request.StepID, request.RunID, ErrStepNotFound)
	}
	if strings.TrimSpace(request.NodeID) != "" && accumulator.summary.NodeID != request.NodeID {
		return ReadRunStepResult{}, fmt.Errorf("step %q was not found under node %q in run %q; %w", request.StepID, request.NodeID, request.RunID, ErrStepNotFound)
	}

	return ReadRunStepResult{
		RunID:   request.RunID,
		AsOfSeq: data.asOfSeq,
		Step: RunStepDetail{
			RunStepSummary:        cloneRunStepSummary(accumulator.summary),
			ActionRefs:            append([]string(nil), accumulator.actionRefs...),
			SubcallAccountingRefs: append([]string(nil), accumulator.subcallAccountingRefs...),
		},
	}, nil
}

// ReadRunArtifact loads one canonical persisted artifact body by ref.
func ReadRunArtifact(request ReadRunArtifactRequest) (ReadRunArtifactResult, error) {
	if err := validateUUIDv7String("run_id", request.RunID); err != nil {
		return ReadRunArtifactResult{}, err
	}

	resolvedBaseDir, err := runtime.ResolveRunsBaseDir(request.RunsBaseDir)
	if err != nil {
		return ReadRunArtifactResult{}, err
	}

	relativeParts, err := runtime.ResolveArtifactRefPath(request.ArtifactRef)
	if err != nil {
		return ReadRunArtifactResult{}, err
	}
	artifactKind, identity, err := parseArtifactIdentity(relativeParts)
	if err != nil {
		return ReadRunArtifactResult{}, err
	}

	relativePath := filepath.Join(append([]string{"artifacts"}, relativeParts...)...)
	absolutePath := filepath.Join(resolvedBaseDir, request.RunID, relativePath)
	content, err := os.ReadFile(filepath.Clean(absolutePath))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return ReadRunArtifactResult{}, fmt.Errorf("artifact %q was not found in run %q; %w", request.ArtifactRef, request.RunID, ErrArtifactNotFound)
		}
		return ReadRunArtifactResult{}, fmt.Errorf("failed to read artifact %q; %w", request.ArtifactRef, err)
	}

	var artifact map[string]any
	if err := json.Unmarshal(content, &artifact); err != nil {
		return ReadRunArtifactResult{}, fmt.Errorf("failed to decode artifact %q as JSON object; %w", request.ArtifactRef, err)
	}
	if artifact == nil {
		return ReadRunArtifactResult{}, fmt.Errorf("artifact %q must decode to a JSON object", request.ArtifactRef)
	}

	return ReadRunArtifactResult{
		RunID:        request.RunID,
		ArtifactRef:  strings.TrimSpace(request.ArtifactRef),
		ArtifactKind: artifactKind,
		Identity:     identity,
		Artifact:     artifact,
	}, nil
}

// BuildRunSubscriptionSnapshot derives one fresh-attach snapshot from a single authoritative event scan.
func BuildRunSubscriptionSnapshot(request BuildRunSubscriptionSnapshotRequest) (RunSubscriptionSnapshot, error) {
	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return RunSubscriptionSnapshot{}, err
	}

	activeNodeID, activeStepID := activeSubscriptionPointers(data.stepOrder, data.stepsByID)
	return RunSubscriptionSnapshot{
		RunID:           request.RunID,
		SnapshotAsOfSeq: data.asOfSeq,
		Run:             cloneRunProjection(data.projection),
		Tree:            cloneRunTreeResult(data.tree),
		ActiveNodeID:    cloneStringPointer(activeNodeID),
		ActiveStepID:    cloneStringPointer(activeStepID),
		Terminal:        runtime.IsTerminalRunState(runtime.RunState(data.projection.State)),
	}, nil
}

// ReplayRunEvents loads canonical events with seq > afterSeq for resume attach handling.
func ReplayRunEvents(request ReplayRunEventsRequest) (ReplayRunEventsResult, error) {
	if request.AfterSeq < 0 {
		return ReplayRunEventsResult{}, fmt.Errorf("after_seq must be >= 0; %w", ErrInvalidResumeCursor)
	}

	data, err := loadDerivedRunData(request.RunsBaseDir, request.RunID)
	if err != nil {
		return ReplayRunEventsResult{}, err
	}
	if request.AfterSeq > data.asOfSeq {
		return ReplayRunEventsResult{}, fmt.Errorf(
			"after_seq %d exceeds run watermark %d; %w",
			request.AfterSeq,
			data.asOfSeq,
			ErrInvalidResumeCursor,
		)
	}

	replayed := make([]runtime.EventEnvelope, 0, len(data.events))
	for _, event := range data.events {
		if event.Seq <= request.AfterSeq {
			continue
		}
		replayed = append(replayed, event)
	}

	return ReplayRunEventsResult{
		RunID:    request.RunID,
		AsOfSeq:  data.asOfSeq,
		Terminal: runtime.IsTerminalRunState(runtime.RunState(data.projection.State)),
		Events:   cloneEventEnvelopes(replayed),
	}, nil
}

func loadDerivedRunData(runsBaseDir string, runID string) (derivedRunData, error) {
	if err := validateUUIDv7String("run_id", runID); err != nil {
		return derivedRunData{}, err
	}

	events, err := readRunEventsFunc(runsBaseDir, runID)
	if err != nil {
		return derivedRunData{}, wrapRunLoadError(runID, err)
	}

	projection, err := deriveRunProjectionFromEvents(runsBaseDir, runID, events)
	if err != nil {
		return derivedRunData{}, wrapRunLoadError(runID, err)
	}

	asOfSeq := lastSequence(events)
	nodesByID, nodeOrder, rootNodeID := buildNodeAccumulators(projection, events)
	stepOrder, stepsByID := deriveRunSteps(events, nodesByID)

	return derivedRunData{
		projection: projection,
		events:     cloneEventEnvelopes(events),
		asOfSeq:    asOfSeq,
		tree:       buildRunTreeResult(projection.RunID, asOfSeq, rootNodeID, nodeOrder, nodesByID),
		nodesByID:  cloneNodeAccumulators(nodesByID),
		stepOrder:  append([]string(nil), stepOrder...),
		stepsByID:  cloneStepAccumulators(stepsByID),
	}, nil
}

func buildNodeAccumulators(projection runtime.RunProjection, events []runtime.EventEnvelope) (map[string]*nodeAccumulator, []string, *string) {
	projectionByNodeID := make(map[string]runtime.RunNodeProjection, len(projection.Nodes))
	for _, node := range projection.Nodes {
		projectionByNodeID[node.NodeID] = node
	}

	nodeOrder := make([]string, 0, len(projection.Nodes))
	parentByChild := make(map[string]*string)
	nodesByID := make(map[string]*nodeAccumulator, len(projection.Nodes))
	var rootNodeID *string

	for _, event := range events {
		payload, ok := event.Payload.(runtime.NodeStartedPayload)
		if !ok || event.NodeID == nil {
			continue
		}

		nodeID := *event.NodeID
		if _, seen := nodesByID[nodeID]; seen {
			continue
		}
		projected, ok := projectionByNodeID[nodeID]
		if !ok {
			continue
		}

		nodeOrder = append(nodeOrder, nodeID)
		if payload.ParentNodeID == nil && rootNodeID == nil {
			rootNodeID = cloneStringPointer(&nodeID)
		}
		parentByChild[nodeID] = cloneStringPointer(payload.ParentNodeID)
		nodesByID[nodeID] = &nodeAccumulator{
			node:       runTreeNodeFromProjection(projected),
			startedSeq: event.Seq,
			stepIDs:    []string{},
		}
	}

	for _, projected := range projection.Nodes {
		if _, seen := nodesByID[projected.NodeID]; seen {
			continue
		}
		nodeOrder = append(nodeOrder, projected.NodeID)
		if projected.ParentNodeID == nil && rootNodeID == nil {
			rootNodeID = cloneStringPointer(&projected.NodeID)
		}
		parentByChild[projected.NodeID] = cloneStringPointer(projected.ParentNodeID)
		nodesByID[projected.NodeID] = &nodeAccumulator{
			node:    runTreeNodeFromProjection(projected),
			stepIDs: []string{},
		}
	}

	for _, nodeID := range nodeOrder {
		parentNodeID := parentByChild[nodeID]
		if parentNodeID == nil {
			continue
		}
		parent, ok := nodesByID[*parentNodeID]
		if !ok {
			continue
		}
		parent.node.ChildNodeIDs = append(parent.node.ChildNodeIDs, nodeID)
	}

	return nodesByID, nodeOrder, rootNodeID
}

func buildRunTreeResult(runID string, asOfSeq int64, rootNodeID *string, nodeOrder []string, nodesByID map[string]*nodeAccumulator) RunTreeResult {
	nodes := make([]RunTreeNode, 0, len(nodeOrder))
	for _, nodeID := range nodeOrder {
		node := nodesByID[nodeID]
		if node == nil {
			continue
		}
		nodes = append(nodes, cloneRunTreeNode(node.node))
	}
	return RunTreeResult{
		RunID:      runID,
		AsOfSeq:    asOfSeq,
		RootNodeID: cloneStringPointer(rootNodeID),
		Nodes:      nodes,
	}
}

func deriveRunSteps(events []runtime.EventEnvelope, nodesByID map[string]*nodeAccumulator) ([]string, map[string]*stepAccumulator) {
	stepOrder := make([]string, 0, 8)
	stepsByID := make(map[string]*stepAccumulator)

	ensureStep := func(nodeID string, stepID string) *stepAccumulator {
		accumulator, ok := stepsByID[stepID]
		if ok {
			return accumulator
		}

		accumulator = &stepAccumulator{
			summary: RunStepSummary{
				StepID: stepID,
				NodeID: nodeID,
				State:  string(runtime.RunStateQueued),
			},
			actionRefs:            []string{},
			subcallAccountingRefs: []string{},
		}
		stepsByID[stepID] = accumulator
		stepOrder = append(stepOrder, stepID)
		if node := nodesByID[nodeID]; node != nil {
			node.stepIDs = append(node.stepIDs, stepID)
		}
		return accumulator
	}

	for _, event := range events {
		switch payload := event.Payload.(type) {
		case runtime.NodeStepStartedPayload:
			if event.NodeID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, payload.StepID)
			accumulator.summary.NodeStepIndex = payload.StepIndex
			accumulator.summary.StepStartedSeq = event.Seq
			accumulator.summary.SchemaID = payload.SchemaID
			accumulator.summary.State = string(runtime.RunStateRunning)
			accumulator.summary.StartedAt = timePointer(event.Timestamp)
			if node := nodesByID[*event.NodeID]; node != nil {
				node.activeStepID = cloneStringPointer(&payload.StepID)
			}
		case runtime.NodeTurnPayload:
			if event.NodeID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, payload.StepID)
			switch payload.Role {
			case runtime.TurnRoleUser:
				accumulator.summary.UserTurnRef = cloneStringPointer(&payload.ContentRef)
			case runtime.TurnRoleModel:
				accumulator.summary.ModelTurnRef = cloneStringPointer(&payload.ContentRef)
			}
		case runtime.NodeActionExecutedPayload:
			if event.NodeID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, payload.StepID)
			accumulator.actionRefs = append(accumulator.actionRefs, payload.ActionRef)
		case runtime.NodeSubcallExecutedPayload:
			if event.NodeID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, payload.StepID)
			accumulator.subcallAccountingRefs = append(accumulator.subcallAccountingRefs, payload.AccountingRef)
		case runtime.NodeStepCompletedPayload:
			if event.NodeID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, payload.StepID)
			accumulator.summary.State = string(runtime.RunStateCompleted)
			accumulator.summary.CompletedAt = timePointer(event.Timestamp)
			decision := string(payload.Decision)
			accumulator.summary.Decision = &decision
			accumulator.summary.DurationMS = cloneIntPointer(payload.DurationMS)
			accumulator.summary.ActionCount = payload.ActionCount
			accumulator.summary.SubcallCount = len(accumulator.subcallAccountingRefs)
			accumulator.summary.AccountingRef = cloneStringPointer(&payload.AccountingRef)
			clearActiveStep(nodesByID, *event.NodeID, payload.StepID)
		case runtime.NodeFailedPayload:
			if event.NodeID == nil || payload.FailedStepID == nil {
				continue
			}
			accumulator := ensureStep(*event.NodeID, *payload.FailedStepID)
			accumulator.summary.State = string(runtime.RunStateFailed)
			accumulator.summary.CompletedAt = timePointer(event.Timestamp)
			accumulator.summary.ErrorCode = cloneStringPointer(&payload.ErrorCode)
			accumulator.summary.ErrorMessage = cloneStringPointer(&payload.ErrorMessage)
			clearActiveStep(nodesByID, *event.NodeID, *payload.FailedStepID)
		case runtime.RunFailedPayload:
			if payload.FailedNodeID == nil || payload.FailedStepID == nil {
				continue
			}
			accumulator := ensureStep(*payload.FailedNodeID, *payload.FailedStepID)
			accumulator.summary.State = string(runtime.RunStateFailed)
			accumulator.summary.CompletedAt = timePointer(event.Timestamp)
			accumulator.summary.ErrorCode = cloneStringPointer(&payload.ErrorCode)
			accumulator.summary.ErrorMessage = cloneStringPointer(&payload.ErrorMessage)
			clearActiveStep(nodesByID, *payload.FailedNodeID, *payload.FailedStepID)
		case runtime.RunInterruptedPayload:
			if payload.InterruptedNodeID == nil {
				continue
			}
			node := nodesByID[*payload.InterruptedNodeID]
			if node == nil || node.activeStepID == nil {
				continue
			}
			accumulator := stepsByID[*node.activeStepID]
			if accumulator == nil || accumulator.summary.State != string(runtime.RunStateRunning) {
				continue
			}
			accumulator.summary.State = string(runtime.RunStateInterrupted)
			accumulator.summary.CompletedAt = timePointer(event.Timestamp)
		}
	}

	for _, stepID := range stepOrder {
		accumulator := stepsByID[stepID]
		if accumulator == nil {
			continue
		}
		if accumulator.summary.ActionCount == 0 {
			accumulator.summary.ActionCount = len(accumulator.actionRefs)
		}
		if accumulator.summary.SubcallCount == 0 {
			accumulator.summary.SubcallCount = len(accumulator.subcallAccountingRefs)
		}
	}

	return stepOrder, stepsByID
}

func clearActiveStep(nodesByID map[string]*nodeAccumulator, nodeID string, stepID string) {
	node := nodesByID[nodeID]
	if node == nil || node.activeStepID == nil || *node.activeStepID != stepID {
		return
	}
	node.activeStepID = nil
}

func lastSequence(events []runtime.EventEnvelope) int64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
}

func parseArtifactIdentity(relativeParts []string) (string, ArtifactIdentity, error) {
	switch {
	case len(relativeParts) == 3 && relativeParts[0] == "node" && relativeParts[2] == "context.json":
		nodeID := relativeParts[1]
		return "context", ArtifactIdentity{NodeID: &nodeID}, nil
	case len(relativeParts) == 3 && relativeParts[0] == "node" && relativeParts[2] == "final-answer.json":
		nodeID := relativeParts[1]
		return "final_answer", ArtifactIdentity{NodeID: &nodeID}, nil
	case len(relativeParts) == 3 && relativeParts[0] == "node" && relativeParts[2] == "accounting.json":
		nodeID := relativeParts[1]
		return "node_accounting", ArtifactIdentity{NodeID: &nodeID}, nil
	case len(relativeParts) == 5 && relativeParts[0] == "node" && relativeParts[2] == "step" && relativeParts[4] == "turn-user.json":
		nodeID := relativeParts[1]
		stepID := relativeParts[3]
		return "turn_user", ArtifactIdentity{NodeID: &nodeID, StepID: &stepID}, nil
	case len(relativeParts) == 5 && relativeParts[0] == "node" && relativeParts[2] == "step" && relativeParts[4] == "turn-model.json":
		nodeID := relativeParts[1]
		stepID := relativeParts[3]
		return "turn_model", ArtifactIdentity{NodeID: &nodeID, StepID: &stepID}, nil
	case len(relativeParts) == 5 && relativeParts[0] == "node" && relativeParts[2] == "step" && relativeParts[4] == "accounting.json":
		nodeID := relativeParts[1]
		stepID := relativeParts[3]
		return "step_accounting", ArtifactIdentity{NodeID: &nodeID, StepID: &stepID}, nil
	case len(relativeParts) == 5 && relativeParts[0] == "node" && relativeParts[2] == "step" && strings.HasPrefix(relativeParts[4], "action-") && strings.HasSuffix(relativeParts[4], ".json"):
		nodeID := relativeParts[1]
		stepID := relativeParts[3]
		index, err := parsePositiveIndex(strings.TrimSuffix(strings.TrimPrefix(relativeParts[4], "action-"), ".json"))
		if err != nil {
			return "", ArtifactIdentity{}, err
		}
		return "action", ArtifactIdentity{NodeID: &nodeID, StepID: &stepID, ActionIndex: &index}, nil
	case len(relativeParts) == 5 && relativeParts[0] == "node" && relativeParts[2] == "step" && strings.HasPrefix(relativeParts[4], "subcall-") && strings.HasSuffix(relativeParts[4], "-accounting.json"):
		nodeID := relativeParts[1]
		stepID := relativeParts[3]
		indexText := strings.TrimSuffix(strings.TrimPrefix(relativeParts[4], "subcall-"), "-accounting.json")
		index, err := parsePositiveIndex(indexText)
		if err != nil {
			return "", ArtifactIdentity{}, err
		}
		return "subcall_accounting", ArtifactIdentity{NodeID: &nodeID, StepID: &stepID, SubcallIndex: &index}, nil
	case len(relativeParts) == 2 && relativeParts[0] == "run" && relativeParts[1] == "accounting.json":
		return "run_accounting", ArtifactIdentity{}, nil
	case len(relativeParts) == 2 && relativeParts[0] == "run" && relativeParts[1] == "submitted-run-config.json":
		return "submitted_run_config", ArtifactIdentity{}, nil
	default:
		return "", ArtifactIdentity{}, fmt.Errorf("artifact_ref does not map to a supported canonical artifact kind")
	}
}

func parsePositiveIndex(raw string) (int, error) {
	index, err := strconv.Atoi(raw)
	if err != nil || index < 1 {
		return 0, fmt.Errorf("artifact_ref contains invalid positive index %q", raw)
	}
	return index, nil
}

func runTreeNodeFromProjection(node runtime.RunNodeProjection) RunTreeNode {
	return RunTreeNode{
		NodeID:        node.NodeID,
		ParentNodeID:  cloneStringPointer(node.ParentNodeID),
		Depth:         node.Depth,
		Role:          node.Role,
		State:         node.State,
		StartedAt:     cloneTimePointer(node.StartedAt),
		TerminalAt:    cloneTimePointer(node.TerminalAt),
		StepCount:     node.StepCount,
		ResultRef:     cloneStringPointer(node.ResultRef),
		AccountingRef: cloneStringPointer(node.AccountingRef),
		ErrorCode:     cloneStringPointer(node.ErrorCode),
		ErrorMessage:  cloneStringPointer(node.ErrorMessage),
		ChildNodeIDs:  []string{},
	}
}

func wrapRunLoadError(runID string, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("run %q was not found; %w", runID, ErrRunNotFound)
	}
	return err
}

func validateUUIDv7String(fieldName string, raw string) error {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return fmt.Errorf("%s is required; %w", fieldName, ErrInvalidIdentifier)
	}
	parsed, err := uuid.Parse(trimmed)
	if err != nil {
		return fmt.Errorf("%s must be UUIDv7; %w", fieldName, ErrInvalidIdentifier)
	}
	if parsed.Version() != uuid.Version(7) {
		return fmt.Errorf("%s must be UUIDv7; %w", fieldName, ErrInvalidIdentifier)
	}
	return nil
}

func cloneRunSummaries(values []runtime.RunSummary) []runtime.RunSummary {
	cloned := make([]runtime.RunSummary, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, runtime.RunSummary{
			RunID:          value.RunID,
			State:          value.State,
			Source:         value.Source,
			QueuedAt:       cloneTimePointer(value.QueuedAt),
			StartedAt:      cloneTimePointer(value.StartedAt),
			TerminalAt:     cloneTimePointer(value.TerminalAt),
			EventsPath:     value.EventsPath,
			PIDStatus:      value.PIDStatus,
			StopRequested:  value.StopRequested,
			FinalAnswerRef: cloneStringPointer(value.FinalAnswerRef),
			AccountingRef:  cloneStringPointer(value.AccountingRef),
			Error:          value.Error,
		})
	}
	return cloned
}

func cloneRunProjection(value runtime.RunProjection) runtime.RunProjection {
	cloned := value
	cloned.AppConfigPath = cloneStringPointer(value.AppConfigPath)
	cloned.RunConfigPath = cloneStringPointer(value.RunConfigPath)
	cloned.QueuedAt = cloneTimePointer(value.QueuedAt)
	cloned.StartedAt = cloneTimePointer(value.StartedAt)
	cloned.TerminalAt = cloneTimePointer(value.TerminalAt)
	cloned.ProcessMetadata = cloneProcessMetadata(value.ProcessMetadata)
	cloned.StopRequest = cloneStopRequest(value.StopRequest)
	cloned.FinalAnswerRef = cloneStringPointer(value.FinalAnswerRef)
	cloned.AccountingRef = cloneStringPointer(value.AccountingRef)
	cloned.ErrorCode = cloneStringPointer(value.ErrorCode)
	cloned.ErrorMessage = cloneStringPointer(value.ErrorMessage)
	cloned.FailedNodeID = cloneStringPointer(value.FailedNodeID)
	cloned.FailedStepID = cloneStringPointer(value.FailedStepID)
	cloned.InterruptedReason = cloneStringPointer(value.InterruptedReason)
	cloned.InterruptedBy = cloneStringPointer(value.InterruptedBy)
	cloned.InterruptedNodeID = cloneStringPointer(value.InterruptedNodeID)
	cloned.Nodes = cloneRunNodeProjections(value.Nodes)
	return cloned
}

func cloneRunTreeResult(value RunTreeResult) RunTreeResult {
	cloned := RunTreeResult{
		RunID:      value.RunID,
		AsOfSeq:    value.AsOfSeq,
		RootNodeID: cloneStringPointer(value.RootNodeID),
		Nodes:      make([]RunTreeNode, 0, len(value.Nodes)),
	}
	for _, node := range value.Nodes {
		cloned.Nodes = append(cloned.Nodes, cloneRunTreeNode(node))
	}
	return cloned
}

func cloneRunTreeNode(value RunTreeNode) RunTreeNode {
	return RunTreeNode{
		NodeID:        value.NodeID,
		ParentNodeID:  cloneStringPointer(value.ParentNodeID),
		Depth:         value.Depth,
		Role:          value.Role,
		State:         value.State,
		StartedAt:     cloneTimePointer(value.StartedAt),
		TerminalAt:    cloneTimePointer(value.TerminalAt),
		StepCount:     value.StepCount,
		ResultRef:     cloneStringPointer(value.ResultRef),
		AccountingRef: cloneStringPointer(value.AccountingRef),
		ErrorCode:     cloneStringPointer(value.ErrorCode),
		ErrorMessage:  cloneStringPointer(value.ErrorMessage),
		ChildNodeIDs:  append([]string(nil), value.ChildNodeIDs...),
	}
}

func cloneRunStepSummary(value RunStepSummary) RunStepSummary {
	return RunStepSummary{
		StepID:         value.StepID,
		NodeID:         value.NodeID,
		NodeStepIndex:  value.NodeStepIndex,
		StepStartedSeq: value.StepStartedSeq,
		SchemaID:       value.SchemaID,
		Decision:       cloneStringPointer(value.Decision),
		State:          value.State,
		StartedAt:      cloneTimePointer(value.StartedAt),
		CompletedAt:    cloneTimePointer(value.CompletedAt),
		DurationMS:     cloneIntPointerValue(value.DurationMS),
		ActionCount:    value.ActionCount,
		SubcallCount:   value.SubcallCount,
		UserTurnRef:    cloneStringPointer(value.UserTurnRef),
		ModelTurnRef:   cloneStringPointer(value.ModelTurnRef),
		AccountingRef:  cloneStringPointer(value.AccountingRef),
		ErrorCode:      cloneStringPointer(value.ErrorCode),
		ErrorMessage:   cloneStringPointer(value.ErrorMessage),
	}
}

func cloneNodeAccumulators(values map[string]*nodeAccumulator) map[string]*nodeAccumulator {
	cloned := make(map[string]*nodeAccumulator, len(values))
	for nodeID, value := range values {
		if value == nil {
			continue
		}
		cloned[nodeID] = &nodeAccumulator{
			node:         cloneRunTreeNode(value.node),
			startedSeq:   value.startedSeq,
			stepIDs:      append([]string(nil), value.stepIDs...),
			activeStepID: cloneStringPointer(value.activeStepID),
		}
	}
	return cloned
}

func cloneStepAccumulators(values map[string]*stepAccumulator) map[string]*stepAccumulator {
	cloned := make(map[string]*stepAccumulator, len(values))
	for stepID, value := range values {
		if value == nil {
			continue
		}
		cloned[stepID] = &stepAccumulator{
			summary:               cloneRunStepSummary(value.summary),
			actionRefs:            append([]string(nil), value.actionRefs...),
			subcallAccountingRefs: append([]string(nil), value.subcallAccountingRefs...),
		}
	}
	return cloned
}

func activeSubscriptionPointers(stepOrder []string, stepsByID map[string]*stepAccumulator) (*string, *string) {
	var activeNodeID *string
	var activeStepID *string
	var highestSeq int64

	for _, stepID := range stepOrder {
		accumulator := stepsByID[stepID]
		if accumulator == nil || accumulator.summary.State != string(runtime.RunStateRunning) {
			continue
		}
		if activeStepID != nil && accumulator.summary.StepStartedSeq <= highestSeq {
			continue
		}

		highestSeq = accumulator.summary.StepStartedSeq
		activeNodeID = cloneOptionalString(accumulator.summary.NodeID)
		activeStepID = cloneOptionalString(accumulator.summary.StepID)
	}

	return activeNodeID, activeStepID
}

func cloneOptionalString(raw string) *string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	value := raw
	return &value
}

func cloneRunNodeProjections(values []runtime.RunNodeProjection) []runtime.RunNodeProjection {
	cloned := make([]runtime.RunNodeProjection, 0, len(values))
	for _, value := range values {
		cloned = append(cloned, runtime.RunNodeProjection{
			NodeID:        value.NodeID,
			ParentNodeID:  cloneStringPointer(value.ParentNodeID),
			Depth:         value.Depth,
			Role:          value.Role,
			State:         value.State,
			StartedAt:     cloneTimePointer(value.StartedAt),
			TerminalAt:    cloneTimePointer(value.TerminalAt),
			StepCount:     value.StepCount,
			ResultRef:     cloneStringPointer(value.ResultRef),
			AccountingRef: cloneStringPointer(value.AccountingRef),
			ErrorCode:     cloneStringPointer(value.ErrorCode),
			ErrorMessage:  cloneStringPointer(value.ErrorMessage),
		})
	}
	return cloned
}

func cloneProcessMetadata(value *runtime.ProcessMetadata) *runtime.ProcessMetadata {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.RecordedAt = cloned.RecordedAt.UTC()
	cloned.StartedAt = cloned.StartedAt.UTC()
	return &cloned
}

func cloneStopRequest(value *runtime.StopRequestMetadata) *runtime.StopRequestMetadata {
	if value == nil {
		return nil
	}
	cloned := *value
	cloned.RequestedAt = cloned.RequestedAt.UTC()
	return &cloned
}

func cloneStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneTimePointer(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := value.UTC()
	return &copyValue
}

func cloneIntPointer(value int) *int {
	copyValue := value
	return &copyValue
}

func cloneIntPointerValue(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneEventEnvelopes(events []runtime.EventEnvelope) []runtime.EventEnvelope {
	cloned := make([]runtime.EventEnvelope, 0, len(events))
	for _, event := range events {
		cloned = append(cloned, event)
	}
	return cloned
}

func timePointer(value time.Time) *time.Time {
	copyValue := value.UTC()
	return &copyValue
}
