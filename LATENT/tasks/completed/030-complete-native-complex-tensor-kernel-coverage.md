# Task 030: Complete native complex tensor kernel coverage

**Priority:** P1  
**Dependencies:** 028,025  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Finish the in-progress complex tensor path with clearly bounded semantics and parity.

## Required work

- [x] Inventory f64 tensor operations and classify complex applicability.
- [x] Implement prioritized reductions and matrix kernels with mixed f64/complex behavior.
- [x] Define division-by-zero, NaN/Inf, equality, printing, and shape errors.
- [x] Add residual-based numerical assertions where exact equality is inappropriate.

## Acceptance criteria

- [x] Support matrix is updated per kernel.
- [x] Interpreter/native parity holds within documented tolerances.
- [x] Ownership and mixed-type promotion are tested.

## Validation

- [x] complex tensor parity suite
- [x] sanitizer run
- [x] residual tests

## Non-goals

- Do not claim production eig accuracy without a production provider.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Audited complex applicability against the f64 tensor inventory and recorded
  the exact native kernel, promotion, error, ownership, and provider rules in
  `docs/design/NATIVE-COMPLEX-ABI.md`.
- The native complex ABI covers rectangular construction; indexing/mutation and
  slices; mixed broadcast arithmetic; reductions; outer products; transpose,
  matrix multiply, trace, determinant, solve/inverse, QR, and Hermitian
  Cholesky. `eig` remains deliberately provider-backed and real-input only.
- Added `TestCBackendComplexTensorResiduals`, which validates native solve,
  Hermitian Cholesky reconstruction, and QR reconstruction residuals at
  `<= 1e-10`; provider-backed eig already uses residual assertions.
- Updated the support matrix to cite complex parity, residual, and ownership
  evidence. The sanitizer script was run; on Darwin it deliberately reports a
  documented ASan harness skip.
- Verified with:

  ```sh
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./internal/osiris -run 'TestCBackend(ComplexScalarParity|ComplexCholeskyParity|ComplexTensorResiduals|EigProviderComplexResidual)$|TestComplexTensorReductions$|TestSupportMatrix' -count=1
  ./scripts/test-native-sanitizers.sh
  git diff --check
  jq empty docs/support-matrix.json
  ```
