# templated

This example runs `sigil run start` with template variables provided via
`--var key=value`, including external context loaded from a file.

The bundled `context.txt` is intentionally large (~350KB) and contains one true
needle token among many noisy chunks and decoys.

The example question is intentionally written as just the task to solve. It
does not mention harness internals like recursion, steps, or `rlm_query`.

The bundled `sigil-run.yaml` is configured for `openai/gpt-5.3-codex`.
Its fallback pricing matches the OpenRouter price card for
`openai/gpt-5.3-codex`, which also matches the OpenAI model page as of
March 7, 2026: `$1.75` input and `$14.00` output per 1M tokens.

The bundled `sigil-run.yaml` also sets generous cumulative accounting budgets:

- `guardrails.max_total_tokens: 1000000`
- `guardrails.max_total_cost_usd: "5"`

Those limits apply to cumulative run `tree_total` accounting. They are meant to
bound the example if recursion runs away without being tight enough to block a
normal successful search over the bundled context. If you expand the context or
push recursion harder, raise those values first.

## Prerequisites

- `OPENROUTER_API_KEY` must be set in the shell that runs the example.
- Build `sigil` first if `./sigil` is not already present.

## Default Human-Readable Run

By default, `sigil run start` now writes human-readable text to stdout:

- a preflight summary showing resolved config and execution settings
- live append-only progress lines as the harness runs
- a terminal run summary with the final answer, `events_path`, and accounting

If either cumulative accounting budget is breached, the run fails with
`harness_limit_exceeded` and includes the configured and observed totals in
`run.failed`.

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

context_value="$(cat ./examples/templated/context.txt)"

./sigil run start \
  --config ./examples/templated/sigil.yaml \
  --run-config ./examples/templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```

The final summary prints the resolved `events_path`. The persisted event log
still lives under `./.sigil/runs/<run-id>/events.jsonl`.

## Machine-Readable JSON Run

Use `-o json` when the result needs to be consumed by another program:

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

context_value="$(cat ./examples/templated/context.txt)"

./sigil run start -o json \
  --config ./examples/templated/sigil.yaml \
  --run-config ./examples/templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```

`-o json` preserves the structured success payload with `run_id`, `state`,
`final_answer`, `final_answer_ref`, `accounting`, and `events_path`.
