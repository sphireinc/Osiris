# Async — PHASE 5: COMPILER AND LOWERING

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 5: COMPILER AND LOWERING.

## Native compiler integration

P0 prerequisite: complete tasks 001-c-backend-real-lowering and 002-native-value-abi first. This work must lower to generated C, not exist only in the Go runtime.

## Remaining checklist

- [ ] Represent `spawn` and `await` as explicit IR boundaries.
- [ ] Represent suspension points explicitly in the IR.
- [ ] Lower async control flow into resumable state machines only where needed.
- [ ] Keep compute-heavy regions in straight-line or loop-heavy form when they have no suspension points.
- [ ] Emit small runtime helper calls for scheduling, suspension, resume, timeout, and cancellation.
- [ ] Emit direct loops for compute-heavy regions.
- [ ] Emit state machines only around actual suspension points.
- [ ] Keep the C backend as the standard native lowering path.
- [ ] Ensure the C backend remains readable and auditable for concurrency lowering.
- [ ] Compile the emitted C with aggressive optimization flags.

