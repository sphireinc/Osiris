# Async — PHASE 8: TESTING AND BENCHMARKS

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 8: TESTING AND BENCHMARKS.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add benchmarks for compute-heavy tensor workloads.
- [ ] Add benchmarks for I/O-heavy workloads.
- [ ] Add benchmarks for task creation and join overhead.
- [ ] Add benchmarks for cancellation and timeout overhead.
- [ ] Add benchmarks for C backend lowering of concurrency constructs.

