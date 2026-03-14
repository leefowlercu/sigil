package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	runtimestd "runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/cmd"
	"github.com/leefowlercu/sigil/internal/config"
	sigilinference "github.com/leefowlercu/sigil/internal/inference"
	sigilschema "github.com/leefowlercu/sigil/internal/inference/schema"
	"github.com/leefowlercu/sigil/internal/logging"
	sigilruntime "github.com/leefowlercu/sigil/internal/runtime"
)

type acceptanceStopResult struct {
	RunID         string `json:"run_id"`
	StopRequested bool   `json:"stop_requested"`
	State         string `json:"state"`
	EventsPath    string `json:"events_path"`
}

type stopInvocation struct {
	Name     string
	RunID    string
	ExitCode int
	Stdout   string
	Stderr   string
	Err      error
	Result   *acceptanceStopResult
}

type acceptanceStopHelperReady struct {
	RunID      string `json:"run_id"`
	RunDir     string `json:"run_dir"`
	EventsPath string `json:"events_path"`
}

type harnessWorld struct {
	workingDir               string
	originalWorkingDir       string
	lastStdout               string
	lastStderr               string
	lastErr                  error
	lastExitCode             int
	configInitErr            error
	loggingInitErr           error
	runConfigInitErr         error
	resolvedAppConfigPath    string
	resolvedRunConfigPath    string
	lifecycle                *sigilruntime.Lifecycle
	lifecycleTransitionErr   error
	lastCreatedNode          sigilruntime.Node
	nodeCountBeforeActivity  int
	nodeCountAfterActivity   int
	eventLogPath             string
	persistedEvents          []sigilruntime.EventEnvelope
	eventAppendErr           error
	eventValidationErr       error
	eventIntegrityErr        error
	rawEventLines            []string
	inferenceService         *sigilinference.Service
	inferenceGatewayRegistry *sigilinference.Registry
	inferenceSchemaRegistry  *sigilschema.Registry
	inferenceRequest         sigilinference.Request
	inferenceResult          sigilinference.Result
	inferenceErr             error
	inferenceResolvedErr     error
	inferenceRetryDelays     []time.Duration
	inferenceRequestBody     map[string]any
	inferenceMockServer      *openRouterMockServer
	rlmState                 *rlmAcceptanceState
	activeRunID              string
	activeRunDir             string
	activeRunEventsPath      string
	activeProcessMetadata    sigilruntime.ProcessMetadata
	activeProcessSeen        bool
	activeStopInvocation     stopInvocation
	terminalRunIDs           []string
	terminalStopInvocations  []stopInvocation
	racingStopInvocations    []stopInvocation
	invalidCaseRunIDs        map[string]string
	invalidStopInvocations   []stopInvocation
	helperDir                string
	helperCmd                *exec.Cmd
	helperStdout             bytes.Buffer
	helperStderr             bytes.Buffer
	invalidProcessCmd        *exec.Cmd
	runInspection            *runInspectionState
	appServer                *appServerAcceptanceState
}

// InitializeScenario wires all acceptance steps for harness.feature.
func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &harnessWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		if err := world.cleanupScenarioResources(); err != nil {
			return ctx, err
		}

		workDir, err := os.MkdirTemp("", "sigil-acceptance-*")
		if err != nil {
			return ctx, err
		}

		world.workingDir = workDir

		originalDir, err := os.Getwd()
		if err != nil {
			return ctx, err
		}
		world.originalWorkingDir = originalDir

		if err := os.Chdir(workDir); err != nil {
			return ctx, err
		}

		world.lastStdout = ""
		world.lastStderr = ""
		world.lastErr = nil
		world.lastExitCode = 0
		world.configInitErr = nil
		world.loggingInitErr = nil
		world.runConfigInitErr = nil
		world.resolvedAppConfigPath = config.DefaultConfigPath
		world.resolvedRunConfigPath = config.DefaultRunConfigPath
		world.lifecycle = nil
		world.lifecycleTransitionErr = nil
		world.lastCreatedNode = sigilruntime.Node{}
		world.nodeCountBeforeActivity = 0
		world.nodeCountAfterActivity = 0
		world.eventLogPath = ""
		world.persistedEvents = nil
		world.eventAppendErr = nil
		world.eventValidationErr = nil
		world.eventIntegrityErr = nil
		world.rawEventLines = nil
		world.inferenceService = nil
		world.inferenceGatewayRegistry = nil
		world.inferenceSchemaRegistry = nil
		world.inferenceRequest = sigilinference.Request{}
		world.inferenceResult = sigilinference.Result{}
		world.inferenceErr = nil
		world.inferenceResolvedErr = nil
		world.inferenceRetryDelays = nil
		world.inferenceRequestBody = nil
		world.inferenceMockServer = nil
		world.rlmState = nil
		world.activeRunID = ""
		world.activeRunDir = ""
		world.activeRunEventsPath = ""
		world.activeProcessMetadata = sigilruntime.ProcessMetadata{}
		world.activeProcessSeen = false
		world.activeStopInvocation = stopInvocation{}
		world.terminalRunIDs = nil
		world.terminalStopInvocations = nil
		world.racingStopInvocations = nil
		world.invalidCaseRunIDs = nil
		world.invalidStopInvocations = nil
		world.helperStdout.Reset()
		world.helperStderr.Reset()
		world.invalidProcessCmd = nil
		world.runInspection = nil
		world.appServer = nil

		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if err := world.cleanupScenarioResources(); err != nil {
			return ctx, err
		}

		return ctx, nil
	})

	ctx.Step(`^a clean sigil working directory$`, world.aCleanSigilWorkingDirectory)
	ctx.Step(`^SIGIL configuration environment variables are cleared$`, world.sigilConfigEnvironmentVariablesAreCleared)
	ctx.Step(`^the sigil application starts without an explicit application config path$`, world.theSigilApplicationStartsWithoutAnExplicitApplicationConfigPath)
	ctx.Step(`^the sigil application starts without an explicit run config path$`, world.theSigilApplicationStartsWithoutAnExplicitRunConfigPath)
	ctx.Step(`^application config exists at "([^"]*)" with:$`, world.applicationConfigExistsAtWith)
	ctx.Step(`^run config file exists at "([^"]*)"$`, world.runConfigFileExistsAt)
	ctx.Step(`^run configuration exists at "([^"]*)" with:$`, world.runConfigurationExistsAtWith)
	ctx.Step(`^a directory exists at "([^"]*)"$`, world.aDirectoryExistsAt)
	ctx.Step(`^a file exists at "([^"]*)"$`, world.aFileExistsAt)
	ctx.Step(`^environment override "([^"]*)" is "([^"]*)"$`, world.environmentOverrideIs)
	ctx.Step(`^application configuration is resolved$`, world.applicationConfigurationIsResolved)
	ctx.Step(`^application configuration is merged$`, world.applicationConfigurationIsMerged)
	ctx.Step(`^application configuration validation runs$`, world.applicationConfigurationValidationRuns)
	ctx.Step(`^application logging is initialized$`, world.applicationLoggingIsInitialized)
	ctx.Step(`^application logging writes an info record with message "([^"]*)"$`, world.applicationLoggingWritesAnInfoRecordWithMessage)
	ctx.Step(`^a new run is created$`, world.aNewRunIsCreated)
	ctx.Step(`^a run in "([^"]*)" state$`, world.aRunInState)
	ctx.Step(`^a run transitions to "([^"]*)"$`, world.aRunTransitionsTo)
	ctx.Step(`^a run in "([^"]*)" state with active recursive nodes$`, world.aRunInStateWithActiveRecursiveNodes)
	ctx.Step(`^lifecycle initialization completes$`, world.lifecycleInitializationCompletes)
	ctx.Step(`^execution starts$`, world.executionStarts)
	ctx.Step(`^node initialization occurs$`, world.nodeInitializationOccurs)
	ctx.Step(`^a child node is created$`, world.aChildNodeIsCreated)
	ctx.Step(`^execution terminates successfully$`, world.executionTerminatesSuccessfully)
	ctx.Step(`^unrecoverable runtime failure occurs$`, world.unrecoverableRuntimeFailureOccurs)
	ctx.Step(`^explicit interruption is requested$`, world.explicitInterruptionIsRequested)
	ctx.Step(`^any further state transition is requested$`, world.anyFurtherStateTransitionIsRequested)
	ctx.Step(`^tool or code execution activity occurs$`, world.toolOrCodeExecutionActivityOccurs)
	ctx.Step(`^run configuration is resolved$`, world.runConfigurationIsResolved)
	ctx.Step(`^run configuration is merged$`, world.runConfigurationIsMerged)
	ctx.Step(`^run configuration validation runs$`, world.runConfigurationValidationRuns)
	ctx.Step(`^the default application config path is "([^"]*)"$`, world.theDefaultApplicationConfigPathIs)
	ctx.Step(`^the default run config path is "([^"]*)"$`, world.theDefaultRunConfigPathIs)
	ctx.Step(`^the application config format is "([^"]*)"$`, world.theApplicationConfigFormatIs)
	ctx.Step(`^baseline application config keys are "([^"]*)" and "([^"]*)"$`, world.baselineApplicationConfigKeysAreAnd)
	ctx.Step(`^effective application logs.level is "([^"]*)"$`, world.effectiveApplicationLogLevelIs)
	ctx.Step(`^effective application logs.dir is "([^"]*)"$`, world.effectiveApplicationLogDirIs)
	ctx.Step(`^the effective log file path is "([^"]*)"$`, world.theEffectiveLogFilePathIs)
	ctx.Step(`^the effective log target path is "([^"]*)"$`, world.theEffectiveLogTargetPathIs)
	ctx.Step(`^log records are structured JSON$`, world.logRecordsAreStructuredJSON)
	ctx.Step(`^run state is "([^"]*)"$`, world.runStateIs)
	ctx.Step(`^run transitions to "([^"]*)"$`, world.runTransitionsTo)
	ctx.Step(`^exactly one root node exists with depth=0 and parent_node_id=null$`, world.exactlyOneRootNodeExistsWithDepthAndParentNodeIDNull)
	ctx.Step(`^it references an existing parent node in the same run$`, world.itReferencesAnExistingParentNodeInTheSameRun)
	ctx.Step(`^transition validation fails$`, world.transitionValidationFails)
	ctx.Step(`^activity is recorded as node-scoped events and no additional node entity is created$`, world.activityIsRecordedAsNodeScopedEventsAndNoAdditionalNodeEntityIsCreated)
	ctx.Step(`^a persisted lifecycle run exists$`, world.aPersistedLifecycleRunExists)
	ctx.Step(`^canonical run lifecycle events are emitted$`, world.canonicalRunLifecycleEventsAreEmitted)
	ctx.Step(`^events are persisted to a per-run append-only events\.jsonl path under \.sigil runs directory$`, world.eventsArePersistedToAPerRunAppendOnlyEventsJSONLPathUnderDotSigilRunsDirectory)
	ctx.Step(`^persisted canonical run events exist$`, world.persistedCanonicalRunEventsExist)
	ctx.Step(`^persisted event identity fields are inspected$`, world.persistedEventIdentityFieldsAreInspected)
	ctx.Step(`^run_id node_id when present and event_id are UUIDv7$`, world.runIDNodeIDWhenPresentAndEventIDAreUUIDv7)
	ctx.Step(`^persisted event sequence values are inspected$`, world.persistedEventSequenceValuesAreInspected)
	ctx.Step(`^seq starts at 1 and increments contiguously by 1$`, world.seqStartsAtAndIncrementsContiguouslyBy1)
	ctx.Step(`^events\.jsonl is parsed line by line$`, world.eventsJSONLIsParsedLineByLine)
	ctx.Step(`^each non-empty line is a valid JSON event envelope$`, world.eachNonEmptyLineIsAValidJSONEventEnvelope)
	ctx.Step(`^required identity fields are validated$`, world.requiredIdentityFieldsAreValidated)
	ctx.Step(`^all events contain run_id and node-scoped events contain node_id$`, world.allEventsContainRunIDAndNodeScopedEventsContainNodeID)
	ctx.Step(`^persistence acknowledgement metrics are inspected$`, world.persistenceAcknowledgementMetricsAreInspected)
	ctx.Step(`^each appended event has been fsynced before acknowledgement$`, world.eachAppendedEventHasBeenFsyncedBeforeAcknowledgement)
	ctx.Step(`^an event append is requested with non-contiguous next sequence$`, world.anEventAppendIsRequestedWithNonContiguousNextSequence)
	ctx.Step(`^event append is rejected for non-contiguous sequence$`, world.eventAppendIsRejectedForNonContiguousSequence)
	ctx.Step(`^events\.jsonl is corrupted with malformed or partial lines$`, world.eventsJSONLIsCorruptedWithMalformedOrPartialLines)
	ctx.Step(`^event-log integrity validation executes$`, world.eventLogIntegrityValidationExecutes)
	ctx.Step(`^integrity validation fails for run recovery$`, world.integrityValidationFailsForRunRecovery)
	ctx.Step(`^an append attempts in-place sequence rewrite$`, world.anAppendAttemptsInPlaceSequenceRewrite)
	ctx.Step(`^event append is rejected by immutable event-store contract$`, world.eventAppendIsRejectedByImmutableEventStoreContract)
	ctx.Step(`^event envelopes are inspected$`, world.eventEnvelopesAreInspected)
	ctx.Step(`^schema_version exists and equals v1$`, world.schemaVersionExistsAndEqualsV1)
	ctx.Step(`^canonical v1 run-event validation rules$`, world.canonicalV1RunEventValidationRules)
	ctx.Step(`^canonical core lifecycle event types are validated$`, world.canonicalCoreLifecycleEventTypesAreValidated)
	ctx.Step(`^canonical runtime event types are validated$`, world.canonicalCoreLifecycleEventTypesAreValidated)
	ctx.Step(`^only canonical v1 lifecycle event types are accepted$`, world.onlyCanonicalV1LifecycleEventTypesAreAccepted)
	ctx.Step(`^only canonical v1 runtime event types are accepted$`, world.onlyCanonicalV1LifecycleEventTypesAreAccepted)
	ctx.Step(`^canonical v1 lifecycle events with payloads$`, world.canonicalV1LifecycleEventsWithPayloads)
	ctx.Step(`^canonical v1 events with payloads$`, world.canonicalV1LifecycleEventsWithPayloads)
	ctx.Step(`^strict payload schema validation is executed$`, world.strictPayloadSchemaValidationIsExecuted)
	ctx.Step(`^required fields types and invariants are enforced per event type$`, world.requiredFieldsTypesAndInvariantsAreEnforcedPerEventType)
	ctx.Step(`^v1 event envelopes with unknown fields or unknown type$`, world.v1EventEnvelopesWithUnknownFieldsOrUnknownType)
	ctx.Step(`^strict v1 extensibility validation is executed$`, world.strictV1ExtensibilityValidationIsExecuted)
	ctx.Step(`^validation fails and events are rejected$`, world.validationFailsAndEventsAreRejected)
	ctx.Step(`^a core lifecycle event payload includes deferred non-core fields$`, world.aCoreLifecycleEventPayloadIncludesDeferredNonCoreFields)
	ctx.Step(`^a canonical v1 event payload includes deferred non-core fields$`, world.aCoreLifecycleEventPayloadIncludesDeferredNonCoreFields)
	ctx.Step(`^core v1 payload validation executes$`, world.coreV1PayloadValidationExecutes)
	ctx.Step(`^canonical v1 payload validation executes$`, world.coreV1PayloadValidationExecutes)
	ctx.Step(`^deferred non-core fields are rejected as out-of-contract$`, world.deferredNonCoreFieldsAreRejectedAsOutOfContract)
	ctx.Step(`^event type validation includes node step tracking events$`, world.eventTypeValidationIncludesNodeStepTrackingEvents)
	ctx.Step(`^node.step.started and node.step.completed are accepted canonical event types$`, world.nodeStepStartedAndNodeStepCompletedAreAcceptedCanonicalEventTypes)
	ctx.Step(`^canonical node step events with payloads$`, world.canonicalNodeStepEventsWithPayloads)
	ctx.Step(`^required node step fields and decision-action invariants are enforced$`, world.requiredNodeStepFieldsAndDecisionActionInvariantsAreEnforced)
	ctx.Step(`^event type validation includes node turn events$`, world.eventTypeValidationIncludesNodeTurnEvents)
	ctx.Step(`^node.turn.user and node.turn.model are accepted canonical event types$`, world.nodeTurnUserAndNodeTurnModelAreAcceptedCanonicalEventTypes)
	ctx.Step(`^canonical node turn events with payloads$`, world.canonicalNodeTurnEventsWithPayloads)
	ctx.Step(`^required node turn fields are enforced and role values match event type semantics$`, world.requiredNodeTurnFieldsAreEnforcedAndRoleValuesMatchEventTypeSemantics)
	ctx.Step(`^canonical node action execution events with payloads$`, world.canonicalNodeActionExecutionEventsWithPayloads)
	ctx.Step(`^node.action.executed payload enforces single-action continue invariants$`, world.nodeActionExecutedPayloadEnforcesSingleActionContinueInvariants)
	ctx.Step(`^effective run llm.provider is "([^"]*)"$`, world.effectiveRunLLMProviderIs)
	ctx.Step(`^effective run llm.model is "([^"]*)"$`, world.effectiveRunLLMModelIs)
	ctx.Step(`^effective run llm.gateway is "([^"]*)"$`, world.effectiveRunLLMGatewayIs)
	ctx.Step(`^effective run llm.openrouter.base_url is "([^"]*)"$`, world.effectiveRunLLMOpenRouterBaseURLIs)
	ctx.Step(`^effective run llm.openrouter.request_timeout_ms is (\d+)$`, world.effectiveRunLLMOpenRouterRequestTimeoutMSIs)
	ctx.Step(`^effective run llm.openrouter.api_key_env is "([^"]*)"$`, world.effectiveRunLLMOpenRouterAPIKeyEnvIs)
	ctx.Step(`^effective run rlm.enabled is (true|false)$`, world.effectiveRunRLMEnabledIs)
	ctx.Step(`^effective run rlm.max_depth is (\d+)$`, world.effectiveRunRLMMaxDepthIs)
	ctx.Step(`^effective run guardrails.max_steps_per_node is (\d+)$`, world.effectiveRunGuardrailsMaxStepsPerNodeIs)
	ctx.Step(`^effective run guardrails.max_total_steps_per_run is (\d+)$`, world.effectiveRunGuardrailsMaxTotalStepsPerRunIs)
	ctx.Step(`^effective run guardrails.max_run_duration_ms is (\d+)$`, world.effectiveRunGuardrailsMaxRunDurationMSIs)
	ctx.Step(`^effective run guardrails.max_consecutive_step_failures is (\d+)$`, world.effectiveRunGuardrailsMaxConsecutiveStepFailuresIs)
	ctx.Step(`^effective run guardrails.max_total_tokens is (\d+)$`, world.effectiveRunGuardrailsMaxTotalTokensIs)
	ctx.Step(`^effective run guardrails.max_total_tokens is unset$`, world.effectiveRunGuardrailsMaxTotalTokensIsUnset)
	ctx.Step(`^effective run guardrails.max_total_cost_usd is "([^"]*)"$`, world.effectiveRunGuardrailsMaxTotalCostUSDIs)
	ctx.Step(`^effective run guardrails.max_total_cost_usd is unset$`, world.effectiveRunGuardrailsMaxTotalCostUSDIsUnset)
	ctx.Step(`^effective run accounting.pricing_version is "([^"]*)"$`, world.effectiveRunAccountingPricingVersionIs)
	ctx.Step(`^effective run accounting fallback pricing for provider "([^"]*)" model "([^"]*)" uses input rate (\d+) output rate (\d+) reasoning rate (\d+)$`, world.effectiveRunAccountingFallbackPricingForProviderModelUsesRates)
	ctx.Step(`^effective run llm.reasoning.enabled is (true|false)$`, world.effectiveRunLLMReasoningEnabledIs)
	ctx.Step(`^effective run llm.reasoning.effort is "([^"]*)"$`, world.effectiveRunLLMReasoningEffortIs)
	ctx.Step(`^application configuration initialization fails$`, world.applicationConfigurationInitializationFails)
	ctx.Step(`^application logging initialization fails$`, world.applicationLoggingInitializationFails)
	ctx.Step(`^run configuration initialization succeeds$`, world.runConfigurationInitializationSucceeds)
	ctx.Step(`^run configuration initialization fails$`, world.runConfigurationInitializationFails)
	ctx.Step(`^the sigil executable is available$`, world.theSigilExecutableIsAvailable)
	ctx.Step("^a user runs `([^`]*)`$", world.aUserRuns)
	ctx.Step(`^command exits with status code (\d+)$`, world.commandExitsWithStatusCode)
	ctx.Step(`^command exits non-zero$`, world.commandExitsNonZero)
	ctx.Step("^command output contains `([^`]*)`$", world.commandOutputContains)
	ctx.Step("^command error contains `([^`]*)`$", world.commandErrorContains)
	ctx.Step(`^root usage/help is printed$`, world.rootUsageHelpIsPrinted)
	ctx.Step(`^run usage/help is printed$`, world.runUsageHelpIsPrinted)
	ctx.Step(`^stop usage/help is printed$`, world.stopUsageHelpIsPrinted)
	ctx.Step(`^a local CLI run is actively executing$`, world.aLocalCLIRunIsActivelyExecuting)
	ctx.Step("^a user runs `sigil run stop` for the active run$", world.aUserRunsSigilRunStopForTheActiveRun)
	ctx.Step("^a user runs `sigil run stop -o json` for the active run$", world.aUserRunsSigilRunStopJSONForTheActiveRun)
	ctx.Step(`^the active run transitions to "([^"]*)"$`, world.theActiveRunTransitionsTo)
	ctx.Step(`^stdout contains one JSON stop result with run_id stop_requested state and events_path$`, world.stdoutContainsOneJSONStopResultWithRunIDStopRequestedStateAndEventsPath)
	ctx.Step(`^the run lifecycle and stop request metadata are inspected$`, world.theRunLifecycleAndStopRequestMetadataAreInspected)
	ctx.Step(`^process\.json exists for the active run$`, world.processJSONExistsForTheActiveRun)
	ctx.Step(`^stop-request\.json is written before SIGTERM is issued$`, world.stopRequestJSONIsWrittenBeforeSIGTERMIsIssued)
	ctx.Step(`^local CLI completed failed and interrupted runs exist$`, world.localCLICompletedFailedAndInterruptedRunsExist)
	ctx.Step(`^terminal stop commands are executed for those runs$`, world.terminalStopCommandsAreExecutedForThoseRuns)
	ctx.Step(`^each terminal stop command exits with status code 0 and returns stop_requested=false$`, world.eachTerminalStopCommandExitsWithStatusCode0AndReturnsStopRequestedFalse)
	ctx.Step(`^stop requests lose the race to completed and failed local CLI runs$`, world.stopRequestsLoseTheRaceToCompletedAndFailedLocalCLIRuns)
	ctx.Step(`^stop commands are executed for those racing runs$`, world.stopCommandsAreExecutedForThoseRacingRuns)
	ctx.Step(`^the JSON stop results contain stop_requested=true and the observed terminal states$`, world.theJSONStopResultsContainStopRequestedTrueAndTheObservedTerminalStates)
	ctx.Step(`^sigil run stop targets unknown corrupt stale and missing-process run state$`, world.sigilRunStopTargetsUnknownCorruptStaleAndMissingProcessRunState)
	ctx.Step(`^stop commands are executed for those invalid control cases$`, world.stopCommandsAreExecutedForThoseInvalidControlCases)
	ctx.Step(`^each invalid control case exits non-zero$`, world.eachInvalidControlCaseExitsNonZero)
	ctx.Step(`^a local CLI run has persisted run\.queued but not run\.running$`, world.aLocalCLIRunHasPersistedRunQueuedButNotRunRunning)
	ctx.Step(`^run\.interrupted contains reason user_request interrupted_by "([^"]*)" and partial accounting$`, world.runInterruptedContainsReasonUserRequestInterruptedByAndPartialAccounting)
	ctx.Step(`^run\.interrupted contains reason user_request interrupted_by cli\.run\.stop and partial accounting$`, world.runInterruptedContainsReasonUserRequestInterruptedByCLIRunStopAndPartialAccounting)
	ctx.Step(`^interrupted stop handling does not append synthetic node\.failed or node\.step\.completed records$`, world.interruptedStopHandlingDoesNotAppendSyntheticNodeFailedOrNodeStepCompletedRecords)
	ctx.Step(`^no default start config files exist$`, world.noDefaultStartConfigFilesExist)
	ctx.Step(`^no default run config files exist$`, world.noDefaultRunConfigFilesExist)
	ctx.Step(`^CLI run start mock responses are configured for "([^"]*)"$`, world.cliRunStartMockResponsesAreConfiguredFor)

	registerRLMHarnessSteps(ctx, world)
	registerInferenceSteps(ctx, world)
	registerRunInspectionSteps(ctx, world)
	registerAppServerSteps(ctx, world)
}

func (w *harnessWorld) cleanupScenarioResources() error {
	var cleanupErrs []error

	if w.inferenceMockServer != nil {
		w.inferenceMockServer.Close()
		w.inferenceMockServer = nil
	}
	if w.appServer != nil {
		if err := w.appServer.close(); err != nil {
			cleanupErrs = append(cleanupErrs, err)
		}
		w.appServer = nil
	}
	if err := w.stopHelperProcess(); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if w.helperCmd != nil && w.helperCmd.ProcessState != nil {
		w.helperCmd = nil
	}
	if err := w.stopInvalidProcess(); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	if w.invalidProcessCmd != nil && w.invalidProcessCmd.ProcessState != nil {
		w.invalidProcessCmd = nil
	}
	if w.lifecycle != nil {
		if err := w.lifecycle.Close(); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to close lifecycle resources; %w", err))
		}
		w.lifecycle = nil
	}

	_ = logging.Close()

	if w.originalWorkingDir != "" {
		if err := os.Chdir(w.originalWorkingDir); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to restore working directory %q; %w", w.originalWorkingDir, err))
		}
		w.originalWorkingDir = ""
	}
	if w.workingDir != "" {
		if err := removeAllWithRetry(w.workingDir, 500*time.Millisecond); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to remove acceptance working directory %q; %w", w.workingDir, err))
		}
		w.workingDir = ""
	}
	if err := cleanupCurrentRunControlHelperDirs(w.helperDir); err != nil {
		cleanupErrs = append(cleanupErrs, err)
	}
	w.helperDir = ""

	return errors.Join(cleanupErrs...)
}

func cleanupCurrentRunControlHelperDirs(activeHelperDir string) error {
	moduleRoot, err := sigilModuleRootDir()
	if err != nil {
		if strings.TrimSpace(activeHelperDir) == "" {
			return fmt.Errorf("failed to resolve sigil module root for helper cleanup; %w", err)
		}
		return errors.Join(
			fmt.Errorf("failed to resolve sigil module root for helper cleanup; %w", err),
			cleanupRunControlHelperDirs("", activeHelperDir),
		)
	}
	return cleanupRunControlHelperDirs(moduleRoot, activeHelperDir)
}

func cleanupRunControlHelperDirs(moduleRoot string, activeHelperDir string) error {
	candidateDirs := map[string]struct{}{}
	if strings.TrimSpace(activeHelperDir) != "" {
		candidateDirs[activeHelperDir] = struct{}{}
	}
	if strings.TrimSpace(moduleRoot) != "" {
		matches, err := filepath.Glob(filepath.Join(moduleRoot, ".acceptance-run-stop-helper-*"))
		if err != nil {
			return fmt.Errorf("failed to enumerate run-stop helper dirs under %q; %w", moduleRoot, err)
		}
		for _, match := range matches {
			candidateDirs[match] = struct{}{}
		}
	}

	var cleanupErrs []error
	for dir := range candidateDirs {
		if err := removeAllWithRetry(dir, 500*time.Millisecond); err != nil {
			cleanupErrs = append(cleanupErrs, fmt.Errorf("failed to remove run-stop helper dir %q; %w", dir, err))
		}
	}
	return errors.Join(cleanupErrs...)
}

func removeAllWithRetry(path string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		lastErr = os.RemoveAll(path)
		if lastErr == nil {
			return nil
		}
		if time.Now().After(deadline) {
			return lastErr
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func (w *harnessWorld) aCleanSigilWorkingDirectory() error {
	return nil
}

func (w *harnessWorld) sigilConfigEnvironmentVariablesAreCleared() error {
	keys := []string{
		"SIGIL_LOGS_LEVEL",
		"SIGIL_LOGS_DIR",
		"OPENROUTER_API_KEY",
		"SIGIL_RUN_LLM_PROVIDER",
		"SIGIL_RUN_LLM_MODEL",
		"SIGIL_RUN_LLM_GATEWAY",
		"SIGIL_RUN_LLM_REASONING_ENABLED",
		"SIGIL_RUN_LLM_REASONING_EFFORT",
		"SIGIL_RUN_LLM_OPENROUTER_BASE_URL",
		"SIGIL_RUN_LLM_OPENROUTER_REQUEST_TIMEOUT_MS",
		"SIGIL_RUN_LLM_OPENROUTER_API_KEY_ENV",
		"SIGIL_RUN_RLM_ENABLED",
		"SIGIL_RUN_RLM_MAX_DEPTH",
		"SIGIL_RUN_GUARDRAILS_MAX_STEPS_PER_NODE",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_STEPS_PER_RUN",
		"SIGIL_RUN_GUARDRAILS_MAX_RUN_DURATION_MS",
		"SIGIL_RUN_GUARDRAILS_MAX_CONSECUTIVE_STEP_FAILURES",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_TOKENS",
		"SIGIL_RUN_GUARDRAILS_MAX_TOTAL_COST_USD",
		"SIGIL_RUN_ACCOUNTING_PRICING_VERSION",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_INPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_OUTPUT_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_ACCOUNTING_FALLBACK_PRICING_OPENAI_GPT_5_1_REASONING_MICROUSD_PER_MILLION_TOKENS",
		"SIGIL_RUN_SYSTEM_PROMPT_APPEND",
		"SIGIL_RUN_PROMPT",
		"SIGIL_RUN_PROMPT_TEMPLATE",
		"SIGIL_RUN_CONTEXT",
		"SIGIL_RUN_CONTEXT_TEMPLATE",
	}

	for _, key := range keys {
		if err := os.Unsetenv(key); err != nil {
			return err
		}
	}

	return nil
}

func (w *harnessWorld) theSigilApplicationStartsWithoutAnExplicitApplicationConfigPath() error {
	w.resolvedAppConfigPath = config.DefaultConfigPath
	return nil
}

func (w *harnessWorld) theSigilApplicationStartsWithoutAnExplicitRunConfigPath() error {
	w.resolvedRunConfigPath = config.DefaultRunConfigPath
	return nil
}

func (w *harnessWorld) applicationConfigExistsAtWith(path string, body *godog.DocString) error {
	w.resolvedAppConfigPath = path
	return os.WriteFile(filepath.Clean(path), []byte(body.Content), 0o644)
}

func (w *harnessWorld) runConfigFileExistsAt(path string) error {
	content := "prompt: test\ncontext: test\nllm:\n  provider: openai\n  model: gpt-5.1\n"
	return os.WriteFile(filepath.Clean(path), []byte(content), 0o644)
}

func (w *harnessWorld) runConfigurationExistsAtWith(path string, body *godog.DocString) error {
	w.resolvedRunConfigPath = path
	return os.WriteFile(filepath.Clean(path), []byte(body.Content), 0o644)
}

func (w *harnessWorld) aDirectoryExistsAt(path string) error {
	return os.MkdirAll(filepath.Clean(path), 0o755)
}

func (w *harnessWorld) aFileExistsAt(path string) error {
	return os.WriteFile(filepath.Clean(path), []byte("fixture"), 0o644)
}

func (w *harnessWorld) environmentOverrideIs(key string, value string) error {
	return os.Setenv(key, value)
}

func (w *harnessWorld) applicationConfigurationIsResolved() error {
	w.configInitErr = config.Init()
	return nil
}

func (w *harnessWorld) applicationConfigurationIsMerged() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	return nil
}

func (w *harnessWorld) applicationConfigurationValidationRuns() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	return nil
}

func (w *harnessWorld) applicationLoggingIsInitialized() error {
	w.configInitErr = config.InitFromPath(w.resolvedAppConfigPath)
	if w.configInitErr != nil {
		w.loggingInitErr = fmt.Errorf("config initialization failed; %w", w.configInitErr)
		return nil
	}

	w.loggingInitErr = logging.Init(config.MustGet())
	return nil
}

func (w *harnessWorld) applicationLoggingWritesAnInfoRecordWithMessage(message string) error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	slog.Info(message)
	return nil
}

func (w *harnessWorld) aNewRunIsCreated() error {
	return w.resetLifecycleToState(sigilruntime.RunStateQueued)
}

func (w *harnessWorld) aRunInState(state string) error {
	runState, err := parseRunState(state)
	if err != nil {
		return err
	}

	return w.resetLifecycleToState(runState)
}

func (w *harnessWorld) aRunTransitionsTo(state string) error {
	runState, err := parseRunState(state)
	if err != nil {
		return err
	}

	return w.resetLifecycleToState(runState)
}

func (w *harnessWorld) aRunInStateWithActiveRecursiveNodes(state string) error {
	runState, err := parseRunState(state)
	if err != nil {
		return err
	}
	if runState != sigilruntime.RunStateRunning {
		return fmt.Errorf("active recursive nodes are only defined for running state, got %q", runState)
	}

	if err := w.resetLifecycleToState(sigilruntime.RunStateRunning); err != nil {
		return err
	}

	rootNode, err := w.rootNode()
	if err != nil {
		return err
	}

	childNode, err := w.lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		return fmt.Errorf("failed to create recursive child node; %w", err)
	}
	w.lastCreatedNode = childNode
	return nil
}

func (w *harnessWorld) lifecycleInitializationCompletes() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before completion assertion")
	}

	return nil
}

func (w *harnessWorld) executionStarts() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before execution start")
	}

	if err := w.lifecycle.StartExecution(); err != nil {
		return err
	}

	return nil
}

func (w *harnessWorld) nodeInitializationOccurs() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before node initialization")
	}

	return nil
}

func (w *harnessWorld) aChildNodeIsCreated() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before child creation")
	}

	rootNode, err := w.rootNode()
	if err != nil {
		return err
	}

	childNode, err := w.lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		return err
	}

	w.lastCreatedNode = childNode
	return nil
}

func (w *harnessWorld) executionTerminatesSuccessfully() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before completion")
	}

	return w.lifecycle.Complete()
}

func (w *harnessWorld) unrecoverableRuntimeFailureOccurs() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before failure transition")
	}

	return w.lifecycle.Fail()
}

func (w *harnessWorld) explicitInterruptionIsRequested() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before interruption transition")
	}

	return w.lifecycle.Interrupt()
}

func (w *harnessWorld) anyFurtherStateTransitionIsRequested() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before requesting additional transition")
	}

	w.lifecycleTransitionErr = w.lifecycle.StartExecution()
	return nil
}

func (w *harnessWorld) toolOrCodeExecutionActivityOccurs() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before recording node-scoped activity")
	}

	targetNodeID := w.lastCreatedNode.ID
	if strings.TrimSpace(targetNodeID) == "" {
		rootNode, err := w.rootNode()
		if err != nil {
			return err
		}
		targetNodeID = rootNode.ID
	}

	w.nodeCountBeforeActivity = len(w.lifecycle.Nodes())
	if err := w.lifecycle.RecordNodeActivity(targetNodeID, sigilruntime.NodeActivityKindToolExec); err != nil {
		return err
	}
	if err := w.lifecycle.RecordNodeActivity(targetNodeID, sigilruntime.NodeActivityKindCodeExec); err != nil {
		return err
	}
	w.nodeCountAfterActivity = len(w.lifecycle.Nodes())

	return nil
}

func (w *harnessWorld) runConfigurationIsResolved() error {
	w.runConfigInitErr = config.InitRun()
	return nil
}

func (w *harnessWorld) runConfigurationIsMerged() error {
	w.runConfigInitErr = config.InitRunFromPath(w.resolvedRunConfigPath)
	return nil
}

func (w *harnessWorld) runConfigurationValidationRuns() error {
	w.runConfigInitErr = config.InitRunFromPath(w.resolvedRunConfigPath)
	return nil
}

func (w *harnessWorld) theDefaultApplicationConfigPathIs(expectedPath string) error {
	if config.DefaultConfigPath != expectedPath {
		return fmt.Errorf("expected default path %q, got %q", expectedPath, config.DefaultConfigPath)
	}

	return nil
}

func (w *harnessWorld) theDefaultRunConfigPathIs(expectedPath string) error {
	if config.DefaultRunConfigPath != expectedPath {
		return fmt.Errorf("expected run default path %q, got %q", expectedPath, config.DefaultRunConfigPath)
	}

	return nil
}

func (w *harnessWorld) theApplicationConfigFormatIs(expectedFormat string) error {
	if strings.ToLower(expectedFormat) != "yaml" {
		return fmt.Errorf("unsupported expected format %q", expectedFormat)
	}

	return nil
}

func (w *harnessWorld) baselineApplicationConfigKeysAreAnd(expectedKeyOne string, expectedKeyTwo string) error {
	tags := collectMapstructureTags(reflect.TypeOf(config.Config{}), "")

	if _, ok := tags[expectedKeyOne]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyOne)
	}
	if _, ok := tags[expectedKeyTwo]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyTwo)
	}

	return nil
}

func collectMapstructureTags(typeInfo reflect.Type, prefix string) map[string]struct{} {
	collected := map[string]struct{}{}
	if typeInfo.Kind() == reflect.Pointer {
		typeInfo = typeInfo.Elem()
	}
	if typeInfo.Kind() != reflect.Struct {
		return collected
	}

	for index := 0; index < typeInfo.NumField(); index++ {
		field := typeInfo.Field(index)
		tag := strings.TrimSpace(field.Tag.Get("mapstructure"))
		if tag == "" || tag == "-" {
			continue
		}
		fullKey := tag
		if prefix != "" {
			fullKey = prefix + "." + tag
		}
		collected[fullKey] = struct{}{}

		nested := field.Type
		if nested.Kind() == reflect.Pointer {
			nested = nested.Elem()
		}
		if nested.Kind() == reflect.Struct && nested.PkgPath() == typeInfo.PkgPath() {
			for nestedKey := range collectMapstructureTags(nested, fullKey) {
				collected[nestedKey] = struct{}{}
			}
		}
	}

	return collected
}

func (w *harnessWorld) effectiveApplicationLogLevelIs(expectedLevel string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.Logs.Level != expectedLevel {
		return fmt.Errorf("expected logs.level %q, got %q", expectedLevel, cfg.Logs.Level)
	}

	return nil
}

func (w *harnessWorld) effectiveApplicationLogDirIs(expectedDir string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	expectedPath, err := config.ExpandPath(expectedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve expected logs.dir %q; %w", expectedDir, err)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.Logs.Dir != expectedPath {
		return fmt.Errorf("expected logs.dir %q, got %q", expectedPath, cfg.Logs.Dir)
	}

	return nil
}

func (w *harnessWorld) theEffectiveLogFilePathIs(expectedPath string) error {
	return w.assertEffectiveLogPath(expectedPath)
}

func (w *harnessWorld) theEffectiveLogTargetPathIs(expectedPath string) error {
	return w.assertEffectiveLogPath(expectedPath)
}

func (w *harnessWorld) logRecordsAreStructuredJSON() error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	logPath, err := logging.ActiveLogFilePath()
	if err != nil {
		return err
	}

	content, err := os.ReadFile(filepath.Clean(logPath))
	if err != nil {
		return fmt.Errorf("failed to read log file %q; %w", logPath, err)
	}

	lines := strings.Split(string(content), "\n")
	recordCount := 0
	for _, rawLine := range lines {
		line := strings.TrimSpace(rawLine)
		if line == "" {
			continue
		}

		record := make(map[string]any)
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			return fmt.Errorf("expected structured JSON record, got invalid line %q; %w", line, err)
		}

		if _, ok := record["time"]; !ok {
			return fmt.Errorf("structured JSON record missing time field: %v", record)
		}
		if _, ok := record["level"]; !ok {
			return fmt.Errorf("structured JSON record missing level field: %v", record)
		}
		if _, ok := record["msg"]; !ok {
			return fmt.Errorf("structured JSON record missing msg field: %v", record)
		}

		recordCount++
	}

	if recordCount == 0 {
		return fmt.Errorf("expected at least one structured JSON record")
	}

	return nil
}

func (w *harnessWorld) runStateIs(expectedStateRaw string) error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before state assertion")
	}

	expectedState, err := parseRunState(expectedStateRaw)
	if err != nil {
		return err
	}

	if w.lifecycle.State() != expectedState {
		return fmt.Errorf("expected run state %q, got %q", expectedState, w.lifecycle.State())
	}

	return nil
}

func (w *harnessWorld) runTransitionsTo(expectedStateRaw string) error {
	return w.runStateIs(expectedStateRaw)
}

func (w *harnessWorld) exactlyOneRootNodeExistsWithDepthAndParentNodeIDNull() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before root-node assertion")
	}

	nodes := w.lifecycle.Nodes()
	if len(nodes) != 1 {
		return fmt.Errorf("expected exactly one node after start, got %d", len(nodes))
	}

	rootNode := nodes[0]
	if rootNode.Depth != 0 {
		return fmt.Errorf("expected root depth 0, got %d", rootNode.Depth)
	}
	if rootNode.ParentNodeID != nil {
		return fmt.Errorf("expected root parent_node_id nil, got %v", rootNode.ParentNodeID)
	}

	return nil
}

func (w *harnessWorld) itReferencesAnExistingParentNodeInTheSameRun() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before child-parent assertion")
	}
	if strings.TrimSpace(w.lastCreatedNode.ID) == "" {
		return fmt.Errorf("expected child node to be created before parent assertion")
	}
	if w.lastCreatedNode.ParentNodeID == nil {
		return fmt.Errorf("expected child node to include parent_node_id")
	}

	parentNodeID := *w.lastCreatedNode.ParentNodeID
	nodes := w.lifecycle.Nodes()
	parentFound := false
	for _, node := range nodes {
		if node.ID == parentNodeID {
			parentFound = true
			if node.RunID != w.lastCreatedNode.RunID {
				return fmt.Errorf("expected same run_id for parent and child, parent=%q child=%q", node.RunID, w.lastCreatedNode.RunID)
			}
			break
		}
	}

	if !parentFound {
		return fmt.Errorf("expected parent node %q to exist in same run", parentNodeID)
	}

	return nil
}

func (w *harnessWorld) transitionValidationFails() error {
	if w.lifecycleTransitionErr == nil {
		return fmt.Errorf("expected transition validation failure")
	}

	if !errors.Is(w.lifecycleTransitionErr, sigilruntime.ErrTerminalState) {
		return fmt.Errorf("expected terminal transition error, got %v", w.lifecycleTransitionErr)
	}

	return nil
}

func (w *harnessWorld) activityIsRecordedAsNodeScopedEventsAndNoAdditionalNodeEntityIsCreated() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before activity assertion")
	}

	if w.nodeCountBeforeActivity != w.nodeCountAfterActivity {
		return fmt.Errorf("expected node count unchanged during activity recording, before=%d after=%d", w.nodeCountBeforeActivity, w.nodeCountAfterActivity)
	}

	activities := w.lifecycle.Activities()
	if len(activities) < 2 {
		return fmt.Errorf("expected at least two node-scoped activity records, got %d", len(activities))
	}

	foundToolExec := false
	foundCodeExec := false
	for _, activity := range activities {
		if activity.RunID != w.lifecycle.RunID() {
			return fmt.Errorf("expected activity run_id %q, got %q", w.lifecycle.RunID(), activity.RunID)
		}

		switch activity.Kind {
		case sigilruntime.NodeActivityKindToolExec:
			foundToolExec = true
		case sigilruntime.NodeActivityKindCodeExec:
			foundCodeExec = true
		}
	}

	if !foundToolExec || !foundCodeExec {
		return fmt.Errorf("expected both tool_exec and code_exec activities, got %+v", activities)
	}

	return nil
}

func (w *harnessWorld) aPersistedLifecycleRunExists() error {
	if err := w.resetLifecycleToState(sigilruntime.RunStateQueued); err != nil {
		return err
	}

	return w.capturePersistedEvents()
}

func (w *harnessWorld) canonicalRunLifecycleEventsAreEmitted() error {
	if err := w.aPersistedLifecycleRunExists(); err != nil {
		return err
	}

	if err := w.lifecycle.StartExecution(); err != nil {
		return err
	}

	rootNode, err := w.rootNode()
	if err != nil {
		return err
	}

	childNode, err := w.lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		return err
	}
	w.lastCreatedNode = childNode

	if err := w.lifecycle.Complete(); err != nil {
		return err
	}

	return w.capturePersistedEvents()
}

func (w *harnessWorld) eventsArePersistedToAPerRunAppendOnlyEventsJSONLPathUnderDotSigilRunsDirectory() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}

	actualPath := filepath.Clean(w.eventLogPath)
	expectedSegment := filepath.Join(".sigil", "runs", w.lifecycle.RunID(), "events.jsonl")
	if !strings.Contains(actualPath, expectedSegment) {
		return fmt.Errorf("expected events path to contain %q, got %q", expectedSegment, actualPath)
	}

	expectedSuffix := filepath.Join(w.lifecycle.RunID(), "events.jsonl")
	if !strings.HasSuffix(actualPath, expectedSuffix) {
		return fmt.Errorf("expected events path suffix %q, got %q", expectedSuffix, actualPath)
	}

	info, err := os.Stat(actualPath)
	if err != nil {
		return fmt.Errorf("expected persisted events file at %q; %w", actualPath, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("expected events path to be regular file, got mode %s", info.Mode())
	}

	return nil
}

func (w *harnessWorld) persistedCanonicalRunEventsExist() error {
	if err := w.canonicalRunLifecycleEventsAreEmitted(); err != nil {
		return err
	}
	return w.capturePersistedEvents()
}

func (w *harnessWorld) persistedEventIdentityFieldsAreInspected() error {
	return w.capturePersistedEvents()
}

func (w *harnessWorld) runIDNodeIDWhenPresentAndEventIDAreUUIDv7() error {
	if len(w.persistedEvents) == 0 {
		if err := w.capturePersistedEvents(); err != nil {
			return err
		}
	}

	for _, event := range w.persistedEvents {
		if !isUUIDv7String(event.RunID) {
			return fmt.Errorf("expected run_id UUIDv7, got %q", event.RunID)
		}
		if !isUUIDv7String(event.EventID) {
			return fmt.Errorf("expected event_id UUIDv7, got %q", event.EventID)
		}
		if event.NodeID != nil && !isUUIDv7String(*event.NodeID) {
			return fmt.Errorf("expected node_id UUIDv7, got %q", *event.NodeID)
		}
	}

	return nil
}

func (w *harnessWorld) persistedEventSequenceValuesAreInspected() error {
	return w.capturePersistedEvents()
}

func (w *harnessWorld) seqStartsAtAndIncrementsContiguouslyBy1() error {
	if len(w.persistedEvents) == 0 {
		if err := w.capturePersistedEvents(); err != nil {
			return err
		}
	}

	for index, event := range w.persistedEvents {
		expected := int64(index + 1)
		if event.Seq != expected {
			return fmt.Errorf("expected event seq %d, got %d", expected, event.Seq)
		}
	}

	return nil
}

func (w *harnessWorld) eventsJSONLIsParsedLineByLine() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}

	content, err := os.ReadFile(filepath.Clean(w.eventLogPath))
	if err != nil {
		return fmt.Errorf("failed to read events file %q; %w", w.eventLogPath, err)
	}

	lines := strings.Split(string(content), "\n")
	w.rawEventLines = w.rawEventLines[:0]
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		w.rawEventLines = append(w.rawEventLines, line)
	}

	return nil
}

func (w *harnessWorld) eachNonEmptyLineIsAValidJSONEventEnvelope() error {
	if len(w.rawEventLines) == 0 {
		return fmt.Errorf("expected parsed event lines before envelope validation")
	}

	for _, line := range w.rawEventLines {
		if _, err := sigilruntime.ParseEventEnvelopeStrict([]byte(line)); err != nil {
			return fmt.Errorf("expected valid JSON event envelope line %q; %w", line, err)
		}
	}

	return nil
}

func (w *harnessWorld) requiredIdentityFieldsAreValidated() error {
	return w.capturePersistedEvents()
}

func (w *harnessWorld) allEventsContainRunIDAndNodeScopedEventsContainNodeID() error {
	if len(w.persistedEvents) == 0 {
		if err := w.capturePersistedEvents(); err != nil {
			return err
		}
	}

	for _, event := range w.persistedEvents {
		if strings.TrimSpace(event.RunID) == "" {
			return fmt.Errorf("event %q is missing run_id", event.EventID)
		}
		if strings.HasPrefix(string(event.Type), "node.") && event.NodeID == nil {
			return fmt.Errorf("node-scoped event %q is missing node_id", event.EventID)
		}
	}

	return nil
}

func (w *harnessWorld) persistenceAcknowledgementMetricsAreInspected() error {
	return w.capturePersistedEvents()
}

func (w *harnessWorld) eachAppendedEventHasBeenFsyncedBeforeAcknowledgement() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle before fsync assertion")
	}
	if len(w.persistedEvents) == 0 {
		if err := w.capturePersistedEvents(); err != nil {
			return err
		}
	}

	syncCount := w.lifecycle.EventStoreSyncCount()
	if syncCount != len(w.persistedEvents) {
		return fmt.Errorf("expected sync count %d, got %d", len(w.persistedEvents), syncCount)
	}

	return nil
}

func (w *harnessWorld) anEventAppendIsRequestedWithNonContiguousNextSequence() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}
	if len(w.persistedEvents) == 0 {
		return fmt.Errorf("expected existing events before non-contiguous append request")
	}

	eventID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	lastEvent := w.persistedEvents[len(w.persistedEvents)-1]
	w.eventAppendErr = w.lifecycle.EventStore().Append(sigilruntime.EventEnvelope{
		EventID:       eventID.String(),
		SchemaVersion: sigilruntime.SchemaVersionV1,
		RunID:         w.lifecycle.RunID(),
		Seq:           lastEvent.Seq + 2,
		Timestamp:     time.Now().UTC(),
		Type:          sigilruntime.EventTypeRunRunning,
		CausationID:   lastEvent.EventID,
		CorrelationID: w.lifecycle.RunID(),
		Payload: sigilruntime.RunRunningPayload{
			Executor: "rlm",
			MaxDepth: 0,
		},
	})

	return nil
}

func (w *harnessWorld) eventAppendIsRejectedForNonContiguousSequence() error {
	if !errors.Is(w.eventAppendErr, sigilruntime.ErrNonContiguousSequence) {
		return fmt.Errorf("expected non-contiguous sequence append error, got %v", w.eventAppendErr)
	}
	return nil
}

func (w *harnessWorld) eventsJSONLIsCorruptedWithMalformedOrPartialLines() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Clean(w.eventLogPath), os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return fmt.Errorf("failed to open events file for corruption fixture; %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	if _, err := file.WriteString("not-json\n"); err != nil {
		return err
	}
	if _, err := file.WriteString(`{"partial":true`); err != nil {
		return err
	}

	return nil
}

func (w *harnessWorld) eventLogIntegrityValidationExecutes() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle before integrity validation")
	}

	w.eventIntegrityErr = w.lifecycle.ValidateEventLogIntegrity()
	return nil
}

func (w *harnessWorld) integrityValidationFailsForRunRecovery() error {
	if !errors.Is(w.eventIntegrityErr, sigilruntime.ErrIntegrityFailure) {
		return fmt.Errorf("expected integrity failure, got %v", w.eventIntegrityErr)
	}
	return nil
}

func (w *harnessWorld) anAppendAttemptsInPlaceSequenceRewrite() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}
	if len(w.persistedEvents) == 0 {
		return fmt.Errorf("expected existing events before immutable append request")
	}

	eventID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	lastEvent := w.persistedEvents[len(w.persistedEvents)-1]
	w.eventAppendErr = w.lifecycle.EventStore().Append(sigilruntime.EventEnvelope{
		EventID:       eventID.String(),
		SchemaVersion: sigilruntime.SchemaVersionV1,
		RunID:         w.lifecycle.RunID(),
		Seq:           lastEvent.Seq,
		Timestamp:     time.Now().UTC(),
		Type:          sigilruntime.EventTypeRunRunning,
		CausationID:   lastEvent.EventID,
		CorrelationID: w.lifecycle.RunID(),
		Payload: sigilruntime.RunRunningPayload{
			Executor: "rlm",
			MaxDepth: 0,
		},
	})

	return nil
}

func (w *harnessWorld) eventAppendIsRejectedByImmutableEventStoreContract() error {
	if !errors.Is(w.eventAppendErr, sigilruntime.ErrImmutableEventLog) {
		return fmt.Errorf("expected immutable event log append rejection, got %v", w.eventAppendErr)
	}
	return nil
}

func (w *harnessWorld) eventEnvelopesAreInspected() error {
	return w.capturePersistedEvents()
}

func (w *harnessWorld) schemaVersionExistsAndEqualsV1() error {
	if len(w.persistedEvents) == 0 {
		if err := w.capturePersistedEvents(); err != nil {
			return err
		}
	}

	for _, event := range w.persistedEvents {
		if event.SchemaVersion != sigilruntime.SchemaVersionV1 {
			return fmt.Errorf("expected schema_version v1, got %q", event.SchemaVersion)
		}
	}

	return nil
}

func (w *harnessWorld) canonicalV1RunEventValidationRules() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) canonicalCoreLifecycleEventTypesAreValidated() error {
	w.eventValidationErr = nil
	if err := validateCanonicalEventTypeFixtures(); err != nil {
		w.eventValidationErr = err
	}
	return nil
}

func (w *harnessWorld) onlyCanonicalV1LifecycleEventTypesAreAccepted() error {
	if w.eventValidationErr != nil {
		return w.eventValidationErr
	}

	eventID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	runID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	raw, err := marshalEnvelope(map[string]any{
		"event_id":       eventID.String(),
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID.String(),
		"seq":            1,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           "run.unknown",
		"causation_id":   eventID.String(),
		"correlation_id": runID.String(),
		"payload": map[string]any{
			"source": string(sigilruntime.RunQueuedSourceInternalResume),
		},
	})
	if err != nil {
		return err
	}

	if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, sigilruntime.ErrUnknownEventType) {
		return fmt.Errorf("expected unknown event type rejection, got %v", err)
	}

	return nil
}

func (w *harnessWorld) canonicalV1LifecycleEventsWithPayloads() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) strictPayloadSchemaValidationIsExecuted() error {
	w.eventValidationErr = nil

	invalidPayloads := []map[string]any{
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         mustUUIDv7StringOrPanic(),
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunRunning,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": mustUUIDv7StringOrPanic(),
			"payload": map[string]any{
				"executor":  "rlm",
				"max_depth": -1,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         mustUUIDv7StringOrPanic(),
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeStarted,
			"node_id":        mustUUIDv7StringOrPanic(),
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": mustUUIDv7StringOrPanic(),
			"payload": map[string]any{
				"depth":          0,
				"parent_node_id": nil,
				"role":           "recursive_subcall",
				"attempt":        1,
			},
		},
	}

	for _, envelope := range invalidPayloads {
		runID := envelope["run_id"].(string)
		envelope["correlation_id"] = runID
		raw, err := marshalEnvelope(envelope)
		if err != nil {
			w.eventValidationErr = err
			return nil
		}

		if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); err == nil {
			w.eventValidationErr = fmt.Errorf("expected strict payload validation failure for envelope: %+v", envelope)
			return nil
		}
	}

	return nil
}

func (w *harnessWorld) requiredFieldsTypesAndInvariantsAreEnforcedPerEventType() error {
	if w.eventValidationErr != nil {
		return w.eventValidationErr
	}
	return nil
}

func (w *harnessWorld) v1EventEnvelopesWithUnknownFieldsOrUnknownType() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) strictV1ExtensibilityValidationIsExecuted() error {
	w.eventValidationErr = nil

	runID := mustUUIDv7StringOrPanic()
	eventID := mustUUIDv7StringOrPanic()

	cases := []struct {
		envelope map[string]any
		wantErr  error
	}{
		{
			envelope: map[string]any{
				"event_id":       eventID,
				"schema_version": sigilruntime.SchemaVersionV1,
				"run_id":         runID,
				"seq":            1,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           sigilruntime.EventTypeRunQueued,
				"causation_id":   eventID,
				"correlation_id": runID,
				"payload": map[string]any{
					"source": string(sigilruntime.RunQueuedSourceInternalResume),
				},
				"unexpected": true,
			},
			wantErr: sigilruntime.ErrUnknownEnvelopeField,
		},
		{
			envelope: map[string]any{
				"event_id":       eventID,
				"schema_version": sigilruntime.SchemaVersionV1,
				"run_id":         runID,
				"seq":            1,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           sigilruntime.EventTypeRunQueued,
				"causation_id":   eventID,
				"correlation_id": runID,
				"payload": map[string]any{
					"source":  string(sigilruntime.RunQueuedSourceInternalResume),
					"unknown": "value",
				},
			},
			wantErr: sigilruntime.ErrUnknownPayloadField,
		},
		{
			envelope: map[string]any{
				"event_id":       eventID,
				"schema_version": sigilruntime.SchemaVersionV1,
				"run_id":         runID,
				"seq":            1,
				"ts":             time.Now().UTC().Format(time.RFC3339Nano),
				"type":           "node.tool.exec",
				"causation_id":   eventID,
				"correlation_id": runID,
				"payload": map[string]any{
					"source": string(sigilruntime.RunQueuedSourceInternalResume),
				},
			},
			wantErr: sigilruntime.ErrUnknownEventType,
		},
	}

	for _, testCase := range cases {
		raw, err := marshalEnvelope(testCase.envelope)
		if err != nil {
			w.eventValidationErr = err
			return nil
		}

		if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, testCase.wantErr) {
			w.eventValidationErr = fmt.Errorf("expected %v, got %v", testCase.wantErr, err)
			return nil
		}
	}

	return nil
}

func (w *harnessWorld) validationFailsAndEventsAreRejected() error {
	if w.eventValidationErr != nil {
		return w.eventValidationErr
	}
	return nil
}

func (w *harnessWorld) aCoreLifecycleEventPayloadIncludesDeferredNonCoreFields() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) coreV1PayloadValidationExecutes() error {
	runID := mustUUIDv7StringOrPanic()
	eventID := mustUUIDv7StringOrPanic()

	raw, err := marshalEnvelope(map[string]any{
		"event_id":       eventID,
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           sigilruntime.EventTypeRunCompleted,
		"causation_id":   mustUUIDv7StringOrPanic(),
		"correlation_id": runID,
		"payload": map[string]any{
			"status":         "completed",
			"duration_ms":    0,
			"accounting":     acceptanceAccountingRollup("openai", "gpt-5.1"),
			"accounting_ref": "run-artifact://run/accounting.json",
			"tool_trace": map[string]any{
				"name": "deferred",
			},
		},
	})
	if err != nil {
		w.eventValidationErr = err
		return nil
	}

	if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, sigilruntime.ErrUnknownPayloadField) {
		w.eventValidationErr = fmt.Errorf("expected deferred payload field rejection, got %v", err)
		return nil
	}

	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) deferredNonCoreFieldsAreRejectedAsOutOfContract() error {
	if w.eventValidationErr != nil {
		return w.eventValidationErr
	}
	return nil
}

func (w *harnessWorld) eventTypeValidationIncludesNodeStepTrackingEvents() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) nodeStepStartedAndNodeStepCompletedAreAcceptedCanonicalEventTypes() error {
	if err := validateCanonicalEventTypeFixtures(); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) canonicalNodeStepEventsWithPayloads() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) requiredNodeStepFieldsAndDecisionActionInvariantsAreEnforced() error {
	runID := mustUUIDv7StringOrPanic()
	nodeID := mustUUIDv7StringOrPanic()
	stepID := mustUUIDv7StringOrPanic()
	raw, err := marshalEnvelope(map[string]any{
		"event_id":       mustUUIDv7StringOrPanic(),
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           sigilruntime.EventTypeNodeStepCompleted,
		"node_id":        nodeID,
		"causation_id":   mustUUIDv7StringOrPanic(),
		"correlation_id": runID,
		"payload": map[string]any{
			"step_id":        stepID,
			"decision":       "continue",
			"action_count":   0,
			"duration_ms":    1,
			"accounting":     acceptanceAccountingRollup("openai", "gpt-5.1"),
			"accounting_ref": "run-artifact://node/" + nodeID + "/step/" + stepID + "/accounting.json",
		},
	})
	if err != nil {
		return err
	}
	if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, sigilruntime.ErrInvalidEvent) {
		return fmt.Errorf("expected invalid node.step.completed invariants rejection, got %v", err)
	}
	return nil
}

func (w *harnessWorld) eventTypeValidationIncludesNodeTurnEvents() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) nodeTurnUserAndNodeTurnModelAreAcceptedCanonicalEventTypes() error {
	if err := validateCanonicalEventTypeFixtures(); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) canonicalNodeTurnEventsWithPayloads() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) requiredNodeTurnFieldsAreEnforcedAndRoleValuesMatchEventTypeSemantics() error {
	runID := mustUUIDv7StringOrPanic()
	nodeID := mustUUIDv7StringOrPanic()
	stepID := mustUUIDv7StringOrPanic()
	raw, err := marshalEnvelope(map[string]any{
		"event_id":       mustUUIDv7StringOrPanic(),
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           sigilruntime.EventTypeNodeTurnUser,
		"node_id":        nodeID,
		"causation_id":   mustUUIDv7StringOrPanic(),
		"correlation_id": runID,
		"payload": map[string]any{
			"step_id":     stepID,
			"role":        "model",
			"content_ref": "run-artifact://node/turn/1",
		},
	})
	if err != nil {
		return err
	}
	if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, sigilruntime.ErrInvalidEvent) {
		return fmt.Errorf("expected node.turn role mismatch rejection, got %v", err)
	}
	return nil
}

func (w *harnessWorld) canonicalNodeActionExecutionEventsWithPayloads() error {
	w.eventValidationErr = nil
	return nil
}

func (w *harnessWorld) nodeActionExecutedPayloadEnforcesSingleActionContinueInvariants() error {
	runID := mustUUIDv7StringOrPanic()
	nodeID := mustUUIDv7StringOrPanic()
	stepID := mustUUIDv7StringOrPanic()
	actionRef, err := sigilruntime.BuildActionArtifactRef(nodeID, stepID, 2)
	if err != nil {
		return err
	}
	raw, err := marshalEnvelope(map[string]any{
		"event_id":       mustUUIDv7StringOrPanic(),
		"schema_version": sigilruntime.SchemaVersionV1,
		"run_id":         runID,
		"seq":            2,
		"ts":             time.Now().UTC().Format(time.RFC3339Nano),
		"type":           sigilruntime.EventTypeNodeActionExecuted,
		"node_id":        nodeID,
		"causation_id":   mustUUIDv7StringOrPanic(),
		"correlation_id": runID,
		"payload": map[string]any{
			"step_id":      stepID,
			"action_index": 2,
			"action_type":  "repl_code",
			"language":     "go",
			"status":       "completed",
			"duration_ms":  1,
			"action_ref":   actionRef,
		},
	})
	if err != nil {
		return err
	}
	if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); !errors.Is(err, sigilruntime.ErrInvalidEvent) {
		return fmt.Errorf("expected node.action.executed single-action invariant rejection, got %v", err)
	}
	return nil
}

func (w *harnessWorld) effectiveRunLLMProviderIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Provider != expected {
		return fmt.Errorf("expected llm.provider %q, got %q", expected, cfg.LLM.Provider)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMModelIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Model != expected {
		return fmt.Errorf("expected llm.model %q, got %q", expected, cfg.LLM.Model)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMGatewayIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Gateway != expected {
		return fmt.Errorf("expected llm.gateway %q, got %q", expected, cfg.LLM.Gateway)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterBaseURLIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.BaseURL != expected {
		return fmt.Errorf("expected llm.openrouter.base_url %q, got %q", expected, cfg.LLM.OpenRouter.BaseURL)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterRequestTimeoutMSIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.RequestTimeoutMS != expected {
		return fmt.Errorf("expected llm.openrouter.request_timeout_ms %d, got %d", expected, cfg.LLM.OpenRouter.RequestTimeoutMS)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMOpenRouterAPIKeyEnvIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.OpenRouter.APIKeyEnv != expected {
		return fmt.Errorf("expected llm.openrouter.api_key_env %q, got %q", expected, cfg.LLM.OpenRouter.APIKeyEnv)
	}

	return nil
}

func (w *harnessWorld) effectiveRunRLMEnabledIs(expectedRaw string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	expected, err := strconv.ParseBool(expectedRaw)
	if err != nil {
		return fmt.Errorf("failed to parse bool %q; %w", expectedRaw, err)
	}

	if cfg.RLM.Enabled != expected {
		return fmt.Errorf("expected rlm.enabled %t, got %t", expected, cfg.RLM.Enabled)
	}

	return nil
}

func (w *harnessWorld) effectiveRunRLMMaxDepthIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.RLM.MaxDepth != expected {
		return fmt.Errorf("expected rlm.max_depth %d, got %d", expected, cfg.RLM.MaxDepth)
	}

	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxStepsPerNodeIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxStepsPerNode != expected {
		return fmt.Errorf("expected guardrails.max_steps_per_node %d, got %d", expected, cfg.Guardrails.MaxStepsPerNode)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxTotalStepsPerRunIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxTotalStepsPerRun != expected {
		return fmt.Errorf("expected guardrails.max_total_steps_per_run %d, got %d", expected, cfg.Guardrails.MaxTotalStepsPerRun)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxRunDurationMSIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxRunDurationMS != expected {
		return fmt.Errorf("expected guardrails.max_run_duration_ms %d, got %d", expected, cfg.Guardrails.MaxRunDurationMS)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxConsecutiveStepFailuresIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxConsecutiveStepFailures != expected {
		return fmt.Errorf("expected guardrails.max_consecutive_step_failures %d, got %d", expected, cfg.Guardrails.MaxConsecutiveStepFailures)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxTotalTokensIs(expected int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxTotalTokens == nil || *cfg.Guardrails.MaxTotalTokens != int64(expected) {
		return fmt.Errorf("expected guardrails.max_total_tokens %d, got %+v", expected, cfg.Guardrails.MaxTotalTokens)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxTotalTokensIsUnset() error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxTotalTokens != nil {
		return fmt.Errorf("expected guardrails.max_total_tokens to be unset, got %d", *cfg.Guardrails.MaxTotalTokens)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxTotalCostUSDIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxTotalCostUSD == nil || *cfg.Guardrails.MaxTotalCostUSD != expected {
		return fmt.Errorf("expected guardrails.max_total_cost_usd %q, got %+v", expected, cfg.Guardrails.MaxTotalCostUSD)
	}
	return nil
}

func (w *harnessWorld) effectiveRunGuardrailsMaxTotalCostUSDIsUnset() error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Guardrails.MaxTotalCostUSD != nil {
		return fmt.Errorf("expected guardrails.max_total_cost_usd to be unset, got %q", *cfg.Guardrails.MaxTotalCostUSD)
	}
	return nil
}

func (w *harnessWorld) effectiveRunAccountingPricingVersionIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	if cfg.Accounting.PricingVersion != expected {
		return fmt.Errorf("expected accounting.pricing_version %q, got %q", expected, cfg.Accounting.PricingVersion)
	}
	return nil
}

func (w *harnessWorld) effectiveRunAccountingFallbackPricingForProviderModelUsesRates(provider string, model string, expectedInput int, expectedOutput int, expectedReasoning int) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}
	pricing, ok := cfg.Accounting.PricingFor(provider, model)
	if !ok {
		return fmt.Errorf("expected fallback pricing for provider %q model %q", provider, model)
	}
	if pricing.InputMicrousdPerMillionTokens != int64(expectedInput) {
		return fmt.Errorf("expected input rate %d, got %d", expectedInput, pricing.InputMicrousdPerMillionTokens)
	}
	if pricing.OutputMicrousdPerMillionTokens != int64(expectedOutput) {
		return fmt.Errorf("expected output rate %d, got %d", expectedOutput, pricing.OutputMicrousdPerMillionTokens)
	}
	if pricing.ReasoningMicrousdPerMillionTokens == nil || *pricing.ReasoningMicrousdPerMillionTokens != int64(expectedReasoning) {
		return fmt.Errorf("expected reasoning rate %d, got %v", expectedReasoning, pricing.ReasoningMicrousdPerMillionTokens)
	}
	return nil
}

func (w *harnessWorld) effectiveRunLLMReasoningEnabledIs(expectedRaw string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	expected, err := strconv.ParseBool(expectedRaw)
	if err != nil {
		return fmt.Errorf("failed to parse bool %q; %w", expectedRaw, err)
	}

	if cfg.LLM.Reasoning.Enabled != expected {
		return fmt.Errorf("expected llm.reasoning.enabled %t, got %t", expected, cfg.LLM.Reasoning.Enabled)
	}

	return nil
}

func (w *harnessWorld) effectiveRunLLMReasoningEffortIs(expected string) error {
	cfg, err := w.activeRunConfig()
	if err != nil {
		return err
	}

	if cfg.LLM.Reasoning.Effort != expected {
		return fmt.Errorf("expected llm.reasoning.effort %q, got %q", expected, cfg.LLM.Reasoning.Effort)
	}

	return nil
}

func (w *harnessWorld) activeRunConfig() (config.RunConfig, error) {
	if w.runConfigInitErr != nil {
		return config.RunConfig{}, fmt.Errorf("run configuration initialization failed: %w", w.runConfigInitErr)
	}

	return config.GetRun()
}

func (w *harnessWorld) applicationConfigurationInitializationFails() error {
	if w.configInitErr == nil {
		return fmt.Errorf("expected config initialization failure")
	}

	return nil
}

func (w *harnessWorld) applicationLoggingInitializationFails() error {
	if w.loggingInitErr == nil {
		return fmt.Errorf("expected application logging initialization failure")
	}

	return nil
}

func (w *harnessWorld) runConfigurationInitializationSucceeds() error {
	if w.runConfigInitErr != nil {
		return fmt.Errorf("expected run config initialization success, got: %w", w.runConfigInitErr)
	}

	if _, err := config.GetRun(); err != nil {
		return fmt.Errorf("expected active run config, got: %w", err)
	}

	return nil
}

func (w *harnessWorld) runConfigurationInitializationFails() error {
	if w.runConfigInitErr == nil {
		return fmt.Errorf("expected run config initialization failure")
	}

	return nil
}

func (w *harnessWorld) theSigilExecutableIsAvailable() error {
	rootCmd := cmd.NewRootCmd()
	if rootCmd.Use != "sigil" {
		return fmt.Errorf("expected root use sigil, got %q", rootCmd.Use)
	}

	return nil
}

func (w *harnessWorld) aUserRuns(commandLine string) error {
	tokens := strings.Fields(commandLine)
	if len(tokens) == 0 {
		return fmt.Errorf("command line is empty")
	}
	if tokens[0] != "sigil" {
		return fmt.Errorf("expected command to start with sigil, got %q", tokens[0])
	}

	return w.executeSigilArgs(tokens[1:])
}

func (w *harnessWorld) executeSigilArgs(args []string) error {
	if isRunStartArgs(args) {
		if err := w.ensureRunStartMockGateway(); err != nil {
			return err
		}
	}

	rootCmd := cmd.NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(args)

	w.lastErr = rootCmd.Execute()
	w.lastStdout = stdout.String()
	w.lastStderr = stderr.String()
	if w.lastErr != nil {
		w.lastExitCode = 1
		return nil
	}

	w.lastExitCode = 0
	return nil
}

func isRunStartArgs(args []string) bool {
	if len(args) == 0 || args[0] != "run" {
		return false
	}

	for _, arg := range args[1:] {
		if arg == "start" {
			return true
		}
	}

	return false
}

func (w *harnessWorld) ensureRunStartMockGateway() error {
	if w.inferenceMockServer == nil {
		w.inferenceMockServer = newOpenRouterMockServer()
		w.inferenceMockServer.SetResponses(mockGatewayResponse{
			statusCode: 200,
			body:       validFinalGatewayResponseBody(),
		})
	}

	if err := osSetEnv("OPENROUTER_API_KEY", "test-openrouter-key"); err != nil {
		return err
	}
	if err := osSetEnv("SIGIL_RUN_LLM_OPENROUTER_BASE_URL", w.inferenceMockServer.URL()); err != nil {
		return err
	}
	if err := osSetEnv("SIGIL_RUN_LLM_OPENROUTER_API_KEY_ENV", "OPENROUTER_API_KEY"); err != nil {
		return err
	}

	return nil
}

func (w *harnessWorld) commandExitsWithStatusCode(expectedCode int) error {
	if w.lastExitCode != expectedCode {
		return fmt.Errorf("expected exit code %d, got %d", expectedCode, w.lastExitCode)
	}

	return nil
}

func (w *harnessWorld) commandExitsNonZero() error {
	if w.lastExitCode == 0 {
		return fmt.Errorf("expected non-zero exit code")
	}

	return nil
}

func (w *harnessWorld) commandOutputContains(expectedText string) error {
	combined := w.lastStdout + "\n" + w.lastStderr
	if !strings.Contains(combined, expectedText) {
		return fmt.Errorf("expected command output to contain %q, got %q", expectedText, combined)
	}

	return nil
}

func (w *harnessWorld) commandErrorContains(expectedText string) error {
	combined := w.lastStderr
	if w.lastErr != nil {
		combined += "\n" + w.lastErr.Error()
	}

	if !strings.Contains(combined, expectedText) {
		return fmt.Errorf("expected command error to contain %q, got %q", expectedText, combined)
	}

	return nil
}

func (w *harnessWorld) rootUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil") {
		return fmt.Errorf("expected root usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) runUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil run") {
		return fmt.Errorf("expected run usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) stopUsageHelpIsPrinted() error {
	if !strings.Contains(w.lastStdout, "Usage:") || !strings.Contains(w.lastStdout, "sigil run stop") {
		return fmt.Errorf("expected stop usage in stdout, got %q", w.lastStdout)
	}

	return nil
}

func (w *harnessWorld) aLocalCLIRunIsActivelyExecuting() error {
	return w.startRunControlHelperWithRequester("active_interrupt", sigilruntime.StopRequesterCLIRunStop)
}

func (w *harnessWorld) aUserRunsSigilRunStopForTheActiveRun() error {
	if strings.TrimSpace(w.activeRunID) == "" {
		return fmt.Errorf("expected active run id before executing stop")
	}

	invocation, err := w.executeStopCommandForRun(w.activeRunID, "active", "text")
	if err != nil {
		return err
	}
	w.activeStopInvocation = invocation
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunStopJSONForTheActiveRun() error {
	if strings.TrimSpace(w.activeRunID) == "" {
		return fmt.Errorf("expected active run id before executing stop")
	}

	invocation, err := w.executeStopCommandForRun(w.activeRunID, "active-json", "json")
	if err != nil {
		return err
	}
	w.activeStopInvocation = invocation
	return nil
}

func (w *harnessWorld) theActiveRunTransitionsTo(expectedStateRaw string) error {
	expectedState, err := parseRunState(expectedStateRaw)
	if err != nil {
		return err
	}
	if w.activeStopInvocation.Result != nil && w.activeStopInvocation.Result.State != string(expectedState) {
		return fmt.Errorf("expected stop result state %q, got %q", expectedState, w.activeStopInvocation.Result.State)
	}

	status, err := sigilruntime.ResolveRunStatus(sigilruntime.DefaultRunsBaseDir, w.activeRunID)
	if err != nil {
		return fmt.Errorf("failed to resolve active run status; %w", err)
	}
	if status.State != expectedState {
		return fmt.Errorf("expected resolved run state %q, got %q", expectedState, status.State)
	}
	w.activeRunEventsPath = status.EventsPath
	return nil
}

func (w *harnessWorld) stdoutContainsOneJSONStopResultWithRunIDStopRequestedStateAndEventsPath() error {
	if w.activeStopInvocation.ExitCode != 0 {
		return fmt.Errorf("expected successful stop command, got exit code %d", w.activeStopInvocation.ExitCode)
	}
	if w.activeStopInvocation.Result == nil {
		return fmt.Errorf("expected parsed stop result, got stdout %q", w.activeStopInvocation.Stdout)
	}
	result := w.activeStopInvocation.Result
	if result.RunID != w.activeRunID {
		return fmt.Errorf("expected run_id %q, got %q", w.activeRunID, result.RunID)
	}
	if !result.StopRequested {
		return fmt.Errorf("expected stop_requested=true, got false")
	}
	if strings.TrimSpace(result.State) == "" {
		return fmt.Errorf("expected non-empty state in stop result")
	}
	if strings.TrimSpace(result.EventsPath) == "" {
		return fmt.Errorf("expected non-empty events_path in stop result")
	}
	expectedSuffix := filepath.ToSlash(filepath.Join(".sigil", "runs", w.activeRunID, "events.jsonl"))
	if !strings.HasSuffix(filepath.ToSlash(result.EventsPath), expectedSuffix) {
		return fmt.Errorf("expected events_path for active run, got %q", result.EventsPath)
	}
	return nil
}

func (w *harnessWorld) theRunLifecycleAndStopRequestMetadataAreInspected() error {
	if strings.TrimSpace(w.activeRunID) == "" {
		return fmt.Errorf("expected active run before metadata inspection")
	}

	metadata, err := sigilruntime.ReadProcessMetadata(sigilruntime.DefaultRunsBaseDir, w.activeRunID)
	if err != nil {
		return fmt.Errorf("expected process metadata for active run; %w", err)
	}
	w.activeProcessMetadata = metadata
	w.activeProcessSeen = true

	invocation, err := w.executeStopCommandForRun(w.activeRunID, "metadata-inspection", "text")
	if err != nil {
		return err
	}
	w.activeStopInvocation = invocation
	return nil
}

func (w *harnessWorld) processJSONExistsForTheActiveRun() error {
	if !w.activeProcessSeen {
		return fmt.Errorf("expected process metadata to be captured before assertion")
	}

	processPath, err := sigilruntime.ProcessMetadataPath(sigilruntime.DefaultRunsBaseDir, w.activeRunID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(w.activeProcessMetadata.RunID) != w.activeRunID {
		return fmt.Errorf("expected process metadata run_id %q, got %q", w.activeRunID, w.activeProcessMetadata.RunID)
	}
	if w.activeProcessMetadata.PID < 1 {
		return fmt.Errorf("expected active process pid, got %d", w.activeProcessMetadata.PID)
	}
	if w.activeProcessMetadata.StartedAt.IsZero() {
		return fmt.Errorf("expected active process started_at, got zero value")
	}
	if _, err := os.Stat(filepath.Clean(processPath)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("expected process metadata path to be readable; %w", err)
	}
	return nil
}

func (w *harnessWorld) stopRequestJSONIsWrittenBeforeSIGTERMIsIssued() error {
	request, ok, err := sigilruntime.ReadStopRequestMetadata(sigilruntime.DefaultRunsBaseDir, w.activeRunID)
	if err != nil {
		return fmt.Errorf("failed to read stop request metadata; %w", err)
	}
	if !ok {
		return fmt.Errorf("expected stop-request metadata for run %q", w.activeRunID)
	}
	if request.RunID != w.activeRunID {
		return fmt.Errorf("expected stop request run_id %q, got %q", w.activeRunID, request.RunID)
	}
	if request.RequestedBy != sigilruntime.StopRequesterCLIRunStop {
		return fmt.Errorf("expected requested_by %q, got %q", sigilruntime.StopRequesterCLIRunStop, request.RequestedBy)
	}
	if request.Signal != sigilruntime.StopSignalSIGTERM {
		return fmt.Errorf("expected signal %q, got %q", sigilruntime.StopSignalSIGTERM, request.Signal)
	}
	if w.helperCmd == nil || w.helperCmd.ProcessState == nil || !w.helperCmd.ProcessState.Success() {
		return fmt.Errorf("expected helper process to confirm stop-request ordering, got stderr %q", w.helperStderr.String())
	}
	return nil
}

func (w *harnessWorld) localCLICompletedFailedAndInterruptedRunsExist() error {
	states := []sigilruntime.RunState{
		sigilruntime.RunStateCompleted,
		sigilruntime.RunStateFailed,
		sigilruntime.RunStateInterrupted,
	}
	w.terminalRunIDs = make([]string, 0, len(states))
	for _, state := range states {
		runID, err := w.createTerminalRun(state)
		if err != nil {
			return err
		}
		w.terminalRunIDs = append(w.terminalRunIDs, runID)
	}
	return nil
}

func (w *harnessWorld) terminalStopCommandsAreExecutedForThoseRuns() error {
	w.terminalStopInvocations = w.terminalStopInvocations[:0]
	for _, runID := range w.terminalRunIDs {
		invocation, err := w.executeStopCommandForRun(runID, "terminal-"+runID, "json")
		if err != nil {
			return err
		}
		w.terminalStopInvocations = append(w.terminalStopInvocations, invocation)
	}
	return nil
}

func (w *harnessWorld) eachTerminalStopCommandExitsWithStatusCode0AndReturnsStopRequestedFalse() error {
	if len(w.terminalStopInvocations) != 3 {
		return fmt.Errorf("expected three terminal stop invocations, got %d", len(w.terminalStopInvocations))
	}
	for _, invocation := range w.terminalStopInvocations {
		if invocation.ExitCode != 0 {
			return fmt.Errorf("expected exit code 0 for %s, got %d (stderr=%q)", invocation.Name, invocation.ExitCode, invocation.Stderr)
		}
		if invocation.Result == nil {
			return fmt.Errorf("expected parsed stop result for %s", invocation.Name)
		}
		if invocation.Result.StopRequested {
			return fmt.Errorf("expected stop_requested=false for %s", invocation.Name)
		}
	}
	return nil
}

func (w *harnessWorld) stopRequestsLoseTheRaceToCompletedAndFailedLocalCLIRuns() error {
	w.racingStopInvocations = nil
	return nil
}

func (w *harnessWorld) stopCommandsAreExecutedForThoseRacingRuns() error {
	testCases := []struct {
		name string
		mode string
	}{
		{name: "completed", mode: "complete_on_sigterm"},
		{name: "failed", mode: "fail_on_sigterm"},
	}
	w.racingStopInvocations = make([]stopInvocation, 0, len(testCases))
	for _, testCase := range testCases {
		if err := w.startRunControlHelper(testCase.mode); err != nil {
			return err
		}
		invocation, err := w.executeStopCommandForRun(w.activeRunID, testCase.name, "json")
		if err != nil {
			return err
		}
		w.racingStopInvocations = append(w.racingStopInvocations, invocation)
	}
	return nil
}

func (w *harnessWorld) theJSONStopResultsContainStopRequestedTrueAndTheObservedTerminalStates() error {
	if len(w.racingStopInvocations) != 2 {
		return fmt.Errorf("expected two racing stop invocations, got %d", len(w.racingStopInvocations))
	}
	expectedStates := map[string]string{
		"completed": "completed",
		"failed":    "failed",
	}
	for _, invocation := range w.racingStopInvocations {
		if invocation.ExitCode != 0 {
			return fmt.Errorf("expected exit code 0 for %s, got %d", invocation.Name, invocation.ExitCode)
		}
		if invocation.Result == nil {
			return fmt.Errorf("expected parsed stop result for %s", invocation.Name)
		}
		if !invocation.Result.StopRequested {
			return fmt.Errorf("expected stop_requested=true for %s", invocation.Name)
		}
		if invocation.Result.State != expectedStates[invocation.Name] {
			return fmt.Errorf("expected observed state %q for %s, got %q", expectedStates[invocation.Name], invocation.Name, invocation.Result.State)
		}
	}
	return nil
}

func (w *harnessWorld) sigilRunStopTargetsUnknownCorruptStaleAndMissingProcessRunState() error {
	w.invalidCaseRunIDs = make(map[string]string, 4)

	unknownID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	w.invalidCaseRunIDs["unknown"] = unknownID.String()

	corruptID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	corruptRunDir := filepath.Join(sigilruntime.DefaultRunsBaseDir, corruptID.String())
	if err := os.MkdirAll(filepath.Clean(corruptRunDir), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(corruptRunDir, "events.jsonl"), []byte("{not-json}\n"), 0o644); err != nil {
		return err
	}
	w.invalidCaseRunIDs["corrupt"] = corruptID.String()

	staleLifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir:  sigilruntime.DefaultRunsBaseDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     1,
	})
	if err != nil {
		return err
	}
	w.invalidCaseRunIDs["stale-process"] = staleLifecycle.RunID()
	if err := staleLifecycle.StartExecution(); err != nil {
		_ = staleLifecycle.Close()
		return err
	}
	staleProcess := exec.Command("sh", "-c", "sleep 30")
	if err := staleProcess.Start(); err != nil {
		_ = staleLifecycle.Close()
		return err
	}
	w.invalidProcessCmd = staleProcess
	if err := sigilruntime.WriteProcessMetadata(sigilruntime.DefaultRunsBaseDir, sigilruntime.ProcessMetadata{
		RunID:      staleLifecycle.RunID(),
		PID:        staleProcess.Process.Pid,
		RecordedAt: time.Now().UTC(),
		StartedAt:  time.Unix(1_700_000_000, 0).UTC(),
		Source:     sigilruntime.RunSourceCLIRunStart,
	}); err != nil {
		_ = staleLifecycle.Close()
		return err
	}
	if err := staleLifecycle.Close(); err != nil {
		return err
	}

	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir:  sigilruntime.DefaultRunsBaseDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     1,
	})
	if err != nil {
		return err
	}
	w.invalidCaseRunIDs["missing-process"] = lifecycle.RunID()
	if err := lifecycle.StartExecution(); err != nil {
		_ = lifecycle.Close()
		return err
	}
	if err := lifecycle.Close(); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) stopCommandsAreExecutedForThoseInvalidControlCases() error {
	order := []string{"unknown", "corrupt", "stale-process", "missing-process"}
	w.invalidStopInvocations = make([]stopInvocation, 0, len(order))
	for _, name := range order {
		runID := w.invalidCaseRunIDs[name]
		invocation, err := w.executeStopCommandForRun(runID, name, "text")
		if err != nil {
			return err
		}
		w.invalidStopInvocations = append(w.invalidStopInvocations, invocation)
	}
	return nil
}

func (w *harnessWorld) eachInvalidControlCaseExitsNonZero() error {
	if len(w.invalidStopInvocations) != 4 {
		return fmt.Errorf("expected four invalid control invocations, got %d", len(w.invalidStopInvocations))
	}
	for _, invocation := range w.invalidStopInvocations {
		if invocation.ExitCode == 0 {
			return fmt.Errorf("expected non-zero exit for %s", invocation.Name)
		}
		if invocation.Err == nil {
			return fmt.Errorf("expected command error for %s", invocation.Name)
		}
	}
	return nil
}

func (w *harnessWorld) aLocalCLIRunHasPersistedRunQueuedButNotRunRunning() error {
	return w.startRunControlHelperWithRequester("queued_interrupt", sigilruntime.StopRequesterCLIRunStop)
}

func (w *harnessWorld) runInterruptedContainsReasonUserRequestInterruptedByCLIRunStopAndPartialAccounting() error {
	return w.runInterruptedContainsReasonUserRequestInterruptedByAndPartialAccounting(sigilruntime.StopRequesterCLIRunStop)
}

func (w *harnessWorld) runInterruptedContainsReasonUserRequestInterruptedByAndPartialAccounting(expectedInterruptedBy string) error {
	eventsPath := w.activeRunEventsPath
	if w.activeStopInvocation.Result != nil && strings.TrimSpace(w.activeStopInvocation.Result.EventsPath) != "" {
		eventsPath = w.activeStopInvocation.Result.EventsPath
	}
	if strings.TrimSpace(eventsPath) == "" {
		return fmt.Errorf("expected active events path before interruption payload assertion")
	}
	events, err := readEventsFromPath(eventsPath)
	if err != nil {
		return err
	}
	for index := len(events) - 1; index >= 0; index-- {
		event := events[index]
		if event.Type != sigilruntime.EventTypeRunInterrupted {
			continue
		}
		payload, ok := event.Payload.(sigilruntime.RunInterruptedPayload)
		if !ok {
			return fmt.Errorf("expected run.interrupted payload type, got %T", event.Payload)
		}
		if payload.Reason != sigilruntime.RunInterruptedReasonUserRequest {
			return fmt.Errorf("expected interruption reason %q, got %q", sigilruntime.RunInterruptedReasonUserRequest, payload.Reason)
		}
		if payload.InterruptedBy == nil || *payload.InterruptedBy != expectedInterruptedBy {
			return fmt.Errorf("expected interrupted_by=%q, got %+v", expectedInterruptedBy, payload.InterruptedBy)
		}
		if payload.Accounting.TreeTotal.TokenStatus != "partial" && payload.Accounting.TreeTotal.CostStatus != "partial" {
			return fmt.Errorf("expected partial accounting in run.interrupted payload, got %+v", payload.Accounting.TreeTotal)
		}
		if payload.AccountingRef == nil || strings.TrimSpace(*payload.AccountingRef) == "" {
			return fmt.Errorf("expected accounting_ref in run.interrupted payload")
		}
		w.activeRunEventsPath = eventsPath
		return nil
	}
	return fmt.Errorf("expected run.interrupted event in %q", eventsPath)
}

func (w *harnessWorld) interruptedStopHandlingDoesNotAppendSyntheticNodeFailedOrNodeStepCompletedRecords() error {
	eventsPath := w.activeRunEventsPath
	if strings.TrimSpace(eventsPath) == "" {
		return fmt.Errorf("expected active events path before synthetic-event assertion")
	}
	events, err := readEventsFromPath(eventsPath)
	if err != nil {
		return err
	}
	sawActiveWork := false
	for _, event := range events {
		switch event.Type {
		case sigilruntime.EventTypeNodeStepStarted:
			sawActiveWork = true
		case sigilruntime.EventTypeNodeFailed:
			return fmt.Errorf("unexpected synthetic node.failed event after stop request")
		case sigilruntime.EventTypeNodeStepCompleted:
			return fmt.Errorf("unexpected synthetic node.step.completed event after stop request")
		}
	}
	if !sawActiveWork {
		return fmt.Errorf("expected active work evidence via node.step.started before stop")
	}
	return nil
}

func (w *harnessWorld) noDefaultStartConfigFilesExist() error {
	_ = os.Remove(filepath.Clean("./sigil.yaml"))
	_ = os.Remove(filepath.Clean("./sigil-run.yaml"))
	return nil
}

func (w *harnessWorld) noDefaultRunConfigFilesExist() error {
	_ = os.Remove(filepath.Clean(config.DefaultRunConfigPath))
	return nil
}

func (w *harnessWorld) resetLifecycleToState(targetState sigilruntime.RunState) error {
	if w.lifecycle != nil {
		_ = w.lifecycle.Close()
	}

	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir: sigilruntime.DefaultRunsBaseDir,
		MaxDepth:    3,
	})
	if err != nil {
		return err
	}

	switch targetState {
	case sigilruntime.RunStateQueued:
	case sigilruntime.RunStateRunning:
		if err := lifecycle.StartExecution(); err != nil {
			return err
		}
	case sigilruntime.RunStateCompleted:
		if err := lifecycle.StartExecution(); err != nil {
			return err
		}
		if err := lifecycle.Complete(); err != nil {
			return err
		}
	case sigilruntime.RunStateFailed:
		if err := lifecycle.StartExecution(); err != nil {
			return err
		}
		if err := lifecycle.Fail(); err != nil {
			return err
		}
	case sigilruntime.RunStateInterrupted:
		if err := lifecycle.StartExecution(); err != nil {
			return err
		}
		if err := lifecycle.Interrupt(); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unsupported run state %q", targetState)
	}

	w.lifecycle = lifecycle
	w.lifecycleTransitionErr = nil
	w.lastCreatedNode = sigilruntime.Node{}
	w.nodeCountBeforeActivity = 0
	w.nodeCountAfterActivity = 0
	w.eventLogPath = ""
	w.persistedEvents = nil
	w.eventAppendErr = nil
	w.eventValidationErr = nil
	w.eventIntegrityErr = nil
	w.rawEventLines = nil
	return nil
}

func (w *harnessWorld) capturePersistedEvents() error {
	if w.lifecycle == nil {
		return fmt.Errorf("expected lifecycle to be initialized before reading persisted events")
	}

	events, err := w.lifecycle.PersistedEvents()
	if err != nil {
		return err
	}
	w.persistedEvents = events

	logPath, err := w.lifecycle.EventsFilePath()
	if err != nil {
		return err
	}
	w.eventLogPath = logPath
	return nil
}

func (w *harnessWorld) executeStopCommandForRun(runID string, name string, outputFormat string) (stopInvocation, error) {
	commandLine := fmt.Sprintf("sigil run stop %s", runID)
	if strings.TrimSpace(outputFormat) == "json" {
		commandLine = fmt.Sprintf("sigil run stop -o json %s", runID)
	}

	if err := w.aUserRuns(commandLine); err != nil {
		return stopInvocation{}, err
	}

	invocation := stopInvocation{
		Name:     name,
		RunID:    runID,
		ExitCode: w.lastExitCode,
		Stdout:   w.lastStdout,
		Stderr:   w.lastStderr,
		Err:      w.lastErr,
	}
	if w.lastExitCode == 0 && strings.TrimSpace(outputFormat) == "json" {
		result, err := parseAcceptanceStopResult(w.lastStdout)
		if err != nil {
			return stopInvocation{}, err
		}
		invocation.Result = &result
	}
	if w.helperCmd != nil && w.helperCmd.ProcessState == nil {
		if err := w.waitForHelperExit(3 * time.Second); err != nil {
			return stopInvocation{}, err
		}
	}
	if invocation.Result != nil {
		w.activeRunEventsPath = invocation.Result.EventsPath
	}
	return invocation, nil
}

func (w *harnessWorld) cliRunStartMockResponsesAreConfiguredFor(fixture string) error {
	if strings.TrimSpace(fixture) == "" {
		return fmt.Errorf("CLI run start fixture is required")
	}
	if w.inferenceMockServer == nil {
		w.inferenceMockServer = newOpenRouterMockServer()
	}

	responses, err := cliRunStartResponsesForFixture(fixture)
	if err != nil {
		return err
	}
	w.inferenceMockServer.SetResponses(responses...)
	return nil
}

func cliRunStartResponsesForFixture(fixture string) ([]mockGatewayResponse, error) {
	switch fixture {
	case "recursive-progress":
		return []mockGatewayResponse{
			{
				statusCode: 200,
				body: continueGatewayResponseBody(
					`import "fmt"; answer, err := rlm_query("child prompt", "child context"); if err != nil { panic(err) }; fmt.Print(answer)`,
				),
			},
			{statusCode: 200, body: finalGatewayResponseBody("child final")},
			{statusCode: 200, body: finalGatewayResponseBody("root final")},
		}, nil
	case "fallback-progress":
		return []mockGatewayResponse{
			{
				statusCode: 200,
				body: continueGatewayResponseBody(
					`import "fmt"; answer, err := rlm_query("child prompt", "child context"); if err != nil { panic(err) }; fmt.Print(answer)`,
				),
			},
			{statusCode: 200, body: llmAnswerValidGatewayResponseBody()},
			{statusCode: 200, body: finalGatewayResponseBody("root final")},
		}, nil
	default:
		return nil, fmt.Errorf("unsupported CLI run start fixture %q", fixture)
	}
}

func continueGatewayResponseBody(replCode string) map[string]any {
	return map[string]any{
		"id":       "resp_cli_continue",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": fmt.Sprintf(
							`{"decision":"continue","continuation":{"repl_code":%q,"intent":"visualize architecture","expected_observation":"subcall progress is rendered"}}`,
							replCode,
						),
					},
				},
			},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
}

func finalGatewayResponseBody(answer string) map[string]any {
	return map[string]any{
		"id":       "resp_cli_final",
		"status":   "completed",
		"provider": "openai",
		"model":    "gpt-5.1",
		"output": []any{
			map[string]any{
				"content": []any{
					map[string]any{
						"type": "output_text",
						"text": fmt.Sprintf(
							`{"decision":"final","final":{"answer":%q,"evidence":[{"ref":"__context_ref__"}],"confidence":"medium"}}`,
							answer,
						),
					},
				},
			},
		},
		"usage": map[string]any{"input_tokens": 10, "output_tokens": 5, "total_tokens": 15},
	}
}

func parseAcceptanceStopResult(stdout string) (acceptanceStopResult, error) {
	var result acceptanceStopResult
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return acceptanceStopResult{}, fmt.Errorf("expected JSON stop result, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return acceptanceStopResult{}, fmt.Errorf("failed to decode JSON stop result %q; %w", stdout, err)
	}
	return result, nil
}

func (w *harnessWorld) startRunControlHelper(mode string) error {
	return w.startRunControlHelperWithRequester(mode, sigilruntime.StopRequesterCLIRunStop)
}

func (w *harnessWorld) startRunControlHelperWithRequester(mode string, expectedRequestedBy string) error {
	if err := w.stopHelperProcess(); err != nil {
		return err
	}
	if err := w.ensureRunControlHelperDir(); err != nil {
		return err
	}

	existingRunIDs, err := currentRunIDSet()
	if err != nil {
		return err
	}

	w.helperStdout.Reset()
	w.helperStderr.Reset()
	w.activeRunID = ""
	w.activeRunDir = ""
	w.activeRunEventsPath = ""
	w.activeProcessMetadata = sigilruntime.ProcessMetadata{}
	w.activeProcessSeen = false
	w.activeStopInvocation = stopInvocation{}

	cmd := exec.Command(filepath.Join(w.helperDir, "run-stop-helper"))
	cmd.Stdout = &w.helperStdout
	cmd.Stderr = &w.helperStderr
	cmd.Env = append(os.Environ(),
		"SIGIL_ACCEPTANCE_WORKDIR="+w.workingDir,
		"SIGIL_ACCEPTANCE_HELPER_MODE="+mode,
		"SIGIL_ACCEPTANCE_EXPECTED_REQUESTED_BY="+expectedRequestedBy,
	)
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start run-control helper; %w", err)
	}
	w.helperCmd = cmd

	if err := w.waitForHelperRun(mode, existingRunIDs); err != nil {
		return err
	}
	return nil
}

func (w *harnessWorld) ensureRunControlHelperDir() error {
	if strings.TrimSpace(w.helperDir) != "" {
		return nil
	}

	moduleRoot, err := sigilModuleRootDir()
	if err != nil {
		return err
	}
	helperDir, err := os.MkdirTemp(moduleRoot, ".acceptance-run-stop-helper-")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(helperDir, "main.go"), []byte(runControlHelperSource()), 0o644); err != nil {
		return err
	}
	buildCmd := exec.Command("go", "build", "-o", "run-stop-helper", ".")
	buildCmd.Dir = helperDir
	buildOutput, err := buildCmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to build run-stop helper; %w; output=%s", err, string(buildOutput))
	}
	w.helperDir = helperDir
	return nil
}

func sigilModuleRootDir() (string, error) {
	_, file, _, ok := runtimestd.Caller(0)
	if !ok {
		return "", fmt.Errorf("failed to resolve cli_steps.go path")
	}
	return filepath.Dir(filepath.Dir(filepath.Dir(file))), nil
}

func runControlHelperSource() string {
	return `package main

import (
	"fmt"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/runtime"
)

func main() {
	workDir := os.Getenv("SIGIL_ACCEPTANCE_WORKDIR")
	if workDir == "" {
		fatalf("SIGIL_ACCEPTANCE_WORKDIR is required")
	}
	if err := os.Chdir(workDir); err != nil {
		fatalf("failed to change working directory: %v", err)
	}
	mode := os.Getenv("SIGIL_ACCEPTANCE_HELPER_MODE")
	if mode == "" {
		fatalf("SIGIL_ACCEPTANCE_HELPER_MODE is required")
	}
	expectedRequestedBy := os.Getenv("SIGIL_ACCEPTANCE_EXPECTED_REQUESTED_BY")
	if expectedRequestedBy == "" {
		expectedRequestedBy = runtime.StopRequesterCLIRunStop
	}

	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		RunsBaseDir:  runtime.DefaultRunsBaseDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     1,
	})
	if err != nil {
		fatalf("failed to create lifecycle: %v", err)
	}
	defer func() {
		if closeErr := lifecycle.Close(); closeErr != nil {
			fatalf("failed to close lifecycle: %v", closeErr)
		}
	}()

	processMetadata, err := runtime.CurrentProcessMetadata(runtime.RunSourceCLIRunStart)
	if err != nil {
		fatalf("failed to capture process metadata: %v", err)
	}
	processMetadata.RunID = lifecycle.RunID()
	if err := runtime.WriteProcessMetadata(runtime.DefaultRunsBaseDir, processMetadata); err != nil {
		fatalf("failed to write process metadata: %v", err)
	}

	var interruptedNodeID *string
	switch mode {
	case "active_interrupt", "complete_on_sigterm", "fail_on_sigterm":
		if err := lifecycle.StartExecution(); err != nil {
			fatalf("failed to start execution: %v", err)
		}
		rootNode, err := lifecycle.RootNode()
		if err != nil {
			fatalf("failed to resolve root node: %v", err)
		}
		if mode == "active_interrupt" {
			if _, err := lifecycle.AppendNodeStepStarted(rootNode.ID); err != nil {
				fatalf("failed to append node.step.started: %v", err)
			}
			interruptedNodeID = &rootNode.ID
		}
	case "queued_interrupt":
	default:
		fatalf("unsupported helper mode %q", mode)
	}

	signalCh := make(chan os.Signal, 1)
	signal.Notify(signalCh, syscall.SIGTERM)
	defer signal.Stop(signalCh)

	select {
	case <-signalCh:
	case <-time.After(10 * time.Second):
		fatalf("timed out waiting for SIGTERM")
	}

	request, ok, err := runtime.ReadStopRequestMetadata(runtime.DefaultRunsBaseDir, lifecycle.RunID())
	if err != nil {
		fatalf("failed to read stop-request metadata: %v", err)
	}
	if !ok {
		fatalf("stop-request metadata was not present before SIGTERM handling")
	}
	if request.RequestedBy != expectedRequestedBy {
		fatalf("unexpected requested_by %q", request.RequestedBy)
	}
	if request.Signal != runtime.StopSignalSIGTERM {
		fatalf("unexpected signal %q", request.Signal)
	}

	switch mode {
	case "active_interrupt", "queued_interrupt":
		accountingRef := "run-artifact://run/accounting/interrupted.json"
		requestedBy := expectedRequestedBy
		if err := lifecycle.InterruptWith(runtime.RunInterruptedPayload{
			Status:            "interrupted",
			Reason:            runtime.RunInterruptedReasonUserRequest,
			InterruptedBy:     &requestedBy,
			InterruptedNodeID: interruptedNodeID,
			Accounting:        partialRollup(),
			AccountingRef:     &accountingRef,
		}); err != nil {
			fatalf("failed to interrupt run: %v", err)
		}
	case "complete_on_sigterm":
		if err := lifecycle.CompleteWithAccounting(nil, unavailableRollup(), nil); err != nil {
			fatalf("failed to complete run after SIGTERM: %v", err)
		}
	case "fail_on_sigterm":
		if err := lifecycle.FailWith(runtime.RunFailedPayload{
			Status:       "failed",
			ErrorCode:    "helper.race",
			ErrorMessage: "completed failure path before interruption persisted",
			Retryable:    false,
			Accounting:   unavailableRollup(),
		}); err != nil {
			fatalf("failed to fail run after SIGTERM: %v", err)
		}
	}
}

func partialRollup() accounting.Rollup {
	inputTokens := int64(1_000_000)
	modelTotal := accounting.BuildLeafSummary(accounting.LeafInput{
		Provider:       "openai",
		Model:          "gpt-5.1",
		PricingVersion: "acceptance",
		InputTokens:    &inputTokens,
		FallbackPricing: &accounting.FallbackPricing{
			InputMicrousdPerMillionTokens:  100,
			OutputMicrousdPerMillionTokens: 200,
		},
	})
	return accounting.BuildRollup("openai", "gpt-5.1", "acceptance", modelTotal, accounting.ZeroSummary("openai", "gpt-5.1", "acceptance"))
}

func unavailableRollup() accounting.Rollup {
	return accounting.BuildRollup(
		"openai",
		"gpt-5.1",
		"acceptance",
		accounting.UnavailableSummary("openai", "gpt-5.1", "acceptance"),
		accounting.ZeroSummary("openai", "gpt-5.1", "acceptance"),
	)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
`
}

func (w *harnessWorld) waitForHelperRun(mode string, existingRunIDs map[string]struct{}) error {
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		runID, runDir, eventsPath, events, found, err := findNewRun(existingRunIDs)
		if err != nil {
			return err
		}
		if found && helperRunReady(mode, events, filepath.Join(runDir, sigilruntime.ProcessMetadataFileName)) {
			w.activeRunID = runID
			w.activeRunDir = runDir
			w.activeRunEventsPath = eventsPath
			return nil
		}
		if w.helperCmd != nil && w.helperCmd.ProcessState != nil && !w.helperCmd.ProcessState.Success() {
			return fmt.Errorf("helper exited before run became ready; stderr=%q", w.helperStderr.String())
		}
		time.Sleep(25 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for helper run in mode %q; stderr=%q", mode, w.helperStderr.String())
}

func findNewRun(existingRunIDs map[string]struct{}) (string, string, string, []sigilruntime.EventEnvelope, bool, error) {
	matches, err := filepath.Glob(filepath.Join(sigilruntime.DefaultRunsBaseDir, "*", "events.jsonl"))
	if err != nil {
		return "", "", "", nil, false, err
	}
	for _, eventsPath := range matches {
		runDir := filepath.Dir(eventsPath)
		runID := filepath.Base(runDir)
		if _, seen := existingRunIDs[runID]; seen {
			continue
		}
		events, err := readEventsFromPath(eventsPath)
		if err != nil {
			continue
		}
		if len(events) == 0 {
			continue
		}
		return runID, runDir, eventsPath, events, true, nil
	}
	return "", "", "", nil, false, nil
}

func helperRunReady(mode string, events []sigilruntime.EventEnvelope, processPath string) bool {
	if len(events) == 0 {
		return false
	}
	if _, err := os.Stat(filepath.Clean(processPath)); err != nil {
		return false
	}

	hasRunning := false
	hasStepStarted := false
	for _, event := range events {
		switch event.Type {
		case sigilruntime.EventTypeRunRunning:
			hasRunning = true
		case sigilruntime.EventTypeNodeStepStarted:
			hasStepStarted = true
		}
	}

	switch mode {
	case "active_interrupt":
		return hasRunning && hasStepStarted
	case "complete_on_sigterm", "fail_on_sigterm":
		return hasRunning
	case "queued_interrupt":
		return !hasRunning
	default:
		return false
	}
}

func currentRunIDSet() (map[string]struct{}, error) {
	matches, err := filepath.Glob(filepath.Join(sigilruntime.DefaultRunsBaseDir, "*"))
	if err != nil {
		return nil, err
	}
	runIDs := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		runIDs[filepath.Base(match)] = struct{}{}
	}
	return runIDs, nil
}

func (w *harnessWorld) waitForHelperExit(timeout time.Duration) error {
	if w.helperCmd == nil || w.helperCmd.ProcessState != nil {
		return nil
	}

	done := make(chan error, 1)
	go func() {
		done <- w.helperCmd.Wait()
	}()

	select {
	case err := <-done:
		if err != nil {
			return fmt.Errorf("helper process failed; %w; stderr=%q", err, w.helperStderr.String())
		}
		return nil
	case <-time.After(timeout):
		if w.helperCmd.Process != nil {
			_ = w.helperCmd.Process.Kill()
		}
		<-done
		return fmt.Errorf("helper process did not exit within %s; stderr=%q", timeout, w.helperStderr.String())
	}
}

func (w *harnessWorld) stopHelperProcess() error {
	if w.helperCmd == nil {
		return nil
	}
	if w.helperCmd.ProcessState == nil && w.helperCmd.Process != nil {
		_ = w.helperCmd.Process.Kill()
		if err := w.helperCmd.Wait(); err != nil && !isKilledExecError(err) {
			return fmt.Errorf("failed to stop helper process; %w; stderr=%q", err, w.helperStderr.String())
		}
	}
	w.helperCmd = nil
	return nil
}

func (w *harnessWorld) stopInvalidProcess() error {
	if w.invalidProcessCmd == nil {
		return nil
	}
	if w.invalidProcessCmd.ProcessState == nil && w.invalidProcessCmd.Process != nil {
		_ = w.invalidProcessCmd.Process.Kill()
		if err := w.invalidProcessCmd.Wait(); err != nil && !isKilledExecError(err) {
			return fmt.Errorf("failed to stop invalid-case process; %w", err)
		}
	}
	w.invalidProcessCmd = nil
	return nil
}

func isKilledExecError(err error) bool {
	return err != nil && strings.Contains(err.Error(), "signal: killed")
}

func (w *harnessWorld) createTerminalRun(targetState sigilruntime.RunState) (string, error) {
	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir:  sigilruntime.DefaultRunsBaseDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     1,
	})
	if err != nil {
		return "", err
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	switch targetState {
	case sigilruntime.RunStateCompleted:
		if err := lifecycle.StartExecution(); err != nil {
			return "", err
		}
		if err := lifecycle.Complete(); err != nil {
			return "", err
		}
	case sigilruntime.RunStateFailed:
		if err := lifecycle.StartExecution(); err != nil {
			return "", err
		}
		if err := lifecycle.Fail(); err != nil {
			return "", err
		}
	case sigilruntime.RunStateInterrupted:
		if err := lifecycle.StartExecution(); err != nil {
			return "", err
		}
		if err := lifecycle.Interrupt(); err != nil {
			return "", err
		}
	default:
		return "", fmt.Errorf("unsupported terminal run state %q", targetState)
	}
	return lifecycle.RunID(), nil
}

func validateCanonicalEventTypeFixtures() error {
	runID := mustUUIDv7StringOrPanic()
	nodeID := mustUUIDv7StringOrPanic()
	parentID := mustUUIDv7StringOrPanic()
	stepID := mustUUIDv7StringOrPanic()
	contentRef := "run-artifact://node/turn/1"
	actionRef, err := sigilruntime.BuildActionArtifactRef(nodeID, stepID, 1)
	if err != nil {
		return err
	}

	fixtures := []map[string]any{
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            1,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunQueued,
			"causation_id":   "", // set below
			"correlation_id": runID,
			"payload": map[string]any{
				"source": string(sigilruntime.RunQueuedSourceInternalResume),
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunRunning,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"executor":  "rlm",
				"max_depth": 0,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeStarted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"depth":          1,
				"parent_node_id": parentID,
				"role":           string(sigilruntime.NodeRoleRecursiveSubcall),
				"attempt":        1,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeCompleted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"status":      "completed",
				"duration_ms": 0,
				"accounting":  acceptanceAccountingRollup("openai", "gpt-5.1"),
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeStepStarted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":    stepID,
				"step_index": 1,
				"schema_id":  "sigil.rlm.response.v1",
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeStepCompleted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":        stepID,
				"decision":       string(sigilruntime.StepDecisionContinue),
				"action_count":   1,
				"duration_ms":    0,
				"accounting":     acceptanceAccountingRollup("openai", "gpt-5.1"),
				"accounting_ref": "run-artifact://node/" + nodeID + "/step/" + stepID + "/accounting.json",
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeTurnUser,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":     stepID,
				"role":        string(sigilruntime.TurnRoleUser),
				"content_ref": contentRef,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeSubcallExecuted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":        stepID,
				"action_index":   1,
				"subcall_index":  1,
				"subcall_type":   string(sigilruntime.SubcallTypeLLMQuery),
				"execution_mode": string(sigilruntime.SubcallExecutionModePlain),
				"status":         string(sigilruntime.ActionExecutionStatusCompleted),
				"provider":       "openai",
				"model":          "gpt-5.1",
				"prompt_bytes":   1,
				"context_bytes":  1,
				"answer_bytes":   1,
				"duration_ms":    1,
				"accounting":     acceptanceAccountingSummary("openai", "gpt-5.1"),
				"accounting_ref": "run-artifact://node/" + nodeID + "/step/" + stepID + "/subcall-1-accounting.json",
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeTurnModel,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":     stepID,
				"role":        string(sigilruntime.TurnRoleModel),
				"content_ref": contentRef,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeNodeActionExecuted,
			"node_id":        nodeID,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"step_id":      stepID,
				"action_index": 1,
				"action_type":  "repl_code",
				"language":     "go",
				"status":       string(sigilruntime.ActionExecutionStatusCompleted),
				"duration_ms":  0,
				"action_ref":   actionRef,
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunCompleted,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"status":      "completed",
				"duration_ms": 0,
				"accounting":  acceptanceAccountingRollup("openai", "gpt-5.1"),
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunFailed,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"status":        "failed",
				"error_code":    "runtime.failure",
				"error_message": "failure",
				"retryable":     false,
				"accounting":    acceptanceAccountingRollup("openai", "gpt-5.1"),
			},
		},
		{
			"event_id":       mustUUIDv7StringOrPanic(),
			"schema_version": sigilruntime.SchemaVersionV1,
			"run_id":         runID,
			"seq":            2,
			"ts":             time.Now().UTC().Format(time.RFC3339Nano),
			"type":           sigilruntime.EventTypeRunInterrupted,
			"causation_id":   mustUUIDv7StringOrPanic(),
			"correlation_id": runID,
			"payload": map[string]any{
				"status":         "interrupted",
				"reason":         string(sigilruntime.RunInterruptedReasonUserRequest),
				"interrupted_by": sigilruntime.RunInterruptedByLifecycle,
				"accounting":     acceptanceAccountingRollup("openai", "gpt-5.1"),
			},
		},
	}

	for index := range fixtures {
		if index == 0 {
			fixtures[index]["causation_id"] = fixtures[index]["event_id"]
		}
		raw, err := marshalEnvelope(fixtures[index])
		if err != nil {
			return err
		}
		if _, err := sigilruntime.ParseEventEnvelopeStrict(raw); err != nil {
			return err
		}
	}

	return nil
}

func marshalEnvelope(envelope map[string]any) ([]byte, error) {
	return json.Marshal(envelope)
}

func mustUUIDv7StringOrPanic() string {
	id, err := uuid.NewV7()
	if err != nil {
		panic(err)
	}
	return id.String()
}

func isUUIDv7String(raw string) bool {
	parsed, err := uuid.Parse(raw)
	if err != nil {
		return false
	}
	return parsed.Version() == uuid.Version(7)
}

func (w *harnessWorld) rootNode() (sigilruntime.Node, error) {
	if w.lifecycle == nil {
		return sigilruntime.Node{}, fmt.Errorf("lifecycle is not initialized")
	}

	nodes := w.lifecycle.Nodes()
	if len(nodes) == 0 {
		return sigilruntime.Node{}, fmt.Errorf("expected lifecycle to contain nodes")
	}

	rootCount := 0
	var root sigilruntime.Node
	for _, node := range nodes {
		if node.Depth == 0 && node.ParentNodeID == nil {
			root = node
			rootCount++
		}
	}

	if rootCount != 1 {
		return sigilruntime.Node{}, fmt.Errorf("expected exactly one root node, got %d", rootCount)
	}

	return root, nil
}

func parseRunState(raw string) (sigilruntime.RunState, error) {
	switch strings.TrimSpace(raw) {
	case string(sigilruntime.RunStateQueued):
		return sigilruntime.RunStateQueued, nil
	case string(sigilruntime.RunStateRunning):
		return sigilruntime.RunStateRunning, nil
	case string(sigilruntime.RunStateCompleted):
		return sigilruntime.RunStateCompleted, nil
	case string(sigilruntime.RunStateFailed):
		return sigilruntime.RunStateFailed, nil
	case string(sigilruntime.RunStateInterrupted):
		return sigilruntime.RunStateInterrupted, nil
	default:
		return "", fmt.Errorf("unsupported run state %q", raw)
	}
}

func (w *harnessWorld) assertEffectiveLogPath(expectedPath string) error {
	if w.loggingInitErr != nil {
		return fmt.Errorf("application logging initialization failed; %w", w.loggingInitErr)
	}

	expected, err := config.ExpandPath(expectedPath)
	if err != nil {
		return fmt.Errorf("failed to resolve expected log path %q; %w", expectedPath, err)
	}

	actual, err := logging.ActiveLogFilePath()
	if err != nil {
		return err
	}

	if actual != expected {
		return fmt.Errorf("expected effective log path %q, got %q", expected, actual)
	}

	return nil
}
