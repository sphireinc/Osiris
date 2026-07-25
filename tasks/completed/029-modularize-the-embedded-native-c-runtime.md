# Task 029: Modularize the embedded native C runtime

**Priority:** P1  
**Dependencies:** 028  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Reduce risk in `cbackend_native.go` by separating stable runtime components without changing generated behavior.

## Required work

- [x] Define deterministic component boundaries for values, memory, numeric, tensor, complex, linalg, errors, and calls.
- [x] Choose Go string fragments with stable ordering.
- [x] Preserve cache identity and source-map behavior.
- [x] Add generated-C golden tests.

## Acceptance criteria

- [x] Main lowering file becomes materially smaller and reviewable.
- [x] Generated C remains byte-stable or changes through reviewed goldens.
- [x] No runtime symbols collide.

## Validation

- [x] full native parity
- [x] generated source determinism

## Non-goals

- Do not add a second runtime implementation.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Extracted the 325-line complex tensor/complex linalg wrapper runtime into
  `internal/osiris/cbackend_runtime_complex.go`; the lowering file is now 320
  lines smaller and emits the base runtime before the wrapper component.
- Added `docs/design/NATIVE-RUNTIME-COMPONENTS.md`, defining the stable value,
  memory, numeric, tensor, complex, linalg, error, and call boundaries and
  their required emitted order.
- Added `TestCBackendRuntimeComponentGolden`. It verifies repeated generated
  C equality, base-before-complex helper order, and the reviewed SHA-256
  `53f213f6070bf8445fd8945e896d41af39dfeab9816f0ec40e2f39a5ce23d819` for a
  representative complex-tensor program.
- Verified with:

  ```sh
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./internal/osiris -run 'TestCBackend(ExecutableCache|ObjectAndExecutable|RuntimeComponentGolden|DeterministicSource|ComplexScalarParity|EigProviderComplexResidual)$' -count=1
  git diff --check
  ```
