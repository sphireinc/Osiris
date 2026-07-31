# Task 044: Profile and decompose compiler/runtime hotspots

**Priority:** P2  
**Dependencies:** 025,043  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Use evidence to reduce large-file and hotspot risk without semantic churn.

## Required work

- [x] Capture CPU/allocation profiles for representative workloads.
- [x] Identify package/file boundaries that reduce coupling.
- [x] Extract components incrementally with no behavior change.
- [x] Track compile time and allocation effects.

## Acceptance criteria

- [x] Each extraction has unchanged conformance results.
- [x] Profiles and rationale are committed or linked.
- [x] No circular package architecture is introduced.

## Validation

- [x] full conformance
- [x] profile comparison

## Non-goals

- Do not split files solely to meet arbitrary line counts.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`BenchmarkRuntimeNativeLoop` and `profile-workload.sh` capture reproducible
CPU/allocation profiles. The recorded darwin/arm64 baseline showed about
1.76 GB and 12.5M allocations per loop run, dominated by runtime allocation/GC
work rather than a cosmetic source-file hotspot. `PROFILING.md` records the
environment, result, and extraction criteria.

The lexical `binding`/`scope` component was extracted from `runtime.go` into
`scope.go`, making the shared name-resolution boundary explicit for evaluator,
session, macro, match, and bytecode paths without a package cycle. Conformance
passed unchanged; the profile remained essentially unchanged, so this change
is correctly documented as coupling reduction rather than a performance claim.

Focused conformance/profile comparison, full Go tests, task-index checks, and
diff checks passed.
