# Specs

The behavioral source of truth for `sigil/` lives in the superproject docs, not inside ad hoc code comments or tests.

## Primary Spec Sources

- ADR index: `../../docs/sigil/ADR/README.md`
- PRD index: `../../docs/sigil/PRD/README.md`
- Traceability matrix: `../../docs/sigil/PRD/MATRIX.md`
- Submodule acceptance file: `../acceptance/features/harness.feature`

## Update Rules

- Update PRD acceptance criteria before or alongside implementation changes.
- Keep PRD scenario IDs in `SCN-xxxx` form and preserve exact title alignment with the mapped acceptance scenario when behavior is unchanged.
- Update `docs/sigil/PRD/MATRIX.md` and `acceptance/features/harness.feature` in the same change when a mapped behavior changes.
- Update ADRs when architectural direction or a long-lived technical tradeoff changes.

## Verification Rule

- Run `./scripts/verify-specs --subproject sigil` from the superproject root after structural PRD, matrix, or acceptance-title changes.
