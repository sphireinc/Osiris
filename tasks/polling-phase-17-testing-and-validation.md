# Polling — PHASE 17: TESTING AND VALIDATION

Source: roadmaps/POLLING-CHK.txt, section PHASE 17: TESTING AND VALIDATION.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add unit tests for context creation.
- [ ] Add unit tests for backend selection.
- [ ] Add unit tests for explicit backend selection.
- [ ] Add unit tests for backend unavailable errors.
- [ ] Add unit tests for handle validation.
- [ ] Add unit tests for duplicate handle registration.
- [ ] Add unit tests for watcher modification.
- [ ] Add unit tests for watcher removal.
- [ ] Add unit tests for inactive watcher behavior.
- [ ] Add unit tests for timeout behavior.
- [ ] Add unit tests for non-blocking poll behavior.
- [ ] Add unit tests for read readiness.
- [ ] Add unit tests for write readiness.
- [ ] Add unit tests for error readiness.
- [ ] Add unit tests for hang-up readiness.
- [ ] Add integration tests for TCP server workloads.
- [ ] Add integration tests for TCP client workloads.
- [ ] Add stress tests with thousands of watched handles.
- [ ] Add platform-specific tests for advanced backends.
- [ ] Automatically skip backend-specific tests when unsupported.
- [ ] Add regression tests for resource cleanup and descriptor leaks.
- [ ] Add race/concurrency tests if the language runtime is multi-threaded or task-based.

