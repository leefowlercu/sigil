package harness

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type stepExecFakeSessionFactory struct {
	session repl.Session
	err     error
}

func (f *stepExecFakeSessionFactory) NewSession(_ context.Context, _ repl.SessionOptions) (repl.Session, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.session, nil
}

type stepExecFakeSession struct {
	result repl.ExecResult
	err    error
}

func (s *stepExecFakeSession) Exec(_ context.Context, _ string) (repl.ExecResult, error) {
	return s.result, s.err
}

func (s *stepExecFakeSession) Close() error {
	return nil
}

type stepExecNoopSubcalls struct{}

func (stepExecNoopSubcalls) LLMQuery(_ context.Context, _ repl.QueryRequest) (string, error) {
	return "", nil
}

func (stepExecNoopSubcalls) RLMQuery(_ context.Context, _ repl.QueryRequest) (string, error) {
	return "", nil
}

func (stepExecNoopSubcalls) LLMQueryBatched(_ context.Context, _ []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	return nil, nil
}

func (stepExecNoopSubcalls) RLMQueryBatched(_ context.Context, _ []repl.BatchedQueryRequest) ([]repl.BatchedQueryResult, error) {
	return nil, nil
}

func TestStepExecutorExecuteContinueActionRecordsCompletedAction(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{RunsBaseDir: runsDir, MaxDepth: 3})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() { _ = lifecycle.Close() })

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node lookup success, got %v", err)
	}

	step, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected step append success, got %v", err)
	}

	artifacts, err := NewActionArtifactStore(runsDir)
	if err != nil {
		t.Fatalf("expected artifact store creation success, got %v", err)
	}
	manager, err := NewREPLSessionManager(&stepExecFakeSessionFactory{session: &stepExecFakeSession{result: repl.ExecResult{Stdout: "ok", DurationMS: 1}}}, artifacts)
	if err != nil {
		t.Fatalf("expected manager creation success, got %v", err)
	}
	executor, err := NewStepExecutor(lifecycle, manager, artifacts)
	if err != nil {
		t.Fatalf("expected step executor creation success, got %v", err)
	}

	payload, err := executor.ExecuteContinueAction(context.Background(), ContinueActionInput{
		NodeID:   rootNode.ID,
		StepID:   step.StepID,
		Code:     `import "fmt"; fmt.Print("ok")`,
		Context:  "context",
		Subcalls: stepExecNoopSubcalls{},
	})
	if err != nil {
		t.Fatalf("expected action execution success, got %v", err)
	}
	if payload.Status != runtime.ActionExecutionStatusCompleted {
		t.Fatalf("expected completed status, got %q", payload.Status)
	}

	events, err := lifecycle.PersistedEvents()
	if err != nil {
		t.Fatalf("expected persisted event read success, got %v", err)
	}
	last := events[len(events)-1]
	if last.Type != runtime.EventTypeNodeActionExecuted {
		t.Fatalf("expected last event type node.action.executed, got %q", last.Type)
	}
}

func TestStepExecutorExecuteContinueActionRecordsFailedActionWithoutFatalError(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{RunsBaseDir: runsDir, MaxDepth: 3})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() { _ = lifecycle.Close() })

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node lookup success, got %v", err)
	}

	step, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected step append success, got %v", err)
	}

	execErr := repl.NewError(repl.ErrorCodeExecutionCompile, "compile failed")
	artifacts, err := NewActionArtifactStore(runsDir)
	if err != nil {
		t.Fatalf("expected artifact store creation success, got %v", err)
	}
	manager, err := NewREPLSessionManager(&stepExecFakeSessionFactory{session: &stepExecFakeSession{result: repl.ExecResult{Stderr: "compile failed", DurationMS: 1}, err: execErr}}, artifacts)
	if err != nil {
		t.Fatalf("expected manager creation success, got %v", err)
	}
	executor, err := NewStepExecutor(lifecycle, manager, artifacts)
	if err != nil {
		t.Fatalf("expected step executor creation success, got %v", err)
	}

	payload, err := executor.ExecuteContinueAction(context.Background(), ContinueActionInput{
		NodeID:   rootNode.ID,
		StepID:   step.StepID,
		Code:     `broken code`,
		Context:  "context",
		Subcalls: stepExecNoopSubcalls{},
	})
	if err != nil {
		t.Fatalf("expected non-fatal action handling, got %v", err)
	}
	if payload.Status != runtime.ActionExecutionStatusFailed {
		t.Fatalf("expected failed status, got %q", payload.Status)
	}
	if payload.ErrorCode == nil || *payload.ErrorCode != string(repl.ErrorCodeExecutionCompile) {
		t.Fatalf("expected compile error code, got %v", payload.ErrorCode)
	}

	artifact, err := artifacts.Read(lifecycle.RunID(), payload.OutputRef)
	if err != nil {
		t.Fatalf("expected artifact read success, got %v", err)
	}
	if artifact.ErrorDetail == nil {
		t.Fatal("expected compile error_detail in failed artifact")
	}
	if artifact.ErrorDetail.Stage != "compile" {
		t.Fatalf("expected compile error_detail stage, got %q", artifact.ErrorDetail.Stage)
	}
}

func TestStepExecutorExecuteContinueActionReturnsFatalErrorOnSessionInitFailure(t *testing.T) {
	runsDir := filepath.Join(t.TempDir(), "sigil-runs")
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{RunsBaseDir: runsDir, MaxDepth: 3})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	t.Cleanup(func() { _ = lifecycle.Close() })

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node lookup success, got %v", err)
	}

	step, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected step append success, got %v", err)
	}

	expectedErr := errors.New("session init failed")
	artifacts, err := NewActionArtifactStore(runsDir)
	if err != nil {
		t.Fatalf("expected artifact store creation success, got %v", err)
	}
	manager, err := NewREPLSessionManager(&stepExecFakeSessionFactory{err: expectedErr}, artifacts)
	if err != nil {
		t.Fatalf("expected manager creation success, got %v", err)
	}
	executor, err := NewStepExecutor(lifecycle, manager, artifacts)
	if err != nil {
		t.Fatalf("expected step executor creation success, got %v", err)
	}

	_, err = executor.ExecuteContinueAction(context.Background(), ContinueActionInput{
		NodeID:   rootNode.ID,
		StepID:   step.StepID,
		Code:     `broken code`,
		Context:  "context",
		Subcalls: stepExecNoopSubcalls{},
	})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected session init failure, got %v", err)
	}
}
