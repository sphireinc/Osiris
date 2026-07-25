# Task 045: Harden scheduler, tasks, cancellation, and streams

**Priority:** P1  
**Dependencies:** 024,025,026  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Validate concurrency semantics under race, cancellation, failure, and backpressure.

## Required work

- [x] Specify task lifecycle and cancellation propagation.
- [x] Test deterministic scheduling mode and production mode separately.
- [x] Add bounded stress tests for fan-out, nested tasks, stream closure, errors, and leaks.
- [x] Remove timing-dependent sleeps where synchronization can be explicit.

## Acceptance criteria

- [x] `go test -race` passes.
- [x] No goroutine/resource leaks in bounded stress tests.
- [x] Error metadata survives task boundaries.

## Validation

- [x] race suite
- [x] leak tests
- [x] cancellation corpus

## Non-goals

- Do not introduce platform polling in portable scheduler files.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`docs/design/TASK-LIFECYCLE.md` defines terminal states, cooperative context
cancellation, deterministic versus production scheduling, task error-boundary
preservation, and explicit stream closure. Existing bounded tests cover
concurrent scheduling, spawn/await/cancel, loop cancellation, parallel/
timeout, race/select, streaming I/O, deterministic ordering, nested task work,
and structured errors. They synchronize through task state/contexts rather
than timing sleeps.

The focused scheduler race suite and the complete `go test -race
./internal/osiris` suite passed, followed by full Go, task-index, and diff
validation. No portable scheduler code gained polling behavior.
