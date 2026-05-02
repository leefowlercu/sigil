package steps

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cucumber/godog"
	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	sigilharness "github.com/leefowlercu/sigil/internal/harness"
	sigilinference "github.com/leefowlercu/sigil/internal/inference"
	sigilschema "github.com/leefowlercu/sigil/internal/inference/schema"
	sigilrepl "github.com/leefowlercu/sigil/internal/repl"
	sigilruntime "github.com/leefowlercu/sigil/internal/runtime"
)

type boundedCaptureInference struct {
	responses []boundedInferenceResponse
	requests  []sigilinference.Request
	calls     int
}

type boundedInferenceResponse struct {
	result sigilinference.Result
	err    error
}

type markedRecursiveMapReducerInference struct {
	replCode string
	requests []sigilinference.Request
	calls    int
}

func (c *boundedCaptureInference) Infer(_ context.Context, request sigilinference.Request) (sigilinference.Result, error) {
	c.requests = append(c.requests, request)
	if c.calls >= len(c.responses) {
		return sigilinference.Result{}, fmt.Errorf("unexpected inference call")
	}
	response := c.responses[c.calls]
	c.calls++
	return hydrateBoundedFinalEvidenceRef(response.result, request), response.err
}

func (m *markedRecursiveMapReducerInference) Infer(_ context.Context, request sigilinference.Request) (sigilinference.Result, error) {
	m.requests = append(m.requests, request)
	m.calls++

	switch m.calls {
	case 1:
		return hydrateBoundedFinalEvidenceRef(boundedContinueResult(m.replCode), request), nil
	case 2:
		return hydrateBoundedFinalEvidenceRef(boundedFinalResult("child one answer"), request), nil
	case 3:
		return hydrateBoundedFinalEvidenceRef(boundedFinalResult("child two answer"), request), nil
	default:
		return sigilinference.Result{}, fmt.Errorf("unexpected inference call")
	}
}

func hydrateBoundedFinalEvidenceRef(result sigilinference.Result, request sigilinference.Request) sigilinference.Result {
	if result.SchemaID != "sigil.rlm.response.v1" {
		return result
	}
	finalPayload, ok := result.ValidatedPayload["final"].(map[string]any)
	if !ok {
		return result
	}
	evidenceRaw, ok := finalPayload["evidence"].([]any)
	if !ok || len(evidenceRaw) == 0 {
		return result
	}
	evidenceItem, ok := evidenceRaw[0].(map[string]any)
	if !ok {
		return result
	}
	refValue, _ := evidenceItem["ref"].(string)
	if refValue != "__context_ref__" {
		return result
	}
	envelope, err := decodeEnvelopeFromRequest(request)
	if err != nil {
		return result
	}
	if strings.TrimSpace(envelope.ContextMetadata.ContextRef) == "" {
		return result
	}
	evidenceItem["ref"] = envelope.ContextMetadata.ContextRef
	return result
}

func registerRLMBoundedInputSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^a harness runner is configured with raw context "([^"]*)"$`, world.aHarnessRunnerIsConfiguredWithRawContext)
	ctx.Step(`^model-step inference input is constructed for first step$`, world.modelstepInferenceInputIsConstructedForFirstStep)
	ctx.Step(`^full raw context is excluded from outbound model-step inference messages$`, world.fullRawContextIsExcludedFromOutboundModelstepInferenceMessages)
	ctx.Step(`^model-step inference messages are ordered system then user$`, world.modelstepInferenceMessagesAreOrderedSystemThenUser)
	ctx.Step(`^user step envelope contains deterministic query step index and context metadata$`, world.userStepEnvelopeContainsDeterministicQueryStepIndexAndContextMetadata)
	ctx.Step(`^user step envelope includes execution_state with depth step budgets and recursion-permission metadata$`, world.userStepEnvelopeIncludesExecution_stateWithDepthStepBudgetsAndRecursionpermissionMetadata)

	ctx.Step(`^a harness runner has previous continue action feedback$`, world.aHarnessRunnerHasPreviousContinueActionFeedback)
	ctx.Step(`^a harness runner has previous continue action subcall feedback$`, world.aHarnessRunnerHasPreviousContinueActionSubcallFeedback)
	ctx.Step(`^model-step inference input is constructed for next step$`, world.modelstepInferenceInputIsConstructedForNextStep)
	ctx.Step(`^previous-action feedback summary includes action_ref and bounded preview truncation metadata$`, world.previousActionFeedbackSummaryIncludesActionRefAndBoundedPreviewTruncationMetadata)
	ctx.Step(`^previous-action feedback includes deterministic subcall summary counts$`, world.previousactionFeedbackIncludesDeterministicSubcallSummaryCounts)
	ctx.Step(`^previous-action feedback block is omitted from user step envelope$`, world.previousactionFeedbackBlockIsOmittedFromUserStepEnvelope)
	ctx.Step(`^action artifact remains source of truth for full stdout and stderr while model input uses bounded previews$`, world.actionArtifactRemainsSourceOfTruthForFullStdoutAndStderrWhileModelInputUsesBoundedPreviews)
	ctx.Step(`^exact action stdout remains recoverable through read_action_artifact using that action_ref$`, world.exactActionStdoutRemainsRecoverableThroughReadActionArtifactUsingThatActionRef)

	ctx.Step(`^harness user turn artifact input is prepared$`, world.harnessUserTurnArtifactInputIsPrepared)
	ctx.Step(`^compact node turn user artifact is persisted$`, world.compactNodeTurnUserArtifactIsPersisted)
	ctx.Step(`^persisted node turn user artifact excludes full raw context body$`, world.persistedNodeTurnUserArtifactExcludesFullRawContextBody)

	ctx.Step(`^recursive harness child node execution input is prepared$`, world.recursiveHarnessChildNodeExecutionInputIsPrepared)
	ctx.Step(`^model-step inference input is constructed for child step$`, world.modelstepInferenceInputIsConstructedForChildStep)
	ctx.Step(`^bounded model-input contract is applied to recursive child node step$`, world.boundedModelinputContractIsAppliedToRecursiveChildNodeStep)

	ctx.Step(`^non-recursive harness mode is active$`, world.nonrecursiveHarnessModeIsActive)
	ctx.Step(`^model-step inference input is constructed for non-recursive step$`, world.modelstepInferenceInputIsConstructedForNonrecursiveStep)
	ctx.Step(`^bounded model-input contract is applied in non-recursive mode$`, world.boundedModelinputContractIsAppliedInNonrecursiveMode)
	ctx.Step(`^a small-context harness runner already used recursive subcalls in a prior continue step$`, world.aSmallcontextHarnessRunnerAlreadyUsedRecursiveSubcallsInAPriorContinueStep)
	ctx.Step(`^the next step invokes rlm_query on that same node$`, world.theNextStepInvokesRlm_queryOnThatSameNode)
	ctx.Step(`^the next-step execution state disables recursive subcalls$`, world.theNextstepExecutionStateDisablesRecursiveSubcalls)
	ctx.Step(`^repeated small-context rlm_query uses plain fallback without creating another child node$`, world.repeatedSmallcontextRlm_queryUsesPlainFallbackWithoutCreatingAnotherChildNode)

	ctx.Step(`^step-envelope serialization or persistence failure is injected$`, world.stepenvelopeSerializationOrPersistenceFailureIsInjected)
	ctx.Step(`^harness run execution handles bounded model-input failure$`, world.harnessRunExecutionHandlesBoundedModelinputFailure)
	ctx.Step(`^run fails with typed infrastructure metadata for bounded model-input failure$`, world.runFailsWithTypedInfrastructureMetadataForBoundedModelinputFailure)

	ctx.Step(`^bounded model-input execution is active for a node step$`, world.boundedModelinputExecutionIsActiveForANodeStep)
	ctx.Step(`^step and turn events are persisted under bounded model-input execution$`, world.stepAndTurnEventsArePersistedUnderBoundedModelinputExecution)
	ctx.Step(`^canonical run event ordering and references remain valid$`, world.canonicalRunEventOrderingAndReferencesRemainValid)
	ctx.Step(`^a harness runner captures provider-reported accounting for a final step$`, world.aHarnessRunnerCapturesProviderreportedAccountingForAFinalStep)
	ctx.Step(`^a harness runner captures fallback-priced accounting for a final step$`, world.aHarnessRunnerCapturesFallbackpricedAccountingForAFinalStep)
	ctx.Step(`^accounting artifacts are persisted for the completed run$`, world.accountingArtifactsArePersistedForTheCompletedRun)
	ctx.Step(`^successful run summary and terminal events include accounting$`, world.successfulRunSummaryAndTerminalEventsIncludeAccounting)
	ctx.Step(`^a recursive harness run captures subtree accounting$`, world.aRecursiveHarnessRunCapturesSubtreeAccounting)
	ctx.Step(`^accounting rollups are inspected$`, world.accountingRollupsAreInspected)
	ctx.Step(`^recursive accounting tree total includes child node totals$`, world.recursiveAccountingTreeTotalIncludesChildNodeTotals)
	ctx.Step(`^fallback pricing-derived accounting cost is preserved in completed run accounting$`, world.fallbackPricingderivedAccountingCostIsPreservedInCompletedRunAccounting)
	ctx.Step(`^subcall events and action artifacts include leaf accounting summaries$`, world.subcallEventsAndActionArtifactsIncludeLeafAccountingSummaries)
	ctx.Step(`^a harness runner captures partial accounting for a final step$`, world.aHarnessRunnerCapturesPartialAccountingForAFinalStep)
	ctx.Step(`^partial accounting totals remain marked partial instead of zero-complete$`, world.partialAccountingTotalsRemainMarkedPartialInsteadOfZerocomplete)
	ctx.Step(`^a running lifecycle captures partial terminal accounting$`, world.aRunningLifecycleCapturesPartialTerminalAccounting)
	ctx.Step(`^a failed terminal event is persisted with accounting$`, world.aFailedTerminalEventIsPersistedWithAccounting)
	ctx.Step(`^an interrupted terminal event is persisted with accounting$`, world.anInterruptedTerminalEventIsPersistedWithAccounting)
	ctx.Step(`^failed terminal events include partial accounting$`, world.failedTerminalEventsIncludePartialAccounting)
	ctx.Step(`^interrupted terminal events include partial accounting$`, world.interruptedTerminalEventsIncludePartialAccounting)
}

func (w *harnessWorld) aHarnessRunnerIsConfiguredWithRawContext(rawContext string) error {
	state := w.rlm()
	state.boundedRawContext = rawContext
	state.boundedRequests = nil
	state.boundedRunResult = sigilharness.RunResult{}
	state.boundedRunErr = nil
	state.boundedFirstEnvelope = sigilharness.StepInputEnvelope{}
	state.boundedNextEnvelope = sigilharness.StepInputEnvelope{}
	state.boundedActionArtifact = sigilharness.ActionArtifact{}
	state.boundedUserTurnArtifact = nil
	state.boundedPersistedEvents = nil
	return nil
}

func (w *harnessWorld) aRootRecursiveMapActionEmitsCompleteMarkedFinalAnswerOutput() error {
	state := w.rlm()
	state.continuationCode = `import "fmt"; calls := []map[string]string{{"prompt":"child one","context":"chunk one"},{"prompt":"child two","context":"chunk two"}}; answers, err := rlm_query_batched(calls); if err != nil { panic(err) }; fmt.Printf("COVERAGE %d / %d\n", len(answers), len(calls)); fmt.Print("FINAL_ANSWER_START\nalpha=2; beta=1\nFINAL_ANSWER_END\n")`
	state.boundedRequests = nil
	state.boundedRunResult = sigilharness.RunResult{}
	state.boundedRunErr = nil
	return nil
}

func (w *harnessWorld) harnessEvaluatesMarkedRecursiveMapReducerOutput() error {
	state := w.rlm()
	if strings.TrimSpace(state.continuationCode) == "" {
		return fmt.Errorf("marked reducer continuation code is required")
	}
	largeContext := strings.Repeat("large context line with enough bytes to exceed the small-context byte threshold\n", 40)
	runConfig := boundedRunConfig("root prompt", largeContext)
	inferenceClient := &markedRecursiveMapReducerInference{replCode: state.continuationCode}
	runner := sigilharness.NewRunner(
		sigilharness.WithRunsBaseDir(w.runsBaseDir()),
		sigilharness.WithInferenceFactory(func(_ config.RunConfig) (sigilharness.InferenceClient, error) {
			return inferenceClient, nil
		}),
	)

	result, err := runner.Run(context.Background(), sigilharness.RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
		TemplateVars:  map[string]string{},
	})
	state.boundedRunResult = result
	state.boundedRunErr = err
	state.boundedRequests = append([]sigilinference.Request(nil), inferenceClient.requests...)
	return nil
}

func (w *harnessWorld) runCompletesUsingTheMarkedReducerFinalAnswer() error {
	state := w.rlm()
	if state.boundedRunErr != nil {
		return fmt.Errorf("expected marked reducer run to complete, got %v", state.boundedRunErr)
	}
	if state.boundedRunResult.State != string(sigilruntime.RunStateCompleted) {
		return fmt.Errorf("expected completed run, got %q", state.boundedRunResult.State)
	}
	if state.boundedRunResult.FinalAnswer != "alpha=2; beta=1" {
		return fmt.Errorf("expected marked reducer final answer, got %q", state.boundedRunResult.FinalAnswer)
	}
	if strings.TrimSpace(state.boundedRunResult.FinalAnswerRef) == "" {
		return fmt.Errorf("expected final_answer_ref")
	}
	return nil
}

func (w *harnessWorld) noAdditionalRootInferenceTurnIsRequested() error {
	state := w.rlm()
	rootRequests := 0
	for _, request := range state.boundedRequests {
		envelope, err := decodeEnvelopeFromRequest(request)
		if err != nil {
			return err
		}
		if envelope.ExecutionState.NodeDepth == 0 {
			rootRequests++
		}
	}
	if rootRequests != 1 {
		return fmt.Errorf("expected exactly one root inference request, got %d", rootRequests)
	}
	return nil
}

func (w *harnessWorld) modelstepInferenceInputIsConstructedForFirstStep() error {
	cfg := boundedRunConfig("root prompt", w.rlm().boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: boundedFinalResult("done")}}); err != nil {
		return err
	}
	envelope, err := decodeEnvelopeFromRequest(w.rlm().boundedRequests[0])
	if err != nil {
		return err
	}
	w.rlm().boundedFirstEnvelope = envelope
	return nil
}

func (w *harnessWorld) fullRawContextIsExcludedFromOutboundModelstepInferenceMessages() error {
	state := w.rlm()
	if len(state.boundedRequests) == 0 {
		return fmt.Errorf("expected captured bounded inference request")
	}
	for _, message := range state.boundedRequests[0].Messages {
		if strings.Contains(message.Content, state.boundedRawContext) {
			return fmt.Errorf("expected raw context to be excluded from outbound model-step messages")
		}
	}
	return nil
}

func (w *harnessWorld) modelstepInferenceMessagesAreOrderedSystemThenUser() error {
	state := w.rlm()
	if len(state.boundedRequests) == 0 {
		return fmt.Errorf("expected captured bounded inference request")
	}
	messages := state.boundedRequests[0].Messages
	if len(messages) != 2 {
		return fmt.Errorf("expected exactly 2 messages, got %d", len(messages))
	}
	if messages[0].Role != sigilinference.MessageRoleSystem {
		return fmt.Errorf("expected first role system, got %q", messages[0].Role)
	}
	if messages[1].Role != sigilinference.MessageRoleUser {
		return fmt.Errorf("expected second role user, got %q", messages[1].Role)
	}
	return nil
}

func (w *harnessWorld) userStepEnvelopeContainsDeterministicQueryStepIndexAndContextMetadata() error {
	state := w.rlm()
	envelope := state.boundedFirstEnvelope
	if strings.TrimSpace(envelope.Query) == "" {
		return fmt.Errorf("expected non-empty envelope query")
	}
	if envelope.StepIndex != 1 {
		return fmt.Errorf("expected step_index=1, got %d", envelope.StepIndex)
	}
	if envelope.ContextMetadata.ContextType != "string" {
		return fmt.Errorf("expected context_type=string, got %q", envelope.ContextMetadata.ContextType)
	}
	if envelope.ContextMetadata.ContextBytes != len(state.boundedRawContext) {
		return fmt.Errorf("expected context_bytes=%d, got %d", len(state.boundedRawContext), envelope.ContextMetadata.ContextBytes)
	}
	normalized := strings.ReplaceAll(state.boundedRawContext, "\r\n", "\n")
	expectedLines := 0
	if normalized != "" {
		expectedLines = strings.Count(normalized, "\n") + 1
	}
	if envelope.ContextMetadata.ContextLineCount != expectedLines {
		return fmt.Errorf("expected context_line_count=%d, got %d", expectedLines, envelope.ContextMetadata.ContextLineCount)
	}
	sum := sha256.Sum256([]byte(state.boundedRawContext))
	expectedSHA := hex.EncodeToString(sum[:])
	if envelope.ContextMetadata.ContextSHA256 != expectedSHA {
		return fmt.Errorf("expected context_sha256=%q, got %q", expectedSHA, envelope.ContextMetadata.ContextSHA256)
	}
	if strings.TrimSpace(envelope.ContextMetadata.ContextRef) == "" {
		return fmt.Errorf("expected non-empty context_ref")
	}
	return nil
}

func (w *harnessWorld) userStepEnvelopeIncludesExecution_stateWithDepthStepBudgetsAndRecursionpermissionMetadata() error {
	state := w.rlm().boundedFirstEnvelope.ExecutionState
	if state.NodeDepth != 0 || state.MaxDepth != 3 || state.RemainingDepth != 3 {
		return fmt.Errorf("expected execution_state depth 0/3/3, got %+v", state)
	}
	if state.NodeStepsUsed != 1 || state.NodeStepsRemaining != 63 {
		return fmt.Errorf("expected node step budget 1/63, got %d/%d", state.NodeStepsUsed, state.NodeStepsRemaining)
	}
	if state.RunStepsUsed != 1 || state.RunStepsRemaining != 255 {
		return fmt.Errorf("expected run step budget 1/255, got %d/%d", state.RunStepsUsed, state.RunStepsRemaining)
	}
	if state.SameContextAsPreviousStep {
		return fmt.Errorf("expected same_context_as_previous_step=false on first step")
	}
	if !state.SmallContext {
		return fmt.Errorf("expected small_context=true for first-step fixture")
	}
	if !state.RecursiveSubcallsAllowed {
		return fmt.Errorf("expected recursive_subcalls_allowed=true on first step")
	}
	if state.RecursiveSubcallsReason != nil {
		return fmt.Errorf("expected recursive_subcalls_reason omitted on first step, got %+v", state.RecursiveSubcallsReason)
	}
	return nil
}

func (w *harnessWorld) aHarnessRunnerHasPreviousContinueActionFeedback() error {
	state := w.rlm()
	state.boundedRawContext = "needle in haystack context"
	code := `import "fmt"; fmt.Print("` + strings.Repeat("o", 2200) + `"); panic("` + strings.Repeat("e", 2200) + `")`
	cfg := boundedRunConfig("root prompt", state.boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: boundedContinueResult(code)},
		{result: boundedFinalResult("done")},
	}); err != nil {
		return err
	}
	if len(state.boundedRequests) < 2 {
		return fmt.Errorf("expected at least two inference requests")
	}
	firstEnvelope, err := decodeEnvelopeFromRequest(state.boundedRequests[0])
	if err != nil {
		return err
	}
	nextEnvelope, err := decodeEnvelopeFromRequest(state.boundedRequests[1])
	if err != nil {
		return err
	}
	state.boundedFirstEnvelope = firstEnvelope
	state.boundedNextEnvelope = nextEnvelope

	feedback := nextEnvelope.PreviousActionFeedback
	if feedback == nil {
		return fmt.Errorf("expected previous_action_feedback in next-step envelope")
	}
	artifactStore, err := sigilharness.NewActionArtifactStore(w.runsBaseDir())
	if err != nil {
		return err
	}
	actionArtifact, err := artifactStore.Read(state.boundedRunResult.RunID, feedback.ActionRef)
	if err != nil {
		return err
	}
	state.boundedActionArtifact = actionArtifact
	return nil
}

func (w *harnessWorld) aHarnessRunnerHasPreviousContinueActionSubcallFeedback() error {
	state := w.rlm()
	state.boundedRawContext = "root context"
	cfg := boundedRunConfig("root prompt", state.boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: boundedContinueResult(`import "fmt"; answer := ""; var queryErr error; answer, queryErr = llm_query("child prompt", "child context"); if queryErr != nil { panic(queryErr) }; fmt.Print(answer)`)},
		{result: boundedPlainAnswerResult("child answer")},
		{result: boundedFinalResult("done")},
	}); err != nil {
		return err
	}
	if len(state.boundedRequests) < 3 {
		return fmt.Errorf("expected at least three captured inference requests")
	}
	nextEnvelope, err := decodeEnvelopeFromRequest(state.boundedRequests[2])
	if err != nil {
		return err
	}
	state.boundedNextEnvelope = nextEnvelope
	return nil
}

func (w *harnessWorld) modelstepInferenceInputIsConstructedForNextStep() error {
	if w.rlm().boundedNextEnvelope.Query == "" {
		return fmt.Errorf("expected bounded next-step envelope to be constructed")
	}
	return nil
}

func (w *harnessWorld) previousActionFeedbackSummaryIncludesActionRefAndBoundedPreviewTruncationMetadata() error {
	feedback := w.rlm().boundedNextEnvelope.PreviousActionFeedback
	if feedback == nil {
		return fmt.Errorf("expected previous_action_feedback block")
	}
	if strings.TrimSpace(feedback.ActionRef) == "" {
		return fmt.Errorf("expected non-empty previous_action_feedback.action_ref")
	}
	if _, err := sigilruntime.ParseActionArtifactRef(feedback.ActionRef); err != nil {
		return fmt.Errorf("expected canonical previous_action_feedback.action_ref, got %q: %w", feedback.ActionRef, err)
	}
	if !feedback.StdoutTruncated && !feedback.StderrTruncated {
		return fmt.Errorf("expected at least one bounded preview truncation flag to be true")
	}
	if feedback.StdoutTruncated && len(feedback.StdoutPreview) != 2048 {
		return fmt.Errorf("expected stdout preview size 2048 when truncated, got %d", len(feedback.StdoutPreview))
	}
	if feedback.StderrTruncated && len(feedback.StderrPreview) != 2048 {
		return fmt.Errorf("expected stderr preview size 2048 when truncated, got %d", len(feedback.StderrPreview))
	}
	return nil
}

func (w *harnessWorld) previousactionFeedbackIncludesDeterministicSubcallSummaryCounts() error {
	feedback := w.rlm().boundedNextEnvelope.PreviousActionFeedback
	if feedback == nil || feedback.SubcallSummary == nil {
		return fmt.Errorf("expected previous_action_feedback.subcall_summary")
	}
	summary := feedback.SubcallSummary
	if summary.TotalCount != 1 || summary.PlainCount != 1 || summary.CompletedCount != 1 {
		return fmt.Errorf("expected one completed plain subcall, got %+v", *summary)
	}
	if summary.RecursiveCount != 0 || summary.FallbackCount != 0 || summary.FailedCount != 0 {
		return fmt.Errorf("expected zero recursive fallback and failed counts, got %+v", *summary)
	}
	return nil
}

func (w *harnessWorld) previousactionFeedbackBlockIsOmittedFromUserStepEnvelope() error {
	if w.rlm().boundedFirstEnvelope.PreviousActionFeedback != nil {
		return fmt.Errorf("expected first-step envelope to omit previous_action_feedback")
	}
	return nil
}

func (w *harnessWorld) harnessUserTurnArtifactInputIsPrepared() error {
	w.rlm().boundedRawContext = "needle in haystack context"
	return nil
}

func (w *harnessWorld) compactNodeTurnUserArtifactIsPersisted() error {
	cfg := boundedRunConfig("root prompt", w.rlm().boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: boundedFinalResult("done")}}); err != nil {
		return err
	}

	events, err := readEventsFromPath(w.rlm().boundedRunResult.EventsPath)
	if err != nil {
		return err
	}
	for _, event := range events {
		if event.Type != sigilruntime.EventTypeNodeTurnUser {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeTurnPayload)
		if !ok {
			return fmt.Errorf("expected node.turn.user payload type, got %T", event.Payload)
		}
		path, err := resolveArtifactPath(w.runsBaseDir(), w.rlm().boundedRunResult.RunID, payload.ContentRef)
		if err != nil {
			return err
		}
		encoded, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		artifact := map[string]any{}
		if err := json.Unmarshal(encoded, &artifact); err != nil {
			return err
		}
		w.rlm().boundedUserTurnArtifact = artifact
		return nil
	}
	return fmt.Errorf("expected node.turn.user event with content_ref")
}

func (w *harnessWorld) persistedNodeTurnUserArtifactExcludesFullRawContextBody() error {
	artifact := w.rlm().boundedUserTurnArtifact
	if artifact == nil {
		return fmt.Errorf("expected persisted user-turn artifact")
	}
	if _, exists := artifact["context"]; exists {
		return fmt.Errorf("expected compact user-turn artifact to exclude raw context body")
	}
	if _, exists := artifact["model_input_envelope"]; !exists {
		return fmt.Errorf("expected model_input_envelope in compact user-turn artifact")
	}
	if _, exists := artifact["model_input_messages"]; !exists {
		return fmt.Errorf("expected model_input_messages in compact user-turn artifact")
	}
	return nil
}

func (w *harnessWorld) actionArtifactRemainsSourceOfTruthForFullStdoutAndStderrWhileModelInputUsesBoundedPreviews() error {
	feedback := w.rlm().boundedNextEnvelope.PreviousActionFeedback
	if feedback == nil {
		return fmt.Errorf("expected previous_action_feedback for source-of-truth assertion")
	}
	artifact := w.rlm().boundedActionArtifact
	if strings.TrimSpace(artifact.RunID) == "" {
		return fmt.Errorf("expected action artifact to be loaded")
	}
	if feedback.StdoutBytes != len(artifact.Stdout) {
		return fmt.Errorf("expected stdout_bytes=%d, got %d", len(artifact.Stdout), feedback.StdoutBytes)
	}
	if feedback.StderrBytes != len(artifact.Stderr) {
		return fmt.Errorf("expected stderr_bytes=%d, got %d", len(artifact.Stderr), feedback.StderrBytes)
	}
	if feedback.StdoutTruncated && !strings.HasPrefix(artifact.Stdout, feedback.StdoutPreview) {
		return fmt.Errorf("expected stdout preview to match artifact stdout prefix")
	}
	if feedback.StderrTruncated && !strings.HasPrefix(artifact.Stderr, feedback.StderrPreview) {
		return fmt.Errorf("expected stderr preview to match artifact stderr prefix")
	}
	return nil
}

func (w *harnessWorld) exactActionStdoutRemainsRecoverableThroughReadActionArtifactUsingThatActionRef() error {
	state := w.rlm()
	feedback := state.boundedNextEnvelope.PreviousActionFeedback
	if feedback == nil {
		return fmt.Errorf("expected previous_action_feedback for exact stdout recovery")
	}
	parsed, err := sigilruntime.ParseActionArtifactRef(feedback.ActionRef)
	if err != nil {
		return err
	}
	artifactStore, err := sigilharness.NewActionArtifactStore(w.runsBaseDir())
	if err != nil {
		return err
	}
	session, err := sigilrepl.NewFactory().NewSession(context.Background(), sigilrepl.SessionOptions{
		RunID:   state.boundedRunResult.RunID,
		NodeID:  parsed.NodeID,
		Depth:   0,
		Context: "bounded-context",
		LLMQuery: func(_ context.Context, _ sigilrepl.QueryRequest) (string, error) {
			return "", nil
		},
		RLMQuery: func(_ context.Context, _ sigilrepl.QueryRequest) (string, error) {
			return "", nil
		},
		LLMQueryBatched: func(_ context.Context, _ []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			return nil, nil
		},
		RLMQueryBatched: func(_ context.Context, _ []sigilrepl.BatchedQueryRequest) ([]sigilrepl.BatchedQueryResult, error) {
			return nil, nil
		},
		ReadActionArtifact: func(actionRef string) (sigilrepl.ActionOutput, error) {
			if strings.TrimSpace(actionRef) != actionRef {
				return sigilrepl.ActionOutput{}, fmt.Errorf("action_ref %q must be canonical without leading or trailing whitespace", actionRef)
			}
			if _, err := sigilruntime.ParseActionArtifactRef(actionRef); err != nil {
				return sigilrepl.ActionOutput{}, err
			}
			artifact, err := artifactStore.Read(state.boundedRunResult.RunID, actionRef)
			if err != nil {
				return sigilrepl.ActionOutput{}, err
			}
			errorCode := ""
			if artifact.ErrorCode != nil {
				errorCode = *artifact.ErrorCode
			}
			errorMessage := ""
			if artifact.ErrorMessage != nil {
				errorMessage = *artifact.ErrorMessage
			}
			return sigilrepl.ActionOutput{
				Status:       artifact.Status,
				Stdout:       artifact.Stdout,
				Stderr:       artifact.Stderr,
				ErrorCode:    errorCode,
				ErrorMessage: errorMessage,
			}, nil
		},
	})
	if err != nil {
		return fmt.Errorf("failed to create read_action_artifact session; %w", err)
	}
	defer session.Close()

	result, err := session.Exec(context.Background(), `import "fmt"; output, err := read_action_artifact("`+feedback.ActionRef+`"); if err != nil { panic(err) }; fmt.Print(output.Stdout)`)
	if err != nil {
		return fmt.Errorf("failed to recover exact action stdout through read_action_artifact; %w", err)
	}
	if result.Stdout != state.boundedActionArtifact.Stdout {
		return fmt.Errorf("expected exact action stdout %q, got %q", state.boundedActionArtifact.Stdout, result.Stdout)
	}
	return nil
}

func (w *harnessWorld) recursiveHarnessChildNodeExecutionInputIsPrepared() error {
	state := w.rlm()
	state.boundedRawContext = "root needle context"
	cfg := boundedRunConfig("root prompt", state.boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: boundedContinueResult(`import "fmt"; answer, err := rlm_query("child prompt", "child context payload"); if err != nil { panic(err) }; fmt.Print(answer)`)},
		{result: boundedFinalResult("child final")},
		{result: boundedFinalResult("root final")},
	}); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) modelstepInferenceInputIsConstructedForChildStep() error {
	if len(w.rlm().boundedRequests) < 2 {
		return fmt.Errorf("expected child-step request to be captured")
	}
	return nil
}

func (w *harnessWorld) boundedModelinputContractIsAppliedToRecursiveChildNodeStep() error {
	state := w.rlm()
	if len(state.boundedRequests) < 2 {
		return fmt.Errorf("expected recursive run to capture child request")
	}
	childRequest := state.boundedRequests[1]
	if len(childRequest.Messages) != 2 || childRequest.Messages[0].Role != sigilinference.MessageRoleSystem || childRequest.Messages[1].Role != sigilinference.MessageRoleUser {
		return fmt.Errorf("expected child request messages ordered system then user")
	}
	childContext := "child context payload"
	for _, message := range childRequest.Messages {
		if strings.Contains(message.Content, childContext) {
			return fmt.Errorf("expected child raw context to be excluded from child model-step messages")
		}
	}
	envelope, err := decodeEnvelopeFromRequest(childRequest)
	if err != nil {
		return err
	}
	if envelope.Query != "child prompt" {
		return fmt.Errorf("expected child query %q, got %q", "child prompt", envelope.Query)
	}
	if envelope.ContextMetadata.ContextBytes != len(childContext) {
		return fmt.Errorf("expected child context_bytes=%d, got %d", len(childContext), envelope.ContextMetadata.ContextBytes)
	}
	return nil
}

func (w *harnessWorld) modelstepInferenceInputIsConstructedForNonrecursiveStep() error {
	state := w.rlm()
	cfg := boundedRunConfig("root prompt", "non-recursive root context")
	cfg.RLM.Enabled = false
	code := `import "fmt"; _, err := rlm_query("child prompt", "child context payload"); if err != nil { fmt.Print(err.Error()) }`
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: boundedContinueResult(code)},
		{result: boundedFinalResult("done")},
	}); err != nil {
		return err
	}
	events, err := readEventsFromPath(state.boundedRunResult.EventsPath)
	if err != nil {
		return err
	}
	state.boundedPersistedEvents = events
	return nil
}

func (w *harnessWorld) nonrecursiveHarnessModeIsActive() error {
	state := w.rlm()
	state.runMaxDepth = 3
	if err := w.ensureParentNodeAtDepth(3, 3); err != nil {
		return err
	}
	state.nonRecursive = true
	return nil
}

func (w *harnessWorld) boundedModelinputContractIsAppliedInNonrecursiveMode() error {
	state := w.rlm()
	if len(state.boundedRequests) < 2 {
		return fmt.Errorf("expected at least two requests in non-recursive mode")
	}
	for _, event := range state.boundedPersistedEvents {
		if event.Type != sigilruntime.EventTypeNodeStarted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeStartedPayload)
		if ok && payload.Role == sigilruntime.NodeRoleRecursiveSubcall {
			return fmt.Errorf("expected non-recursive mode to avoid child node creation")
		}
	}
	envelope, err := decodeEnvelopeFromRequest(state.boundedRequests[1])
	if err != nil {
		return err
	}
	if envelope.PreviousActionFeedback == nil {
		return fmt.Errorf("expected previous_action_feedback in non-recursive second step envelope")
	}
	return nil
}

func (w *harnessWorld) aSmallcontextHarnessRunnerAlreadyUsedRecursiveSubcallsInAPriorContinueStep() error {
	state := w.rlm()
	state.boundedRawContext = "small root context"
	cfg := boundedRunConfig("root prompt", state.boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: boundedContinueResult(`import "fmt"; answer := ""; var queryErr error; answer, queryErr = rlm_query("child prompt", "child context"); if queryErr != nil { panic(queryErr) }; fmt.Print(answer)`)},
		{result: boundedFinalResult("child one")},
		{result: boundedContinueResult(`import "fmt"; answer := ""; var queryErr error; answer, queryErr = rlm_query("second child prompt", "second child context"); if queryErr != nil { panic(queryErr) }; fmt.Print(answer)`)},
		{result: boundedPlainAnswerResult("fallback answer")},
		{result: boundedFinalResult("done")},
	}); err != nil {
		return err
	}
	if len(state.boundedRequests) < 5 {
		return fmt.Errorf("expected five captured inference requests, got %d", len(state.boundedRequests))
	}
	nextEnvelope, err := decodeEnvelopeFromRequest(state.boundedRequests[2])
	if err != nil {
		return err
	}
	state.boundedNextEnvelope = nextEnvelope
	events, err := readEventsFromPath(state.boundedRunResult.EventsPath)
	if err != nil {
		return err
	}
	state.boundedPersistedEvents = events
	return nil
}

func (w *harnessWorld) theNextStepInvokesRlm_queryOnThatSameNode() error {
	if len(w.rlm().boundedRequests) < 5 {
		return fmt.Errorf("expected repeated small-context run to execute before invocation assertion")
	}
	return nil
}

func (w *harnessWorld) theNextstepExecutionStateDisablesRecursiveSubcalls() error {
	state := w.rlm().boundedNextEnvelope.ExecutionState
	if state.RecursiveSubcallsAllowed {
		return fmt.Errorf("expected recursive_subcalls_allowed=false, got %+v", state)
	}
	if state.RecursiveSubcallsReason == nil || !strings.Contains(*state.RecursiveSubcallsReason, "small context") {
		return fmt.Errorf("expected recursive_subcalls_reason to explain small-context local-only mode, got %+v", state.RecursiveSubcallsReason)
	}
	return nil
}

func (w *harnessWorld) repeatedSmallcontextRlm_queryUsesPlainFallbackWithoutCreatingAnotherChildNode() error {
	state := w.rlm()
	plainSubcalls := 0
	for _, request := range state.boundedRequests {
		if request.SchemaID == sigilschema.SigilLLMAnswerV1SchemaID {
			plainSubcalls++
			if len(request.Messages) < 2 || !strings.Contains(request.Messages[1].Content, `"second child prompt"`) {
				return fmt.Errorf("expected fallback plain subcall payload for repeated rlm_query, got %+v", request.Messages)
			}
		}
	}
	if plainSubcalls != 1 {
		return fmt.Errorf("expected exactly one fallback plain subcall request, got %d", plainSubcalls)
	}

	recursiveChildren := 0
	for _, event := range state.boundedPersistedEvents {
		if event.Type != sigilruntime.EventTypeNodeStarted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.NodeStartedPayload)
		if ok && payload.Role == sigilruntime.NodeRoleRecursiveSubcall {
			recursiveChildren++
		}
	}
	if recursiveChildren != 1 {
		return fmt.Errorf("expected only the first step to create a recursive child node, got %d", recursiveChildren)
	}
	return nil
}

func (w *harnessWorld) stepenvelopeSerializationOrPersistenceFailureIsInjected() error {
	state := w.rlm()
	state.boundedFailureInjected = true
	state.boundedFailureCode = sigilharness.ErrorCodeInfrastructure
	state.boundedFailureMessage = "failed to persist deterministic step input envelope"
	state.boundedFailureTransition = nil
	return w.resetLifecycleToState(sigilruntime.RunStateRunning)
}

func (w *harnessWorld) harnessRunExecutionHandlesBoundedModelinputFailure() error {
	state := w.rlm()
	if !state.boundedFailureInjected {
		return fmt.Errorf("expected bounded model-input failure injection")
	}
	if w.lifecycle == nil {
		if err := w.resetLifecycleToState(sigilruntime.RunStateRunning); err != nil {
			return err
		}
	}
	payload := sigilruntime.RunFailedPayload{
		Status:       "failed",
		ErrorCode:    string(state.boundedFailureCode),
		ErrorMessage: state.boundedFailureMessage,
		Retryable:    false,
	}
	state.boundedFailureTransition = w.lifecycle.FailWith(payload)
	state.failedPayload = payload
	return state.boundedFailureTransition
}

func (w *harnessWorld) runFailsWithTypedInfrastructureMetadataForBoundedModelinputFailure() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle")
	}
	if w.lifecycle.State() != sigilruntime.RunStateFailed {
		return fmt.Errorf("expected run state failed, got %q", w.lifecycle.State())
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
		if payload.ErrorCode != string(sigilharness.ErrorCodeInfrastructure) {
			return fmt.Errorf("expected run.failed error_code=%q, got %q", sigilharness.ErrorCodeInfrastructure, payload.ErrorCode)
		}
		if strings.TrimSpace(payload.ErrorMessage) == "" {
			return fmt.Errorf("expected non-empty run.failed error_message")
		}
		return nil
	}
	return fmt.Errorf("expected run.failed event")
}

func (w *harnessWorld) boundedModelinputExecutionIsActiveForANodeStep() error {
	w.rlm().boundedRawContext = "bounded-context"
	return nil
}

func (w *harnessWorld) stepAndTurnEventsArePersistedUnderBoundedModelinputExecution() error {
	cfg := boundedRunConfig("root prompt", w.rlm().boundedRawContext)
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: boundedFinalResult("done")}}); err != nil {
		return err
	}
	events, err := readEventsFromPath(w.rlm().boundedRunResult.EventsPath)
	if err != nil {
		return err
	}
	w.rlm().boundedPersistedEvents = events
	return nil
}

func (w *harnessWorld) canonicalRunEventOrderingAndReferencesRemainValid() error {
	events := w.rlm().boundedPersistedEvents
	if len(events) == 0 {
		return fmt.Errorf("expected persisted bounded model-input events")
	}
	indexByType := map[sigilruntime.EventType]int{}
	for index, event := range events {
		if event.Type == sigilruntime.EventTypeNodeStepStarted ||
			event.Type == sigilruntime.EventTypeNodeTurnUser ||
			event.Type == sigilruntime.EventTypeNodeTurnModel ||
			event.Type == sigilruntime.EventTypeNodeStepCompleted {
			if _, exists := indexByType[event.Type]; !exists {
				indexByType[event.Type] = index
			}
		}
	}

	required := []sigilruntime.EventType{
		sigilruntime.EventTypeNodeStepStarted,
		sigilruntime.EventTypeNodeTurnUser,
		sigilruntime.EventTypeNodeTurnModel,
		sigilruntime.EventTypeNodeStepCompleted,
	}
	for _, eventType := range required {
		if _, ok := indexByType[eventType]; !ok {
			return fmt.Errorf("expected event type %q in bounded execution", eventType)
		}
	}
	if !(indexByType[sigilruntime.EventTypeNodeStepStarted] < indexByType[sigilruntime.EventTypeNodeTurnUser] &&
		indexByType[sigilruntime.EventTypeNodeTurnUser] < indexByType[sigilruntime.EventTypeNodeTurnModel] &&
		indexByType[sigilruntime.EventTypeNodeTurnModel] < indexByType[sigilruntime.EventTypeNodeStepCompleted]) {
		return fmt.Errorf("expected canonical node step/turn ordering under bounded model-input execution")
	}
	return nil
}

func (w *harnessWorld) aHarnessRunnerCapturesProviderreportedAccountingForAFinalStep() error {
	cfg := boundedRunConfig("root prompt", "root context")
	result := boundedFinalResult("done")
	result.Accounting = acceptanceAccountingSummary("openai", "gpt-5.1")
	return w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: result}})
}

func (w *harnessWorld) aHarnessRunnerCapturesFallbackpricedAccountingForAFinalStep() error {
	cfg := boundedRunConfig("root prompt", "root context")
	result := boundedFinalResult("done")
	result.Accounting = acceptanceFallbackAccountingSummary("openai", "gpt-5.1")
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: result}}); err != nil {
		return err
	}
	return w.accountingArtifactsArePersistedForTheCompletedRun()
}

func (w *harnessWorld) accountingArtifactsArePersistedForTheCompletedRun() error {
	events, err := readEventsFromPath(w.rlm().boundedRunResult.EventsPath)
	if err != nil {
		return err
	}
	w.rlm().boundedPersistedEvents = events
	return nil
}

func (w *harnessWorld) successfulRunSummaryAndTerminalEventsIncludeAccounting() error {
	state := w.rlm()
	if state.boundedRunErr != nil {
		return fmt.Errorf("expected completed run, got %v", state.boundedRunErr)
	}
	if state.boundedRunResult.Accounting.TreeTotal.TotalTokens == nil || *state.boundedRunResult.Accounting.TreeTotal.TotalTokens == 0 {
		return fmt.Errorf("expected non-zero run summary accounting total_tokens, got %+v", state.boundedRunResult.Accounting.TreeTotal)
	}
	if state.boundedRunResult.Accounting.TreeTotal.CostStatus != accounting.StatusComplete {
		return fmt.Errorf("expected complete run summary cost status, got %q", state.boundedRunResult.Accounting.TreeTotal.CostStatus)
	}

	runAccountingPath, err := resolveArtifactPath(w.runsBaseDir(), state.boundedRunResult.RunID, "run-artifact://run/accounting.json")
	if err != nil {
		return err
	}
	if _, err := os.Stat(runAccountingPath); err != nil {
		return fmt.Errorf("expected run accounting artifact %q, got %v", runAccountingPath, err)
	}

	var sawStepCompleted bool
	var sawRunCompleted bool
	var sawModelTurnAccounting bool
	for _, event := range state.boundedPersistedEvents {
		switch event.Type {
		case sigilruntime.EventTypeNodeStepCompleted:
			payload, ok := event.Payload.(sigilruntime.NodeStepCompletedPayload)
			if !ok {
				return fmt.Errorf("expected node.step.completed payload type, got %T", event.Payload)
			}
			if strings.TrimSpace(payload.AccountingRef) == "" {
				return fmt.Errorf("expected node.step.completed accounting_ref")
			}
			sawStepCompleted = true
		case sigilruntime.EventTypeRunCompleted:
			payload, ok := event.Payload.(sigilruntime.RunCompletedPayload)
			if !ok {
				return fmt.Errorf("expected run.completed payload type, got %T", event.Payload)
			}
			if payload.AccountingRef == nil || strings.TrimSpace(*payload.AccountingRef) == "" {
				return fmt.Errorf("expected run.completed accounting_ref")
			}
			if payload.Accounting.TreeTotal.TotalTokens == nil || *payload.Accounting.TreeTotal.TotalTokens == 0 {
				return fmt.Errorf("expected non-zero run.completed accounting tree_total")
			}
			sawRunCompleted = true
		case sigilruntime.EventTypeNodeTurnModel:
			payload, ok := event.Payload.(sigilruntime.NodeTurnPayload)
			if !ok {
				return fmt.Errorf("expected node.turn.model payload type, got %T", event.Payload)
			}
			modelTurnPath, err := resolveArtifactPath(w.runsBaseDir(), state.boundedRunResult.RunID, payload.ContentRef)
			if err != nil {
				return err
			}
			encoded, err := os.ReadFile(modelTurnPath)
			if err != nil {
				return err
			}
			artifact := map[string]any{}
			if err := json.Unmarshal(encoded, &artifact); err != nil {
				return err
			}
			if _, ok := artifact["accounting"].(map[string]any); ok {
				sawModelTurnAccounting = true
			}
		}
	}
	if !sawStepCompleted {
		return fmt.Errorf("expected node.step.completed event with accounting_ref")
	}
	if !sawRunCompleted {
		return fmt.Errorf("expected run.completed event with accounting")
	}
	if !sawModelTurnAccounting {
		return fmt.Errorf("expected node.turn.model artifact to include accounting")
	}
	return nil
}

func (w *harnessWorld) aRecursiveHarnessRunCapturesSubtreeAccounting() error {
	cfg := boundedRunConfig("root prompt", "root context")
	rootContinue := boundedContinueResult(`import "fmt"; answer, err := rlm_query("child prompt", "child context payload"); if err != nil { panic(err) }; fmt.Print(answer)`)
	rootContinue.Accounting = acceptanceAccountingSummary("openai", "gpt-5.1")
	childFinal := boundedFinalResult("child final")
	childFinal.Accounting = acceptanceAccountingSummary("openai", "gpt-5.1")
	rootFinal := boundedFinalResult("root final")
	rootFinal.Accounting = acceptanceAccountingSummary("openai", "gpt-5.1")
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{
		{result: rootContinue},
		{result: childFinal},
		{result: rootFinal},
	}); err != nil {
		return err
	}
	return w.accountingArtifactsArePersistedForTheCompletedRun()
}

func (w *harnessWorld) accountingRollupsAreInspected() error {
	if len(w.rlm().boundedPersistedEvents) == 0 {
		return fmt.Errorf("expected persisted events before accounting inspection")
	}
	return nil
}

func (w *harnessWorld) recursiveAccountingTreeTotalIncludesChildNodeTotals() error {
	rollup := w.rlm().boundedRunResult.Accounting
	if rollup.DirectSubcallsTotal.TotalTokens == nil || *rollup.DirectSubcallsTotal.TotalTokens == 0 {
		return fmt.Errorf("expected non-zero direct_subcalls_total.total_tokens, got %+v", rollup.DirectSubcallsTotal)
	}
	if rollup.ModelTotal.TotalTokens == nil || rollup.TreeTotal.TotalTokens == nil {
		return fmt.Errorf("expected model_total and tree_total token totals, got %+v", rollup)
	}
	if *rollup.TreeTotal.TotalTokens <= *rollup.ModelTotal.TotalTokens {
		return fmt.Errorf("expected tree_total.total_tokens %d to exceed model_total.total_tokens %d", *rollup.TreeTotal.TotalTokens, *rollup.ModelTotal.TotalTokens)
	}
	if rollup.TreeTotal.CostStatus != accounting.StatusComplete {
		return fmt.Errorf("expected complete tree_total cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	return nil
}

func (w *harnessWorld) fallbackPricingderivedAccountingCostIsPreservedInCompletedRunAccounting() error {
	rollup := w.rlm().boundedRunResult.Accounting
	if rollup.TreeTotal.CostSource != accounting.SourceFallbackPricing {
		return fmt.Errorf("expected fallback_pricing cost source, got %q", rollup.TreeTotal.CostSource)
	}
	if rollup.TreeTotal.CostStatus != accounting.StatusComplete {
		return fmt.Errorf("expected complete fallback-priced cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	if rollup.TreeTotal.KnownTotalCostMicrousd == nil || *rollup.TreeTotal.KnownTotalCostMicrousd <= 0 {
		return fmt.Errorf("expected non-zero fallback-priced known_total_cost_microusd, got %+v", rollup.TreeTotal)
	}
	return nil
}

func (w *harnessWorld) subcallEventsAndActionArtifactsIncludeLeafAccountingSummaries() error {
	state := w.rlm()
	var sawSubcallEvent bool
	var sawActionArtifact bool

	artifactStore, err := sigilharness.NewActionArtifactStore(w.runsBaseDir())
	if err != nil {
		return err
	}

	for _, event := range state.boundedPersistedEvents {
		switch event.Type {
		case sigilruntime.EventTypeNodeSubcallExecuted:
			payload, ok := event.Payload.(sigilruntime.NodeSubcallExecutedPayload)
			if !ok {
				return fmt.Errorf("expected node.subcall.executed payload type, got %T", event.Payload)
			}
			if strings.TrimSpace(payload.AccountingRef) == "" {
				return fmt.Errorf("expected node.subcall.executed accounting_ref")
			}
			if payload.Accounting.TotalTokens == nil || *payload.Accounting.TotalTokens == 0 {
				return fmt.Errorf("expected node.subcall.executed accounting total_tokens, got %+v", payload.Accounting)
			}
			sawSubcallEvent = true
		case sigilruntime.EventTypeNodeActionExecuted:
			payload, ok := event.Payload.(sigilruntime.NodeActionExecutedPayload)
			if !ok {
				return fmt.Errorf("expected node.action.executed payload type, got %T", event.Payload)
			}
			artifact, err := artifactStore.Read(state.boundedRunResult.RunID, payload.ActionRef)
			if err != nil {
				return err
			}
			if len(artifact.Subcalls) == 0 {
				return fmt.Errorf("expected action artifact subcalls trace")
			}
			if artifact.Subcalls[0].Accounting == nil {
				return fmt.Errorf("expected action artifact subcall accounting summary")
			}
			if artifact.Subcalls[0].Accounting.TotalTokens == nil || *artifact.Subcalls[0].Accounting.TotalTokens == 0 {
				return fmt.Errorf("expected action artifact subcall accounting total_tokens, got %+v", artifact.Subcalls[0].Accounting)
			}
			sawActionArtifact = true
		}
	}

	if !sawSubcallEvent {
		return fmt.Errorf("expected node.subcall.executed event with accounting")
	}
	if !sawActionArtifact {
		return fmt.Errorf("expected node.action.executed artifact with subcall accounting")
	}
	return nil
}

func (w *harnessWorld) aHarnessRunnerCapturesPartialAccountingForAFinalStep() error {
	cfg := boundedRunConfig("root prompt", "root context")
	result := boundedFinalResult("done")
	result.Accounting = acceptancePartialAccountingSummary("openai", "gpt-5.1")
	if err := w.executeBoundedHarnessRun(cfg, []boundedInferenceResponse{{result: result}}); err != nil {
		return err
	}
	return w.accountingArtifactsArePersistedForTheCompletedRun()
}

func (w *harnessWorld) partialAccountingTotalsRemainMarkedPartialInsteadOfZerocomplete() error {
	rollup := w.rlm().boundedRunResult.Accounting
	if rollup.TreeTotal.TokenStatus != accounting.StatusComplete {
		return fmt.Errorf("expected complete token status, got %q", rollup.TreeTotal.TokenStatus)
	}
	if rollup.TreeTotal.CostStatus != accounting.StatusPartial {
		return fmt.Errorf("expected partial cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	if rollup.TreeTotal.KnownTotalCostMicrousd == nil {
		return fmt.Errorf("expected known subtotal cost under partial status")
	}
	return nil
}

func (w *harnessWorld) aRunningLifecycleCapturesPartialTerminalAccounting() error {
	if err := w.resetLifecycleToState(sigilruntime.RunStateRunning); err != nil {
		return err
	}
	w.rlm().boundedPersistedEvents = nil
	return nil
}

func (w *harnessWorld) aFailedTerminalEventIsPersistedWithAccounting() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle")
	}
	if err := w.lifecycle.FailWith(sigilruntime.RunFailedPayload{
		Status:       "failed",
		ErrorCode:    "runtime.failure",
		ErrorMessage: "failure",
		Retryable:    false,
		Accounting:   acceptancePartialAccountingRollup("openai", "gpt-5.1"),
	}); err != nil {
		return err
	}
	return w.captureBoundedLifecycleEvents()
}

func (w *harnessWorld) anInterruptedTerminalEventIsPersistedWithAccounting() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle")
	}
	interruptedBy := sigilruntime.RunInterruptedByLifecycle
	if err := w.lifecycle.InterruptWith(sigilruntime.RunInterruptedPayload{
		Status:        "interrupted",
		Reason:        sigilruntime.RunInterruptedReasonUserRequest,
		InterruptedBy: &interruptedBy,
		Accounting:    acceptancePartialAccountingRollup("openai", "gpt-5.1"),
	}); err != nil {
		return err
	}
	return w.captureBoundedLifecycleEvents()
}

func (w *harnessWorld) failedTerminalEventsIncludePartialAccounting() error {
	for index := len(w.rlm().boundedPersistedEvents) - 1; index >= 0; index-- {
		event := w.rlm().boundedPersistedEvents[index]
		if event.Type != sigilruntime.EventTypeRunFailed {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunFailedPayload)
		if !ok {
			return fmt.Errorf("expected run.failed payload type, got %T", event.Payload)
		}
		return assertPartialTerminalAccounting(payload.Accounting)
	}
	return fmt.Errorf("expected run.failed event with accounting")
}

func (w *harnessWorld) interruptedTerminalEventsIncludePartialAccounting() error {
	for index := len(w.rlm().boundedPersistedEvents) - 1; index >= 0; index-- {
		event := w.rlm().boundedPersistedEvents[index]
		if event.Type != sigilruntime.EventTypeRunInterrupted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunInterruptedPayload)
		if !ok {
			return fmt.Errorf("expected run.interrupted payload type, got %T", event.Payload)
		}
		return assertPartialTerminalAccounting(payload.Accounting)
	}
	return fmt.Errorf("expected run.interrupted event with accounting")
}

func (w *harnessWorld) captureBoundedLifecycleEvents() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle")
	}
	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	w.rlm().boundedPersistedEvents = events
	return nil
}

func assertPartialTerminalAccounting(rollup accounting.Rollup) error {
	if rollup.TreeTotal.TokenStatus != accounting.StatusComplete {
		return fmt.Errorf("expected complete token status, got %q", rollup.TreeTotal.TokenStatus)
	}
	if rollup.TreeTotal.CostStatus != accounting.StatusPartial {
		return fmt.Errorf("expected partial cost status, got %q", rollup.TreeTotal.CostStatus)
	}
	if rollup.TreeTotal.KnownTotalCostMicrousd == nil {
		return fmt.Errorf("expected known subtotal cost under partial terminal accounting")
	}
	return nil
}

func (w *harnessWorld) executeBoundedHarnessRun(runConfig config.RunConfig, responses []boundedInferenceResponse) error {
	capture := &boundedCaptureInference{responses: responses}
	runner := sigilharness.NewRunner(
		sigilharness.WithRunsBaseDir(w.runsBaseDir()),
		sigilharness.WithInferenceFactory(func(_ config.RunConfig) (sigilharness.InferenceClient, error) {
			return capture, nil
		}),
	)

	result, err := runner.Run(context.Background(), sigilharness.RunInput{
		AppConfigPath: "./sigil.yaml",
		RunConfigPath: "./sigil-run.yaml",
		RunConfig:     runConfig,
		TemplateVars:  map[string]string{},
	})
	state := w.rlm()
	state.boundedRequests = append([]sigilinference.Request(nil), capture.requests...)
	state.boundedRunResult = result
	state.boundedRunErr = err
	if err != nil {
		return err
	}
	if len(state.boundedRequests) == 0 {
		return fmt.Errorf("expected at least one captured inference request")
	}
	return nil
}

func boundedRunConfig(prompt string, contextValue string) config.RunConfig {
	cfg := config.NewDefaultRunConfig()
	cfg.Name = "test-run"
	cfg.Prompt = prompt
	cfg.Context = contextValue
	cfg.PromptTemplate = ""
	cfg.ContextTemplate = ""
	cfg.LLM.Provider = "openai"
	cfg.LLM.Model = "gpt-5.1"
	cfg.LLM.Gateway = "openrouter"
	cfg.LLM.OpenRouter.BaseURL = "http://127.0.0.1:1"
	cfg.LLM.OpenRouter.APIKeyEnv = "OPENROUTER_API_KEY"
	cfg.LLM.OpenRouter.RequestTimeoutMS = 500
	cfg.RLM.Enabled = true
	cfg.RLM.MaxDepth = 3
	return cfg
}

func boundedContinueResult(replCode string) sigilinference.Result {
	return sigilinference.Result{
		SchemaID:          "sigil.rlm.response.v1",
		ValidatedPayload:  map[string]any{"decision": "continue", "continuation": map[string]any{"repl_code": replCode, "intent": "inspect context chunk", "expected_observation": "needle appears in output"}},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_continue",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func boundedFinalResult(answer string) sigilinference.Result {
	return sigilinference.Result{
		SchemaID:          "sigil.rlm.response.v1",
		ValidatedPayload:  map[string]any{"decision": "final", "final": map[string]any{"answer": answer, "evidence": []any{map[string]any{"ref": "__context_ref__"}}}},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_final",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func boundedPlainAnswerResult(answer string) sigilinference.Result {
	return sigilinference.Result{
		SchemaID:          sigilschema.SigilLLMAnswerV1SchemaID,
		ValidatedPayload:  map[string]any{"answer": answer},
		Gateway:           "openrouter",
		Provider:          "openai",
		Model:             "gpt-5.1",
		GatewayResponseID: "resp_plain",
		FinishStatus:      "completed",
		RawMetadata:       map[string]any{},
	}
}

func decodeEnvelopeFromRequest(request sigilinference.Request) (sigilharness.StepInputEnvelope, error) {
	if len(request.Messages) < 2 {
		return sigilharness.StepInputEnvelope{}, fmt.Errorf("expected system and user messages")
	}
	if request.Messages[0].Role != sigilinference.MessageRoleSystem || request.Messages[1].Role != sigilinference.MessageRoleUser {
		return sigilharness.StepInputEnvelope{}, fmt.Errorf("expected ordered message roles system then user")
	}
	var envelope sigilharness.StepInputEnvelope
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &envelope); err != nil {
		return sigilharness.StepInputEnvelope{}, fmt.Errorf("failed to decode user step envelope; %w", err)
	}
	return envelope, nil
}

func readEventsFromPath(path string) ([]sigilruntime.EventEnvelope, error) {
	encoded, err := os.ReadFile(filepath.Clean(path))
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(encoded), "\n")
	events := make([]sigilruntime.EventEnvelope, 0, len(lines))
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		event, parseErr := sigilruntime.ParseEventEnvelopeStrict([]byte(trimmed))
		if parseErr != nil {
			return nil, parseErr
		}
		events = append(events, event)
	}
	return events, nil
}

func resolveArtifactPath(runsBaseDir string, runID string, contentRef string) (string, error) {
	trimmed := strings.TrimSpace(contentRef)
	const prefix = "run-artifact://"
	if !strings.HasPrefix(trimmed, prefix) {
		return "", fmt.Errorf("unsupported content_ref %q", contentRef)
	}
	relative := strings.TrimPrefix(trimmed, prefix)
	if strings.TrimSpace(relative) == "" {
		return "", fmt.Errorf("content_ref path is empty")
	}
	return filepath.Join(runsBaseDir, runID, "artifacts", filepath.FromSlash(relative)), nil
}
