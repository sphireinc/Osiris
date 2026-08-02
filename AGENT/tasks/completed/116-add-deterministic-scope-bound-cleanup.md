# Task 116: Add deterministic scope-bound cleanup

**Priority:** P1
**Dependencies:** 013, 045, 102, 109
**Status:** Complete

## Current implementation evidence

`defer { ... }` is parsed, formatted, type-checked, macro-expanded, and
implemented with deterministic LIFO cleanup frames in the interpreter and VM.
Native IR lowers explicit cleanup thunks on normal, return, loop-break, and
loop-continue edges, with generated-C error propagation on cleanup failure.
Registration snapshots lexical mutable cells, cancellation is suppressed while
cleanup runs, and body/cleanup failures preserve both causes.

The complete control-flow, task, closable-resource, optimizer/IR ownership,
native executable, race, and sanitizer evidence is recorded below.

## Objective

Add one universal cleanup mechanism that runs reliably on normal return,
early return, propagated failure, cancellation, and task termination.

## Required language contract

- Choose a primary construct (`defer`, `finally`, `using`, or another explicit
  Osiris form) and document evaluation/registration/execution order.
- Define lexical scope, LIFO ordering, captured values, nested cleanup,
  cleanup-generated errors, panic-free behavior, and cancellation semantics.
- Define interaction with `try`/`catch`, `let?`, loops, `break`, `continue`,
  `return`, task cancellation, streams, files, sockets, database transactions,
  locks, and FFI handles.
- Specify what happens when both body and cleanup fail; preserve structured
  causes and metadata deterministically.

## Implementation requirements

- Add parser/formatter and explicit IR/bytecode/native cleanup edges.
- Optimizers must not delete, duplicate, reorder, or bypass cleanup.
- VM and native code must release owned resources exactly once.
- Add standard closable/resource protocol hooks without requiring interfaces
  before task 113 is complete.
- Preserve previous manual-close APIs for compatibility.

## Tests and documentation

- Control-flow matrix covering every exit kind, nesting, multiple cleanups,
  cleanup failure, cancellation, and repeated close.
- Real file/socket/stream fixtures plus deterministic fake resources.
- IR/CFG assertions, native ownership tests, ASan/UBSan, and race tests.
- Checked `newdocs/` examples and migration guidance.

## Acceptance criteria

- [x] Cleanup order and dual-failure behavior are completely documented.
- [x] Every synchronous and task exit path executes registered cleanup once.
- [x] Optimizer and backend tests prove cleanup cannot be bypassed.
- [x] Resource, race, sanitizer, documentation, and full validation pass.

## Completion evidence

- `internal/osiris/scope_cleanup_test.go` covers LIFO ordering, cancellation,
  registration-time mutable captures, return/break/continue/try exits,
  interpreter/VM dual failures, child-task termination, the user-defined
  closable protocol, IR/SSA and optimizer thunk ownership, and native
  executable normal/return cleanup edges.
- `internal/osiris/session.go` snapshots lexical cleanup environments and
  converts cleanup control-flow signals into structured cleanup failures.
- `internal/osiris/bytecode.go` snapshots VM frames at registration, runs each
  cleanup with its captured frame exactly once, and prevents receiver methods
  from shadowing builtin names such as `close`.
- `internal/osiris/ssa.go` verifies cleanup thunk operands and arity;
  `internal/osiris/optimize.go` retains cleanup-only references and treats
  cleanup blocks as effect barriers.
- `internal/osiris/runtime.go` preserves idempotent built-in closure and
  dispatches `close` to zero-argument user-defined or trait/interface methods.
- `AGENT/docs/design/SCOPE-CLEANUP.md`, `AGENT/LANG.txt`, and the public
  `docs/src/` language reference and support matrix define registration,
  capture, ordering, cancellation, dual failures, closable ownership, native
  boundaries, and migration from explicit close calls.
- Validation included the full Go package suite, race-focused cleanup tests,
  native executable cleanup tests, Homebrew LLVM ASan/UBSan runs, and Linux
  ARM64 ASan in the Colima container with its matching Clang runtime. The
  public documentation build passes with the supported Node runtime. The full
  reference checker still reports pre-existing task-118 `std/net` export
  inventory omissions, and the examples checker cannot run with this host's
  Go 1.25.5 because `go.mod` requires Go 1.26; neither issue is caused by the
  task-116 changes.

## Non-goals

- No nondeterministic finalizers or garbage-collector-dependent cleanup.
