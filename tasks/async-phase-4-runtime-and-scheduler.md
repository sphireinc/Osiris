# Async — PHASE 4: RUNTIME AND SCHEDULER

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 4: RUNTIME AND SCHEDULER.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Ensure the scheduler supports work stealing or equivalent load balancing.
- [ ] Ensure the scheduler can wake suspended tasks efficiently.

