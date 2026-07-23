# Errors — PHASE 9: COMPILER AND STATIC ANALYSIS

Source: roadmaps/ERROR-CHK.txt, section PHASE 9: COMPILER AND STATIC ANALYSIS.

## Native compiler integration

P1 native requirement: follow the generated-C error ABI from task 002-native-value-abi; preserve Osiris errors without relying on Go interpreter values.

## Remaining checklist

- [ ] Add must-handle annotations for fallible APIs.
- [ ] Add compiler warnings for discarded results.
- [ ] Add compiler errors for discarded critical results.
- [ ] Add lint rules for empty catch blocks.
- [ ] Add lint rules for catch-all handlers.
- [ ] Add lint rules for swallowing errors silently.
- [ ] Add lint rules for panic/throw in cleanup paths.
- [ ] Add lint rules for string-matching error messages.
- [ ] Add lint rules for losing wrapped causes.
- [ ] Add lint rules for converting all errors to generic internal errors.
- [ ] Add lint rules for returning user-facing details from internal errors.
- [ ] Add lint rules for retry loops without backoff.
- [ ] Add lint rules for blocking calls without timeout.
- [ ] Add lint rules for cancellation-unaware operations.
- [ ] Add static checks for exhaustive error handling where possible.
- [ ] Add static checks for unreachable error branches where possible.
- [ ] Add IDE support for generated error-handling skeletons.

