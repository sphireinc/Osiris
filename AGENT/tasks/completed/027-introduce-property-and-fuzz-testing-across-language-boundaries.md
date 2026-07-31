# Task 027: Introduce property and fuzz testing across language boundaries

**Priority:** P1  
**Dependencies:** 025,026  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Find parser, formatter, codec, numeric, and backend inconsistencies automatically.

## Required work

- [x] Add parse-format-parse properties.
- [x] Round-trip values through supported codecs.
- [x] Generate bounded AST/program subsets for interpreter/VM parity.
- [x] Fuzz structured error serialization and source maps.
- [x] Persist minimized regressions.

## Acceptance criteria

- [x] Fuzz targets run with short CI budgets and longer local commands.
- [x] No unbounded allocation or hanging seed is accepted.
- [x] Each discovered bug becomes a deterministic regression test.

## Validation

- [x] `go test -fuzz` smoke runs
- [x] seed corpus tests

## Non-goals

- Do not fuzz native compilation without strict process/time/resource bounds.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added bounded interpreter/VM expression parity and source-map/normalized-error
  fuzz targets alongside existing formatter/evaluator/codec coverage. All
  targets cap source size; expression fuzzing is statement-loop-free.
- Added short CI fuzz budgets and retained the normal `go test -fuzz` commands
  for longer local exploration.
- The VM fuzz target found a minimized `0&&B` regression: interpreter
  fingerprinting eagerly resolved a short-circuited operand. Logical binaries
  now bypass that memoization path; a deterministic short-circuit regression
  test and the fuzzer seed are retained.
- Validation passed: formatter, interpreter/VM, and source-map/error fuzz
  smokes; focused regression tests; and full `go test ./...`.
