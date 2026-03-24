package appserver

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/leefowlercu/sigil/internal/query"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	runSummaryEventsFileName    = "events.jsonl"
	runSummarySubscriberBufSize = 32
	runSummaryChangeKindUpsert  = "upsert"
	runSummaryChangeKindRemove  = "remove"
	runSummaryChangeKindReset   = "reset"
	runSummaryDeliverySource    = "summary_index"
	runSummaryPollTriggerBuffer = 1
	runSummaryReconcileBufSize  = 1
)

type runSummaryChange struct {
	Revision int64
	Kind     string
	RunID    string
	Run      runtime.RunSummary
}

type runSummaryFileFingerprint struct {
	Exists          bool
	Size            int64
	ModTimeUnixNano int64
}

type runSummaryFingerprint struct {
	Events      runSummaryFileFingerprint
	Process     runSummaryFileFingerprint
	StopRequest runSummaryFileFingerprint
}

// RunSummaryIndex maintains an authoritative in-memory summary view for the
// configured run corpus and broadcasts targeted collection deltas to subscribers.
type RunSummaryIndex struct {
	runsBaseDir  string
	pollInterval time.Duration
	logger       *slog.Logger

	startOnce sync.Once
	startErr  error

	mu               sync.RWMutex
	resolvedRunsDir  string
	revision         int64
	summariesByRunID map[string]runtime.RunSummary
	orderedRunIDs    []string
	fingerprints     map[string]runSummaryFingerprint
	dirtyRunIDs      map[string]struct{}
	subscribers      map[string]chan runSummaryChange

	scanTriggerCh   chan struct{}
	reconcileWakeCh chan struct{}
}

func newRunSummaryIndex(runsBaseDir string, pollInterval time.Duration, logger *slog.Logger) *RunSummaryIndex {
	if pollInterval <= 0 {
		pollInterval = defaultSubscriptionPollInterval
	}
	if logger == nil {
		logger = slog.Default()
	}

	return &RunSummaryIndex{
		runsBaseDir:      runsBaseDir,
		pollInterval:     pollInterval,
		logger:           logger.With("component", "appserver.run_summary_index"),
		summariesByRunID: map[string]runtime.RunSummary{},
		orderedRunIDs:    []string{},
		fingerprints:     map[string]runSummaryFingerprint{},
		dirtyRunIDs:      map[string]struct{}{},
		subscribers:      map[string]chan runSummaryChange{},
		scanTriggerCh:    make(chan struct{}, runSummaryPollTriggerBuffer),
		reconcileWakeCh:  make(chan struct{}, runSummaryReconcileBufSize),
	}
}

func (i *RunSummaryIndex) Start(ctx context.Context) error {
	if i == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}

	i.startOnce.Do(func() {
		i.startErr = i.start(ctx)
	})
	return i.startErr
}

func (i *RunSummaryIndex) start(ctx context.Context) error {
	resolvedRunsDir, err := runtime.ResolveRunsBaseDir(i.runsBaseDir)
	if err != nil {
		return err
	}

	i.mu.Lock()
	i.resolvedRunsDir = resolvedRunsDir
	i.mu.Unlock()

	if err := i.scanAndMarkDirty(); err != nil {
		return err
	}
	i.reconcileDirtyRuns()

	go i.runPollLoop(ctx)
	go i.runReconcileLoop(ctx)
	go func() {
		<-ctx.Done()
		i.closeSubscribers()
	}()
	return nil
}

func (i *RunSummaryIndex) Snapshot() ([]runtime.RunSummary, int64) {
	if i == nil {
		return nil, 0
	}

	i.mu.RLock()
	defer i.mu.RUnlock()

	items := make([]runtime.RunSummary, 0, len(i.orderedRunIDs))
	for _, runID := range i.orderedRunIDs {
		summary, ok := i.summariesByRunID[runID]
		if !ok {
			continue
		}
		items = append(items, cloneRuntimeRunSummary(summary))
	}
	return items, i.revision
}

func (i *RunSummaryIndex) ListPage(limit int, cursor *string) (query.ListRunsPageResult, error) {
	items, _ := i.Snapshot()
	return query.PaginateRunSummaries(query.PaginateRunSummariesRequest{
		Summaries: items,
		Limit:     limit,
		Cursor:    cursor,
	})
}

func (i *RunSummaryIndex) Subscribe(connectionKey string) ([]runtime.RunSummary, int64, <-chan runSummaryChange) {
	if i == nil || strings.TrimSpace(connectionKey) == "" {
		return nil, 0, nil
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if existing := i.subscribers[connectionKey]; existing != nil {
		close(existing)
	}
	changes := make(chan runSummaryChange, runSummarySubscriberBufSize)
	i.subscribers[connectionKey] = changes

	items := make([]runtime.RunSummary, 0, len(i.orderedRunIDs))
	for _, runID := range i.orderedRunIDs {
		summary, ok := i.summariesByRunID[runID]
		if !ok {
			continue
		}
		items = append(items, cloneRuntimeRunSummary(summary))
	}
	return items, i.revision, changes
}

func (i *RunSummaryIndex) Unsubscribe(connectionKey string) {
	if i == nil || strings.TrimSpace(connectionKey) == "" {
		return
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	if existing := i.subscribers[connectionKey]; existing != nil {
		delete(i.subscribers, connectionKey)
		close(existing)
	}
}

func (i *RunSummaryIndex) MarkDirty(runID string) {
	if i == nil {
		return
	}

	trimmed := strings.TrimSpace(runID)
	if trimmed == "" {
		return
	}

	i.mu.Lock()
	i.dirtyRunIDs[trimmed] = struct{}{}
	i.mu.Unlock()
	i.notifyReconciler()
}

func (i *RunSummaryIndex) MarkCorpusDirty() {
	if i == nil {
		return
	}

	select {
	case i.scanTriggerCh <- struct{}{}:
	default:
	}
}

func (i *RunSummaryIndex) runPollLoop(ctx context.Context) {
	ticker := time.NewTicker(i.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		case <-i.scanTriggerCh:
		}

		if err := i.scanAndMarkDirty(); err != nil {
			i.logger.Warn("failed to scan run summary corpus", "error", err)
		}
	}
}

func (i *RunSummaryIndex) runReconcileLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case <-i.reconcileWakeCh:
			i.reconcileDirtyRuns()
		}
	}
}

func (i *RunSummaryIndex) scanAndMarkDirty() error {
	resolvedRunsDir := i.currentRunsDir()
	scannedFingerprints, err := i.scanFingerprints(resolvedRunsDir)
	if err != nil {
		return err
	}

	i.mu.Lock()
	defer i.mu.Unlock()

	dirtyCount := 0
	for runID, fingerprint := range scannedFingerprints {
		if previous, ok := i.fingerprints[runID]; !ok || previous != fingerprint {
			if _, alreadyDirty := i.dirtyRunIDs[runID]; !alreadyDirty {
				dirtyCount++
			}
			i.dirtyRunIDs[runID] = struct{}{}
		}
	}
	for runID := range i.fingerprints {
		if _, ok := scannedFingerprints[runID]; ok {
			continue
		}
		if _, alreadyDirty := i.dirtyRunIDs[runID]; !alreadyDirty {
			dirtyCount++
		}
		i.dirtyRunIDs[runID] = struct{}{}
	}
	for runID, summary := range i.summariesByRunID {
		if !shouldRefreshRunSummary(summary) {
			continue
		}
		if _, alreadyDirty := i.dirtyRunIDs[runID]; alreadyDirty {
			continue
		}
		i.dirtyRunIDs[runID] = struct{}{}
		dirtyCount++
	}

	i.fingerprints = scannedFingerprints
	if dirtyCount > 0 {
		i.notifyReconciler()
	}
	return nil
}

func (i *RunSummaryIndex) scanFingerprints(resolvedRunsDir string) (map[string]runSummaryFingerprint, error) {
	entries, err := os.ReadDir(resolvedRunsDir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return map[string]runSummaryFingerprint{}, nil
		}
		return nil, err
	}

	fingerprints := make(map[string]runSummaryFingerprint, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		runID := entry.Name()
		if !isUUIDv7(runID) {
			continue
		}

		fingerprint, err := i.fingerprintForRun(resolvedRunsDir, runID)
		if err != nil {
			return nil, err
		}
		fingerprints[runID] = fingerprint
	}
	return fingerprints, nil
}

func (i *RunSummaryIndex) fingerprintForRun(resolvedRunsDir string, runID string) (runSummaryFingerprint, error) {
	processPath, err := runtime.ProcessMetadataPath(resolvedRunsDir, runID)
	if err != nil {
		return runSummaryFingerprint{}, err
	}
	stopRequestPath, err := runtime.StopRequestPath(resolvedRunsDir, runID)
	if err != nil {
		return runSummaryFingerprint{}, err
	}

	eventsFingerprint, err := statRunSummaryFile(filepath.Join(resolvedRunsDir, runID, runSummaryEventsFileName))
	if err != nil {
		return runSummaryFingerprint{}, err
	}
	processFingerprint, err := statRunSummaryFile(processPath)
	if err != nil {
		return runSummaryFingerprint{}, err
	}
	stopRequestFingerprint, err := statRunSummaryFile(stopRequestPath)
	if err != nil {
		return runSummaryFingerprint{}, err
	}

	return runSummaryFingerprint{
		Events:      eventsFingerprint,
		Process:     processFingerprint,
		StopRequest: stopRequestFingerprint,
	}, nil
}

func (i *RunSummaryIndex) reconcileDirtyRuns() {
	for {
		runIDs := i.takeDirtyRunIDs()
		if len(runIDs) == 0 {
			return
		}
		for _, runID := range runIDs {
			i.reconcileRun(runID)
		}
	}
}

func (i *RunSummaryIndex) takeDirtyRunIDs() []string {
	i.mu.Lock()
	defer i.mu.Unlock()

	if len(i.dirtyRunIDs) == 0 {
		return nil
	}

	runIDs := make([]string, 0, len(i.dirtyRunIDs))
	for runID := range i.dirtyRunIDs {
		runIDs = append(runIDs, runID)
		delete(i.dirtyRunIDs, runID)
	}
	sort.Strings(runIDs)
	return runIDs
}

func (i *RunSummaryIndex) reconcileRun(runID string) {
	summary, exists, err := i.readRunSummary(runID)
	if err != nil {
		i.logger.Warn("failed to refresh run summary", "run_id", runID, "error", err)
		return
	}

	var change *runSummaryChange

	i.mu.Lock()
	previous, hadPrevious := i.summariesByRunID[runID]
	switch {
	case !exists:
		if hadPrevious {
			delete(i.summariesByRunID, runID)
			i.rebuildOrderedRunIDsLocked()
			i.revision++
			change = &runSummaryChange{
				Revision: i.revision,
				Kind:     runSummaryChangeKindRemove,
				RunID:    runID,
			}
		}
	case hadPrevious && reflect.DeepEqual(previous, summary):
		// No-op; keep the current revision.
	default:
		i.summariesByRunID[runID] = cloneRuntimeRunSummary(summary)
		i.rebuildOrderedRunIDsLocked()
		i.revision++
		change = &runSummaryChange{
			Revision: i.revision,
			Kind:     runSummaryChangeKindUpsert,
			RunID:    runID,
			Run:      cloneRuntimeRunSummary(summary),
		}
	}
	i.mu.Unlock()

	if change != nil {
		i.broadcastChange(*change)
	}
}

func (i *RunSummaryIndex) readRunSummary(runID string) (runtime.RunSummary, bool, error) {
	summary, err := query.ReadRunSummary(query.ReadRunSummaryRequest{
		RunsBaseDir: i.currentRunsDir(),
		RunID:       runID,
	})
	if err == nil {
		return summary, true, nil
	}
	if errors.Is(err, query.ErrRunNotFound) {
		return runtime.RunSummary{}, false, nil
	}
	if errors.Is(err, query.ErrInvalidIdentifier) {
		return runtime.RunSummary{}, false, err
	}

	return runtime.RunSummary{
		RunID:      runID,
		State:      runtime.RunStateUnknown,
		EventsPath: filepath.Join(i.currentRunsDir(), runID, runSummaryEventsFileName),
		PIDStatus:  runtime.RunPIDStatusMissing,
		Error:      err.Error(),
	}, true, nil
}

func (i *RunSummaryIndex) rebuildOrderedRunIDsLocked() {
	summaries := make([]runtime.RunSummary, 0, len(i.summariesByRunID))
	for _, summary := range i.summariesByRunID {
		summaries = append(summaries, cloneRuntimeRunSummary(summary))
	}
	runtime.SortRunSummaries(summaries)

	i.orderedRunIDs = make([]string, 0, len(summaries))
	for _, summary := range summaries {
		i.orderedRunIDs = append(i.orderedRunIDs, summary.RunID)
	}
}

func (i *RunSummaryIndex) broadcastChange(change runSummaryChange) {
	i.mu.RLock()
	subscribers := make(map[string]chan runSummaryChange, len(i.subscribers))
	for connectionKey, changes := range i.subscribers {
		subscribers[connectionKey] = changes
	}
	i.mu.RUnlock()

	for connectionKey, changes := range subscribers {
		select {
		case changes <- change:
		default:
			i.logger.Warn("removed slow run summary subscriber",
				"connection_key", connectionKey,
				"revision", change.Revision,
				"change_kind", change.Kind,
			)
			i.Unsubscribe(connectionKey)
		}
	}
}

func (i *RunSummaryIndex) notifyReconciler() {
	select {
	case i.reconcileWakeCh <- struct{}{}:
	default:
	}
}

func (i *RunSummaryIndex) closeSubscribers() {
	i.mu.Lock()
	defer i.mu.Unlock()

	for connectionKey, changes := range i.subscribers {
		delete(i.subscribers, connectionKey)
		close(changes)
	}
}

func (i *RunSummaryIndex) currentRunsDir() string {
	i.mu.RLock()
	defer i.mu.RUnlock()

	if strings.TrimSpace(i.resolvedRunsDir) != "" {
		return i.resolvedRunsDir
	}
	return i.runsBaseDir
}

func cloneRuntimeRunSummary(value runtime.RunSummary) runtime.RunSummary {
	return runtime.RunSummary{
		RunID:          value.RunID,
		Name:           value.Name,
		State:          value.State,
		Source:         value.Source,
		QueuedAt:       cloneTimePointer(value.QueuedAt),
		StartedAt:      cloneTimePointer(value.StartedAt),
		TerminalAt:     cloneTimePointer(value.TerminalAt),
		EventsPath:     value.EventsPath,
		PIDStatus:      value.PIDStatus,
		StopRequested:  value.StopRequested,
		FinalAnswerRef: cloneStringPointer(value.FinalAnswerRef),
		AccountingRef:  cloneStringPointer(value.AccountingRef),
		Error:          value.Error,
	}
}

func shouldRefreshRunSummary(summary runtime.RunSummary) bool {
	if summary.PIDStatus == runtime.RunPIDStatusCurrent {
		return true
	}
	return !runtime.IsTerminalRunState(runtime.RunState(summary.State))
}

func statRunSummaryFile(path string) (runSummaryFileFingerprint, error) {
	info, err := os.Stat(filepath.Clean(path))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return runSummaryFileFingerprint{}, nil
		}
		return runSummaryFileFingerprint{}, err
	}

	return runSummaryFileFingerprint{
		Exists:          true,
		Size:            info.Size(),
		ModTimeUnixNano: info.ModTime().UTC().UnixNano(),
	}, nil
}

func isUUIDv7(raw string) bool {
	parsed, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil {
		return false
	}
	return parsed.Version() == uuid.Version(7)
}
