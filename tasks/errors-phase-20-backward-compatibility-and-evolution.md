# Errors — PHASE 20: BACKWARD COMPATIBILITY AND EVOLUTION

Source: roadmaps/ERROR-CHK.txt, section PHASE 20: BACKWARD COMPATIBILITY AND EVOLUTION.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Define whether adding a new error type is a breaking change.
- [ ] Define whether adding a new error code is a breaking change.
- [ ] Define whether changing an error message is a breaking change.
- [ ] Define whether changing wrapped causes is a breaking change.
- [ ] Define whether changing stack trace formatting is a breaking change.
- [ ] Define whether changing metadata keys is a breaking change.
- [ ] Version public error codes.
- [ ] Keep user-facing messages separate from stable programmatic codes.
- [ ] Add deprecation paths for old error types.
- [ ] Add compatibility shims for renamed errors.
- [ ] Add migration tools for major error model changes.
- [ ] Keep the core error model small enough to evolve safely.

