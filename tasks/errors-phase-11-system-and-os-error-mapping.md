# Errors — PHASE 11: SYSTEM AND OS ERROR MAPPING

Source: roadmaps/ERROR-CHK.txt, section PHASE 11: SYSTEM AND OS ERROR MAPPING.

## Native compiler integration

P1 native requirement: follow the generated-C error ABI from task 002-native-value-abi; preserve Osiris errors without relying on Go interpreter values.

## Remaining checklist

- [ ] Add a portable mapping layer for OS errors.
- [ ] Map file-not-found errors consistently.
- [ ] Map permission-denied errors consistently.
- [ ] Map already-exists errors consistently.
- [ ] Map would-block errors consistently.
- [ ] Map interrupted-operation errors consistently.
- [ ] Map connection-refused errors consistently.
- [ ] Map connection-reset errors consistently.
- [ ] Map broken-pipe errors consistently.
- [ ] Map timed-out errors consistently.
- [ ] Map address-in-use errors consistently.
- [ ] Map address-not-available errors consistently.
- [ ] Map disk-full errors consistently.
- [ ] Map too-many-open-files errors consistently.
- [ ] Preserve native errno / platform code.
- [ ] Preserve native platform message when useful.
- [ ] Avoid making platform-specific errors impossible to inspect.
- [ ] Document unavoidable platform differences.

