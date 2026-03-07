# run-start-templated

This example runs `sigil run start` with template variables provided via
`--var key=value`, including external context loaded from a file.

The bundled `context.txt` is intentionally large (~350KB) and contains one true
needle token among many noisy chunks and decoys.

## Prerequisites

- `OPENROUTER_API_KEY` must be set in the shell that runs the example.
- Build `sigil` first if `./sigil` is not already present.

## Default Human-Readable Run

By default, `sigil run start` now writes human-readable text to stdout:

- a preflight summary showing resolved config and execution settings
- live append-only progress lines as the harness runs
- a terminal run summary with the final answer, `events_path`, and accounting

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

context_value="$(cat ./examples/run-start-templated/context.txt)"

./sigil run start \
  --config ./examples/run-start-templated/sigil.yaml \
  --run-config ./examples/run-start-templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Use recursive decomposition with REPL + rlm_query before final answer. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```

The final summary prints the resolved `events_path`. The persisted event log
still lives under `./.sigil/runs/<run-id>/events.jsonl`.

## Machine-Readable JSON Run

Use `-o json` when the result needs to be consumed by another program:

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

context_value="$(cat ./examples/run-start-templated/context.txt)"

./sigil run start -o json \
  --config ./examples/run-start-templated/sigil.yaml \
  --run-config ./examples/run-start-templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Use recursive decomposition with REPL + rlm_query before final answer. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```

`-o json` preserves the structured success payload with `run_id`, `state`,
`final_answer`, `final_answer_ref`, `accounting`, and `events_path`.
