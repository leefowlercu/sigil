# ruler-frequency-aggregation

This example asks the harness to count repeated signal tokens across many packets.

This is a synthetic, checked-in fixture inspired by RULER common/frequent words extraction. It is not
copied from the upstream benchmark. The goal is to give Sigil a
repeatable long-context example that stresses the same kind of harness
behavior while keeping the answer contract small enough to inspect.

The bundled `sigil-run.yaml` uses `openai/gpt-5.3-codex` via OpenRouter,
with reasoning disabled, recursion enabled, and showcase-biased
step/accounting guardrails.

## Prerequisites

- `OPENROUTER_API_KEY` must be set in the shell that runs the example.
- Build `sigil` first if `./sigil` is not already present.

## Default Human-Readable Run

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/ruler-frequency-aggregation/question.txt)"
context_value="$(cat ./examples/ruler-frequency-aggregation/context.txt)"

./sigil run start \
  --config ./examples/ruler-frequency-aggregation/sigil.yaml \
  --run-config ./examples/ruler-frequency-aggregation/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value"
```

## Machine-Readable JSON Run

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/ruler-frequency-aggregation/question.txt)"
context_value="$(cat ./examples/ruler-frequency-aggregation/context.txt)"

./sigil run start -o json \
  --config ./examples/ruler-frequency-aggregation/sigil.yaml \
  --run-config ./examples/ruler-frequency-aggregation/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value"
```

## Expected Answer Verification

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/ruler-frequency-aggregation/question.txt)"
context_value="$(cat ./examples/ruler-frequency-aggregation/context.txt)"
output_json="$(mktemp)"

./sigil run start -o json \
  --config ./examples/ruler-frequency-aggregation/sigil.yaml \
  --run-config ./examples/ruler-frequency-aggregation/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value" >"$output_json"

python3 - "$output_json" ./examples/ruler-frequency-aggregation/expected-answer.txt <<'PY'
import json
import pathlib
import sys

actual = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))["final_answer"].strip()
expected = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8").strip()
if actual != expected:
    raise SystemExit("final_answer mismatch")
print("final_answer matches expected-answer.txt")
PY
```

See `benchmark-metadata.json` for fixture provenance and design notes.
