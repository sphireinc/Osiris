# Task 026: Build a language corpus and golden conformance suite

**Priority:** P0  
**Dependencies:** 022,025  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Create stable positive and negative programs that define the language contract.

## Required work

- [x] Organize syntax, semantics, errors, modules, numeric families, tensors, tasks, and native subsets.
- [x] Store expected formatted source, output, value/error representation, and diagnostics.
- [x] Add deterministic golden-update tooling with review safeguards.
- [x] Include regression programs for historical bugs.

## Acceptance criteria

- [x] Corpus runs in CI.
- [x] Golden updates are explicit and deterministic.
- [x] Negative cases assert source ranges and stable error codes.

## Validation

- [x] conformance runner tests
- [x] golden no-diff check

## Non-goals

- Do not snapshot unstable pointer addresses, timings, or host paths.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added a discoverable JSON-golden corpus under
  `internal/osiris/testdata/conformance/`, covering syntax, semantics, parser
  diagnostics, modules, numeric rendering, tensors, tasks, and the native
  subset.
- Added `TestConformanceCorpus`, which checks interpreter/VM output, golden
  output and diagnostics, explicit native rejection, and native executable
  parity for native-marked cases.
- Added guarded deterministic updating via
  `scripts/update-conformance-goldens.sh`; it refuses unless
  `OSIRIS_UPDATE_GOLDENS=1` is set and asks the author to review the diff.
- The initial run captured two real stable contracts: default integer output is
  `u64`-styled and parse diagnostics use the lexer term `ident`.
- Validation passed: golden guard refusal smoke test, focused corpus suite,
  support-matrix check, full `go test ./...`, task-index check, and
  `git diff --check`.
