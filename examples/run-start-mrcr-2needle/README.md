# run-start-mrcr-2needle

This example is a pinned OpenAI MRCR showcase run for the 2-needle task.

The checked-in assets are derived from `openai/mrcr`,
`2needle/2needle_0.parquet`, row `0`. The source row contains `772` total
messages, `708925` prompt characters, and targets message index `721`.

The bundled `sigil-run.yaml` preserves the benchmark's native answer contract:
the model must return the exact benchmark answer with the required random
prefix and no extra wrapper text.

The run config uses `openai/gpt-5.3-codex` via OpenRouter, with reasoning
disabled, recursion enabled, and showcase-biased step/accounting guardrails.

## Prerequisites

- `OPENROUTER_API_KEY` must be set in the shell that runs the example.
- Build `sigil` first if `./sigil` is not already present.

## Default Human-Readable Run

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/run-start-mrcr-2needle/question.txt)"
context_value="$(cat ./examples/run-start-mrcr-2needle/context.txt)"

./sigil run start \
  --config ./examples/run-start-mrcr-2needle/sigil.yaml \
  --run-config ./examples/run-start-mrcr-2needle/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value"
```

## Machine-Readable JSON Run

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/run-start-mrcr-2needle/question.txt)"
context_value="$(cat ./examples/run-start-mrcr-2needle/context.txt)"

./sigil run start -o json \
  --config ./examples/run-start-mrcr-2needle/sigil.yaml \
  --run-config ./examples/run-start-mrcr-2needle/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value"
```

## Exact Answer Verification

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

question_value="$(cat ./examples/run-start-mrcr-2needle/question.txt)"
context_value="$(cat ./examples/run-start-mrcr-2needle/context.txt)"
output_json="$(mktemp)"

./sigil run start -o json \
  --config ./examples/run-start-mrcr-2needle/sigil.yaml \
  --run-config ./examples/run-start-mrcr-2needle/sigil-run.yaml \
  --var question="$question_value" \
  --var external_context="$context_value" >"$output_json"

python3 - "$output_json" ./examples/run-start-mrcr-2needle/expected-answer.txt <<'PY'
import json
import pathlib
import sys

actual = json.loads(pathlib.Path(sys.argv[1]).read_text(encoding="utf-8"))["final_answer"].rstrip("\n")
expected = pathlib.Path(sys.argv[2]).read_text(encoding="utf-8").rstrip("\n")
if actual != expected:
    raise SystemExit("final_answer mismatch")
print("final_answer matches expected-answer.txt")
PY
```

See `mrcr-metadata.json` for pinned provenance back to
<https://huggingface.co/datasets/openai/mrcr>.
