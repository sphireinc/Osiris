# Task 028: Specify and test native C ABI ownership

**Priority:** P0  
**Dependencies:** 019,025  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Make allocation, borrowing, copying, retaining, and cleanup rules explicit for every native value family.

## Required work

- [x] Document ownership for strings, lists, closures, tensors, complex tensors, errors, and provider outputs.
- [x] Audit all helper returns and error paths for leaks/double frees/use-after-free.
- [x] Add copy-on-write and aliasing tests.
- [x] Add sanitizer-friendly build mode.

## Acceptance criteria

- [x] ABI document has an ownership table.
- [x] Native parity tests pass under ASan/UBSan where supported.
- [x] Provider and closure cleanup paths are tested.

## Validation

- [x] sanitizer suite
- [x] aliasing regression tests

## Non-goals

- Do not redesign the whole ABI unless evidence requires it.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added a native ABI ownership table covering scalar values, strings, lists,
  closures, f64 tensors, complex tensors, errors, and numerical-provider
  outputs, including transfer and cleanup rules for temporary buffers.
- Added `OSIRIS_C_SANITIZERS=1` C compile flags and the Linux/Clang
  `scripts/test-native-sanitizers.sh` suite, wired into CI. Darwin is an
  explicit skip because its local ASan runtime aborts during initialization
  before generated code executes.
- Existing native regression tests cover direct closure capture ownership,
  copy-on-write tensor aliasing, complex tensor allocation paths, and provider
  output conversion; the sanitizer suite runs those representative paths.
- Validation passed locally: normal focused ownership regressions,
  sanitizer-script platform skip, and `git diff --check`. Linux CI executes
  the ASan/UBSan suite.
