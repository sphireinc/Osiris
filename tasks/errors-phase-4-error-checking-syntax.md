# Errors — PHASE 4: ERROR CHECKING SYNTAX

Source: roadmaps/ERROR-CHK.txt, section PHASE 4: ERROR CHECKING SYNTAX.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add binding syntax for successful values.
- [ ] Add binding syntax for error values.
- [ ] Add exhaustiveness checking for closed error sets.
- [ ] Add compiler warnings for ignored errors.
- [ ] Add compiler errors for ignored must-handle errors.
- [ ] Add syntax for intentionally discarding an error.

