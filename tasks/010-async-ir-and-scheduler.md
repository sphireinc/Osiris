# Async IR boundaries and scheduler behavior

Source: ASYNC-PLAN-CHK.txt.

Represent spawn, await, cancellation, and potential suspension in IR while
keeping no-suspension compute regions ordinary control flow. Improve load
balancing or wakeups only after benchmark evidence exposes a problem.

Acceptance: IR/SSA/CFG show async boundaries; scheduler stress tests retain
cooperative cancellation and deterministic debug mode; wake/imbalance benchmarks
justify any architecture change.
