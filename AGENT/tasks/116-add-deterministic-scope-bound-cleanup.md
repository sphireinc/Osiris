# Task 116: Add deterministic scope-bound cleanup

**Priority:** P1
**Dependencies:** 013, 045, 102, 109
**Status:** Ready

## Current implementation evidence

`defer { ... }` is parsed, formatted, type-checked, macro-expanded, and
implemented with LIFO cleanup frames in the interpreter and VM. Native IR now
lowers cleanup thunks on normal, return, loop-break, and loop-continue edges.
Cancellation is suppressed while cleanup runs, and body/cleanup failures
preserve both causes. Focused tests cover LIFO order, cancellation, dual
failures, and the native cleanup edge.

This task is not complete yet: the full return/break/continue/task/resource
matrix, resource protocol integration, optimizer/IR ownership assertions, and
sanitizer/race evidence are still required.

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

- [ ] Cleanup order and dual-failure behavior are completely documented.
- [ ] Every synchronous and task exit path executes registered cleanup once.
- [ ] Optimizer and backend tests prove cleanup cannot be bypassed.
- [ ] Resource, race, sanitizer, documentation, and full validation pass.

## Non-goals

- No nondeterministic finalizers or garbage-collector-dependent cleanup.
