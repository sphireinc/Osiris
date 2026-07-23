# Errors — PHASE 8: CONTEXT AND WRAPPING

Source: roadmaps/ERROR-CHK.txt, section PHASE 8: CONTEXT AND WRAPPING.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add standard `as` / `cast` checks for wrapped errors.
- [ ] Add support for multiple causes where useful.
- [ ] Add support for error groups from parallel operations.

