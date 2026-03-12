package runtime

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
)

func TestListRunsReturnsEmptyWhenBaseDirMissing(t *testing.T) {
	summaries, err := ListRuns(filepath.Join(t.TempDir(), "missing-runs"))
	if err != nil {
		t.Fatalf("expected empty list success, got %v", err)
	}
	if len(summaries) != 0 {
		t.Fatalf("expected zero summaries, got %d", len(summaries))
	}
}

func TestLoadRunProjectionIncludesControlMetadata(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	processMetadata := mustCreateLiveProcessMetadata(t)

	var err error
	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir:     runsBaseDir,
		QueuedSource:    RunQueuedSourceCLIRunStart,
		MaxDepth:        3,
		ProcessMetadata: &processMetadata,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected lifecycle start success, got %v", err)
	}
	if err := WriteStopRequestMetadata(runsBaseDir, StopRequestMetadata{
		RunID:       lifecycle.RunID(),
		RequestedAt: lifecycle.RunStartedAt(),
		RequestedBy: StopRequesterCLIRunStop,
		Signal:      StopSignalSIGTERM,
	}); err != nil {
		t.Fatalf("expected stop request write success, got %v", err)
	}
	if err := lifecycle.Interrupt(); err != nil {
		t.Fatalf("expected lifecycle interrupt success, got %v", err)
	}

	projection, err := LoadRunProjection(runsBaseDir, lifecycle.RunID())
	if err != nil {
		t.Fatalf("expected projection success, got %v", err)
	}

	if projection.RunID != lifecycle.RunID() {
		t.Fatalf("expected run_id %q, got %q", lifecycle.RunID(), projection.RunID)
	}
	if projection.State != string(RunStateInterrupted) {
		t.Fatalf("expected interrupted state, got %q", projection.State)
	}
	if projection.Source != string(RunQueuedSourceCLIRunStart) {
		t.Fatalf("expected source %q, got %q", RunQueuedSourceCLIRunStart, projection.Source)
	}
	if projection.PIDStatus != RunPIDStatusCurrent {
		t.Fatalf("expected pid_status %q, got %q", RunPIDStatusCurrent, projection.PIDStatus)
	}
	if !projection.StopRequested {
		t.Fatal("expected stop_requested=true")
	}
	if projection.ProcessMetadata == nil {
		t.Fatal("expected process metadata to be populated")
	}
	if projection.StopRequest == nil {
		t.Fatal("expected stop request metadata to be populated")
	}
	if projection.NodeCount != 1 {
		t.Fatalf("expected one projected node, got %d", projection.NodeCount)
	}
	if len(projection.Nodes) != 1 {
		t.Fatalf("expected one projected node entry, got %d", len(projection.Nodes))
	}
	if projection.Nodes[0].Depth != 0 {
		t.Fatalf("expected root node depth 0, got %d", projection.Nodes[0].Depth)
	}
}

func TestLoadRunProjectionIncludesConfiguredPathsAndDerivedCounts(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	appPath := "./sigil.yaml"
	runPath := "./sigil-run.yaml"

	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir:   runsBaseDir,
		QueuedSource:  RunQueuedSourceCLIRunStart,
		AppConfigPath: &appPath,
		RunConfigPath: &runPath,
		MaxDepth:      5,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected lifecycle start success, got %v", err)
	}

	rootNode := lifecycle.Nodes()[0]
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected step started append success, got %v", err)
	}
	if err := lifecycle.AppendNodeSubcallExecuted(rootNode.ID, NodeSubcallExecutedPayload{
		StepID:        stepStarted.StepID,
		ActionIndex:   1,
		SubcallIndex:  1,
		SubcallType:   SubcallTypeLLMQuery,
		ExecutionMode: SubcallExecutionModePlain,
		Status:        ActionExecutionStatusCompleted,
		Provider:      "openai",
		Model:         "gpt-5.1",
		PromptBytes:   3,
		ContextBytes:  5,
		AnswerBytes:   7,
		DurationMS:    11,
		Accounting:    accounting.UnavailableSummary("openai", "gpt-5.1", ""),
		AccountingRef: "run-output://node/root/subcall-accounting.json",
	}); err != nil {
		t.Fatalf("expected subcall append success, got %v", err)
	}
	outputRef, err := BuildActionOutputRef(rootNode.ID, stepStarted.StepID, 1)
	if err != nil {
		t.Fatalf("expected action output ref build success, got %v", err)
	}
	if err := lifecycle.AppendNodeActionExecuted(rootNode.ID, NodeActionExecutedPayload{
		StepID:      stepStarted.StepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      ActionExecutionStatusCompleted,
		DurationMS:  13,
		OutputRef:   outputRef,
	}); err != nil {
		t.Fatalf("expected action append success, got %v", err)
	}
	if err := lifecycle.CompleteNode(rootNode.ID, nil); err != nil {
		t.Fatalf("expected node completion success, got %v", err)
	}
	if err := lifecycle.Complete(); err != nil {
		t.Fatalf("expected lifecycle completion success, got %v", err)
	}

	projection, err := LoadRunProjection(runsBaseDir, lifecycle.RunID())
	if err != nil {
		t.Fatalf("expected projection success, got %v", err)
	}

	if projection.AppConfigPath == nil || *projection.AppConfigPath != appPath {
		t.Fatalf("expected app_config_path %q, got %+v", appPath, projection.AppConfigPath)
	}
	if projection.RunConfigPath == nil || *projection.RunConfigPath != runPath {
		t.Fatalf("expected run_config_path %q, got %+v", runPath, projection.RunConfigPath)
	}
	if projection.Executor != "rlm" {
		t.Fatalf("expected executor %q, got %q", "rlm", projection.Executor)
	}
	if projection.MaxDepth != 5 {
		t.Fatalf("expected max_depth 5, got %d", projection.MaxDepth)
	}
	if projection.ActionCount != 1 {
		t.Fatalf("expected action_count 1, got %d", projection.ActionCount)
	}
	if projection.SubcallCount != 1 {
		t.Fatalf("expected subcall_count 1, got %d", projection.SubcallCount)
	}
}

func TestLoadRunProjectionIgnoresInvalidAuxiliaryMetadata(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustCreateCompletedProjectionRun(t, runsBaseDir)

	processPath, err := ProcessMetadataPath(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected process metadata path success, got %v", err)
	}
	if err := os.WriteFile(processPath, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatalf("expected corrupt process metadata write success, got %v", err)
	}
	stopRequestPath, err := StopRequestPath(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected stop request path success, got %v", err)
	}
	if err := os.WriteFile(stopRequestPath, []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatalf("expected corrupt stop request write success, got %v", err)
	}

	projection, err := LoadRunProjection(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected projection success with corrupt auxiliary metadata, got %v", err)
	}
	if projection.PIDStatus != RunPIDStatusMissing {
		t.Fatalf("expected pid_status %q when auxiliary process metadata is invalid, got %q", RunPIDStatusMissing, projection.PIDStatus)
	}
	if projection.StopRequested {
		t.Fatal("expected stop_requested=false when auxiliary stop-request metadata is invalid")
	}
}

func TestLoadRunProjectionIgnoresUnreadableProcessMetadata(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustCreateCompletedProjectionRun(t, runsBaseDir)

	processPath, err := ProcessMetadataPath(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected process metadata path success, got %v", err)
	}
	if err := os.MkdirAll(processPath, 0o755); err != nil {
		t.Fatalf("expected process metadata directory creation success, got %v", err)
	}

	projection, err := LoadRunProjection(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected projection success with unreadable process metadata, got %v", err)
	}
	if projection.PIDStatus != RunPIDStatusMissing {
		t.Fatalf("expected pid_status %q when process metadata is unreadable, got %q", RunPIDStatusMissing, projection.PIDStatus)
	}
	if projection.ProcessMetadata != nil {
		t.Fatalf("expected process metadata to be omitted, got %+v", projection.ProcessMetadata)
	}
}

func TestLoadRunProjectionIgnoresUnreadableStopRequestMetadata(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := mustCreateCompletedProjectionRun(t, runsBaseDir)

	stopRequestPath, err := StopRequestPath(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected stop request path success, got %v", err)
	}
	if err := os.MkdirAll(stopRequestPath, 0o755); err != nil {
		t.Fatalf("expected stop request directory creation success, got %v", err)
	}

	projection, err := LoadRunProjection(runsBaseDir, runID)
	if err != nil {
		t.Fatalf("expected projection success with unreadable stop request metadata, got %v", err)
	}
	if projection.StopRequested {
		t.Fatal("expected stop_requested=false when stop-request metadata is unreadable")
	}
	if projection.StopRequest != nil {
		t.Fatalf("expected stop request metadata to be omitted, got %+v", projection.StopRequest)
	}
}

func TestListRunsIncludesErrorSummaryForCorruptRun(t *testing.T) {
	runsBaseDir := filepath.Join(t.TempDir(), "sigil-runs")
	runID := testRunID(t)
	runDir := filepath.Join(runsBaseDir, runID)
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatalf("expected run dir creation success, got %v", err)
	}
	if err := os.WriteFile(filepath.Join(runDir, eventsFileName), []byte("{not-json}\n"), 0o644); err != nil {
		t.Fatalf("expected corrupt events write success, got %v", err)
	}

	summaries, err := ListRuns(runsBaseDir)
	if err != nil {
		t.Fatalf("expected best-effort list success, got %v", err)
	}
	if len(summaries) != 1 {
		t.Fatalf("expected one summary, got %d", len(summaries))
	}
	if summaries[0].RunID != runID {
		t.Fatalf("expected run_id %q, got %q", runID, summaries[0].RunID)
	}
	if summaries[0].State != RunStateUnknown {
		t.Fatalf("expected state %q, got %q", RunStateUnknown, summaries[0].State)
	}
	if summaries[0].Error == "" {
		t.Fatal("expected corrupt run summary to include an error")
	}
}

func TestReadRunEventsRejectsUnknownRun(t *testing.T) {
	_, err := ReadRunEvents(filepath.Join(t.TempDir(), "sigil-runs"), testRunID(t))
	if err == nil {
		t.Fatal("expected missing run to fail")
	}
	if !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected os.ErrNotExist, got %v", err)
	}
}

func mustCreateCompletedProjectionRun(t *testing.T, runsBaseDir string) string {
	t.Helper()

	lifecycle, err := NewLifecycleWithOptions(LifecycleOptions{
		RunsBaseDir: runsBaseDir,
		MaxDepth:    3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() {
		_ = lifecycle.Close()
	})
	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected lifecycle start success, got %v", err)
	}
	if err := lifecycle.Complete(); err != nil {
		t.Fatalf("expected lifecycle complete success, got %v", err)
	}

	return lifecycle.RunID()
}

func mustCreateLiveProcessMetadata(t *testing.T) ProcessMetadata {
	t.Helper()

	cmd := exec.Command("sleep", "30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("expected helper process start success, got %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		_ = cmd.Wait()
	})

	startedAt, err := readProcessStartedAt(cmd.Process.Pid)
	if err != nil {
		t.Fatalf("expected helper process start time resolution success, got %v", err)
	}

	return ProcessMetadata{
		PID:        cmd.Process.Pid,
		RecordedAt: time.Now().UTC(),
		StartedAt:  startedAt,
		Source:     RunSourceCLIRunStart,
	}
}
