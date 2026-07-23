# Polling — PHASE 7: WATCHER MODEL

Source: roadmaps/POLLING-CHK.txt, section PHASE 7: WATCHER MODEL.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add a way to modify the watched events for an existing watcher.
- [ ] Add a way to modify watcher-associated user data.
- [ ] Add a way to remove a watcher from its poll context.
- [ ] Define whether removing a watcher is idempotent.
- [ ] Define whether inactive watchers can be inspected after removal.
- [ ] Define whether inactive watchers can ever be reactivated.
- [ ] Prevent duplicate watcher registration for the same handle unless explicitly supported.
- [ ] Decide whether multiple contexts may watch the same handle.
- [ ] Define watcher ownership and cleanup semantics.
- [ ] Ensure watcher removal releases backend-specific registrations.

