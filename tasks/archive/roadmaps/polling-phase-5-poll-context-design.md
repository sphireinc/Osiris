# Polling — PHASE 5: POLL CONTEXT DESIGN

Source: roadmaps/POLLING-CHK.txt, section PHASE 5: POLL CONTEXT DESIGN.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Decide on timeout units and make them consistent across the language.
- [ ] Normalize timeout overflow or invalid timeout values predictably.
- [ ] Allow limiting the maximum number of returned events.
- [ ] Decide whether `wait` returns watchers, event records, tuples, or callbacks.
- [ ] Ensure `wait` can report read, write, error, and hang-up conditions.
- [ ] Ensure polling behavior is safe when resources are closed during polling.
- [ ] Define whether poll contexts are reusable after errors.
- [ ] Define lifecycle rules for creating, using, and destroying poll contexts.
- [ ] Document whether poll contexts are thread-safe, task-local, or runtime-local.

