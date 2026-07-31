# Task lifecycle and cancellation

Tasks move through pending, running, done, failed, or canceled states. A task
has exactly one terminal result. `await` returns that result or its structured
failure; `cancel` requests cooperative cancellation through the task context.

Cancellation propagates to child work that shares the task context. It does
not forcibly stop arbitrary host code; loops, evaluator boundaries, and
task-aware I/O observe the context cooperatively. Error kind, message, source,
and metadata remain attached when a task failure crosses an `await` boundary.

The deterministic scheduler executes ready tasks in stable key order for tests
and traces. Production scheduling may run independent tasks concurrently, but
must retain output/error semantics. Stream readers/writers close explicitly;
closed streams return terminal errors rather than blocking indefinitely.

## Native C subset

Native C represents a task as an arena-owned captured thunk, one terminal
value or error, and a pending/running/done state. `spawn` does not call its
body. First `await` executes the thunk cooperatively and retains its outcome;
repeated awaits reuse that outcome. `parallel` constructs one thunk per source
branch, executes them in stable order, and returns results in source order.
`cancel` deterministically marks a pending task as cancelled, repeated
cancellation is stable, awaiting it propagates `cancelled/context canceled`,
and cancelling a completed task returns `false`. Caught task failures do not
write to standard error.

This subset is intentionally single-process and deterministic. It does not yet
provide native streams, host-thread execution, deadlines, `race`, `select`, or
interruption of a task already executing host work. Those forms remain
compile-time diagnostics. `TestCBackendDeferredTaskAndFailureParity`,
`TestCBackendStructuredResultFailureParity`, and
`TestCBackendRequiredNativeSurfaceParity` are the native evidence.
