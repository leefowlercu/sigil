# Siĝil

Siĝil is a Go-based recursive language-model harness for running
long-context tasks with durable events, artifacts, accounting, guardrails, and
a local app-server protocol surface.

The executable has two primary entrypoints:

- `sigil run ...` executes, stops, lists, and inspects harness runs.
- `sigil app-server ...` serves the JSON-RPC app-server transport and
  generates protocol artifacts for clients.

## What Siĝil Does

Siĝil runs a configured prompt and context through a harness loop. Each run can
use model calls, local Go REPL actions, plain `llm_query` subcalls, and
recursive `rlm_query` subcalls to split, search, aggregate, and verify work over
large contexts.

Every run is persisted under a runs directory, by default `./.sigil/runs`.
The canonical event log is `events.jsonl`; supporting artifacts hold model
turn summaries, action output, recursive subcall accounting, final answers, and
run-level accounting.

The app-server exposes the same run corpus to local clients over stdio or
WebSocket using JSON-RPC 2.0. It supports read models, live subscriptions,
run start/stop, heartbeat notifications, and deterministic TypeScript and JSON
Schema generation from the typed protocol definitions.

## Quickstart

Prerequisites:

- Go `1.25` or newer.
- An OpenRouter API key exported in the shell that runs Siĝil.

Build the local executable:

```bash
make build
```

Run the bundled templated example:

```bash
export OPENROUTER_API_KEY="..."

context_value="$(cat ./examples/templated/context.txt)"

./sigil --config ./examples/templated/sigil.yaml run start \
  --run-config ./examples/templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```

The text output prints a preflight summary, append-only progress lines, and a
terminal summary with the run ID, final answer, accounting totals, and
`events_path`.

Inspect persisted runs:

```bash
./sigil run list
./sigil run status <run-id>
./sigil run inspect <run-id>
./sigil run events <run-id>
```

Use JSON output for scripts:

```bash
./sigil -o json run list
./sigil -o json run status <run-id>
./sigil -o json run inspect <run-id>
./sigil -o json run events <run-id>
```

## Configuration

Siĝil uses two YAML configuration files.

Application config defaults to `./sigil.yaml` and is controlled by the global
`--config` flag:

```bash
cp ./sigil.yaml.example ./sigil.yaml
./sigil --config ./sigil.yaml run list
```

Application config currently covers logs and app-server settings:

- `logs.level`
- `logs.dir`
- `app_server.instance_name`
- `app_server.instance_id`
- `app_server.run_dir`
- `app_server.allowed_origins`
- `app_server.websocket.listen_addr`
- `app_server.websocket.path`
- `app_server.health.ready_path`
- `app_server.health.live_path`
- `app_server.subscriptions.poll_interval_ms`
- `app_server.limits.max_connections`
- `app_server.limits.max_frame_bytes`

Environment variables prefixed with `SIGIL_` override application config
values. For example, `SIGIL_LOGS_LEVEL=debug` overrides `logs.level`.

Run config defaults to `./sigil-run.yaml` for `sigil run start`:

```bash
cp ./sigil-run.yaml.example ./sigil-run.yaml
./sigil run start --run-config ./sigil-run.yaml
```

Run config defines:

- `name`
- exactly one of `prompt` or `prompt_template`
- exactly one of `context` or `context_template`
- `system_prompt_append`
- `llm.provider`, `llm.model`, and `llm.gateway`
- `llm.reasoning`
- `llm.openrouter`
- `rlm.enabled` and `rlm.max_depth`
- deterministic `guardrails`
- accounting policy and fallback pricing

Environment variables prefixed with `SIGIL_RUN_` override run config values.
The default OpenRouter API key variable is `OPENROUTER_API_KEY`, but the run
config can change that through `llm.openrouter.api_key_env`.

## CLI Reference

Show root help:

```bash
./sigil --help
```

Global flags:

- `--config string`: path to the application config file. Default:
  `./sigil.yaml`.
- `-o, --output text|json`: output format. Default: `text`.

Run commands:

```bash
./sigil run start
./sigil run stop <run-id>
./sigil run list
./sigil run status <run-id>
./sigil run inspect <run-id>
./sigil run events <run-id>
```

The `run` command has an inherited `--run-dir` flag for selecting the runs base
directory:

```bash
./sigil run --run-dir /tmp/sigil-runs list
./sigil run --run-dir /tmp/sigil-runs status <run-id>
```

`sigil run start` runs in the foreground until the run reaches a terminal
state. It validates that the application config path is readable, resolves the
run config, renders templates with repeatable `--var key=value` inputs, and
persists events and artifacts under the selected runs directory.

`sigil run stop` writes stop-request metadata, sends `SIGTERM` to the active
local CLI process when present, and waits for a terminal state.

`sigil run list`, `status`, `inspect`, and `events` are read-only inspection
commands over persisted run state.

## App-Server

The app-server serves the local JSON-RPC 2.0 protocol version
`sigil.appserver.v1alpha1`.

Serve over stdio:

```bash
./sigil app-server serve --listen stdio://
```

Serve over WebSocket:

```bash
./sigil --config ./sigil.yaml app-server serve \
  --listen ws://127.0.0.1:8765
```

For browser clients, configure `app_server.allowed_origins` in `sigil.yaml` or
override it on the command line:

```bash
./sigil --config ./sigil.yaml app-server serve \
  --listen ws://127.0.0.1:8765 \
  --allowed-origin http://localhost:3000
```

WebSocket-specific flags:

- `--websocket-path`
- `--ready-path`
- `--live-path`
- `--allowed-origin`

The generated protocol artifacts are derived from the same typed method
definitions used by the server:

```bash
./sigil app-server generate-ts
./sigil app-server generate-ts --output-file ./generated/app_server.ts

./sigil app-server generate-json-schema
./sigil app-server generate-json-schema --output-file ./generated/app_server.schema.json
```

Implemented request methods include `initialize`, `server/ping`, `runs/list`,
`run/read`, `run/events/read`, `run/tree/read`, `run/steps/list`,
`run/node/read`, `run/step/read`, `run/artifact/read`, `run/start`,
`run/stop`, `run/subscribe`, `run/unsubscribe`, `runs/subscribe`, and
`runs/unsubscribe`.

Server notifications include `server/heartbeat`, `run/started`,
`run/eventAppended`, `run/statusChanged`, `run/completed`, and `runs/changed`.

## Runtime Outputs

By default, Siĝil writes runtime state under `./.sigil`:

```text
.sigil/
├── logs/
│   └── sigil.log
└── runs/
    └── <run-id>/
        ├── events.jsonl
        ├── process.json
        ├── stop-request.json
        └── artifacts/
```

`events.jsonl` is the canonical append-only event source for a run. Inspection
commands and app-server read models derive status, tree, step, and artifact
views from this persisted state.

Common artifact refs include:

- `run-artifact://run/accounting.json`
- `run-artifact://run/submitted-run-config.json`
- `run-artifact://node/<node-id>/context.json`
- `run-artifact://node/<node-id>/final-answer.json`
- `run-artifact://node/<node-id>/step/<step-id>/turn-user.json`
- `run-artifact://node/<node-id>/step/<step-id>/turn-model.json`
- `run-artifact://node/<node-id>/step/<step-id>/accounting.json`

## Examples

Starter config templates live at:

- [`sigil.yaml.example`](./sigil.yaml.example)
- [`sigil-run.yaml.example`](./sigil-run.yaml.example)

Runnable examples live under [`examples/`](./examples/README.md):

- `templated`: baseline templated run with external context supplied through
  `--var`.
- `ruler-variable-tracking`: synthetic RULER-style mutable-state tracking.
- `ruler-frequency-aggregation`: synthetic RULER-style frequency aggregation.
- `longbench-multidoc`: synthetic LongBench-style multi-document QA.
- `nolima-semantic-needle`: synthetic NoLiMa-style semantic needle retrieval.
- `helmet-citation-rag`: synthetic HELMET-style citation-grounded RAG.
- `bright-rag-reasoning`: synthetic BRIGHT-style multi-hop technical RAG.
- `mrcr-2needle`, `mrcr-4needle`, and `mrcr-8needle`: pinned OpenAI MRCR
  showcase examples.

Each example includes local config, prompt/context assets, expected answer, and
fixture metadata where relevant.

## Development

Run commands from the repository root of this subproject.

```bash
make help
make build
make install
make fmt
make tidy
make test-unit
make test-acceptance
make test
make verify
make clean
make clean-run
make clean-all
```

Useful direct Go entrypoints:

```bash
go run . --help
go test ./cmd/... ./internal/... -count=1
go test ./acceptance/... -count=1
go test ./... -count=1
```

Unit tests live under `cmd/` and `internal/`. Acceptance tests live under
`acceptance/` and use Godog feature files as their behavior source.

## Repository Layout

- `cmd/`: Cobra command tree and CLI-facing validation.
- `internal/config/`: typed Viper config structs, defaults, bootstrap, and
  validation.
- `internal/logging/`: `slog` logger setup and active log-file handling.
- `internal/runtime/`: lifecycle events, event persistence, run projection, and
  run control.
- `internal/harness/`: harness loop, system prompts, REPL integration,
  recursive subcalls, artifacts, accounting, and guardrails.
- `internal/inference/`: provider registry, schema registry, retries, and
  OpenRouter integration.
- `internal/repl/`: Go REPL session lifecycle and import handling.
- `internal/appserver/`: app-server transports, JSON-RPC dispatch, live
  subscriptions, read handlers, and protocol generation.
- `internal/accounting/`: token and cost accounting summaries, ledgers, and
  rollups.
- `acceptance/features/`: Godog feature source of truth for external behavior.
- `acceptance/steps/`: acceptance step bindings and fixtures.
- `examples/`: runnable run configs and benchmark-inspired fixtures.

## Specification Alignment

This repository is the production implementation for the Sigil specs in the
`project-sigil` superproject. Behavior changes should stay aligned with:

- `docs/sigil/ADR`
- `docs/sigil/PRD`
- `docs/sigil/PRD/MATRIX.md`
- `sigil/acceptance/features/*.feature`

When behavior changes, update the relevant PRD scenario, matrix row, and
acceptance scenario together.
