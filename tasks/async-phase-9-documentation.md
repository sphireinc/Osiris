# Async — PHASE 9: DOCUMENTATION

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 9: DOCUMENTATION.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Document the concurrency model in `LANG.txt`.
- [ ] Document the I/O model in `IO.txt`.
- [ ] Document the async and concurrency plan in `ASYNC-PLAN.txt`.
- [ ] Document the implementation checklist in `ASYNC-PLAN-CHK.txt`.
- [ ] Document the runtime semantics of `spawn`, `await`, `parallel`, `cancel`, and `timeout`.
- [ ] Document which I/O calls are synchronous and which are async-aware.
- [ ] Document how the C backend lowers async constructs.
- [ ] Document the performance model for numeric code versus wait-heavy code.
- [ ] Osiris can express structured concurrency without a JS-style event loop.
- [ ] Osiris can overlap I/O effectively while keeping synchronous source semantics readable.
- [ ] Osiris can keep compute-heavy code fast through vectorization, cache locality, parallelism, memory layout, and native lowering.
- [ ] The C backend can lower concurrency constructs cleanly.
- [ ] The runtime can schedule, suspend, resume, cancel, and time out tasks predictably.
- [ ] The docs tell users how to write fast concurrent Osiris code without obscuring the language model.

