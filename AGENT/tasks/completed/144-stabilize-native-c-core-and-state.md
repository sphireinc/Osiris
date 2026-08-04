# Task 144: Stabilize native C core language and state parity

**Priority:** P1
**Dependencies:** 139,140,141,116,117
**Status:** Complete

## Objective

Close the native C gap for core syntax/control flow, typed functions, and
deterministic `const`/`let`/`var` state.

## Exact scope and ownership

- Own the core lowering portions of `internal/osiris/cbackend_native.go`,
  generated-C runtime helpers, and native core parity tests.
- Implement or parity-test scalar/aggregate function calls, closures, typed
  signatures, generic instantiations that have a C representation, loops,
  branches, returns, `break`/`continue`, ordered emits, and mutation barriers.
- Preserve exact integer widths, bool/string ownership, cleanup edges, and
  source-spanned diagnostics.
- Add generated-C executable, sanitizer, differential, and negative tests.
- Do not edit WASM, GPU, formatter, LSP, or support-matrix files.

## Matrix transitions

Promote native C for core syntax, typed functions, and deterministic state from
`partial` to `stable` only after all represented forms have parity evidence.

## Parallelism

Native tasks 144–148 are intentionally sequential because they own the shared
native lowering dispatch and C runtime. They must not run concurrently with one
another.

## Implementation evidence (2026-08-04)

- Added explicit mutable-capture metadata to `IRFunction` and propagated it
  through `irBuilder.recordCapture`, `lowerFuncStmt`, and `lowerDeferredExpr`.
- The native core now uses stable generated-C `OCell` carriers for `var`
  bindings captured by closures. `emitCFunctionBody` unwraps captured values,
  while `emitCInstr` preserves the cell through `copy`, `assign`, `makefunc`,
  and direct typed calls. The cell is tracked by the existing process-lifetime
  ownership list and reports invalid updates through the native error channel.
- Added focused differential and negative tests in
  `internal/osiris/cbackend_core_test.go` for aggregate closure state, later
  mutation, mutation inside a closure, loop/branch mutation, task capture
  behavior, aggregate and generic calls, immutable assignment, and typed-call
  diagnostics.
- Focused generated-C executable checks passed before the unrelated dirty
  `internal/osiris/infer.go` change invalidated package rebuilds:
  `TestCBackendMutableClosureCellParity`,
  `TestCBackendMutableClosureObservesLaterMutation`,
  `TestCBackendMutableStateAcrossControlFlowParity`,
  `TestCBackendTaskMutableCaptureSnapshotParity`,
  `TestCBackendAggregateAndGenericCallParity`, and
  `TestCBackendTypedCallDiagnosticIsSourceSpanned`. The deferred-task and
  structured-result native checks also passed with
  `OSIRIS_C_SANITIZERS=undefined`.
- Follow-up validation after the initial handoff removed the stale build
  concern. The focused mutable-closure, task-snapshot, block-state,
  break/continue, typed-call, aggregate/generic-call, and immutable-assignment
  suite passes with `GOCACHE=/tmp/osiris-task144-cache`.
- The generated-C runtime golden hash was refreshed for the owned-cell ABI;
  `git diff --check` and the package native-core suite pass.
