# Logging

Use application logging as structured operator observability, not as an ad hoc
debug print stream.

## Core Rules

- Use `log/slog` for application logging.
- Route application logs through the existing bootstrap in `internal/logging/`.
- Prefer `slog.Default().With(...)` or package-local helper loggers over
  one-off field names.
- Keep `component` in structured fields.
- Keep method names in structured fields.
- Keep domain error codes in structured fields.
- Keep run IDs, node IDs, and step IDs in structured fields when relevant.
- Keep artifact refs plus sequence/cursor values in structured fields when
  relevant.
- Log durable lifecycle transitions at `info`.
- Log rejected inputs, degraded behavior, write failures, and unexpected runtime
  conditions at `warn` or `error`.
- Keep high-volume request traces, heartbeat sends, and similar routine records
  at `debug` unless they indicate a problem.

## Data Handling Rules

- Do not log secrets, API keys, auth headers, or raw environment-sensitive
  values.
- Do not dump full prompts, large artifact bodies, or whole canonical event
  streams into the log sink.
- Prefer refs, IDs, counts, cursors, and sequence ranges over large inline
  payloads.
- Treat logs as supplemental evidence. Canonical runtime state must still live
  in the owned runtime/event contracts rather than in log text.

## Working Guidance

- Start from `internal/logging/` when changing sink behavior, log-file
  derivation, or logger bootstrap.
- Use component-scoped helper loggers when a subsystem has repeated records,
  such as CLI startup, runtime control, or app-server transport.
- When behavior depends on stable machine-readable failures, prefer logging the
  same domain code that the CLI or protocol returns.
- Add focused log assertions when a feature introduces important lifecycle or
  rejection records that operators will rely on.

## Reference Documents

- Logging bootstrap: `internal/logging/`
- Logging PRD: `../../docs/sigil/PRD/PRD-0140-sigil-application-logging-specification.md`
- Application config PRD: `../../docs/sigil/PRD/PRD-0100-sigil-application-config-specification.md`
