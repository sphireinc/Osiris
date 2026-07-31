# Polling — PHASE 6: POLLABLE HANDLE ABSTRACTION

Source: roadmaps/POLLING-CHK.txt, section PHASE 6: POLLABLE HANDLE ABSTRACTION.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add poll handles for file or stream resources.
- [ ] Add poll handles for socket resources.
- [ ] Add poll handles for process pipes.
- [ ] Add poll handles for runtime-managed network connections.
- [ ] Add poll handles for TLS-wrapped connections where possible.
- [ ] Add poll handles for HTTP/client transport integrations.
- [ ] Add poll handles for custom native/runtime extensions.
- [ ] Decide whether user-defined types may implement pollable handles directly.
- [ ] If user-defined handles are allowed, define a safe descriptor and validity contract.
- [ ] If user-defined handles are not allowed, provide wrapper types for supported resources.
- [ ] Add validity checks for handles before registration.
- [ ] Add validity checks for handles during watcher modification.
- [ ] Define behavior when a handle becomes invalid while being watched.

