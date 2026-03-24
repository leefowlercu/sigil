Feature: Sigil app-server transport and protocol tooling
  The Sigil app-server exposes a stdio transport plus deterministic
  protocol artifact generation for client integration work.

  Background:
    Given a clean sigil working directory
    And SIGIL configuration environment variables are cleared

  Scenario: Exposes app-server subcommands under sigil
    Given the sigil executable is available
    When a user runs `sigil app-server --help`
    Then command exits with status code 0
    And command output contains `serve`
    And command output contains `generate-ts`
    And command output contains `generate-json-schema`

  Scenario: Rejects app-server requests before initialize on stdio
    Given the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"server/ping","params":{}}
      """
    Then the app-server response error code is -32600
    And the app-server response data.code is "handshake_required"

  Scenario: Rejects repeated initialize on stdio
    Given the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"2","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    Then the app-server response error code is -32600
    And the app-server response data.code is "already_initialized"

  Scenario: Serves server ping after initialize handshake on stdio
    Given the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"2","method":"server/ping","params":{}}
      """
    Then the app-server response result field "serverName" is "sigil"
    And the app-server response result field "protocolVersion" is "sigil.appserver.v1alpha1"

  Scenario: Serves server ping after initialize handshake on WebSocket
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        allowed_origins:
          - http://localhost:3000
      """
    And the app-server websocket transport is running
    When the websocket app-server client connects with origin "http://localhost:3000"
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"2","method":"server/ping","params":{}}
      """
    Then the app-server response result field "serverName" is "sigil"
    And the app-server response result field "protocolVersion" is "sigil.appserver.v1alpha1"

  Scenario: Exposes ready and live health endpoints while WebSocket listener is serving
    Given the app-server websocket transport is running
    Then the app-server ready and live endpoints return success

  Scenario: Emits heartbeat notifications on idle WebSocket connections
    Given the app-server websocket transport is running
    When the websocket app-server client connects without an origin header
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    Then the websocket app-server client receives JSON-RPC notification method "server/heartbeat"

  Scenario: Reconnects and resumes run subscribe after missed heartbeat on WebSocket without gaps
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server websocket transport is running
    And the websocket app-server client routes through a controllable proxy
    When the websocket app-server client connects without an origin header
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the websocket app-server client requests run/subscribe for fixture run "liveRunId"
    Then the app-server response JSON pointer "/result/snapshotAsOfSeq" equals integer 1
    And the app-server response JSON pointer "/result/payload/snapshot/terminal" equals boolean false
    And the websocket app-server client receives JSON-RPC notification method "server/heartbeat"
    When the websocket app-server proxy stops forwarding server frames
    Then the websocket app-server client detects a missed heartbeat window
    When the websocket app-server proxy resumes forwarding server frames
    And the websocket app-server client reconnects
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"3","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the websocket app-server client requests run/subscribe for fixture run "liveRunId" after seq 1
    Then the app-server response JSON pointer "/result/snapshotAsOfSeq" equals integer 1
    And the app-server response JSON pointer "/result/payload/terminal" equals boolean false
    When the queued live app-server fixture starts execution
    Then the websocket app-server client receives 2 unique "run/eventAppended" notification seq values greater than 1

  Scenario: Serves fresh runs subscribe snapshot after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/subscribe
    Then the app-server response JSON pointer "/result/payload/items" has length 2
    And the app-server response JSON pointer "/result/payload/items/0/runId" equals fixture "primaryRunId"
    And the app-server response JSON pointer "/result/payload/items/1/runId" equals fixture "secondaryRunId"

  Scenario: Delivers live runs changed upserts for new or changed runs after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/subscribe
    Then the app-server response JSON pointer "/result/payload/items" has length 1
    And the app-server response JSON pointer "/result/payload/items/0/runId" equals fixture "liveRunId"
    And the app-server response JSON pointer "/result/payload/items/0/state" equals "queued"
    When the queued live app-server fixture starts execution
    Then the stdio app-server client receives 1 "runs/changed" upsert notifications for fixture run "liveRunId" with state "running"
    When the queued live app-server fixture completes execution
    Then the stdio app-server client receives 1 "runs/changed" upsert notifications for fixture run "liveRunId" with state "completed"

  Scenario: Delivers live runs changed removals when a persisted run disappears after subscribe
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/subscribe
    When the persisted app-server fixture run "primaryRunId" directory is removed
    Then the stdio app-server client receives 1 "runs/changed" remove notifications for fixture run "primaryRunId"

  Scenario: Unsubscribes runs subscribe idempotently after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/subscribe
    And the app-server requests runs/unsubscribe
    Then the app-server response JSON pointer "/result/payload/unsubscribed" equals boolean true
    When the app-server requests runs/unsubscribe
    Then the app-server response JSON pointer "/result/payload/unsubscribed" equals boolean false

  Scenario: Replaces duplicate runs subscribe requests without duplicate live delivery after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/subscribe
    And the app-server requests runs/subscribe
    When the queued live app-server fixture starts execution
    Then the stdio app-server client receives 1 "runs/changed" upsert notifications for fixture run "liveRunId" with state "running"

  Scenario: Reconnects and resubscribes runs subscribe after missed heartbeat on WebSocket with fresh authoritative snapshot
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server websocket transport is running
    And the websocket app-server client routes through a controllable proxy
    When the websocket app-server client connects without an origin header
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the websocket app-server client requests runs/subscribe
    Then the app-server response JSON pointer "/result/payload/items" has length 1
    And the app-server response JSON pointer "/result/payload/items/0/runId" equals fixture "liveRunId"
    And the app-server response JSON pointer "/result/payload/items/0/state" equals "queued"
    And the websocket app-server client receives JSON-RPC notification method "server/heartbeat"
    When the websocket app-server proxy stops forwarding server frames
    Then the websocket app-server client detects a missed heartbeat window
    When the websocket app-server proxy resumes forwarding server frames
    And the websocket app-server client reconnects
    And the websocket app-server client sends JSON-RPC message:
      """
      {"jsonrpc":"2.0","id":"3","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the websocket app-server client sends JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the websocket app-server client requests runs/subscribe
    Then the app-server response JSON pointer "/result/payload/items" has length 1
    And the app-server response JSON pointer "/result/payload/items/0/runId" equals fixture "liveRunId"
    And the app-server response JSON pointer "/result/payload/items/0/state" equals "queued"
    When the queued live app-server fixture starts execution
    Then the websocket app-server client receives 1 "runs/changed" upsert notifications for fixture run "liveRunId" with state "running"

  Scenario: Generates deterministic TypeScript bindings from typed app-server protocol definitions
    When TypeScript app-server bindings are generated twice deterministically
    Then command output contains `AppServerMethodMap`

  Scenario: Generates deterministic JSON Schema bundles from typed app-server protocol definitions
    When JSON Schema app-server bundles are generated twice deterministically
    Then command output contains `"methods"`

  Scenario: Serves paged runs list after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests runs/list with limit 1
    Then the app-server response JSON pointer "/result/payload/items" has length 1
    And the app-server response JSON pointer "/result/payload/items/0/runId" equals fixture "primaryRunId"
    And the app-server response JSON pointer "/result/payload/nextCursor" equals fixture "primaryRunId"

  Scenario: Serves run projection and canonical event history after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests "run/read" for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/runId" equals fixture "primaryRunId"
    And the app-server response JSON pointer "/result/asOfSeq" is a positive integer
    And the app-server response JSON pointer "/result/payload/run/runId" equals fixture "primaryRunId"
    When the app-server requests "run/events/read" for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/payload/events" has length 13
    And the app-server response JSON pointer "/result/payload/firstSeq" is a positive integer
    And the app-server response JSON pointer "/result/payload/events/0/type" equals "run.queued"

  Scenario: Serves node tree and node linkage after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests "run/tree/read" for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/payload/rootNodeId" equals fixture "rootNodeId"
    And the app-server response JSON pointer "/result/payload/nodes" has length 2
    And the app-server response JSON pointer "/result/payload/nodes/0/nodeId" equals fixture "rootNodeId"
    And the app-server response JSON pointer "/result/payload/nodes/0/childNodeIds/0" equals fixture "childNodeId"
    When the app-server requests run/node/read for fixture run "primaryRunId" node "rootNodeId"
    Then the app-server response JSON pointer "/result/payload/node/stepIds/0" equals fixture "stepId"
    And the app-server response JSON pointer "/result/payload/node/childNodeIds/0" equals fixture "childNodeId"

  Scenario: Serves run-global step timeline after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests "run/steps/list" for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/asOfSeq" is a positive integer
    And the app-server response JSON pointer "/result/payload/steps" has length 1
    And the app-server response JSON pointer "/result/payload/steps/0/stepId" equals fixture "stepId"
    And the app-server response JSON pointer "/result/payload/steps/0/schemaId" equals "sigil.rlm.response.v1"

  Scenario: Serves step detail and typed artifact bodies after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/step/read for fixture run "primaryRunId" node "rootNodeId" step "stepId"
    Then the app-server response JSON pointer "/result/payload/step/actionRefs/0" equals fixture "actionRef"
    And the app-server response JSON pointer "/result/payload/step/schemaId" equals "sigil.rlm.response.v1"
    When the app-server requests run/artifact/read for fixture run "primaryRunId" artifact "actionRef"
    Then the app-server response JSON pointer "/result/payload/artifactKind" equals "action"
    And the app-server response JSON pointer "/result/payload/identity/nodeId" equals fixture "rootNodeId"
    And the app-server response JSON pointer "/result/payload/identity/actionIndex" is a positive integer
    And the app-server response JSON pointer "/result/payload/artifact/stdout" equals "fixture-stdout"

  Scenario: Starts a queued app-server run from inline YAML after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/start with inline YAML:
      """
      name: test-run
      prompt: hello from app-server
      context: acceptance context
      llm:
        provider: openai
        model: gpt-5.1
      """
    Then the app-server response JSON pointer "/result/asOfSeq" is a positive integer
    And the app-server response JSON pointer "/result/payload/run/state" equals "queued"
    And the app-server response JSON pointer "/result/payload/run/source" equals "app_server.run.start"

  Scenario: Serves fresh attach snapshot for run subscribe after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/subscribe for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/snapshotAsOfSeq" is a positive integer
    And the app-server response JSON pointer "/result/payload/snapshot/terminal" equals boolean true
    And the app-server response JSON pointer "/result/payload/snapshot/tree/rootNodeId" equals fixture "rootNodeId"

  Scenario: Replays canonical events for run subscribe resume attach after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/subscribe for fixture run "primaryRunId" after seq 11
    Then the app-server response JSON pointer "/result/payload/replayEvents" has length 2
    And the app-server response JSON pointer "/result/payload/replayEvents/0/seq" equals integer 12
    And the app-server response JSON pointer "/result/payload/terminal" equals boolean true

  Scenario: Unsubscribes idempotently after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/subscribe for fixture run "primaryRunId"
    And the app-server requests run/unsubscribe for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/payload/unsubscribed" equals boolean true
    When the app-server requests run/unsubscribe for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/payload/unsubscribed" equals boolean false

  Scenario: Replaces duplicate run subscribe requests without duplicate live delivery after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/subscribe for fixture run "liveRunId"
    And the app-server requests run/subscribe for fixture run "liveRunId"
    When the queued live app-server fixture starts execution
    Then the stdio app-server client receives 2 unique "run/eventAppended" notification seq values

  Scenario: Returns terminal no-op run stop results after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
      """
    And persisted app-server run fixtures exist in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests "run/stop" for fixture run "primaryRunId"
    Then the app-server response JSON pointer "/result/payload/stopRequested" equals boolean false
    And the app-server response JSON pointer "/result/payload/state" equals "completed"

  Scenario: Delivers live canonical notifications for subscribed runs after initialize handshake on stdio
    Given application config exists at "sigil.yaml" with:
      """
      app_server:
        run_dir: ./custom-runs
        subscriptions:
          poll_interval_ms: 25
      """
    And a queued live app-server fixture exists in the configured run directory
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/subscribe for fixture run "liveRunId"
    Then the app-server response JSON pointer "/result/payload/snapshot/terminal" equals boolean false
    When the queued live app-server fixture starts execution
    Then the stdio app-server client receives JSON-RPC notification method "run/eventAppended"

  Scenario: Stops an actively running local CLI run after initialize handshake on stdio
    Given a local CLI run is actively executing for app-server stop control
    And the app-server stdio transport is running
    When the app-server receives JSON-RPC line:
      """
      {"jsonrpc":"2.0","id":"1","method":"initialize","params":{"protocolVersions":["sigil.appserver.v1alpha1"],"client":{"name":"acceptance","version":"1.0.0"}}}
      """
    And the app-server receives JSON-RPC notification:
      """
      {"jsonrpc":"2.0","method":"initialized","params":{}}
      """
    And the app-server requests run/stop for the active run
    Then the app-server response JSON pointer "/result/payload/stopRequested" equals boolean true
    And the app-server response JSON pointer "/result/payload/state" equals "interrupted"
    And the active run transitions to "interrupted"
    And the active run stop-request metadata requested_by is "app_server.run.stop"
    And run.interrupted contains reason user_request interrupted_by "app_server.run.stop" and partial accounting
