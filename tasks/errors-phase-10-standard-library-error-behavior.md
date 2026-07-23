# Errors — PHASE 10: STANDARD LIBRARY ERROR BEHAVIOR

Source: roadmaps/ERROR-CHK.txt, section PHASE 10: STANDARD LIBRARY ERROR BEHAVIOR.

## Native compiler integration

P1 native requirement: follow the generated-C error ABI from task 002-native-value-abi; preserve Osiris errors without relying on Go interpreter values.

## Remaining checklist

- [ ] Standardize file-system errors.
- [ ] Standardize network errors.
- [ ] Standardize parsing errors.
- [ ] Standardize encoding/decoding errors.
- [ ] Standardize process execution errors.
- [ ] Standardize concurrency errors.
- [ ] Standardize cancellation errors.
- [ ] Standardize timeout errors.
- [ ] Standardize permission errors.
- [ ] Standardize resource exhaustion errors.
- [ ] Standardize unsupported platform errors.
- [ ] Ensure standard errors expose machine-readable kinds.
- [ ] Ensure standard errors expose human-readable messages.
- [ ] Ensure standard errors expose original system/native error codes when relevant.
- [ ] Ensure standard errors avoid leaking secrets.
- [ ] Ensure standard errors are stable enough for library authors to depend on.
- [ ] Ensure standard library docs list possible errors for each fallible API.

