# Scope-bound cleanup

Osiris uses `defer { ... }` as its one universal lexical cleanup construct.
There is no separate `finally`, `using`, destructor, or garbage-collector
finalizer form.

## Registration and execution

`defer` registers its block when control reaches the statement and stores the
cleanup action for the containing lexical scope:

```osiris
let input = openread("input.txt")
defer { close(input) }
let first = readline(input)
```

Actions execute exactly once when that scope exits, in reverse registration
order (LIFO). A cleanup block is a normal block, so nested blocks have their
own cleanup frame and complete before the enclosing action returns.

Cleanup is attached to the scope that registered it, not to the source file or
the lifetime of a value. The action captures the lexical environment at
registration: immutable bindings retain their lexical definition and mutable
cells retain their current value. Later assignments cannot change which value
an already-registered action observes. Captured handles still refer to the
same owned resource, so an explicit close and a deferred close are safe only
when the resource's close operation is idempotent.

The interpreter and VM run cleanup on normal fallthrough, function `return`,
loop `break`, loop `continue`, propagated `try`/`catch` or `let?` failure,
cancellation, and task termination. Native C lowers the same cleanup thunks on
normal, return, loop-break, loop-continue, and generated thunk failure edges
for the supported native ABI. Unsupported host resources remain compile-time
native boundaries rather than silently falling back to the interpreter.

Cancellation is suppressed while cleanup is running. A cleanup may therefore
flush or close an owned handle even when the enclosing task was cancelled; the
original cancellation remains the primary failure. Cleanup does not create a
new task and does not extend the owning scope after its terminal action.

## Closable resources and failures

`close(handle)` retains the existing stream, TCP-listener, and UDP-socket APIs.
It also dispatches to a zero-argument `close` method on a user-defined
resource, including a method supplied through a trait/interface implementation.
Closable implementations must be idempotent: the first successful release
returns their success value, and subsequent calls must not release the
underlying resource again. `defer { close(handle) }` is the compatibility-safe
way to make ownership scope-bound while existing explicit `close` calls remain
valid.

If the body and cleanup both fail, the body error remains primary and cleanup
errors are attached as structured causes in LIFO execution order. If several
cleanups fail, all are retained in that order. Cleanup failures never panic,
silently replace an earlier error, or bypass source and call-stack metadata.
A `return`, `break`, or `continue` emitted by cleanup code is converted to a
cleanup failure rather than escaping into the surrounding control-flow
construct.

## Compiler and optimizer contract

AST, bytecode, and IR each represent cleanup explicitly. IR cleanup operands
are verified as single-use thunk references, and cleanup instructions are
effect barriers: source and native optimizers may not delete, duplicate,
reorder, inline across, or bypass them. Every generated normal and early exit
edge consumes the appropriate lexical cleanup suffix once.

The focused evidence is in
`internal/osiris/scope_cleanup_test.go`: LIFO order, cancellation, captured
mutable values, nested control-flow exits, dual failures, user-defined
closable resources, IR/SSA ownership, optimizer preservation, and native
executable cleanup edges are covered. The filesystem and networking tests
cover idempotent host-handle closure.

This mechanism is deterministic scope cleanup, not a promise that arbitrary
values are automatically closed. A resource must be returned by a closable
API or implement the documented `close` protocol, and programs should register
ownership immediately after successful acquisition.
