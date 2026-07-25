# Task 022: Create a canonical feature and backend support matrix

**Priority:** P0  
**Dependencies:** 019  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Make feature claims precise across parser, interpreter, VM, native C, WASM, GPU, formatter, and LSP.

## Required work

- [x] Derive current support from tests and implementation, not README claims.
- [x] Create a machine-readable matrix plus a rendered documentation page.
- [x] Mark unsupported, partial, experimental, and stable states.
- [x] Link each row to representative tests or task IDs.

## Acceptance criteria

- [x] The matrix covers every documented language feature and standard module.
- [x] README/help/docs no longer overstate backend coverage.
- [x] A CI check detects malformed or orphaned matrix entries.

## Validation

- [x] matrix schema validation
- [x] documentation generation test

## Non-goals

- Do not implement missing features as part of the inventory.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added `docs/support-matrix.json`, the canonical machine-readable contract for
  every current reference chapter and standard module. Each row names parser,
  interpreter, VM, native C, WASM, GPU, formatter, and LSP state, its evidence,
  and the limitation behind every non-stable state.
- Added `docs/support.html` and `docs/support.js`, which render that JSON at
  the documentation site rather than maintaining a second handwritten table.
- Added `internal/osiris/support_matrix_test.go` and the CI-invoked
  `scripts/check-support-matrix.sh`. They validate the schema, all state
  values, complete reference-page coverage, existing documentation links, and
  every cited test/task path or test symbol.
- Corrected backend wording in `README.txt`, the static documentation, and
  CLI help. Native C now explicitly describes a tested lowering subset;
  WebAssembly and GPU remain experimental artifact paths.
- Validation passed: `./scripts/check-support-matrix.sh` (with the repository
  Go cache override), `go test ./...`, and `git diff --check`.
