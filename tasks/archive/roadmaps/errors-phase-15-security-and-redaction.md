# Errors — PHASE 15: SECURITY AND REDACTION

Source: roadmaps/ERROR-CHK.txt, section PHASE 15: SECURITY AND REDACTION.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Separate internal debug messages from external user-facing messages.
- [ ] Add redaction for secrets, tokens, passwords, keys, and credentials.
- [ ] Add redaction for personally identifiable information where required.
- [ ] Add safe serialization mode for API responses.
- [ ] Add unsafe debug serialization mode for local development only.
- [ ] Prevent stack traces from being exposed to end users by default.
- [ ] Prevent raw SQL, headers, environment variables, and file paths from leaking by default.
- [ ] Prevent wrapped internal errors from bypassing redaction.
- [ ] Add tests for redaction behavior.
- [ ] Add lint rules for unsafe error construction.
- [ ] Add documentation for secure error handling.
- [ ] Define production versus development error output modes.

