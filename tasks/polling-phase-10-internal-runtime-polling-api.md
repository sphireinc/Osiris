# Polling — PHASE 10: INTERNAL RUNTIME POLLING API

Source: roadmaps/POLLING-CHK.txt, section PHASE 10: INTERNAL RUNTIME POLLING API.

## Native compiler integration

P1 native requirement: when implemented, this runtime integration must be callable by generated C through the native runtime ABI.

## Remaining checklist

- [ ] Add internal functions for creating a poll context.
- [ ] Add internal functions for initializing a poll context.
- [ ] Add internal functions for adding a descriptor or handle.
- [ ] Add internal functions for modifying watched events.
- [ ] Add internal functions for removing a descriptor or handle.
- [ ] Add internal functions for waiting with timeout.
- [ ] Add internal functions for retrieving backend errors.
- [ ] Add internal functions for destroying poll contexts.
- [ ] Provide extension/module hooks for registering custom handle types.
- [ ] Keep low-level descriptor access internal unless the language intentionally exposes it.
- [ ] Consolidate ad-hoc runtime polling code onto the shared internal API.
- [ ] Use the internal API for signal handling where appropriate.
- [ ] Use the internal API for timers where appropriate.
- [ ] Use the internal API for server accept loops where appropriate.
- [ ] Use the internal API for async runtime scheduling where appropriate.

