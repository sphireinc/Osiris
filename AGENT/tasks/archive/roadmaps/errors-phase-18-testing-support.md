# Errors — PHASE 18: TESTING SUPPORT

Source: roadmaps/ERROR-CHK.txt, section PHASE 18: TESTING SUPPORT.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add assertions for expected error type.
- [ ] Add assertions for expected error kind/code.
- [ ] Add assertions for expected wrapped cause.
- [ ] Add assertions for expected metadata.
- [ ] Add assertions for expected validation field/path.
- [ ] Add assertions for expected panic/trap if supported.
- [ ] Add assertions for expected cancellation.
- [ ] Add assertions for expected timeout.
- [ ] Add helpers for comparing errors without relying on exact message text.
- [ ] Add helpers for snapshotting formatted errors safely.
- [ ] Add fake errors for standard categories.
- [ ] Add fault-injection support for I/O errors.
- [ ] Add fault-injection support for network errors.
- [ ] Add fault-injection support for allocation failures if applicable.
- [ ] Add tests for stack trace preservation.
- [ ] Add tests for cause-chain preservation.
- [ ] Add tests for redaction.
- [ ] Add tests for serialization.

