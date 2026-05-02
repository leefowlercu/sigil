package harness

import (
	"context"
	"fmt"
	"log/slog"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/inference/openrouter"
	"github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	previousStepFeedbackCodeRecursivePartitionMapRequired = "recursive_partition_map_required"
	previousStepFeedbackCodePrematureFinalRejected        = "premature_final_rejected"
	previousStepFeedbackCodeRecursiveMapReducerRequired   = "recursive_map_reducer_required"
	previousStepFeedbackCodeRecursiveReducerFinalRejected = "recursive_reducer_final_rejected"
)

type executionContext struct {
	runConfig    config.RunConfig
	runContext   context.Context
	lifecycle    *runtime.Lifecycle
	inference    InferenceClient
	sessions     *REPLSessionManager
	artifacts    *ActionArtifactStore
	stepExecutor *StepExecutor
	runArtifacts *RunArtifactStore
	ledger       *accounting.Ledger
	systemPrompt string
	nonRecursive bool
	guardrails   *deterministicGuardrails
}

type nodeExecutionResult struct {
	answer   string
	finalRef string
}

type acceptedRun struct {
	input                 RunInput
	guardrailRunContext   context.Context
	cancel                context.CancelCauseFunc
	cancelDeadline        context.CancelFunc
	lifecycle             *runtime.Lifecycle
	eventsPath            string
	guardrails            *deterministicGuardrails
	submittedRunConfigRef *string
}

// Run executes one blocking harness invocation for `sigil run start`.
func (r *Runner) Run(ctx context.Context, input RunInput) (RunResult, error) {
	accepted, err := r.acceptRun(ctx, input)
	if err != nil {
		return RunResult{}, err
	}
	return r.executeAcceptedRun(accepted)
}

// StartAsync accepts one durable run and continues execution in the background.
func (r *Runner) StartAsync(ctx context.Context, input RunInput) (*StartedRun, error) {
	accepted, err := r.acceptRun(ctx, input)
	if err != nil {
		return nil, err
	}

	doneCh := make(chan RunCompletion, 1)
	launchCh := make(chan struct{})
	started := &StartedRun{
		RunID:                 accepted.lifecycle.RunID(),
		EventsPath:            accepted.eventsPath,
		AsOfSeq:               1,
		SubmittedRunConfigRef: cloneOptionalStringPointer(accepted.submittedRunConfigRef),
		done:                  doneCh,
		cancel:                accepted.cancel,
		launch: func() {
			close(launchCh)
		},
	}

	go func() {
		select {
		case <-launchCh:
		case <-accepted.guardrailRunContext.Done():
		}
		result, runErr := r.executeAcceptedRun(accepted)
		doneCh <- RunCompletion{Result: result, Err: runErr}
		close(doneCh)
	}()

	return started, nil
}

func (r *Runner) acceptRun(ctx context.Context, input RunInput) (_ *acceptedRun, err error) {
	logger := harnessRunnerLogger()
	baseRunContext := ctx
	if baseRunContext == nil {
		baseRunContext = context.Background()
	}

	if err := r.validateDependencies(); err != nil {
		return nil, err
	}

	runStart := time.Now().UTC()
	guardrails, err := newDeterministicGuardrails(input.RunConfig.Guardrails, runStart)
	if err != nil {
		logger.Error("failed to initialize deterministic guardrails", "error", err)
		return nil, WrapError(ErrorCodeInfrastructure, "failed to initialize deterministic guardrails", err)
	}

	runContext, cancel := context.WithCancelCause(baseRunContext)
	guardrailRunContext := runContext
	var cancelDeadline context.CancelFunc
	if deadline := guardrails.Deadline(); !deadline.IsZero() {
		guardrailRunContext, cancelDeadline = context.WithDeadline(runContext, deadline)
	}

	queuedSource := resolveQueuedSource(input)
	processMetadataSource := resolveProcessMetadataSource(input, queuedSource)

	defer func() {
		if err == nil {
			return
		}
		if cancelDeadline != nil {
			cancelDeadline()
		}
		cancel(nil)
	}()

	logger.Info("starting harness run",
		"runs_base_dir", r.runsBaseDir,
		"queued_source", queuedSource,
		"llm_gateway", input.RunConfig.LLM.Gateway,
		"llm_provider", input.RunConfig.LLM.Provider,
		"llm_model", input.RunConfig.LLM.Model,
		"rlm_enabled", input.RunConfig.RLM.Enabled,
		"rlm_max_depth", input.RunConfig.RLM.MaxDepth,
		"template_var_count", len(input.TemplateVars),
	)

	processMetadata, err := runtime.CurrentProcessMetadata(processMetadataSource)
	if err != nil {
		logger.Error("failed to capture current process metadata", "error", err)
		return nil, WrapError(ErrorCodeInfrastructure, "failed to capture current process metadata", err)
	}

	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		Name:            input.RunConfig.Name,
		RunsBaseDir:     r.runsBaseDir,
		QueuedSource:    queuedSource,
		AppConfigPath:   cloneOptionalString(input.AppConfigPath),
		RunConfigPath:   cloneOptionalString(input.RunConfigPath),
		MaxDepth:        input.RunConfig.RLM.MaxDepth,
		EventObservers:  append([]runtime.EventObserver(nil), r.eventObservers...),
		ProcessMetadata: &processMetadata,
	})
	if err != nil {
		logger.Error("failed to initialize run lifecycle", "error", err)
		return nil, WrapError(ErrorCodeInfrastructure, "failed to initialize run lifecycle", err)
	}
	defer func() {
		if err == nil {
			return
		}
		if removeErr := runtime.RemoveProcessMetadata(r.runsBaseDir, lifecycle.RunID()); removeErr != nil {
			logger.Error("failed to remove process metadata after acceptance failure",
				"run_id", lifecycle.RunID(),
				"error", removeErr,
			)
		}
		if closeErr := lifecycle.Close(); closeErr != nil {
			logger.Error("failed to close lifecycle after acceptance failure",
				"run_id", lifecycle.RunID(),
				"error", closeErr,
			)
		}
	}()

	eventsPath, err := lifecycle.EventsFilePath()
	if err != nil {
		logger.Error("failed to resolve events path", "run_id", lifecycle.RunID(), "error", err)
		return nil, WrapError(ErrorCodeInfrastructure, "failed to resolve run events path", err)
	}
	logger.Info("initialized run lifecycle",
		"run_id", lifecycle.RunID(),
		"events_path", eventsPath,
	)

	var submittedRunConfigRef *string
	if input.SubmittedRunConfigYAML != nil {
		runArtifacts, artifactErr := NewRunArtifactStore(r.runsBaseDir)
		if artifactErr != nil {
			logger.Error("failed to initialize run artifact store for submitted config provenance",
				"run_id", lifecycle.RunID(),
				"error", artifactErr,
			)
			return nil, WrapError(ErrorCodeInfrastructure, "failed to initialize submitted run config provenance store", artifactErr)
		}

		ref, artifactErr := runArtifacts.PersistSubmittedRunConfig(lifecycle.RunID(), *input.SubmittedRunConfigYAML, input.TemplateVars)
		if artifactErr != nil {
			logger.Error("failed to persist submitted run config artifact",
				"run_id", lifecycle.RunID(),
				"error", artifactErr,
			)
			return nil, WrapError(ErrorCodeInfrastructure, "failed to persist submitted run config artifact", artifactErr)
		}
		submittedRunConfigRef = &ref
	}

	return &acceptedRun{
		input:                 input,
		guardrailRunContext:   guardrailRunContext,
		cancel:                cancel,
		cancelDeadline:        cancelDeadline,
		lifecycle:             lifecycle,
		eventsPath:            eventsPath,
		guardrails:            guardrails,
		submittedRunConfigRef: submittedRunConfigRef,
	}, nil
}

func (r *Runner) executeAcceptedRun(accepted *acceptedRun) (RunResult, error) {
	logger := harnessRunnerLogger()
	if accepted == nil || accepted.lifecycle == nil {
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "accepted run is required", nil)
	}

	lifecycle := accepted.lifecycle
	input := accepted.input

	defer func() {
		if accepted.cancelDeadline != nil {
			accepted.cancelDeadline()
		}
		if err := runtime.RemoveProcessMetadata(r.runsBaseDir, lifecycle.RunID()); err != nil {
			logger.Error("failed to remove process metadata", "run_id", lifecycle.RunID(), "error", err)
		}
		if err := lifecycle.Close(); err != nil {
			logger.Error("failed to close lifecycle event store", "run_id", lifecycle.RunID(), "error", err)
		}
		accepted.cancel(nil)
	}()

	if interruptErr := interruptionError(accepted.guardrailRunContext, ""); interruptErr != nil {
		logger.Warn("run interrupted before execution start", "run_id", lifecycle.RunID())
		return RunResult{}, failRunningRunWithAccounting(lifecycle, nil, nil, input.RunConfig, nil, interruptErr)
	}

	if err := lifecycle.StartExecution(); err != nil {
		logger.Error("failed to start run lifecycle", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, WrapError(ErrorCodeInfrastructure, "failed to start run lifecycle", err)
	}
	if interruptErr := interruptionError(accepted.guardrailRunContext, ""); interruptErr != nil {
		logger.Warn("run interrupted immediately after execution start", "run_id", lifecycle.RunID())
		return RunResult{}, failRunningRunWithAccounting(lifecycle, nil, nil, input.RunConfig, nil, interruptErr)
	}

	actionArtifacts, err := NewActionArtifactStore(r.runsBaseDir)
	if err != nil {
		logger.Error("failed to initialize action artifact store", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to initialize action artifact store", err))
	}

	sessions, err := NewREPLSessionManager(r.replFactory, actionArtifacts)
	if err != nil {
		logger.Error("failed to initialize repl session manager", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to initialize repl session manager", err))
	}
	defer func() {
		if closeErr := sessions.CloseAll(); closeErr != nil {
			logger.Error("failed to close all repl sessions", "run_id", lifecycle.RunID(), "error", closeErr)
		}
	}()

	runArtifacts, err := NewRunArtifactStore(r.runsBaseDir)
	if err != nil {
		logger.Error("failed to initialize run artifact store", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to initialize run artifact store", err))
	}

	stepExecutor, err := NewStepExecutor(lifecycle, sessions, actionArtifacts)
	if err != nil {
		logger.Error("failed to initialize step executor", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to initialize step executor", err))
	}

	inferenceClient, err := r.inferenceFactory(input.RunConfig)
	if err != nil {
		logger.Error("failed to initialize inference service", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to initialize inference service", err))
	}

	_, effectiveSystemPrompt, err := r.promptResolver.ResolveEffective(input.RunConfig.LLM.Provider, input.RunConfig.SystemPromptAppend)
	if err != nil {
		logger.Error("failed to resolve effective system prompt", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to resolve effective system prompt", err))
	}

	effectivePrompt, effectiveContext, err := resolvePromptAndContext(input.RunConfig, input.TemplateVars, r.templateRenderer)
	if err != nil {
		logger.Error("failed to resolve prompt or context templates", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeTemplateRender, "failed to resolve prompt/context", err))
	}

	rootNode, err := lifecycle.RootNode()
	if err != nil {
		logger.Error("failed to resolve root node", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRun(lifecycle, nil, input.RunConfig, WrapError(ErrorCodeInfrastructure, "failed to resolve root node", err))
	}
	ledger := accounting.NewLedger(input.RunConfig.Accounting.PricingVersion)
	ledger.SetRootNodeID(rootNode.ID)
	logger.Info("starting root node execution",
		"run_id", lifecycle.RunID(),
		"node_id", rootNode.ID,
		"node_depth", rootNode.Depth,
		"effective_prompt_bytes", len(effectivePrompt),
		"effective_context_bytes", len(effectiveContext),
	)

	execCtx := executionContext{
		runConfig:    input.RunConfig,
		runContext:   accepted.guardrailRunContext,
		lifecycle:    lifecycle,
		inference:    inferenceClient,
		sessions:     sessions,
		artifacts:    actionArtifacts,
		stepExecutor: stepExecutor,
		runArtifacts: runArtifacts,
		ledger:       ledger,
		systemPrompt: effectiveSystemPrompt,
		nonRecursive: !input.RunConfig.RLM.Enabled,
		guardrails:   accepted.guardrails,
	}

	rootResult, err := r.executeNode(accepted.guardrailRunContext, &execCtx, rootNode, effectivePrompt, effectiveContext)
	if err != nil {
		failedNodeID := rootNode.ID
		logger.Error("root node execution failed",
			"run_id", lifecycle.RunID(),
			"node_id", failedNodeID,
			"error", err,
		)
		return RunResult{}, failRunningRunWithAccounting(lifecycle, runArtifacts, ledger, input.RunConfig, &failedNodeID, err)
	}

	runAccounting := ledger.RunRollup()
	runAccountingRef, err := runArtifacts.PersistRunAccounting(lifecycle.RunID(), runAccounting)
	if err != nil {
		logger.Error("failed to persist run accounting artifact", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRunWithAccounting(lifecycle, runArtifacts, ledger, input.RunConfig, nil, WrapError(ErrorCodeInfrastructure, "failed to persist run accounting artifact", err))
	}
	if err := lifecycle.CompleteWithAccounting(&rootResult.finalRef, runAccounting, &runAccountingRef); err != nil {
		logger.Error("failed to complete run", "run_id", lifecycle.RunID(), "error", err)
		return RunResult{}, failRunningRunWithAccounting(lifecycle, runArtifacts, ledger, input.RunConfig, nil, WrapError(ErrorCodeInfrastructure, "failed to complete run", err))
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
		EventsPath:     accepted.eventsPath,
		Accounting:     runAccounting,
	}, nil
}

func (r *Runner) validateDependencies() error {
	if r == nil {
		return WrapError(ErrorCodeInfrastructure, "runner is required", nil)
	}
	if r.templateRenderer == nil {
		return WrapError(ErrorCodeInfrastructure, "template renderer is required", nil)
	}
	if r.promptResolver == nil {
		return WrapError(ErrorCodeInfrastructure, "system prompt resolver is required", nil)
	}
	if r.replFactory == nil {
		return WrapError(ErrorCodeInfrastructure, "repl session factory is required", nil)
	}
	if r.inferenceFactory == nil {
		return WrapError(ErrorCodeInfrastructure, "inference factory is required", nil)
	}
	return nil
}

func resolveQueuedSource(input RunInput) runtime.RunQueuedSource {
	if strings.TrimSpace(string(input.QueuedSource)) == "" {
		return runtime.RunQueuedSourceCLIRunStart
	}
	return input.QueuedSource
}

func resolveProcessMetadataSource(input RunInput, queuedSource runtime.RunQueuedSource) string {
	if strings.TrimSpace(input.ProcessMetadataSource) != "" {
		return input.ProcessMetadataSource
	}
	if queuedSource == runtime.RunQueuedSourceAppServerStart {
		return runtime.RunSourceAppServerStart
	}
	return runtime.RunSourceCLIRunStart
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
		if code, ok := CodeOf(runErr); ok && code == ErrorCodeInterrupted {
			if closeErr := execCtx.sessions.CloseNode(node.ID); closeErr != nil {
				logger.Error("failed to close node repl session", "error", closeErr)
			}
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
	if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
		return nodeExecutionResult{}, interruptErr
	}

	contextRef, err := execCtx.runArtifacts.PersistContext(execCtx.lifecycle.RunID(), node.ID, baseContext)
	if err != nil {
		logger.Error("failed to persist node context artifact", "error", err)
		return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist node context artifact", err)
	}
	contextMetadata := buildContextMetadata(baseContext, contextRef)
	var previousFeedback *PreviousActionFeedback
	var previousStepFeedback *PreviousStepFeedback
	recursiveSubcallsAllowed := true
	var recursiveSubcallsReason *string
	for {
		if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
			return nodeExecutionResult{}, interruptErr
		}
		failedStepID = nil
		if err := execCtx.guardrails.CheckBeforeStep(node.ID, time.Now().UTC()); err != nil {
			logger.Error("deterministic runtime guardrail blocked new step start", "error", err)
			return nodeExecutionResult{}, err
		}
		stepStart := time.Now().UTC()

		stepStarted, err := execCtx.lifecycle.AppendNodeStepStarted(node.ID)
		if err != nil {
			logger.Error("failed to append node.step.started", "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.started", err)
		}
		execCtx.guardrails.RecordStepStarted(node.ID)
		budgetSnapshot := execCtx.guardrails.StepBudgetSnapshot(node.ID)
		stepID := stepStarted.StepID
		failedStepID = &stepID
		logger.Debug("started node step",
			"step_id", stepStarted.StepID,
			"step_index", stepStarted.StepIndex,
		)
		if shouldDisableChildRecursionAfterStall(node, budgetSnapshot, recursiveSubcallsAllowed, previousFeedback) {
			reason := "child node has used multiple steps without finalizing; stay local and return a bounded answer or unresolved result to the parent"
			recursiveSubcallsAllowed = false
			recursiveSubcallsReason = &reason
			logger.Info("switching stalled child node to local-only recursive subcall fallback",
				"node_id", node.ID,
				"step_id", stepStarted.StepID,
				"node_steps_used", budgetSnapshot.NodeStepsUsed,
			)
		}

		executionState := buildStepExecutionState(
			node,
			execCtx.lifecycle.MaxDepth(),
			budgetSnapshot.NodeStepsUsed,
			budgetSnapshot.NodeStepsRemaining,
			budgetSnapshot.RunStepsUsed,
			budgetSnapshot.RunStepsRemaining,
			contextMetadata,
			previousFeedback,
			recursiveSubcallsAllowed,
			recursiveSubcallsReason,
		)
		envelope, err := buildStepInputEnvelope(prompt, stepStarted.StepIndex, contextMetadata, executionState, previousFeedback)
		if err != nil {
			logger.Error("failed to build deterministic step input envelope", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to build deterministic step input envelope", err)
		}
		envelope.PreviousStepFeedback = previousStepFeedback

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

		userTurnRef, err := execCtx.runArtifacts.PersistUserTurn(execCtx.lifecycle.RunID(), node.ID, stepStarted.StepID, envelope, messages)
		if err != nil {
			logger.Error("failed to persist user turn artifact", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist user turn artifact", err)
		}
		if err := execCtx.lifecycle.AppendNodeTurn(node.ID, runtime.TurnRoleUser, stepStarted.StepID, userTurnRef, nil); err != nil {
			logger.Error("failed to append node.turn.user", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.turn.user", err)
		}
		if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
			return nodeExecutionResult{}, interruptErr
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
			Accounting: buildInferenceAccountingConfig(execCtx.runConfig),
		})
		if err != nil {
			if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
				logger.Warn("inference interrupted by external stop request",
					"step_id", stepStarted.StepID,
					"node_id", node.ID,
				)
				return nodeExecutionResult{}, interruptErr
			}
			if limitErr := execCtx.guardrails.CheckRunDuration(node.ID, stepStarted.StepID, time.Now().UTC()); limitErr != nil {
				logger.Error("deterministic runtime guardrail interrupted active inference step",
					"step_id", stepStarted.StepID,
					"error", limitErr,
				)
				return nodeExecutionResult{}, limitErr
			}
			logger.Error("inference execution failed",
				"step_id", stepStarted.StepID,
				"error", err,
			)
			return nodeExecutionResult{}, WrapError(ErrorCodeInference, "inference execution failed", err)
		}
		execCtx.ledger.RecordModelTurn(node.ID, stepStarted.StepID, inferenceResult.Accounting)

		modelTurnRef, err := execCtx.runArtifacts.PersistModelTurn(execCtx.lifecycle.RunID(), node.ID, stepStarted.StepID, inferenceResult)
		if err != nil {
			logger.Error("failed to persist model turn artifact", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to persist model turn artifact", err)
		}
		modelTokens := &runtime.TurnTokenUsage{
			InputTokens:     &inferenceResult.Usage.InputTokens,
			OutputTokens:    &inferenceResult.Usage.OutputTokens,
			TotalTokens:     &inferenceResult.Usage.TotalTokens,
			ReasoningTokens: inferenceResult.Usage.ReasoningTokens,
		}
		if err := execCtx.lifecycle.AppendNodeTurn(node.ID, runtime.TurnRoleModel, stepStarted.StepID, modelTurnRef, modelTokens); err != nil {
			logger.Error("failed to append node.turn.model", "step_id", stepStarted.StepID, "error", err)
			return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.turn.model", err)
		}
		if err := execCtx.guardrails.CheckRunAccounting(node.ID, stepStarted.StepID, execCtx.ledger.RunRollup().TreeTotal); err != nil {
			logger.Error("deterministic runtime guardrail breached after model-turn accounting update",
				"step_id", stepStarted.StepID,
				"error", err,
			)
			return nodeExecutionResult{}, err
		}
		if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
			return nodeExecutionResult{}, interruptErr
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
				Lifecycle:      execCtx.lifecycle,
				Inference:      execCtx.inference,
				RunConfig:      execCtx.runConfig,
				Node:           node,
				StepID:         stepStarted.StepID,
				ActionIndex:    1,
				ForceLocalOnly: !executionState.RecursiveSubcallsAllowed,
				NonRecursive:   execCtx.nonRecursive,
				RunArtifacts:   execCtx.runArtifacts,
				Ledger:         execCtx.ledger,
				Guardrails:     execCtx.guardrails,
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
				if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
					logger.Warn("continue action interrupted by external stop request",
						"step_id", stepStarted.StepID,
						"node_id", node.ID,
					)
					return nodeExecutionResult{}, interruptErr
				}
				logger.Error("failed to execute continue action",
					"step_id", stepStarted.StepID,
					"error", actionErr,
				)
				if code, ok := CodeOf(actionErr); ok && code == ErrorCodeLimitExceeded {
					if actionPayload.StepID != "" {
						if err := appendContinueStepCompleted(execCtx, node.ID, stepStarted, stepStart); err != nil {
							logger.Error("failed to append node.step.completed for fatal continue decision",
								"step_id", stepStarted.StepID,
								"error", err,
							)
							return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.completed", err)
						}
					}
					return nodeExecutionResult{}, actionErr
				}
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to execute continue action", actionErr)
			}
			if interruptErr := interruptionError(ctx, node.ID); interruptErr != nil {
				return nodeExecutionResult{}, interruptErr
			}
			if err := execCtx.guardrails.CheckRunDuration(node.ID, stepStarted.StepID, time.Now().UTC()); err != nil {
				logger.Error("deterministic runtime guardrail interrupted active continue step", "step_id", stepStarted.StepID, "error", err)
				return nodeExecutionResult{}, err
			}
			if err := execCtx.guardrails.RecordContinueAction(node.ID, stepStarted.StepID, actionPayload.Status); err != nil {
				if stepCompleteErr := appendContinueStepCompleted(execCtx, node.ID, stepStarted, stepStart); stepCompleteErr != nil {
					logger.Error("failed to append node.step.completed for guardrail-breached continue decision",
						"step_id", stepStarted.StepID,
						"error", stepCompleteErr,
					)
					return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.completed", stepCompleteErr)
				}
				logger.Error("deterministic runtime guardrail breached after continue action", "step_id", stepStarted.StepID, "error", err)
				return nodeExecutionResult{}, err
			}
			if actionPayload.Status == runtime.ActionExecutionStatusFailed {
				logger.Warn("continue action recorded non-fatal failure",
					"step_id", stepStarted.StepID,
					"action_index", actionPayload.ActionIndex,
					"error_code", valueOrEmpty(actionPayload.ErrorCode),
					"action_ref", actionPayload.ActionRef,
				)
			} else {
				logger.Info("continue action completed",
					"step_id", stepStarted.StepID,
					"action_index", actionPayload.ActionIndex,
					"action_ref", actionPayload.ActionRef,
				)
			}

			if err := appendContinueStepCompleted(execCtx, node.ID, stepStarted, stepStart); err != nil {
				logger.Error("failed to append node.step.completed for continue decision",
					"step_id", stepStarted.StepID,
					"error", err,
				)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to append node.step.completed", err)
			}
			if err := execCtx.guardrails.CheckRunDuration(node.ID, stepStarted.StepID, time.Now().UTC()); err != nil {
				logger.Error("deterministic runtime guardrail breached after continue step completion", "step_id", stepStarted.StepID, "error", err)
				return nodeExecutionResult{}, err
			}
			feedback, feedbackErr := buildPreviousActionFeedback(execCtx.lifecycle.RunID(), execCtx.artifacts, actionPayload)
			if feedbackErr != nil {
				logger.Error("failed to build repl feedback", "step_id", stepStarted.StepID, "error", feedbackErr)
				return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to build repl feedback", feedbackErr)
			}
			previousFeedback = feedback
			if actionPayload.Status == runtime.ActionExecutionStatusCompleted {
				artifact, err := execCtx.artifacts.Read(execCtx.lifecycle.RunID(), actionPayload.ActionRef)
				if err != nil {
					logger.Error("failed to inspect completed action artifact for marked final answer", "step_id", stepStarted.StepID, "error", err)
					return nodeExecutionResult{}, WrapError(ErrorCodeInfrastructure, "failed to inspect completed action artifact", err)
				}
				if candidate, ok := extractMarkedFinalAnswerCandidate(artifact.Stdout); ok {
					finalAnswer := normalizeFinalAnswer(candidate)
					if finalAnswer == "" {
						logger.Error("marked final answer candidate is empty after normalization", "step_id", stepStarted.StepID)
						return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "marked final answer candidate is empty after boundary whitespace normalization", nil)
					}
					if shouldRejectPrematureRecursiveMarkedFinal(node, envelope, feedback, finalAnswer) {
						previousStepFeedback = buildRecursivePartitionMapRequiredFeedback(stepStarted.StepID)
						logger.Info("rejecting marked final answer until recursive partition-map action runs",
							"node_id", node.ID,
							"step_id", stepStarted.StepID,
						)
						continue
					}
					if shouldRejectMarkedFinalBeforeRecursiveReducer(node, envelope, finalAnswer) {
						previousStepFeedback = buildRecursiveReducerFinalRejectedFeedback(stepStarted.StepID)
						logger.Info("rejecting marked final answer until recursive-map reducer action runs",
							"node_id", node.ID,
							"step_id", stepStarted.StepID,
						)
						continue
					}
					if shouldRequireRecursiveMapReducer(node, envelope, feedback) &&
						!markedFinalAnswerCompletesRecursiveMapReducer(artifact.Stdout, feedback.SubcallSummary) {
						previousStepFeedback = buildRecursiveMapReducerRequiredFeedback(stepStarted.StepID, feedback.SubcallSummary)
						logger.Info("rejecting marked final answer until recursive-map reducer step completes",
							"node_id", node.ID,
							"step_id", stepStarted.StepID,
							"recursive_subcalls", feedback.SubcallSummary.RecursiveCount,
						)
						continue
					}
					finalEvidence := []FinalEvidence{{Ref: actionPayload.ActionRef}}
					finalRef, err := completeNodeWithFinalAnswer(execCtx, node, finalAnswer, finalEvidence, nil)
					if err != nil {
						logger.Error("failed to complete node from marked final answer candidate", "step_id", stepStarted.StepID, "error", err)
						return nodeExecutionResult{}, err
					}
					failedStepID = nil
					logger.Info("node reached marked final answer from continue action",
						"step_id", stepStarted.StepID,
						"duration_ms", durationMS(stepStart),
						"final_answer_ref", finalRef,
						"final_answer_bytes", len(finalAnswer),
					)
					return nodeExecutionResult{answer: finalAnswer, finalRef: finalRef}, nil
				}
			}
			logger.Debug("completed continue step",
				"step_id", stepStarted.StepID,
				"duration_ms", durationMS(stepStart),
			)

			if shouldRequireRecursivePartitionMapRetry(node, envelope, feedback) {
				previousStepFeedback = buildRecursivePartitionMapRequiredFeedback(stepStarted.StepID)
				logger.Info("requesting recursive partition-map retry after local-only root step",
					"node_id", node.ID,
					"step_id", stepStarted.StepID,
					"action_status", feedback.Status,
				)
			} else if shouldRequireRecursiveMapReducer(node, envelope, feedback) {
				previousStepFeedback = buildRecursiveMapReducerRequiredFeedback(stepStarted.StepID, feedback.SubcallSummary)
				logger.Info("requesting local reducer step after root recursive map",
					"node_id", node.ID,
					"step_id", stepStarted.StepID,
					"recursive_subcalls", feedback.SubcallSummary.RecursiveCount,
				)
			} else {
				previousStepFeedback = nil
			}
			if shouldDisableChildRecursionAfterRecursiveWork(node, recursiveSubcallsAllowed, feedback) {
				reason := "child node already used recursive subcalls; finish locally or return a bounded unresolved result to the parent"
				recursiveSubcallsAllowed = false
				recursiveSubcallsReason = &reason
				logger.Info("switching recursive child node to local-only fallback after child recursion",
					"node_id", node.ID,
					"step_id", stepStarted.StepID,
				)
			} else if executionState.SmallContext && recursiveSubcallsAllowed && feedback.SubcallSummary != nil && feedback.SubcallSummary.RecursiveCount > 0 {
				reason := "small context already used recursive subcalls in this node; stay local in later steps"
				recursiveSubcallsAllowed = false
				recursiveSubcallsReason = &reason
				logger.Info("switching node to local-only recursive subcall fallback",
					"node_id", node.ID,
					"step_id", stepStarted.StepID,
				)
			}
			continue
		}

		logger.Error("unsupported non-action model decision",
			"step_id", stepStarted.StepID,
			"decision", decisionPayload.Decision,
		)
		return nodeExecutionResult{}, WrapError(ErrorCodeOutputValidation, "decision=final is not supported; final answers must be emitted by continuation.repl_code using FINAL_ANSWER_START and FINAL_ANSWER_END", nil)
	}
}

func shouldRequireRecursivePartitionMapRetry(node runtime.Node, envelope StepInputEnvelope, feedback *PreviousActionFeedback) bool {
	if node.Depth != 0 || feedback == nil {
		return false
	}
	if envelope.RecursionPolicy == nil || *envelope.RecursionPolicy != StepRecursionPolicyPartitionMapRecursive {
		return false
	}
	if envelope.ExecutionState.SmallContext || !envelope.ExecutionState.RecursiveSubcallsAllowed || envelope.ExecutionState.RemainingDepth <= 0 {
		return false
	}
	if envelope.PreviousStepFeedback != nil &&
		(envelope.PreviousStepFeedback.Code == previousStepFeedbackCodeRecursiveMapReducerRequired ||
			envelope.PreviousStepFeedback.Code == previousStepFeedbackCodeRecursiveReducerFinalRejected) {
		return false
	}
	if feedback.SubcallSummary == nil {
		return true
	}
	return feedback.SubcallSummary.RecursiveCount == 0
}

func shouldRequireRecursiveMapReducer(node runtime.Node, envelope StepInputEnvelope, feedback *PreviousActionFeedback) bool {
	if node.Depth != 0 || feedback == nil || feedback.SubcallSummary == nil {
		return false
	}
	if envelope.ExecutionState.SmallContext {
		return false
	}
	if feedback.Status != string(runtime.ActionExecutionStatusCompleted) {
		return false
	}
	if feedback.SubcallSummary.RecursiveCount <= 1 || feedback.SubcallSummary.FailedCount > 0 {
		return false
	}
	if envelope.PreviousStepFeedback != nil &&
		(envelope.PreviousStepFeedback.Code == previousStepFeedbackCodeRecursiveMapReducerRequired ||
			envelope.PreviousStepFeedback.Code == previousStepFeedbackCodeRecursiveReducerFinalRejected) {
		return false
	}
	return true
}

func markedFinalAnswerCompletesRecursiveMapReducer(stdout string, summary *PreviousActionSubcallSummary) bool {
	if summary == nil {
		return false
	}
	if summary.TotalCount <= 0 || summary.RecursiveCount <= 1 {
		return false
	}
	if summary.FailedCount > 0 || summary.CompletedCount != summary.TotalCount {
		return false
	}
	return stdoutContainsCompleteCoverage(stdout, summary.CompletedCount, summary.TotalCount)
}

func stdoutContainsCompleteCoverage(stdout string, completed int, total int) bool {
	normalized := strings.ReplaceAll(stdout, "\r\n", "\n")
	for _, line := range strings.Split(normalized, "\n") {
		actualCompleted, actualTotal, ok := parseCoverageLine(line)
		if ok && actualCompleted == completed && actualTotal == total {
			return true
		}
	}
	return false
}

func parseCoverageLine(line string) (int, int, bool) {
	fields := strings.Fields(line)
	if len(fields) == 4 && strings.EqualFold(fields[0], "COVERAGE") && fields[2] == "/" {
		completed, completedErr := strconv.Atoi(fields[1])
		total, totalErr := strconv.Atoi(fields[3])
		if completedErr == nil && totalErr == nil {
			return completed, total, true
		}
	}
	if len(fields) == 2 && strings.EqualFold(fields[0], "COVERAGE") {
		parts := strings.Split(fields[1], "/")
		if len(parts) == 2 {
			completed, completedErr := strconv.Atoi(strings.TrimSpace(parts[0]))
			total, totalErr := strconv.Atoi(strings.TrimSpace(parts[1]))
			if completedErr == nil && totalErr == nil {
				return completed, total, true
			}
		}
	}
	return 0, 0, false
}

func shouldRejectPrematureRecursiveMarkedFinal(node runtime.Node, envelope StepInputEnvelope, feedback *PreviousActionFeedback, finalAnswer string) bool {
	if node.Depth != 0 {
		return false
	}
	if envelope.RecursionPolicy == nil || *envelope.RecursionPolicy != StepRecursionPolicyPartitionMapRecursive {
		return false
	}
	if envelope.ExecutionState.SmallContext || !envelope.ExecutionState.RecursiveSubcallsAllowed || envelope.ExecutionState.RemainingDepth <= 0 {
		return false
	}
	if strings.EqualFold(normalizeFinalAnswer(finalAnswer), "NONE") {
		return false
	}
	if feedback != nil && feedback.SubcallSummary != nil && feedback.SubcallSummary.RecursiveCount > 0 {
		return false
	}
	if envelope.StepIndex == 1 && envelope.PreviousActionFeedback == nil && envelope.PreviousStepFeedback == nil {
		return true
	}
	return envelope.PreviousStepFeedback != nil
}

func shouldRejectMarkedFinalBeforeRecursiveReducer(node runtime.Node, envelope StepInputEnvelope, finalAnswer string) bool {
	if node.Depth != 0 || envelope.PreviousStepFeedback == nil {
		return false
	}
	switch envelope.PreviousStepFeedback.Code {
	case previousStepFeedbackCodeRecursiveMapReducerRequired, previousStepFeedbackCodeRecursiveReducerFinalRejected:
	default:
		return false
	}
	return !strings.EqualFold(normalizeFinalAnswer(finalAnswer), "NONE")
}

func shouldRejectPrematureRecursiveFinal(node runtime.Node, envelope StepInputEnvelope, decisionPayload DecisionPayload) bool {
	if node.Depth != 0 || decisionPayload.Final == nil {
		return false
	}
	if envelope.RecursionPolicy == nil || *envelope.RecursionPolicy != StepRecursionPolicyPartitionMapRecursive {
		return false
	}
	if envelope.ExecutionState.SmallContext || !envelope.ExecutionState.RecursiveSubcallsAllowed || envelope.ExecutionState.RemainingDepth <= 0 {
		return false
	}
	if strings.EqualFold(normalizeFinalAnswer(decisionPayload.Final.Answer), "NONE") {
		return false
	}
	if envelope.StepIndex == 1 && envelope.PreviousActionFeedback == nil && envelope.PreviousStepFeedback == nil {
		return true
	}
	return envelope.PreviousStepFeedback != nil
}

func shouldRejectFinalBeforeRecursiveReducer(node runtime.Node, envelope StepInputEnvelope, decisionPayload DecisionPayload) bool {
	if node.Depth != 0 || decisionPayload.Final == nil || envelope.PreviousStepFeedback == nil {
		return false
	}
	switch envelope.PreviousStepFeedback.Code {
	case previousStepFeedbackCodeRecursiveMapReducerRequired, previousStepFeedbackCodeRecursiveReducerFinalRejected:
		return !strings.EqualFold(normalizeFinalAnswer(decisionPayload.Final.Answer), "NONE")
	default:
		return false
	}
}

func shouldDisableChildRecursionAfterStall(node runtime.Node, budgetSnapshot stepBudgetSnapshot, recursiveSubcallsAllowed bool, previousFeedback *PreviousActionFeedback) bool {
	return node.Depth > 0 && recursiveSubcallsAllowed && previousFeedback != nil && budgetSnapshot.NodeStepsUsed >= 3
}

func shouldDisableChildRecursionAfterRecursiveWork(node runtime.Node, recursiveSubcallsAllowed bool, feedback *PreviousActionFeedback) bool {
	if node.Depth == 0 || !recursiveSubcallsAllowed || feedback == nil || feedback.SubcallSummary == nil {
		return false
	}
	return feedback.SubcallSummary.RecursiveCount > 0
}

func buildRecursivePartitionMapRequiredFeedback(stepID string) *PreviousStepFeedback {
	policy := StepRecursionPolicyPartitionMapRecursive
	return &PreviousStepFeedback{
		StepID:                  stepID,
		Code:                    previousStepFeedbackCodeRecursivePartitionMapRequired,
		Message:                 "The previous root step was classified as partition_map_recursive but executed no recursive subcalls. Retry with a compile-safe recursive partition-map action, or return NONE only if the task is explicitly absent.",
		RequiredRecursionPolicy: &policy,
	}
}

func buildPrematureFinalRejectedFeedback(stepID string) *PreviousStepFeedback {
	policy := StepRecursionPolicyPartitionMapRecursive
	return &PreviousStepFeedback{
		StepID:                  stepID,
		Code:                    previousStepFeedbackCodePrematureFinalRejected,
		Message:                 "The previous root final was rejected because large-context recursive partition-map evidence is still required. Execute recursive child subcalls before finalizing, unless the canonical answer is exactly NONE.",
		RequiredRecursionPolicy: &policy,
	}
}

func buildRecursiveMapReducerRequiredFeedback(stepID string, summary *PreviousActionSubcallSummary) *PreviousStepFeedback {
	policy := StepRecursionPolicyLocalOrLeaf
	recursiveCount := 0
	if summary != nil {
		recursiveCount = summary.RecursiveCount
	}
	return &PreviousStepFeedback{
		StepID:                  stepID,
		Code:                    previousStepFeedbackCodeRecursiveMapReducerRequired,
		Message:                 fmt.Sprintf("The previous root action completed %d recursive subcall(s). Run one local reducer action before finalizing: read the previous action artifact, aggregate all child answer strings, verify coverage, and print the reduced result.", recursiveCount),
		RequiredRecursionPolicy: &policy,
	}
}

func buildRecursiveReducerFinalRejectedFeedback(stepID string) *PreviousStepFeedback {
	policy := StepRecursionPolicyLocalOrLeaf
	return &PreviousStepFeedback{
		StepID:                  stepID,
		Code:                    previousStepFeedbackCodeRecursiveReducerFinalRejected,
		Message:                 "The previous root final was rejected because recursive-map results must be reduced by a local action first. Use read_action_artifact(previous_action_ref), aggregate every child result, verify coverage, then finalize.",
		RequiredRecursionPolicy: &policy,
	}
}

func completeNodeWithFinalAnswer(execCtx *executionContext, node runtime.Node, finalAnswer string, evidence []FinalEvidence, confidence *string) (string, error) {
	finalRef, err := execCtx.runArtifacts.PersistFinalAnswer(
		execCtx.lifecycle.RunID(),
		node.ID,
		finalAnswer,
		evidence,
		confidence,
	)
	if err != nil {
		return "", WrapError(ErrorCodeInfrastructure, "failed to persist final answer output", err)
	}
	nodeAccounting := execCtx.ledger.NodeRollup(node.ID)
	nodeAccountingRef, err := execCtx.runArtifacts.PersistNodeAccounting(execCtx.lifecycle.RunID(), node.ID, nodeAccounting)
	if err != nil {
		return "", WrapError(ErrorCodeInfrastructure, "failed to persist node accounting output", err)
	}
	if err := execCtx.lifecycle.CompleteNodeWithAccounting(node.ID, &finalRef, nodeAccounting, &nodeAccountingRef); err != nil {
		return "", WrapError(ErrorCodeInfrastructure, "failed to complete node", err)
	}
	if err := execCtx.sessions.CloseNode(node.ID); err != nil {
		return "", WrapError(ErrorCodeInfrastructure, "failed to close node repl session", err)
	}
	return finalRef, nil
}

func appendContinueStepCompleted(execCtx *executionContext, nodeID string, stepStarted runtime.NodeStepStartedPayload, stepStart time.Time) error {
	if execCtx == nil {
		return fmt.Errorf("execution context is required")
	}
	stepRollup := execCtx.ledger.StepRollup(nodeID, stepStarted.StepID)
	stepAccountingRef, err := execCtx.runArtifacts.PersistStepAccounting(execCtx.lifecycle.RunID(), nodeID, stepStarted.StepID, stepRollup)
	if err != nil {
		return fmt.Errorf("failed to persist step accounting output; %w", err)
	}
	return execCtx.lifecycle.AppendNodeStepCompleted(nodeID, runtime.NodeStepCompletedPayload{
		StepID:        stepStarted.StepID,
		Decision:      runtime.StepDecisionContinue,
		ActionCount:   1,
		DurationMS:    durationMS(stepStart),
		Accounting:    stepRollup,
		AccountingRef: stepAccountingRef,
	})
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

func failRunningRun(lifecycle *runtime.Lifecycle, failedNodeID *string, runConfig config.RunConfig, runErr error) error {
	return failRunningRunWithAccounting(lifecycle, nil, nil, runConfig, failedNodeID, runErr)
}

func failRunningRunWithAccounting(lifecycle *runtime.Lifecycle, runArtifacts *RunArtifactStore, ledger *accounting.Ledger, runConfig config.RunConfig, failedNodeID *string, runErr error) error {
	if lifecycle == nil || runErr == nil {
		return runErr
	}
	if interruptMetadata, ok := InterruptOf(runErr); ok {
		return interruptRunWithAccounting(lifecycle, runArtifacts, ledger, runConfig, interruptMetadata, runErr)
	}
	if lifecycle.State() != runtime.RunStateRunning {
		return runErr
	}

	errorCode := ErrorCodeInfrastructure
	if code, ok := CodeOf(runErr); ok {
		errorCode = code
	}
	limitMetadata, hasLimit := LimitOf(runErr)
	resolvedFailedNodeID := failedNodeID
	if hasLimit && strings.TrimSpace(limitMetadata.NodeID) != "" {
		resolvedFailedNodeID = cloneOptionalString(limitMetadata.NodeID)
	}
	var failedStepID *string
	var limitKey *string
	var configuredValue *string
	var observedValue *string
	if hasLimit {
		failedStepID = cloneOptionalString(limitMetadata.StepID)
		limitKey = cloneOptionalString(limitMetadata.LimitKey)
		configuredValue = cloneOptionalString(limitMetadata.ConfiguredValue)
		observedValue = cloneOptionalString(limitMetadata.ObservedValue)
	}
	runAccounting := unavailableAccountingRollupForRun(runConfig)
	var runAccountingRef *string
	if ledger != nil {
		runAccounting = ledger.RunRollup()
	}
	if runArtifacts != nil {
		ref, err := runArtifacts.PersistRunAccounting(lifecycle.RunID(), runAccounting)
		if err != nil {
			harnessRunnerLogger().Error("failed to persist run.failed accounting output",
				"run_id", lifecycle.RunID(),
				"error", err,
			)
			runErr = fmt.Errorf("%w; additionally failed to persist run accounting artifact: %v", runErr, err)
		} else {
			runAccountingRef = &ref
		}
	}

	payload := runtime.RunFailedPayload{
		Status:          "failed",
		ErrorCode:       string(errorCode),
		ErrorMessage:    runErr.Error(),
		FailedNodeID:    resolvedFailedNodeID,
		FailedStepID:    failedStepID,
		LimitKey:        limitKey,
		ConfiguredValue: configuredValue,
		ObservedValue:   observedValue,
		Retryable:       false,
		Accounting:      runAccounting,
		AccountingRef:   runAccountingRef,
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

func interruptRunWithAccounting(lifecycle *runtime.Lifecycle, runArtifacts *RunArtifactStore, ledger *accounting.Ledger, runConfig config.RunConfig, interrupt InterruptMetadata, runErr error) error {
	if lifecycle == nil || runErr == nil {
		return runErr
	}
	if lifecycle.State() != runtime.RunStateQueued && lifecycle.State() != runtime.RunStateRunning {
		return runErr
	}

	runAccounting := unavailableAccountingRollupForRun(runConfig)
	var runAccountingRef *string
	if ledger != nil {
		runAccounting = ledger.RunRollup()
	}
	if runArtifacts != nil {
		ref, err := runArtifacts.PersistRunAccounting(lifecycle.RunID(), runAccounting)
		if err != nil {
			harnessRunnerLogger().Error("failed to persist run.interrupted accounting output",
				"run_id", lifecycle.RunID(),
				"error", err,
			)
			runErr = fmt.Errorf("%w; additionally failed to persist run accounting artifact: %v", runErr, err)
		} else {
			runAccountingRef = &ref
		}
	}

	interruptedByValue := resolveInterruptedBy(lifecycle, runArtifacts)
	payload := runtime.RunInterruptedPayload{
		Status:        "interrupted",
		Reason:        runtime.RunInterruptedReasonUserRequest,
		InterruptedBy: interruptedByValue,
		Accounting:    runAccounting,
		AccountingRef: runAccountingRef,
	}
	if strings.TrimSpace(interrupt.NodeID) != "" {
		payload.InterruptedNodeID = cloneOptionalString(interrupt.NodeID)
	}
	if err := lifecycle.InterruptWith(payload); err != nil {
		harnessRunnerLogger().Error("failed to persist run.interrupted event",
			"run_id", lifecycle.RunID(),
			"error", err,
		)
		return fmt.Errorf("%w; additionally failed to persist run.interrupted event: %v", runErr, err)
	}

	harnessRunnerLogger().Warn("run marked interrupted",
		"run_id", lifecycle.RunID(),
		"interrupted_by", valueOrEmpty(payload.InterruptedBy),
		"interrupted_node_id", valueOrEmpty(payload.InterruptedNodeID),
	)

	return runErr
}

func interruptionError(ctx context.Context, nodeID string) error {
	if !runtime.IsExternalStopContext(ctx) {
		return nil
	}
	return NewInterruptError("run interrupted by external stop request", InterruptMetadata{NodeID: nodeID})
}

func resolveInterruptedBy(lifecycle *runtime.Lifecycle, runArtifacts *RunArtifactStore) *string {
	runsBaseDir, err := runsBaseDirForLifecycle(lifecycle, runArtifacts)
	if err != nil {
		fallback := "signal.sigterm"
		return &fallback
	}
	request, ok, err := runtime.ReadStopRequestMetadata(runsBaseDir, lifecycle.RunID())
	if err != nil {
		fallback := "signal.sigterm"
		return &fallback
	}
	if ok {
		requestedBy := request.RequestedBy
		return &requestedBy
	}
	fallback := "signal.sigterm"
	return &fallback
}

func runsBaseDirForLifecycle(lifecycle *runtime.Lifecycle, runArtifacts *RunArtifactStore) (string, error) {
	if runArtifacts != nil && strings.TrimSpace(runArtifacts.runsBaseDir) != "" {
		return runArtifacts.runsBaseDir, nil
	}
	if lifecycle == nil {
		return "", fmt.Errorf("lifecycle is required")
	}
	eventsPath, err := lifecycle.EventsFilePath()
	if err != nil {
		return "", err
	}
	return filepath.Dir(filepath.Dir(eventsPath)), nil
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
