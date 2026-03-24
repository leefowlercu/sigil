package appserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/leefowlercu/sigil/internal/accounting"
	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/harness"
	"github.com/leefowlercu/sigil/internal/inference"
	"github.com/leefowlercu/sigil/internal/runtime"
)

type rpcResponse struct {
	JSONRPC string                `json:"jsonrpc"`
	ID      json.RawMessage       `json:"id"`
	Result  json.RawMessage       `json:"result"`
	Error   *protocol.ErrorObject `json:"error"`
}

func TestServeStdioRejectsRequestsBeforeInitialize(t *testing.T) {
	responses := serveJSONLines(t,
		`{"jsonrpc":"2.0","id":"1","method":"server/ping","params":{}}`,
	)

	if len(responses) != 1 {
		t.Fatalf("expected one response, got %d", len(responses))
	}
	if responses[0].Error == nil {
		t.Fatal("expected handshake error response")
	}
	if responses[0].Error.Code != protocol.CodeInvalidRequest {
		t.Fatalf("expected invalid request code, got %d", responses[0].Error.Code)
	}
	if responses[0].Error.Data == nil || responses[0].Error.Data.Code != "handshake_required" {
		t.Fatalf("expected handshake_required domain code, got %+v", responses[0].Error.Data)
	}
}

func TestServeStdioRejectsRepeatedInitialize(t *testing.T) {
	responses := serveJSONLines(t,
		initializeRequest(protocol.DefaultProtocolVersion()),
		initializeRequest(protocol.DefaultProtocolVersion()),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Fatalf("expected first initialize to succeed, got %+v", responses[0].Error)
	}
	if responses[1].Error == nil || responses[1].Error.Data == nil || responses[1].Error.Data.Code != "already_initialized" {
		t.Fatalf("expected already_initialized error, got %+v", responses[1].Error)
	}
}

func TestServeStdioRejectsUnsupportedProtocolVersion(t *testing.T) {
	responses := serveJSONLines(t,
		initializeRequest("sigil.appserver.unsupported"),
	)

	if len(responses) != 1 {
		t.Fatalf("expected one response, got %d", len(responses))
	}
	if responses[0].Error == nil {
		t.Fatal("expected unsupported protocol version error")
	}
	if responses[0].Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("expected invalid params code, got %d", responses[0].Error.Code)
	}
	if responses[0].Error.Data == nil || responses[0].Error.Data.Code != "unsupported_protocol_version" {
		t.Fatalf("expected unsupported_protocol_version domain code, got %+v", responses[0].Error.Data)
	}
}

func TestServeStdioServesPingAfterHandshake(t *testing.T) {
	responses := serveJSONLines(t,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"2","method":"server/ping","params":{}}`,
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Fatalf("expected initialize success, got %+v", responses[0].Error)
	}
	if responses[1].Error != nil {
		t.Fatalf("expected ping success, got %+v", responses[1].Error)
	}

	var ping protocol.ServerPingResult
	if err := json.Unmarshal(responses[1].Result, &ping); err != nil {
		t.Fatalf("expected server/ping result to decode, got %v", err)
	}
	if ping.InstanceID != config.NewDefaultConfig().AppServer.InstanceID {
		t.Fatalf("expected instance id %q, got %q", config.NewDefaultConfig().AppServer.InstanceID, ping.InstanceID)
	}
	if ping.ProtocolVersion != protocol.DefaultProtocolVersion() {
		t.Fatalf("expected protocol version %q, got %q", protocol.DefaultProtocolVersion(), ping.ProtocolVersion)
	}
}

func TestServeStdioInitializeAdvertisesRunReadCapabilities(t *testing.T) {
	responses := serveJSONLines(t,
		initializeRequest(protocol.DefaultProtocolVersion()),
	)

	if len(responses) != 1 {
		t.Fatalf("expected one response, got %d", len(responses))
	}
	if responses[0].Error != nil {
		t.Fatalf("expected initialize success, got %+v", responses[0].Error)
	}

	var initialize protocol.InitializeResult
	if err := json.Unmarshal(responses[0].Result, &initialize); err != nil {
		t.Fatalf("expected initialize result to decode, got %v", err)
	}
	if !containsString(initialize.MethodFamilies, "run") {
		t.Fatalf("expected method families to include run, got %+v", initialize.MethodFamilies)
	}
	if !containsString(initialize.MethodFamilies, "runs") {
		t.Fatalf("expected method families to include runs, got %+v", initialize.MethodFamilies)
	}
	if !initialize.Capabilities.Views.RunTree {
		t.Fatal("expected runTree capability=true")
	}
	if !initialize.Capabilities.Views.StepDetail {
		t.Fatal("expected stepDetail capability=true")
	}
	if !initialize.Capabilities.Views.LiveSubscriptions {
		t.Fatal("expected liveSubscriptions capability=true")
	}
}

func TestServeStdioServesRunReadMethodsAfterHandshake(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"2","method":"runs/list","params":{"limit":1}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"run/read","params":{"runId":"%s"}}`, fixture.RunID),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"4","method":"run/step/read","params":{"runId":"%s","nodeId":"%s","stepId":"%s"}}`, fixture.RunID, fixture.RootNodeID, fixture.StepID),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"5","method":"run/artifact/read","params":{"runId":"%s","artifactRef":"%s"}}`, fixture.RunID, fixture.ActionRef),
	)

	if len(responses) != 5 {
		t.Fatalf("expected five responses, got %d", len(responses))
	}
	for index, response := range responses {
		if response.Error != nil {
			t.Fatalf("expected response %d success, got %+v", index, response.Error)
		}
	}

	var listResult protocol.RunsListResult
	if err := json.Unmarshal(responses[1].Result, &listResult); err != nil {
		t.Fatalf("expected runs/list result to decode, got %v", err)
	}
	if len(listResult.Payload.Items) != 1 || listResult.Payload.Items[0].RunID != fixture.RunID {
		t.Fatalf("expected runs/list to return fixture run %q, got %+v", fixture.RunID, listResult.Payload.Items)
	}

	var readResult protocol.RunReadResult
	if err := json.Unmarshal(responses[2].Result, &readResult); err != nil {
		t.Fatalf("expected run/read result to decode, got %v", err)
	}
	if readResult.RunID != fixture.RunID {
		t.Fatalf("expected run/read runId %q, got %q", fixture.RunID, readResult.RunID)
	}
	if readResult.AsOfSeq < 1 {
		t.Fatalf("expected run/read asOfSeq >= 1, got %d", readResult.AsOfSeq)
	}
	if readResult.Payload.Run.RunID != fixture.RunID {
		t.Fatalf("expected payload runId %q, got %q", fixture.RunID, readResult.Payload.Run.RunID)
	}

	var stepResult protocol.RunStepReadResult
	if err := json.Unmarshal(responses[3].Result, &stepResult); err != nil {
		t.Fatalf("expected run/step/read result to decode, got %v", err)
	}
	if len(stepResult.Payload.Step.ActionRefs) != 1 || stepResult.Payload.Step.ActionRefs[0] != fixture.ActionRef {
		t.Fatalf("expected action refs to include %q, got %+v", fixture.ActionRef, stepResult.Payload.Step.ActionRefs)
	}
	if stepResult.Payload.Step.SchemaID != "sigil.rlm.response.v1" {
		t.Fatalf("expected schema id sigil.rlm.response.v1, got %q", stepResult.Payload.Step.SchemaID)
	}

	var artifactResult protocol.RunArtifactReadResult
	if err := json.Unmarshal(responses[4].Result, &artifactResult); err != nil {
		t.Fatalf("expected run/artifact/read result to decode, got %v", err)
	}
	if artifactResult.Payload.ArtifactKind != "action" {
		t.Fatalf("expected artifact kind action, got %q", artifactResult.Payload.ArtifactKind)
	}
	if artifactResult.Payload.Identity.ActionIndex == nil || *artifactResult.Payload.Identity.ActionIndex != 1 {
		t.Fatalf("expected action index 1, got %v", artifactResult.Payload.Identity.ActionIndex)
	}
	if artifactResult.Payload.Artifact["stdout"] != "fixture-stdout" {
		t.Fatalf("expected stdout fixture-stdout, got %v", artifactResult.Payload.Artifact["stdout"])
	}
}

func TestServeStdioServesRunStepsListAfterHandshake(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/steps/list","params":{"runId":"%s"}}`, fixture.RunID),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error != nil {
		t.Fatalf("expected run/steps/list success, got %+v", responses[1].Error)
	}

	var result protocol.RunStepsListResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("expected run/steps/list result to decode, got %v", err)
	}
	if result.RunID != fixture.RunID {
		t.Fatalf("expected runId %q, got %q", fixture.RunID, result.RunID)
	}
	if result.AsOfSeq != 13 {
		t.Fatalf("expected asOfSeq 13, got %d", result.AsOfSeq)
	}
	if len(result.Payload.Steps) != 1 {
		t.Fatalf("expected 1 step, got %d", len(result.Payload.Steps))
	}
	if result.Payload.Steps[0].StepID != fixture.StepID {
		t.Fatalf("expected stepId %q, got %q", fixture.StepID, result.Payload.Steps[0].StepID)
	}
	if result.Payload.Steps[0].SchemaID != "sigil.rlm.response.v1" {
		t.Fatalf("expected schema id sigil.rlm.response.v1, got %q", result.Payload.Steps[0].SchemaID)
	}
}

func TestServeStdioReturnsNotFoundDomainCodeForMissingNode(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	missingNodeID := "019c97f5-4e56-70d3-8421-3fd92033094d"
	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/node/read","params":{"runId":"%s","nodeId":"%s"}}`, fixture.RunID, missingNodeID),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error == nil {
		t.Fatal("expected node read error response")
	}
	if responses[1].Error.Code != protocol.CodeInvalidParams {
		t.Fatalf("expected invalid params code, got %d", responses[1].Error.Code)
	}
	if responses[1].Error.Data == nil || responses[1].Error.Data.Code != "node_not_found" {
		t.Fatalf("expected node_not_found domain code, got %+v", responses[1].Error.Data)
	}
}

func TestServeStdioReturnsMethodNotFound(t *testing.T) {
	responses := serveJSONLines(t,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"3","method":"server/unknown","params":{}}`,
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error == nil || responses[1].Error.Code != protocol.CodeMethodNotFound {
		t.Fatalf("expected method not found error, got %+v", responses[1].Error)
	}
	if responses[1].Error.Data == nil || responses[1].Error.Data.Code != "method_not_found" {
		t.Fatalf("expected method_not_found domain code, got %+v", responses[1].Error.Data)
	}
}

func TestServeStdioRunStartReturnsQueuedSnapshot(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = t.TempDir()

	responses := serveJSONLinesWithServerOptions(t, cfg, ServerOptions{
		RunnerFactory: func() *harness.Runner {
			return harness.NewRunner(
				harness.WithRunsBaseDir(cfg.AppServer.RunDir),
				harness.WithInferenceFactory(func(config.RunConfig) (harness.InferenceClient, error) {
					return singleFinalInference{}, nil
				}),
			)
		},
	},
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"2","method":"run/start","params":{"runConfigYaml":"name: test-run\nprompt: hello\ncontext: world\nllm:\n  provider: openai\n  model: gpt-5.1\n"}}`,
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error != nil {
		t.Fatalf("expected run/start success, got %+v", responses[1].Error)
	}

	var result protocol.RunStartResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("expected run/start result to decode, got %v", err)
	}
	if result.AsOfSeq != 1 {
		t.Fatalf("expected run/start asOfSeq 1, got %d", result.AsOfSeq)
	}
	if result.Payload.Run.State != string(runtime.RunStateQueued) {
		t.Fatalf("expected queued run state, got %q", result.Payload.Run.State)
	}
	if result.Payload.Run.Source != string(runtime.RunQueuedSourceAppServerStart) {
		t.Fatalf("expected app-server queued source %q, got %q", runtime.RunQueuedSourceAppServerStart, result.Payload.Run.Source)
	}
	if result.Payload.Run.RunConfigPath != nil {
		t.Fatalf("expected run_config_path to be omitted, got %v", result.Payload.Run.RunConfigPath)
	}
}

func TestServeStdioRunSubscribeFreshAttachStreamsAppendedEvents(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/subscribe","params":{"runId":"%s"}}`, lifecycle.RunID()))

	subscribeResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected subscribe response, got %v", err)
	}
	if resultField, ok := subscribeResponse["result"].(map[string]any); !ok || resultField["snapshotAsOfSeq"] == nil {
		t.Fatalf("expected fresh attach snapshot response, got %+v", subscribeResponse)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	methods := collectNotificationMethods(t, server, 4, 2*time.Second)
	if !containsString(methods, "run/eventAppended") {
		t.Fatalf("expected run/eventAppended notification, got %+v", methods)
	}
	if !containsString(methods, "run/started") {
		t.Fatalf("expected run/started notification, got %+v", methods)
	}
	if !containsString(methods, "run/statusChanged") {
		t.Fatalf("expected run/statusChanged notification, got %+v", methods)
	}
}

func TestServeStdioRunSubscribeUsesInProcessObserverForOwnedRun(t *testing.T) {
	sink := captureTestLogs(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = t.TempDir()
	cfg.AppServer.Subscriptions.PollIntervalMS = 1500

	server := startInteractiveStdioServer(t, cfg, ServerOptions{
		RunnerFactory: func() *harness.Runner {
			return harness.NewRunner(
				harness.WithRunsBaseDir(cfg.AppServer.RunDir),
				harness.WithInferenceFactory(func(config.RunConfig) (harness.InferenceClient, error) {
					return blockingCancelInference{}, nil
				}),
			)
		},
	})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"run/start","params":{"runConfigYaml":"name: test-run\nprompt: hello\ncontext: world\nllm:\n  provider: openai\n  model: gpt-5.1\n"}}`)

	startResponse, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	})
	if err != nil {
		t.Fatalf("expected run/start response, got %v", err)
	}
	startResultField, ok := startResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected run/start result payload, got %+v", startResponse)
	}
	runID, ok := startResultField["runId"].(string)
	if !ok || strings.TrimSpace(runID) == "" {
		t.Fatalf("expected run/start runId, got %+v", startResponse)
	}

	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"run/subscribe","params":{"runId":"%s"}}`, runID))
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "3"
	}); err != nil {
		t.Fatalf("expected run/subscribe response, got %v", err)
	}

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "attached app-server run subscription" &&
			entry["run_id"] == runID &&
			entry["delivery_source"] == subscriptionSourceInProcessObserver
	})
	if record["attach_mode"] != "fresh" {
		t.Fatalf("expected fresh attach mode, got %v", record["attach_mode"])
	}

	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"4","method":"run/stop","params":{"runId":"%s"}}`, runID))

	deadline := time.Now().Add(750 * time.Millisecond)
	sawStopResponse := false
	sawEventAppended := false
	for time.Now().Before(deadline) {
		message, readErr := server.readMessage(t, time.Until(deadline))
		if readErr != nil {
			break
		}

		if id, _ := message["id"].(string); id == "4" {
			sawStopResponse = true
			if message["error"] != nil {
				t.Fatalf("expected run/stop success, got %+v", message["error"])
			}
		}
		if method, _ := message["method"].(string); method == "run/eventAppended" {
			sawEventAppended = true
		}
		if sawStopResponse && sawEventAppended {
			break
		}
	}

	if !sawStopResponse {
		t.Fatal("expected run/stop response before polling fallback interval elapsed")
	}
	if !sawEventAppended {
		t.Fatal("expected run/eventAppended notification before polling fallback interval elapsed")
	}
}

func TestServeStdioRunSubscribeResumeReplaysEventsAfterCursor(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/subscribe","params":{"runId":"%s","afterSeq":11}}`, fixture.RunID),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error != nil {
		t.Fatalf("expected resume attach success, got %+v", responses[1].Error)
	}

	var result protocol.RunSubscribeResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("expected run/subscribe result to decode, got %v", err)
	}
	if !result.Payload.Terminal {
		t.Fatal("expected completed fixture replay to be terminal")
	}
	if len(result.Payload.ReplayEvents) != 2 {
		t.Fatalf("expected 2 replayed events after seq 11, got %d", len(result.Payload.ReplayEvents))
	}
	if result.Payload.ReplayEvents[0].Seq != 12 {
		t.Fatalf("expected first replay seq 12, got %d", result.Payload.ReplayEvents[0].Seq)
	}
}

func TestServeStdioRunUnsubscribeStopsFurtherNotifications(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/subscribe","params":{"runId":"%s"}}`, lifecycle.RunID()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected subscribe response, got %v", err)
	}

	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"run/unsubscribe","params":{"runId":"%s"}}`, lifecycle.RunID()))
	unsubscribeResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected unsubscribe response, got %v", err)
	}
	resultField, ok := unsubscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected unsubscribe result payload, got %+v", unsubscribeResponse)
	}
	payloadField, ok := resultField["payload"].(map[string]any)
	if !ok || payloadField["unsubscribed"] != true {
		t.Fatalf("expected unsubscribed=true, got %+v", unsubscribeResponse)
	}

	server.sendLine(t, `{"jsonrpc":"2.0","id":"4","method":"runs/unsubscribe","params":{}}`)
	secondUnsubscribeResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected second runs/unsubscribe response, got %v", err)
	}
	secondResultField, ok := secondUnsubscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected second runs/unsubscribe result payload, got %+v", secondUnsubscribeResponse)
	}
	secondPayloadField, ok := secondResultField["payload"].(map[string]any)
	if !ok || secondPayloadField["unsubscribed"] != false {
		t.Fatalf("expected unsubscribed=false on repeated unsubscribe, got %+v", secondUnsubscribeResponse)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	if _, err := server.readMessage(t, 300*time.Millisecond); err == nil {
		t.Fatal("expected no live notifications after unsubscribe")
	}
}

func TestServeStdioDuplicateRunSubscribeReplacesEarlierSubscription(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/subscribe","params":{"runId":"%s"}}`, lifecycle.RunID()))
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	}); err != nil {
		t.Fatalf("expected first subscribe response, got %v", err)
	}

	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"run/subscribe","params":{"runId":"%s"}}`, lifecycle.RunID()))
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "3"
	}); err != nil {
		t.Fatalf("expected replacement subscribe response, got %v", err)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}

	messages := collectMessagesForDuration(t, server, 500*time.Millisecond)
	seqs := runEventAppendedSeqs(t, messages)
	if len(seqs) != 2 {
		t.Fatalf("expected exactly 2 canonical run/eventAppended notifications, got %d from %+v", len(seqs), seqs)
	}
	if seqs[0] == seqs[1] {
		t.Fatalf("expected unique run/eventAppended seq values, got %+v", seqs)
	}
}

func TestServeStdioRunsSubscribeReturnsFreshSnapshot(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`,
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error != nil {
		t.Fatalf("expected runs/subscribe success, got %+v", responses[1].Error)
	}

	var result protocol.RunsSubscribeResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("expected runs/subscribe result to decode, got %v", err)
	}
	if len(result.Payload.Items) != 1 {
		t.Fatalf("expected one summary item, got %d", len(result.Payload.Items))
	}
	if result.Payload.Revision < 1 {
		t.Fatalf("expected runs/subscribe revision >= 1, got %d", result.Payload.Revision)
	}
	if result.Payload.Items[0].RunID != fixture.RunID {
		t.Fatalf("expected runs/subscribe runId %q, got %q", fixture.RunID, result.Payload.Items[0].RunID)
	}
	if result.Payload.Items[0].State != string(runtime.RunStateCompleted) {
		t.Fatalf("expected completed summary state, got %q", result.Payload.Items[0].State)
	}
}

func TestServeStdioRunsSubscribeDoesNotRedeliverUnchangedSummaries(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)

	subscribeResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected runs/subscribe response, got %v", err)
	}
	resultField, ok := subscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected runs/subscribe result payload, got %+v", subscribeResponse)
	}
	payloadField, ok := resultField["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected runs/subscribe payload, got %+v", subscribeResponse)
	}
	items, ok := payloadField["items"].([]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one runs/subscribe item, got %+v", subscribeResponse)
	}

	if _, err := server.readMessage(t, 200*time.Millisecond); err == nil {
		t.Fatal("expected no runs/changed notifications before any run summary changes")
	}
}

func TestServeStdioRunsSubscribeStreamsChangedUpsertsWithoutRunSubscribe(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	}); err != nil {
		t.Fatalf("expected runs/subscribe response, got %v", err)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		return isRunSummaryUpsertNotification(message, "runs/changed", lifecycle.RunID(), string(runtime.RunStateRunning))
	}); err != nil {
		t.Fatalf("expected running runs/changed notification, got %v", err)
	}

	if err := lifecycle.Complete(); err != nil {
		t.Fatalf("expected complete success, got %v", err)
	}
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		return isRunSummaryUpsertNotification(message, "runs/changed", lifecycle.RunID(), string(runtime.RunStateCompleted))
	}); err != nil {
		t.Fatalf("expected completed runs/changed notification, got %v", err)
	}
}

func TestServeStdioRunsSubscribeStreamsRemoveNotificationsWhenRunDirDisappears(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir
	cfg.AppServer.Subscriptions.PollIntervalMS = 25

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	}); err != nil {
		t.Fatalf("expected runs/subscribe response, got %v", err)
	}

	if err := os.RemoveAll(filepath.Join(fixture.RunsDir, fixture.RunID)); err != nil {
		t.Fatalf("expected run directory removal success, got %v", err)
	}
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		return isRunSummaryRemoveNotification(message, "runs/changed", fixture.RunID)
	}); err != nil {
		t.Fatalf("expected runs/changed remove notification, got %v", err)
	}
}

func TestServeStdioRunsUnsubscribeStopsFurtherSummaryNotifications(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	}); err != nil {
		t.Fatalf("expected runs/subscribe response, got %v", err)
	}

	server.sendLine(t, `{"jsonrpc":"2.0","id":"3","method":"runs/unsubscribe","params":{}}`)
	unsubscribeResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected runs/unsubscribe response, got %v", err)
	}
	resultField, ok := unsubscribeResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected runs/unsubscribe result payload, got %+v", unsubscribeResponse)
	}
	payloadField, ok := resultField["payload"].(map[string]any)
	if !ok || payloadField["unsubscribed"] != true {
		t.Fatalf("expected unsubscribed=true, got %+v", unsubscribeResponse)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	if _, err := server.readMessage(t, 300*time.Millisecond); err == nil {
		t.Fatal("expected no runs/changed notifications after runs/unsubscribe")
	}
}

func TestServeStdioDuplicateRunsSubscribeReplacesEarlierSubscription(t *testing.T) {
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

	server := startInteractiveStdioServer(t, cfg, ServerOptions{})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"runs/subscribe","params":{}}`)
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "2"
	}); err != nil {
		t.Fatalf("expected first runs/subscribe response, got %v", err)
	}

	server.sendLine(t, `{"jsonrpc":"2.0","id":"3","method":"runs/subscribe","params":{}}`)
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		id, _ := message["id"].(string)
		return id == "3"
	}); err != nil {
		t.Fatalf("expected replacement runs/subscribe response, got %v", err)
	}

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	if _, err := waitForMessage(t, server, 2*time.Second, func(message map[string]any) bool {
		return isRunSummaryUpsertNotification(message, "runs/changed", lifecycle.RunID(), string(runtime.RunStateRunning))
	}); err != nil {
		t.Fatalf("expected running runs/changed notification, got %v", err)
	}
	if _, err := server.readMessage(t, 300*time.Millisecond); err == nil {
		t.Fatal("expected no duplicate runs/changed notifications after replacement subscribe")
	}
}

func TestServeStdioRunStopReturnsTerminalNoOpForCompletedRun(t *testing.T) {
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/stop","params":{"runId":"%s"}}`, fixture.RunID),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error != nil {
		t.Fatalf("expected run/stop success, got %+v", responses[1].Error)
	}

	var result protocol.RunStopResult
	if err := json.Unmarshal(responses[1].Result, &result); err != nil {
		t.Fatalf("expected run/stop result to decode, got %v", err)
	}
	if result.RunID != fixture.RunID {
		t.Fatalf("expected runId %q, got %q", fixture.RunID, result.RunID)
	}
	if result.Payload.StopRequested {
		t.Fatal("expected terminal no-op stop_requested=false")
	}
	if result.Payload.State != string(runtime.RunStateCompleted) {
		t.Fatalf("expected completed state, got %q", result.Payload.State)
	}
}

func TestServeStdioRunStopUsesInProcessFastPathForOwnedRun(t *testing.T) {
	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = t.TempDir()

	server := startInteractiveStdioServer(t, cfg, ServerOptions{
		RunnerFactory: func() *harness.Runner {
			return harness.NewRunner(
				harness.WithRunsBaseDir(cfg.AppServer.RunDir),
				harness.WithInferenceFactory(func(config.RunConfig) (harness.InferenceClient, error) {
					return blockingCancelInference{}, nil
				}),
			)
		},
	})
	defer server.close(t)

	server.sendLine(t, initializeRequest(protocol.DefaultProtocolVersion()))
	if _, err := server.readMessage(t, 2*time.Second); err != nil {
		t.Fatalf("expected initialize response, got %v", err)
	}
	server.sendLine(t, `{"jsonrpc":"2.0","method":"initialized","params":{}}`)
	server.sendLine(t, `{"jsonrpc":"2.0","id":"2","method":"run/start","params":{"runConfigYaml":"name: test-run\nprompt: hello\ncontext: world\nllm:\n  provider: openai\n  model: gpt-5.1\n"}}`)

	startResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected run/start response, got %v", err)
	}
	startResultField, ok := startResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected run/start result payload, got %+v", startResponse)
	}
	runID, ok := startResultField["runId"].(string)
	if !ok || strings.TrimSpace(runID) == "" {
		t.Fatalf("expected run/start runId, got %+v", startResponse)
	}

	server.sendLine(t, fmt.Sprintf(`{"jsonrpc":"2.0","id":"3","method":"run/stop","params":{"runId":"%s"}}`, runID))
	stopResponse, err := server.readMessage(t, 5*time.Second)
	if err != nil {
		t.Fatalf("expected run/stop response, got %v", err)
	}
	if stopResponse["error"] != nil {
		t.Fatalf("expected run/stop success, got %+v", stopResponse["error"])
	}

	stopResultField, ok := stopResponse["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected run/stop result payload, got %+v", stopResponse)
	}
	stopPayload, ok := stopResultField["payload"].(map[string]any)
	if !ok {
		t.Fatalf("expected run/stop payload, got %+v", stopResponse)
	}
	if stopPayload["stopRequested"] != true {
		t.Fatalf("expected stopRequested=true, got %+v", stopPayload["stopRequested"])
	}
	if stopPayload["state"] != string(runtime.RunStateInterrupted) {
		t.Fatalf("expected interrupted state, got %+v", stopPayload["state"])
	}

	stopRequest, ok, err := runtime.ReadStopRequestMetadata(cfg.AppServer.RunDir, runID)
	if err != nil {
		t.Fatalf("expected stop-request metadata read success, got %v", err)
	}
	if !ok {
		t.Fatalf("expected stop-request metadata for run %q", runID)
	}
	if stopRequest.RequestedBy != runtime.StopRequesterAppServerRunStop {
		t.Fatalf("expected requested_by %q, got %q", runtime.StopRequesterAppServerRunStop, stopRequest.RequestedBy)
	}

	server.sendLine(t, `{"jsonrpc":"2.0","id":"4","method":"server/ping","params":{}}`)
	pingResponse, err := server.readMessage(t, 2*time.Second)
	if err != nil {
		t.Fatalf("expected server/ping response after run/stop, got %v", err)
	}
	if pingResponse["error"] != nil {
		t.Fatalf("expected server/ping success after run/stop, got %+v", pingResponse["error"])
	}
}

func serveJSONLines(t *testing.T, lines ...string) []rpcResponse {
	t.Helper()

	return serveJSONLinesWithConfig(t, config.NewDefaultConfig(), lines...)
}

func serveJSONLinesWithConfig(t *testing.T, cfg config.Config, lines ...string) []rpcResponse {
	t.Helper()

	return serveJSONLinesWithServerOptions(t, cfg, ServerOptions{}, lines...)
}

func serveJSONLinesWithServerOptions(t *testing.T, cfg config.Config, options ServerOptions, lines ...string) []rpcResponse {
	t.Helper()

	server := NewWithOptions(cfg, options)

	input := strings.Join(lines, "\n")
	if input != "" {
		input += "\n"
	}

	var output bytes.Buffer
	if err := server.ServeStdio(context.Background(), bytes.NewBufferString(input), &output); err != nil {
		t.Fatalf("expected serve success, got %v", err)
	}

	rawLines := strings.Split(strings.TrimSpace(output.String()), "\n")
	if strings.TrimSpace(output.String()) == "" {
		return nil
	}

	responses := make([]rpcResponse, 0, len(rawLines))
	for _, rawLine := range rawLines {
		var response rpcResponse
		if err := json.Unmarshal([]byte(rawLine), &response); err != nil {
			t.Fatalf("expected valid JSON-RPC response, got %v for %q", err, rawLine)
		}
		responses = append(responses, response)
	}
	return responses
}

type interactiveStdioServer struct {
	stdin  *io.PipeWriter
	stdout *bufio.Reader
	done   chan error
}

type singleFinalInference struct{}

type blockingCancelInference struct{}

func (singleFinalInference) Infer(_ context.Context, request inference.Request) (inference.Result, error) {
	return hydrateTestFinalEvidenceRef(finalTestResult("done"), request), nil
}

func (blockingCancelInference) Infer(ctx context.Context, _ inference.Request) (inference.Result, error) {
	<-ctx.Done()
	return inference.Result{}, context.Cause(ctx)
}

func finalTestResult(answer string) inference.Result {
	return inference.Result{
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

func hydrateTestFinalEvidenceRef(result inference.Result, request inference.Request) inference.Result {
	if len(request.Messages) < 2 {
		return result
	}

	var envelope harness.StepInputEnvelope
	if err := json.Unmarshal([]byte(request.Messages[1].Content), &envelope); err != nil {
		return result
	}

	finalPayload, ok := result.ValidatedPayload["final"].(map[string]any)
	if !ok {
		return result
	}
	evidenceRaw, ok := finalPayload["evidence"].([]any)
	if !ok {
		return result
	}
	for _, rawItem := range evidenceRaw {
		evidenceItem, ok := rawItem.(map[string]any)
		if !ok {
			continue
		}
		if evidenceItem["ref"] == "__context_ref__" && strings.TrimSpace(envelope.ContextMetadata.ContextRef) != "" {
			evidenceItem["ref"] = envelope.ContextMetadata.ContextRef
		}
	}
	return result
}

func startInteractiveStdioServer(t *testing.T, cfg config.Config, options ServerOptions) *interactiveStdioServer {
	t.Helper()

	server := NewWithOptions(cfg, options)
	inputReader, inputWriter := io.Pipe()
	outputReader, outputWriter := io.Pipe()

	done := make(chan error, 1)
	go func() {
		done <- server.ServeStdio(context.Background(), inputReader, outputWriter)
		_ = outputWriter.Close()
	}()

	return &interactiveStdioServer{
		stdin:  inputWriter,
		stdout: bufio.NewReader(outputReader),
		done:   done,
	}
}

func (s *interactiveStdioServer) sendLine(t *testing.T, line string) {
	t.Helper()

	if _, err := io.WriteString(s.stdin, line+"\n"); err != nil {
		t.Fatalf("expected stdio request write success, got %v", err)
	}
}

func (s *interactiveStdioServer) readMessage(t *testing.T, timeout time.Duration) (map[string]any, error) {
	t.Helper()

	resultCh := make(chan struct {
		value map[string]any
		err   error
	}, 1)
	go func() {
		line, err := s.stdout.ReadString('\n')
		if err != nil {
			resultCh <- struct {
				value map[string]any
				err   error
			}{err: err}
			return
		}

		decoder := json.NewDecoder(strings.NewReader(strings.TrimSpace(line)))
		decoder.UseNumber()
		decoded := map[string]any{}
		if err := decoder.Decode(&decoded); err != nil {
			resultCh <- struct {
				value map[string]any
				err   error
			}{err: err}
			return
		}
		resultCh <- struct {
			value map[string]any
			err   error
		}{value: decoded}
	}()

	select {
	case result := <-resultCh:
		return result.value, result.err
	case <-time.After(timeout):
		return nil, fmt.Errorf("timed out waiting for stdio app-server message")
	}
}

func (s *interactiveStdioServer) close(t *testing.T) {
	t.Helper()

	if s.stdin != nil {
		_ = s.stdin.Close()
	}
	if s.done != nil {
		if err := <-s.done; err != nil {
			t.Fatalf("expected stdio server shutdown success, got %v", err)
		}
	}
}

func collectNotificationMethods(t *testing.T, server *interactiveStdioServer, count int, timeout time.Duration) []string {
	t.Helper()

	methods := make([]string, 0, count)
	for len(methods) < count {
		message, err := server.readMessage(t, timeout)
		if err != nil {
			t.Fatalf("expected app-server notification, got %v", err)
		}
		method, _ := message["method"].(string)
		if method == "" {
			continue
		}
		methods = append(methods, method)
	}
	return methods
}

func isRunSummaryUpsertNotification(message map[string]any, method string, runID string, expectedState string) bool {
	notificationMethod, _ := message["method"].(string)
	if notificationMethod != method {
		return false
	}

	params, ok := message["params"].(map[string]any)
	if !ok {
		return false
	}
	payload, ok := params["payload"].(map[string]any)
	if !ok {
		return false
	}
	if kind, _ := payload["kind"].(string); notificationMethod == "runs/changed" && kind != "upsert" {
		return false
	}
	runPayload, ok := payload["run"].(map[string]any)
	if !ok {
		return false
	}

	actualRunID, _ := runPayload["runId"].(string)
	actualState, _ := runPayload["state"].(string)
	return actualRunID == runID && actualState == expectedState
}

func isRunSummaryRemoveNotification(message map[string]any, method string, runID string) bool {
	notificationMethod, _ := message["method"].(string)
	if notificationMethod != method {
		return false
	}

	params, ok := message["params"].(map[string]any)
	if !ok {
		return false
	}
	payload, ok := params["payload"].(map[string]any)
	if !ok {
		return false
	}
	if kind, _ := payload["kind"].(string); notificationMethod == "runs/changed" && kind != "remove" {
		return false
	}

	actualRunID, _ := payload["runId"].(string)
	return actualRunID == runID
}

func waitForMessage(
	t *testing.T,
	server *interactiveStdioServer,
	timeout time.Duration,
	match func(map[string]any) bool,
) (map[string]any, error) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		message, err := server.readMessage(t, time.Until(deadline))
		if err != nil {
			return nil, err
		}
		if match(message) {
			return message, nil
		}
	}
	return nil, fmt.Errorf("timed out waiting for matching app-server message")
}

func collectMessagesForDuration(t *testing.T, server *interactiveStdioServer, duration time.Duration) []map[string]any {
	t.Helper()

	deadline := time.Now().Add(duration)
	messages := []map[string]any{}
	for time.Now().Before(deadline) {
		message, err := server.readMessage(t, time.Until(deadline))
		if err != nil {
			if strings.Contains(err.Error(), "timed out waiting for stdio app-server message") {
				return messages
			}
			t.Fatalf("expected app-server message drain success, got %v", err)
		}
		messages = append(messages, message)
	}
	return messages
}

func runEventAppendedSeqs(t *testing.T, messages []map[string]any) []int64 {
	t.Helper()

	seqs := []int64{}
	for _, message := range messages {
		method, _ := message["method"].(string)
		if method != "run/eventAppended" {
			continue
		}
		params, ok := message["params"].(map[string]any)
		if !ok {
			t.Fatalf("expected run/eventAppended params payload, got %T", message["params"])
		}
		seqValue, ok := params["seq"].(json.Number)
		if !ok {
			t.Fatalf("expected run/eventAppended seq number, got %T", params["seq"])
		}
		seq, err := seqValue.Int64()
		if err != nil {
			t.Fatalf("expected run/eventAppended seq int64, got %v", err)
		}
		seqs = append(seqs, seq)
	}
	return seqs
}

func initializeRequest(version string) string {
	return `{"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["` + version + `"],"client":{"name":"test-client","version":"1.0.0"}}}`
}

func containsString(values []string, expected string) bool {
	for _, value := range values {
		if value == expected {
			return true
		}
	}
	return false
}

type richRunFixture struct {
	RunsDir    string
	RunID      string
	RootNodeID string
	StepID     string
	ActionRef  string
}

func createRichRunFixture(t *testing.T) richRunFixture {
	t.Helper()

	runsDir := t.TempDir()
	lifecycle, err := runtime.NewLifecycleWithOptions(runtime.LifecycleOptions{
		Name:         "test-run",
		RunsBaseDir:  runsDir,
		QueuedSource: runtime.RunQueuedSourceCLIRunStart,
		MaxDepth:     3,
	})
	if err != nil {
		t.Fatalf("expected lifecycle creation success, got %v", err)
	}
	defer func() {
		_ = lifecycle.Close()
	}()

	if err := lifecycle.StartExecution(); err != nil {
		t.Fatalf("expected start execution success, got %v", err)
	}
	rootNode, err := lifecycle.RootNode()
	if err != nil {
		t.Fatalf("expected root node success, got %v", err)
	}
	stepStarted, err := lifecycle.AppendNodeStepStarted(rootNode.ID)
	if err != nil {
		t.Fatalf("expected node.step.started success, got %v", err)
	}
	childNode, err := lifecycle.CreateChildNode(rootNode.ID)
	if err != nil {
		t.Fatalf("expected child node creation success, got %v", err)
	}

	fixture := richRunFixture{
		RunsDir:    runsDir,
		RunID:      lifecycle.RunID(),
		RootNodeID: rootNode.ID,
		StepID:     stepStarted.StepID,
	}

	userTurnRef := fmt.Sprintf("run-artifact://node/%s/step/%s/turn-user.json", rootNode.ID, stepStarted.StepID)
	modelTurnRef := fmt.Sprintf("run-artifact://node/%s/step/%s/turn-model.json", rootNode.ID, stepStarted.StepID)
	stepAccountingRef := fmt.Sprintf("run-artifact://node/%s/step/%s/accounting.json", rootNode.ID, stepStarted.StepID)
	subcallAccountingRef := fmt.Sprintf("run-artifact://node/%s/step/%s/subcall-1-accounting.json", rootNode.ID, stepStarted.StepID)
	runAccountingRef := "run-artifact://run/accounting.json"
	finalAnswerRef := fmt.Sprintf("run-artifact://node/%s/final-answer.json", rootNode.ID)
	rootNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", rootNode.ID)
	childNodeAccountingRef := fmt.Sprintf("run-artifact://node/%s/accounting.json", childNode.ID)
	fixture.ActionRef, err = runtime.BuildActionArtifactRef(rootNode.ID, stepStarted.StepID, 1)
	if err != nil {
		t.Fatalf("expected action ref build success, got %v", err)
	}

	writeFixtureArtifact(t, runsDir, fixture.RunID, userTurnRef, map[string]any{
		"run_id":  fixture.RunID,
		"node_id": rootNode.ID,
		"step_id": stepStarted.StepID,
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, modelTurnRef, map[string]any{
		"run_id":            fixture.RunID,
		"node_id":           rootNode.ID,
		"step_id":           stepStarted.StepID,
		"schema_id":         "sigil.rlm.response.v1",
		"validated_payload": map[string]any{"decision": "final"},
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, fixture.ActionRef, map[string]any{
		"run_id":       fixture.RunID,
		"node_id":      rootNode.ID,
		"step_id":      stepStarted.StepID,
		"action_index": 1,
		"action_type":  "repl_code",
		"language":     "go",
		"status":       "completed",
		"stdout":       "fixture-stdout",
		"stderr":       "",
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, subcallAccountingRef, map[string]any{
		"run_id":        fixture.RunID,
		"node_id":       rootNode.ID,
		"step_id":       stepStarted.StepID,
		"subcall_index": 1,
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, stepAccountingRef, map[string]any{
		"run_id":  fixture.RunID,
		"node_id": rootNode.ID,
		"step_id": stepStarted.StepID,
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, rootNodeAccountingRef, map[string]any{
		"run_id":  fixture.RunID,
		"node_id": rootNode.ID,
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, childNodeAccountingRef, map[string]any{
		"run_id":  fixture.RunID,
		"node_id": childNode.ID,
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, finalAnswerRef, map[string]any{
		"run_id":       fixture.RunID,
		"node_id":      rootNode.ID,
		"final_answer": "fixture final answer",
	})
	writeFixtureArtifact(t, runsDir, fixture.RunID, runAccountingRef, map[string]any{
		"run_id": fixture.RunID,
	})

	if err := lifecycle.AppendNodeTurn(rootNode.ID, runtime.TurnRoleUser, stepStarted.StepID, userTurnRef, nil); err != nil {
		t.Fatalf("expected append node.turn.user success, got %v", err)
	}
	if err := lifecycle.AppendNodeTurn(rootNode.ID, runtime.TurnRoleModel, stepStarted.StepID, modelTurnRef, nil); err != nil {
		t.Fatalf("expected append node.turn.model success, got %v", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(childNode.ID, nil, testRollup(), &childNodeAccountingRef); err != nil {
		t.Fatalf("expected child node completion success, got %v", err)
	}
	if err := lifecycle.AppendNodeSubcallExecuted(rootNode.ID, runtime.NodeSubcallExecutedPayload{
		StepID:        stepStarted.StepID,
		ActionIndex:   1,
		SubcallIndex:  1,
		SubcallType:   runtime.SubcallTypeRLMQuery,
		ExecutionMode: runtime.SubcallExecutionModeRecursive,
		Status:        runtime.ActionExecutionStatusCompleted,
		Provider:      "openai",
		Model:         "gpt-5.1",
		PromptBytes:   12,
		ContextBytes:  34,
		AnswerBytes:   56,
		DurationMS:    78,
		ChildNodeID:   &childNode.ID,
		Accounting:    testSummary(),
		AccountingRef: subcallAccountingRef,
	}); err != nil {
		t.Fatalf("expected append node.subcall.executed success, got %v", err)
	}
	if err := lifecycle.AppendNodeActionExecuted(rootNode.ID, runtime.NodeActionExecutedPayload{
		StepID:      stepStarted.StepID,
		ActionIndex: 1,
		ActionType:  "repl_code",
		Language:    "go",
		Status:      runtime.ActionExecutionStatusCompleted,
		DurationMS:  91,
		ActionRef:   fixture.ActionRef,
	}); err != nil {
		t.Fatalf("expected append node.action.executed success, got %v", err)
	}
	if err := lifecycle.AppendNodeStepCompleted(rootNode.ID, runtime.NodeStepCompletedPayload{
		StepID:        stepStarted.StepID,
		Decision:      runtime.StepDecisionContinue,
		ActionCount:   1,
		DurationMS:    120,
		Accounting:    testRollup(),
		AccountingRef: stepAccountingRef,
	}); err != nil {
		t.Fatalf("expected append node.step.completed success, got %v", err)
	}
	if err := lifecycle.CompleteNodeWithAccounting(rootNode.ID, &finalAnswerRef, testRollup(), &rootNodeAccountingRef); err != nil {
		t.Fatalf("expected root node completion success, got %v", err)
	}
	if err := lifecycle.CompleteWithAccounting(&finalAnswerRef, testRollup(), &runAccountingRef); err != nil {
		t.Fatalf("expected run completion success, got %v", err)
	}

	return fixture
}

func writeFixtureArtifact(t *testing.T, runsDir string, runID string, artifactRef string, payload map[string]any) {
	t.Helper()

	relativeParts, err := runtime.ResolveArtifactRefPath(artifactRef)
	if err != nil {
		t.Fatalf("expected artifact ref path resolution success, got %v", err)
	}
	path := filepath.Join(append([]string{runsDir, runID, "artifacts"}, relativeParts...)...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("expected artifact directory creation success, got %v", err)
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("expected artifact encoding success, got %v", err)
	}
	if err := os.WriteFile(path, encoded, 0o644); err != nil {
		t.Fatalf("expected artifact write success, got %v", err)
	}
}

func testSummary() accounting.Summary {
	return accounting.UnavailableSummary("openai", "gpt-5.1", "test")
}

func testRollup() accounting.Rollup {
	return accounting.BuildRollup(
		"openai",
		"gpt-5.1",
		"test",
		testSummary(),
		accounting.ZeroSummary("openai", "gpt-5.1", "test"),
	)
}
