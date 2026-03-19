package steps

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/cucumber/godog"
	"github.com/gorilla/websocket"
	"github.com/leefowlercu/sigil/cmd"
	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/config"
	sigilruntime "github.com/leefowlercu/sigil/internal/runtime"
)

type appServerAcceptanceState struct {
	cancel           context.CancelFunc
	stdinWriter      *io.PipeWriter
	stdoutReader     *bufio.Reader
	stdoutPipe       *io.PipeReader
	websocketURL     string
	websocketOrigin  string
	readyURL         string
	liveURL          string
	websocketConn    *websocket.Conn
	websocketProxy   *appServerWebSocketProxy
	done             chan error
	stderr           bytes.Buffer
	lastResponse     map[string]any
	lastNotification map[string]any
	nextRequestID    int
	fixtures         *appServerReadFixtures
	liveLifecycle    *sigilruntime.Lifecycle
}

type appServerReadFixtures struct {
	PrimaryRunID   string
	SecondaryRunID string
	LiveRunID      string
	RootNodeID     string
	ChildNodeID    string
	StepID         string
	ActionRef      string
}

type appServerWebSocketProxy struct {
	listener           net.Listener
	targetAddr         string
	clientURL          string
	done               chan error
	mu                 sync.RWMutex
	serverFramesPaused bool
}

func registerAppServerSteps(ctx *godog.ScenarioContext, world *harnessWorld) {
	ctx.Step(`^the app-server stdio transport is running$`, world.theAppServerStdioTransportIsRunning)
	ctx.Step(`^the app-server websocket transport is running$`, world.theAppServerWebSocketTransportIsRunning)
	ctx.Step(`^the app-server receives JSON-RPC line:$`, world.theAppServerReceivesJSONRPCLine)
	ctx.Step(`^the app-server receives JSON-RPC notification:$`, world.theAppServerReceivesJSONRPCNotification)
	ctx.Step(`^the app-server response error code is (-?\d+)$`, world.theAppServerResponseErrorCodeIs)
	ctx.Step(`^the app-server response data\.code is "([^"]*)"$`, world.theAppServerResponseDataCodeIs)
	ctx.Step(`^the app-server response result field "([^"]*)" is "([^"]*)"$`, world.theAppServerResponseResultFieldIs)
	ctx.Step(`^persisted app-server run fixtures exist in the configured run directory$`, world.persistedAppServerRunFixturesExistInTheConfiguredRunDirectory)
	ctx.Step(`^a queued live app-server fixture exists in the configured run directory$`, world.aQueuedLiveAppServerFixtureExistsInTheConfiguredRunDirectory)
	ctx.Step(`^a local CLI run is actively executing for app-server stop control$`, world.aLocalCLIRunIsActivelyExecutingForAppServerStopControl)
	ctx.Step(`^the app-server requests run/list with limit (\d+)$`, world.theAppServerRequestsRunListWithLimit)
	ctx.Step(`^the app-server requests "([^"]*)" for fixture run "([^"]*)"$`, world.theAppServerRequestsForFixtureRun)
	ctx.Step(`^the app-server requests run/start with inline YAML:$`, world.theAppServerRequestsRunStartWithInlineYAML)
	ctx.Step(`^the app-server requests run/subscribe for fixture run "([^"]*)"$`, world.theAppServerRequestsRunSubscribeForFixtureRun)
	ctx.Step(`^the app-server requests run/subscribe for fixture run "([^"]*)" after seq (\d+)$`, world.theAppServerRequestsRunSubscribeForFixtureRunAfterSeq)
	ctx.Step(`^the app-server requests run/stop for the active run$`, world.theAppServerRequestsRunStopForTheActiveRun)
	ctx.Step(`^the app-server requests run/unsubscribe for fixture run "([^"]*)"$`, world.theAppServerRequestsRunUnsubscribeForFixtureRun)
	ctx.Step(`^the app-server requests run/node/read for fixture run "([^"]*)" node "([^"]*)"$`, world.theAppServerRequestsRunNodeReadForFixtureRunNode)
	ctx.Step(`^the app-server requests run/step/read for fixture run "([^"]*)" node "([^"]*)" step "([^"]*)"$`, world.theAppServerRequestsRunStepReadForFixtureRunNodeStep)
	ctx.Step(`^the app-server requests run/artifact/read for fixture run "([^"]*)" artifact "([^"]*)"$`, world.theAppServerRequestsRunArtifactReadForFixtureRunArtifact)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" equals "([^"]*)"$`, world.theAppServerResponseJSONPointerEquals)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" equals boolean (true|false)$`, world.theAppServerResponseJSONPointerEqualsBoolean)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" equals integer (\d+)$`, world.theAppServerResponseJSONPointerEqualsInteger)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" equals fixture "([^"]*)"$`, world.theAppServerResponseJSONPointerEqualsFixture)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" has length (\d+)$`, world.theAppServerResponseJSONPointerHasLength)
	ctx.Step(`^the app-server response JSON pointer "([^"]*)" is a positive integer$`, world.theAppServerResponseJSONPointerIsAPositiveInteger)
	ctx.Step(`^the websocket app-server client connects with origin "([^"]*)"$`, world.theWebSocketAppServerClientConnectsWithOrigin)
	ctx.Step(`^the websocket app-server client connects without an origin header$`, world.theWebSocketAppServerClientConnectsWithoutAnOriginHeader)
	ctx.Step(`^the websocket app-server client routes through a controllable proxy$`, world.theWebSocketAppServerClientRoutesThroughAControllableProxy)
	ctx.Step(`^the websocket app-server client sends JSON-RPC message:$`, world.theWebSocketAppServerClientSendsJSONRPCMessage)
	ctx.Step(`^the websocket app-server client sends JSON-RPC notification:$`, world.theWebSocketAppServerClientSendsJSONRPCNotification)
	ctx.Step(`^the stdio app-server client receives JSON-RPC notification method "([^"]*)"$`, world.theStdioAppServerClientReceivesJSONRPCNotificationMethod)
	ctx.Step(`^the stdio app-server client receives (\d+) unique "([^"]*)" notification seq values$`, world.theStdioAppServerClientReceivesUniqueNotificationSeqValues)
	ctx.Step(`^the websocket app-server proxy stops forwarding server frames$`, world.theWebSocketAppServerProxyStopsForwardingServerFrames)
	ctx.Step(`^the websocket app-server proxy resumes forwarding server frames$`, world.theWebSocketAppServerProxyResumesForwardingServerFrames)
	ctx.Step(`^the websocket app-server client detects a missed heartbeat window$`, world.theWebSocketAppServerClientDetectsAMissedHeartbeatWindow)
	ctx.Step(`^the websocket app-server client reconnects$`, world.theWebSocketAppServerClientReconnects)
	ctx.Step(`^the websocket app-server client requests run/subscribe for fixture run "([^"]*)"$`, world.theWebSocketAppServerClientRequestsRunSubscribeForFixtureRun)
	ctx.Step(`^the websocket app-server client requests run/subscribe for fixture run "([^"]*)" after seq (\d+)$`, world.theWebSocketAppServerClientRequestsRunSubscribeForFixtureRunAfterSeq)
	ctx.Step(`^the websocket app-server client receives (\d+) unique "([^"]*)" notification seq values greater than (\d+)$`, world.theWebSocketAppServerClientReceivesUniqueNotificationSeqValuesGreaterThan)
	ctx.Step(`^the websocket app-server client receives JSON-RPC notification method "([^"]*)"$`, world.theWebSocketAppServerClientReceivesJSONRPCNotificationMethod)
	ctx.Step(`^the queued live app-server fixture starts execution$`, world.theQueuedLiveAppServerFixtureStartsExecution)
	ctx.Step(`^the active run stop-request metadata requested_by is "([^"]*)"$`, world.theActiveRunStopRequestMetadataRequestedByIs)
	ctx.Step(`^the app-server ready and live endpoints return success$`, world.theAppServerReadyAndLiveEndpointsReturnSuccess)
	ctx.Step(`^TypeScript app-server bindings are generated twice deterministically$`, world.typeScriptAppServerBindingsAreGeneratedTwiceDeterministically)
	ctx.Step(`^JSON Schema app-server bundles are generated twice deterministically$`, world.jsonSchemaAppServerBundlesAreGeneratedTwiceDeterministically)
}

func (w *harnessWorld) theAppServerStdioTransportIsRunning() error {
	if w.appServer != nil && w.appServer.stdinWriter != nil {
		return nil
	}
	var fixtures *appServerReadFixtures
	var liveLifecycle *sigilruntime.Lifecycle
	if w.appServer != nil {
		fixtures = w.appServer.fixtures
		liveLifecycle = w.appServer.liveLifecycle
	}

	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()
	rootCmd := cmd.NewRootCmd()
	rootCmd.SetArgs([]string{"app-server", "serve", "--listen", "stdio://"})
	rootCmd.SetIn(inputReader)
	rootCmd.SetOut(outputWriter)
	rootCmd.SetErr(&bytes.Buffer{})

	state := &appServerAcceptanceState{
		stdinWriter:   inputWriter,
		stdoutReader:  bufio.NewReader(outputReader),
		stdoutPipe:    outputReader,
		done:          make(chan error, 1),
		nextRequestID: 2,
		fixtures:      fixtures,
		liveLifecycle: liveLifecycle,
	}
	rootCmd.SetErr(&state.stderr)

	go func() {
		state.done <- rootCmd.Execute()
		_ = outputWriter.Close()
	}()

	w.appServer = state
	return nil
}

func (w *harnessWorld) theAppServerWebSocketTransportIsRunning() error {
	if w.appServer != nil && w.appServer.websocketURL != "" {
		return nil
	}
	var fixtures *appServerReadFixtures
	var liveLifecycle *sigilruntime.Lifecycle
	if w.appServer != nil {
		fixtures = w.appServer.fixtures
		liveLifecycle = w.appServer.liveLifecycle
	}

	cfg, err := loadAcceptanceAppConfig()
	if err != nil {
		return err
	}

	port, err := reserveLoopbackPort()
	if err != nil {
		return err
	}

	rootCmd := cmd.NewRootCmd()
	commandCtx, cancel := context.WithCancel(context.Background())
	rootCmd.SetContext(commandCtx)
	rootCmd.SetArgs([]string{"app-server", "serve", "--listen", "ws://127.0.0.1:" + port})
	rootCmd.SetIn(bytes.NewBuffer(nil))
	rootCmd.SetOut(&bytes.Buffer{})

	state := &appServerAcceptanceState{
		cancel:        cancel,
		websocketURL:  fmt.Sprintf("ws://127.0.0.1:%s%s", port, cfg.AppServer.WebSocket.Path),
		readyURL:      fmt.Sprintf("http://127.0.0.1:%s%s", port, cfg.AppServer.Health.ReadyPath),
		liveURL:       fmt.Sprintf("http://127.0.0.1:%s%s", port, cfg.AppServer.Health.LivePath),
		done:          make(chan error, 1),
		nextRequestID: 2,
		fixtures:      fixtures,
		liveLifecycle: liveLifecycle,
	}
	rootCmd.SetErr(&state.stderr)

	go func() {
		state.done <- rootCmd.Execute()
	}()

	if err := waitForHTTP200(state.readyURL); err != nil {
		_ = state.close()
		return err
	}

	w.appServer = state
	return nil
}

func (w *harnessWorld) theAppServerReceivesJSONRPCLine(body *godog.DocString) error {
	if err := w.writeAppServerLine(body.Content); err != nil {
		return err
	}
	responseLine, err := w.appServer.readResponse()
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(strings.NewReader(responseLine))
	decoder.UseNumber()
	decoded := map[string]any{}
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("failed to decode app-server response; %w", err)
	}
	w.appServer.lastResponse = decoded
	return nil
}

func (w *harnessWorld) theAppServerReceivesJSONRPCNotification(body *godog.DocString) error {
	return w.writeAppServerLine(body.Content)
}

func (w *harnessWorld) theWebSocketAppServerClientConnectsWithOrigin(origin string) error {
	return w.connectWebSocketAppServer(origin)
}

func (w *harnessWorld) theWebSocketAppServerClientConnectsWithoutAnOriginHeader() error {
	return w.connectWebSocketAppServer("")
}

func (w *harnessWorld) theWebSocketAppServerClientRoutesThroughAControllableProxy() error {
	if w.appServer == nil || strings.TrimSpace(w.appServer.websocketURL) == "" {
		return fmt.Errorf("websocket app-server transport is not running")
	}
	if w.appServer.websocketProxy != nil {
		return nil
	}

	proxy, err := startAppServerWebSocketProxy(w.appServer.websocketURL)
	if err != nil {
		return err
	}
	w.appServer.websocketProxy = proxy
	return nil
}

func (w *harnessWorld) theWebSocketAppServerClientSendsJSONRPCMessage(body *godog.DocString) error {
	if err := w.writeWebSocketMessage(body.Content); err != nil {
		return err
	}
	return w.captureWebSocketMessage(5 * time.Second)
}

func (w *harnessWorld) theWebSocketAppServerClientSendsJSONRPCNotification(body *godog.DocString) error {
	return w.writeWebSocketMessage(body.Content)
}

func (w *harnessWorld) theStdioAppServerClientReceivesJSONRPCNotificationMethod(expectedMethod string) error {
	if w.appServer == nil {
		return fmt.Errorf("app-server stdio transport is not running")
	}
	line, err := w.appServer.readResponse()
	if err != nil {
		return err
	}

	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	decoded := map[string]any{}
	if err := decoder.Decode(&decoded); err != nil {
		return fmt.Errorf("failed to decode stdio app-server notification; %w", err)
	}
	w.appServer.lastNotification = decoded

	method, _ := decoded["method"].(string)
	if method != expectedMethod {
		return fmt.Errorf("expected stdio notification method %q, got %q", expectedMethod, method)
	}
	return nil
}

func (w *harnessWorld) theStdioAppServerClientReceivesUniqueNotificationSeqValues(expectedCount int, method string) error {
	if w.appServer == nil {
		return fmt.Errorf("app-server stdio transport is not running")
	}

	uniqueSeqs := map[int64]struct{}{}
	totalNotifications := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(uniqueSeqs) < expectedCount {
		line, err := w.appServer.readResponseWithin(time.Until(deadline))
		if err != nil {
			return err
		}

		decoded, err := decodeAppServerEnvelope(line)
		if err != nil {
			return err
		}
		w.appServer.lastNotification = decoded

		notificationMethod, _ := decoded["method"].(string)
		if notificationMethod != method {
			continue
		}

		seq, err := appServerNotificationSeq(decoded)
		if err != nil {
			return err
		}
		totalNotifications++
		uniqueSeqs[seq] = struct{}{}
	}

	if len(uniqueSeqs) != expectedCount {
		return fmt.Errorf("expected %d unique %s notification seq values, got %d", expectedCount, method, len(uniqueSeqs))
	}

	quietDeadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(quietDeadline) {
		line, err := w.appServer.readResponseWithin(time.Until(quietDeadline))
		if err != nil {
			if strings.Contains(err.Error(), "timed out waiting for app-server response") {
				break
			}
			return err
		}

		decoded, err := decodeAppServerEnvelope(line)
		if err != nil {
			return err
		}
		w.appServer.lastNotification = decoded

		notificationMethod, _ := decoded["method"].(string)
		if notificationMethod != method {
			continue
		}

		seq, err := appServerNotificationSeq(decoded)
		if err != nil {
			return err
		}
		totalNotifications++
		uniqueSeqs[seq] = struct{}{}
	}

	if totalNotifications != expectedCount {
		return fmt.Errorf(
			"expected exactly %d %s notifications without duplicates, got %d total and %d unique seq values",
			expectedCount,
			method,
			totalNotifications,
			len(uniqueSeqs),
		)
	}
	return nil
}

func (w *harnessWorld) theWebSocketAppServerClientReceivesJSONRPCNotificationMethod(expectedMethod string) error {
	if err := w.captureWebSocketMessage(5 * time.Second); err != nil {
		return err
	}
	if w.appServer == nil || w.appServer.lastNotification == nil {
		return fmt.Errorf("no websocket app-server notification has been captured")
	}
	method, _ := w.appServer.lastNotification["method"].(string)
	if method != expectedMethod {
		return fmt.Errorf("expected websocket notification method %q, got %q", expectedMethod, method)
	}
	return nil
}

func (w *harnessWorld) theWebSocketAppServerProxyStopsForwardingServerFrames() error {
	if w.appServer == nil || w.appServer.websocketProxy == nil {
		return fmt.Errorf("websocket app-server proxy is not running")
	}
	w.appServer.websocketProxy.pauseServerFrames()
	return nil
}

func (w *harnessWorld) theWebSocketAppServerProxyResumesForwardingServerFrames() error {
	if w.appServer == nil || w.appServer.websocketProxy == nil {
		return fmt.Errorf("websocket app-server proxy is not running")
	}
	w.appServer.websocketProxy.resumeServerFrames()
	return nil
}

func (w *harnessWorld) theWebSocketAppServerClientDetectsAMissedHeartbeatWindow() error {
	if w.appServer == nil || w.appServer.websocketConn == nil {
		return fmt.Errorf("websocket app-server client is not connected")
	}

	timeout := 2750 * time.Millisecond
	if interval, ok := heartbeatIntervalFromNotification(w.appServer.lastNotification); ok {
		timeout = interval + 750*time.Millisecond
	}

	_, err := w.readWebSocketEnvelope(timeout)
	if err == nil {
		return fmt.Errorf("expected websocket app-server client to miss a heartbeat window, but a message arrived")
	}
	if !isWebSocketReadTimeout(err) {
		return fmt.Errorf("expected missed-heartbeat timeout, got %v", err)
	}
	return nil
}

func (w *harnessWorld) theWebSocketAppServerClientReconnects() error {
	if w.appServer == nil {
		return fmt.Errorf("websocket app-server transport is not running")
	}
	return w.connectWebSocketAppServer(w.appServer.websocketOrigin)
}

func (w *harnessWorld) theWebSocketAppServerClientRequestsRunSubscribeForFixtureRun(runKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendWebSocketAppServerRPCRequest("run/subscribe", map[string]any{
		"runId": runID,
	})
}

func (w *harnessWorld) theWebSocketAppServerClientRequestsRunSubscribeForFixtureRunAfterSeq(runKey string, afterSeq int) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendWebSocketAppServerRPCRequest("run/subscribe", map[string]any{
		"runId":    runID,
		"afterSeq": afterSeq,
	})
}

func (w *harnessWorld) theWebSocketAppServerClientReceivesUniqueNotificationSeqValuesGreaterThan(expectedCount int, method string, minimumSeq int) error {
	if w.appServer == nil || w.appServer.websocketConn == nil {
		return fmt.Errorf("websocket app-server client is not connected")
	}

	uniqueSeqs := map[int64]struct{}{}
	totalNotifications := 0
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(uniqueSeqs) < expectedCount {
		envelope, err := w.readWebSocketEnvelope(time.Until(deadline))
		if err != nil {
			return err
		}

		notificationMethod, _ := envelope["method"].(string)
		if notificationMethod != method {
			continue
		}

		seq, err := appServerNotificationSeq(envelope)
		if err != nil {
			return err
		}
		if seq <= int64(minimumSeq) {
			return fmt.Errorf("expected %s seq to be > %d, got %d", method, minimumSeq, seq)
		}
		totalNotifications++
		uniqueSeqs[seq] = struct{}{}
	}

	if len(uniqueSeqs) != expectedCount {
		return fmt.Errorf("expected %d unique %s notification seq values > %d, got %d", expectedCount, method, minimumSeq, len(uniqueSeqs))
	}

	quietDeadline := time.Now().Add(300 * time.Millisecond)
	for time.Now().Before(quietDeadline) {
		envelope, err := w.readWebSocketEnvelope(time.Until(quietDeadline))
		if err != nil {
			if isWebSocketReadTimeout(err) {
				break
			}
			return err
		}

		notificationMethod, _ := envelope["method"].(string)
		if notificationMethod != method {
			continue
		}

		seq, err := appServerNotificationSeq(envelope)
		if err != nil {
			return err
		}
		if seq <= int64(minimumSeq) {
			return fmt.Errorf("expected %s seq to be > %d, got %d", method, minimumSeq, seq)
		}
		totalNotifications++
		uniqueSeqs[seq] = struct{}{}
	}

	if totalNotifications != expectedCount {
		return fmt.Errorf(
			"expected exactly %d %s notifications > %d without duplicates, got %d total and %d unique seq values",
			expectedCount,
			method,
			minimumSeq,
			totalNotifications,
			len(uniqueSeqs),
		)
	}
	return nil
}

func (w *harnessWorld) theQueuedLiveAppServerFixtureStartsExecution() error {
	if w.appServer == nil || w.appServer.liveLifecycle == nil {
		return fmt.Errorf("queued live app-server fixture is not available")
	}
	return w.appServer.liveLifecycle.StartExecution()
}

func (w *harnessWorld) theAppServerReadyAndLiveEndpointsReturnSuccess() error {
	if w.appServer == nil || w.appServer.readyURL == "" || w.appServer.liveURL == "" {
		return fmt.Errorf("websocket app-server transport is not running")
	}
	for _, endpoint := range []string{w.appServer.readyURL, w.appServer.liveURL} {
		response, err := http.Get(endpoint)
		if err != nil {
			return fmt.Errorf("failed to read app-server health endpoint %q; %w", endpoint, err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			return fmt.Errorf("expected app-server health endpoint %q to return 200, got %d", endpoint, response.StatusCode)
		}
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseErrorCodeIs(expectedCode int) error {
	errorPayload, err := w.appServerErrorPayload()
	if err != nil {
		return err
	}
	codeValue, ok := errorPayload["code"].(json.Number)
	if !ok {
		return fmt.Errorf("expected numeric error code, got %T", errorPayload["code"])
	}
	parsed, err := codeValue.Int64()
	if err != nil {
		return fmt.Errorf("failed to parse error code; %w", err)
	}
	if int(parsed) != expectedCode {
		return fmt.Errorf("expected error code %d, got %d", expectedCode, parsed)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseDataCodeIs(expectedCode string) error {
	errorPayload, err := w.appServerErrorPayload()
	if err != nil {
		return err
	}
	dataPayload, ok := errorPayload["data"].(map[string]any)
	if !ok {
		return fmt.Errorf("expected error data payload, got %T", errorPayload["data"])
	}
	codeValue, _ := dataPayload["code"].(string)
	if codeValue != expectedCode {
		return fmt.Errorf("expected error data.code %q, got %q", expectedCode, codeValue)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseResultFieldIs(field string, expectedValue string) error {
	if w.appServer == nil || w.appServer.lastResponse == nil {
		return fmt.Errorf("no app-server response has been captured")
	}
	resultPayload, ok := w.appServer.lastResponse["result"].(map[string]any)
	if !ok {
		return fmt.Errorf("expected result payload, got %T", w.appServer.lastResponse["result"])
	}
	value, _ := resultPayload[field].(string)
	if value != expectedValue {
		return fmt.Errorf("expected result field %q to be %q, got %q", field, expectedValue, value)
	}
	return nil
}

func (w *harnessWorld) persistedAppServerRunFixturesExistInTheConfiguredRunDirectory() error {
	if w.appServer != nil && w.appServer.fixtures != nil {
		return nil
	}

	cfg, err := loadAcceptanceAppConfig()
	if err != nil {
		return err
	}

	olderRunID, err := w.createCompletedRun(cfg.AppServer.RunDir, nil, nil)
	if err != nil {
		return err
	}
	time.Sleep(10 * time.Millisecond)

	fixtures, err := w.createRichAppServerRun(cfg.AppServer.RunDir)
	if err != nil {
		return err
	}
	fixtures.SecondaryRunID = olderRunID

	if w.appServer == nil {
		w.appServer = &appServerAcceptanceState{nextRequestID: 2}
	}
	w.appServer.fixtures = fixtures
	return nil
}

func (w *harnessWorld) aQueuedLiveAppServerFixtureExistsInTheConfiguredRunDirectory() error {
	cfg, err := loadAcceptanceAppConfig()
	if err != nil {
		return err
	}

	if w.appServer == nil {
		w.appServer = &appServerAcceptanceState{nextRequestID: 2}
	}
	if w.appServer.liveLifecycle != nil && w.appServer.fixtures != nil && w.appServer.fixtures.LiveRunID != "" {
		return nil
	}

	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		Name:         "test-run",
		RunsBaseDir:  cfg.AppServer.RunDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     2,
	})
	if err != nil {
		return fmt.Errorf("failed to create queued live app-server fixture; %w", err)
	}

	if w.appServer.fixtures == nil {
		w.appServer.fixtures = &appServerReadFixtures{}
	}
	w.appServer.fixtures.LiveRunID = lifecycle.RunID()
	w.appServer.liveLifecycle = lifecycle
	return nil
}

func (w *harnessWorld) aLocalCLIRunIsActivelyExecutingForAppServerStopControl() error {
	return w.startRunControlHelperWithRequester("active_interrupt", sigilruntime.StopRequesterAppServerRunStop)
}

func (w *harnessWorld) theAppServerRequestsRunListWithLimit(limit int) error {
	return w.sendAppServerRPCRequest("run/list", map[string]any{
		"limit": limit,
	})
}

func (w *harnessWorld) theAppServerRequestsRunStartWithInlineYAML(body *godog.DocString) error {
	return w.sendAppServerRPCRequest("run/start", map[string]any{
		"runConfigYaml": body.Content,
	})
}

func (w *harnessWorld) theAppServerRequestsForFixtureRun(method string, runKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest(method, map[string]any{
		"runId": runID,
	})
}

func (w *harnessWorld) theAppServerRequestsRunSubscribeForFixtureRun(runKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/subscribe", map[string]any{
		"runId": runID,
	})
}

func (w *harnessWorld) theAppServerRequestsRunSubscribeForFixtureRunAfterSeq(runKey string, afterSeq int) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/subscribe", map[string]any{
		"runId":    runID,
		"afterSeq": afterSeq,
	})
}

func (w *harnessWorld) theAppServerRequestsRunUnsubscribeForFixtureRun(runKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/unsubscribe", map[string]any{
		"runId": runID,
	})
}

func (w *harnessWorld) theAppServerRequestsRunStopForTheActiveRun() error {
	if strings.TrimSpace(w.activeRunID) == "" {
		return fmt.Errorf("expected active run id before app-server run/stop request")
	}
	if err := w.sendAppServerRPCRequest("run/stop", map[string]any{
		"runId": w.activeRunID,
	}); err != nil {
		return err
	}

	result, err := w.appServerStopResult()
	if err != nil {
		return err
	}
	w.activeStopInvocation = stopInvocation{
		Name:     "app-server-active",
		RunID:    result.RunID,
		ExitCode: 0,
		Result:   &result,
	}
	if strings.TrimSpace(result.EventsPath) != "" {
		w.activeRunEventsPath = result.EventsPath
	}
	return nil
}

func (w *harnessWorld) theAppServerRequestsRunNodeReadForFixtureRunNode(runKey string, nodeKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	nodeID, err := w.appServerFixtureValue(nodeKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/node/read", map[string]any{
		"runId":  runID,
		"nodeId": nodeID,
	})
}

func (w *harnessWorld) theAppServerRequestsRunStepReadForFixtureRunNodeStep(runKey string, nodeKey string, stepKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	nodeID, err := w.appServerFixtureValue(nodeKey)
	if err != nil {
		return err
	}
	stepID, err := w.appServerFixtureValue(stepKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/step/read", map[string]any{
		"runId":  runID,
		"nodeId": nodeID,
		"stepId": stepID,
	})
}

func (w *harnessWorld) theAppServerRequestsRunArtifactReadForFixtureRunArtifact(runKey string, artifactKey string) error {
	runID, err := w.appServerFixtureValue(runKey)
	if err != nil {
		return err
	}
	artifactRef, err := w.appServerFixtureValue(artifactKey)
	if err != nil {
		return err
	}
	return w.sendAppServerRPCRequest("run/artifact/read", map[string]any{
		"runId":       runID,
		"artifactRef": artifactRef,
	})
}

func (w *harnessWorld) theAppServerResponseJSONPointerEquals(pointer string, expectedValue string) error {
	value, err := w.appServerResponseJSONPointer(pointer)
	if err != nil {
		return err
	}
	stringValue, ok := value.(string)
	if !ok {
		return fmt.Errorf("expected JSON pointer %q to resolve to string, got %T", pointer, value)
	}
	if stringValue != expectedValue {
		return fmt.Errorf("expected JSON pointer %q to equal %q, got %q", pointer, expectedValue, stringValue)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseJSONPointerEqualsBoolean(pointer string, expectedRaw string) error {
	value, err := w.appServerResponseJSONPointer(pointer)
	if err != nil {
		return err
	}
	expectedValue, err := strconv.ParseBool(expectedRaw)
	if err != nil {
		return fmt.Errorf("failed to parse bool %q; %w", expectedRaw, err)
	}
	boolValue, ok := value.(bool)
	if !ok {
		return fmt.Errorf("expected JSON pointer %q to resolve to bool, got %T", pointer, value)
	}
	if boolValue != expectedValue {
		return fmt.Errorf("expected JSON pointer %q to equal %t, got %t", pointer, expectedValue, boolValue)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseJSONPointerEqualsInteger(pointer string, expectedValue int) error {
	value, err := w.appServerResponseJSONPointer(pointer)
	if err != nil {
		return err
	}

	numberValue, ok := value.(json.Number)
	if !ok {
		return fmt.Errorf("expected JSON pointer %q to resolve to number, got %T", pointer, value)
	}
	parsed, err := numberValue.Int64()
	if err != nil {
		return fmt.Errorf("failed to parse JSON pointer %q number; %w", pointer, err)
	}
	if int(parsed) != expectedValue {
		return fmt.Errorf("expected JSON pointer %q to equal %d, got %d", pointer, expectedValue, parsed)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseJSONPointerEqualsFixture(pointer string, fixtureKey string) error {
	expectedValue, err := w.appServerFixtureValue(fixtureKey)
	if err != nil {
		return err
	}
	return w.theAppServerResponseJSONPointerEquals(pointer, expectedValue)
}

func (w *harnessWorld) theAppServerResponseJSONPointerHasLength(pointer string, expectedLength int) error {
	value, err := w.appServerResponseJSONPointer(pointer)
	if err != nil {
		return err
	}
	switch typed := value.(type) {
	case []any:
		if len(typed) != expectedLength {
			return fmt.Errorf("expected JSON pointer %q length %d, got %d", pointer, expectedLength, len(typed))
		}
	case string:
		if len(typed) != expectedLength {
			return fmt.Errorf("expected JSON pointer %q string length %d, got %d", pointer, expectedLength, len(typed))
		}
	default:
		return fmt.Errorf("expected JSON pointer %q to resolve to array or string, got %T", pointer, value)
	}
	return nil
}

func (w *harnessWorld) theAppServerResponseJSONPointerIsAPositiveInteger(pointer string) error {
	value, err := w.appServerResponseJSONPointer(pointer)
	if err != nil {
		return err
	}
	number, ok := value.(json.Number)
	if !ok {
		return fmt.Errorf("expected JSON pointer %q to resolve to json.Number, got %T", pointer, value)
	}
	parsed, err := number.Int64()
	if err != nil {
		return fmt.Errorf("failed to parse JSON pointer %q as integer; %w", pointer, err)
	}
	if parsed < 1 {
		return fmt.Errorf("expected JSON pointer %q to resolve to positive integer, got %d", pointer, parsed)
	}
	return nil
}

func (w *harnessWorld) typeScriptAppServerBindingsAreGeneratedTwiceDeterministically() error {
	if err := w.executeSigilArgs([]string{"app-server", "generate-ts"}); err != nil {
		return err
	}
	firstOutput := w.lastStdout
	if w.lastErr != nil {
		return fmt.Errorf("expected first TypeScript generation success, got %v", w.lastErr)
	}
	if err := w.executeSigilArgs([]string{"app-server", "generate-ts"}); err != nil {
		return err
	}
	if w.lastErr != nil {
		return fmt.Errorf("expected second TypeScript generation success, got %v", w.lastErr)
	}
	if firstOutput != w.lastStdout {
		return fmt.Errorf("expected deterministic TypeScript output")
	}
	return nil
}

func (w *harnessWorld) jsonSchemaAppServerBundlesAreGeneratedTwiceDeterministically() error {
	if err := w.executeSigilArgs([]string{"app-server", "generate-json-schema"}); err != nil {
		return err
	}
	firstOutput := w.lastStdout
	if w.lastErr != nil {
		return fmt.Errorf("expected first JSON Schema generation success, got %v", w.lastErr)
	}
	if err := w.executeSigilArgs([]string{"app-server", "generate-json-schema"}); err != nil {
		return err
	}
	if w.lastErr != nil {
		return fmt.Errorf("expected second JSON Schema generation success, got %v", w.lastErr)
	}
	if firstOutput != w.lastStdout {
		return fmt.Errorf("expected deterministic JSON Schema output")
	}
	return nil
}

func (w *harnessWorld) theActiveRunStopRequestMetadataRequestedByIs(expected string) error {
	if strings.TrimSpace(w.activeRunID) == "" {
		return fmt.Errorf("expected active run id before stop-request metadata assertion")
	}
	request, ok, err := sigilruntime.ReadStopRequestMetadata(sigilruntime.DefaultRunsBaseDir, w.activeRunID)
	if err != nil {
		return fmt.Errorf("failed to read stop request metadata; %w", err)
	}
	if !ok {
		return fmt.Errorf("expected stop-request metadata for run %q", w.activeRunID)
	}
	if request.RequestedBy != expected {
		return fmt.Errorf("expected requested_by %q, got %q", expected, request.RequestedBy)
	}
	return nil
}

func (w *harnessWorld) writeAppServerLine(line string) error {
	if w.appServer == nil {
		return fmt.Errorf("app-server is not running")
	}
	if _, err := io.WriteString(w.appServer.stdinWriter, strings.TrimSpace(line)+"\n"); err != nil {
		return fmt.Errorf("failed to write app-server request; %w", err)
	}
	return nil
}

func (w *harnessWorld) appServerErrorPayload() (map[string]any, error) {
	if w.appServer == nil || w.appServer.lastResponse == nil {
		return nil, fmt.Errorf("no app-server response has been captured")
	}
	errorPayload, ok := w.appServer.lastResponse["error"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("expected error payload, got %T", w.appServer.lastResponse["error"])
	}
	return errorPayload, nil
}

func (w *harnessWorld) connectWebSocketAppServer(origin string) error {
	if w.appServer == nil || w.appServer.websocketURL == "" {
		return fmt.Errorf("websocket app-server transport is not running")
	}
	if w.appServer.websocketConn != nil {
		_ = w.appServer.websocketConn.Close()
		w.appServer.websocketConn = nil
	}

	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	targetURL := w.appServer.websocketURL
	if w.appServer.websocketProxy != nil {
		targetURL = w.appServer.websocketProxy.clientURL
	}
	connection, _, err := websocket.DefaultDialer.Dial(targetURL, headers)
	if err != nil {
		return fmt.Errorf("failed to connect websocket app-server client; %w", err)
	}
	w.appServer.websocketConn = connection
	w.appServer.websocketOrigin = origin
	return nil
}

func (w *harnessWorld) writeWebSocketMessage(message string) error {
	if w.appServer == nil || w.appServer.websocketConn == nil {
		return fmt.Errorf("websocket app-server client is not connected")
	}
	if err := w.appServer.websocketConn.WriteMessage(websocket.TextMessage, []byte(strings.TrimSpace(message))); err != nil {
		return fmt.Errorf("failed to write websocket app-server message; %w", err)
	}
	return nil
}

func (w *harnessWorld) captureWebSocketMessage(timeout time.Duration) error {
	_, err := w.readWebSocketEnvelope(timeout)
	return err
}

func (w *harnessWorld) readWebSocketEnvelope(timeout time.Duration) (map[string]any, error) {
	if w.appServer == nil || w.appServer.websocketConn == nil {
		return nil, fmt.Errorf("websocket app-server client is not connected")
	}

	if err := w.appServer.websocketConn.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return nil, fmt.Errorf("failed to set websocket read deadline; %w", err)
	}
	_, payload, err := w.appServer.websocketConn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("failed to read websocket app-server message; %w", err)
	}

	decoder := json.NewDecoder(strings.NewReader(string(payload)))
	decoder.UseNumber()
	decoded := map[string]any{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode websocket app-server message; %w", err)
	}
	if method, ok := decoded["method"].(string); ok && method != "" {
		w.appServer.lastNotification = decoded
		return decoded, nil
	}
	w.appServer.lastResponse = decoded
	return decoded, nil
}

func reserveLoopbackPort() (string, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", fmt.Errorf("failed to reserve loopback port; %w", err)
	}
	defer listener.Close()

	_, port, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		return "", fmt.Errorf("failed to resolve reserved loopback port; %w", err)
	}
	return port, nil
}

func waitForHTTP200(target string) error {
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(target)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return nil
			}
		}
		time.Sleep(50 * time.Millisecond)
	}
	return fmt.Errorf("timed out waiting for app-server health endpoint %q", target)
}

func loadAcceptanceAppConfig() (config.Config, error) {
	if err := config.InitFromPath(config.DefaultConfigPath); err != nil {
		return config.Config{}, fmt.Errorf("failed to initialize acceptance app config; %w", err)
	}
	return config.MustGet(), nil
}

func (w *harnessWorld) sendAppServerRPCRequest(method string, params any) error {
	if w.appServer == nil {
		return fmt.Errorf("app-server is not running")
	}
	requestID := strconv.Itoa(w.appServer.nextRequestID)
	w.appServer.nextRequestID++
	line, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("failed to encode app-server request; %w", err)
	}
	return w.theAppServerReceivesJSONRPCLine(&godog.DocString{Content: string(line)})
}

func (w *harnessWorld) sendWebSocketAppServerRPCRequest(method string, params any) error {
	if w.appServer == nil || w.appServer.websocketConn == nil {
		return fmt.Errorf("websocket app-server client is not connected")
	}
	requestID := strconv.Itoa(w.appServer.nextRequestID)
	w.appServer.nextRequestID++
	line, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      requestID,
		"method":  method,
		"params":  params,
	})
	if err != nil {
		return fmt.Errorf("failed to encode websocket app-server request; %w", err)
	}
	if err := w.writeWebSocketMessage(string(line)); err != nil {
		return err
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		envelope, err := w.readWebSocketEnvelope(time.Until(deadline))
		if err != nil {
			return err
		}
		if responseIDMatches(envelope["id"], requestID) {
			return nil
		}
	}
	return fmt.Errorf("timed out waiting for websocket app-server response id %q", requestID)
}

func (w *harnessWorld) appServerStopResult() (acceptanceStopResult, error) {
	if w.appServer == nil || w.appServer.lastResponse == nil {
		return acceptanceStopResult{}, fmt.Errorf("no app-server response has been captured")
	}

	runIDValue, err := lookupJSONPointer(w.appServer.lastResponse, "/result/runId")
	if err != nil {
		return acceptanceStopResult{}, err
	}
	runID, ok := runIDValue.(string)
	if !ok {
		return acceptanceStopResult{}, fmt.Errorf("expected run/stop runId string, got %T", runIDValue)
	}

	stopRequestedValue, err := lookupJSONPointer(w.appServer.lastResponse, "/result/payload/stopRequested")
	if err != nil {
		return acceptanceStopResult{}, err
	}
	stopRequested, ok := stopRequestedValue.(bool)
	if !ok {
		return acceptanceStopResult{}, fmt.Errorf("expected run/stop stopRequested bool, got %T", stopRequestedValue)
	}

	stateValue, err := lookupJSONPointer(w.appServer.lastResponse, "/result/payload/state")
	if err != nil {
		return acceptanceStopResult{}, err
	}
	state, ok := stateValue.(string)
	if !ok {
		return acceptanceStopResult{}, fmt.Errorf("expected run/stop state string, got %T", stateValue)
	}

	eventsPathValue, err := lookupJSONPointer(w.appServer.lastResponse, "/result/payload/eventsPath")
	if err != nil {
		return acceptanceStopResult{}, err
	}
	eventsPath, ok := eventsPathValue.(string)
	if !ok {
		return acceptanceStopResult{}, fmt.Errorf("expected run/stop eventsPath string, got %T", eventsPathValue)
	}

	return acceptanceStopResult{
		RunID:         runID,
		StopRequested: stopRequested,
		State:         state,
		EventsPath:    eventsPath,
	}, nil
}

func (w *harnessWorld) appServerFixtureValue(key string) (string, error) {
	if w.appServer == nil || w.appServer.fixtures == nil {
		return "", fmt.Errorf("app-server fixtures are not available")
	}
	switch key {
	case "primaryRunId":
		return w.appServer.fixtures.PrimaryRunID, nil
	case "secondaryRunId":
		return w.appServer.fixtures.SecondaryRunID, nil
	case "liveRunId":
		return w.appServer.fixtures.LiveRunID, nil
	case "rootNodeId":
		return w.appServer.fixtures.RootNodeID, nil
	case "childNodeId":
		return w.appServer.fixtures.ChildNodeID, nil
	case "stepId":
		return w.appServer.fixtures.StepID, nil
	case "actionRef":
		return w.appServer.fixtures.ActionRef, nil
	default:
		return "", fmt.Errorf("unknown app-server fixture key %q", key)
	}
}

func (w *harnessWorld) appServerResponseJSONPointer(pointer string) (any, error) {
	if w.appServer == nil || w.appServer.lastResponse == nil {
		return nil, fmt.Errorf("no app-server response has been captured")
	}
	return lookupJSONPointer(w.appServer.lastResponse, pointer)
}

func lookupJSONPointer(document any, pointer string) (any, error) {
	if pointer == "" {
		return document, nil
	}
	if !strings.HasPrefix(pointer, "/") {
		return nil, fmt.Errorf("json pointer %q must start with /", pointer)
	}

	current := document
	segments := strings.Split(pointer, "/")[1:]
	for _, segment := range segments {
		token := strings.ReplaceAll(strings.ReplaceAll(segment, "~1", "/"), "~0", "~")
		switch typed := current.(type) {
		case map[string]any:
			next, ok := typed[token]
			if !ok {
				return nil, fmt.Errorf("json pointer segment %q was not found", token)
			}
			current = next
		case []any:
			index, err := strconv.Atoi(token)
			if err != nil {
				return nil, fmt.Errorf("json pointer segment %q is not a valid array index", token)
			}
			if index < 0 || index >= len(typed) {
				return nil, fmt.Errorf("json pointer index %d is out of range", index)
			}
			current = typed[index]
		default:
			return nil, fmt.Errorf("json pointer segment %q cannot descend into %T", token, current)
		}
	}
	return current, nil
}

func decodeAppServerEnvelope(line string) (map[string]any, error) {
	decoder := json.NewDecoder(strings.NewReader(line))
	decoder.UseNumber()
	decoded := map[string]any{}
	if err := decoder.Decode(&decoded); err != nil {
		return nil, fmt.Errorf("failed to decode app-server envelope; %w", err)
	}
	return decoded, nil
}

func appServerNotificationSeq(decoded map[string]any) (int64, error) {
	params, ok := decoded["params"].(map[string]any)
	if !ok {
		return 0, fmt.Errorf("expected app-server notification params payload, got %T", decoded["params"])
	}
	seqValue, ok := params["seq"].(json.Number)
	if !ok {
		return 0, fmt.Errorf("expected app-server notification seq number, got %T", params["seq"])
	}
	seq, err := seqValue.Int64()
	if err != nil {
		return 0, fmt.Errorf("failed to parse app-server notification seq; %w", err)
	}
	return seq, nil
}

func heartbeatIntervalFromNotification(decoded map[string]any) (time.Duration, bool) {
	if method, _ := decoded["method"].(string); method != "server/heartbeat" {
		return 0, false
	}
	params, ok := decoded["params"].(map[string]any)
	if !ok {
		return 0, false
	}
	rawValue, ok := params["heartbeatIntervalMs"].(json.Number)
	if !ok {
		return 0, false
	}
	value, err := rawValue.Int64()
	if err != nil {
		return 0, false
	}
	return time.Duration(value) * time.Millisecond, true
}

func responseIDMatches(rawID any, expected string) bool {
	switch typed := rawID.(type) {
	case string:
		return typed == expected
	case json.Number:
		return typed.String() == expected
	default:
		return false
	}
}

func isWebSocketReadTimeout(err error) bool {
	if err == nil {
		return false
	}
	return strings.Contains(err.Error(), "i/o timeout") || strings.Contains(err.Error(), "timed out")
}

func (w *harnessWorld) createRichAppServerRun(runsBaseDir string) (*appServerReadFixtures, error) {
	lifecycle, err := sigilruntime.NewLifecycleWithOptions(sigilruntime.LifecycleOptions{
		Name:         "test-run",
		RunsBaseDir:  runsBaseDir,
		QueuedSource: sigilruntime.RunQueuedSourceCLIRunStart,
		MaxDepth:     3,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create lifecycle; %w", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	if err := lifecycle.StartExecution(); err != nil {
		return nil, fmt.Errorf("failed to start lifecycle; %w", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		return nil, fmt.Errorf("failed to read root node; %w", err)
	}
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to append node.step.started; %w", err)
	}
	childNode, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to create child node; %w", err)
	}

	actionRef, err := sigilruntime.BuildActionArtifactRef(rootNode.ID, stepStarted.StepID, 1)
	if err != nil {
		return nil, fmt.Errorf("failed to build action ref; %w", err)
	}
	fixtures := &appServerReadFixtures{
		PrimaryRunID: lifecycle.RunID(),
		RootNodeID:   rootNode.ID,
		ChildNodeID:  childNode.ID,
		StepID:       stepStarted.StepID,
		ActionRef:    actionRef,
	}

	userTurnRef := fmt.Sprintf("run-artifact://node/%s/step/%s/turn-user.json", rootNode.ID, stepStarted.StepID)
	modelTurnRef := fmt.Sprintf("run-artifact://node/%s/step/%s/turn-model.json", rootNode.ID, stepStarted.StepID)
	stepAccountingRef := fmt.Sprintf("run-artifact://node/%s/step/%s/accounting.json", rootNode.ID, stepStarted.StepID)
	subcallAccountingRef := fmt.Sprintf("run-artifact://node/%s/step/%s/subcall-1-accounting.json", rootNode.ID, stepStarted.StepID)
	runAccountingRef := "run-artifact://run/accounting.json"
	finalAnswerRef := fmt.Sprintf("run-artifact://node/%s/final-answer.json", rootNode.ID)
	rootNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", rootNode.ID)
	childNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", childNode.ID)

	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, userTurnRef, map[string]any{
		"run_id":  fixtures.PrimaryRunID,
		"node_id": rootNode.ID,
		"step_id": stepStarted.StepID,
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, modelTurnRef, map[string]any{
		"run_id":            fixtures.PrimaryRunID,
		"node_id":           rootNode.ID,
		"step_id":           stepStarted.StepID,
		"schema_id":         "sigil.rlm.response.v1",
		"validated_payload": map[string]any{"decision": "continue"},
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, actionRef, map[string]any{
		"run_id":       fixtures.PrimaryRunID,
		"node_id":      rootNode.ID,
		"step_id":      stepStarted.StepID,
		"action_index": 1,
		"action_type":  "repl_code",
		"language":     "go",
		"status":       "completed",
		"stdout":       "fixture-stdout",
		"stderr":       "",
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, subcallAccountingRef, map[string]any{
		"run_id":        fixtures.PrimaryRunID,
		"node_id":       rootNode.ID,
		"step_id":       stepStarted.StepID,
		"subcall_index": 1,
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, stepAccountingRef, map[string]any{
		"run_id":  fixtures.PrimaryRunID,
		"node_id": rootNode.ID,
		"step_id": stepStarted.StepID,
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, rootNodeAccountingRef, map[string]any{
		"run_id":  fixtures.PrimaryRunID,
		"node_id": rootNode.ID,
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, childNodeAccountingRef, map[string]any{
		"run_id":  fixtures.PrimaryRunID,
		"node_id": childNode.ID,
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, finalAnswerRef, map[string]any{
		"run_id":       fixtures.PrimaryRunID,
		"node_id":      rootNode.ID,
		"final_answer": "fixture final answer",
	}); err != nil {
		return nil, err
	}
	if err := writeAcceptanceAppServerArtifact(runsBaseDir, fixtures.PrimaryRunID, runAccountingRef, map[string]any{
		"run_id": fixtures.PrimaryRunID,
	}); err != nil {
		return nil, err
	}

	if err := lifecycle.AppendNodeTurn(rootNode.ID, sigilruntime.TurnRoleUser, stepStarted.StepID, userTurnRef); err != nil {
		return nil, fmt.Errorf("failed to append node.turn.user; %w", err)
	}
	if err := lifecycle.AppendNodeTurn(rootNode.ID, sigilruntime.TurnRoleModel, stepStarted.StepID, modelTurnRef); err != nil {
		return nil, fmt.Errorf("failed to append node.turn.model; %w", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(childNode.ID, nil, acceptanceUnavailableRollup(), &childNodeAccountingRef); err != nil {
		return nil, fmt.Errorf("failed to complete child node; %w", err)
	}
	if err := lifecycle.AppendNodeSubcallExecuted(rootNode.ID, sigilruntime.NodeSubcallExecutedPayload{
		StepID:        stepStarted.StepID,
		ActionIndex:   1,
		SubcallIndex:  1,
		SubcallType:   sigilruntime.SubcallTypeRLMQuery,
		ExecutionMode: sigilruntime.SubcallExecutionModeRecursive,
		Status:        sigilruntime.ActionExecutionStatusCompleted,
		Provider:      "openai",
		Model:         "gpt-5.1",
		PromptBytes:   12,
		ContextBytes:  34,
		AnswerBytes:   56,
		DurationMS:    78,
		ChildNodeID:   &childNode.ID,
		Accounting:    accounting.UnavailableSummary("openai", "gpt-5.1", "acceptance"),
		AccountingRef: subcallAccountingRef,
	}); err != nil {
		return nil, fmt.Errorf("failed to append node.subcall.executed; %w", err)
	}
	if err := lifecycle.AppendNodeActionExecuted(rootNode.ID, sigilruntime.NodeActionExecutedPayload{
		StepID:      stepStarted.StepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      sigilruntime.ActionExecutionStatusCompleted,
		DurationMS:  91,
		ActionRef:   actionRef,
	}); err != nil {
		return nil, fmt.Errorf("failed to append node.action.executed; %w", err)
	}
	if err := lifecycle.AppendNodeStepCompleted(rootNode.ID, sigilruntime.NodeStepCompletedPayload{
		StepID:        stepStarted.StepID,
		Decision:      sigilruntime.StepDecisionContinue,
		ActionCount:   1,
		DurationMS:    120,
		Accounting:    acceptanceUnavailableRollup(),
		AccountingRef: stepAccountingRef,
	}); err != nil {
		return nil, fmt.Errorf("failed to append node.step.completed; %w", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(rootNode.ID, &finalAnswerRef, acceptanceUnavailableRollup(), &rootNodeAccountingRef); err != nil {
		return nil, fmt.Errorf("failed to complete root node; %w", err)
	}
	if err := lifecycle.CompleteWithAccounting(&finalAnswerRef, acceptanceUnavailableRollup(), &runAccountingRef); err != nil {
		return nil, fmt.Errorf("failed to complete run; %w", err)
	}

	return fixtures, nil
}

func writeAcceptanceAppServerArtifact(runsBaseDir string, runID string, artifactRef string, payload map[string]any) error {
	relativeParts, err := sigilruntime.ResolveArtifactRefPath(artifactRef)
	if err != nil {
		return fmt.Errorf("failed to resolve artifact ref path; %w", err)
	}
	path := filepath.Join(append([]string{runsBaseDir, runID, "artifacts"}, relativeParts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("failed to create app-server fixture artifact directory; %w", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("failed to encode app-server fixture artifact; %w", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		return fmt.Errorf("failed to write app-server fixture artifact; %w", err)
	}
	return nil
}

func (s *appServerAcceptanceState) readResponse() (string, error) {
	return s.readResponseWithin(5 * time.Second)
}

func (s *appServerAcceptanceState) readResponseWithin(timeout time.Duration) (string, error) {
	resultCh := make(chan struct {
		line string
		err  error
	}, 1)

	go func() {
		line, err := s.stdoutReader.ReadString('\n')
		resultCh <- struct {
			line string
			err  error
		}{line: line, err: err}
	}()

	select {
	case result := <-resultCh:
		if result.err != nil {
			return "", fmt.Errorf("failed to read app-server response; %w", result.err)
		}
		return strings.TrimSpace(result.line), nil
	case <-time.After(timeout):
		return "", fmt.Errorf("timed out waiting for app-server response")
	}
}

func (s *appServerAcceptanceState) close() error {
	if s == nil {
		return nil
	}
	if s.websocketConn != nil {
		_ = s.websocketConn.Close()
		s.websocketConn = nil
	}
	if s.websocketProxy != nil {
		if err := s.websocketProxy.close(); err != nil {
			return err
		}
		s.websocketProxy = nil
	}
	if s.liveLifecycle != nil {
		_ = s.liveLifecycle.Close()
		s.liveLifecycle = nil
	}
	if s.stdinWriter != nil {
		_ = s.stdinWriter.Close()
		s.stdinWriter = nil
	}
	if s.stdoutPipe != nil {
		_ = s.stdoutPipe.Close()
		s.stdoutPipe = nil
	}
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	if s.done != nil {
		select {
		case err := <-s.done:
			if err != nil {
				return fmt.Errorf("app-server command failed: %w; stderr=%s", err, s.stderr.String())
			}
		case <-time.After(2 * time.Second):
			return fmt.Errorf("timed out waiting for app-server command to stop")
		}
	}
	return nil
}

func startAppServerWebSocketProxy(targetURL string) (*appServerWebSocketProxy, error) {
	parsed, err := url.Parse(strings.TrimSpace(targetURL))
	if err != nil {
		return nil, fmt.Errorf("failed to parse websocket app-server url; %w", err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("failed to listen for websocket app-server proxy; %w", err)
	}

	clientURL := *parsed
	clientURL.Host = listener.Addr().String()
	proxy := &appServerWebSocketProxy{
		listener:   listener,
		targetAddr: parsed.Host,
		clientURL:  clientURL.String(),
		done:       make(chan error, 1),
	}

	go proxy.serve()
	return proxy, nil
}

func (p *appServerWebSocketProxy) serve() {
	var serveErr error
	defer func() {
		p.done <- serveErr
		close(p.done)
	}()

	for {
		clientConn, err := p.listener.Accept()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				continue
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			serveErr = fmt.Errorf("websocket app-server proxy accept failed; %w", err)
			return
		}

		upstreamConn, err := net.Dial("tcp", p.targetAddr)
		if err != nil {
			_ = clientConn.Close()
			continue
		}

		go p.copyLoop(clientConn, upstreamConn, false)
		go p.copyLoop(upstreamConn, clientConn, true)
	}
}

func (p *appServerWebSocketProxy) copyLoop(source net.Conn, destination net.Conn, pausable bool) {
	defer func() {
		_ = source.Close()
		_ = destination.Close()
	}()

	buffer := make([]byte, 32*1024)
	for {
		bytesRead, err := source.Read(buffer)
		if bytesRead > 0 {
			if !pausable || !p.serverFramesArePaused() {
				if _, writeErr := destination.Write(buffer[:bytesRead]); writeErr != nil {
					return
				}
			}
		}
		if err != nil {
			return
		}
	}
}

func (p *appServerWebSocketProxy) pauseServerFrames() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.serverFramesPaused = true
}

func (p *appServerWebSocketProxy) resumeServerFrames() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.serverFramesPaused = false
}

func (p *appServerWebSocketProxy) serverFramesArePaused() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.serverFramesPaused
}

func (p *appServerWebSocketProxy) close() error {
	if p == nil {
		return nil
	}
	_ = p.listener.Close()
	select {
	case err, ok := <-p.done:
		if !ok || err == nil {
			return nil
		}
		return err
	case <-time.After(2 * time.Second):
		return fmt.Errorf("timed out waiting for websocket app-server proxy to stop")
	}
}
