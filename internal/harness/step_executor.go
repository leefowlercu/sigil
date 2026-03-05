package harness

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/leefowlercu/sigil/internal/repl"
	"github.com/leefowlercu/sigil/internal/runtime"
)

// ContinueActionInput defines one continue action execution request.
type ContinueActionInput struct {
	NodeID     string
	StepID     string
	Code       string
	Context    string
	RunContext context.Context
	Subcalls   SubcallBindings
	Collector  *SubcallRouter
}

// StepExecutor executes continue actions in node-local REPL sessions.
type StepExecutor struct {
	lifecycle *runtime.Lifecycle
	sessions  *REPLSessionManager
	artifacts *ActionArtifactStore
}

// NewStepExecutor constructs StepExecutor.
func NewStepExecutor(lifecycle *runtime.Lifecycle, sessions *REPLSessionManager, artifacts *ActionArtifactStore) (*StepExecutor, error) {
	if lifecycle == nil {
		return nil, fmt.Errorf("lifecycle is required")
	}
	if sessions == nil {
		return nil, fmt.Errorf("session manager is required")
	}
	if artifacts == nil {
		return nil, fmt.Errorf("artifact store is required")
	}

	return &StepExecutor{
		lifecycle: lifecycle,
		sessions:  sessions,
		artifacts: artifacts,
	}, nil
}

// ExecuteContinueAction executes one continuation.repl_code action.
//
// Returns fatal infrastructure errors only. Non-fatal execution errors are
// represented in returned payload with status=failed and typed error metadata.
func (e *StepExecutor) ExecuteContinueAction(ctx context.Context, input ContinueActionInput) (runtime.NodeActionExecutedPayload, error) {
	logger := stepExecutorLogger().With(
		"run_id", e.lifecycle.RunID(),
		"node_id", input.NodeID,
		"step_id", input.StepID,
		"action_index", 1,
	)
	logger.Debug("executing continue action",
		"language", "go",
		"code_bytes", len(input.Code),
	)

	node, err := e.lifecycle.NodeByID(input.NodeID)
	if err != nil {
		logger.Error("failed to resolve node for continue action", "error", err)
		return runtime.NodeActionExecutedPayload{}, err
	}

	session, err := e.sessions.SessionForNode(ctx, NodeSessionInput{
		RunID:      e.lifecycle.RunID(),
		NodeID:     node.ID,
		Depth:      node.Depth,
		Context:    input.Context,
		RunContext: input.RunContext,
		Bindings:   input.Subcalls,
	})
	if err != nil {
		logger.Error("failed to initialize node repl session", "error", err)
		return runtime.NodeActionExecutedPayload{}, fmt.Errorf("failed to initialize node REPL session; %w", err)
	}

	execResult, execErr := session.Exec(ctx, input.Code)
	payload := runtime.NodeActionExecutedPayload{
		StepID:      input.StepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      runtime.ActionExecutionStatusCompleted,
		DurationMS:  execResult.DurationMS,
	}

	if input.Collector != nil {
		if fatalErr := input.Collector.FatalError(); fatalErr != nil {
			logger.Error("subcall event persistence failed", "error", fatalErr)
			return runtime.NodeActionExecutedPayload{}, fatalErr
		}
	}

	artifact := ActionArtifact{
		RunID:       e.lifecycle.RunID(),
		NodeID:      node.ID,
		StepID:      input.StepID,
		ActionIndex: 1,
		ActionType:  payload.ActionType,
		Language:    payload.Language,
		Status:      string(payload.Status),
		Code:        input.Code,
		Stdout:      execResult.Stdout,
		Stderr:      execResult.Stderr,
		DurationMS:  execResult.DurationMS,
	}
	if input.Collector != nil {
		artifact.Subcalls = input.Collector.Traces()
	}

	if execErr != nil {
		payload.Status = runtime.ActionExecutionStatusFailed
		errorCode := string(repl.ErrorCodeExecutionRuntime)
		if code, ok := repl.CodeOf(execErr); ok {
			errorCode = string(code)
		}
		errorMessage := execErr.Error()
		payload.ErrorCode = &errorCode
		payload.ErrorMessage = &errorMessage
		artifact.Status = string(payload.Status)
		artifact.ErrorCode = &errorCode
		artifact.ErrorMessage = &errorMessage
		if detail := compileErrorDetail(execErr, execResult.Stderr); detail != nil {
			artifact.ErrorDetail = detail
		}
		logger.Warn("continue action execution returned non-fatal failure",
			"error_code", errorCode,
			"error_message", errorMessage,
			"duration_ms", execResult.DurationMS,
		)
	}

	outputRef, err := e.artifacts.Persist(artifact)
	if err != nil {
		logger.Error("failed to persist action artifact", "error", err)
		return runtime.NodeActionExecutedPayload{}, fmt.Errorf("failed to persist action artifact; %w", err)
	}
	payload.OutputRef = outputRef

	if err := e.lifecycle.AppendNodeActionExecuted(node.ID, payload); err != nil {
		logger.Error("failed to append node.action.executed", "error", err)
		return runtime.NodeActionExecutedPayload{}, err
	}

	logger.Info("continue action recorded",
		"status", payload.Status,
		"duration_ms", payload.DurationMS,
		"output_ref", payload.OutputRef,
	)

	return payload, nil
}

func stepExecutorLogger() *slog.Logger {
	return slog.Default().With("component", "harness.step_executor")
}
