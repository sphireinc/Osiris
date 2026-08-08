# Task 154: Stabilize the Native C and LSP support-matrix columns

**Priority:** P0
**Dependencies:** 160,161,162,163,168
**Status:** Blocked until the Native C and LSP closure tasks complete

## Objective

Bring the Native C and LSP columns of the Backend Support Matrix from
`partial` to `stable`. This is the active parity gate for the current release
priority. WASM, GPU, alternate-platform, and all-backend release work is
tracked separately by tasks 300–308, 310, and 400 and is not required to begin
this gate.

## Exact scope

- Reconcile every Native C row against generated-C execution, interpreter/VM
  differential fixtures, ownership and cancellation checks, and granular
  documentation.
- Reconcile every LSP row against protocol fixtures, UTF-16/source-span
  checks, cancellation/race evidence, workspace/refactoring behavior, and
  documented response shapes.
- Add a Native C/LSP-only matrix audit so these columns cannot be promoted by
  prose or by evidence belonging to another backend.
- Keep WASM and GPU statuses unchanged. Task 310 owns the eventual all-backend
  release decision after tasks 300–308 and 400 are complete.

## Acceptance criteria

- Every Native C and LSP cell in `docs/src/support-matrix.json` is exactly
  `stable` and has executable evidence and a granular documentation route.
- Native C evidence includes normal, race/sanitizer-equivalent, ownership,
  cancellation, generated-executable, and documented unsupported-path checks.
- LSP evidence includes every advertised method, malformed and cancelled
  requests, UTF-16 positions, stale document versions, multi-file workspace
  edits, and deterministic protocol fixtures.
- No Native C or LSP row invokes an interpreter, VM, planner, or reference
  provider as a hidden fallback.
- Tasks 160, 161, 162, 163, and 168 have complete records under
  `AGENT/tasks/completed/` before this task is marked complete.

## Current gate evidence

The focused Native C/LSP parity suites pass, including the task-160 semantic
and task-161 workspace fixtures, task-162 tuple/data differential coverage,
and task-163 text/stdlib boundary coverage. The support-matrix checker also
passes its schema and documentation checks.

The release gate remains blocked: the matrix currently contains 28 `partial`
Native C cells and 28 `partial` LSP cells. The bounded slices above do not
prove all rows, sanitizer/cross-platform execution, or the full ownership and
cancellation contracts required by this task, so no matrix promotion or
completion record is justified yet.

## Explicitly deferred

WASM lowering/import execution, GPU hardware/device execution, alternate-OS
evidence, and the final all-backend matrix transition remain in tasks 300–310.
This task must not weaken their requirements or change their matrix cells.
