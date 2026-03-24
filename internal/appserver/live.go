package appserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/control"
	"github.com/leefowlercu/sigil/internal/harness"
	"github.com/leefowlercu/sigil/internal/query"
	"github.com/leefowlercu/sigil/internal/runtime"
)

const (
	defaultSubscriptionPollInterval     = 500 * time.Millisecond
	ownedRunObserverBufferSize          = 32
	subscriptionSourcePersistedPolling  = "persisted_polling"
	subscriptionSourceInProcessObserver = "in_process_observer"
)

type connectionContextKey struct{}

type outboundWriter interface {
	writeText(message []byte) error
}

type ownedRunManager struct {
	runnerFactory func() *harness.Runner
	logger        *slog.Logger
	summaryDirty  func(runID string)

	mu             sync.Mutex
	runs           map[string]*harness.StartedRun
	runObservers   map[string]map[int]chan runtime.EventEnvelope
	nextObserverID int
}

type connectionSubscription struct {
	runID          string
	cancel         context.CancelFunc
	done           chan struct{}
	observerEvents <-chan runtime.EventEnvelope
	observerCancel func()
	deliverySource string
}

type connectionRunsSubscription struct {
	key            string
	cancel         context.CancelFunc
	done           chan struct{}
	changes        <-chan runSummaryChange
	deliverySource string
}

func withConnectionHandler(ctx context.Context, connection *connectionHandler) context.Context {
	return context.WithValue(ctx, connectionContextKey{}, connection)
}

func connectionFromContext(ctx context.Context) *connectionHandler {
	connection, _ := ctx.Value(connectionContextKey{}).(*connectionHandler)
	return connection
}

func newOwnedRunManager(runnerFactory func() *harness.Runner, summaryDirty func(runID string)) *ownedRunManager {
	return &ownedRunManager{
		runnerFactory: runnerFactory,
		logger:        slog.Default().With("component", "appserver.runs"),
		summaryDirty:  summaryDirty,
		runs:          map[string]*harness.StartedRun{},
		runObservers:  map[string]map[int]chan runtime.EventEnvelope{},
	}
}

func (m *ownedRunManager) Accept(input harness.RunInput) (*harness.StartedRun, error) {
	if m == nil || m.runnerFactory == nil {
		return nil, fmt.Errorf("owned run manager is not configured")
	}

	runner := m.runnerFactory()
	if runner == nil {
		return nil, fmt.Errorf("runner factory returned nil")
	}
	runner.AddEventObserver(m.observeEvent)

	started, err := runner.StartAsync(context.Background(), input)
	if err != nil {
		return nil, err
	}
	m.logger.Info("accepted app-server run",
		"run_id", started.RunID,
		"as_of_seq", started.AsOfSeq,
		"events_path", started.EventsPath,
	)
	return started, nil
}

func (m *ownedRunManager) Activate(started *harness.StartedRun) {
	if m == nil || started == nil {
		return
	}

	m.mu.Lock()
	m.runs[started.RunID] = started
	m.mu.Unlock()
	started.Launch()

	go func() {
		completion, ok := <-started.Done()
		if ok {
			if completion.Err != nil {
				m.logger.Warn("app-server run completed with error",
					"run_id", started.RunID,
					"error", completion.Err,
				)
			} else {
				m.logger.Info("app-server run completed",
					"run_id", started.RunID,
					"state", completion.Result.State,
				)
			}
		}

		m.mu.Lock()
		delete(m.runs, started.RunID)
		delete(m.runObservers, started.RunID)
		m.mu.Unlock()
	}()
}

func (m *ownedRunManager) RequestStop(runID string) bool {
	if m == nil {
		return false
	}

	m.mu.Lock()
	started := m.runs[runID]
	m.mu.Unlock()
	if started == nil {
		return false
	}

	started.Cancel(runtime.ErrExternalStopRequested)
	return true
}

func (m *ownedRunManager) SubscribeEvents(runID string) (<-chan runtime.EventEnvelope, func(), bool) {
	if m == nil {
		return nil, nil, false
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if m.runs[runID] == nil {
		return nil, nil, false
	}
	if m.runObservers == nil {
		m.runObservers = map[string]map[int]chan runtime.EventEnvelope{}
	}
	if m.runObservers[runID] == nil {
		m.runObservers[runID] = map[int]chan runtime.EventEnvelope{}
	}

	observerID := m.nextObserverID
	m.nextObserverID++
	events := make(chan runtime.EventEnvelope, ownedRunObserverBufferSize)
	m.runObservers[runID][observerID] = events

	return events, func() {
		m.removeObserver(runID, observerID)
	}, true
}

func (m *ownedRunManager) removeObserver(runID string, observerID int) {
	if m == nil {
		return
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	observers := m.runObservers[runID]
	if observers == nil {
		return
	}
	delete(observers, observerID)
	if len(observers) == 0 {
		delete(m.runObservers, runID)
	}
}

func (m *ownedRunManager) observeEvent(event runtime.EventEnvelope) {
	if m == nil {
		return
	}

	m.mu.Lock()
	observers := m.runObservers[event.RunID]
	channels := make([]chan runtime.EventEnvelope, 0, len(observers))
	for _, events := range observers {
		if events != nil {
			channels = append(channels, events)
		}
	}
	m.mu.Unlock()

	for _, events := range channels {
		select {
		case events <- event:
		default:
			m.logger.Warn("dropped in-process live event and left persisted catch-up active",
				"run_id", event.RunID,
				"seq", event.Seq,
				"type", event.Type,
				"delivery_source", subscriptionSourceInProcessObserver,
				"reason", "observer_backpressure",
			)
		}
	}
	if m.summaryDirty != nil && summaryRelevantEventType(event.Type) {
		m.summaryDirty(event.RunID)
	}
}

func summaryRelevantEventType(eventType runtime.EventType) bool {
	switch eventType {
	case runtime.EventTypeRunQueued,
		runtime.EventTypeRunRunning,
		runtime.EventTypeRunCompleted,
		runtime.EventTypeRunFailed,
		runtime.EventTypeRunInterrupted:
		return true
	default:
		return false
	}
}

func (s *Server) registerRunLiveHandlers() {
	s.dispatcher.Register("run/start", s.handleRunStart)
	s.dispatcher.Register("run/subscribe", s.handleRunSubscribe)
	s.dispatcher.Register("run/stop", s.handleRunStop)
	s.dispatcher.Register("run/unsubscribe", s.handleRunUnsubscribe)
	s.dispatcher.Register("runs/subscribe", s.handleRunsSubscribe)
	s.dispatcher.Register("runs/unsubscribe", s.handleRunsUnsubscribe)
}

func (s *Server) handleRunStart(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunStartParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}

	runConfigYAML := params.RunConfigYAML
	runConfig, err := configDecodeRunConfigYAML(strings.TrimSpace(runConfigYAML))
	if err != nil {
		appServerLogger().Warn("rejected app-server run start request",
			"domain_code", "invalid_run_config_yaml",
			"template_var_count", len(params.TemplateVars),
			"error", err,
		)
		return nil, protocol.NewError(protocol.CodeInvalidParams, "invalid run config yaml", "invalid_run_config_yaml", false, map[string]interface{}{
			"error": err.Error(),
		})
	}

	started, err := s.ownedRuns.Accept(harness.RunInput{
		RunConfig:              runConfig,
		TemplateVars:           cloneTemplateVars(params.TemplateVars),
		QueuedSource:           runtime.RunQueuedSourceAppServerStart,
		ProcessMetadataSource:  runtime.RunSourceAppServerStart,
		SubmittedRunConfigYAML: &runConfigYAML,
	})
	if err != nil {
		appServerLogger().Error("failed to accept app-server run",
			"domain_code", "run_start_failed",
			"template_var_count", len(params.TemplateVars),
			"error", err,
		)
		return nil, protocol.NewError(protocol.CodeInternalError, "failed to start run", "run_start_failed", true, nil)
	}

	snapshot, snapshotErr := query.BuildRunSubscriptionSnapshot(query.BuildRunSubscriptionSnapshotRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       started.RunID,
	})
	if snapshotErr != nil {
		started.Cancel(runtime.ErrExternalStopRequested)
		appServerLogger().Error("failed to build queued app-server run snapshot",
			"run_id", started.RunID,
			"error", snapshotErr,
		)
		return nil, protocol.NewError(protocol.CodeInternalError, "failed to start run", "run_start_failed", true, nil)
	}

	s.ownedRuns.Activate(started)
	appServerLogger().Info("app-server run accepted and activated",
		"run_id", started.RunID,
		"as_of_seq", snapshot.SnapshotAsOfSeq,
		"submitted_run_config_ref", optionalStringValue(started.SubmittedRunConfigRef),
	)

	return protocol.RunStartResult{
		RunID:   started.RunID,
		AsOfSeq: snapshot.SnapshotAsOfSeq,
		Payload: protocol.RunStartPayload{
			Run: mapRunProjection(snapshot.Run),
		},
	}, nil
}

func (s *Server) handleRunSubscribe(ctx context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	connection := connectionFromContext(ctx)
	if connection == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "subscription context is unavailable", "subscription_unavailable", true, nil)
	}

	params, errObject := decodeParams[protocol.RunSubscribeParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}

	pollInterval := time.Duration(s.config.AppServer.Subscriptions.PollIntervalMS) * time.Millisecond
	if params.AfterSeq == nil {
		snapshot, err := query.BuildRunSubscriptionSnapshot(query.BuildRunSubscriptionSnapshotRequest{
			RunsBaseDir: s.config.AppServer.RunDir,
			RunID:       params.RunID,
		})
		if err != nil {
			return nil, mapRunLiveQueryError(err)
		}

		deliverySource, replaceErr := connection.replaceRunSubscription(params.RunID, snapshot.SnapshotAsOfSeq, pollInterval)
		if replaceErr != nil {
			return nil, protocol.NewError(protocol.CodeInternalError, "failed to subscribe to run", "subscription_unavailable", true, nil)
		}
		appServerLogger().Info("attached app-server run subscription",
			"run_id", snapshot.RunID,
			"attach_mode", "fresh",
			"snapshot_as_of_seq", snapshot.SnapshotAsOfSeq,
			"terminal", snapshot.Terminal,
			"delivery_source", deliverySource,
		)

		return protocol.RunSubscribeResult{
			RunID:           snapshot.RunID,
			SnapshotAsOfSeq: snapshot.SnapshotAsOfSeq,
			Payload: protocol.RunSubscribePayload{
				Snapshot: &protocol.RunSubscriptionSnapshotView{
					Run: mapRunProjection(snapshot.Run),
					Tree: protocol.RunTreeReadPayload{
						RootNodeID: cloneStringPointer(snapshot.Tree.RootNodeID),
						Nodes:      mapRunTreeNodes(snapshot.Tree.Nodes),
					},
					ActiveNodeID: cloneStringPointer(snapshot.ActiveNodeID),
					ActiveStepID: cloneStringPointer(snapshot.ActiveStepID),
					Terminal:     snapshot.Terminal,
				},
				Terminal: snapshot.Terminal,
			},
		}, nil
	}

	replay, err := query.ReplayRunEvents(query.ReplayRunEventsRequest{
		RunsBaseDir: s.config.AppServer.RunDir,
		RunID:       params.RunID,
		AfterSeq:    *params.AfterSeq,
	})
	if err != nil {
		return nil, mapRunLiveQueryError(err)
	}

	deliverySource, replaceErr := connection.replaceRunSubscription(params.RunID, replay.AsOfSeq, pollInterval)
	if replaceErr != nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "failed to subscribe to run", "subscription_unavailable", true, nil)
	}
	appServerLogger().Info("attached app-server run subscription",
		"run_id", replay.RunID,
		"attach_mode", "resume",
		"after_seq", *params.AfterSeq,
		"snapshot_as_of_seq", replay.AsOfSeq,
		"replay_event_count", len(replay.Events),
		"terminal", replay.Terminal,
		"delivery_source", deliverySource,
	)

	return protocol.RunSubscribeResult{
		RunID:           replay.RunID,
		SnapshotAsOfSeq: replay.AsOfSeq,
		Payload: protocol.RunSubscribePayload{
			ReplayEvents: mapEventEnvelopes(replay.Events),
			Terminal:     replay.Terminal,
		},
	}, nil
}

func (s *Server) handleRunUnsubscribe(ctx context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	connection := connectionFromContext(ctx)
	if connection == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "subscription context is unavailable", "subscription_unavailable", true, nil)
	}

	params, errObject := decodeParams[protocol.RunUnsubscribeParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}

	unsubscribed := connection.removeRunSubscription(params.RunID)
	appServerLogger().Info("removed app-server run subscription",
		"run_id", params.RunID,
		"unsubscribed", unsubscribed,
	)
	return protocol.RunUnsubscribeResult{
		RunID: params.RunID,
		Payload: protocol.RunUnsubscribePayload{
			Unsubscribed: unsubscribed,
		},
	}, nil
}

func (s *Server) handleRunsSubscribe(ctx context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	connection := connectionFromContext(ctx)
	if connection == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "subscription context is unavailable", "subscription_unavailable", true, nil)
	}
	if s.summaryIndex == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "subscription context is unavailable", "subscription_unavailable", true, nil)
	}

	if _, errObject := decodeParams[protocol.RunsSubscribeParams](rawParams); errObject != nil {
		return nil, errObject
	}

	connection.removeRunsSubscription()
	items, revision, changes := s.summaryIndex.Subscribe(connection.runsSubscriptionKey())
	deliverySource, replaceErr := connection.replaceRunsSubscription(changes)
	if replaceErr != nil {
		s.summaryIndex.Unsubscribe(connection.runsSubscriptionKey())
		return nil, protocol.NewError(protocol.CodeInternalError, "failed to subscribe to runs", "subscription_unavailable", true, nil)
	}

	appServerLogger().Info("attached app-server runs subscription",
		"item_count", len(items),
		"revision", revision,
		"delivery_source", deliverySource,
	)

	return protocol.RunsSubscribeResult{
		Payload: protocol.RunsSubscribePayload{
			Items:    mapRunSummaries(items),
			Revision: revision,
		},
	}, nil
}

func (s *Server) handleRunsUnsubscribe(ctx context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	connection := connectionFromContext(ctx)
	if connection == nil {
		return nil, protocol.NewError(protocol.CodeInternalError, "subscription context is unavailable", "subscription_unavailable", true, nil)
	}

	if _, errObject := decodeParams[protocol.RunsUnsubscribeParams](rawParams); errObject != nil {
		return nil, errObject
	}

	unsubscribed := connection.removeRunsSubscription()
	appServerLogger().Info("removed app-server runs subscription",
		"unsubscribed", unsubscribed,
	)
	return protocol.RunsUnsubscribeResult{
		Payload: protocol.RunsUnsubscribePayload{
			Unsubscribed: unsubscribed,
		},
	}, nil
}

func (s *Server) handleRunStop(_ context.Context, rawParams json.RawMessage) (interface{}, *protocol.ErrorObject) {
	params, errObject := decodeParams[protocol.RunStopParams](rawParams)
	if errObject != nil {
		return nil, errObject
	}

	result, err := control.StopRun(control.StopRunRequest{
		RunsBaseDir:      s.config.AppServer.RunDir,
		RunID:            params.RunID,
		RequestedBy:      runtime.StopRequesterAppServerRunStop,
		Signal:           runtime.StopSignalSIGTERM,
		PollInterval:     control.DefaultStopPollInterval,
		ProcessExitGrace: control.DefaultStopProcessExitGrace,
		InProcessStop: func(runID string, _ runtime.ProcessMetadata) bool {
			return s.ownedRuns.RequestStop(runID)
		},
	})
	if err != nil {
		appServerLogger().Warn("failed to stop app-server run",
			"run_id", params.RunID,
			"error", err,
		)
		return nil, mapRunStopError(err)
	}

	appServerLogger().Info("completed app-server run stop request",
		"run_id", result.RunID,
		"stop_requested", result.StopRequested,
		"state", result.State,
		"events_path", result.EventsPath,
		"used_in_process_fast_path", result.UsedInProcessFastPath,
	)
	return protocol.RunStopResult{
		RunID: result.RunID,
		Payload: protocol.RunStopPayload{
			StopRequested: result.StopRequested,
			State:         result.State,
			EventsPath:    result.EventsPath,
		},
	}, nil
}

func (c *connectionHandler) replaceRunSubscription(runID string, lastDeliveredSeq int64, pollInterval time.Duration) (string, error) {
	if c == nil || strings.TrimSpace(runID) == "" {
		return "", fmt.Errorf("run subscription requires a run id")
	}
	if c.writer == nil {
		return "", fmt.Errorf("connection writer is not configured")
	}
	if pollInterval <= 0 {
		pollInterval = defaultSubscriptionPollInterval
	}

	observerEvents, observerCancel, observed := c.server.ownedRuns.SubscribeEvents(runID)
	deliverySource := subscriptionSourcePersistedPolling
	if observed {
		deliverySource = subscriptionSourceInProcessObserver
	}
	if observerCancel == nil {
		observerCancel = func() {}
	}

	subCtx, cancel := context.WithCancel(c.connectionCtx)
	subscription := &connectionSubscription{
		runID:          runID,
		cancel:         cancel,
		done:           make(chan struct{}),
		observerEvents: observerEvents,
		observerCancel: observerCancel,
		deliverySource: deliverySource,
	}

	c.mu.Lock()
	if c.subscriptions == nil {
		c.subscriptions = map[string]*connectionSubscription{}
	}
	existing := c.subscriptions[runID]
	c.subscriptions[runID] = subscription
	c.mu.Unlock()

	if existing != nil {
		existing.cancel()
		<-existing.done
	}

	go c.runSubscriptionLoop(subCtx, subscription, lastDeliveredSeq, pollInterval)
	return deliverySource, nil
}

func (c *connectionHandler) replaceRunsSubscription(changes <-chan runSummaryChange) (string, error) {
	if c == nil {
		return "", fmt.Errorf("runs subscription requires a connection")
	}
	if c.writer == nil {
		return "", fmt.Errorf("connection writer is not configured")
	}
	if changes == nil {
		return "", fmt.Errorf("runs subscription change stream is unavailable")
	}

	subCtx, cancel := context.WithCancel(c.connectionCtx)
	subscription := &connectionRunsSubscription{
		key:            c.runsSubscriptionKey(),
		cancel:         cancel,
		done:           make(chan struct{}),
		changes:        changes,
		deliverySource: runSummaryDeliverySource,
	}

	c.mu.Lock()
	existing := c.runsSubscription
	c.runsSubscription = subscription
	c.mu.Unlock()

	if existing != nil {
		existing.cancel()
		<-existing.done
	}

	go c.runsSubscriptionLoop(subCtx, subscription)
	return subscription.deliverySource, nil
}

func (c *connectionHandler) removeRunSubscription(runID string) bool {
	if c == nil || strings.TrimSpace(runID) == "" {
		return false
	}

	c.mu.Lock()
	subscription := c.subscriptions[runID]
	if subscription != nil {
		delete(c.subscriptions, runID)
	}
	c.mu.Unlock()

	if subscription == nil {
		return false
	}
	subscription.cancel()
	<-subscription.done
	return true
}

func (c *connectionHandler) removeRunsSubscription() bool {
	if c == nil {
		return false
	}

	c.mu.Lock()
	subscription := c.runsSubscription
	c.runsSubscription = nil
	c.mu.Unlock()

	if subscription == nil {
		return false
	}
	if c.server != nil && c.server.summaryIndex != nil {
		c.server.summaryIndex.Unsubscribe(subscription.key)
	}
	subscription.cancel()
	<-subscription.done
	return true
}

func (c *connectionHandler) closeSubscriptions() {
	if c == nil {
		return
	}

	c.mu.Lock()
	runsSubscription := c.runsSubscription
	c.runsSubscription = nil
	subscriptions := make([]*connectionSubscription, 0, len(c.subscriptions))
	for runID, subscription := range c.subscriptions {
		if subscription == nil {
			continue
		}
		delete(c.subscriptions, runID)
		subscriptions = append(subscriptions, subscription)
	}
	c.mu.Unlock()

	if runsSubscription != nil {
		if c.server != nil && c.server.summaryIndex != nil {
			c.server.summaryIndex.Unsubscribe(runsSubscription.key)
		}
		runsSubscription.cancel()
		<-runsSubscription.done
	}
	for _, subscription := range subscriptions {
		subscription.cancel()
		<-subscription.done
	}
}

func (c *connectionHandler) runSubscriptionLoop(
	ctx context.Context,
	subscription *connectionSubscription,
	lastDeliveredSeq int64,
	pollInterval time.Duration,
) {
	defer func() {
		if subscription.observerCancel != nil {
			subscription.observerCancel()
		}
		c.mu.Lock()
		if c.subscriptions[subscription.runID] == subscription {
			delete(c.subscriptions, subscription.runID)
		}
		c.mu.Unlock()
		close(subscription.done)
	}()

	logger := c.logger.With(
		"run_id", subscription.runID,
		"after_seq", lastDeliveredSeq,
		"poll_interval_ms", int(pollInterval/time.Millisecond),
		"delivery_source", subscription.deliverySource,
	)
	logger.Info("started run subscription")

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	deliveredSeq := lastDeliveredSeq
	observerEvents := subscription.observerEvents
	for {
		if err := c.replaySubscriptionEvents(subscription.runID, &deliveredSeq, logger); err != nil {
			if !errors.Is(err, query.ErrRunNotFound) {
				logger.Warn("failed to replay subscription events", "error", err)
			}
			return
		}

		select {
		case <-ctx.Done():
			logger.Info("stopped run subscription", "last_delivered_seq", deliveredSeq)
			return
		case event := <-observerEvents:
			if event.Seq <= deliveredSeq {
				continue
			}
			if err := c.sendRunEventNotifications(event); err != nil {
				logger.Warn("failed to deliver in-process live run event", "seq", event.Seq, "error", err)
				return
			}
			deliveredSeq = event.Seq
			logger.Debug("delivered in-process live run event", "seq", event.Seq)
		case <-ticker.C:
		}
	}
}

func (c *connectionHandler) runsSubscriptionLoop(
	ctx context.Context,
	subscription *connectionRunsSubscription,
) {
	defer func() {
		if c.server != nil && c.server.summaryIndex != nil {
			c.server.summaryIndex.Unsubscribe(subscription.key)
		}
		c.mu.Lock()
		if c.runsSubscription == subscription {
			c.runsSubscription = nil
		}
		c.mu.Unlock()
		close(subscription.done)
	}()

	logger := c.logger.With(
		"delivery_source", subscription.deliverySource,
	)
	logger.Info("started runs subscription")

	for {
		select {
		case <-ctx.Done():
			logger.Info("stopped runs subscription")
			return
		case change, ok := <-subscription.changes:
			if !ok {
				logger.Info("stopped runs subscription", "reason", "summary_index_closed")
				return
			}
			if err := c.sendRunsChangedNotification(change); err != nil {
				logger.Warn("failed to deliver runs changed notification",
					"revision", change.Revision,
					"change_kind", change.Kind,
					"error", err,
				)
				return
			}
		}
	}
}

func (c *connectionHandler) replaySubscriptionEvents(runID string, deliveredSeq *int64, logger *slog.Logger) error {
	replay, err := query.ReplayRunEvents(query.ReplayRunEventsRequest{
		RunsBaseDir: c.server.config.AppServer.RunDir,
		RunID:       runID,
		AfterSeq:    *deliveredSeq,
	})
	if err != nil {
		return err
	}

	replayedCount := 0
	var firstSeq int64
	var lastSeq int64
	for _, event := range replay.Events {
		if event.Seq <= *deliveredSeq {
			continue
		}
		if err := c.sendRunEventNotifications(event); err != nil {
			return err
		}
		if replayedCount == 0 {
			firstSeq = event.Seq
		}
		lastSeq = event.Seq
		replayedCount++
		*deliveredSeq = event.Seq
	}
	if replayedCount > 0 && logger != nil {
		logger.Debug("replayed canonical subscription events",
			"replay_event_count", replayedCount,
			"first_replay_seq", firstSeq,
			"last_replay_seq", lastSeq,
		)
	}
	return nil
}

func (c *connectionHandler) sendRunEventNotifications(event runtime.EventEnvelope) error {
	eventView, err := mapEventEnvelope(event)
	if err != nil {
		return err
	}
	if err := c.sendNotification("run/eventAppended", protocol.RunEventAppendedParams{
		RunID: event.RunID,
		Seq:   event.Seq,
		Payload: protocol.RunEventAppendedPayload{
			Event: eventView,
		},
	}); err != nil {
		return err
	}

	switch event.Type {
	case runtime.EventTypeRunRunning:
		runProjection, err := query.ReadRun(query.ReadRunRequest{
			RunsBaseDir: c.server.config.AppServer.RunDir,
			RunID:       event.RunID,
		})
		if err == nil {
			if err := c.sendNotification("run/started", protocol.RunStartedParams{
				RunID: event.RunID,
				Seq:   event.Seq,
				Payload: protocol.RunStartedPayload{
					Run: mapRunProjection(runProjection),
				},
			}); err != nil {
				return err
			}
		}
		return c.sendNotification("run/statusChanged", protocol.RunStatusChangedParams{
			RunID: event.RunID,
			Seq:   event.Seq,
			Payload: protocol.RunStatusChangedPayload{
				State:    string(runtime.RunStateRunning),
				Terminal: false,
			},
		})
	case runtime.EventTypeRunCompleted, runtime.EventTypeRunFailed, runtime.EventTypeRunInterrupted:
		state := terminalStateForEvent(event.Type)
		if err := c.sendNotification("run/statusChanged", protocol.RunStatusChangedParams{
			RunID: event.RunID,
			Seq:   event.Seq,
			Payload: protocol.RunStatusChangedPayload{
				State:    state,
				Terminal: true,
			},
		}); err != nil {
			return err
		}
		return c.sendNotification("run/completed", protocol.RunCompletedParams{
			RunID: event.RunID,
			Seq:   event.Seq,
			Payload: protocol.RunCompletedPayload{
				State:    state,
				Terminal: true,
			},
		})
	default:
		return nil
	}
}

func (c *connectionHandler) sendRunsChangedNotification(change runSummaryChange) error {
	params := protocol.RunsChangedParams{
		Revision: change.Revision,
		Payload: protocol.RunsChangedPayload{
			Kind: protocol.RunsChangedKind(change.Kind),
		},
	}

	switch change.Kind {
	case runSummaryChangeKindUpsert:
		runView := mapRunSummary(change.Run)
		params.Payload.Run = &runView
	case runSummaryChangeKindRemove:
		runID := change.RunID
		params.Payload.RunID = &runID
	case runSummaryChangeKindReset:
		// Reset only needs the revision and kind marker.
	}

	return c.sendNotification("runs/changed", params)
}

func (c *connectionHandler) sendNotification(method string, params interface{}) error {
	if c == nil || c.writer == nil {
		return fmt.Errorf("connection writer is not configured")
	}

	encoded, err := marshalNotification(method, params)
	if err != nil {
		return err
	}
	if err := c.writer.writeText(encoded); err != nil {
		if c.onWriteFailure != nil {
			c.onWriteFailure(err)
		}
		return err
	}
	if c.markOutbound != nil {
		c.markOutbound()
	}
	return nil
}

func (c *connectionHandler) runsSubscriptionKey() string {
	return fmt.Sprintf("connection:%p", c)
}

func mapRunLiveQueryError(err error) *protocol.ErrorObject {
	switch {
	case errors.Is(err, query.ErrInvalidIdentifier):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid identifier", "invalid_identifier", false, nil)
	case errors.Is(err, query.ErrRunNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "run not found", "run_not_found", false, nil)
	case errors.Is(err, query.ErrInvalidResumeCursor):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid resume cursor", "invalid_resume_cursor", false, nil)
	default:
		return protocol.NewError(protocol.CodeInternalError, "query failed", "query_failed", true, nil)
	}
}

func mapRunStopError(err error) *protocol.ErrorObject {
	switch {
	case errors.Is(err, control.ErrInvalidRunID):
		return protocol.NewError(protocol.CodeInvalidParams, "invalid identifier", "invalid_identifier", false, nil)
	case errors.Is(err, control.ErrRunNotFound):
		return protocol.NewError(protocol.CodeInvalidParams, "run not found", "run_not_found", false, nil)
	case errors.Is(err, control.ErrProcessMetadataUnavailable):
		return protocol.NewError(protocol.CodeInvalidParams, "live process metadata is unavailable", "process_metadata_unavailable", false, nil)
	case errors.Is(err, control.ErrStaleProcessMetadata):
		return protocol.NewError(protocol.CodeInvalidParams, "live process metadata is stale", "stale_process_metadata", false, nil)
	case errors.Is(err, control.ErrCurrentProcessFastPathRequired):
		return protocol.NewError(protocol.CodeInternalError, "failed to stop run", "stop_unavailable", true, nil)
	case errors.Is(err, os.ErrNotExist):
		return protocol.NewError(protocol.CodeInvalidParams, "run not found", "run_not_found", false, nil)
	default:
		return protocol.NewError(protocol.CodeInternalError, "failed to stop run", "run_stop_failed", true, nil)
	}
}

func mapRunTreeNodes(values []query.RunTreeNode) []protocol.RunTreeNodeView {
	mapped := make([]protocol.RunTreeNodeView, 0, len(values))
	for _, value := range values {
		mapped = append(mapped, mapRunTreeNode(value))
	}
	return mapped
}

func mapEventEnvelopes(values []runtime.EventEnvelope) []protocol.EventEnvelopeView {
	mapped := make([]protocol.EventEnvelopeView, 0, len(values))
	for _, value := range values {
		mappedValue, err := mapEventEnvelope(value)
		if err != nil {
			continue
		}
		mapped = append(mapped, mappedValue)
	}
	return mapped
}

func terminalStateForEvent(eventType runtime.EventType) string {
	switch eventType {
	case runtime.EventTypeRunCompleted:
		return string(runtime.RunStateCompleted)
	case runtime.EventTypeRunInterrupted:
		return string(runtime.RunStateInterrupted)
	default:
		return string(runtime.RunStateFailed)
	}
}

func configDecodeRunConfigYAML(content string) (config.RunConfig, error) {
	return config.DecodeRunConfigYAML(content)
}

func cloneTemplateVars(values map[string]string) map[string]string {
	if len(values) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(values))
	for key, value := range values {
		cloned[key] = value
	}
	return cloned
}

func optionalStringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
