package steps

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/leefowlercu/sigil/internal/config"
	sigilharness "github.com/leefowlercu/sigil/internal/harness"
	sigilinference "github.com/leefowlercu/sigil/internal/inference"
	sigilschema "github.com/leefowlercu/sigil/internal/inference/schema"
	sigilrepl "github.com/leefowlercu/sigil/internal/repl"
	sigilruntime "github.com/leefowlercu/sigil/internal/runtime"
)

type rlmAcceptanceState struct {
	providerForPrompt  string
	resolvedProvider   string
	resolvedPrompt     string
	systemPromptAppend string
	effectivePrompt    string

	runsBaseDir string

	sessionManager *sigilharness.REPLSessionManager
	artifactStore  *sigilharness.ActionArtifactStore
	stepExecutor   *sigilharness.StepExecutor

	activeNode          sigilruntime.Node
	parentNode          sigilruntime.Node
	childNode           sigilruntime.Node
	parentDepth         int
	runMaxDepth         int
	activeStepID        string
	continuationCode    string
	outputValidationErr error

	actionPayload sigilruntime.NodeActionExecutedPayload
	actionErr     error
	artifactPath  string

	queryResult  string
	queryErr     error
	nonRecursive bool

	childFinalAnswer string
	rootFinalAnswer  string
	rootFinalRef     string

	runtimeVersion int

	fatalErr      error
	failedPayload sigilruntime.RunFailedPayload
	bindingResult string

	boundedRawContext        string
	boundedRequests          []sigilinference.Request
	boundedRunResult         sigilharness.RunResult
	boundedRunErr            error
	boundedFirstEnvelope     sigilharness.StepInputEnvelope
	boundedNextEnvelope      sigilharness.StepInputEnvelope
	boundedUserTurnArtifact  map[string]any
	boundedActionArtifact    sigilharness.ActionArtifact
	boundedPersistedEvents   []sigilruntime.EventEnvelope
	boundedFailureCode       sigilharness.ErrorCode
	boundedFailureMessage    string
	boundedFailureInjected   bool
	boundedFailureTransition error
}

type acceptanceSubcalls struct {
	query func(ctx context.Context, request sigilrepl.QueryRequest) (string, error)
}

func (s acceptanceSubcalls) LLMQuery(ctx context.Context, request sigilrepl.QueryRequest) (string, error) {
	return s.query(ctx, request)
}

func (s acceptanceSubcalls) RLMQuery(ctx context.Context, request sigilrepl.QueryRequest) (string, error) {
	return s.query(ctx, request)
}

func (s acceptanceSubcalls) LLMQueryBatched(ctx context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
	results := make([]sigilrepl.BatchedQueryResult, len(requests))
	for index, request := range requests {
		answer, err := s.query(ctx, sigilrepl.QueryRequest{Prompt: request.Prompt, Context: request.Context})
		results[index] = sigilrepl.BatchedQueryResult{Answer: answer}
		if err != nil {
			if code, ok := sigilrepl.CodeOf(err); ok {
				results[index].ErrorCode = string(code)
			}
			results[index].ErrorMessage = err.Error()
		}
	}
	return results, nil
}

func (s acceptanceSubcalls) RLMQueryBatched(ctx context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
	return s.LLMQueryBatched(ctx, requests)
}

func registerRLMHarnessSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^harness base system prompt resolution runs$`, world.harnessBaseSystemPromptResolutionRuns)
	ctx.Step(`^resolved base system prompt is "([^"]*)"$`, world.resolvedBaseSystemPromptIs)
	ctx.Step(`^unregistered provider key "([^"]*)" for system prompt resolution$`, world.unregisteredProviderKeyForSystemPromptResolution)
	ctx.Step(`^run config system_prompt_append is "([^"]*)"$`, world.runConfigSystem_prompt_appendIs)
	ctx.Step(`^harness effective system prompt is constructed$`, world.harnessEffectiveSystemPromptIsConstructed)
	ctx.Step(`^effective system prompt equals base prompt plus two newlines plus append text$`, world.effectiveSystemPromptEqualsBasePromptPlusTwoNewlinesPlusAppendText)
	ctx.Step(`^effective system prompt equals resolved base prompt$`, world.effectiveSystemPromptEqualsResolvedBasePrompt)

	ctx.Step(`^harness execution starts$`, world.harnessExecutionStarts)
	ctx.Step(`^exactly one root node exists with depth (\d+) and null parent$`, world.exactlyOneRootNodeExistsWithDepthAndNullParent)
	ctx.Step(`^an active node in harness execution$`, world.anActiveNodeInHarnessExecution)
	ctx.Step(`^one inference request and response handling cycle complete$`, world.oneInferenceRequestAndResponseHandlingCycleComplete)
	ctx.Step(`^exactly one node-local step is recorded$`, world.exactlyOneNodelocalStepIsRecorded)

	ctx.Step(`^a node-local step is in progress$`, world.aNodelocalStepIsInProgress)
	ctx.Step(`^transcript contributions are persisted for that step$`, world.transcriptContributionsArePersistedForThatStep)
	ctx.Step(`^each turn contribution is recorded with role user or model$`, world.eachTurnContributionIsRecordedWithRoleUserOrModel)

	ctx.Step(`^a node-local step with decision continue$`, world.aNodelocalStepWithDecisionContinue)
	ctx.Step(`^continuation payload is validated$`, world.continuationPayloadIsValidated)
	ctx.Step(`^exactly one executable action is accepted for that step$`, world.exactlyOneExecutableActionIsAcceptedForThatStep)

	ctx.Step(`^an active node with continuation payload containing non-empty continuation\.repl_code$`, world.anActiveNodeWithContinuationPayloadContainingNonemptyContinuationrepl_code)
	ctx.Step(`^harness processes continuation step$`, world.harnessProcessesContinuationStep)
	ctx.Step(`^exactly one continuation\.repl_code action executes in node-local REPL state$`, world.exactlyOneContinuationrepl_codeActionExecutesInNodelocalREPLState)

	ctx.Step(`^an active node continuation payload with decision continue and missing continuation\.repl_code$`, world.anActiveNodeContinuationPayloadWithDecisionContinueAndMissingContinuationrepl_code)
	ctx.Step(`^strict output validation runs$`, world.strictOutputValidationRuns)
	ctx.Step(`^continuation step fails with typed output-validation error$`, world.continuationStepFailsWithTypedOutputvalidationError)

	ctx.Step(`^an active parent node at depth (\d+)$`, world.anActiveParentNodeAtDepth)
	ctx.Step(`^run max recursion depth is (\d+)$`, world.runMaxRecursionDepthIs)
	ctx.Step(`^rlm_query is invoked from node-local Go REPL context$`, world.rlm_queryIsInvokedFromNodelocalGoREPLContext)
	ctx.Step(`^child node is created at depth (\d+)$`, world.childNodeIsCreatedAtDepth)
	ctx.Step(`^child node creation is rejected due to depth limit$`, world.childNodeCreationIsRejectedDueToDepthLimit)
	ctx.Step(`^deterministic depth-limit feedback is returned to caller REPL context$`, world.deterministicDepthlimitFeedbackIsReturnedToCallerREPLContext)
	ctx.Step(`^plain subcall fallback answer is returned and child node is not created$`, world.plainSubcallFallbackAnswerIsReturnedAndChildNodeIsNotCreated)

	ctx.Step(`^an active parent node with child node in progress$`, world.anActiveParentNodeWithChildNodeInProgress)
	ctx.Step(`^child node inference result is decision final with answer "([^"]*)"$`, world.childNodeInferenceResultIsDecisionFinalWithAnswer)
	ctx.Step(`^child node completes$`, world.childNodeCompletes)
	ctx.Step(`^caller REPL context receives rlm_query result "([^"]*)"$`, world.callerREPLContextReceivesRlm_queryResult)
	ctx.Step(`^child final answer is returned to caller REPL context$`, world.childFinalAnswerIsReturnedToCallerREPLContext)

	ctx.Step(`^an active root node inference result is decision final with answer "([^"]*)"$`, world.anActiveRootNodeInferenceResultIsDecisionFinalWithAnswer)
	ctx.Step(`^harness evaluates root node step$`, world.harnessEvaluatesRootNodeStep)
	ctx.Step(`^run completion references terminal root final output$`, world.runCompletionReferencesTerminalRootFinalOutput)

	ctx.Step(`^v(\d+) REPL runtime architecture rules$`, world.vREPLRuntimeArchitectureRules)
	ctx.Step(`^REPL engine configuration is resolved$`, world.rEPLEngineConfigurationIsResolved)
	ctx.Step(`^embedded in-process Go interpretation is selected$`, world.embeddedInprocessGoInterpretationIsSelected)

	ctx.Step(`^an active node with no existing REPL session$`, world.anActiveNodeWithNoExistingREPLSession)
	ctx.Step(`^first continue action executes$`, world.firstContinueActionExecutes)
	ctx.Step(`^one REPL session is created and associated to that node$`, world.oneREPLSessionIsCreatedAndAssociatedToThatNode)

	ctx.Step(`^an active node with existing REPL session state$`, world.anActiveNodeWithExistingREPLSessionState)
	ctx.Step(`^additional continue actions execute for that node$`, world.additionalContinueActionsExecuteForThatNode)
	ctx.Step(`^subsequent actions run in the same node REPL session state$`, world.subsequentActionsRunInTheSameNodeREPLSessionState)

	ctx.Step(`^active node REPL sessions exist$`, world.activeNodeREPLSessionsExist)
	ctx.Step(`^node completes or run enters terminal state$`, world.nodeCompletesOrRunEntersTerminalState)
	ctx.Step(`^corresponding REPL sessions are closed$`, world.correspondingREPLSessionsAreClosed)

	ctx.Step(`^a continue step with non-empty continuation\.repl_code$`, world.aContinueStepWithNonemptyContinuationrepl_code)
	ctx.Step(`^harness executes action handling$`, world.harnessExecutesActionHandling)
	ctx.Step(`^continuation\.repl_code executes in the current node-local REPL session$`, world.continuationrepl_codeExecutesInTheCurrentNodelocalREPLSession)

	ctx.Step(`^a node-local REPL session is initialized$`, world.aNodelocalREPLSessionIsInitialized)
	ctx.Step(`^REPL bindings are inspected$`, world.rEPLBindingsAreInspected)
	ctx.Step(`^rlm_query\(prompt, context\) is available and returns answer plus error$`, world.rlm_querypromptContextIsAvailableAndReturnsAnswerPlusError)

	ctx.Step(`^parent node depth and run max recursion depth permit recursion$`, world.parentNodeDepthAndRunMaxRecursionDepthPermitRecursion)
	ctx.Step(`^child node is created and executed$`, world.childNodeIsCreatedAndExecuted)
	ctx.Step(`^typed depth-limit error is returned and child node is not created$`, world.typedDepthlimitErrorIsReturnedAndChildNodeIsNotCreated)

	ctx.Step(`^a continue action fails with non-fatal REPL execution error$`, world.aContinueActionFailsWithNonfatalREPLExecutionError)
	ctx.Step(`^action failure is handled$`, world.actionFailureIsHandled)
	ctx.Step(`^action failure is recorded and node execution continues to next step$`, world.actionFailureIsRecordedAndNodeExecutionContinuesToNextStep)

	ctx.Step(`^a continue action exceeding (\d+) seconds execution time$`, world.aContinueActionExceedingSecondsExecutionTime)
	ctx.Step(`^REPL runtime enforces guardrails$`, world.rEPLRuntimeEnforcesGuardrails)
	ctx.Step(`^action times out with typed timeout error$`, world.actionTimesOutWithTypedTimeoutError)

	ctx.Step(`^a continue action with repl_code payload larger than (\d+) bytes$`, world.aContinueActionWithRepl_codePayloadLargerThanBytes)
	ctx.Step(`^payload guardrails are validated$`, world.payloadGuardrailsAreValidated)
	ctx.Step(`^action is rejected with typed code-size error$`, world.actionIsRejectedWithTypedCodesizeError)

	ctx.Step(`^an action execution producing stdout or stderr over (\d+) bytes$`, world.anActionExecutionProducingStdoutOrStderrOverBytes)
	ctx.Step(`^output capture guardrails are enforced$`, world.outputCaptureGuardrailsAreEnforced)
	ctx.Step(`^outputs are truncated with deterministic truncation marker$`, world.outputsAreTruncatedWithDeterministicTruncationMarker)

	ctx.Step(`^continue action code imports blocked packages$`, world.continueActionCodeImportsBlockedPackages)
	ctx.Step(`^REPL import policy validation executes$`, world.rEPLImportPolicyValidationExecutes)
	ctx.Step(`^action fails with typed import-blocked error$`, world.actionFailsWithTypedImportblockedError)

	ctx.Step(`^an action execution completes or fails$`, world.anActionExecutionCompletesOrFails)
	ctx.Step(`^action artifact persistence executes$`, world.actionArtifactPersistenceExecutes)
	ctx.Step(`^artifact is persisted and node\.action\.executed\.output_ref is set to canonical artifact reference$`, world.artifactIsPersistedAndNodeactionexecutedoutput_refIsSetToCanonicalArtifactReference)

	ctx.Step(`^fatal REPL infrastructure failure occurs$`, world.fatalREPLInfrastructureFailureOccurs)
	ctx.Step(`^harness handles failure propagation$`, world.harnessHandlesFailurePropagation)
	ctx.Step(`^run transitions to failed with typed error metadata$`, world.runTransitionsToFailedWithTypedErrorMetadata)

	registerRLMBoundedInputSteps(ctx, world)
}

func (w *harnessWorld) rlm() *rlmAcceptanceState {
	if w.rlmState == nil {
		w.rlmState = &rlmAcceptanceState{runMaxDepth: 3}
	}
	return w.rlmState
}

func (w *harnessWorld) runsBaseDir() string {
	return filepath.Join(w.workingDir, ".sigil", "runs")
}

func (w *harnessWorld) ensureRuntime(maxDepth int, factory sigilrepl.SessionFactory) error {
	if w.lifecycle != nil {
		_ = w.lifecycle.Close()
	}

	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir: w.runsBaseDir(),
		MaxDepth:    maxDepth,
	})
	if err != nil {
		return err
	}
	if err := lifecycle.StartExecution(); err != nil {
		return err
	}

	rootNode, err := lifecycle.RootNode()
	if err != nil {
		return err
	}

	if factory == nil {
		factory = sigilrepl.NewFactory()
	}
	manager, err := sigilharness.NewREPLSessionManager(factory)
	if err != nil {
		return err
	}
	artifactStore, err := sigilharness.NewActionArtifactStore(w.runsBaseDir())
	if err != nil {
		return err
	}
	stepExecutor, err := sigilharness.NewStepExecutor(lifecycle, manager, artifactStore)
	if err != nil {
		return err
	}

	state := w.rlm()
	w.lifecycle = lifecycle
	w.lastCreatedNode = rootNode
	state.runsBaseDir = w.runsBaseDir()
	state.sessionManager = manager
	state.artifactStore = artifactStore
	state.stepExecutor = stepExecutor
	state.activeNode = rootNode
	state.parentNode = rootNode
	state.childNode = sigilruntime.Node{}
	state.activeStepID = ""
	state.actionPayload = sigilruntime.NodeActionExecutedPayload{}
	state.actionErr = nil
	state.queryResult = ""
	state.queryErr = nil
	state.nonRecursive = false
	state.continuationCode = ""
	state.outputValidationErr = nil
	state.fatalErr = nil
	state.bindingResult = ""
	return nil
}

func (w *harnessWorld) ensureParentNodeAtDepth(depth int, maxDepth int) error {
	if maxDepth < depth {
		maxDepth = depth
	}
	if err := w.ensureRuntime(maxDepth, nil); err != nil {
		return err
	}

	parent := w.rlm().activeNode
	for parent.Depth < depth {
		childNode, err := w.lifecycle.CreateChildNode(parent.ID)
		if err != nil {
			return err
		}
		parent = childNode
	}
	w.rlm().parentNode = parent
	w.rlm().parentDepth = depth
	return nil
}

func (w *harnessWorld) executeContinueAction(code string) error {
	state := w.rlm()
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle before continue action execution")
	}
	if state.stepExecutor == nil {
		return fmt.Errorf("expected step executor before continue action execution")
	}

	nodeID := state.activeNode.ID
	if strings.TrimSpace(nodeID) == "" {
		nodeID = state.parentNode.ID
	}
	if strings.TrimSpace(nodeID) == "" {
		rootNode, err := w.lifecycle.RootNode()
		if err != nil {
			return err
		}
		nodeID = rootNode.ID
		state.activeNode = rootNode
	}

	stepStarted, err := w.lifecycle.AppendNodeStepStarted(nodeID)
	if err != nil {
		return err
	}
	state.activeStepID = stepStarted.StepID
	state.continuationCode = code

	queryFunc := func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
		parent := state.parentNode
		if strings.TrimSpace(parent.ID) == "" {
			parent = state.activeNode
		}
		if strings.TrimSpace(parent.ID) == "" {
			var lookupErr error
			parent, lookupErr = w.lifecycle.RootNode()
			if lookupErr != nil {
				return "", sigilrepl.WrapError(sigilrepl.ErrorCodeChildFailure, "failed to resolve parent node", lookupErr)
			}
		}

		childNode, childErr := w.lifecycle.CreateChildNode(parent.ID)
		if childErr != nil {
			if errors.Is(childErr, sigilruntime.ErrDepthLimitExceeded) {
				return "", sigilrepl.WrapError(sigilrepl.ErrorCodeChildDepthLimit, "rlm_query depth limit exceeded", childErr)
			}
			return "", sigilrepl.WrapError(sigilrepl.ErrorCodeChildFailure, "rlm_query child node creation failed", childErr)
		}
		state.childNode = childNode

		answer := state.childFinalAnswer
		if strings.TrimSpace(answer) == "" {
			answer = request.Prompt + "|" + request.Context
		}
		if err := w.lifecycle.CompleteNode(childNode.ID, nil); err != nil {
			return "", sigilrepl.WrapError(sigilrepl.ErrorCodeChildFailure, "failed to complete child node", err)
		}

		return answer, nil
	}

	payload, execErr := state.stepExecutor.ExecuteContinueAction(context.Background(), sigilharness.ContinueActionInput{
		NodeID:   nodeID,
		StepID:   stepStarted.StepID,
		Code:     code,
		Context:  "root-context",
		Subcalls: acceptanceSubcalls{query: queryFunc},
	})
	state.actionPayload = payload
	state.actionErr = execErr

	if execErr == nil {
		if err := w.lifecycle.AppendNodeStepCompleted(nodeID, sigilruntime.NodeStepCompletedPayload{
			StepID:      stepStarted.StepID,
			Decision:    sigilruntime.StepDecisionContinue,
			ActionCount: 1,
			DurationMS:  payload.DurationMS,
		}); err != nil {
			return err
		}
	}

	if execErr == nil {
		artifact, artifactPath, artifactErr := w.readActionArtifact(payload.OutputRef)
		if artifactErr == nil {
			state.artifactPath = artifactPath
			if strings.TrimSpace(artifact.Stdout) != "" {
				state.queryResult = artifact.Stdout
			}
		}
	}

	return execErr
}

func (w *harnessWorld) readActionArtifact(outputRef string) (sigilharness.ActionArtifact, string, error) {
	parsed, err := sigilruntime.ParseActionOutputRef(outputRef)
	if err != nil {
		return sigilharness.ActionArtifact{}, "", err
	}
	artifactPath := filepath.Join(
		w.rlm().runsBaseDir,
		w.lifecycle.RunID(),
		"artifacts",
		"node",
		parsed.NodeID,
		"step",
		parsed.StepID,
		fmt.Sprintf("action-%d.json", parsed.ActionIndex),
	)

	bytes, err := os.ReadFile(filepath.Clean(artifactPath))
	if err != nil {
		return sigilharness.ActionArtifact{}, "", err
	}
	var artifact sigilharness.ActionArtifact
	if err := json.Unmarshal(bytes, &artifact); err != nil {
		return sigilharness.ActionArtifact{}, "", err
	}

	return artifact, artifactPath, nil
}

func (w *harnessWorld) providerFromRunConfig() (string, error) {
	if err := config.InitRunFromPath(w.resolvedRunConfigPath); err != nil {
		return "", err
	}
	return strings.TrimSpace(config.MustGetRun().LLM.Provider), nil
}

func (w *harnessWorld) harnessBaseSystemPromptResolutionRuns() error {
	state := w.rlm()
	provider := strings.TrimSpace(state.providerForPrompt)
	if provider == "" {
		resolvedProvider, err := w.providerFromRunConfig()
		if err != nil {
			provider = "openai"
		} else {
			provider = resolvedProvider
		}
	}
	resolver := sigilharness.NewSystemPromptResolver()
	resolvedProvider, resolvedPrompt, err := resolver.ResolveBase(provider)
	if err != nil {
		return err
	}
	state.resolvedProvider, state.resolvedPrompt = resolvedProvider, resolvedPrompt
	return nil
}

func (w *harnessWorld) resolvedBaseSystemPromptIs(expected string) error {
	state := w.rlm()
	normalized := strings.TrimSpace(expected)
	if strings.TrimSpace(state.resolvedProvider) == "" {
		resolver := sigilharness.NewSystemPromptResolver()
		resolvedProvider, resolvedPrompt, err := resolver.ResolveBase(normalized)
		if err != nil {
			return err
		}
		state.resolvedProvider, state.resolvedPrompt = resolvedProvider, resolvedPrompt
		return nil
	}

	if state.resolvedProvider != normalized {
		return fmt.Errorf("expected resolved base system prompt provider %q, got %q", normalized, state.resolvedProvider)
	}
	return nil
}

func (w *harnessWorld) unregisteredProviderKeyForSystemPromptResolution(provider string) error {
	w.rlm().providerForPrompt = strings.TrimSpace(provider)
	return nil
}

func (w *harnessWorld) runConfigSystem_prompt_appendIs(value string) error {
	w.rlm().systemPromptAppend = value
	return nil
}

func (w *harnessWorld) harnessEffectiveSystemPromptIsConstructed() error {
	state := w.rlm()
	resolver := sigilharness.NewSystemPromptResolver()
	provider := state.resolvedProvider
	if strings.TrimSpace(provider) == "" {
		if err := w.harnessBaseSystemPromptResolutionRuns(); err != nil {
			return err
		}
		provider = state.resolvedProvider
	}
	_, effectivePrompt, err := resolver.ResolveEffective(provider, state.systemPromptAppend)
	if err != nil {
		return err
	}
	state.effectivePrompt = effectivePrompt
	return nil
}

func (w *harnessWorld) effectiveSystemPromptEqualsBasePromptPlusTwoNewlinesPlusAppendText() error {
	state := w.rlm()
	expected := state.resolvedPrompt + "\n\n" + state.systemPromptAppend
	if state.effectivePrompt != expected {
		return fmt.Errorf("expected effective system prompt %q, got %q", expected, state.effectivePrompt)
	}
	return nil
}

func (w *harnessWorld) effectiveSystemPromptEqualsResolvedBasePrompt() error {
	state := w.rlm()
	if state.effectivePrompt != state.resolvedPrompt {
		return fmt.Errorf("expected effective prompt to equal base prompt %q, got %q", state.resolvedPrompt, state.effectivePrompt)
	}
	return nil
}

func (w *harnessWorld) harnessExecutionStarts() error {
	return w.executionStarts()
}

func (w *harnessWorld) exactlyOneRootNodeExistsWithDepthAndNullParent(expectedDepth int) error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before root-node assertion")
	}
	rootNode, err := w.lifecycle.RootNode()
	if err != nil {
		return err
	}
	if rootNode.Depth != expectedDepth {
		return fmt.Errorf("expected root depth %d, got %d", expectedDepth, rootNode.Depth)
	}
	if rootNode.ParentNodeID != nil {
		return fmt.Errorf("expected root parent_node_id nil, got %v", rootNode.ParentNodeID)
	}
	w.rlm().activeNode = rootNode
	return nil
}

func (w *harnessWorld) anActiveNodeInHarnessExecution() error {
	return w.ensureRuntime(3, nil)
}

func (w *harnessWorld) oneInferenceRequestAndResponseHandlingCycleComplete() error {
	rootNode, err := w.lifecycle.RootNode()
	if err != nil {
		return err
	}
	step, err := w.lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		return err
	}
	w.rlm().activeStepID = step.StepID
	return w.lifecycle.AppendNodeStepCompleted(rootNode.ID, sigilruntime.NodeStepCompletedPayload{
		StepID:      step.StepID,
		Decision:    sigilruntime.StepDecisionFinal,
		ActionCount: 0,
		DurationMS:  1,
	})
}

func (w *harnessWorld) exactlyOneNodelocalStepIsRecorded() error {
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	rootNode, err := w.lifecycle.RootNode()
	if err != nil {
		return err
	}
	count := 0
	for _, event := range events {
		if event.Type != sigilruntime.EventTypeNodeStepStarted {
			continue
		}
		if event.NodeID != nil && *event.NodeID == rootNode.ID {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected one node-local step, got %d", count)
	}
	return nil
}

func (w *harnessWorld) aNodelocalStepIsInProgress() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	rootNode, err := w.lifecycle.RootNode()
	if err != nil {
		return err
	}
	step, err := w.lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		return err
	}
	w.rlm().activeStepID = step.StepID
	w.rlm().activeNode = rootNode
	return nil
}

func (w *harnessWorld) transcriptContributionsArePersistedForThatStep() error {
	state := w.rlm()
	if strings.TrimSpace(state.activeStepID) == "" {
		return fmt.Errorf("expected active step before recording transcript contributions")
	}
	nodeID := state.activeNode.ID
	if err := w.lifecycle.AppendNodeTurn(nodeID, sigilruntime.TurnRoleUser, state.activeStepID, "run-output://turn/user"); err != nil {
		return err
	}
	return w.lifecycle.AppendNodeTurn(nodeID, sigilruntime.TurnRoleModel, state.activeStepID, "run-output://turn/model")
}

func (w *harnessWorld) eachTurnContributionIsRecordedWithRoleUserOrModel() error {
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	state := w.rlm()
	seenUser := false
	seenModel := false
	for _, event := range events {
		if event.Type != sigilruntime.EventTypeNodeTurnUser && event.Type != sigilruntime.EventTypeNodeTurnModel {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeTurnPayload)
		if !ok || payload.StepID != state.activeStepID {
			continue
		}
		if payload.Role == sigilruntime.TurnRoleUser {
			seenUser = true
		}
		if payload.Role == sigilruntime.TurnRoleModel {
			seenModel = true
		}
	}
	if !seenUser || !seenModel {
		return fmt.Errorf("expected both user and model turns for step %q", state.activeStepID)
	}
	return nil
}

func (w *harnessWorld) aNodelocalStepWithDecisionContinue() error {
	if err := w.aNodelocalStepIsInProgress(); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; fmt.Print("ok")`
	return nil
}

func (w *harnessWorld) continuationPayloadIsValidated() error {
	state := w.rlm()
	if strings.TrimSpace(state.continuationCode) == "" {
		state.outputValidationErr = sigilinference.NewError(sigilinference.ErrorCodeOutputValidation, "continuation.repl_code is required")
		return nil
	}
	state.outputValidationErr = nil
	return nil
}

func (w *harnessWorld) exactlyOneExecutableActionIsAcceptedForThatStep() error {
	if w.rlm().outputValidationErr != nil {
		return fmt.Errorf("expected valid continue payload, got %v", w.rlm().outputValidationErr)
	}
	return nil
}

func (w *harnessWorld) anActiveNodeWithContinuationPayloadContainingNonemptyContinuationrepl_code() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; fmt.Print("continue")`
	return nil
}

func (w *harnessWorld) harnessProcessesContinuationStep() error {
	if err := w.continuationPayloadIsValidated(); err != nil {
		return err
	}
	if w.rlm().outputValidationErr != nil {
		return nil
	}
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) exactlyOneContinuationrepl_codeActionExecutesInNodelocalREPLState() error {
	if w.rlm().actionErr != nil {
		return fmt.Errorf("expected non-fatal continue action execution, got %v", w.rlm().actionErr)
	}
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	count := 0
	for _, event := range events {
		if event.Type != sigilruntime.EventTypeNodeActionExecuted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeActionExecutedPayload)
		if ok && payload.StepID == w.rlm().activeStepID {
			count++
		}
	}
	if count != 1 {
		return fmt.Errorf("expected one node.action.executed event for step %q, got %d", w.rlm().activeStepID, count)
	}
	return nil
}

func (w *harnessWorld) anActiveNodeContinuationPayloadWithDecisionContinueAndMissingContinuationrepl_code() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = ""
	return nil
}

func (w *harnessWorld) strictOutputValidationRuns() error {
	definition, err := sigilschema.NewRegistry().Resolve(sigilschema.SigilRLMResponseV1SchemaID)
	if err != nil {
		return err
	}
	payload := map[string]any{"decision": "continue", "continuation": map[string]any{}}
	if strings.TrimSpace(w.rlm().continuationCode) != "" {
		payload["continuation"] = map[string]any{
			"repl_code":            w.rlm().continuationCode,
			"intent":               "inspect context chunk",
			"expected_observation": "needle appears in output",
		}
	}
	if err := definition.Validate(payload); err != nil {
		w.rlm().outputValidationErr = sigilinference.WrapError(sigilinference.ErrorCodeOutputValidation, "structured output failed strict schema validation", err)
		return nil
	}
	w.rlm().outputValidationErr = nil
	return nil
}

func (w *harnessWorld) continuationStepFailsWithTypedOutputvalidationError() error {
	if !sigilinference.IsCode(w.rlm().outputValidationErr, sigilinference.ErrorCodeOutputValidation) {
		return fmt.Errorf("expected typed output_validation error, got %v", w.rlm().outputValidationErr)
	}
	return nil
}

func (w *harnessWorld) anActiveParentNodeAtDepth(depth int) error {
	w.rlm().parentDepth = depth
	maxDepth := depth + 1
	if w.rlm().runMaxDepth > 0 {
		maxDepth = w.rlm().runMaxDepth
	}
	return w.ensureParentNodeAtDepth(depth, maxDepth)
}

func (w *harnessWorld) runMaxRecursionDepthIs(maxDepth int) error {
	w.rlm().runMaxDepth = maxDepth
	return w.ensureParentNodeAtDepth(w.rlm().parentDepth, maxDepth)
}

func (w *harnessWorld) rlm_queryIsInvokedFromNodelocalGoREPLContext() error {
	state := w.rlm()
	if strings.TrimSpace(state.parentNode.ID) == "" {
		return fmt.Errorf("expected active parent node before rlm_query invocation")
	}

	if state.nonRecursive {
		state.queryErr = sigilrepl.WrapError(sigilrepl.ErrorCodeChildDepthLimit, "rlm_query is disabled in non-recursive mode", nil)
		state.queryResult = ""
		state.childNode = sigilruntime.Node{}
		return nil
	}

	childNode, err := w.lifecycle.CreateChildNode(state.parentNode.ID)
	if err != nil {
		if errors.Is(err, sigilruntime.ErrDepthLimitExceeded) {
			state.queryErr = nil
			state.queryResult = "fallback answer"
			state.childNode = sigilruntime.Node{}
			return nil
		}
		state.queryErr = sigilrepl.WrapError(sigilrepl.ErrorCodeChildFailure, "rlm_query child creation failed", err)
		state.queryResult = ""
		return nil
	}

	state.childNode = childNode
	state.queryErr = nil
	state.queryResult = state.childFinalAnswer
	if strings.TrimSpace(state.queryResult) == "" {
		state.queryResult = "child answer"
	}
	return nil
}

func (w *harnessWorld) childNodeIsCreatedAtDepth(expectedDepth int) error {
	if strings.TrimSpace(w.rlm().childNode.ID) == "" {
		return fmt.Errorf("expected child node to be created")
	}
	if w.rlm().childNode.Depth != expectedDepth {
		return fmt.Errorf("expected child node depth %d, got %d", expectedDepth, w.rlm().childNode.Depth)
	}
	return nil
}

func (w *harnessWorld) childNodeCreationIsRejectedDueToDepthLimit() error {
	if !sigilrepl.IsCode(w.rlm().queryErr, sigilrepl.ErrorCodeChildDepthLimit) {
		return fmt.Errorf("expected typed depth-limit error, got %v", w.rlm().queryErr)
	}
	return nil
}

func (w *harnessWorld) deterministicDepthlimitFeedbackIsReturnedToCallerREPLContext() error {
	if w.rlm().queryErr == nil {
		return fmt.Errorf("expected depth-limit query error")
	}
	if !strings.Contains(strings.ToLower(w.rlm().queryErr.Error()), "depth") {
		return fmt.Errorf("expected deterministic depth-limit feedback, got %v", w.rlm().queryErr)
	}
	return nil
}

func (w *harnessWorld) plainSubcallFallbackAnswerIsReturnedAndChildNodeIsNotCreated() error {
	if w.rlm().queryErr != nil {
		return fmt.Errorf("expected fallback without query error, got %v", w.rlm().queryErr)
	}
	if strings.TrimSpace(w.rlm().queryResult) == "" {
		return fmt.Errorf("expected non-empty fallback answer")
	}
	if strings.TrimSpace(w.rlm().childNode.ID) != "" {
		return fmt.Errorf("expected no child node on fallback, got %q", w.rlm().childNode.ID)
	}
	return nil
}

func (w *harnessWorld) anActiveParentNodeWithChildNodeInProgress() error {
	if err := w.ensureParentNodeAtDepth(1, 3); err != nil {
		return err
	}
	childNode, err := w.lifecycle.CreateChildNode(w.rlm().parentNode.ID)
	if err != nil {
		return err
	}
	w.rlm().childNode = childNode
	return nil
}

func (w *harnessWorld) childNodeInferenceResultIsDecisionFinalWithAnswer(answer string) error {
	w.rlm().childFinalAnswer = answer
	return nil
}

func (w *harnessWorld) childNodeCompletes() error {
	if strings.TrimSpace(w.rlm().childNode.ID) == "" {
		return fmt.Errorf("expected child node before completion")
	}
	outputRef := "run-output://child/final-answer"
	if err := w.lifecycle.CompleteNode(w.rlm().childNode.ID, &outputRef); err != nil {
		return err
	}
	w.rlm().queryResult = w.rlm().childFinalAnswer
	return nil
}

func (w *harnessWorld) callerREPLContextReceivesRlm_queryResult(expected string) error {
	if w.rlm().queryResult != expected {
		return fmt.Errorf("expected caller REPL result %q, got %q", expected, w.rlm().queryResult)
	}
	return nil
}

func (w *harnessWorld) childFinalAnswerIsReturnedToCallerREPLContext() error {
	if w.rlm().queryResult != w.rlm().childFinalAnswer {
		return fmt.Errorf("expected child final answer %q to be returned, got %q", w.rlm().childFinalAnswer, w.rlm().queryResult)
	}
	return nil
}

func (w *harnessWorld) anActiveRootNodeInferenceResultIsDecisionFinalWithAnswer(answer string) error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().rootFinalAnswer = answer
	w.rlm().rootFinalRef = "run-output://node/root/final-answer"
	return nil
}

func (w *harnessWorld) harnessEvaluatesRootNodeStep() error {
	rootNode, err := w.lifecycle.RootNode()
	if err != nil {
		return err
	}
	if err := w.lifecycle.CompleteNode(rootNode.ID, &w.rlm().rootFinalRef); err != nil {
		return err
	}
	return w.lifecycle.CompleteWith(&w.rlm().rootFinalRef)
}

func (w *harnessWorld) runCompletionReferencesTerminalRootFinalOutput() error {
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != sigilruntime.EventTypeRunCompleted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunCompletedPayload)
		if !ok {
			return fmt.Errorf("expected run.completed payload type, got %T", event.Payload)
		}
		if payload.FinalAnswerRef == nil || *payload.FinalAnswerRef != w.rlm().rootFinalRef {
			return fmt.Errorf("expected run.completed final_answer_ref %q, got %v", w.rlm().rootFinalRef, payload.FinalAnswerRef)
		}
		return nil
	}
	return fmt.Errorf("expected run.completed event")
}

func (w *harnessWorld) vREPLRuntimeArchitectureRules(version int) error {
	w.rlm().runtimeVersion = version
	return nil
}

func (w *harnessWorld) rEPLEngineConfigurationIsResolved() error {
	return w.ensureRuntime(3, sigilrepl.NewFactory())
}

func (w *harnessWorld) embeddedInprocessGoInterpretationIsSelected() error {
	if w.rlm().runtimeVersion != 1 {
		return fmt.Errorf("expected REPL runtime version 1, got %d", w.rlm().runtimeVersion)
	}
	if w.rlm().stepExecutor == nil {
		return fmt.Errorf("expected step executor to be initialized")
	}
	return nil
}

func (w *harnessWorld) anActiveNodeWithNoExistingREPLSession() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	if w.rlm().sessionManager.SessionCount() != 0 {
		return fmt.Errorf("expected no existing REPL sessions")
	}
	return nil
}

func (w *harnessWorld) firstContinueActionExecutes() error {
	return w.executeContinueAction(`counter := 1`)
}

func (w *harnessWorld) oneREPLSessionIsCreatedAndAssociatedToThatNode() error {
	if w.rlm().sessionManager.SessionCount() != 1 {
		return fmt.Errorf("expected one REPL session, got %d", w.rlm().sessionManager.SessionCount())
	}
	return nil
}

func (w *harnessWorld) anActiveNodeWithExistingREPLSessionState() error {
	if err := w.anActiveNodeWithNoExistingREPLSession(); err != nil {
		return err
	}
	return w.firstContinueActionExecutes()
}

func (w *harnessWorld) additionalContinueActionsExecuteForThatNode() error {
	return w.executeContinueAction(`import "fmt"; counter = counter + 1; fmt.Print(counter)`)
}

func (w *harnessWorld) subsequentActionsRunInTheSameNodeREPLSessionState() error {
	if w.rlm().sessionManager.SessionCount() != 1 {
		return fmt.Errorf("expected one persistent REPL session, got %d", w.rlm().sessionManager.SessionCount())
	}
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.OutputRef)
	if err != nil {
		return err
	}
	if strings.TrimSpace(artifact.Stdout) != "2" {
		return fmt.Errorf("expected persisted stdout to reflect reused REPL state value 2, got %q", artifact.Stdout)
	}
	return nil
}

func (w *harnessWorld) activeNodeREPLSessionsExist() error {
	if err := w.anActiveNodeWithNoExistingREPLSession(); err != nil {
		return err
	}
	return w.firstContinueActionExecutes()
}

func (w *harnessWorld) nodeCompletesOrRunEntersTerminalState() error {
	nodeID := w.rlm().activeNode.ID
	if err := w.rlm().sessionManager.CloseNode(nodeID); err != nil {
		return err
	}
	if err := w.lifecycle.CompleteNode(nodeID, nil); err != nil {
		return err
	}
	return w.lifecycle.Complete()
}

func (w *harnessWorld) correspondingREPLSessionsAreClosed() error {
	if w.rlm().sessionManager.SessionCount() != 0 {
		return fmt.Errorf("expected all REPL sessions to be closed")
	}
	return nil
}

func (w *harnessWorld) aContinueStepWithNonemptyContinuationrepl_code() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; fmt.Print("continuation")`
	return nil
}

func (w *harnessWorld) harnessExecutesActionHandling() error {
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) continuationrepl_codeExecutesInTheCurrentNodelocalREPLSession() error {
	if w.rlm().actionPayload.Status != sigilruntime.ActionExecutionStatusCompleted {
		return fmt.Errorf("expected completed action, got %q", w.rlm().actionPayload.Status)
	}
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.OutputRef)
	if err != nil {
		return err
	}
	if strings.TrimSpace(artifact.Stdout) != "continuation" {
		return fmt.Errorf("expected continuation stdout, got %q", artifact.Stdout)
	}
	return nil
}

func (w *harnessWorld) aNodelocalREPLSessionIsInitialized() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	_, err := w.rlm().sessionManager.SessionForNode(context.Background(), sigilharness.NodeSessionInput{
		RunID:   w.lifecycle.RunID(),
		NodeID:  w.rlm().activeNode.ID,
		Depth:   w.rlm().activeNode.Depth,
		Context: "root-context",
		Bindings: acceptanceSubcalls{query: func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
			return request.Prompt + "|" + request.Context, nil
		}},
	})
	return err
}

func (w *harnessWorld) rEPLBindingsAreInspected() error {
	session, err := w.rlm().sessionManager.SessionForNode(context.Background(), sigilharness.NodeSessionInput{
		RunID:   w.lifecycle.RunID(),
		NodeID:  w.rlm().activeNode.ID,
		Depth:   w.rlm().activeNode.Depth,
		Context: "root-context",
		Bindings: acceptanceSubcalls{query: func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
			return request.Prompt + "|" + request.Context, nil
		}},
	})
	if err != nil {
		return err
	}
	result, err := session.Exec(context.Background(), `
import "fmt"
llm, err := llm_query("prompt", "subctx")
if err != nil { panic(err) }
rlm, err := rlm_query("prompt", "subctx")
if err != nil { panic(err) }
calls := []map[string]string{{"prompt":"p1","context":"c1"}}
lb, err := llm_query_batched(calls)
if err != nil { panic(err) }
rb, err := rlm_query_batched(calls)
if err != nil { panic(err) }
fmt.Print(llm + "|" + rlm + "|" + lb[0]["answer"] + "|" + rb[0]["answer"] + "|" + context)
`)
	if err != nil {
		return err
	}
	w.rlm().bindingResult = strings.TrimSpace(result.Stdout)
	return nil
}

func (w *harnessWorld) rlm_querypromptContextIsAvailableAndReturnsAnswerPlusError() error {
	expected := "prompt|subctx|prompt|subctx|p1|c1|p1|c1|root-context"
	if w.rlm().bindingResult != expected {
		return fmt.Errorf("expected binding result %q, got %q", expected, w.rlm().bindingResult)
	}
	return nil
}

func (w *harnessWorld) parentNodeDepthAndRunMaxRecursionDepthPermitRecursion() error {
	w.rlm().parentDepth = 1
	w.rlm().runMaxDepth = 3
	return w.ensureParentNodeAtDepth(1, 3)
}

func (w *harnessWorld) childNodeIsCreatedAndExecuted() error {
	if strings.TrimSpace(w.rlm().childNode.ID) == "" {
		return fmt.Errorf("expected child node to be created")
	}
	if err := w.lifecycle.CompleteNode(w.rlm().childNode.ID, nil); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) typedDepthlimitErrorIsReturnedAndChildNodeIsNotCreated() error {
	if !sigilrepl.IsCode(w.rlm().queryErr, sigilrepl.ErrorCodeChildDepthLimit) {
		return fmt.Errorf("expected typed depth-limit error, got %v", w.rlm().queryErr)
	}
	if strings.TrimSpace(w.rlm().childNode.ID) != "" {
		return fmt.Errorf("expected no child node to be created, got %q", w.rlm().childNode.ID)
	}
	return nil
}

func (w *harnessWorld) aContinueActionFailsWithNonfatalREPLExecutionError() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	return w.executeContinueAction(`panic("boom")`)
}

func (w *harnessWorld) actionFailureIsHandled() error {
	return nil
}

func (w *harnessWorld) actionFailureIsRecordedAndNodeExecutionContinuesToNextStep() error {
	if w.rlm().actionPayload.Status != sigilruntime.ActionExecutionStatusFailed {
		return fmt.Errorf("expected failed action payload, got %q", w.rlm().actionPayload.Status)
	}
	_, err := w.lifecycle.AppendNodeStepStarted(w.rlm().activeNode.ID)
	if err != nil {
		return fmt.Errorf("expected continuation to next step after non-fatal action error; %w", err)
	}
	return nil
}

func (w *harnessWorld) aContinueActionExceedingSecondsExecutionTime(_ int) error {
	factory := sigilrepl.NewFactory(sigilrepl.WithActionTimeout(20 * time.Millisecond))
	if err := w.ensureRuntime(3, factory); err != nil {
		return err
	}
	w.rlm().continuationCode = `for {}`
	return nil
}

func (w *harnessWorld) rEPLRuntimeEnforcesGuardrails() error {
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) actionTimesOutWithTypedTimeoutError() error {
	if w.rlm().actionPayload.ErrorCode == nil || *w.rlm().actionPayload.ErrorCode != string(sigilrepl.ErrorCodeExecutionTimeout) {
		return fmt.Errorf("expected timeout error code %q, got %v", sigilrepl.ErrorCodeExecutionTimeout, w.rlm().actionPayload.ErrorCode)
	}
	return nil
}

func (w *harnessWorld) aContinueActionWithRepl_codePayloadLargerThanBytes(maxBytes int) error {
	factory := sigilrepl.NewFactory(sigilrepl.WithMaxCodeBytes(maxBytes))
	if err := w.ensureRuntime(3, factory); err != nil {
		return err
	}
	w.rlm().continuationCode = strings.Repeat("a", maxBytes+1)
	return nil
}

func (w *harnessWorld) payloadGuardrailsAreValidated() error {
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) actionIsRejectedWithTypedCodesizeError() error {
	if w.rlm().actionPayload.ErrorCode == nil || *w.rlm().actionPayload.ErrorCode != string(sigilrepl.ErrorCodeCodeSizeExceeded) {
		return fmt.Errorf("expected code-size error code %q, got %v", sigilrepl.ErrorCodeCodeSizeExceeded, w.rlm().actionPayload.ErrorCode)
	}
	return nil
}

func (w *harnessWorld) anActionExecutionProducingStdoutOrStderrOverBytes(_ int) error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; for i := 0; i < 300000; i++ { fmt.Print("abcd") }`
	return nil
}

func (w *harnessWorld) outputCaptureGuardrailsAreEnforced() error {
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) outputsAreTruncatedWithDeterministicTruncationMarker() error {
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.OutputRef)
	if err != nil {
		return err
	}
	if !strings.Contains(artifact.Stdout, sigilrepl.OutputTruncationMarker) && !strings.Contains(artifact.Stderr, sigilrepl.OutputTruncationMarker) {
		return fmt.Errorf("expected truncation marker in stdout/stderr output")
	}
	return nil
}

func (w *harnessWorld) continueActionCodeImportsBlockedPackages() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "os"`
	return nil
}

func (w *harnessWorld) rEPLImportPolicyValidationExecutes() error {
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) actionFailsWithTypedImportblockedError() error {
	if w.rlm().actionPayload.ErrorCode == nil || *w.rlm().actionPayload.ErrorCode != string(sigilrepl.ErrorCodeImportBlocked) {
		return fmt.Errorf("expected import-blocked error code %q, got %v", sigilrepl.ErrorCodeImportBlocked, w.rlm().actionPayload.ErrorCode)
	}
	return nil
}

func (w *harnessWorld) anActionExecutionCompletesOrFails() error {
	if err := w.ensureRuntime(3, nil); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; fmt.Print("artifact")`
	return w.executeContinueAction(w.rlm().continuationCode)
}

func (w *harnessWorld) actionArtifactPersistenceExecutes() error {
	if strings.TrimSpace(w.rlm().actionPayload.OutputRef) == "" {
		return fmt.Errorf("expected action output_ref to be present")
	}
	return nil
}

func (w *harnessWorld) artifactIsPersistedAndNodeactionexecutedoutput_refIsSetToCanonicalArtifactReference() error {
	payload := w.rlm().actionPayload
	if strings.TrimSpace(payload.OutputRef) == "" {
		return fmt.Errorf("expected non-empty action output_ref")
	}
	parsed, err := sigilruntime.ParseActionOutputRef(payload.OutputRef)
	if err != nil {
		return err
	}
	artifactPath := filepath.Join(
		w.rlm().runsBaseDir,
		w.lifecycle.RunID(),
		"artifacts",
		"node",
		parsed.NodeID,
		"step",
		parsed.StepID,
		fmt.Sprintf("action-%d.json", parsed.ActionIndex),
	)
	if _, err := os.Stat(filepath.Clean(artifactPath)); err != nil {
		return fmt.Errorf("expected persisted action artifact %q; %w", artifactPath, err)
	}
	return nil
}

type failingSessionFactory struct {
	err error
}

func (f *failingSessionFactory) NewSession(_ context.Context, _ sigilrepl.SessionOptions) (sigilrepl.Session, error) {
	return nil, f.err
}

func (w *harnessWorld) fatalREPLInfrastructureFailureOccurs() error {
	if err := w.ensureRuntime(3, &failingSessionFactory{err: errors.New("session init failure")}); err != nil {
		return err
	}
	w.rlm().continuationCode = `import "fmt"; fmt.Print("x")`
	w.rlm().fatalErr = w.executeContinueAction(w.rlm().continuationCode)
	if w.rlm().fatalErr == nil {
		return fmt.Errorf("expected fatal REPL infrastructure error")
	}
	return nil
}

func (w *harnessWorld) harnessHandlesFailurePropagation() error {
	if w.rlm().fatalErr == nil {
		return fmt.Errorf("expected fatal infrastructure error before failure propagation")
	}
	payload := sigilruntime.RunFailedPayload{
		Status:       "failed",
		ErrorCode:    string(sigilrepl.ErrorCodeSessionInit),
		ErrorMessage: w.rlm().fatalErr.Error(),
		Retryable:    false,
	}
	w.rlm().failedPayload = payload
	return w.lifecycle.FailWith(payload)
}

func (w *harnessWorld) runTransitionsToFailedWithTypedErrorMetadata() error {
	if w.lifecycle.State() != sigilruntime.RunStateFailed {
		return fmt.Errorf("expected failed run state, got %q", w.lifecycle.State())
	}
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != sigilruntime.EventTypeRunFailed {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunFailedPayload)
		if !ok {
			return fmt.Errorf("expected run.failed payload type, got %T", event.Payload)
		}
		if payload.ErrorCode != w.rlm().failedPayload.ErrorCode {
			return fmt.Errorf("expected run.failed error_code %q, got %q", w.rlm().failedPayload.ErrorCode, payload.ErrorCode)
		}
		if strings.TrimSpace(payload.ErrorMessage) == "" {
			return fmt.Errorf("expected non-empty run.failed error_message")
		}
		return nil
	}
	return fmt.Errorf("expected run.failed event")
}
