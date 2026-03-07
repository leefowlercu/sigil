# Changes

Work inside `sigil/` as an implementation submodule of the `project-sigil` superproject. Keep local code changes aligned with the superproject specs and avoid unrelated repo-state churn.

## Boundary Rules

- Treat `sigil/` as the implementation layer for `docs/sigil/`.
- Keep spec updates in the superproject docs when behavior changes require them.
- Do not modify `sigil-web/` or unrelated superproject docs unless the task requires coordinated cross-repo work.

## Authorization Rules

- Do not run `git commit` unless the user explicitly asks for a commit in the current conversation.
- Do not run `git push` unless the user explicitly asks for a push in the current conversation.
- Do not update the `sigil/` submodule pointer from the superproject unless the task explicitly requires it.

## Hygiene Rules

- Keep generated files and transient runtime artifacts out of commits.
- Prefer targeted changes over broad refactors when the behavior contract is already narrow.
- State any intentional spec or implementation gaps explicitly in the final handoff.
