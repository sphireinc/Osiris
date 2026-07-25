# Task 033: Make static module propagation sound through CFG and closures

**Priority:** P1  
**Dependencies:** 025,032  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Complete conservative static-module identity propagation needed for native lowering.

## Required work

- [x] Specify lattice/merge rules for module identity.
- [x] Handle block parameters, branches, loops, lexical captures, and nested functions.
- [x] Prove aliases only when all incoming paths agree.
- [x] Emit deliberate diagnostics for escaping/dynamic module values.

## Acceptance criteria

- [x] No unsound static member lowering occurs.
- [x] Positive nested capture cases compile natively.
- [x] Conflicting-path cases remain dynamic or fail clearly.

## Validation

- [x] analysis unit tests
- [x] native module parity cases

## Non-goals

- Do not introduce runtime dynamic imports into native code.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added `cbackend_modules.go`, a conservative static-module identity analysis:
  import paths introduce facts, copies preserve facts, CFG parameters require
  every incoming path to agree, and lexical captures use the same all-sites
  agreement rule. Loop-carried facts converge without treating an unresolved
  back-edge as a dynamic value.
- Added terminal conflict handling for both CFG and closure capture merges, so
  a conflict becomes permanently unknown rather than oscillating during the
  fixed-point solve.
- Native member lowering now rejects unknown module values with `dynamic
  module member "name" cannot be lowered`; known non-`std/math` modules get a
  separate explicit diagnostic. No dynamic module runtime was introduced.
- Added direct analysis coverage for branches, loops, agreeing/conflicting
  captures, plus native parity for a captured `std/math` call and a negative
  conflicting-module merge case. Documented the lattice in
  `docs/design/NATIVE-COMPILER.md` and updated the support matrix.
- Verified with:

  ```sh
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./internal/osiris -run 'TestNativeModuleIdentities|TestCBackend(StaticMathCapturedModuleParity|RejectsConflictingModuleMerge|StaticMathImportParity)$' -count=1
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...
  scripts/check-task-index.sh
  git diff --check
  ```
