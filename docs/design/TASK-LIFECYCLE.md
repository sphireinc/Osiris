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
