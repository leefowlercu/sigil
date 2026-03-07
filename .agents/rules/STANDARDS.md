# Standards

Follow the repo's Go standards first. This repository is not a place for alternate framework or layout conventions.

## Required Local Patterns

- Use TDD with the standard-library `testing` package for unit coverage.
- Keep acceptance coverage in Godog feature files plus step bindings.
- Use `log/slog` for logging.
- Keep typed Viper access inside `internal/config/`; prefer `config.Get().Section.Field` over string-key access.
- Keep Cobra commands organized under `cmd/` with `root.go`, parent-command directories, and `subcommands/`.
- Use package-scope flag variables bound with `*Var` helpers instead of repeated `Flags().Get*()` access.

## Repo-Specific Expectations

- Do not introduce a `pkg/` directory.
- Keep reusable implementation code in clearly named top-level directories that match current repo patterns.
- Preserve the existing split between CLI, runtime, harness, inference, REPL, and accounting packages.
- Match the current Cobra error-wrapping style with semicolons in `%w` messages for CLI initialization paths.

## Source Documents

- Superproject agent rules: `../../AGENTS.md`
- Architecture decisions: `../../docs/sigil/ADR/README.md`
- Product requirements: `../../docs/sigil/PRD/README.md`
