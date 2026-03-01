package steps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
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
}

// InitializeScenario wires all acceptance steps for harness.feature.
func InitializeScenario(ctx *godog.ScenarioContext) {
	world := &harnessWorld{}

	ctx.Before(func(ctx context.Context, _ *godog.Scenario) (context.Context, error) {
		if world.inferenceMockServer != nil {
			world.inferenceMockServer.Close()
			world.inferenceMockServer = nil
		}

		if world.lifecycle != nil {
			if err := world.lifecycle.Close(); err != nil {
				return ctx, fmt.Errorf("failed to close lifecycle from previous scenario; %w", err)
			}
			world.lifecycle = nil
		}

		_ = logging.Close()

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

		return ctx, nil
	})

	ctx.After(func(ctx context.Context, _ *godog.Scenario, _ error) (context.Context, error) {
		if world.inferenceMockServer != nil {
			world.inferenceMockServer.Close()
			world.inferenceMockServer = nil
		}

		if world.lifecycle != nil {
			if err := world.lifecycle.Close(); err != nil {
				return ctx, fmt.Errorf("failed to close lifecycle resources; %w", err)
			}
			world.lifecycle = nil
		}

		_ = logging.Close()

		if world.originalWorkingDir != "" {
			_ = os.Chdir(world.originalWorkingDir)
		}
		if world.workingDir != "" {
			_ = os.RemoveAll(world.workingDir)
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
	ctx.Step(`^effective application log_level is "([^"]*)"$`, world.effectiveApplicationLogLevelIs)
	ctx.Step(`^effective application log_dir is "([^"]*)"$`, world.effectiveApplicationLogDirIs)
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
	ctx.Step(`^events are persisted to a per-run append-only events\.jsonl path under sigil runs directory$`, world.eventsArePersistedToAPerRunAppendOnlyEventsJSONLPathUnderSigilRunsDirectory)
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
	ctx.Step(`^only canonical v1 lifecycle event types are accepted$`, world.onlyCanonicalV1LifecycleEventTypesAreAccepted)
	ctx.Step(`^canonical v1 lifecycle events with payloads$`, world.canonicalV1LifecycleEventsWithPayloads)
	ctx.Step(`^strict payload schema validation is executed$`, world.strictPayloadSchemaValidationIsExecuted)
	ctx.Step(`^required fields types and invariants are enforced per event type$`, world.requiredFieldsTypesAndInvariantsAreEnforcedPerEventType)
	ctx.Step(`^v1 event envelopes with unknown fields or unknown type$`, world.v1EventEnvelopesWithUnknownFieldsOrUnknownType)
	ctx.Step(`^strict v1 extensibility validation is executed$`, world.strictV1ExtensibilityValidationIsExecuted)
	ctx.Step(`^validation fails and events are rejected$`, world.validationFailsAndEventsAreRejected)
	ctx.Step(`^a core lifecycle event payload includes deferred non-core fields$`, world.aCoreLifecycleEventPayloadIncludesDeferredNonCoreFields)
	ctx.Step(`^core v1 payload validation executes$`, world.coreV1PayloadValidationExecutes)
	ctx.Step(`^deferred non-core fields are rejected as out-of-contract$`, world.deferredNonCoreFieldsAreRejectedAsOutOfContract)
	ctx.Step(`^effective run llm.provider is "([^"]*)"$`, world.effectiveRunLLMProviderIs)
	ctx.Step(`^effective run llm.model is "([^"]*)"$`, world.effectiveRunLLMModelIs)
	ctx.Step(`^effective run llm.gateway is "([^"]*)"$`, world.effectiveRunLLMGatewayIs)
	ctx.Step(`^effective run llm.openrouter.base_url is "([^"]*)"$`, world.effectiveRunLLMOpenRouterBaseURLIs)
	ctx.Step(`^effective run llm.openrouter.request_timeout_ms is (\d+)$`, world.effectiveRunLLMOpenRouterRequestTimeoutMSIs)
	ctx.Step(`^effective run llm.openrouter.api_key_env is "([^"]*)"$`, world.effectiveRunLLMOpenRouterAPIKeyEnvIs)
	ctx.Step(`^effective run rlm.enabled is (true|false)$`, world.effectiveRunRLMEnabledIs)
	ctx.Step(`^effective run rlm.max_depth is (\d+)$`, world.effectiveRunRLMMaxDepthIs)
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
	ctx.Step(`^no default start config files exist$`, world.noDefaultStartConfigFilesExist)
	ctx.Step(`^no default run config files exist$`, world.noDefaultRunConfigFilesExist)

	registerInferenceSteps(ctx, world)
}

func (w *harnessWorld) aCleanSigilWorkingDirectory() error {
	return nil
}

func (w *harnessWorld) sigilConfigEnvironmentVariablesAreCleared() error {
	keys := []string{
		"SIGIL_LOG_LEVEL",
		"SIGIL_LOG_DIR",
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
	cfgType := reflect.TypeOf(config.Config{})
	tags := make(map[string]struct{}, cfgType.NumField())
	for index := 0; index < cfgType.NumField(); index++ {
		field := cfgType.Field(index)
		tag := field.Tag.Get("mapstructure")
		tags[tag] = struct{}{}
	}

	if _, ok := tags[expectedKeyOne]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyOne)
	}
	if _, ok := tags[expectedKeyTwo]; !ok {
		return fmt.Errorf("missing baseline config key %q", expectedKeyTwo)
	}

	return nil
}

func (w *harnessWorld) effectiveApplicationLogLevelIs(expectedLevel string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.LogLevel != expectedLevel {
		return fmt.Errorf("expected log_level %q, got %q", expectedLevel, cfg.LogLevel)
	}

	return nil
}

func (w *harnessWorld) effectiveApplicationLogDirIs(expectedDir string) error {
	if w.configInitErr != nil {
		return fmt.Errorf("config initialization failed: %w", w.configInitErr)
	}

	expectedPath, err := config.ExpandPath(expectedDir)
	if err != nil {
		return fmt.Errorf("failed to resolve expected log_dir %q; %w", expectedDir, err)
	}

	cfg, err := config.Get()
	if err != nil {
		return err
	}

	if cfg.LogDir != expectedPath {
		return fmt.Errorf("expected log_dir %q, got %q", expectedPath, cfg.LogDir)
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

func (w *harnessWorld) eventsArePersistedToAPerRunAppendOnlyEventsJSONLPathUnderSigilRunsDirectory() error {
	if err := w.capturePersistedEvents(); err != nil {
		return err
	}

	actualPath := filepath.Clean(w.eventLogPath)
	expectedSegment := filepath.Join("sigil", "runs", w.lifecycle.RunID(), "events.jsonl")
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
			"status":      "completed",
			"duration_ms": 0,
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

	rootCmd := cmd.NewRootCmd()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	rootCmd.SetOut(&stdout)
	rootCmd.SetErr(&stderr)
	rootCmd.SetArgs(tokens[1:])

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
		RunsBaseDir: "./sigil/runs",
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

func validateCanonicalEventTypeFixtures() error {
	runID := mustUUIDv7StringOrPanic()
	nodeID := mustUUIDv7StringOrPanic()
	parentID := mustUUIDv7StringOrPanic()

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
				"status": "interrupted",
				"reason": string(sigilruntime.RunInterruptedReasonUserRequest),
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
