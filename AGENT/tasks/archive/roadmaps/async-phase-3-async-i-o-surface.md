# Async — PHASE 3: ASYNC I/O SURFACE

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 3: ASYNC I/O SURFACE.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add network I/O primitives for socket-based workloads.
- [ ] Make blocking semantics explicit and predictable in docs and runtime behavior.
- [ ] Decide which I/O calls may suspend a task and document those suspension points.

