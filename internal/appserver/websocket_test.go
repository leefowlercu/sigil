package appserver

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/runtime"
)

func TestServeWebSocketServesPingAfterHandshake(t *testing.T) {
	server, address, cleanup := startWebSocketTestServer(t, WebSocketServeOptions{
		AllowedOrigins:    []string{"http://localhost:3000"},
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	connection := dialWebSocket(t, address, "/app-server", "http://localhost:3000")
	defer connection.Close()

	writeWebSocketMessage(t, connection, initializeRequest(protocol.DefaultProtocolVersion()))
	initializeResponse := readRPCResponse(t, connection)
	if initializeResponse.Error != nil {
		t.Fatalf("expected initialize success, got %+v", initializeResponse.Error)
	}

	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","id":"2","method":"server/ping","params":{}}`)

	pingResponse := readRPCResponse(t, connection)
	if pingResponse.Error != nil {
		t.Fatalf("expected ping success, got %+v", pingResponse.Error)
	}

	var ping protocol.ServerPingResult
	if err := json.Unmarshal(pingResponse.Result, &ping); err != nil {
		t.Fatalf("expected ping result to decode, got %v", err)
	}
	if ping.InstanceID != server.config.AppServer.InstanceID {
		t.Fatalf("expected instance id %q, got %q", server.config.AppServer.InstanceID, ping.InstanceID)
	}
}

func TestServeWebSocketExposesHealthEndpoints(t *testing.T) {
	_, address, cleanup := startWebSocketTestServer(t, WebSocketServeOptions{
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	for _, path := range []string{"/readyz", "/healthz"} {
		response, err := http.Get("http://" + address + path)
		if err != nil {
			t.Fatalf("expected health endpoint request success, got %v", err)
		}
		_ = response.Body.Close()
		if response.StatusCode != http.StatusOK {
			t.Fatalf("expected %s to return 200, got %d", path, response.StatusCode)
		}
	}

}

func TestServeWebSocketRejectsDisallowedOrigin(t *testing.T) {
	sink := captureTestLogs(t)

	_, address, cleanup := startWebSocketTestServer(t, WebSocketServeOptions{
		AllowedOrigins:    []string{"http://localhost:3000"},
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	headers := http.Header{}
	headers.Set("Origin", "http://example.com")
	_, _, err := websocket.DefaultDialer.Dial("ws://"+address+"/app-server", headers)
	if err == nil {
		t.Fatal("expected websocket dial to fail for disallowed origin")
	}

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "rejected websocket app-server connection" && entry["reason"] == "disallowed_origin"
	})
	if record["component"] != "appserver.websocket" {
		t.Fatalf("expected appserver.websocket component, got %v", record["component"])
	}
	if record["origin"] != "http://example.com" {
		t.Fatalf("expected logged origin, got %v", record["origin"])
	}
}

func TestServeWebSocketEmitsHeartbeatOnIdleConnection(t *testing.T) {
	_, address, cleanup := startWebSocketTestServer(t, WebSocketServeOptions{
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	connection := dialWebSocket(t, address, "/app-server", "")
	defer connection.Close()

	writeWebSocketMessage(t, connection, initializeRequest(protocol.DefaultProtocolVersion()))
	initializeResponse := readRPCResponse(t, connection)
	if initializeResponse.Error != nil {
		t.Fatalf("expected initialize success, got %+v", initializeResponse.Error)
	}

	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)

	notification := readWebSocketEnvelope(t, connection)
	method, _ := notification["method"].(string)
	if method != "server/heartbeat" {
		t.Fatalf("expected server/heartbeat notification, got %q", method)
	}

	params, ok := notification["params"].(map[string]any)
	if !ok {
		t.Fatalf("expected heartbeat params payload, got %T", notification["params"])
	}
	interval, ok := params["heartbeatIntervalMs"].(float64)
	if !ok {
		t.Fatalf("expected numeric heartbeat interval, got %T", params["heartbeatIntervalMs"])
	}
	if int(interval) != 50 {
		t.Fatalf("expected heartbeat interval 50ms, got %v", interval)
	}
}

func TestServeWebSocketReconnectResumeSubscribeWithoutGaps(t *testing.T) {
	runsDir := t.TempDir()
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		Name:         "test-run",
		RunsBaseDir:  runsDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     2,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = runsDir
	cfg.AppServer.Subscriptions.PollIntervalMS = 25

	_, address, cleanup := startWebSocketTestServerWithConfig(t, cfg, WebSocketServeOptions{
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	connection := dialWebSocket(t, address, cfg.AppServer.WebSocket.Path, "")
	writeWebSocketMessage(t, connection, initializeRequest(protocol.DefaultProtocolVersion()))
	initializeResponse := readRPCResponse(t, connection)
	if initializeResponse.Error != nil {
		t.Fatalf("expected initialize success, got %+v", initializeResponse.Error)
	}
	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","id":"2","method":"run/subscribe","params":{"runId":"`+lifecycle.RunID()+`"}}`)

	subscribeResponse := readWebSocketEnvelope(t, connection)
	resultField, ok := subscribeResponse["result"].(map[string]any)
	if !ok || resultField["snapshotAsOfSeq"] == nil {
		t.Fatalf("expected websocket subscribe snapshot response, got %+v", subscribeResponse)
	}

	heartbeat := readWebSocketEnvelope(t, connection)
	if heartbeat["method"] != "server/heartbeat" {
		t.Fatalf("expected websocket heartbeat before reconnect, got %+v", heartbeat)
	}
	_ = connection.Close()

	reconnected := dialWebSocket(t, address, cfg.AppServer.WebSocket.Path, "")
	defer reconnected.Close()

	writeWebSocketMessage(t, reconnected, initializeRequest(protocol.DefaultProtocolVersion()))
	reinitializeResponse := readRPCResponse(t, reconnected)
	if reinitializeResponse.Error != nil {
		t.Fatalf("expected reconnect initialize success, got %+v", reinitializeResponse.Error)
	}
	writeWebSocketMessage(t, reconnected, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	writeWebSocketMessage(t, reconnected, `{"jsonrpc":"2.0","id":"3","method":"run/subscribe","params":{"runId":"`+lifecycle.RunID()+`","afterSeq":1}}`)

	resumeResponse := readWebSocketEnvelope(t, reconnected)
	resumeResult, ok := resumeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket resume subscribe response, got %+v", resumeResponse)
	}
	resumePayload, ok := resumeResult["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket resume payload, got %+v", resumeResponse)
	}
	if snapshotAsOfSeq, ok := resumeResult["snapshotAsOfSeq"].(float64); !ok || int(snapshotAsOfSeq) != 1 {
		t.Fatalf("expected resume snapshotAsOfSeq 1, got %+v", resumeResult["snapshotAsOfSeq"])
	}
	if replayEvents, ok := resumePayload["replayEvents"].([]any); ok && len(replayEvents) != 0 {
		t.Fatalf("expected zero replay events before execution starts, got %+v", replayEvents)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	seqs := collectWebSocketNotificationSeqs(t, reconnected, "run/eventAppended", 2, 500*time.Millisecond)
	if len(seqs) != 2 {
		t.Fatalf("expected 2 canonical live seq values, got %+v", seqs)
	}
	if seqs[0] != 2 || seqs[1] != 3 {
		t.Fatalf("expected resumed live seq values [2 3], got %+v", seqs)
	}
}

func TestServeWebSocketReconnectResubscribesRunsWithFreshSnapshot(t *testing.T) {
	runsDir := t.TempDir()
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		Name:         "test-run",
		RunsBaseDir:  runsDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     2,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = runsDir
	cfg.AppServer.Subscriptions.PollIntervalMS = 25

	_, address, cleanup := startWebSocketTestServerWithConfig(t, cfg, WebSocketServeOptions{
		HeartbeatInterval: 50 * time.Millisecond,
	})
	defer cleanup()

	connection := dialWebSocket(t, address, cfg.AppServer.WebSocket.Path, "")
	writeWebSocketMessage(t, connection, initializeRequest(protocol.DefaultProtocolVersion()))
	initializeResponse := readRPCResponse(t, connection)
	if initializeResponse.Error != nil {
		t.Fatalf("expected initialize success, got %+v", initializeResponse.Error)
	}
	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	writeWebSocketMessage(t, connection, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)

	subscribeResponse, err := waitForWebSocketEnvelope(t, connection, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	})
	if err != nil {
		t.Fatalf("expected websocket runs/subscribe response, got %v", err)
	}
	subscribeResult, ok := subscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket runs/subscribe result, got %+v", subscribeResponse)
	}
	subscribePayload, ok := subscribeResult["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket runs/subscribe payload, got %+v", subscribeResponse)
	}
	items, ok := subscribePayload["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one websocket runs/subscribe item, got %+v", subscribeResponse)
	}
	if revision, ok := subscribePayload["revision"].(json.Number); !ok || revision.String() == "0" {
		t.Fatalf("expected websocket runs/subscribe revision >= 1, got %+v", subscribePayload["revision"])
	}
	firstItem, ok := items[0].(map[string]any)
	if !ok || firstItem["runId"] != lifecycle.RunID() || firstItem["state"] != string(runtime.RunStateQueued) {
		t.Fatalf("expected queued websocket runs/subscribe item for %q, got %+v", lifecycle.RunID(), items[0])
	}

	heartbeat, err := waitForWebSocketEnvelope(t, connection, 2*time.Second, func(message map[string]any) bool {
		method, _ := message["method"].(string)
		return method == "server/heartbeat"
	})
	if err != nil {
		t.Fatalf("expected websocket heartbeat before reconnect, got %v", err)
	}
	if heartbeat["method"] != "server/heartbeat" {
		t.Fatalf("expected websocket heartbeat before reconnect, got %+v", heartbeat)
	}
	_ = connection.Close()

	reconnected := dialWebSocket(t, address, cfg.AppServer.WebSocket.Path, "")
	defer reconnected.Close()

	writeWebSocketMessage(t, reconnected, initializeRequest(protocol.DefaultProtocolVersion()))
	reinitializeResponse := readRPCResponse(t, reconnected)
	if reinitializeResponse.Error != nil {
		t.Fatalf("expected reconnect initialize success, got %+v", reinitializeResponse.Error)
	}
	writeWebSocketMessage(t, reconnected, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	writeWebSocketMessage(t, reconnected, `{"jsonrpc":"2.0","id":"3","method":"runs/subscribe","params":{}}`)

	resubscribeResponse, err := waitForWebSocketEnvelope(t, reconnected, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "3"
	})
	if err != nil {
		t.Fatalf("expected websocket resubscribe response, got %v", err)
	}
	resubscribeResult, ok := resubscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket resubscribe result, got %+v", resubscribeResponse)
	}
	resubscribePayload, ok := resubscribeResult["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected websocket resubscribe payload, got %+v", resubscribeResponse)
	}
	resubscribeItems, ok := resubscribePayload["items"].([]any)
	if !ok || len(resubscribeItems) != 1 {
		t.Fatalf("expected one websocket resubscribe item, got %+v", resubscribeResponse)
	}
	if revision, ok := resubscribePayload["revision"].(json.Number); !ok || revision.String() == "0" {
		t.Fatalf("expected websocket resubscribe revision >= 1, got %+v", resubscribePayload["revision"])
	}
	secondItem, ok := resubscribeItems[0].(map[string]any)
	if !ok || secondItem["runId"] != lifecycle.RunID() || secondItem["state"] != string(runtime.RunStateQueued) {
		t.Fatalf("expected queued websocket resubscribe item for %q, got %+v", lifecycle.RunID(), resubscribeItems[0])
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	if _, err := waitForWebSocketEnvelope(t, reconnected, 2*time.Second, func(message map[string]any) bool {
		return isRunSummaryUpsertNotification(message, "runs/changed", lifecycle.RunID(), string(runtime.RunStateRunning))
	}); err != nil {
		t.Fatalf("expected websocket runs/changed notification, got %v", err)
	}
}

func TestServeWebSocketRejectsNonLoopbackListenAddr(t *testing.T) {
	cfg := config.NewDefaultConfig()
	server := New(cfg)
	err := server.ServeWebSocket(context.Background(), WebSocketServeOptions{
		ListenAddr:        "0.0.0.0:8765",
		WebSocketPath:     cfg.AppServer.WebSocket.Path,
		ReadyPath:         cfg.AppServer.Health.ReadyPath,
		LivePath:          cfg.AppServer.Health.LivePath,
		MaxConnections:    cfg.AppServer.Limits.MaxConnections,
		MaxFrameBytes:     cfg.AppServer.Limits.MaxFrameBytes,
		HeartbeatInterval: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatal("expected non-loopback listen address rejection")
	}
	if !strings.Contains(err.Error(), "localhost or a loopback IP") {
		t.Fatalf("expected localhost rejection error, got %v", err)
	}
}

func startWebSocketTestServer(t *testing.T, options WebSocketServeOptions) (*Server, string, func()) {
	t.Helper()

	cfg := config.NewDefaultConfig()
	return startWebSocketTestServerWithConfig(t, cfg, options)
}

func startWebSocketTestServerWithConfig(t *testing.T, cfg config.Config, options WebSocketServeOptions) (*Server, string, func()) {
	t.Helper()

	server := New(cfg)
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("expected test listener setup success, got %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() {
		errCh <- server.ServeWebSocket(ctx, WebSocketServeOptions{
			Listener:          listener,
			WebSocketPath:     cfg.AppServer.WebSocket.Path,
			ReadyPath:         cfg.AppServer.Health.ReadyPath,
			LivePath:          cfg.AppServer.Health.LivePath,
			AllowedOrigins:    options.AllowedOrigins,
			MaxConnections:    cfg.AppServer.Limits.MaxConnections,
			MaxFrameBytes:     cfg.AppServer.Limits.MaxFrameBytes,
			HeartbeatInterval: options.HeartbeatInterval,
		})
	}()

	waitForTestHTTP200(t, "http://"+listener.Addr().String()+cfg.AppServer.Health.ReadyPath)
	return server, listener.Addr().String(), func() {
		cancel()
		expectServeStopped(t, errCh)
	}
}

func collectWebSocketNotificationSeqs(
	t *testing.T,
	connection *websocket.Conn,
	method string,
	expectedCount int,
	quietWindow time.Duration,
) []int64 {
	t.Helper()

	seqs := make([]int64, 0, expectedCount)
	seen := map[int64]struct{}{}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) && len(seqs) < expectedCount {
		envelope := readWebSocketEnvelope(t, connection)
		notificationMethod, _ := envelope["method"].(string)
		if notificationMethod != method {
			continue
		}

		params, ok := envelope["params"].(map[string]any)
		if !ok {
			t.Fatalf("expected notification params payload, got %T", envelope["params"])
		}
		rawSeq, ok := params["seq"].(float64)
		if !ok {
			t.Fatalf("expected notification seq number, got %T", params["seq"])
		}
		seq := int64(rawSeq)
		if _, exists := seen[seq]; exists {
			t.Fatalf("expected unique notification seq values, got duplicate %d", seq)
		}
		seen[seq] = struct{}{}
		seqs = append(seqs, seq)
	}

	readUntil := time.Now().Add(quietWindow)
	for time.Now().Before(readUntil) {
		if err := connection.SetReadDeadline(time.Now().Add(time.Until(readUntil))); err != nil {
			t.Fatalf("expected websocket read deadline success, got %v", err)
		}
		_, payload, err := connection.ReadMessage()
		if err != nil {
			if ne, ok := err.(net.Error); ok && ne.Timeout() {
				break
			}
			t.Fatalf("expected websocket quiet-window read success, got %v", err)
		}

		envelope := map[string]any{}
		if err := json.Unmarshal(payload, &envelope); err != nil {
			t.Fatalf("expected websocket quiet-window decode success, got %v", err)
		}
		notificationMethod, _ := envelope["method"].(string)
		if notificationMethod != method {
			continue
		}
		params, ok := envelope["params"].(map[string]any)
		if !ok {
			t.Fatalf("expected notification params payload, got %T", envelope["params"])
		}
		rawSeq, ok := params["seq"].(float64)
		if !ok {
			t.Fatalf("expected notification seq number, got %T", params["seq"])
		}
		seq := int64(rawSeq)
		if _, exists := seen[seq]; exists {
			t.Fatalf("expected unique notification seq values, got duplicate %d", seq)
		}
		t.Fatalf("expected only %d %s notifications, got extra seq %d", expectedCount, method, seq)
	}

	return seqs
}

func waitForWebSocketEnvelope(
	t *testing.T,
	connection *websocket.Conn,
	timeout time.Duration,
	match func(map[string]any) bool,
) (map[string]any, error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if err := connection.SetReadDeadline(time.Now().Add(time.Until(deadline))); err != nil {
			t.Fatalf("expected websocket read deadline success, got %v", err)
		}
		_, payload, err := connection.ReadMessage()
		if err != nil {
			return nil, err
		}
		decoder := json.NewDecoder(strings.NewReader(string(payload)))
		decoder.UseNumber()
		decoded := map[string]any{}
		if err := decoder.Decode(&decoded); err != nil {
			t.Fatalf("expected websocket envelope decode success, got %v", err)
		}
		if match(decoded) {
			return decoded, nil
		}
	}
	return nil, context.DeadlineExceeded
}

func dialWebSocket(t *testing.T, address string, path string, origin string) *websocket.Conn {
	t.Helper()

	headers := http.Header{}
	if origin != "" {
		headers.Set("Origin", origin)
	}
	connection, _, err := websocket.DefaultDialer.Dial("ws://"+address+path, headers)
	if err != nil {
		t.Fatalf("expected websocket dial success, got %v", err)
	}
	return connection
}

func writeWebSocketMessage(t *testing.T, connection *websocket.Conn, payload string) {
	t.Helper()

	if err := connection.WriteMessage(websocket.TextMessage, []byte(payload)); err != nil {
		t.Fatalf("expected websocket write success, got %v", err)
	}
}

func readRPCResponse(t *testing.T, connection *websocket.Conn) rpcResponse {
	t.Helper()

	var response rpcResponse
	if err := readJSONInto(connection, &response, 2*time.Second); err != nil {
		t.Fatalf("expected websocket response read success, got %v", err)
	}
	return response
}

func readWebSocketEnvelope(t *testing.T, connection *websocket.Conn) map[string]any {
	t.Helper()

	decoded := map[string]any{}
	if err := readJSONInto(connection, &decoded, 2*time.Second); err != nil {
		t.Fatalf("expected websocket envelope read success, got %v", err)
	}
	return decoded
}

func readJSONInto(connection *websocket.Conn, target any, timeout time.Duration) error {
	if err := connection.SetReadDeadline(time.Now().Add(timeout)); err != nil {
		return err
	}
	_, payload, err := connection.ReadMessage()
	if err != nil {
		return err
	}
	return json.Unmarshal(payload, target)
}

func waitForTestHTTP200(t *testing.T, target string) {
	t.Helper()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		response, err := http.Get(target)
		if err == nil {
			_ = response.Body.Close()
			if response.StatusCode == http.StatusOK {
				return
			}
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", target)
}

func expectServeStopped(t *testing.T, errCh chan error) {
	t.Helper()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("expected websocket server shutdown success, got %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for websocket server shutdown")
	}
}
