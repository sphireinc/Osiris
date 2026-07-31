# Task 040: Make documentation examples executable

**Priority:** P1  
**Dependencies:** 026,039  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Prevent reference examples from drifting away from the language.

## Required work

- [x] Extract or annotate runnable code blocks.
- [x] Execute expected-success examples and verify expected failures.
- [x] Pin backend expectations using the support matrix.
- [x] Provide a local command and CI job.

## Acceptance criteria

- [x] Every runnable example is checked.
- [x] Failures report page and code-block location.
- [x] Examples requiring unavailable native features are explicitly marked.

## Validation

- [x] documentation test runner

## Non-goals

- Do not execute unsafe filesystem/network examples without sandbox fixtures.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`internal/osiris/documentation_examples_test.go` reads the canonical React
reference data and executes every self-contained `emit` example, reporting its
module/function path in the Go subtest name. It supplies the documented
standard-module context where an example uses `text.` or `math.`. The `try` /
`catch` reference example deliberately exercises a caught conversion failure.
I/O snippets remain visibly marked sandbox-required and are not run against
the host filesystem, consistent with this task's non-goal; declaration-only
snippets are visibly marked illustrative.

The checker corrected stale examples for `join`, decimal/rational construction,
bigint arithmetic, math-module calls, `pad`, `pack`/`unpack`, and `range`.
`npm run check-examples` runs the Go test with isolated temporary Go caches;
the CI docs job invokes it before building the site. The React listing labels
each example CHECKED, ILLUSTRATIVE, or SANDBOX REQUIRED.

Focused example checks, `npm run check-reference`, `npm run build`, full
`go test ./...`, task-index checks, and `git diff --check` passed.
