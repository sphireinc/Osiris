# Errors — PHASE 17: INTEROPERABILITY AND FFI

Source: roadmaps/ERROR-CHK.txt, section PHASE 17: INTEROPERABILITY AND FFI.

## Native compiler integration

P1 native requirement: follow the generated-C error ABI from task 002-native-value-abi; preserve Osiris errors without relying on Go interpreter values.

## Remaining checklist

- [ ] Define how language errors cross native boundaries.
- [ ] Define how native errors become language errors.
- [ ] Define how exceptions interact with C ABI boundaries.
- [ ] Define whether panics may cross FFI boundaries.
- [ ] Define whether stack traces include native frames.
- [ ] Define how OS error codes are preserved.
- [ ] Define how foreign library error codes are preserved.
- [ ] Define ownership rules for error messages returned from native code.
- [ ] Define memory-safety rules for error objects crossing FFI.
- [ ] Define serialization rules for errors crossing process boundaries.
- [ ] Add safe wrappers for common native error conventions.
- [ ] Add tests for FFI error translation.
- [ ] Make FFI failure modes explicit in documentation.

