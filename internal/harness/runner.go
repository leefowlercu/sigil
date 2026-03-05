package harness

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/openrouter"
	"github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type executionContext struct {
	runConfig    config.RunConfig
	runContext   context.Context
	lifecycle    *runtime.Lifecycle
	inference    InferenceClient
	sessions     *REPLSessionManager
	artifacts    *ActionArtifactStore
	stepExecutor *StepExecutor
	turnOutputs  *TurnOutputStore
	systemPrompt string
	nonRecursive bool
}

type nodeExecutionResult struct {
	answer   string
	finalRef string
}

// Run executes one blocking harness invocation for `sigil run start`.
func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	logger := harnessRunnerLogger()
	runContext := ctx
	if runContext == nil {
		runContext = context.Background()
	}

	if r == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "runner is required", nil)
	}
	if r.templateRenderer == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "template renderer is required", nil)
	}
	if r.promptResolver == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "system prompt resolver is required", nil)
	}
	if r.replFactory == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "repl session factory is required", nil)
	}
	if r.inferenceFactory == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "inference factory is required", nil)
	}

	logger.Info("starting harness run",
		"runs_base_dir", r.runsBaseDir,
		"llm_gateway", input.RunConfig.LLM.Gateway,
		"llm_provider", input.RunConfig.LLM.Provider,
		"llm_model", input.RunConfig.LLM.Model,
		"rlm_enabled", input.RunConfig.RLM.Enabled,
		"rlm_max_depth", input.RunConfig.RLM.MaxDepth,
		"template_var_count", len(input.TemplateVars),
	)

	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir:   r.runsBaseDir,
		QueuedSource:  runtime.RunQueuedSourceCLIRunStart,
		AppConfigPath: cloneOptionalString(input.AppConfigPath),
		RunConfigPath: cloneOptionalString(input.RunConfigPath),
		MaxDepth:      input.RunConfig.RLM.MaxDepth,
	})
	if err != nil {
		logger.Error("failed to initialize run lifecycle", "error", err)
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "failed to initialize run lifecycle", err)
	}
	defer lifecycle.Close()

	eventsPath, err := lifecycle.EventsFilePath()
	if err != nil {
		logger.Error("failed to resolve events path", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "failed to resolve run events path", err)
	}
	logger.Info("initialized run lifecycle",
		"run_id", lifecycle.RunID(),
		"events_path", eventsPath,
	)

	if err := lifecycle.StartExecution(); err != nil {
		logger.Error("failed to start run lifecycle", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "failed to start run lifecycle", err)
	}

	sessions, err := NewREPLSessionManager(r.replFactory)
	if err != nil {
		logger.Error("failed to initialize repl session manager", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to initialize repl session manager", err))
	}
	defer func() {
		if closeErr := sessions.CloseAll(); closeErr != nil {
			logger.Error("failed to close all repl sessions", "run_id", lifecycle.RunID(), "error", closeErr)
		}
	}()

	actionArtifacts, err := NewActionArtifactStore(r.runsBaseDir)
	if err != nil {
		logger.Error("failed to initialize action artifact store", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to initialize action artifact store", err))
	}

	turnOutputs, err := NewTurnOutputStore(r.runsBaseDir)
	if err != nil {
		logger.Error("failed to initialize turn output store", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to initialize turn output store", err))
	}

	stepExecutor, err := NewStepExecutor(lifecycle, sessions, actionArtifacts)
	if err != nil {
		logger.Error("failed to initialize step executor", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to initialize step executor", err))
	}

	inferenceClient, err := r.inferenceFactory(input.RunConfig)
	if err != nil {
		logger.Error("failed to initialize inference service", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to initialize inference service", err))
	}

	_, effectiveSystemPrompt, err := r.promptResolver.ResolveEffective(input.RunConfig.LLM.Provider, input.RunConfig.SystemPromptAppend)
	if err != nil {
		logger.Error("failed to resolve effective system prompt", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to resolve effective system prompt", err))
	}

	effectivePrompt, effectiveContext, err := resolvePromptAndContext(input.RunConfig, input.TemplateVars, r.templateRenderer)
	if err != nil {
		logger.Error("failed to resolve prompt or context templates", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeTemplateRender, "failed to resolve prompt/context", err))
	}

	rootNode, err := lifecycle.RootNode()
	if err != nil {
		logger.Error("failed to resolve root node", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to resolve root node", err))
	}
	logger.Info("starting root node execution",
		"run_id", lifecycle.RunID(),
		"node_id", rootNode.ID,
		"node_depth", rootNode.Depth,
		"effective_prompt_bytes", len(effectivePrompt),
		"effective_context_bytes", len(effectiveContext),
	)

	execCtx := executionContext{
		runConfig:    input.RunConfig,
		runContext:   runContext,
		lifecycle:    lifecycle,
		inference:    inferenceClient,
		sessions:     sessions,
		artifacts:    actionArtifacts,
		stepExecutor: stepExecutor,
		turnOutputs:  turnOutputs,
		systemPrompt: effectiveSystemPrompt,
		nonRecursive: !input.RunConfig.RLM.Enabled,
	}

	rootResult, err := r.executeNode(runContext, &execCtx, rootNode, effectivePrompt, effectiveContext)
	if err != nil {
		failedNodeID := rootNode.ID
		logger.Error("root node execution failed",
			"run_id", lifecycle.RunID(),
			"node_id", failedNodeID,
			"error", err,
		)
		return RunResult{}, failRunningRun(lifecycle, &failedNodeID, err)
	}

	if err := lifecycle.CompleteWith(&rootResult.finalRef); err != nil {
		logger.Error("failed to complete run", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, WrapError(ErrorCodeInfrastructure, "failed to complete run", err))
	}

	logger.Info("harness run completed",
		"run_id", lifecycle.RunID(),
		"state", lifecycle.State(),
		"final_answer_ref", rootResult.finalRef,
		"final_answer_bytes", len(rootResult.answer),
	)

	return RunResult{
		RunID:          lifecycle.RunID(),
		State:          string(lifecycle.State()),
		FinalAnswer:    rootResult.answer,
		FinalAnswerRef: rootResult.finalRef,
		EventsPath:     eventsPath,
	}, nil
}

func (r *Runner) executeNode(ctx context.Context, execCtx *executionContext, node runtime.Node, prompt string, baseContext string) (result nodeExecutionResult, runErr error) {
	if execCtx == nil {
		return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "execution context is required", nil)
	}

	logger := harnessRunnerLogger().With(
		"run_id", execCtx.lifecycle.RunID(),
		"node_id", node.ID,
		"node_depth", node.Depth,
	)
	nodeStart := time.Now().UTC()
	var failedStepID *string
	defer func() {
		if runErr == nil {
			return
		}

		if execCtx.lifecycle.State() == runtime.RunStateRunning {
			if _, terminalized := execCtx.lifecycle.NodeTerminalEvent(node.ID); !terminalized {
				errorCode := ErrorCodeInfrastructure
				if code, ok := CodeOf(runErr); ok {
					errorCode = code
				}

				payload := runtime.NodeFailedPayload{
					Status:       "failed",
					DurationMS:   durationMS(nodeStart),
					ErrorCode:    string(errorCode),
					ErrorMessage: runErr.Error(),
					FailedStepID: cloneOptionalStringPointer(failedStepID),
				}
				if err := execCtx.lifecycle.AppendNodeFailed(node.ID, payload); err != nil {
					runErr = WrapError(ErrorCodeInfrastructure, "failed to append node.failed", err)
				}
			}
		}

		if closeErr := execCtx.sessions.CloseNode(node.ID); closeErr != nil {
			logger.Error("failed to close node repl session", "error", closeErr)
		}
	}()

	logger.Info("executing node",
		"prompt_bytes", len(prompt),
		"initial_context_bytes", len(baseContext),
	)

	contextRef, err := execCtx.turnOutputs.PersistContext(execCtx.lifecycle.RunID(), node.ID, baseContext)
	if err != nil {
		logger.Error("failed to persist node context output", "error", err)
		return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist node context output", err)
	}
	contextMetadata := buildContextMetadata(baseContext, contextRef)
	var previousFeedback *PreviousActionFeedback
	for {
		stepStart := time.Now().UTC()

		stepStarted, err := execCtx.lifecycle.AppendNodeStepStarted(node.ID)
		if err != nil {
			logger.Error("failed to append node.step.started", "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.started", err)
		}
		stepID := stepStarted.StepID
		failedStepID = &stepID
		logger.Debug("started node step",
			"step_id", stepStarted.StepID,
			"step_index", stepStarted.StepIndex,
		)

		envelope, err := buildStepInputEnvelope(prompt, stepStarted.StepIndex, contextMetadata, previousFeedback)
		if err != nil {
			logger.Error("failed to build deterministic step input envelope", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to build deterministic step input envelope", err)
		}

		userMessage, err := encodeStepInputEnvelope(envelope)
		if err != nil {
			logger.Error("failed to serialize deterministic step input envelope", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to serialize deterministic step input envelope", err)
		}

		systemPrompt := strings.TrimSpace(execCtx.systemPrompt)
		if systemPrompt == "" {
			logger.Error("effective system prompt is empty", "step_id", stepStarted.StepID)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "effective system prompt must be non-empty", nil)
		}

		messages := []inference.Message{
			{Role: inference.MessageRoleSystem, Content: systemPrompt},
			{Role: inference.MessageRoleUser, Content: userMessage},
		}

		userTurnRef, err := execCtx.turnOutputs.PersistUserTurn(execCtx.lifecycle.RunID(), node.ID, stepStarted.StepID, envelope, messages)
		if err != nil {
			logger.Error("failed to persist user turn output", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist user turn output", err)
		}
		if err := execCtx.lifecycle.AppendNodeTurn(node.ID, runtime.TurnRoleUser, stepStarted.StepID, userTurnRef); err != nil {
			logger.Error("failed to append node.turn.user", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.turn.user", err)
		}

		inferenceResult, err := execCtx.inference.Infer(ctx, inference.Request{
			Messages: messages,
			SchemaID: schema.SigilRLMResponseV1SchemaID,
			Gateway:  execCtx.runConfig.LLM.Gateway,
			Provider: execCtx.runConfig.LLM.Provider,
			Model:    execCtx.runConfig.LLM.Model,
			GatewayConfig: inference.GatewayConfig{
				BaseURL:          execCtx.runConfig.LLM.OpenRouter.BaseURL,
				RequestTimeoutMS: execCtx.runConfig.LLM.OpenRouter.RequestTimeoutMS,
				APIKeyEnv:        execCtx.runConfig.LLM.OpenRouter.APIKeyEnv,
			},
			Reasoning: inference.ReasoningConfig{
				Enabled: execCtx.runConfig.LLM.Reasoning.Enabled,
				Effort:  execCtx.runConfig.LLM.Reasoning.Effort,
			},
		})
		if err != nil {
			logger.Error("inference execution failed",
				"step_id", stepStarted.StepID,
				"error", err,
			)
			return nodeExecutionResult{}, WrapError(ErrorCodeInference, "inference execution failed", err)
		}

		modelTurnRef, err := execCtx.turnOutputs.PersistModelTurn(execCtx.lifecycle.RunID(), node.ID, stepStarted.StepID, inferenceResult)
		if err != nil {
			logger.Error("failed to persist model turn output", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist model turn output", err)
		}
		if err := execCtx.lifecycle.AppendNodeTurn(node.ID, runtime.TurnRoleModel, stepStarted.StepID, modelTurnRef); err != nil {
			logger.Error("failed to append node.turn.model", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.turn.model", err)
		}

		decisionPayload, err := parseDecisionPayload(inferenceResult.ValidatedPayload)
		if err != nil {
			logger.Error("inference decision payload validation failed", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "inference payload failed decision validation", err)
		}
		logger.Info("model decision resolved",
			"step_id", stepStarted.StepID,
			"step_index", stepStarted.StepIndex,
			"decision", decisionPayload.Decision,
		)

		if decisionPayload.Decision == runtime.StepDecisionContinue {
			if decisionPayload.Continuation == nil {
				logger.Error("continuation payload is missing", "step_id", stepStarted.StepID)
				return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "continuation payload is required for continue decision", nil)
			}
			subcalls, subcallErr := NewSubcallRouter(SubcallRouterInput{
				Lifecycle:    execCtx.lifecycle,
				Inference:    execCtx.inference,
				RunConfig:    execCtx.runConfig,
				Node:         node,
				StepID:       stepStarted.StepID,
				ActionIndex:  1,
				NonRecursive: execCtx.nonRecursive,
				ExecuteChild: func(queryCtx context.Context, child runtime.Node, subPrompt string, subContext string) (nodeExecutionResult, error) {
					logger.Info("executing recursive child node subcall",
						"step_id", stepStarted.StepID,
						"child_node_id", child.ID,
						"child_depth", child.Depth,
						"query_prompt_bytes", len(subPrompt),
						"query_context_bytes", len(subContext),
					)
					return r.executeNode(queryCtx, execCtx, child, subPrompt, subContext)
				},
			})
			if subcallErr != nil {
				logger.Error("failed to initialize subcall router", "step_id", stepStarted.StepID, "error", subcallErr)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to initialize subcall router", subcallErr)
			}

			actionPayload, actionErr := execCtx.stepExecutor.ExecuteContinueAction(ctx, ContinueActionInput{
				NodeID:     node.ID,
				StepID:     stepStarted.StepID,
				Code:       decisionPayload.Continuation.ReplCode,
				Context:    baseContext,
				RunContext: execCtx.runContext,
				Subcalls:   subcalls,
				Collector:  subcalls,
			})
			if actionErr != nil {
				logger.Error("failed to execute continue action",
					"step_id", stepStarted.StepID,
					"error", actionErr,
				)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to execute continue action", actionErr)
			}
			if actionPayload.Status == runtime.ActionExecutionStatusFailed {
				logger.Warn("continue action recorded non-fatal failure",
					"step_id", stepStarted.StepID,
					"action_index", actionPayload.ActionIndex,
					"error_code", valueOrEmpty(actionPayload.ErrorCode),
					"output_ref", actionPayload.OutputRef,
				)
			} else {
				logger.Info("continue action completed",
					"step_id", stepStarted.StepID,
					"action_index", actionPayload.ActionIndex,
					"output_ref", actionPayload.OutputRef,
				)
			}

			if err := execCtx.lifecycle.AppendNodeStepCompleted(node.ID, runtime.NodeStepCompletedPayload{
				StepID:      stepStarted.StepID,
				Decision:    runtime.StepDecisionContinue,
				ActionCount: 1,
				DurationMS:  durationMS(stepStart),
			}); err != nil {
				logger.Error("failed to append node.step.completed for continue decision",
					"step_id", stepStarted.StepID,
					"error", err,
				)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.completed", err)
			}
			logger.Debug("completed continue step",
				"step_id", stepStarted.StepID,
				"duration_ms", durationMS(stepStart),
			)

			feedback, feedbackErr := buildPreviousActionFeedback(execCtx.lifecycle.RunID(), execCtx.artifacts, actionPayload)
			if feedbackErr != nil {
				logger.Error("failed to build repl feedback", "step_id", stepStarted.StepID, "error", feedbackErr)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to build repl feedback", feedbackErr)
			}
			previousFeedback = feedback
			continue
		}

		if err := execCtx.lifecycle.AppendNodeStepCompleted(node.ID, runtime.NodeStepCompletedPayload{
			StepID:      stepStarted.StepID,
			Decision:    runtime.StepDecisionFinal,
			ActionCount: 0,
			DurationMS:  durationMS(stepStart),
		}); err != nil {
			logger.Error("failed to append node.step.completed for final decision",
				"step_id", stepStarted.StepID,
				"error", err,
			)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.completed", err)
		}

		if decisionPayload.Final == nil {
			logger.Error("final payload is missing", "step_id", stepStarted.StepID)
			return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "final payload is required for final decision", nil)
		}

		if err := resolveFinalEvidenceRefs(execCtx.lifecycle.RunID(), execCtx.turnOutputs.runsBaseDir, execCtx.artifacts, decisionPayload.Final.Evidence); err != nil {
			logger.Error("failed to resolve final evidence refs", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "final evidence resolution failed", err)
		}

		finalRef, err := execCtx.turnOutputs.PersistFinalAnswer(
			execCtx.lifecycle.RunID(),
			node.ID,
			decisionPayload.Final.Answer,
			decisionPayload.Final.Evidence,
			decisionPayload.Final.Confidence,
		)
		if err != nil {
			logger.Error("failed to persist final answer output", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist final answer output", err)
		}
		if err := execCtx.lifecycle.CompleteNode(node.ID, &finalRef); err != nil {
			logger.Error("failed to persist node.completed event", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to complete node", err)
		}
		if err := execCtx.sessions.CloseNode(node.ID); err != nil {
			logger.Error("failed to close node repl session", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to close node repl session", err)
		}
		failedStepID = nil
		logger.Info("node reached final decision",
			"step_id", stepStarted.StepID,
			"duration_ms", durationMS(stepStart),
			"final_answer_ref", finalRef,
			"final_answer_bytes", len(decisionPayload.Final.Answer),
		)

		return nodeExecutionResult{answer: decisionPayload.Final.Answer, finalRef: finalRef}, nil
	}
}

func durationMS(start time.Time) int {
	delta := int(time.Since(start).Milliseconds())
	if delta < 0 {
		return 0
	}

	return delta
}

func cloneOptionalString(raw string) *string {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return nil
	}

	value := raw
	return &value
}

func cloneOptionalStringPointer(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func harnessRunnerLogger() *slog.Logger {
	return slog.Default().With("component", "harness.runner")
}

func failRunningRun(lifecycle *runtime.Lifecycle, failedNodeID *string, runErr error) error {
	if lifecycle == nil || runErr == nil {
		return runErr
	}
	if lifecycle.State() != runtime.RunStateRunning {
		return runErr
	}

	errorCode := ErrorCodeInfrastructure
	if code, ok := CodeOf(runErr); ok {
		errorCode = code
	}

	payload := runtime.RunFailedPayload{
		Status:       "failed",
		ErrorCode:    string(errorCode),
		ErrorMessage: runErr.Error(),
		FailedNodeID: failedNodeID,
		Retryable:    false,
	}
	if err := lifecycle.FailWith(payload); err != nil {
		harnessRunnerLogger().Error("failed to persist run.failed event",
			"run_id", lifecycle.RunID(),
			"error_code", payload.ErrorCode,
			"error", err,
		)
		return fmt.Errorf("%w; additionally failed to persist run.failed event: %v", runErr, err)
	}

	harnessRunnerLogger().Error("run marked failed",
		"run_id", lifecycle.RunID(),
		"error_code", payload.ErrorCode,
		"failed_node_id", valueOrEmpty(failedNodeID),
		"error", runErr,
	)

	return runErr
}

func newDefaultInferenceClient(_ config.RunConfig) (InferenceClient, error) {
	registry := inference.NewRegistry()
	if err := registry.Register("openrouter", openrouter.NewGateway); err != nil {
		return nil, fmt.Errorf("failed to register openrouter gateway; %w", err)
	}

	service, err := inference.NewService(registry, schema.NewRegistry())
	if err != nil {
		return nil, fmt.Errorf("failed to construct inference service; %w", err)
	}

	return service, nil
}
