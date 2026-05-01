# Examples

This directory contains self-contained `sigil run start` examples.

- [`templated`](./templated/README.md): baseline templated
  example with a synthetic single-needle context.
- [`ruler-variable-tracking`](./ruler-variable-tracking/README.md):
  synthetic RULER-style mutable-state tracking over a noisy ledger.
- [`ruler-frequency-aggregation`](./ruler-frequency-aggregation/README.md):
  synthetic RULER-style top-k frequency aggregation across packet records.
- [`longbench-multidoc`](./longbench-multidoc/README.md):
  synthetic LongBench-style multi-document question answering.
- [`nolima-semantic-needle`](./nolima-semantic-needle/README.md):
  synthetic NoLiMa-style semantic needle retrieval with low lexical overlap.
- [`helmet-citation-rag`](./helmet-citation-rag/README.md):
  synthetic HELMET-style citation-grounded RAG decision.
- [`bright-rag-reasoning`](./bright-rag-reasoning/README.md):
  synthetic BRIGHT-style multi-hop technical RAG reasoning.
- [`mrcr-2needle`](./mrcr-2needle/README.md): pinned
  OpenAI MRCR example derived from `2needle/2needle_0.parquet`, row `0`.
- [`mrcr-4needle`](./mrcr-4needle/README.md): pinned
  OpenAI MRCR example derived from `4needle/4needle_0.parquet`, row `0`.
- [`mrcr-8needle`](./mrcr-8needle/README.md): pinned
  OpenAI MRCR example derived from `8needle/8needle_0.parquet`, row `0`.

The synthetic benchmark-inspired examples check in:

- `question.txt`: the runnable task prompt
- `context.txt`: the synthetic long-context corpus
- `expected-answer.txt`: the exact expected final answer
- `benchmark-metadata.json`: fixture provenance and design notes
- local `sigil.yaml`, `sigil-run.yaml`, and `README.md`

Each MRCR example checks in only derived assets:

- `question.txt`: the final benchmark user ask
- `context.txt`: the earlier transcript rendered as numbered role-tagged blocks
- `expected-answer.txt`: the exact benchmark answer
- `mrcr-metadata.json`: provenance and benchmark metadata
- local `sigil.yaml`, `sigil-run.yaml`, and `README.md`
