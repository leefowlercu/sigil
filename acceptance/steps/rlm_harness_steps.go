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
	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/accounting"
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

	actionPayload      sigilruntime.NodeActionExecutedPayload
	actionErr          error
	artifactPath       string
	actionRef          string
	actionOutputResult sigilrepl.ActionOutput
	actionOutputErr    error

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

	recursiveSubcallTimeoutSeconds int
	actionTimeoutSeconds           int
	observedRecursiveDeadline      time.Duration
	recursiveCancel                context.CancelFunc
	recursiveCancelSubcallErrCh    chan error
	recursiveCancelExecErrCh       chan error
	recursiveCancelExecErr         error

	guardrailFixture          string
	guardrailRunResult        sigilharness.RunResult
	guardrailRunErr           error
	guardrailRunFailedPayload sigilruntime.RunFailedPayload
	guardrailPersistedEvents  []sigilruntime.EventEnvelope
	guardrailPayload          map[string]any
	guardrailPayloadValidated bool
	guardrailPayloadErr       error
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

func noopActionOutputRead(_ string) (sigilrepl.ActionOutput, error) {
	return sigilrepl.ActionOutput{}, nil
}

func registerRLMHarnessSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^harness base system prompt resolution runs$`, world.harnessBaseSystemPromptResolutionRuns)
	ctx.Step(`^resolved base system prompt is "([^"]*)"$`, world.resolvedBaseSystemPromptIs)
	ctx.Step(`^unregistered provider key "([^"]*)" for system prompt resolution$`, world.unregisteredProviderKeyForSystemPromptResolution)
	ctx.Step(`^run config system_prompt_append is "([^"]*)"$`, world.runConfigSystem_prompt_appendIs)
	ctx.Step(`^harness effective system prompt is constructed$`, world.harnessEffectiveSystemPromptIsConstructed)
	ctx.Step(`^effective system prompt equals base prompt plus two newlines plus append text$`, world.effectiveSystemPromptEqualsBasePromptPlusTwoNewlinesPlusAppendText)
	ctx.Step(`^effective system prompt equals resolved base prompt$`, world.effectiveSystemPromptEqualsResolvedBasePrompt)
	ctx.Step(`^openai system prompt uses block sections and hard finalization gate$`, world.openaiSystemPromptUsesBlockSectionsAndHardFinalizationGate)
	ctx.Step(`^openai system prompt includes search discipline and timeout recovery rules$`, world.openaiSystemPromptIncludesSearchDisciplineAndTimeoutRecoveryRules)
	ctx.Step(`^openai system prompt explains the plain subcall answer-string contract$`, world.openaiSystemPromptExplainsThePlainSubcallAnswerstringContract)
	ctx.Step(`^openai system prompt explains compile-safe structured prompt strings$`, world.openaiSystemPromptExplainsCompilesafeStructuredPromptStrings)
	ctx.Step(`^openai system prompt explains safe structured parsing in repl code$`, world.openaiSystemPromptExplainsSafeStructuredParsingInReplCode)
	ctx.Step(`^anthropic system prompt preserves safety rules without openai block sections$`, world.anthropicSystemPromptPreservesSafetyRulesWithoutOpenaiBlockSections)
	ctx.Step(`^system prompt requires byte-for-byte previous_action_feedback\.action_ref reuse with context_ref fallback$`, world.systemPromptRequiresByteforbytePrevious_action_feedbackaction_refReuseWithContext_refFallback)

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
	ctx.Step(`^recursive subcall timeout budget is configured to (\d+) seconds$`, world.recursiveSubcallTimeoutBudgetIsConfiguredToSeconds)
	ctx.Step(`^action timeout budget is configured to (\d+) seconds$`, world.actionTimeoutBudgetIsConfiguredToSeconds)
	ctx.Step(`^recursive subcall timeout budgets are observed$`, world.recursiveSubcallTimeoutBudgetsAreObserved)
	ctx.Step(`^recursive subcall timeout budget is independent of parent action and recursive-level elapsed deadlines$`, world.recursiveSubcallTimeoutBudgetIsIndependentOfParentActionElapsedTime)
	ctx.Step(`^recursive subcall execution is in progress$`, world.recursiveSubcallExecutionIsInProgress)
	ctx.Step(`^run context is canceled during recursive subcall$`, world.runContextIsCanceledDuringRecursiveSubcall)
	ctx.Step(`^recursive subcall execution is canceled by run context$`, world.recursiveSubcallExecutionIsCanceledByRunContext)

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
	ctx.Step(`^artifact is persisted and node\.action\.executed\.action_ref is set to canonical artifact reference$`, world.artifactIsPersistedAndNodeactionexecutedaction_refIsSetToCanonicalArtifactReference)
	ctx.Step(`^a canonical current-run action artifact with exact stdout and stderr is persisted$`, world.aCanonicalCurrentrunActionArtifactWithExactStdoutAndStderrIsPersisted)
	ctx.Step(`^read_action_artifact is invoked in REPL with that action_ref$`, world.readActionArtifactIsInvokedInREPLWithThatActionRef)
	ctx.Step(`^exact action output fields are returned to REPL context$`, world.exactActionOutputFieldsAreReturnedToREPLContext)

	ctx.Step(`^fatal REPL infrastructure failure occurs$`, world.fatalREPLInfrastructureFailureOccurs)
	ctx.Step(`^harness handles failure propagation$`, world.harnessHandlesFailurePropagation)
	ctx.Step(`^run transitions to failed with typed error metadata$`, world.runTransitionsToFailedWithTypedErrorMetadata)
	ctx.Step(`^run\.failed payload includes deterministic guardrail metadata$`, world.runfailedPayloadIncludesDeterministicGuardrailMetadata)
	ctx.Step(`^run\.failed payload includes limit_key without configured_value or observed_value$`, world.runfailedPayloadIncludesLimit_keyWithoutConfigured_valueOrObserved_value)
	ctx.Step(`^run\.failed payload includes non-uuidv7 failed_step_id$`, world.runfailedPayloadIncludesNonuuidv7Failed_step_id)
	ctx.Step(`^strict payload schema validation is executed for run\.failed$`, world.strictPayloadSchemaValidationIsExecutedForRunfailed)
	ctx.Step(`^run\.failed payload validation succeeds$`, world.runfailedPayloadValidationSucceeds)
	ctx.Step(`^run\.failed payload validation fails$`, world.runfailedPayloadValidationFails)
	ctx.Step(`^deterministic runtime guardrail fixture \"([^\"]*)\" is prepared$`, world.deterministicRuntimeGuardrailFixtureIsPrepared)
	ctx.Step(`^deterministic runtime guardrail harness run executes$`, world.deterministicRuntimeGuardrailHarnessRunExecutes)
	ctx.Step(`^deterministic runtime guardrail breach uses limit key \"([^\"]*)\"$`, world.deterministicRuntimeGuardrailBreachUsesLimitKey)
	ctx.Step(`^a user runs guardrail-breach harness entrypoint$`, world.aUserRunsGuardrailbreachHarnessEntrypoint)
	ctx.Step(`^deterministic runtime guardrail reset fixture completes successfully$`, world.deterministicRuntimeGuardrailResetFixtureCompletesSuccessfully)
	ctx.Step(`^run\.failed includes deterministic runtime guardrail metadata$`, world.runfailedIncludesDeterministicRuntimeGuardrailMetadata)
	ctx.Step(`^run\.failed includes failed_node_id and optional failed_step_id for guardrail breaches$`, world.runfailedIncludesFailed_node_idAndOptionalFailed_step_idForGuardrailBreaches)
	ctx.Step(`^deterministic runtime guardrail parity is preserved for recursive and non-recursive profiles$`, world.deterministicRuntimeGuardrailParityIsPreservedForRecursiveAndNonrecursiveProfiles)
	ctx.Step(`^max_run_duration_ms interrupts the active step before completion$`, world.max_run_duration_msInterruptsTheActiveStepBeforeCompletion)
	ctx.Step(`^deterministic runtime guardrail failure requires complete accounting for limit key \"([^\"]*)\" with status \"([^\"]*)\" and observed value \"([^\"]*)\"$`, world.deterministicRuntimeGuardrailFailureRequiresCompleteAccountingForLimitKeyWithStatusAndObservedValue)

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
		Name:        "test-run",
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
	artifactStore, err := sigilharness.NewActionArtifactStore(w.runsBaseDir())
	if err != nil {
		return err
	}
	manager, err := sigilharness.NewREPLSessionManager(factory, artifactStore)
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
	state.actionRef = ""
	state.actionOutputResult = sigilrepl.ActionOutput{}
	state.actionOutputErr = nil
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
		artifact, artifactPath, artifactErr := w.readActionArtifact(payload.ActionRef)
		if artifactErr == nil {
			state.artifactPath = artifactPath
			if strings.TrimSpace(artifact.Stdout) != "" {
				state.queryResult = artifact.Stdout
			}
		}
	}

	return execErr
}

func (w *harnessWorld) readActionArtifact(actionRef string) (sigilharness.ActionArtifact, string, error) {
	parsed, err := sigilruntime.ParseActionArtifactRef(actionRef)
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

func (w *harnessWorld) systemPromptRequiresByteforbytePrevious_action_feedbackaction_refReuseWithContext_refFallback() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"If you cite previous_action_feedback.action_ref, copy it byte-for-byte.",
		"Do not shorten, rewrite, splice, or synthesize run-artifact or run-output UUID segments.",
		"If you cannot preserve an exact action_ref, cite context_ref instead of inventing a run-artifact ref.",
		`{"ref":"run-artifact://node/019cc5fc-b991-7b33-bb66-c4e2508378f8/step/019cc5fc-b99b-7b33-bb66-c4e2508378f8/action-1.json"}`,
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) openaiSystemPromptUsesBlockSectionsAndHardFinalizationGate() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"<tool_selection>",
		"<retrieval_strategy>",
		"<citation_rules>",
		"<finalization_gate>",
		"<recovery_rules>",
		"decision=final is allowed only when all of the following are true:",
		"Do not finalize on a guess, on partial formatting, or on unsupported evidence.",
		`{"decision":"final","final":{"answer":"NONE"`,
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) openaiSystemPromptIncludesSearchDisciplineAndTimeoutRecoveryRules() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"Each continue action may perform at most 4 recursive subcalls and at most 8 total subcalls.",
		"If more expansion is needed, finish the current action, record what narrowed successfully, and use a new step before expanding again.",
		"Do NOT use rlm_query_batched for coarse search over unknown full-context partitions.",
		"If execution_state.small_context=true, solve locally with REPL or llm_query and do not call rlm_query or rlm_query_batched.",
		"If execution_state.recursive_subcalls_allowed=false, stay local for this step even if recursive APIs are available.",
		"read_action_artifact(action_ref string) (ActionOutput, error)",
		"If an action times out or previous_action_feedback.error_message indicates timeout, reduce chunk size and fan-out on the next step and prefer REPL or llm_query before more recursion.",
		"If a complete local scan of the current context finds no matching evidence, finalize absence now rather than repartitioning the same context again.",
		"include span_start or span_end only when you know exact integer offsets",
		"If stdout_preview or stderr_preview is truncated, treat the preview as partial evidence only. Call read_action_artifact(action_ref) before rescanning large context, or continue with a smaller and more targeted action.",
		"If execution_state.same_context_as_previous_step=true and previous_action_feedback.action_ref is present, ask first whether the prior action output might already contain the deliverable.",
		"Signals that the prior action likely already has the deliverable include: preview text shows the answer prefix, labeled extraction markers such as FINAL_START or FINAL_END, reported exact lengths, or found=true style indicators next to long text.",
		"When those signals are present, do NOT re-scan the full raw context first. Call read_action_artifact(previous_action_feedback.action_ref), inspect the exact stdout or stderr locally, and continue from that recovered value.",
		"If read_action_artifact(action_ref) returns the exact long string you need, assign it to a persistent REPL variable and verify its length before using a later step to finalize.",
		"After read_action_artifact recovers the exact prior output, only return to a full-context scan if that recovered output still lacks the needed data.",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) openaiSystemPromptExplainsThePlainSubcallAnswerstringContract() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"llm_query and rlm_query return a plain string answer to your Go code, not an arbitrary top-level JSON object.",
		`The harness already owns the outer {"answer":"..."} wrapper; your REPL code only receives the inner answer string.`,
		`Do NOT ask llm_query or rlm_query to emit a top-level object like {"has_token":true,"token":"...","line":"..."}.`,
		"If you need structured data from a subcall, instruct it to return minified JSON text inside the answer string, then parse that returned string in REPL with encoding/json.",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) openaiSystemPromptExplainsCompilesafeStructuredPromptStrings() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"If a prompt string needs literal JSON examples or many embedded quotes, prefer a raw string literal with backquotes.",
		"Prefer describing required JSON keys in words over embedding heavily escaped JSON examples inside double-quoted Go strings.",
		`Do NOT over-escape prompt strings with sequences like {\\"has_token\\":true} inside double-quoted Go code.`,
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) openaiSystemPromptExplainsSafeStructuredParsingInReplCode() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"For structured parsing from map[string]any, prefer predeclared variables plus assignment over compact two-value short declarations.",
		"At REPL top level, do NOT introduce ok/present/type flags with := and then reference them in later statements.",
		`hasRaw := any(nil)`,
		`hasRaw, present = parsed["has_token"]`,
		`hasTokenBool, typeOK = hasRaw.(bool)`,
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	return nil
}

func (w *harnessWorld) anthropicSystemPromptPreservesSafetyRulesWithoutOpenaiBlockSections() error {
	prompt := w.rlm().effectivePrompt
	requiredSnippets := []string{
		"read_action_artifact(action_ref string) (ActionOutput, error)",
		"Evidence rules:",
		"Finalization gate:",
		"decision=final is allowed only when the requested deliverable is obtained, final.answer matches the requested answer format, and at least one valid evidence ref directly supports the answer.",
		"If you cite previous_action_feedback.action_ref, copy it byte-for-byte.",
		"llm_query and rlm_query return a plain string answer to your Go code, not an arbitrary top-level JSON object.",
		"If you need structured data, ask the subcall to return minified JSON text inside the answer string and parse that string in REPL.",
		"If execution_state.small_context=true, solve locally with REPL or llm_query and do not call rlm_query or rlm_query_batched.",
		"If previous_action_feedback refers to the same context and its preview suggests the prior action already found or printed the target, prefer exact output recovery over another raw-context scan.",
		"On preview truncation, treat previews as partial and call read_action_artifact(action_ref) before rescanning large context.",
		"If execution_state.same_context_as_previous_step=true and previous_action_feedback.action_ref is present, first ask whether the prior action output might already contain the deliverable.",
		"When those signals are present, do not rescan the full raw context first. Call read_action_artifact(previous_action_feedback.action_ref), inspect the recovered stdout or stderr locally, and continue from that exact output.",
		"Include span_start or span_end only when you know exact integer offsets; otherwise omit span fields entirely.",
	}
	for _, snippet := range requiredSnippets {
		if !strings.Contains(prompt, snippet) {
			return fmt.Errorf("expected effective system prompt to include %q", snippet)
		}
	}
	if strings.Contains(prompt, "<tool_selection>") {
		return fmt.Errorf("expected anthropic prompt to omit openai block sections, got %q", prompt)
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
	if err := w.lifecycle.AppendNodeTurn(nodeID, sigilruntime.TurnRoleUser, state.activeStepID, "run-artifact://turn/user"); err != nil {
		return err
	}
	return w.lifecycle.AppendNodeTurn(nodeID, sigilruntime.TurnRoleModel, state.activeStepID, "run-artifact://turn/model")
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
	actionRef := "run-artifact://child/final-answer"
	if err := w.lifecycle.CompleteNode(w.rlm().childNode.ID, &actionRef); err != nil {
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
	w.rlm().rootFinalRef = "run-artifact://node/root/final-answer"
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
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.ActionRef)
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
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.ActionRef)
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

func (w *harnessWorld) recursiveSubcallTimeoutBudgetIsConfiguredToSeconds(seconds int) error {
	if seconds < 1 {
		return fmt.Errorf("recursive subcall timeout seconds must be >= 1")
	}
	w.rlm().recursiveSubcallTimeoutSeconds = seconds
	return nil
}

func (w *harnessWorld) actionTimeoutBudgetIsConfiguredToSeconds(seconds int) error {
	if seconds < 1 {
		return fmt.Errorf("action timeout seconds must be >= 1")
	}
	w.rlm().actionTimeoutSeconds = seconds
	return nil
}

func (w *harnessWorld) recursiveSubcallTimeoutBudgetsAreObserved() error {
	subcallSeconds := w.rlm().recursiveSubcallTimeoutSeconds
	if subcallSeconds < 1 {
		subcallSeconds = 90
	}
	actionSeconds := w.rlm().actionTimeoutSeconds
	if actionSeconds < 1 {
		actionSeconds = 180
	}

	factory := sigilrepl.NewFactory(
		sigilrepl.WithActionTimeout(time.Duration(actionSeconds)*time.Second),
		sigilrepl.WithRecursiveSubcallTimeout(time.Duration(subcallSeconds)*time.Second),
	)
	runID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate run id; %w", err)
	}
	nodeID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate node id; %w", err)
	}

	session, err := factory.NewSession(context.Background(), sigilrepl.SessionOptions{
		RunID:   runID.String(),
		NodeID:  nodeID.String(),
		Depth:   0,
		Context: "root-context",
		LLMQuery: func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
			return request.Prompt + "|" + request.Context, nil
		},
		RLMQuery: func(ctx context.Context, request sigilrepl.QueryRequest) (string, error) {
			deadline, ok := ctx.Deadline()
			if !ok {
				return "", fmt.Errorf("recursive subcall context missing deadline")
			}
			w.rlm().observedRecursiveDeadline = time.Until(deadline)
			return request.Prompt + "|" + request.Context, nil
		},
		LLMQueryBatched: func(_ context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			results := make([]sigilrepl.BatchedQueryResult, len(requests))
			for index, request := range requests {
				results[index] = sigilrepl.BatchedQueryResult{Answer: request.Prompt + "|" + request.Context}
			}
			return results, nil
		},
		RLMQueryBatched: func(ctx context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			results := make([]sigilrepl.BatchedQueryResult, len(requests))
			deadline, ok := ctx.Deadline()
			if !ok {
				return nil, fmt.Errorf("recursive subcall context missing deadline")
			}
			w.rlm().observedRecursiveDeadline = time.Until(deadline)
			for index, request := range requests {
				results[index] = sigilrepl.BatchedQueryResult{Answer: request.Prompt + "|" + request.Context}
			}
			return results, nil
		},
		ReadActionArtifact: noopActionOutputRead,
	})
	if err != nil {
		return fmt.Errorf("failed to create repl session; %w", err)
	}
	defer session.Close()

	if _, err := session.Exec(context.Background(), `_, _ = rlm_query("prompt", "subctx")`); err != nil {
		return fmt.Errorf("failed to execute recursive subcall observation action; %w", err)
	}
	return nil
}

func (w *harnessWorld) recursiveSubcallTimeoutBudgetIsIndependentOfParentActionElapsedTime() error {
	if w.rlm().observedRecursiveDeadline <= 0 {
		return fmt.Errorf("expected observed recursive deadline to be > 0, got %s", w.rlm().observedRecursiveDeadline)
	}
	if w.rlm().recursiveSubcallTimeoutSeconds > 0 {
		target := time.Duration(w.rlm().recursiveSubcallTimeoutSeconds) * time.Second
		if w.rlm().observedRecursiveDeadline < target-15*time.Second || w.rlm().observedRecursiveDeadline > target+2*time.Second {
			return fmt.Errorf("expected recursive deadline near %s, got %s", target, w.rlm().observedRecursiveDeadline)
		}
		if w.rlm().actionTimeoutSeconds > 0 {
			actionBudget := time.Duration(w.rlm().actionTimeoutSeconds) * time.Second
			if target > actionBudget && w.rlm().observedRecursiveDeadline <= actionBudget+2*time.Second {
				return fmt.Errorf(
					"expected recursive deadline %s to exceed action budget %s when recursive budget is larger",
					w.rlm().observedRecursiveDeadline,
					actionBudget,
				)
			}
		}
	}
	return nil
}

func (w *harnessWorld) recursiveSubcallExecutionIsInProgress() error {
	factory := sigilrepl.NewFactory(
		sigilrepl.WithActionTimeout(180*time.Second),
		sigilrepl.WithRecursiveSubcallTimeout(300*time.Second),
	)
	runID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate run id; %w", err)
	}
	nodeID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate node id; %w", err)
	}

	started := make(chan struct{})
	subcallErrCh := make(chan error, 1)
	execErrCh := make(chan error, 1)
	session, err := factory.NewSession(context.Background(), sigilrepl.SessionOptions{
		RunID:   runID.String(),
		NodeID:  nodeID.String(),
		Depth:   0,
		Context: "root-context",
		LLMQuery: func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
			return request.Prompt + "|" + request.Context, nil
		},
		RLMQuery: func(ctx context.Context, _ sigilrepl.QueryRequest) (string, error) {
			close(started)
			<-ctx.Done()
			subcallErrCh <- ctx.Err()
			return "", ctx.Err()
		},
		LLMQueryBatched: func(_ context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			results := make([]sigilrepl.BatchedQueryResult, len(requests))
			for index := range requests {
				results[index] = sigilrepl.BatchedQueryResult{}
			}
			return results, nil
		},
		RLMQueryBatched: func(_ context.Context, requests []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			results := make([]sigilrepl.BatchedQueryResult, len(requests))
			for index := range requests {
				results[index] = sigilrepl.BatchedQueryResult{}
			}
			return results, nil
		},
		ReadActionArtifact: noopActionOutputRead,
	})
	if err != nil {
		return fmt.Errorf("failed to create repl session; %w", err)
	}

	runCtx, cancel := context.WithCancel(context.Background())
	w.rlm().recursiveCancel = cancel
	w.rlm().recursiveCancelSubcallErrCh = subcallErrCh
	w.rlm().recursiveCancelExecErrCh = execErrCh
	go func() {
		_, execErr := session.Exec(runCtx, `_, _ = rlm_query("prompt", "subctx")`)
		execErrCh <- execErr
		_ = session.Close()
	}()

	select {
	case <-started:
		return nil
	case <-time.After(1 * time.Second):
		_ = session.Close()
		cancel()
		return fmt.Errorf("recursive subcall did not start")
	}
}

func (w *harnessWorld) runContextIsCanceledDuringRecursiveSubcall() error {
	if w.rlm().recursiveCancel == nil {
		return fmt.Errorf("expected recursive cancel function to be initialized")
	}
	w.rlm().recursiveCancel()
	return nil
}

func (w *harnessWorld) recursiveSubcallExecutionIsCanceledByRunContext() error {
	if w.rlm().recursiveCancelSubcallErrCh == nil || w.rlm().recursiveCancelExecErrCh == nil {
		return fmt.Errorf("expected recursive cancel channels to be initialized")
	}

	select {
	case subcallErr := <-w.rlm().recursiveCancelSubcallErrCh:
		if !errors.Is(subcallErr, context.Canceled) {
			return fmt.Errorf("expected recursive subcall context canceled error, got %v", subcallErr)
		}
	case <-time.After(1 * time.Second):
		return fmt.Errorf("timed out waiting for recursive subcall cancellation")
	}

	select {
	case execErr := <-w.rlm().recursiveCancelExecErrCh:
		w.rlm().recursiveCancelExecErr = execErr
	case <-time.After(1 * time.Second):
		return fmt.Errorf("timed out waiting for REPL execution completion")
	}
	if w.rlm().recursiveCancelExecErr != nil && !sigilrepl.IsCode(w.rlm().recursiveCancelExecErr, sigilrepl.ErrorCodeExecutionTimeout) {
		return fmt.Errorf("expected nil or execution-timeout after run cancel, got %v", w.rlm().recursiveCancelExecErr)
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
	artifact, _, err := w.readActionArtifact(w.rlm().actionPayload.ActionRef)
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
	if strings.TrimSpace(w.rlm().actionPayload.ActionRef) == "" {
		return fmt.Errorf("expected action_ref to be present")
	}
	return nil
}

func (w *harnessWorld) artifactIsPersistedAndNodeactionexecutedaction_refIsSetToCanonicalArtifactReference() error {
	payload := w.rlm().actionPayload
	if strings.TrimSpace(payload.ActionRef) == "" {
		return fmt.Errorf("expected non-empty action_ref")
	}
	parsed, err := sigilruntime.ParseActionArtifactRef(payload.ActionRef)
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

func (w *harnessWorld) aCanonicalCurrentrunActionArtifactWithExactStdoutAndStderrIsPersisted() error {
	state := w.rlm()
	if state.artifactStore == nil {
		return fmt.Errorf("expected action artifact store to be initialized")
	}
	stepID, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("failed to generate action artifact step id; %w", err)
	}
	errorCode := "repl_execution_compile"
	errorMessage := "compile failed exactly"
	actionRef, err := state.artifactStore.Persist(sigilharness.ActionArtifact{
		RunID:        w.lifecycle.RunID(),
		NodeID:       state.activeNode.ID,
		StepID:       stepID.String(),
		ActionIndex:  1,
		ActionType:   "repl_code",
		Language:     "go",
		Status:       "failed",
		Stdout:       "exact stdout",
		Stderr:       "exact stderr",
		ErrorCode:    &errorCode,
		ErrorMessage: &errorMessage,
	})
	if err != nil {
		return fmt.Errorf("failed to persist canonical action artifact; %w", err)
	}
	state.actionRef = actionRef
	state.actionOutputResult = sigilrepl.ActionOutput{}
	state.actionOutputErr = nil
	return nil
}

func (w *harnessWorld) readActionArtifactIsInvokedInREPLWithThatActionRef() error {
	state := w.rlm()
	if strings.TrimSpace(state.actionRef) == "" {
		return fmt.Errorf("expected action_ref fixture to be initialized")
	}
	session, err := state.sessionManager.SessionForNode(context.Background(), sigilharness.NodeSessionInput{
		RunID:   w.lifecycle.RunID(),
		NodeID:  state.activeNode.ID,
		Depth:   state.activeNode.Depth,
		Context: "root-context",
		Bindings: acceptanceSubcalls{query: func(_ context.Context, request sigilrepl.QueryRequest) (string, error) {
			return request.Prompt + "|" + request.Context, nil
		}},
	})
	if err != nil {
		return err
	}
	result, err := session.Exec(context.Background(), `
import (
	"encoding/json"
	"fmt"
)
output, err := read_action_artifact("`+state.actionRef+`")
if err != nil { panic(err) }
encoded, err := json.Marshal(output)
if err != nil { panic(err) }
fmt.Print(string(encoded))
`)
	state.actionOutputErr = err
	if err != nil {
		return err
	}
	state.actionOutputResult = sigilrepl.ActionOutput{}
	if err := json.Unmarshal([]byte(result.Stdout), &state.actionOutputResult); err != nil {
		return fmt.Errorf("failed to decode read_action_artifact result; %w", err)
	}
	return nil
}

func (w *harnessWorld) exactActionOutputFieldsAreReturnedToREPLContext() error {
	if w.rlm().actionOutputErr != nil {
		return fmt.Errorf("expected read_action_artifact success, got %v", w.rlm().actionOutputErr)
	}
	expected := sigilrepl.ActionOutput{
		Status:       "failed",
		Stdout:       "exact stdout",
		Stderr:       "exact stderr",
		ErrorCode:    "repl_execution_compile",
		ErrorMessage: "compile failed exactly",
	}
	if w.rlm().actionOutputResult != expected {
		return fmt.Errorf("expected exact action output %+v, got %+v", expected, w.rlm().actionOutputResult)
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

type guardrailFixtureInference struct {
	fixture string
	calls   int
}

func (g *guardrailFixtureInference) Infer(_ context.Context, request sigilinference.Request) (sigilinference.Result, error) {
	g.calls++
	switch g.fixture {
	case "max_total_tokens":
		inputTokens := int64(6)
		outputTokens := int64(5)
		totalTokens := int64(11)
		return hydrateBoundedFinalEvidenceRef(guardrailFinalResultWithAccounting("done", accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:       "openai",
			Model:          "gpt-5.1",
			PricingVersion: "v1",
			InputTokens:    &inputTokens,
			OutputTokens:   &outputTokens,
			TotalTokens:    &totalTokens,
		})), request), nil
	case "max_total_cost_usd":
		inputTokens := int64(6)
		outputTokens := int64(5)
		totalTokens := int64(11)
		totalCost := int64(1_250_000)
		return hydrateBoundedFinalEvidenceRef(guardrailFinalResultWithAccounting("done", accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:                 "openai",
			Model:                    "gpt-5.1",
			PricingVersion:           "v1",
			InputTokens:              &inputTokens,
			OutputTokens:             &outputTokens,
			TotalTokens:              &totalTokens,
			GatewayTotalCostMicrousd: &totalCost,
		})), request), nil
	case "max_total_tokens_incomplete":
		inputTokens := int64(5)
		totalTokens := int64(5)
		return hydrateBoundedFinalEvidenceRef(guardrailFinalResultWithAccounting("done", accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:       "openai",
			Model:          "gpt-5.1",
			PricingVersion: "v1",
			InputTokens:    &inputTokens,
			TotalTokens:    &totalTokens,
		})), request), nil
	case "max_total_cost_usd_incomplete":
		inputTokens := int64(5)
		outputTokens := int64(4)
		totalTokens := int64(9)
		return hydrateBoundedFinalEvidenceRef(guardrailFinalResultWithAccounting("done", accounting.BuildLeafSummary(accounting.LeafInput{
			Provider:       "openai",
			Model:          "gpt-5.1",
			PricingVersion: "v1",
			InputTokens:    &inputTokens,
			OutputTokens:   &outputTokens,
			TotalTokens:    &totalTokens,
		})), request), nil
	case "max_total_steps_per_run":
		switch g.calls {
		case 1:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
		default:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; fmt.Print("child")`), request), nil
		}
	case "max_run_duration_ms":
		if g.calls == 1 {
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; import "time"; time.Sleep(50 * time.Millisecond); fmt.Print("after-sleep")`), request), nil
		}
		return hydrateBoundedFinalEvidenceRef(guardrailFinalResult("done"), request), nil
	case "max_consecutive_step_failures":
		return hydrateBoundedFinalEvidenceRef(guardrailContinueResult("if {"), request), nil
	case "recursive_profile_parity":
		switch g.calls {
		case 1:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; _, err := rlm_query("child prompt", "child context"); if err != nil { fmt.Print(err.Error()) }`), request), nil
		default:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; fmt.Print("child")`), request), nil
		}
	case "consecutive_failure_reset":
		switch g.calls {
		case 1:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult("if {"), request), nil
		case 2:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; fmt.Print("ok")`), request), nil
		case 3:
			return hydrateBoundedFinalEvidenceRef(guardrailContinueResult("if {"), request), nil
		default:
			return hydrateBoundedFinalEvidenceRef(guardrailFinalResult("done"), request), nil
		}
	default:
		return hydrateBoundedFinalEvidenceRef(guardrailContinueResult(`import "fmt"; fmt.Print("loop")`), request), nil
	}
}

func guardrailContinueResult(code string) sigilinference.Result {
	return sigilinference.Result{
		SchemaID: "sigil.rlm.response.v1",
		ValidatedPayload: map[string]any{
			"decision": "continue",
			"continuation": map[string]any{
				"repl_code":            code,
				"intent":               "exercise deterministic guardrail",
				"expected_observation": "guardrail accounting progresses",
			},
		},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_continue",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func guardrailContinueResultWithAccounting(code string, summary accounting.Summary) sigilinference.Result {
	result := guardrailContinueResult(code)
	result.Accounting = summary
	return result
}

func guardrailFinalResult(answer string) sigilinference.Result {
	return sigilinference.Result{
		SchemaID: "sigil.rlm.response.v1",
		ValidatedPayload: map[string]any{
			"decision": "final",
			"final": map[string]any{
				"answer":   answer,
				"evidence": []any{map[string]any{"ref": "__context_ref__"}},
			},
		},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_final",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func guardrailFinalResultWithAccounting(answer string, summary accounting.Summary) sigilinference.Result {
	result := guardrailFinalResult(answer)
	result.Accounting = summary
	return result
}

func (w *harnessWorld) runfailedPayloadIncludesDeterministicGuardrailMetadata() error {
	w.rlm().guardrailPayload = map[string]any{
		"status":           "failed",
		"error_code":       "harness_limit_exceeded",
		"error_message":    "guardrail breached",
		"failed_node_id":   mustUUIDv7StringOrPanic(),
		"failed_step_id":   mustUUIDv7StringOrPanic(),
		"limit_key":        "max_steps_per_node",
		"configured_value": "1",
		"observed_value":   "1",
		"retryable":        false,
		"accounting":       acceptanceAccountingRollup("openai", "gpt-5.1"),
	}
	w.rlm().guardrailPayloadValidated = false
	w.rlm().guardrailPayloadErr = nil
	return nil
}

func (w *harnessWorld) runfailedPayloadIncludesLimit_keyWithoutConfigured_valueOrObserved_value() error {
	w.rlm().guardrailPayload = map[string]any{
		"status":         "failed",
		"error_code":     "harness_limit_exceeded",
		"error_message":  "guardrail breached",
		"failed_node_id": mustUUIDv7StringOrPanic(),
		"limit_key":      "max_steps_per_node",
		"retryable":      false,
		"accounting":     acceptanceAccountingRollup("openai", "gpt-5.1"),
	}
	w.rlm().guardrailPayloadValidated = false
	w.rlm().guardrailPayloadErr = nil
	return nil
}

func (w *harnessWorld) runfailedPayloadIncludesNonuuidv7Failed_step_id() error {
	w.rlm().guardrailPayload = map[string]any{
		"status":           "failed",
		"error_code":       "harness_limit_exceeded",
		"error_message":    "guardrail breached",
		"failed_node_id":   mustUUIDv7StringOrPanic(),
		"failed_step_id":   "not-uuidv7",
		"limit_key":        "max_steps_per_node",
		"configured_value": "1",
		"observed_value":   "1",
		"retryable":        false,
		"accounting":       acceptanceAccountingRollup("openai", "gpt-5.1"),
	}
	w.rlm().guardrailPayloadValidated = false
	w.rlm().guardrailPayloadErr = nil
	return nil
}

func validateRunFailedPayload(payload map[string]any) error {
	eventID := mustUUIDv7StringOrPanic()
	causationID := mustUUIDv7StringOrPanic()
	runID := mustUUIDv7StringOrPanic()
	encoded, err := marshalEnvelope(map[string]any{
		"event_id":       eventID,
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           sigilruntime.EventTypeRunFailed,
		"causation_id":   causationID,
		"correlation_id": runID,
		"payload":        payload,
	})
	if err != nil {
		return err
	}
	_, err = sigilruntime.ParseEventEnvelopeStrict(encoded)
	return err
}

func (w *harnessWorld) strictPayloadSchemaValidationIsExecutedForRunfailed() error {
	if w.rlm().guardrailPayload == nil {
		return fmt.Errorf("run.failed payload fixture is required")
	}
	w.rlm().guardrailPayloadValidated = true
	w.rlm().guardrailPayloadErr = validateRunFailedPayload(w.rlm().guardrailPayload)
	return nil
}

func (w *harnessWorld) runfailedPayloadValidationSucceeds() error {
	if !w.rlm().guardrailPayloadValidated {
		return fmt.Errorf("expected run.failed payload validation to execute")
	}
	if w.rlm().guardrailPayloadErr != nil {
		return fmt.Errorf("expected run.failed payload validation success, got %v", w.rlm().guardrailPayloadErr)
	}
	return nil
}

func (w *harnessWorld) runfailedPayloadValidationFails() error {
	if !w.rlm().guardrailPayloadValidated {
		return fmt.Errorf("expected run.failed payload validation to execute")
	}
	if w.rlm().guardrailPayloadErr == nil {
		return fmt.Errorf("expected run.failed payload validation failure")
	}
	return nil
}

func (w *harnessWorld) deterministicRuntimeGuardrailFixtureIsPrepared(name string) error {
	w.rlm().guardrailFixture = name
	w.rlm().guardrailRunResult = sigilharness.RunResult{}
	w.rlm().guardrailRunErr = nil
	w.rlm().guardrailRunFailedPayload = sigilruntime.RunFailedPayload{}
	w.rlm().guardrailPersistedEvents = nil
	w.rlm().guardrailPayload = nil
	w.rlm().guardrailPayloadValidated = false
	w.rlm().guardrailPayloadErr = nil
	return nil
}

func (w *harnessWorld) deterministicRuntimeGuardrailHarnessRunExecutes() error {
	fixture := w.rlm().guardrailFixture
	if strings.TrimSpace(fixture) == "" {
		return fmt.Errorf("guardrail fixture is required")
	}

	if fixture == "recursive_profile_parity" {
		recPayload, recEvents, err := w.executeGuardrailFixture("recursive_profile_parity", false)
		if err != nil {
			return err
		}
		nonRecPayload, nonRecEvents, err := w.executeGuardrailFixture("recursive_profile_parity", true)
		if err != nil {
			return err
		}
		if recPayload.LimitKey == nil || nonRecPayload.LimitKey == nil {
			return fmt.Errorf("expected limit metadata in parity fixture payloads")
		}
		if *recPayload.LimitKey != *nonRecPayload.LimitKey {
			return fmt.Errorf("expected parity limit key match, got recursive=%q non_recursive=%q", *recPayload.LimitKey, *nonRecPayload.LimitKey)
		}
		if countEventsByType(recEvents, sigilruntime.EventTypeNodeStarted) != 2 {
			return fmt.Errorf("expected recursive guardrail fixture to create root and child nodes, got %d node.started events", countEventsByType(recEvents, sigilruntime.EventTypeNodeStarted))
		}
		if countEventsByType(nonRecEvents, sigilruntime.EventTypeNodeStarted) != 1 {
			return fmt.Errorf("expected non-recursive guardrail fixture to avoid child nodes, got %d node.started events", countEventsByType(nonRecEvents, sigilruntime.EventTypeNodeStarted))
		}
		w.rlm().guardrailRunFailedPayload = recPayload
		w.rlm().bindingResult = "parity_ok"
		return nil
	}

	payload, events, err := w.executeGuardrailFixture(fixture, false)
	if err != nil {
		return err
	}
	w.rlm().guardrailRunFailedPayload = payload
	w.rlm().guardrailPersistedEvents = events
	return nil
}

func (w *harnessWorld) executeGuardrailFixture(fixture string, nonRecursive bool) (sigilruntime.RunFailedPayload, []sigilruntime.EventEnvelope, error) {
	baseDir := w.runsBaseDir()
	w.rlm().runsBaseDir = baseDir
	existingEvents, err := filepath.Glob(filepath.Join(baseDir, "*", "events.jsonl"))
	if err != nil {
		return sigilruntime.RunFailedPayload{}, nil, err
	}
	existingSet := make(map[string]struct{}, len(existingEvents))
	for _, path := range existingEvents {
		existingSet[path] = struct{}{}
	}

	cfg := guardrailRunConfig(fixture, nonRecursive)
	runner := sigilharness.NewRunner(
		sigilharness.WithRunsBaseDir(baseDir),
		sigilharness.WithInferenceFactory(func(_ config.RunConfig) (sigilharness.InferenceClient, error) {
			return &guardrailFixtureInference{fixture: fixture}, nil
		}),
	)

	result, runErr := runner.Run(context.Background(), sigilharness.RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     cfg,
	})
	w.rlm().guardrailRunResult = result
	w.rlm().guardrailRunErr = runErr

	if fixture == "consecutive_failure_reset" {
		if runErr != nil {
			return sigilruntime.RunFailedPayload{}, nil, fmt.Errorf("expected reset fixture to complete, got %v", runErr)
		}
		return sigilruntime.RunFailedPayload{}, nil, nil
	}

	if runErr == nil {
		return sigilruntime.RunFailedPayload{}, nil, fmt.Errorf("expected guardrail fixture %q to fail run", fixture)
	}

	eventsPath := result.EventsPath
	if strings.TrimSpace(eventsPath) == "" {
		matches, err := filepath.Glob(filepath.Join(baseDir, "*", "events.jsonl"))
		if err != nil {
			return sigilruntime.RunFailedPayload{}, nil, err
		}
		candidates := make([]string, 0, len(matches))
		for _, path := range matches {
			if _, seen := existingSet[path]; !seen {
				candidates = append(candidates, path)
			}
		}
		switch {
		case len(candidates) > 0:
			eventsPath, err = newestFileByModTime(candidates)
		case len(matches) > 0:
			eventsPath, err = newestFileByModTime(matches)
		default:
			return sigilruntime.RunFailedPayload{}, nil, fmt.Errorf("failed to resolve events file for guardrail fixture %q", fixture)
		}
		if err != nil {
			return sigilruntime.RunFailedPayload{}, nil, err
		}
	}

	events, err := readEventsFromPath(eventsPath)
	if err != nil {
		return sigilruntime.RunFailedPayload{}, nil, err
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != sigilruntime.EventTypeRunFailed {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunFailedPayload)
		if !ok {
			return sigilruntime.RunFailedPayload{}, nil, fmt.Errorf("expected run.failed payload type, got %T", event.Payload)
		}
		return payload, events, nil
	}
	return sigilruntime.RunFailedPayload{}, nil, fmt.Errorf("expected run.failed payload in events")
}

func guardrailRunConfig(fixture string, nonRecursive bool) config.RunConfig {
	cfg := config.NewDefaultRunConfig()
	cfg.Name = "test-run"
	cfg.Prompt = "guardrail prompt"
	cfg.Context = "guardrail context"
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-5.1"
	cfg.LLM.Gateway = "openrouter"
	cfg.LLM.OpenRouter.BaseURL = "http://127.0.0.1:1"
	cfg.LLM.OpenRouter.APIKeyEnv = "OPENROUTER_API_KEY"
	cfg.LLM.OpenRouter.RequestTimeoutMS = 100
	cfg.RLM.Enabled = !nonRecursive
	cfg.RLM.MaxDepth = 3
	cfg.Guardrails.MaxStepsPerNode = 64
	cfg.Guardrails.MaxTotalStepsPerRun = 256
	cfg.Guardrails.MaxRunDurationMS = 1200000
	cfg.Guardrails.MaxConsecutiveStepFailures = 6

	switch fixture {
	case "max_steps_per_node":
		cfg.Guardrails.MaxStepsPerNode = 1
		cfg.Guardrails.MaxTotalStepsPerRun = 50
	case "max_total_tokens":
		cfg.Guardrails.MaxTotalTokens = int64Pointer(10)
	case "max_total_cost_usd":
		cfg.Guardrails.MaxTotalCostUSD = stringPointer("1")
	case "max_total_tokens_incomplete":
		cfg.Guardrails.MaxTotalTokens = int64Pointer(10)
	case "max_total_cost_usd_incomplete":
		cfg.Guardrails.MaxTotalCostUSD = stringPointer("1")
	case "recursive_profile_parity":
		cfg.Guardrails.MaxStepsPerNode = 1
		cfg.Guardrails.MaxTotalStepsPerRun = 50
	case "max_total_steps_per_run":
		cfg.Guardrails.MaxStepsPerNode = 2
		cfg.Guardrails.MaxTotalStepsPerRun = 2
	case "max_run_duration_ms":
		cfg.Guardrails.MaxRunDurationMS = 15
		cfg.Guardrails.MaxStepsPerNode = 10
		cfg.Guardrails.MaxTotalStepsPerRun = 20
	case "max_consecutive_step_failures":
		cfg.Guardrails.MaxConsecutiveStepFailures = 2
		cfg.Guardrails.MaxStepsPerNode = 10
	case "consecutive_failure_reset":
		cfg.Guardrails.MaxConsecutiveStepFailures = 2
		cfg.Guardrails.MaxStepsPerNode = 10
		cfg.Guardrails.MaxTotalStepsPerRun = 20
	}

	return cfg
}

func (w *harnessWorld) deterministicRuntimeGuardrailBreachUsesLimitKey(expected string) error {
	payload := w.rlm().guardrailRunFailedPayload
	if payload.LimitKey == nil {
		return fmt.Errorf("expected run.failed limit_key metadata")
	}
	if *payload.LimitKey != expected {
		return fmt.Errorf("expected limit_key %q, got %q", expected, *payload.LimitKey)
	}
	if w.rlm().guardrailFixture == "max_steps_per_node" {
		if countEventsByType(w.rlm().guardrailPersistedEvents, sigilruntime.EventTypeNodeStepStarted) != 1 {
			return fmt.Errorf("expected max_steps_per_node fixture to stop before a second node.step.started, got %d node.step.started events", countEventsByType(w.rlm().guardrailPersistedEvents, sigilruntime.EventTypeNodeStepStarted))
		}
	}
	return nil
}

func (w *harnessWorld) aUserRunsGuardrailbreachHarnessEntrypoint() error {
	if err := w.prepareGuardrailCLIEntrypointFixture(w.rlm().guardrailFixture); err != nil {
		return err
	}
	return w.aUserRuns("sigil run start")
}

func newestFileByModTime(paths []string) (string, error) {
	if len(paths) == 0 {
		return "", fmt.Errorf("no file candidates provided")
	}
	newestPath := paths[0]
	newestInfo, err := os.Stat(newestPath)
	if err != nil {
		return "", err
	}
	for _, path := range paths[1:] {
		info, statErr := os.Stat(path)
		if statErr != nil {
			return "", statErr
		}
		if info.ModTime().After(newestInfo.ModTime()) {
			newestPath = path
			newestInfo = info
		}
	}
	return newestPath, nil
}

func (w *harnessWorld) deterministicRuntimeGuardrailResetFixtureCompletesSuccessfully() error {
	if w.rlm().guardrailRunErr != nil {
		return fmt.Errorf("expected reset fixture success, got %v", w.rlm().guardrailRunErr)
	}
	if w.rlm().guardrailRunResult.State != "completed" {
		return fmt.Errorf("expected reset fixture completed run, got %q", w.rlm().guardrailRunResult.State)
	}
	return nil
}

func (w *harnessWorld) runfailedIncludesDeterministicRuntimeGuardrailMetadata() error {
	payload := w.rlm().guardrailRunFailedPayload
	if payload.ErrorCode != string(sigilharness.ErrorCodeLimitExceeded) {
		return fmt.Errorf("expected error_code %q, got %q", sigilharness.ErrorCodeLimitExceeded, payload.ErrorCode)
	}
	if payload.LimitKey == nil || strings.TrimSpace(*payload.LimitKey) == "" {
		return fmt.Errorf("expected non-empty limit_key")
	}
	if payload.ConfiguredValue == nil || strings.TrimSpace(*payload.ConfiguredValue) == "" {
		return fmt.Errorf("expected non-empty configured_value")
	}
	if payload.ObservedValue == nil || strings.TrimSpace(*payload.ObservedValue) == "" {
		return fmt.Errorf("expected non-empty observed_value")
	}
	return nil
}

func (w *harnessWorld) runfailedIncludesFailed_node_idAndOptionalFailed_step_idForGuardrailBreaches() error {
	payload := w.rlm().guardrailRunFailedPayload
	if payload.FailedNodeID == nil || strings.TrimSpace(*payload.FailedNodeID) == "" {
		return fmt.Errorf("expected non-empty failed_node_id")
	}
	if payload.FailedStepID != nil && strings.TrimSpace(*payload.FailedStepID) == "" {
		return fmt.Errorf("expected failed_step_id to be non-empty when present")
	}
	return nil
}

func (w *harnessWorld) deterministicRuntimeGuardrailParityIsPreservedForRecursiveAndNonrecursiveProfiles() error {
	if w.rlm().bindingResult != "parity_ok" {
		return fmt.Errorf("expected recursive/non-recursive guardrail parity to be established")
	}
	return nil
}

func (w *harnessWorld) max_run_duration_msInterruptsTheActiveStepBeforeCompletion() error {
	if w.rlm().guardrailFixture != "max_run_duration_ms" {
		return fmt.Errorf("max_run_duration_ms interrupt assertion requires max_run_duration_ms fixture, got %q", w.rlm().guardrailFixture)
	}
	if countEventsByType(w.rlm().guardrailPersistedEvents, sigilruntime.EventTypeNodeStepCompleted) != 0 {
		return fmt.Errorf(
			"expected duration guardrail to interrupt before node.step.completed, got %d node.step.completed events",
			countEventsByType(w.rlm().guardrailPersistedEvents, sigilruntime.EventTypeNodeStepCompleted),
		)
	}
	artifacts, err := w.guardrailActionArtifacts()
	if err != nil {
		return err
	}
	for _, artifact := range artifacts {
		if strings.Contains(artifact.Stdout, "after-sleep") {
			return fmt.Errorf("expected duration guardrail to interrupt before post-sleep output, got stdout %q", artifact.Stdout)
		}
	}
	return nil
}

func (w *harnessWorld) guardrailActionArtifacts() ([]sigilharness.ActionArtifact, error) {
	runID := strings.TrimSpace(w.rlm().guardrailRunResult.RunID)
	if runID == "" && len(w.rlm().guardrailPersistedEvents) > 0 {
		runID = strings.TrimSpace(w.rlm().guardrailPersistedEvents[0].RunID)
	}
	if runID == "" {
		return nil, fmt.Errorf("guardrail run_id is required")
	}
	artifacts := make([]sigilharness.ActionArtifact, 0, 1)
	for _, event := range w.rlm().guardrailPersistedEvents {
		if event.Type != sigilruntime.EventTypeNodeActionExecuted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeActionExecutedPayload)
		if !ok {
			return nil, fmt.Errorf("expected node.action.executed payload, got %T", event.Payload)
		}
		parsed, err := sigilruntime.ParseActionArtifactRef(payload.ActionRef)
		if err != nil {
			return nil, err
		}
		artifactPath := filepath.Join(
			w.rlm().runsBaseDir,
			runID,
			"artifacts",
			"node",
			parsed.NodeID,
			"step",
			parsed.StepID,
			fmt.Sprintf("action-%d.json", parsed.ActionIndex),
		)
		bytes, err := os.ReadFile(filepath.Clean(artifactPath))
		if err != nil {
			return nil, err
		}
		var artifact sigilharness.ActionArtifact
		if err := json.Unmarshal(bytes, &artifact); err != nil {
			return nil, err
		}
		artifacts = append(artifacts, artifact)
	}
	if len(artifacts) == 0 {
		return nil, nil
	}
	return artifacts, nil
}

func (w *harnessWorld) deterministicRuntimeGuardrailFailureRequiresCompleteAccountingForLimitKeyWithStatusAndObservedValue(limitKey string, status string, observed string) error {
	payload := w.rlm().guardrailRunFailedPayload
	if payload.LimitKey == nil || *payload.LimitKey != limitKey {
		return fmt.Errorf("expected limit_key %q, got %+v", limitKey, payload.LimitKey)
	}
	if payload.ObservedValue == nil || *payload.ObservedValue != observed {
		return fmt.Errorf("expected observed_value %q, got %+v", observed, payload.ObservedValue)
	}
	if !strings.Contains(payload.ErrorMessage, "accounting_status="+status) {
		return fmt.Errorf("expected accounting_status=%s in error message %q", status, payload.ErrorMessage)
	}
	switch limitKey {
	case "max_total_tokens":
		if payload.Accounting.TreeTotal.TokenStatus != accounting.Status(status) {
			return fmt.Errorf("expected token_status %q in run.failed accounting, got %q", status, payload.Accounting.TreeTotal.TokenStatus)
		}
	case "max_total_cost_usd":
		if payload.Accounting.TreeTotal.CostStatus != accounting.Status(status) {
			return fmt.Errorf("expected cost_status %q in run.failed accounting, got %q", status, payload.Accounting.TreeTotal.CostStatus)
		}
	default:
		return fmt.Errorf("unsupported limit key %q for incomplete accounting assertion", limitKey)
	}
	return nil
}

func (w *harnessWorld) prepareGuardrailCLIEntrypointFixture(fixture string) error {
	if strings.TrimSpace(fixture) == "" {
		return fmt.Errorf("guardrail fixture is required")
	}
	if fixture != "max_steps_per_node" {
		return fmt.Errorf("guardrail CLI fixture %q is not supported", fixture)
	}
	if w.inferenceMockServer == nil {
		w.inferenceMockServer = newOpenRouterMockServer()
	}
	w.inferenceMockServer.SetResponses(mockGatewayResponse{
		statusCode: 200,
		body:       guardrailContinueGatewayResponseBody(`import "fmt"; fmt.Print("loop")`),
	})
	if err := osSetEnv("OPENROUTER_API_KEY", "test-openrouter-key"); err != nil {
		return err
	}
	if err := osSetEnv("SIGIL_RUN_LLM_OPENROUTER_BASE_URL", w.inferenceMockServer.URL()); err != nil {
		return err
	}
	if err := osSetEnv("SIGIL_RUN_LLM_OPENROUTER_API_KEY_ENV", "OPENROUTER_API_KEY"); err != nil {
		return err
	}
	return os.WriteFile(filepath.Clean(config.DefaultRunConfigPath), []byte(guardrailRunConfigYAML(fixture, false)), 0o644)
}

func guardrailRunConfigYAML(fixture string, nonRecursive bool) string {
	cfg := guardrailRunConfig(fixture, nonRecursive)
	return fmt.Sprintf(
		"name: %s\nprompt: %s\ncontext: %s\nllm:\n  provider: %s\n  model: %s\nrlm:\n  enabled: %t\n  max_depth: %d\nguardrails:\n  max_steps_per_node: %d\n  max_total_steps_per_run: %d\n  max_run_duration_ms: %d\n  max_consecutive_step_failures: %d\n",
		cfg.Name,
		cfg.Prompt,
		cfg.Context,
		cfg.LLM.Provider,
		cfg.LLM.Model,
		cfg.RLM.Enabled,
		cfg.RLM.MaxDepth,
		cfg.Guardrails.MaxStepsPerNode,
		cfg.Guardrails.MaxTotalStepsPerRun,
		cfg.Guardrails.MaxRunDurationMS,
		cfg.Guardrails.MaxConsecutiveStepFailures,
	)
}

func int64Pointer(value int64) *int64 {
	return &value
}

func stringPointer(value string) *string {
	return &value
}

func countEventsByType(events []sigilruntime.EventEnvelope, eventType sigilruntime.EventType) int {
	count := 0
	for _, event := range events {
		if event.Type == eventType {
			count++
		}
	}
	return count
}

func guardrailContinueGatewayResponseBody(replCode string) map[string]any {
	return map[string]any{
		"id":       "resp_guardrail_continue",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": fmt.Sprintf(`{"decision":"continue","continuation":{"repl_code":%q,"intent":"exercise guardrail path","expected_observation":"loop output is recorded"}}`, replCode),
					},
				},
			},
		},
	}
}
