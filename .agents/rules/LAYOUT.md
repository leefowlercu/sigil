# Layout

This submodule follows a small Go application layout with the CLI at the edge, implementation under `internal/`, and black-box acceptance coverage under `acceptance/`.

## Top-Level Directories

- `cmd/`: Cobra command tree and CLI-facing validation.
- `internal/config/`: typed Viper config structs, defaults, bootstrap, and validation.
- `internal/logging/`: `slog` logger setup and active log-file handling.
- `internal/runtime/`: run lifecycle, event contracts, and persistence.
- `internal/harness/`: harness control loop, step execution, prompts, artifacts, and guardrails.
- `internal/inference/`: provider registry, schema registry, retries, and gateway integrations.
- `internal/repl/`: Go REPL session lifecycle and import handling.
- `internal/accounting/`: accounting summaries, ledger helpers, and rollup validation.
- `acceptance/features/`: Godog feature source of truth for submodule behavior.
- `acceptance/steps/`: step bindings and test fixtures for acceptance scenarios.
- `examples/`: example config and prompt/context assets.
- `.sigil/`: local runtime artifacts such as logs and run outputs.

## Working Guidance

- Start from `cmd/` when changing CLI behavior.
- Start from `internal/config/` for config shape, defaults, env overrides, or validation.
- Start from `internal/runtime/` and `internal/harness/` for run execution or eventing behavior.
- Start from `acceptance/features/harness.feature` when the change is user-visible or spec-driven.
