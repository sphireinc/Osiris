# Errors — PHASE 16: API AND LIBRARY DESIGN RULES

Source: roadmaps/ERROR-CHK.txt, section PHASE 16: API AND LIBRARY DESIGN RULES.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Require public APIs to document possible errors.
- [ ] Encourage specific error types over generic strings.
- [ ] Encourage stable machine-readable codes.
- [ ] Encourage wrapping lower-level errors with domain context.
- [ ] Encourage preserving causes.
- [ ] Encourage returning typed validation errors.
- [ ] Encourage distinguishing retryable from non-retryable failures.
- [ ] Encourage distinguishing caller error from server/internal error.
- [ ] Avoid exposing implementation-specific errors directly from high-level APIs.
- [ ] Avoid changing public error contracts in minor releases.
- [ ] Avoid making callers parse error strings.
- [ ] Provide conventions for naming error types.
- [ ] Provide conventions for naming error codes.
- [ ] Provide conventions for wrapping errors.
- [ ] Provide conventions for logging errors.
- [ ] Provide conventions for converting internal errors into user-facing responses.

