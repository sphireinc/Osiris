# Async IR boundaries and scheduler behavior

Source: ASYNC-PLAN-CHK.txt.

Represent spawn, await, cancellation, and potential suspension in IR while
keeping no-suspension compute regions ordinary control flow. Improve load
balancing or wakeups only after benchmark evidence exposes a problem.

Acceptance: IR/SSA/CFG show async boundaries; scheduler stress tests retain
cooperative cancellation and deterministic debug mode; wake/imbalance benchmarks
justify any architecture change.

## Active increment: explicit async IR boundaries

- `spawn`, `await`, `cancel`, `parallel`, `timeout`, `race`, `select`, and
  task-aware I/O forms now lower as `async.<form>` instructions instead of
  indistinguishable ordinary calls.
- SSA treats their operands as ordinary value uses; CFG retains existing
  control-flow topology and adds deterministic per-block async-boundary
  annotations. This does not invent suspension CFG edges or change runtime
  scheduler behavior.
- `TestAsyncFormsRemainExplicitIRBoundaries` proves IR rendering, SSA
  verification, and CFG annotations. Existing scheduler cancellation and
  deterministic-mode tests remain the runtime safety evidence; no scheduler
  wake/load-balancing change is justified by a benchmark in this increment.

## Completion evidence

- The normal scheduler is already bounded by `GOMAXPROCS`, uses context
  cancellation to stop job dispatch, and preserves input-order result slots.
  `TestTaskSpawnAwaitAndCancel`, `TestTaskCancellationStopsLoops`, the
  concurrency scheduler test, and `TestTaskTraceAndDeterministicDebugMode`
  cover its cooperative-cancellation and deterministic-debug contracts.
- `bench.sh` is intentionally an interpreter/VM/native execution benchmark,
  not scheduler evidence. No measured wakeup/imbalance regression exists, so
  this task makes no speculative scheduler architecture change.
- Focused async IR/SSA/CFG coverage plus the full internal runtime suite and
  task-index/whitespace checks pass.
