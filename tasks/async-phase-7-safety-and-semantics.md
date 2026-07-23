# Async — PHASE 7: SAFETY AND SEMANTICS

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 7: SAFETY AND SEMANTICS.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Define cancellation as cooperative, not violent.
- [ ] Define what happens when a cancelled task is awaited.
- [ ] Define what happens when a timeout expires during I/O.
- [ ] Distinguish timeout errors from normal runtime errors.
- [ ] Define whether I/O calls may block a worker thread or suspend a task.
- [ ] Define which operations are safe points for cancellation.
- [ ] Ensure cancellation does not corrupt runtime state.
- [ ] Ensure task boundaries do not break deterministic programs unexpectedly.

