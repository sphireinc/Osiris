# Errors — PHASE 5: RESULT AND OPTION TYPES

Source: roadmaps/ERROR-CHK.txt, section PHASE 5: RESULT AND OPTION TYPES.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add helpers for mapping successful values.
- [ ] Add helpers for mapping error values.
- [ ] Add helpers for chaining fallible operations.
- [ ] Add helpers for converting `Option` into `Result`.
- [ ] Add helpers for converting `Result` into `Option`.
- [ ] Add helpers for retrying recoverable failures.
- [ ] Add helpers for aggregating multiple errors.
- [ ] Add helpers for collecting many fallible results.
- [ ] Add async/task-aware variants where needed.

