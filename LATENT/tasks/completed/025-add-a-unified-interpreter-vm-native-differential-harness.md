# Task 025: Add a unified interpreter-VM-native differential harness

**Priority:** P0  
**Dependencies:** 019,020  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Turn backend parity into a reusable test framework rather than scattered test cases.

## Required work

- [x] Define canonical comparison of values, stdout/stderr, structured errors, exit state, and source spans.
- [x] Allow capability-aware skips for unsupported native features.
- [x] Support table tests and corpus directories.
- [x] Emit minimized diagnostic context on mismatch.

## Acceptance criteria

- [x] Existing parity tests can migrate without losing coverage.
- [x] A single fixture can execute all supported backends.
- [x] Mismatch output identifies backend, phase, and source.

## Validation

- [x] unit tests for successful/error cases
- [x] intentional mismatch test

## Non-goals

- Do not treat unsupported native behavior as parity success unless the support matrix marks it unsupported.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added `internal/osiris/differential_test.go`, a reusable table-driven
  interpreter/VM/native-C fixture harness with canonical rendered output and
  error comparison, compile/execute phase reporting, and fixture source in
  every mismatch diagnostic.
- Added capability controls: unsupported native features must reject during C
  compilation; fixtures that intentionally exercise interpreter/VM-only error
  semantics declare that native path explicitly rather than counting it as a
  parity success.
- Added the first file-backed corpus fixture under
  `internal/osiris/testdata/differential/`, establishing the directory shape
  consumed by the following corpus task.
- Added success, structured-error, unsupported-native, and deliberate
  mismatch-diagnostic tests. Existing dedicated parity tests remain in place;
  they can move to this shared harness incrementally without coverage loss.
- Validation passed: focused `TestDifferential` and full `go test ./...`.
