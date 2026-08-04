# Task 140: Complete interpreter semantic parity

**Priority:** P1
**Dependencies:** 139
**Status:** Complete

## Objective

Ensure the interpreter is the executable semantic reference for every parsed
language construct and every standard-library contract before other backends
claim parity.

## Exact scope and ownership

- Own interpreter/session paths in `internal/osiris/runtime.go`,
  `internal/osiris/session.go`, `internal/osiris/evaluator.go` where present,
  and interpreter-only tests.
- Exercise constants, immutable bindings, mutable cells, typed/generic calls,
  traits/interfaces/impl dispatch, iterators, errors/results/options,
  patterns, macros, modules, tasks, cancellation, cleanup, and all stdlib
  modules.
- Reconcile emitted output, error propagation, cancellation, ownership, and
  source locations with the language specification.
- Add a table-driven interpreter conformance suite consumed by VM/native/WASM/
  GPU differential tasks.
- Do not modify VM, native C, WASM, GPU, formatter, or LSP implementations.

## Acceptance criteria

- Every parser conformance fixture has an interpreter result or an explicit
  stable language diagnostic.
- Interpreter behavior is deterministic under repeated execution and injected
  providers.
- No interpreter path silently accepts a construct that static checking or
  the parser rejects.
- The conformance suite exposes exact output/error witnesses for later tasks.

## Parallelism

This task owns interpreter execution files. After task 139 completes, it may
run concurrently with tasks 138 and 142; task 141 and the artifact backends
must wait for its semantic-reference fixtures.

## Implementation evidence

- `internal/osiris/session.go:ExecSource`, `ExecProgram`, and `checkProgram`
  retain successful declaration/type-check history across source fragments,
  commit parser context only after successful execution, and preserve committed
  output and cleanup behavior on later errors.
- `internal/osiris/session.go:evalBlockExpr` and `internal/osiris/runtime.go:evalBlock`
  use explicit mutable binding cells and implement typed bindings and tuple
  destructuring consistently in session and interpreter execution.
- `internal/osiris/macro.go:expandStmtSeq` and `expandTemplateBlock` preserve
  destructured names and hygienic renaming during macro expansion; runtime
  fingerprinting mirrors the same lexical bindings.
- `internal/osiris/interpreter_parity_test.go` provides deterministic,
  table-driven coverage for bindings, generic calls, traits/interfaces/impls,
  iterators, Result/Option patterns, macros, modules, tasks, cancellation,
  cleanup, committed output, errors, source spans, and later declarations.

Validation: the focused interpreter/session suites, complete `internal/osiris`
suite, race-enabled interpreter/VM coverage, `go test ./...`, and `go vet ./...`
pass.
