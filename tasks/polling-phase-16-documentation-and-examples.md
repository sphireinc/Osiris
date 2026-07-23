# Polling — PHASE 16: DOCUMENTATION AND EXAMPLES

Source: roadmaps/POLLING-CHK.txt, section PHASE 16: DOCUMENTATION AND EXAMPLES.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add a basic TCP server example.
- [ ] Add a basic TCP client example.
- [ ] Add a callback-based event loop example.
- [ ] Add a watcher modification example.
- [ ] Add a watcher removal example.
- [ ] Add a timeout example.
- [ ] Add a non-blocking poll example.
- [ ] Add an edge-triggered example.
- [ ] Add a backend introspection example.
- [ ] Add examples showing correct EOF, hang-up, and error handling.
- [ ] Add examples showing cancellation with polling.
- [ ] Add examples showing integration with the language task scheduler.
- [ ] Document what polling is.
- [ ] Document what polling is not.
- [ ] Document when to use polling directly versus higher-level async APIs.

