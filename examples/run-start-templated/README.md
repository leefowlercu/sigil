# run-start-templated

This example runs `sigil run start` with template variables provided via
`--var key=value`, including external context loaded from a file.

The bundled `context.txt` is intentionally large (~350KB) and contains one true
needle token among many noisy chunks and decoys.

```bash
cd /Users/lee/Dev/project/project-sigil/sigil

context_value="$(cat ./examples/run-start-templated/context.txt)"

./sigil run start \
  --config ./examples/run-start-templated/sigil.yaml \
  --run-config ./examples/run-start-templated/sigil-run.yaml \
  --var question="Find the one true token matching SIGIL-NEEDLE-2026-03-03-ALPHA-OMEGA-[0-9]{4}. Tokens ending with XXXX are decoys and invalid. Use recursive decomposition with REPL + rlm_query before final answer. Return exactly: token=<value>; chunk=<chunk-id>; evidence=<full line containing token>." \
  --var external_context="$context_value"
```
