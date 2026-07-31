# Task 032: Harden native numeric semantics and overflow parity

**Priority:** P0  
**Dependencies:** 025,026  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Ensure native scalar lowering exactly matches interpreter rules for widths, promotion, overflow, division, powers, decimals, and bigint transitions.

## Required work

- [x] Create a semantic table for every numeric family/operator.
- [x] Add boundary-value differential tests.
- [x] Implement only operations whose host lowering can preserve semantics; reject the rest clearly.
- [x] Cover signed minimums, unsigned overflow, shifts, NaN, infinities, negative zero, and exponentiation.

## Acceptance criteria

- [x] No silent C overflow is observable where Osiris requires promotion/error.
- [x] Result kind and width match every backend.
- [x] Unsupported cases fail at compile time with stable diagnostics.

## Validation

- [x] boundary corpus
- [x] differential harness
- [x] generated C inspection

## Non-goals

- Do not map integer `pow` blindly to libm.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added a numeric lowering table to `docs/design/NATIVE-ABI.md` defining the
  supported width/overflow/division/shift/float/complex contract and explicit
  rejection of bigint, decimal, wide, packed, and integer-power lowering.
- Added differential boundary coverage for `u8` wraparound, signed-minimum
  representation, f32 arithmetic, negative zero, and explicit bigint native
  rejection. Existing native regressions cover overflow diagnostics,
  invalid shifts, zero division/remainder, NaN/infinity domains, and float
  widths.
- Verified generated native C preserves those fixture outputs and rejects the
  bigint fixture at compile time rather than coercing it to host arithmetic.
- Validation passed: focused differential harness, full `go test ./...`, and
  `git diff --check`.
