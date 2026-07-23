# Polling — PHASE 12: EVENT LOOP BOUNDARY

Source: roadmaps/POLLING-CHK.txt, section PHASE 12: EVENT LOOP BOUNDARY.

## Native compiler integration

P1 native requirement: when implemented, this runtime integration must be callable by generated C through the native runtime ABI.

## Remaining checklist

- [ ] Define how timers should integrate with polling.
- [ ] Define how cancellation should interrupt or cooperate with polling.
- [ ] Define how task schedulers can suspend on poll waits.
- [ ] Define how callbacks can be layered over watchers.
- [ ] Define whether the standard library will include a callback event loop helper.
- [ ] Define whether the standard library will include futures/promises/tasks over polling.
- [ ] Document the difference between readiness polling and async execution.
- [ ] Document how framework authors should use the polling API as a backend.

