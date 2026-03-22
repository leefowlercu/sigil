package query

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type richRunFixture struct {
	RunsDir              string
	RunID                string
	RootNodeID           string
	ChildNodeID          string
	StepID               string
	UserTurnRef          string
	ModelTurnRef         string
	ActionRef            string
	StepAccountingRef    string
	SubcallAccountingRef string
	RunAccountingRef     string
	FinalAnswerRef       string
}

func TestReadRunTreePreservesCanonicalNodeOrderAndLinkage(t *testing.T) {
	fixture := createRichRunFixture(t)

	result, err := ReadRunTree(ReadRunTreeRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
	})
	if err != nil {
		t.Fatalf("expected read run tree success, got %v", err)
	}

	if result.AsOfSeq < 1 {
		t.Fatalf("expected as_of_seq >= 1, got %d", result.AsOfSeq)
	}
	if result.RootNodeID == nil || *result.RootNodeID != fixture.RootNodeID {
		t.Fatalf("expected root node %q, got %v", fixture.RootNodeID, result.RootNodeID)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
	if result.Nodes[0].NodeID != fixture.RootNodeID {
		t.Fatalf("expected root node first, got %q", result.Nodes[0].NodeID)
	}
	if len(result.Nodes[0].ChildNodeIDs) != 1 || result.Nodes[0].ChildNodeIDs[0] != fixture.ChildNodeID {
		t.Fatalf("expected root child linkage to %q, got %+v", fixture.ChildNodeID, result.Nodes[0].ChildNodeIDs)
	}
	if result.Nodes[1].NodeID != fixture.ChildNodeID {
		t.Fatalf("expected child node second, got %q", result.Nodes[1].NodeID)
	}
}

func TestListRunStepsAndReadRunNodeExposeTimelineAndNodeLinkage(t *testing.T) {
	fixture := createRichRunFixture(t)

	stepsResult, err := ListRunSteps(ListRunStepsRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
	})
	if err != nil {
		t.Fatalf("expected list run steps success, got %v", err)
	}
	if len(stepsResult.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(stepsResult.Steps))
	}
	step := stepsResult.Steps[0]
	if step.StepID != fixture.StepID {
		t.Fatalf("expected step %q, got %q", fixture.StepID, step.StepID)
	}
	if step.SchemaID != "sigil.rlm.response.v1" {
		t.Fatalf("expected schema id sigil.rlm.response.v1, got %q", step.SchemaID)
	}
	if step.StepStartedSeq < 1 {
		t.Fatalf("expected step_started_seq >= 1, got %d", step.StepStartedSeq)
	}
	if step.ActionCount != 1 {
		t.Fatalf("expected action_count=1, got %d", step.ActionCount)
	}
	if step.SubcallCount != 1 {
		t.Fatalf("expected subcall_count=1, got %d", step.SubcallCount)
	}
	if step.UserTurnRef == nil || *step.UserTurnRef != fixture.UserTurnRef {
		t.Fatalf("expected user turn ref %q, got %v", fixture.UserTurnRef, step.UserTurnRef)
	}
	if step.ModelTurnRef == nil || *step.ModelTurnRef != fixture.ModelTurnRef {
		t.Fatalf("expected model turn ref %q, got %v", fixture.ModelTurnRef, step.ModelTurnRef)
	}
	if step.AccountingRef == nil || *step.AccountingRef != fixture.StepAccountingRef {
		t.Fatalf("expected step accounting ref %q, got %v", fixture.StepAccountingRef, step.AccountingRef)
	}

	nodeResult, err := ReadRunNode(ReadRunNodeRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		NodeID:      fixture.RootNodeID,
	})
	if err != nil {
		t.Fatalf("expected read run node success, got %v", err)
	}
	if len(nodeResult.Node.StepIDs) != 1 || nodeResult.Node.StepIDs[0] != fixture.StepID {
		t.Fatalf("expected node step linkage to %q, got %+v", fixture.StepID, nodeResult.Node.StepIDs)
	}
	if nodeResult.Node.ActiveStepID != nil {
		t.Fatalf("expected completed node to have no active step, got %v", nodeResult.Node.ActiveStepID)
	}
	if len(nodeResult.Node.ChildNodeIDs) != 1 || nodeResult.Node.ChildNodeIDs[0] != fixture.ChildNodeID {
		t.Fatalf("expected node child linkage to %q, got %+v", fixture.ChildNodeID, nodeResult.Node.ChildNodeIDs)
	}
}

func TestReadRunStepAndArtifactExposeRefsAndTypedArtifactBody(t *testing.T) {
	fixture := createRichRunFixture(t)

	stepResult, err := ReadRunStep(ReadRunStepRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		NodeID:      fixture.RootNodeID,
		StepID:      fixture.StepID,
	})
	if err != nil {
		t.Fatalf("expected read run step success, got %v", err)
	}
	if len(stepResult.Step.ActionRefs) != 1 || stepResult.Step.ActionRefs[0] != fixture.ActionRef {
		t.Fatalf("expected action ref %q, got %+v", fixture.ActionRef, stepResult.Step.ActionRefs)
	}
	if len(stepResult.Step.SubcallAccountingRefs) != 1 || stepResult.Step.SubcallAccountingRefs[0] != fixture.SubcallAccountingRef {
		t.Fatalf("expected subcall accounting ref %q, got %+v", fixture.SubcallAccountingRef, stepResult.Step.SubcallAccountingRefs)
	}

	artifactResult, err := ReadRunArtifact(ReadRunArtifactRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		ArtifactRef: fixture.ActionRef,
	})
	if err != nil {
		t.Fatalf("expected read run artifact success, got %v", err)
	}
	if artifactResult.ArtifactKind != "action" {
		t.Fatalf("expected artifact kind action, got %q", artifactResult.ArtifactKind)
	}
	if artifactResult.Identity.NodeID == nil || *artifactResult.Identity.NodeID != fixture.RootNodeID {
		t.Fatalf("expected artifact node id %q, got %v", fixture.RootNodeID, artifactResult.Identity.NodeID)
	}
	if artifactResult.Identity.StepID == nil || *artifactResult.Identity.StepID != fixture.StepID {
		t.Fatalf("expected artifact step id %q, got %v", fixture.StepID, artifactResult.Identity.StepID)
	}
	if artifactResult.Identity.ActionIndex == nil || *artifactResult.Identity.ActionIndex != 1 {
		t.Fatalf("expected action index 1, got %v", artifactResult.Identity.ActionIndex)
	}
	if artifactResult.Artifact["stdout"] != "fixture-stdout" {
		t.Fatalf("expected stdout fixture-stdout, got %v", artifactResult.Artifact["stdout"])
	}
}

func TestBuildRunSubscriptionSnapshotExposesActivePointersAndTerminalFlag(t *testing.T) {
	runsDir := t.TempDir()
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir:  runsDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     2,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node success, got %v", err)
	}
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected active step start success, got %v", err)
	}

	snapshot, err := BuildRunSubscriptionSnapshot(BuildRunSubscriptionSnapshotRequest{
		RunsBaseDir: runsDir,
		RunID:       lifecycle.RunID(),
	})
	if err != nil {
		t.Fatalf("expected subscription snapshot success, got %v", err)
	}
	if snapshot.SnapshotAsOfSeq != 4 {
		t.Fatalf("expected snapshot watermark 4, got %d", snapshot.SnapshotAsOfSeq)
	}
	if snapshot.ActiveNodeID == nil || *snapshot.ActiveNodeID != rootNode.ID {
		t.Fatalf("expected active node id %q, got %v", rootNode.ID, snapshot.ActiveNodeID)
	}
	if snapshot.ActiveStepID == nil || *snapshot.ActiveStepID != stepStarted.StepID {
		t.Fatalf("expected active step id %q, got %v", stepStarted.StepID, snapshot.ActiveStepID)
	}
	if snapshot.Terminal {
		t.Fatal("expected running snapshot to be non-terminal")
	}
}

func TestBuildRunSubscriptionSnapshotUsesOneCanonicalEventScan(t *testing.T) {
	fixture := createRichRunFixture(t)

	previousLoadRunProjection := loadRunProjectionFunc
	previousReadRunEvents := readRunEventsFunc
	previousDeriveRunProjection := deriveRunProjectionFromEvents
	t.Cleanup(func() {
		loadRunProjectionFunc = previousLoadRunProjection
		readRunEventsFunc = previousReadRunEvents
		deriveRunProjectionFromEvents = previousDeriveRunProjection
	})

	loadProjectionCalls := 0
	readEventsCalls := 0
	deriveProjectionCalls := 0
	readRunEventsFunc = func(baseDir string, runID string) ([]runtime.EventEnvelope, error) {
		readEventsCalls++
		return []runtime.EventEnvelope{
			{
				RunID:         runID,
				Seq:           1,
				Type:          runtime.EventTypeRunQueued,
				SchemaVersion: runtime.SchemaVersionV1,
				Timestamp:     time.Unix(1, 0).UTC(),
				Payload: runtime.RunQueuedPayload{
					Source: runtime.RunQueuedSourceCLIRunStart,
				},
			},
		}, nil
	}
	loadRunProjectionFunc = func(baseDir string, runID string) (runtime.RunProjection, error) {
		loadProjectionCalls++
		return runtime.RunProjection{}, nil
	}
	deriveRunProjectionFromEvents = func(baseDir string, runID string, events []runtime.EventEnvelope) (runtime.RunProjection, error) {
		deriveProjectionCalls++
		return runtime.RunProjection{
			RunID:      runID,
			State:      string(runtime.RunStateRunning),
			RunDir:     filepath.Join(baseDir, runID),
			EventsPath: filepath.Join(baseDir, runID, "events.jsonl"),
		}, nil
	}

	snapshot, err := BuildRunSubscriptionSnapshot(BuildRunSubscriptionSnapshotRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
	})
	if err != nil {
		t.Fatalf("expected subscription snapshot success, got %v", err)
	}
	if readEventsCalls != 1 {
		t.Fatalf("expected one canonical event read, got %d", readEventsCalls)
	}
	if deriveProjectionCalls != 1 {
		t.Fatalf("expected one projection derivation from events, got %d", deriveProjectionCalls)
	}
	if loadProjectionCalls != 0 {
		t.Fatalf("expected no direct projection load, got %d", loadProjectionCalls)
	}
	if snapshot.SnapshotAsOfSeq != 1 {
		t.Fatalf("expected snapshot watermark 1, got %d", snapshot.SnapshotAsOfSeq)
	}
	if snapshot.Run.State != string(runtime.RunStateRunning) {
		t.Fatalf("expected running projection state, got %q", snapshot.Run.State)
	}
}

func TestReplayRunEventsReturnsEventsAfterCursorAndRejectsInvalidCursor(t *testing.T) {
	fixture := createRichRunFixture(t)

	replay, err := ReplayRunEvents(ReplayRunEventsRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		AfterSeq:    2,
	})
	if err != nil {
		t.Fatalf("expected replay success, got %v", err)
	}
	if replay.AsOfSeq != 13 {
		t.Fatalf("expected replay watermark 13, got %d", replay.AsOfSeq)
	}
	if !replay.Terminal {
		t.Fatal("expected completed fixture replay to be terminal")
	}
	if len(replay.Events) != 11 {
		t.Fatalf("expected 11 replayed events after seq 2, got %d", len(replay.Events))
	}
	if replay.Events[0].Seq != 3 {
		t.Fatalf("expected first replayed seq 3, got %d", replay.Events[0].Seq)
	}

	_, err = ReplayRunEvents(ReplayRunEventsRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		AfterSeq:    99,
	})
	if !errors.Is(err, ErrInvalidResumeCursor) {
		t.Fatalf("expected ErrInvalidResumeCursor, got %v", err)
	}
}

func TestReplayRunEventsUsesOneCanonicalEventScan(t *testing.T) {
	fixture := createRichRunFixture(t)

	previousLoadRunProjection := loadRunProjectionFunc
	previousReadRunEvents := readRunEventsFunc
	previousDeriveRunProjection := deriveRunProjectionFromEvents
	t.Cleanup(func() {
		loadRunProjectionFunc = previousLoadRunProjection
		readRunEventsFunc = previousReadRunEvents
		deriveRunProjectionFromEvents = previousDeriveRunProjection
	})

	loadProjectionCalls := 0
	readEventsCalls := 0
	deriveProjectionCalls := 0
	readRunEventsFunc = func(baseDir string, runID string) ([]runtime.EventEnvelope, error) {
		readEventsCalls++
		return []runtime.EventEnvelope{
			{
				RunID:         runID,
				Seq:           1,
				Type:          runtime.EventTypeRunQueued,
				SchemaVersion: runtime.SchemaVersionV1,
				Timestamp:     time.Unix(1, 0).UTC(),
				Payload: runtime.RunQueuedPayload{
					Source: runtime.RunQueuedSourceCLIRunStart,
				},
			},
			{
				RunID:         runID,
				Seq:           2,
				Type:          runtime.EventTypeRunRunning,
				SchemaVersion: runtime.SchemaVersionV1,
				Timestamp:     time.Unix(2, 0).UTC(),
				Payload: runtime.RunRunningPayload{
					Executor: "test-executor",
					MaxDepth: 2,
				},
			},
		}, nil
	}
	loadRunProjectionFunc = func(baseDir string, runID string) (runtime.RunProjection, error) {
		loadProjectionCalls++
		return runtime.RunProjection{}, nil
	}
	deriveRunProjectionFromEvents = func(baseDir string, runID string, events []runtime.EventEnvelope) (runtime.RunProjection, error) {
		deriveProjectionCalls++
		return runtime.RunProjection{
			RunID:      runID,
			State:      string(runtime.RunStateRunning),
			RunDir:     filepath.Join(baseDir, runID),
			EventsPath: filepath.Join(baseDir, runID, "events.jsonl"),
		}, nil
	}

	replay, err := ReplayRunEvents(ReplayRunEventsRequest{
		RunsBaseDir: fixture.RunsDir,
		RunID:       fixture.RunID,
		AfterSeq:    1,
	})
	if err != nil {
		t.Fatalf("expected replay success, got %v", err)
	}
	if readEventsCalls != 1 {
		t.Fatalf("expected one canonical event read, got %d", readEventsCalls)
	}
	if deriveProjectionCalls != 1 {
		t.Fatalf("expected one projection derivation from events, got %d", deriveProjectionCalls)
	}
	if loadProjectionCalls != 0 {
		t.Fatalf("expected no direct projection load, got %d", loadProjectionCalls)
	}
	if replay.AsOfSeq != 2 {
		t.Fatalf("expected replay watermark 2, got %d", replay.AsOfSeq)
	}
	if len(replay.Events) != 1 || replay.Events[0].Seq != 2 {
		t.Fatalf("expected replayed seq 2, got %+v", replay.Events)
	}
}

func createRichRunFixture(t *testing.T) richRunFixture {
	t.Helper()

	runsDir := t.TempDir()
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir:  runsDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node success, got %v", err)
	}
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected node.step.started success, got %v", err)
	}
	childNode, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		t.Fatalf("expected child node creation success, got %v", err)
	}

	fixture := richRunFixture{
		RunsDir:              runsDir,
		RunID:                lifecycle.RunID(),
		RootNodeID:           rootNode.ID,
		ChildNodeID:          childNode.ID,
		StepID:               stepStarted.StepID,
		UserTurnRef:          fmt.Sprintf("run-artifact://node/%s/step/%s/turn-user.json", rootNode.ID, stepStarted.StepID),
		ModelTurnRef:         fmt.Sprintf("run-artifact://node/%s/step/%s/turn-model.json", rootNode.ID, stepStarted.StepID),
		StepAccountingRef:    fmt.Sprintf("run-artifact://node/%s/step/%s/accounting.json", rootNode.ID, stepStarted.StepID),
		SubcallAccountingRef: fmt.Sprintf("run-artifact://node/%s/step/%s/subcall-1-accounting.json", rootNode.ID, stepStarted.StepID),
		RunAccountingRef:     "run-artifact://run/accounting.json",
		FinalAnswerRef:       fmt.Sprintf("run-artifact://node/%s/final-answer.json", rootNode.ID),
	}
	fixture.ActionRef, err = runtime.BuildActionArtifactRef(rootNode.ID, stepStarted.StepID, 1)
	if err != nil {
		t.Fatalf("expected action ref build success, got %v", err)
	}

	rootNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", rootNode.ID)
	childNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", childNode.ID)

	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.UserTurnRef, map[string]any{
		"run_id":  fixture.RunID,
		"node_id": fixture.RootNodeID,
		"step_id": fixture.StepID,
		"query":   "fixture user turn",
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.ModelTurnRef, map[string]any{
		"run_id":            fixture.RunID,
		"node_id":           fixture.RootNodeID,
		"step_id":           fixture.StepID,
		"schema_id":         "sigil.rlm.response.v1",
		"validated_payload": map[string]any{"decision": "final"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.ActionRef, map[string]any{
		"run_id":       fixture.RunID,
		"node_id":      fixture.RootNodeID,
		"step_id":      fixture.StepID,
		"action_index": 1,
		"action_type":  "repl_code",
		"language":     "go",
		"status":       "completed",
		"stdout":       "fixture-stdout",
		"stderr":       "",
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.SubcallAccountingRef, map[string]any{
		"run_id":        fixture.RunID,
		"node_id":       fixture.RootNodeID,
		"step_id":       fixture.StepID,
		"subcall_index": 1,
		"accounting":    map[string]any{"token_status": "complete"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.StepAccountingRef, map[string]any{
		"run_id":     fixture.RunID,
		"node_id":    fixture.RootNodeID,
		"step_id":    fixture.StepID,
		"accounting": map[string]any{"token_status": "complete"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, rootNodeAccountingRef, map[string]any{
		"run_id":     fixture.RunID,
		"node_id":    fixture.RootNodeID,
		"accounting": map[string]any{"token_status": "complete"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, childNodeAccountingRef, map[string]any{
		"run_id":     fixture.RunID,
		"node_id":    fixture.ChildNodeID,
		"accounting": map[string]any{"token_status": "complete"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.FinalAnswerRef, map[string]any{
		"run_id":       fixture.RunID,
		"node_id":      fixture.RootNodeID,
		"final_answer": "fixture final answer",
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.RunAccountingRef, map[string]any{
		"run_id":     fixture.RunID,
		"accounting": map[string]any{"token_status": "complete"},
	})

	if err := lifecycle.AppendNodeTurn(rootNode.ID, runtime.TurnRoleUser, stepStarted.StepID, fixture.UserTurnRef, nil); err != nil {
		t.Fatalf("expected append node.turn.user success, got %v", err)
	}
	if err := lifecycle.AppendNodeTurn(rootNode.ID, runtime.TurnRoleModel, stepStarted.StepID, fixture.ModelTurnRef, nil); err != nil {
		t.Fatalf("expected append node.turn.model success, got %v", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(childNode.ID, nil, testRollup(), &childNodeAccountingRef); err != nil {
		t.Fatalf("expected child node completion success, got %v", err)
	}
	if err := lifecycle.AppendNodeSubcallExecuted(rootNode.ID, runtime.NodeSubcallExecutedPayload{
		StepID:        stepStarted.StepID,
		ActionIndex:   1,
		SubcallIndex:  1,
		SubcallType:   runtime.SubcallTypeRLMQuery,
		ExecutionMode: runtime.SubcallExecutionModeRecursive,
		Status:        runtime.ActionExecutionStatusCompleted,
		Provider:      "openai",
		Model:         "gpt-5.1",
		PromptBytes:   12,
		ContextBytes:  34,
		AnswerBytes:   56,
		DurationMS:    78,
		ChildNodeID:   &childNode.ID,
		Accounting:    testSummary(),
		AccountingRef: fixture.SubcallAccountingRef,
	}); err != nil {
		t.Fatalf("expected append node.subcall.executed success, got %v", err)
	}
	if err := lifecycle.AppendNodeActionExecuted(rootNode.ID, runtime.NodeActionExecutedPayload{
		StepID:      stepStarted.StepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      runtime.ActionExecutionStatusCompleted,
		DurationMS:  91,
		ActionRef:   fixture.ActionRef,
	}); err != nil {
		t.Fatalf("expected append node.action.executed success, got %v", err)
	}
	if err := lifecycle.AppendNodeStepCompleted(rootNode.ID, runtime.NodeStepCompletedPayload{
		StepID:        stepStarted.StepID,
		Decision:      runtime.StepDecisionContinue,
		ActionCount:   1,
		DurationMS:    120,
		Accounting:    testRollup(),
		AccountingRef: fixture.StepAccountingRef,
	}); err != nil {
		t.Fatalf("expected append node.step.completed success, got %v", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(rootNode.ID, &fixture.FinalAnswerRef, testRollup(), &rootNodeAccountingRef); err != nil {
		t.Fatalf("expected root node completion success, got %v", err)
	}
	if err := lifecycle.CompleteWithAccounting(&fixture.FinalAnswerRef, testRollup(), &fixture.RunAccountingRef); err != nil {
		t.Fatalf("expected run completion success, got %v", err)
	}

	return fixture
}

func writeFixtureArtifact(t *testing.T, runsDir string, runID string, artifactRef string, payload map[string]any) {
	t.Helper()

	relativeParts, err := runtime.ResolveArtifactRefPath(artifactRef)
	if err != nil {
		t.Fatalf("expected artifact ref path resolution success, got %v", err)
	}
	path := filepath.Join(append([]string{runsDir, runID, "artifacts"}, relativeParts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("expected artifact directory creation success, got %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("expected artifact encoding success, got %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("expected artifact write success, got %v", err)
	}
}

func testSummary() accounting.Summary {
	return accounting.UnavailableSummary("openai", "gpt-5.1", "test")
}

func testRollup() accounting.Rollup {
	return accounting.BuildRollup(
		"openai",
		"gpt-5.1",
		"test",
		testSummary(),
		accounting.ZeroSummary("openai", "gpt-5.1", "test"),
	)
}
