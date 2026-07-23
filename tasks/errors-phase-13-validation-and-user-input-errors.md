# Errors — PHASE 13: VALIDATION AND USER INPUT ERRORS

Source: roadmaps/ERROR-CHK.txt, section PHASE 13: VALIDATION AND USER INPUT ERRORS.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add a standard validation error type.
- [ ] Allow validation errors to include field/path information.
- [ ] Allow validation errors to include expected constraints.
- [ ] Allow validation errors to include received values when safe.
- [ ] Allow validation errors to include multiple field failures.
- [ ] Allow validation errors to be converted into user-facing messages.
- [ ] Allow validation errors to be converted into machine-readable API responses.
- [ ] Distinguish invalid input from parse failure.
- [ ] Distinguish invalid input from permission failure.
- [ ] Distinguish invalid input from internal failure.
- [ ] Add helpers for building validation errors.
- [ ] Add helpers for aggregating validation errors.
- [ ] Add helpers for redacting sensitive field values.
- [ ] Add localization support for user-facing validation messages if in scope.

