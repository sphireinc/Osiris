# Polling — PHASE 14: PERFORMANCE AND SCALABILITY

Source: roadmaps/POLLING-CHK.txt, section PHASE 14: PERFORMANCE AND SCALABILITY.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Benchmark small descriptor sets.
- [ ] Benchmark large descriptor sets.
- [ ] Benchmark many idle connections.
- [ ] Benchmark many active connections.
- [ ] Benchmark add/modify/remove watcher overhead.
- [ ] Benchmark timeout-heavy workloads.
- [ ] Benchmark edge-triggered mode where supported.
- [ ] Compare fallback backend behavior against advanced backends.
- [ ] Document expected complexity for each backend.
- [ ] Avoid O(n) scans where backend support allows scalable dispatch.
- [ ] Avoid unnecessary allocations in hot polling paths.
- [ ] Reuse event buffers where safe.
- [ ] Ensure polling does not starve the task scheduler.
- [ ] Ensure polling does not block unrelated runtime work.

