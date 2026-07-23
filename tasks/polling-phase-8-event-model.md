# Polling — PHASE 8: EVENT MODEL

Source: roadmaps/POLLING-CHK.txt, section PHASE 8: EVENT MODEL.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Decide whether half-close/read-hang-up events are exposed.
- [ ] Decide whether error and hang-up events are always monitored automatically.
- [ ] Decide which events may be requested as input events.
- [ ] Decide which events may only appear as output events.
- [ ] Add helper methods or predicates for checking whether an event was triggered.
- [ ] Define how multiple triggered events are represented.
- [ ] Define how unsupported platform-specific events are handled.
- [ ] Document differences between read readiness, EOF, hang-up, and error.
- [ ] Document whether readiness means an operation will definitely complete without blocking.
- [ ] Document spurious wakeups or stale readiness notifications if they can occur.

