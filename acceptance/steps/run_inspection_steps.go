package steps

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/cucumber/godog"
	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/harness"
	sigilruntime "github.com/leefowlercu/sigil/internal/runtime"
)

type runInspectionState struct {
	selectedRunsArg string
	selectedRunsDir string
	missingRunsArg  string
	missingRunsDir  string

	helpOutputs []string

	olderRunID    string
	targetedRunID string
	defaultRunID  string

	summaryRunID       string
	missingRunID       string
	notRunningRunID    string
	staleRunID         string
	corruptRunID       string
	missingEventsRunID string

	startResult harness.RunResult
	stopResult  acceptanceStopResult

	listTextOutput    string
	listJSONOutput    string
	statusTextOutput  string
	statusJSONOutput  string
	inspectTextOutput string
	inspectJSONOutput string
	eventsJSONOutput  string

	overrideListOutput    string
	overrideStatusOutput  string
	overrideInspectOutput string
	overrideEventsOutput  string

	derivedSummaries           []sigilruntime.RunSummary
	derivedProjection          sigilruntime.RunProjection
	derivedSecondaryProjection sigilruntime.RunProjection
	queryErrors                []error
	eventCountBefore           int
	eventCountAfter            int
}

func registerRunInspectionSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^valid application and run configuration inputs$`, world.validApplicationAndRunConfigurationInputs)
	ctx.Step(`^run subcommand help surfaces are inspected for inherited run-dir support$`, world.runSubcommandHelpSurfacesAreInspectedForInheritedRunDirSupport)
	ctx.Step("^each inspected run help surface documents `([^`]*)`$", world.eachInspectedRunHelpSurfaceDocuments)
	ctx.Step(`^a user runs sigil run start with inherited run-dir override in json mode$`, world.aUserRunsSigilRunStartWithInheritedRunDirOverrideInJSONMode)
	ctx.Step("^the run start result events path is stored under \"([^\"]*)\"$", world.theRunStartResultEventsPathIsStoredUnder)
	ctx.Step(`^sigil run start is invoked with an explicit empty inherited run-dir value$`, world.sigilRunStartIsInvokedWithAnExplicitEmptyInheritedRunDirValue)
	ctx.Step(`^a terminal run exists only in the selected run directory$`, world.aTerminalRunExistsOnlyInTheSelectedRunDirectory)
	ctx.Step(`^a user runs sigil run stop for the selected run directory$`, world.aUserRunsSigilRunStopForTheSelectedRunDirectory)
	ctx.Step("^the stop result references the selected run directory \"([^\"]*)\"$", world.theStopResultReferencesTheSelectedRunDirectory)
	ctx.Step(`^sigil run stop is invoked with an explicit empty inherited run-dir value$`, world.sigilRunStopIsInvokedWithAnExplicitEmptyInheritedRunDirValue)
	ctx.Step(`^one or more persisted runs exist in the selected run directory$`, world.oneOrMorePersistedRunsExistInTheSelectedRunDirectory)
	ctx.Step(`^persisted runs exist in the selected run directory$`, world.persistedRunsExistInTheSelectedRunDirectory)
	ctx.Step(`^a targeted persisted run exists in the selected run directory$`, world.aTargetedPersistedRunExistsInTheSelectedRunDirectory)
	ctx.Step(`^persisted runs exist outside the default run storage directory$`, world.persistedRunsExistOutsideTheDefaultRunStorageDirectory)
	ctx.Step(`^the selected run directory does not exist$`, world.theSelectedRunDirectoryDoesNotExist)
	ctx.Step(`^a user runs sigil run list for the selected run directory$`, world.aUserRunsSigilRunListForTheSelectedRunDirectory)
	ctx.Step(`^a user runs sigil run list for the missing selected run directory$`, world.aUserRunsSigilRunListForTheMissingSelectedRunDirectory)
	ctx.Step(`^the returned run summaries are ordered newest-first by queued time$`, world.theReturnedRunSummariesAreOrderedNewestFirstByQueuedTime)
	ctx.Step(`^the command exits with status code 0 and returns an empty result$`, world.theCommandExitsWithStatusCode0AndReturnsAnEmptyResult)
	ctx.Step(`^a user runs sigil run list in text mode and in json mode$`, world.aUserRunsSigilRunListInTextModeAndInJSONMode)
	ctx.Step(`^both outputs return the same run-summary set in their respective formats$`, world.bothOutputsReturnTheSameRunSummarySetInTheirRespectiveFormats)
	ctx.Step(`^a user runs sigil run status for the targeted run in text mode and in json mode$`, world.aUserRunsSigilRunStatusForTheTargetedRunInTextModeAndInJSONMode)
	ctx.Step(`^both outputs return the same run summary in their respective formats$`, world.bothOutputsReturnTheSameRunSummaryInTheirRespectiveFormats)
	ctx.Step(`^a user runs sigil run inspect for the targeted run in text mode and in json mode$`, world.aUserRunsSigilRunInspectForTheTargetedRunInTextModeAndInJSONMode)
	ctx.Step(`^both outputs return the same run inspection summary in their respective formats$`, world.bothOutputsReturnTheSameRunInspectionSummaryInTheirRespectiveFormats)
	ctx.Step(`^a user runs sigil run events for the targeted run in json mode$`, world.aUserRunsSigilRunEventsForTheTargetedRunInJSONMode)
	ctx.Step(`^the returned event stream preserves canonical append order$`, world.theReturnedEventStreamPreservesCanonicalAppendOrder)
	ctx.Step(`^run inspection commands are executed with inherited run-dir override$`, world.runInspectionCommandsAreExecutedWithInheritedRunDirOverride)
	ctx.Step("^only runs stored under \"([^\"]*)\" are inspected$", world.onlyRunsStoredUnderAreInspected)
	ctx.Step(`^canonical events and auxiliary control metadata exist for one run$`, world.canonicalEventsAndAuxiliaryControlMetadataExistForOneRun)
	ctx.Step(`^a run summary is requested from the selected run directory$`, world.aRunSummaryIsRequestedFromTheSelectedRunDirectory)
	ctx.Step(`^the summary is derived from canonical events plus auxiliary control metadata$`, world.theSummaryIsDerivedFromCanonicalEventsPlusAuxiliaryControlMetadata)
	ctx.Step(`^canonical events and run-local refs exist for one run$`, world.canonicalEventsAndRunLocalRefsExistForOneRun)
	ctx.Step(`^a run projection is requested from the selected run directory$`, world.aRunProjectionIsRequestedFromTheSelectedRunDirectory)
	ctx.Step(`^the projection is derived on demand without persisting a separate read model$`, world.theProjectionIsDerivedOnDemandWithoutPersistingASeparateReadModel)
	ctx.Step(`^process metadata states vary across runs$`, world.processMetadataStatesVaryAcrossRuns)
	ctx.Step(`^run summaries are derived from the selected run directory$`, world.runSummariesAreDerivedFromTheSelectedRunDirectory)
	ctx.Step(`^pid_status reports current missing not_running or stale accordingly$`, world.pidStatusReportsCurrentMissingNotRunningOrStaleAccordingly)
	ctx.Step(`^stop-request metadata exists for one run$`, world.stopRequestMetadataExistsForOneRun)
	ctx.Step(`^a run summary or run projection is requested from the selected run directory$`, world.aRunSummaryOrRunProjectionIsRequestedFromTheSelectedRunDirectory)
	ctx.Step(`^stop_requested is surfaced without changing canonical event authority$`, world.stopRequestedIsSurfacedWithoutChangingCanonicalEventAuthority)
	ctx.Step(`^canonical terminal refs exist for one run$`, world.canonicalTerminalRefsExistForOneRun)
	ctx.Step(`^final-answer and accounting data are exposed as refs rather than inline artifact bodies$`, world.finalAnswerAndAccountingDataAreExposedAsRefsRatherThanInlineArtifactBodies)
	ctx.Step(`^a targeted run has missing or corrupt canonical event storage$`, world.aTargetedRunHasMissingOrCorruptCanonicalEventStorage)
	ctx.Step(`^targeted run queries are requested from the selected run directory$`, world.targetedRunQueriesAreRequestedFromTheSelectedRunDirectory)
	ctx.Step(`^targeted run queries fail for missing or corrupt canonical event storage$`, world.targetedRunQueriesFailForMissingOrCorruptCanonicalEventStorage)
}

func (w *harnessWorld) validApplicationAndRunConfigurationInputs() error {
	if err := os.WriteFile(filepath.Clean("./sigil.yaml"), []byte("log_level: info\n"), 0o644); err != nil {
		return fmt.Errorf("failed to write sigil.yaml; %w", err)
	}
	runConfig := strings.Join([]string{
		"prompt: test prompt",
		"context: test context",
		"llm:",
		"  provider: openai",
		"  model: gpt-5.1",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Clean("./sigil-run.yaml"), []byte(runConfig), 0o644); err != nil {
		return fmt.Errorf("failed to write sigil-run.yaml; %w", err)
	}
	return nil
}

func (w *harnessWorld) runSubcommandHelpSurfacesAreInspectedForInheritedRunDirSupport() error {
	state := w.inspectionState()
	commands := [][]string{
		{"run", "start", "--help"},
		{"run", "stop", "--help"},
		{"run", "list", "--help"},
		{"run", "status", "--help"},
		{"run", "inspect", "--help"},
		{"run", "events", "--help"},
	}
	state.helpOutputs = state.helpOutputs[:0]
	for _, args := range commands {
		if err := w.executeSigilArgs(args); err != nil {
			return err
		}
		if w.lastExitCode != 0 {
			return fmt.Errorf("expected help success for %v, got exit code %d with stderr %q", args, w.lastExitCode, w.lastStderr)
		}
		state.helpOutputs = append(state.helpOutputs, w.lastStdout)
	}
	return nil
}

func (w *harnessWorld) eachInspectedRunHelpSurfaceDocuments(expected string) error {
	state := w.inspectionState()
	if len(state.helpOutputs) == 0 {
		return fmt.Errorf("expected inspected help outputs before assertion")
	}
	for _, output := range state.helpOutputs {
		if !strings.Contains(output, expected) {
			return fmt.Errorf("expected help output to contain %q, got %q", expected, output)
		}
	}
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunStartWithInheritedRunDirOverrideInJSONMode() error {
	if err := w.validApplicationAndRunConfigurationInputs(); err != nil {
		return err
	}
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "start", "-o", "json"}); err != nil {
		return err
	}
	result, err := decodeRunStartResult(w.lastStdout)
	if err != nil {
		return err
	}
	state.startResult = result
	return nil
}

func (w *harnessWorld) theRunStartResultEventsPathIsStoredUnder(path string) error {
	state := w.inspectionState()
	expectedRunsDir, err := sigilruntime.ResolveRunsBaseDir(path)
	if err != nil {
		return err
	}
	expectedPath := filepath.Join(expectedRunsDir, state.startResult.RunID, "events.jsonl")
	if filepath.Clean(state.startResult.EventsPath) != filepath.Clean(expectedPath) {
		return fmt.Errorf("expected events_path %q, got %q", expectedPath, state.startResult.EventsPath)
	}
	return nil
}

func (w *harnessWorld) sigilRunStartIsInvokedWithAnExplicitEmptyInheritedRunDirValue() error {
	return w.executeSigilArgs([]string{"run", "--run-dir", "", "start"})
}

func (w *harnessWorld) aTerminalRunExistsOnlyInTheSelectedRunDirectory() error {
	state := w.inspectionState()
	if state.targetedRunID != "" {
		return nil
	}

	finalAnswerRef := "run-artifact://run/final-answer.json"
	accountingRef := "run-artifact://run/accounting.json"
	runID, err := w.createCompletedRun(state.selectedRunsDir, &finalAnswerRef, &accountingRef)
	if err != nil {
		return err
	}
	state.targetedRunID = runID
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunStopForTheSelectedRunDirectory() error {
	state := w.inspectionState()
	if err := w.aTerminalRunExistsOnlyInTheSelectedRunDirectory(); err != nil {
		return err
	}
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "stop", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	result, err := parseAcceptanceStopResult(w.lastStdout)
	if err != nil {
		return err
	}
	state.stopResult = result
	return nil
}

func (w *harnessWorld) theStopResultReferencesTheSelectedRunDirectory(path string) error {
	state := w.inspectionState()
	expectedRunsDir, err := sigilruntime.ResolveRunsBaseDir(path)
	if err != nil {
		return err
	}
	expectedPath := filepath.Join(expectedRunsDir, state.targetedRunID, "events.jsonl")
	if state.stopResult.RunID != state.targetedRunID {
		return fmt.Errorf("expected stop result run_id %q, got %q", state.targetedRunID, state.stopResult.RunID)
	}
	if filepath.Clean(state.stopResult.EventsPath) != filepath.Clean(expectedPath) {
		return fmt.Errorf("expected stop result events_path %q, got %q", expectedPath, state.stopResult.EventsPath)
	}
	return nil
}

func (w *harnessWorld) sigilRunStopIsInvokedWithAnExplicitEmptyInheritedRunDirValue() error {
	return w.executeSigilArgs([]string{"run", "--run-dir", "", "stop", "019c7714-3b77-74d1-9866-e1f484aae2ab"})
}

func (w *harnessWorld) oneOrMorePersistedRunsExistInTheSelectedRunDirectory() error {
	return w.ensureSelectedRunListFixtures()
}

func (w *harnessWorld) persistedRunsExistInTheSelectedRunDirectory() error {
	return w.ensureSelectedRunListFixtures()
}

func (w *harnessWorld) aTargetedPersistedRunExistsInTheSelectedRunDirectory() error {
	return w.ensureSelectedRunListFixtures()
}

func (w *harnessWorld) persistedRunsExistOutsideTheDefaultRunStorageDirectory() error {
	return w.ensureSelectedRunListFixtures()
}

func (w *harnessWorld) theSelectedRunDirectoryDoesNotExist() error {
	state := w.inspectionState()
	return os.RemoveAll(filepath.Clean(state.missingRunsDir))
}

func (w *harnessWorld) aUserRunsSigilRunListForTheSelectedRunDirectory() error {
	state := w.inspectionState()
	return w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "list", "-o", "json"})
}

func (w *harnessWorld) aUserRunsSigilRunListForTheMissingSelectedRunDirectory() error {
	state := w.inspectionState()
	return w.executeSigilArgs([]string{"run", "--run-dir", state.missingRunsArg, "list", "-o", "json"})
}

func (w *harnessWorld) theReturnedRunSummariesAreOrderedNewestFirstByQueuedTime() error {
	state := w.inspectionState()
	summaries, err := decodeRunSummaries(w.lastStdout)
	if err != nil {
		return err
	}
	if len(summaries) < 2 {
		return fmt.Errorf("expected at least two summaries, got %d", len(summaries))
	}
	if summaries[0].RunID != state.targetedRunID {
		return fmt.Errorf("expected newest run_id %q, got %q", state.targetedRunID, summaries[0].RunID)
	}
	if summaries[1].RunID != state.olderRunID {
		return fmt.Errorf("expected second run_id %q, got %q", state.olderRunID, summaries[1].RunID)
	}
	if summaries[0].QueuedAt == nil || summaries[1].QueuedAt == nil {
		return fmt.Errorf("expected queued_at values in summaries")
	}
	if summaries[0].QueuedAt.Before(*summaries[1].QueuedAt) {
		return fmt.Errorf("expected newest-first ordering, got %s before %s", summaries[0].QueuedAt, summaries[1].QueuedAt)
	}
	return nil
}

func (w *harnessWorld) theCommandExitsWithStatusCode0AndReturnsAnEmptyResult() error {
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected exit code 0, got %d", w.lastExitCode)
	}
	summaries, err := decodeRunSummaries(w.lastStdout)
	if err != nil {
		return err
	}
	if len(summaries) != 0 {
		return fmt.Errorf("expected empty run-summary result, got %d entries", len(summaries))
	}
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunListInTextModeAndInJSONMode() error {
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "list"}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected text list success, got exit code %d", w.lastExitCode)
	}
	state.listTextOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "list", "-o", "json"}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected json list success, got exit code %d", w.lastExitCode)
	}
	state.listJSONOutput = w.lastStdout
	return nil
}

func (w *harnessWorld) bothOutputsReturnTheSameRunSummarySetInTheirRespectiveFormats() error {
	state := w.inspectionState()
	summaries, err := decodeRunSummaries(state.listJSONOutput)
	if err != nil {
		return err
	}
	if len(summaries) != 2 {
		return fmt.Errorf("expected two summaries in json output, got %d", len(summaries))
	}
	lastIndex := -1
	for _, summary := range summaries {
		index := strings.Index(state.listTextOutput, summary.RunID)
		if index < 0 {
			return fmt.Errorf("expected text output to contain run_id %q, got %q", summary.RunID, state.listTextOutput)
		}
		if index < lastIndex {
			return fmt.Errorf("expected text output ordering to match json ordering")
		}
		lastIndex = index
		if !strings.Contains(state.listTextOutput, "State: "+summary.State) {
			return fmt.Errorf("expected text output to contain state %q, got %q", summary.State, state.listTextOutput)
		}
	}
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunStatusForTheTargetedRunInTextModeAndInJSONMode() error {
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "status", state.targetedRunID}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected text status success, got exit code %d", w.lastExitCode)
	}
	state.statusTextOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "status", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected json status success, got exit code %d", w.lastExitCode)
	}
	state.statusJSONOutput = w.lastStdout
	return nil
}

func (w *harnessWorld) bothOutputsReturnTheSameRunSummaryInTheirRespectiveFormats() error {
	state := w.inspectionState()
	summary, err := decodeRunSummary(state.statusJSONOutput)
	if err != nil {
		return err
	}
	if summary.RunID != state.targetedRunID {
		return fmt.Errorf("expected targeted run_id %q, got %q", state.targetedRunID, summary.RunID)
	}
	requiredFragments := []string{
		"Run ID: " + summary.RunID,
		"State: " + summary.State,
		"Events path: " + summary.EventsPath,
	}
	if summary.FinalAnswerRef != nil {
		requiredFragments = append(requiredFragments, "Final answer ref: "+*summary.FinalAnswerRef)
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(state.statusTextOutput, fragment) {
			return fmt.Errorf("expected text status output to contain %q, got %q", fragment, state.statusTextOutput)
		}
	}
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunInspectForTheTargetedRunInTextModeAndInJSONMode() error {
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "inspect", state.targetedRunID}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected text inspect success, got exit code %d", w.lastExitCode)
	}
	state.inspectTextOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "inspect", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected json inspect success, got exit code %d", w.lastExitCode)
	}
	state.inspectJSONOutput = w.lastStdout
	return nil
}

func (w *harnessWorld) bothOutputsReturnTheSameRunInspectionSummaryInTheirRespectiveFormats() error {
	state := w.inspectionState()
	projection, err := decodeRunProjection(state.inspectJSONOutput)
	if err != nil {
		return err
	}
	if projection.RunID != state.targetedRunID {
		return fmt.Errorf("expected targeted run_id %q, got %q", state.targetedRunID, projection.RunID)
	}
	requiredFragments := []string{
		"Run ID: " + projection.RunID,
		"State: " + projection.State,
		fmt.Sprintf("Node count: %d", projection.NodeCount),
		fmt.Sprintf("Step count: %d", projection.StepCount),
	}
	if projection.FinalAnswerRef != nil {
		requiredFragments = append(requiredFragments, "Final answer ref: "+*projection.FinalAnswerRef)
	}
	if projection.AccountingRef != nil {
		requiredFragments = append(requiredFragments, "Accounting ref: "+*projection.AccountingRef)
	}
	for _, fragment := range requiredFragments {
		if !strings.Contains(state.inspectTextOutput, fragment) {
			return fmt.Errorf("expected text inspect output to contain %q, got %q", fragment, state.inspectTextOutput)
		}
	}
	return nil
}

func (w *harnessWorld) aUserRunsSigilRunEventsForTheTargetedRunInJSONMode() error {
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "events", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	if w.lastExitCode != 0 {
		return fmt.Errorf("expected json events success, got exit code %d", w.lastExitCode)
	}
	state.eventsJSONOutput = w.lastStdout
	return nil
}

func (w *harnessWorld) theReturnedEventStreamPreservesCanonicalAppendOrder() error {
	state := w.inspectionState()
	events, err := decodeRunEvents(state.eventsJSONOutput)
	if err != nil {
		return err
	}
	if len(events) == 0 {
		return fmt.Errorf("expected at least one event")
	}
	expectedSeq := int64(1)
	for _, event := range events {
		if event.RunID != state.targetedRunID {
			return fmt.Errorf("expected event run_id %q, got %q", state.targetedRunID, event.RunID)
		}
		if event.Seq != expectedSeq {
			return fmt.Errorf("expected seq %d, got %d", expectedSeq, event.Seq)
		}
		expectedSeq++
	}
	return nil
}

func (w *harnessWorld) runInspectionCommandsAreExecutedWithInheritedRunDirOverride() error {
	state := w.inspectionState()
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "list", "-o", "json"}); err != nil {
		return err
	}
	state.overrideListOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "status", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	state.overrideStatusOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "inspect", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	state.overrideInspectOutput = w.lastStdout
	if err := w.executeSigilArgs([]string{"run", "--run-dir", state.selectedRunsArg, "events", "-o", "json", state.targetedRunID}); err != nil {
		return err
	}
	state.overrideEventsOutput = w.lastStdout
	return nil
}

func (w *harnessWorld) onlyRunsStoredUnderAreInspected(path string) error {
	state := w.inspectionState()
	expectedRunsDir, err := sigilruntime.ResolveRunsBaseDir(path)
	if err != nil {
		return err
	}
	listSummaries, err := decodeRunSummaries(state.overrideListOutput)
	if err != nil {
		return err
	}
	for _, summary := range listSummaries {
		if summary.RunID == state.defaultRunID {
			return fmt.Errorf("expected override list output to exclude default-dir run %q", state.defaultRunID)
		}
		if !strings.HasPrefix(filepath.Clean(summary.EventsPath), filepath.Clean(expectedRunsDir)+string(os.PathSeparator)) {
			return fmt.Errorf("expected override events_path under %q, got %q", expectedRunsDir, summary.EventsPath)
		}
	}
	statusSummary, err := decodeRunSummary(state.overrideStatusOutput)
	if err != nil {
		return err
	}
	inspectProjection, err := decodeRunProjection(state.overrideInspectOutput)
	if err != nil {
		return err
	}
	events, err := decodeRunEvents(state.overrideEventsOutput)
	if err != nil {
		return err
	}
	if statusSummary.RunID != state.targetedRunID || inspectProjection.RunID != state.targetedRunID {
		return fmt.Errorf("expected override status/inspect outputs for run_id %q", state.targetedRunID)
	}
	for _, event := range events {
		if event.RunID != state.targetedRunID {
			return fmt.Errorf("expected override events to belong to run_id %q, got %q", state.targetedRunID, event.RunID)
		}
	}
	return nil
}

func (w *harnessWorld) canonicalEventsAndAuxiliaryControlMetadataExistForOneRun() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) aRunSummaryIsRequestedFromTheSelectedRunDirectory() error {
	state := w.inspectionState()
	summaries, err := sigilruntime.ListRuns(state.selectedRunsDir)
	if err != nil {
		w.runInspection.queryErrors = []error{err}
		return nil
	}
	state.derivedSummaries = summaries
	state.queryErrors = nil
	return nil
}

func (w *harnessWorld) theSummaryIsDerivedFromCanonicalEventsPlusAuxiliaryControlMetadata() error {
	state := w.inspectionState()
	summary, ok := summaryByRunID(state.derivedSummaries, state.summaryRunID)
	if !ok {
		return fmt.Errorf("expected summary for run_id %q", state.summaryRunID)
	}
	if summary.PIDStatus != sigilruntime.RunPIDStatusCurrent {
		return fmt.Errorf("expected pid_status %q, got %q", sigilruntime.RunPIDStatusCurrent, summary.PIDStatus)
	}
	if !summary.StopRequested {
		return fmt.Errorf("expected stop_requested=true")
	}
	if summary.FinalAnswerRef == nil || summary.AccountingRef == nil {
		return fmt.Errorf("expected final answer and accounting refs in summary")
	}
	return nil
}

func (w *harnessWorld) canonicalEventsAndRunLocalRefsExistForOneRun() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) aRunProjectionIsRequestedFromTheSelectedRunDirectory() error {
	state := w.inspectionState()
	projection, err := sigilruntime.LoadRunProjection(state.selectedRunsDir, state.summaryRunID)
	if err != nil {
		return err
	}
	state.derivedProjection = projection
	return nil
}

func (w *harnessWorld) theProjectionIsDerivedOnDemandWithoutPersistingASeparateReadModel() error {
	state := w.inspectionState()
	if state.derivedProjection.RunID != state.summaryRunID {
		return fmt.Errorf("expected projection run_id %q, got %q", state.summaryRunID, state.derivedProjection.RunID)
	}
	candidateFiles := []string{
		filepath.Join(state.selectedRunsDir, state.summaryRunID, "projection.json"),
		filepath.Join(state.selectedRunsDir, state.summaryRunID, "run-projection.json"),
		filepath.Join(state.selectedRunsDir, state.summaryRunID, "summary.json"),
	}
	for _, candidate := range candidateFiles {
		if _, err := os.Stat(filepath.Clean(candidate)); err == nil {
			return fmt.Errorf("unexpected derived read-model file %q", candidate)
		} else if !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("failed to inspect %q; %w", candidate, err)
		}
	}
	return nil
}

func (w *harnessWorld) processMetadataStatesVaryAcrossRuns() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) runSummariesAreDerivedFromTheSelectedRunDirectory() error {
	state := w.inspectionState()
	summaries, err := sigilruntime.ListRuns(state.selectedRunsDir)
	if err != nil {
		return err
	}
	state.derivedSummaries = summaries
	return nil
}

func (w *harnessWorld) pidStatusReportsCurrentMissingNotRunningOrStaleAccordingly() error {
	state := w.inspectionState()
	expected := map[string]string{
		state.summaryRunID:    sigilruntime.RunPIDStatusCurrent,
		state.missingRunID:    sigilruntime.RunPIDStatusMissing,
		state.notRunningRunID: sigilruntime.RunPIDStatusNotRunning,
		state.staleRunID:      sigilruntime.RunPIDStatusStale,
	}
	for runID, pidStatus := range expected {
		summary, ok := summaryByRunID(state.derivedSummaries, runID)
		if !ok {
			return fmt.Errorf("expected summary for run_id %q", runID)
		}
		if summary.PIDStatus != pidStatus {
			return fmt.Errorf("expected pid_status %q for run_id %q, got %q", pidStatus, runID, summary.PIDStatus)
		}
	}
	return nil
}

func (w *harnessWorld) stopRequestMetadataExistsForOneRun() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) aRunSummaryOrRunProjectionIsRequestedFromTheSelectedRunDirectory() error {
	state := w.inspectionState()
	events, err := sigilruntime.ReadRunEvents(state.selectedRunsDir, state.summaryRunID)
	if err != nil {
		return err
	}
	state.eventCountBefore = len(events)
	summaries, err := sigilruntime.ListRuns(state.selectedRunsDir)
	if err != nil {
		return err
	}
	projection, err := sigilruntime.LoadRunProjection(state.selectedRunsDir, state.summaryRunID)
	if err != nil {
		return err
	}
	events, err = sigilruntime.ReadRunEvents(state.selectedRunsDir, state.summaryRunID)
	if err != nil {
		return err
	}
	state.derivedSummaries = summaries
	state.derivedProjection = projection
	state.eventCountAfter = len(events)
	return nil
}

func (w *harnessWorld) stopRequestedIsSurfacedWithoutChangingCanonicalEventAuthority() error {
	state := w.inspectionState()
	summary, ok := summaryByRunID(state.derivedSummaries, state.summaryRunID)
	if !ok {
		return fmt.Errorf("expected summary for run_id %q", state.summaryRunID)
	}
	if !summary.StopRequested || !state.derivedProjection.StopRequested {
		return fmt.Errorf("expected stop_requested=true in summary and projection")
	}
	if state.eventCountBefore != state.eventCountAfter {
		return fmt.Errorf("expected canonical event authority to remain unchanged, got %d events before and %d after", state.eventCountBefore, state.eventCountAfter)
	}
	return nil
}

func (w *harnessWorld) canonicalTerminalRefsExistForOneRun() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) finalAnswerAndAccountingDataAreExposedAsRefsRatherThanInlineArtifactBodies() error {
	state := w.inspectionState()
	if state.derivedProjection.FinalAnswerRef == nil || state.derivedProjection.AccountingRef == nil {
		return fmt.Errorf("expected final answer and accounting refs in projection")
	}
	encoded, err := json.Marshal(state.derivedProjection)
	if err != nil {
		return fmt.Errorf("failed to marshal projection; %w", err)
	}
	if strings.Contains(string(encoded), `"final_answer":`) {
		return fmt.Errorf("expected projection to omit inline final_answer body, got %s", string(encoded))
	}
	return nil
}

func (w *harnessWorld) aTargetedRunHasMissingOrCorruptCanonicalEventStorage() error {
	return w.ensureProjectionFixtures()
}

func (w *harnessWorld) targetedRunQueriesAreRequestedFromTheSelectedRunDirectory() error {
	state := w.inspectionState()
	state.queryErrors = state.queryErrors[:0]
	_, err := sigilruntime.LoadRunProjection(state.selectedRunsDir, state.corruptRunID)
	state.queryErrors = append(state.queryErrors, err)
	_, err = sigilruntime.LoadRunProjection(state.selectedRunsDir, state.missingEventsRunID)
	state.queryErrors = append(state.queryErrors, err)
	return nil
}

func (w *harnessWorld) targetedRunQueriesFailForMissingOrCorruptCanonicalEventStorage() error {
	state := w.inspectionState()
	if len(state.queryErrors) != 2 {
		return fmt.Errorf("expected two targeted query errors, got %d", len(state.queryErrors))
	}
	for _, err := range state.queryErrors {
		if err == nil {
			return fmt.Errorf("expected targeted query failure")
		}
	}
	return nil
}

func (w *harnessWorld) inspectionState() *runInspectionState {
	if w.runInspection != nil {
		return w.runInspection
	}

	selectedRunsDir, err := sigilruntime.ResolveRunsBaseDir("./custom-runs")
	if err != nil {
		selectedRunsDir = filepath.Clean(filepath.Join(w.workingDir, "custom-runs"))
	}
	missingRunsDir, err := sigilruntime.ResolveRunsBaseDir("./missing-runs")
	if err != nil {
		missingRunsDir = filepath.Clean(filepath.Join(w.workingDir, "missing-runs"))
	}

	w.runInspection = &runInspectionState{
		selectedRunsArg: "./custom-runs",
		selectedRunsDir: selectedRunsDir,
		missingRunsArg:  "./missing-runs",
		missingRunsDir:  missingRunsDir,
	}
	return w.runInspection
}

func (w *harnessWorld) ensureSelectedRunListFixtures() error {
	state := w.inspectionState()
	if state.olderRunID != "" && state.targetedRunID != "" {
		return nil
	}

	olderRunID, err := w.createCompletedRun(state.selectedRunsDir, nil, nil)
	if err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)
	finalAnswerRef := "run-artifact://run/final-answer.json"
	accountingRef := "run-artifact://run/accounting.json"
	targetedRunID, err := w.createCompletedRun(state.selectedRunsDir, &finalAnswerRef, &accountingRef)
	if err != nil {
		return err
	}
	defaultRunID, err := w.createCompletedRun(sigilruntime.DefaultRunsBaseDir, nil, nil)
	if err != nil {
		return err
	}

	state.olderRunID = olderRunID
	state.targetedRunID = targetedRunID
	state.defaultRunID = defaultRunID
	return nil
}

func (w *harnessWorld) ensureProjectionFixtures() error {
	state := w.inspectionState()
	if state.summaryRunID != "" {
		return nil
	}

	finalAnswerRef := "run-artifact://run/final-answer.json"
	accountingRef := "run-artifact://run/accounting.json"
	summaryRunID, err := w.createCompletedRun(state.selectedRunsDir, &finalAnswerRef, &accountingRef)
	if err != nil {
		return err
	}
	if err := w.writeCurrentProcessMetadata(state.selectedRunsDir, summaryRunID); err != nil {
		return err
	}
	if err := sigilruntime.WriteStopRequestMetadata(state.selectedRunsDir, sigilruntime.StopRequestMetadata{
		RunID:       summaryRunID,
		RequestedAt: time.Now().UTC(),
		RequestedBy: sigilruntime.StopRequesterCLIRunStop,
		Signal:      sigilruntime.StopSignalSIGTERM,
	}); err != nil {
		return fmt.Errorf("failed to write stop-request metadata; %w", err)
	}

	missingRunID, err := w.createCompletedRun(state.selectedRunsDir, nil, nil)
	if err != nil {
		return err
	}
	notRunningRunID, err := w.createCompletedRun(state.selectedRunsDir, nil, nil)
	if err != nil {
		return err
	}
	if err := w.writeNotRunningProcessMetadata(state.selectedRunsDir, notRunningRunID); err != nil {
		return err
	}
	staleRunID, err := w.createCompletedRun(state.selectedRunsDir, nil, nil)
	if err != nil {
		return err
	}
	if err := w.writeStaleProcessMetadata(state.selectedRunsDir, staleRunID); err != nil {
		return err
	}

	corruptRunID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	corruptRunDir := filepath.Join(state.selectedRunsDir, corruptRunID.String())
	if err := os.MkdirAll(filepath.Clean(corruptRunDir), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(corruptRunDir, "events.jsonl"), []byte("{not-json}\n"), 0o644); err != nil {
		return err
	}

	missingEventsRunID, err := uuid.NewV7()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Join(state.selectedRunsDir, missingEventsRunID.String()), 0o755); err != nil {
		return err
	}

	state.summaryRunID = summaryRunID
	state.missingRunID = missingRunID
	state.notRunningRunID = notRunningRunID
	state.staleRunID = staleRunID
	state.corruptRunID = corruptRunID.String()
	state.missingEventsRunID = missingEventsRunID.String()
	return nil
}

func (w *harnessWorld) createCompletedRun(runsBaseDir string, finalAnswerRef *string, accountingRef *string) (string, error) {
	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		RunsBaseDir:  runsBaseDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     3,
	})
	if err != nil {
		return "", fmt.Errorf("failed to create lifecycle; %w", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	if err := lifecycle.StartExecution(); err != nil {
		return "", fmt.Errorf("failed to start lifecycle; %w", err)
	}
	if err := lifecycle.CompleteWithAccounting(finalAnswerRef, acceptanceUnavailableRollup(), accountingRef); err != nil {
		return "", fmt.Errorf("failed to complete lifecycle; %w", err)
	}
	return lifecycle.RunID(), nil
}

func acceptanceUnavailableRollup() accounting.Rollup {
	return accounting.BuildRollup(
		"openai",
		"gpt-5.1",
		"acceptance",
		accounting.UnavailableSummary("openai", "gpt-5.1", "acceptance"),
		accounting.ZeroSummary("openai", "gpt-5.1", "acceptance"),
	)
}

func (w *harnessWorld) writeCurrentProcessMetadata(runsBaseDir string, runID string) error {
	metadata, err := sigilruntime.CurrentProcessMetadata(sigilruntime.RunSourceCLIRunStart)
	if err != nil {
		return fmt.Errorf("failed to capture current process metadata; %w", err)
	}
	metadata.RunID = runID
	if err := sigilruntime.WriteProcessMetadata(runsBaseDir, metadata); err != nil {
		return fmt.Errorf("failed to write current process metadata; %w", err)
	}
	return nil
}

func (w *harnessWorld) writeNotRunningProcessMetadata(runsBaseDir string, runID string) error {
	cmd := exec.Command("sh", "-c", "sleep 0.1")
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("failed to start short-lived process; %w", err)
	}
	pid := cmd.Process.Pid
	if err := cmd.Wait(); err != nil {
		return fmt.Errorf("failed to wait for short-lived process; %w", err)
	}
	return sigilruntime.WriteProcessMetadata(runsBaseDir, sigilruntime.ProcessMetadata{
		RunID:      runID,
		PID:        pid,
		RecordedAt: time.Now().UTC(),
		StartedAt:  time.Now().UTC().Add(-time.Minute),
		Source:     sigilruntime.RunSourceCLIRunStart,
	})
}

func (w *harnessWorld) writeStaleProcessMetadata(runsBaseDir string, runID string) error {
	metadata, err := sigilruntime.CurrentProcessMetadata(sigilruntime.RunSourceCLIRunStart)
	if err != nil {
		return fmt.Errorf("failed to capture current process metadata; %w", err)
	}
	metadata.RunID = runID
	metadata.StartedAt = metadata.StartedAt.Add(time.Second)
	if err := sigilruntime.WriteProcessMetadata(runsBaseDir, metadata); err != nil {
		return fmt.Errorf("failed to write stale process metadata; %w", err)
	}
	return nil
}

func decodeRunStartResult(stdout string) (harness.RunResult, error) {
	var result harness.RunResult
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return harness.RunResult{}, fmt.Errorf("expected JSON run-start result, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &result); err != nil {
		return harness.RunResult{}, fmt.Errorf("failed to decode run-start result %q; %w", stdout, err)
	}
	return result, nil
}

func decodeRunSummaries(stdout string) ([]sigilruntime.RunSummary, error) {
	var summaries []sigilruntime.RunSummary
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, fmt.Errorf("expected JSON run summaries, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &summaries); err != nil {
		return nil, fmt.Errorf("failed to decode run summaries %q; %w", stdout, err)
	}
	return summaries, nil
}

func decodeRunSummary(stdout string) (sigilruntime.RunSummary, error) {
	var summary sigilruntime.RunSummary
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return sigilruntime.RunSummary{}, fmt.Errorf("expected JSON run summary, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &summary); err != nil {
		return sigilruntime.RunSummary{}, fmt.Errorf("failed to decode run summary %q; %w", stdout, err)
	}
	return summary, nil
}

func decodeRunProjection(stdout string) (sigilruntime.RunProjection, error) {
	var projection sigilruntime.RunProjection
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return sigilruntime.RunProjection{}, fmt.Errorf("expected JSON run projection, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &projection); err != nil {
		return sigilruntime.RunProjection{}, fmt.Errorf("failed to decode run projection %q; %w", stdout, err)
	}
	return projection, nil
}

func decodeRunEvents(stdout string) ([]sigilruntime.EventEnvelope, error) {
	var events []sigilruntime.EventEnvelope
	trimmed := strings.TrimSpace(stdout)
	if trimmed == "" {
		return nil, fmt.Errorf("expected JSON run events, got empty stdout")
	}
	if err := json.Unmarshal([]byte(trimmed), &events); err != nil {
		return nil, fmt.Errorf("failed to decode run events %q; %w", stdout, err)
	}
	return events, nil
}

func summaryByRunID(summaries []sigilruntime.RunSummary, runID string) (sigilruntime.RunSummary, bool) {
	for _, summary := range summaries {
		if summary.RunID == runID {
			return summary, true
		}
	}
	return sigilruntime.RunSummary{}, false
}
