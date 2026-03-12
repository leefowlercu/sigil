# Examples

This directory contains self-contained `sigil run start` examples.

- [`run-start-templated`](./run-start-templated/README.md): baseline templated
  example with a synthetic single-needle context.
- [`run-start-mrcr-2needle`](./run-start-mrcr-2needle/README.md): pinned
  OpenAI MRCR example derived from `2needle/2needle_0.parquet`, row `0`.
- [`run-start-mrcr-4needle`](./run-start-mrcr-4needle/README.md): pinned
  OpenAI MRCR example derived from `4needle/4needle_0.parquet`, row `0`.
- [`run-start-mrcr-8needle`](./run-start-mrcr-8needle/README.md): pinned
  OpenAI MRCR example derived from `8needle/8needle_0.parquet`, row `0`.

Each MRCR example checks in only derived assets:

- `question.txt`: the final benchmark user ask
- `context.txt`: the earlier transcript rendered as numbered role-tagged blocks
- `expected-answer.txt`: the exact benchmark answer
- `mrcr-metadata.json`: provenance and benchmark metadata
- local `sigil.yaml`, `sigil-run.yaml`, and `README.md`
