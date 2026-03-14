package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/query"
	"github.com/leefowlercu/sigil/internal/runtime"
)

func (s *Server) registerRunReadHandlers() {
	if s == nil || s.dispatcher == nil {
		return
	}

	s.dispatcher.Register("run/list", s.handleRunList)
	s.dispatcher.Register("run/read", s.handleRunRead)
	s.dispatcher.Register("run/events/read", s.handleRunEventsRead)
	s.dispatcher.Register("run/tree/read", s.handleRunTreeRead)
	s.dispatcher.Register("run/steps/list", s.handleRunStepsList)
	s.dispatcher.Register("run/node/read", s.handleRunNodeRead)
	s.dispatcher.Register("run/step/read", s.handleRunStepRead)
	s.dispatcher.Register("run/artifact/read", s.handleRunArtifactRead)
}

func (s *Server) handleRunList(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunListParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/list",
		querySource: "run_list_page",
		cursor:      params.Cursor,
		limit:       &params.Limit,
	}

	result, err := query.ListRunsPage(query.ListRunsPageRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		Limit:       params.Limit,
		Cursor:      params.Cursor,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}

	items := make([]protocol.RunSummaryView, 0, len(result.Items))
	for _, item := range result.Items {
		items = append(items, mapRunSummary(item))
	}
	logReadRequestSuccess(readCtx,
		"item_count", len(items),
		"has_next_cursor", result.NextCursor != nil,
	)

	return protocol.RunListResult{
		Payload: protocol.RunListPayload{
			Items:      items,
			NextCursor: cloneStringPointer(result.NextCursor),
		},
	}, nil
}

func (s *Server) handleRunRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/read",
		querySource: "run_projection_and_canonical_events",
		runID:       params.RunID,
	}

	runProjection, err := query.ReadRun(query.ReadRunRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}
	events, err := query.ReadRunEvents(query.ReadRunEventsRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}
	asOfSeq := lastEventSequence(events)
	logReadRequestSuccess(readCtx,
		"as_of_seq", asOfSeq,
		"node_count", len(runProjection.Nodes),
	)

	return protocol.RunReadResult{
		RunID:   params.RunID,
		AsOfSeq: asOfSeq,
		Payload: protocol.RunReadPayload{
			Run: mapRunProjection(runProjection),
		},
	}, nil
}

func (s *Server) handleRunEventsRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunEventsReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/events/read",
		querySource: "canonical_events",
		runID:       params.RunID,
	}

	events, err := query.ReadRunEvents(query.ReadRunEventsRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}

	mappedEvents := make([]protocol.EventEnvelopeView, 0, len(events))
	for _, event := range events {
		mapped, mapErr := mapEventEnvelope(event)
		if mapErr != nil {
			errObject := protocol.NewError(protocol.CodeInternalError, "failed to map canonical event payload", "event_mapping_failed", false, map[string]interface{}{
				"error": mapErr.Error(),
			})
			logReadRequestFailure(readCtx, mapErr, errObject)
			return nil, protocol.NewError(protocol.CodeInternalError, "failed to map canonical event payload", "event_mapping_failed", false, map[string]interface{}{
				"error": mapErr.Error(),
			})
		}
		mappedEvents = append(mappedEvents, mapped)
	}

	payload := protocol.RunEventsReadPayload{
		Events: mappedEvents,
	}
	if len(events) > 0 {
		payload.FirstSeq = events[0].Seq
		payload.LastSeq = events[len(events)-1].Seq
	}
	logReadRequestSuccess(readCtx,
		"event_count", len(mappedEvents),
		"first_seq", payload.FirstSeq,
		"last_seq", payload.LastSeq,
	)

	return protocol.RunEventsReadResult{
		RunID:   params.RunID,
		Payload: payload,
	}, nil
}

func (s *Server) handleRunTreeRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunTreeReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/tree/read",
		querySource: "run_tree_projection",
		runID:       params.RunID,
	}

	result, err := query.ReadRunTree(query.ReadRunTreeRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}

	nodes := make([]protocol.RunTreeNodeView, 0, len(result.Nodes))
	for _, node := range result.Nodes {
		nodes = append(nodes, mapRunTreeNode(node))
	}
	logReadRequestSuccess(readCtx,
		"as_of_seq", result.AsOfSeq,
		"item_count", len(nodes),
	)

	return protocol.RunTreeReadResult{
		RunID:   result.RunID,
		AsOfSeq: result.AsOfSeq,
		Payload: protocol.RunTreeReadPayload{
			RootNodeID: cloneStringPointer(result.RootNodeID),
			Nodes:      nodes,
		},
	}, nil
}

func (s *Server) handleRunStepsList(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunStepsListParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/steps/list",
		querySource: "run_step_timeline",
		runID:       params.RunID,
	}

	result, err := query.ListRunSteps(query.ListRunStepsRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}

	steps := make([]protocol.RunStepSummaryView, 0, len(result.Steps))
	for _, step := range result.Steps {
		steps = append(steps, mapRunStepSummary(step))
	}
	logReadRequestSuccess(readCtx,
		"as_of_seq", result.AsOfSeq,
		"item_count", len(steps),
	)

	return protocol.RunStepsListResult{
		RunID:   result.RunID,
		AsOfSeq: result.AsOfSeq,
		Payload: protocol.RunStepsListPayload{
			Steps: steps,
		},
	}, nil
}

func (s *Server) handleRunNodeRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunNodeReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/node/read",
		querySource: "run_node_detail",
		runID:       params.RunID,
		nodeID:      params.NodeID,
	}

	result, err := query.ReadRunNode(query.ReadRunNodeRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
		NodeID:      params.NodeID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}
	logReadRequestSuccess(readCtx,
		"as_of_seq", result.AsOfSeq,
		"step_count", len(result.Node.StepIDs),
		"child_count", len(result.Node.ChildNodeIDs),
	)

	return protocol.RunNodeReadResult{
		RunID:   result.RunID,
		AsOfSeq: result.AsOfSeq,
		Payload: protocol.RunNodeReadPayload{
			Node: mapRunNodeDetail(result.Node),
		},
	}, nil
}

func (s *Server) handleRunStepRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunStepReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/step/read",
		querySource: "run_step_detail",
		runID:       params.RunID,
		nodeID:      params.NodeID,
		stepID:      params.StepID,
	}

	result, err := query.ReadRunStep(query.ReadRunStepRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
		NodeID:      params.NodeID,
		StepID:      params.StepID,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}
	logReadRequestSuccess(readCtx,
		"as_of_seq", result.AsOfSeq,
		"action_count", len(result.Step.ActionRefs),
		"subcall_count", len(result.Step.SubcallAccountingRefs),
	)

	return protocol.RunStepReadResult{
		RunID:   result.RunID,
		AsOfSeq: result.AsOfSeq,
		Payload: protocol.RunStepReadPayload{
			Step: mapRunStepDetail(result.Step),
		},
	}, nil
}

func (s *Server) handleRunArtifactRead(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunArtifactReadParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}
	readCtx := readLogContext{
		method:      "run/artifact/read",
		querySource: "canonical_artifact",
		runID:       params.RunID,
		artifactRef: params.ArtifactRef,
	}

	result, err := query.ReadRunArtifact(query.ReadRunArtifactRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
		ArtifactRef: params.ArtifactRef,
	})
	if err != nil {
		errObject := appServerQueryError(err)
		logReadRequestFailure(readCtx, err, errObject)
		return nil, errObject
	}
	logReadRequestSuccess(readCtx,
		"artifact_kind", result.ArtifactKind,
	)

	return protocol.RunArtifactReadResult{
		RunID: result.RunID,
		Payload: protocol.RunArtifactReadPayload{
			ArtifactRef:  result.ArtifactRef,
			ArtifactKind: result.ArtifactKind,
			Identity:     mapArtifactIdentity(result.Identity),
			Artifact:     cloneMap(result.Artifact),
		},
	}, nil
}

func appServerQueryError(err error) *protocol.ErrorObject {
	switch {
	case errors.Is(err, query.ErrInvalidListLimit):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid list limit", "invalid_list_limit", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrInvalidCursor):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid cursor", "invalid_cursor", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrInvalidIdentifier):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid identifier", "invalid_identifier", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, runtime.ErrInvalidEvent):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid artifact reference", "invalid_artifact_ref", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrRunNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "run not found", "run_not_found", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrNodeNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "node not found", "node_not_found", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrStepNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "step not found", "step_not_found", false, map[string]interface{}{
			"error": err.Error(),
		})
	case errors.Is(err, query.ErrArtifactNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "artifact not found", "artifact_not_found", false, map[string]interface{}{
			"error": err.Error(),
		})
	default:
		return protocol.NewError(protocol.CodeInternalError, "app-server read query failed", "query_failed", false, map[string]interface{}{
			"error": err.Error(),
		})
	}
}

func mapRunSummary(value runtime.RunSummary) protocol.RunSummaryView {
	return protocol.RunSummaryView{
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
	}
}

func mapRunProjection(value runtime.RunProjection) protocol.RunProjectionView {
	nodes := make([]protocol.RunNodeProjectionView, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, protocol.RunNodeProjectionView{
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
		})
	}

	return protocol.RunProjectionView{
		RunID:             value.RunID,
		State:             value.State,
		RunDir:            value.RunDir,
		EventsPath:        value.EventsPath,
		Source:            value.Source,
		AppConfigPath:     cloneStringPointer(value.AppConfigPath),
		RunConfigPath:     cloneStringPointer(value.RunConfigPath),
		QueuedAt:          cloneTimePointer(value.QueuedAt),
		StartedAt:         cloneTimePointer(value.StartedAt),
		TerminalAt:        cloneTimePointer(value.TerminalAt),
		Executor:          value.Executor,
		MaxDepth:          value.MaxDepth,
		PIDStatus:         value.PIDStatus,
		StopRequested:     value.StopRequested,
		ProcessMetadata:   mapProcessMetadata(value.ProcessMetadata),
		StopRequest:       mapStopRequestMetadata(value.StopRequest),
		FinalAnswerRef:    cloneStringPointer(value.FinalAnswerRef),
		AccountingRef:     cloneStringPointer(value.AccountingRef),
		ErrorCode:         cloneStringPointer(value.ErrorCode),
		ErrorMessage:      cloneStringPointer(value.ErrorMessage),
		FailedNodeID:      cloneStringPointer(value.FailedNodeID),
		FailedStepID:      cloneStringPointer(value.FailedStepID),
		InterruptedReason: cloneStringPointer(value.InterruptedReason),
		InterruptedBy:     cloneStringPointer(value.InterruptedBy),
		InterruptedNodeID: cloneStringPointer(value.InterruptedNodeID),
		NodeCount:         value.NodeCount,
		StepCount:         value.StepCount,
		ActionCount:       value.ActionCount,
		SubcallCount:      value.SubcallCount,
		Nodes:             nodes,
	}
}

func mapProcessMetadata(value *runtime.ProcessMetadata) *protocol.ProcessMetadataView {
	if value == nil {
		return nil
	}
	return &protocol.ProcessMetadataView{
		RunID:      value.RunID,
		PID:        value.PID,
		RecordedAt: value.RecordedAt,
		StartedAt:  value.StartedAt,
		Source:     value.Source,
	}
}

func mapStopRequestMetadata(value *runtime.StopRequestMetadata) *protocol.StopRequestMetadataView {
	if value == nil {
		return nil
	}
	return &protocol.StopRequestMetadataView{
		RunID:       value.RunID,
		RequestedAt: value.RequestedAt,
		RequestedBy: value.RequestedBy,
		Signal:      value.Signal,
	}
}

func mapEventEnvelope(value runtime.EventEnvelope) (protocol.EventEnvelopeView, error) {
	payload := map[string]any{}
	encoded, err := json.Marshal(value.Payload)
	if err != nil {
		return protocol.EventEnvelopeView{}, fmt.Errorf("failed to encode event payload; %w", err)
	}
	if len(encoded) > 0 && string(encoded) != "null" {
		if err := json.Unmarshal(encoded, &payload); err != nil {
			return protocol.EventEnvelopeView{}, fmt.Errorf("failed to decode event payload; %w", err)
		}
	}
	return protocol.EventEnvelopeView{
		EventID:       value.EventID,
		SchemaVersion: value.SchemaVersion,
		RunID:         value.RunID,
		Seq:           value.Seq,
		Timestamp:     value.Timestamp,
		Type:          string(value.Type),
		NodeID:        cloneStringPointer(value.NodeID),
		CausationID:   value.CausationID,
		CorrelationID: value.CorrelationID,
		Payload:       payload,
	}, nil
}

func mapRunTreeNode(value query.RunTreeNode) protocol.RunTreeNodeView {
	return protocol.RunTreeNodeView{
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

func mapRunStepSummary(value query.RunStepSummary) protocol.RunStepSummaryView {
	return protocol.RunStepSummaryView{
		NodeID:         value.NodeID,
		StepID:         value.StepID,
		NodeStepIndex:  value.NodeStepIndex,
		StepStartedSeq: value.StepStartedSeq,
		SchemaID:       value.SchemaID,
		Decision:       cloneStringPointer(value.Decision),
		State:          value.State,
		StartedAt:      cloneTimePointer(value.StartedAt),
		CompletedAt:    cloneTimePointer(value.CompletedAt),
		DurationMS:     cloneIntPointer(value.DurationMS),
		ActionCount:    value.ActionCount,
		SubcallCount:   value.SubcallCount,
		UserTurnRef:    cloneStringPointer(value.UserTurnRef),
		ModelTurnRef:   cloneStringPointer(value.ModelTurnRef),
		AccountingRef:  cloneStringPointer(value.AccountingRef),
		ErrorCode:      cloneStringPointer(value.ErrorCode),
		ErrorMessage:   cloneStringPointer(value.ErrorMessage),
	}
}

func mapRunNodeDetail(value query.RunNodeDetail) protocol.RunNodeDetailView {
	return protocol.RunNodeDetailView{
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
		StepIDs:       append([]string(nil), value.StepIDs...),
		ActiveStepID:  cloneStringPointer(value.ActiveStepID),
		ChildNodeIDs:  append([]string(nil), value.ChildNodeIDs...),
	}
}

func mapRunStepDetail(value query.RunStepDetail) protocol.RunStepDetailView {
	return protocol.RunStepDetailView{
		NodeID:                value.NodeID,
		StepID:                value.StepID,
		NodeStepIndex:         value.NodeStepIndex,
		StepStartedSeq:        value.StepStartedSeq,
		SchemaID:              value.SchemaID,
		Decision:              cloneStringPointer(value.Decision),
		State:                 value.State,
		StartedAt:             cloneTimePointer(value.StartedAt),
		CompletedAt:           cloneTimePointer(value.CompletedAt),
		DurationMS:            cloneIntPointer(value.DurationMS),
		ActionCount:           value.ActionCount,
		SubcallCount:          value.SubcallCount,
		UserTurnRef:           cloneStringPointer(value.UserTurnRef),
		ModelTurnRef:          cloneStringPointer(value.ModelTurnRef),
		AccountingRef:         cloneStringPointer(value.AccountingRef),
		ErrorCode:             cloneStringPointer(value.ErrorCode),
		ErrorMessage:          cloneStringPointer(value.ErrorMessage),
		ActionRefs:            append([]string(nil), value.ActionRefs...),
		SubcallAccountingRefs: append([]string(nil), value.SubcallAccountingRefs...),
	}
}

func mapArtifactIdentity(value query.ArtifactIdentity) protocol.ArtifactIdentityView {
	return protocol.ArtifactIdentityView{
		NodeID:       cloneStringPointer(value.NodeID),
		StepID:       cloneStringPointer(value.StepID),
		ActionIndex:  cloneIntPointer(value.ActionIndex),
		SubcallIndex: cloneIntPointer(value.SubcallIndex),
	}
}

func lastEventSequence(events []runtime.EventEnvelope) int64 {
	if len(events) == 0 {
		return 0
	}
	return events[len(events)-1].Seq
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

func cloneIntPointer(value *int) *int {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneMap(value map[string]any) map[string]any {
	if value == nil {
		return nil
	}
	cloned := make(map[string]any, len(value))
	for key, item := range value {
		cloned[key] = item
	}
	return cloned
}
