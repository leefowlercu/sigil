# AGENTS.md

Siĝil is a Go-based RLM harness that runs spec-governed workflows through a Cobra CLI, typed Viper config, and an evented runtime. This subproject contains the production implementation that must stay aligned with the `docs/sigil/` ADR, PRD, and acceptance traceability contracts in the superproject.

## Table of Contents

- [Development Commands](#development-commands)
- [Repository Layout](#repository-layout)
- [Code Standards](#code-standards)
- [Logging](#logging)
- [Specifications](#specifications)
- [Testing](#testing)
- [Change Control](#change-control)

## Development Commands

Use the documented local command surface before inventing ad hoc build or test flows. Run commands from the `sigil/` subproject root unless a document says otherwise.

- [Development Commands Reference](.agents/rules/COMMANDS.md)

## Repository Layout

Use the layout guide to find the CLI, runtime, harness, acceptance, and example entrypoints quickly before changing code. It explains where behavior lives and which directories are implementation-only.

- [Repository Layout Reference](.agents/rules/LAYOUT.md)

## Code Standards

Follow the local implementation standards for Go structure, Cobra command shape, typed Viper config, and logging. This keeps generated code aligned with the repo's required patterns instead of generic Go defaults.

- [Code Standards Reference](.agents/rules/STANDARDS.md)

## Logging

Use the logging guide when a change emits or depends on application logs. It explains how `sigil` expects structured `slog` records to be routed, how to choose log levels, and which payloads must stay out of the log sink.

- [Logging Reference](.agents/rules/LOGGING.md)

## Specifications

Treat the superproject specs as the behavioral source of truth for this submodule. Use the spec guide to see which ADR, PRD, matrix, and acceptance files must move together when behavior changes.

- [Specification Reference](.agents/rules/SPECS.md)

## Testing

Use the testing guide to keep unit and acceptance coverage separate, deterministic, and spec-aligned. Update the failing contract first, then run the smallest relevant verification path before broader suites.

- [Testing Reference](.agents/rules/TESTING.md)

## Change Control

Use the change-control guide for repo-boundary, commit, and submodule-pointer rules. It explains what can be changed locally in `sigil/` and which actions still require explicit user authorization.

- [Change Control Reference](.agents/rules/CHANGES.md)
