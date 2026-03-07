# Testing

Testing is split into fast implementation checks and black-box acceptance coverage. Choose the narrowest lane that proves the change, then widen to full verification before concluding.

## Unit Tests

- Keep unit tests in the standard `testing` package.
- Prefer table-driven tests when they improve clarity.
- Keep unit tests deterministic and focused on package behavior.
- Use `go test ./cmd/... ./internal/... -count=1` or `make test-unit` for fast loops.

## Acceptance Tests

- Treat `acceptance/features/harness.feature` as the external behavior source of truth.
- Update or add the failing scenario first when behavior changes.
- Keep assertions black-box and observable through CLI output, files, or runtime artifacts.
- Use `go test ./acceptance/... -count=1` or `make test-acceptance` for acceptance verification.

## Full Verification

- Use `make test` or `go test ./... -count=1` before concluding implementation work.
- Use `make verify` when you also need formatting and module hygiene checked.
- Re-run uncached tests after changes to config loading, harness execution, runtime events, or acceptance fixtures.
