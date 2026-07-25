# Polling — PHASE 11: ERROR AND EXCEPTION DESIGN

Source: roadmaps/POLLING-CHK.txt, section PHASE 11: ERROR AND EXCEPTION DESIGN.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add an error for backend initialization failure.
- [ ] Add an error for unavailable backends.
- [ ] Add an error for invalid handles.
- [ ] Add an error for handles already being watched.
- [ ] Add an error for inactive watchers.
- [ ] Add an error for failed watcher modification.
- [ ] Add an error for failed wait operations.
- [ ] Add portable error codes for system failures.
- [ ] Add portable error codes for invalid arguments.
- [ ] Add portable error codes for permission failures.
- [ ] Add portable error codes for unsupported operations.
- [ ] Add portable error codes for too many watched resources.
- [ ] Add portable error codes for interrupted waits.
- [ ] Add portable error codes for would-block / try-again conditions.
- [ ] Decide whether timeout is an error, an empty result, or a distinct result state.
- [ ] Preserve native OS error details for diagnostics where possible.
- [ ] Keep user-facing error messages stable and language-level, not backend-specific.

