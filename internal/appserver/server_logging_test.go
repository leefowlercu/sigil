package appserver

import (
	"fmt"
	"testing"

	"github.com/leefowlercu/sigil/internal/appserver/protocol"
	"github.com/leefowlercu/sigil/internal/config"
	"github.com/leefowlercu/sigil/internal/runtime"
)

func TestServeStdioLogsInitializeLifecycle(t *testing.T) {
	sink := captureTestLogs(t)

	responses := serveJSONLines(t,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		`{"jsonrpc":"2.0","id":"2","method":"server/ping","params":{}}`,
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}

	initializeRecord := waitForTestLogRecord(t, sink, func(record map[string]any) bool {
		return record["msg"] == "app-server connection initialized"
	})
	if initializeRecord["component"] != "appserver.server" {
		t.Fatalf("expected appserver.server component, got %v", initializeRecord["component"])
	}
	if initializeRecord["transport"] != "stdio" {
		t.Fatalf("expected stdio transport, got %v", initializeRecord["transport"])
	}
	if initializeRecord["client_name"] != "test-client" {
		t.Fatalf("expected test-client, got %v", initializeRecord["client_name"])
	}
	if initializeRecord["protocol_version"] != protocol.DefaultProtocolVersion() {
		t.Fatalf("expected protocol version %q, got %v", protocol.DefaultProtocolVersion(), initializeRecord["protocol_version"])
	}

	readyRecord := waitForTestLogRecord(t, sink, func(record map[string]any) bool {
		return record["msg"] == "app-server connection ready"
	})
	if readyRecord["transport"] != "stdio" {
		t.Fatalf("expected stdio transport, got %v", readyRecord["transport"])
	}
}

func TestServeStdioLogsHandshakeRejection(t *testing.T) {
	sink := captureTestLogs(t)

	responses := serveJSONLines(t,
		`{"jsonrpc":"2.0","id":"1","method":"server/ping","params":{}}`,
	)

	if len(responses) != 1 {
		t.Fatalf("expected one response, got %d", len(responses))
	}

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "rejected app-server request" && entry["domain_code"] == "handshake_required"
	})
	if record["level"] != "WARN" {
		t.Fatalf("expected WARN level, got %v", record["level"])
	}
	if record["transport"] != "stdio" {
		t.Fatalf("expected stdio transport, got %v", record["transport"])
	}
	if record["rpc_method"] != "server/ping" {
		t.Fatalf("expected server/ping method, got %v", record["rpc_method"])
	}
}

func TestServeStdioLogsRunStepsListReadSummary(t *testing.T) {
	sink := captureTestLogs(t)
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

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "completed app-server read request" &&
			entry["rpc_method"] == "run/steps/list"
	})
	if record["run_id"] != fixture.RunID {
		t.Fatalf("expected run id %q, got %v", fixture.RunID, record["run_id"])
	}
	if record["query_source"] != "run_step_timeline" {
		t.Fatalf("expected query_source run_step_timeline, got %v", record["query_source"])
	}
	if record["as_of_seq"] != float64(13) {
		t.Fatalf("expected as_of_seq 13, got %v", record["as_of_seq"])
	}
	if record["item_count"] != float64(1) {
		t.Fatalf("expected item_count 1, got %v", record["item_count"])
	}
}

func TestServeStdioLogsRunListCursorValidationFailures(t *testing.T) {
	sink := captureTestLogs(t)
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	missingCursor := "019c97f5-4e56-70d3-8421-3fd92033094d"
	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/list","params":{"cursor":"%s"}}`, missingCursor),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error == nil {
		t.Fatal("expected run/list cursor error response")
	}

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "app-server read request failed" &&
			entry["rpc_method"] == "run/list" &&
			entry["domain_code"] == "invalid_cursor"
	})
	if record["cursor"] != missingCursor {
		t.Fatalf("expected cursor %q, got %v", missingCursor, record["cursor"])
	}
	if record["query_source"] != "run_list_page" {
		t.Fatalf("expected query_source run_list_page, got %v", record["query_source"])
	}
}

func TestServeStdioLogsArtifactNotFoundTargets(t *testing.T) {
	sink := captureTestLogs(t)
	fixture := createRichRunFixture(t)

	cfg := config.NewDefaultConfig()
	cfg.AppServer.RunDir = fixture.RunsDir

	missingArtifactRef, err := runtime.BuildActionArtifactRef(fixture.RootNodeID, fixture.StepID, 2)
	if err != nil {
		t.Fatalf("expected missing artifact ref build success, got %v", err)
	}

	responses := serveJSONLinesWithConfig(t, cfg,
		initializeRequest(protocol.DefaultProtocolVersion()),
		`{"jsonrpc":"2.0","method":"initialized","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","id":"2","method":"run/artifact/read","params":{"runId":"%s","artifactRef":"%s"}}`, fixture.RunID, missingArtifactRef),
	)

	if len(responses) != 2 {
		t.Fatalf("expected two responses, got %d", len(responses))
	}
	if responses[1].Error == nil {
		t.Fatal("expected artifact not found error response")
	}

	record := waitForTestLogRecord(t, sink, func(entry map[string]any) bool {
		return entry["msg"] == "app-server read request failed" &&
			entry["rpc_method"] == "run/artifact/read" &&
			entry["domain_code"] == "artifact_not_found"
	})
	if record["run_id"] != fixture.RunID {
		t.Fatalf("expected run id %q, got %v", fixture.RunID, record["run_id"])
	}
	if record["artifact_ref"] != missingArtifactRef {
		t.Fatalf("expected artifact ref %q, got %v", missingArtifactRef, record["artifact_ref"])
	}
	if record["query_source"] != "canonical_artifact" {
		t.Fatalf("expected query_source canonical_artifact, got %v", record["query_source"])
	}
}
