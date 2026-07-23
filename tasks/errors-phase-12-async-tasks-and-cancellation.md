# Errors — PHASE 12: ASYNC, TASKS, AND CANCELLATION

Source: roadmaps/ERROR-CHK.txt, section PHASE 12: ASYNC, TASKS, AND CANCELLATION.

## Native compiler integration

P1 native requirement: follow the generated-C error ABI from task 002-native-value-abi; preserve Osiris errors without relying on Go interpreter values.

## Remaining checklist

- [ ] Define how errors propagate across async tasks.
- [ ] Define how errors propagate from child tasks to parent tasks.
- [ ] Define how task groups aggregate errors.
- [ ] Define whether the first failure cancels sibling tasks.
- [ ] Define whether multiple task failures are preserved.
- [ ] Define whether cancellation has its own error type.
- [ ] Define whether cancellation is catchable.
- [ ] Define whether cancellation is automatically propagated.
- [ ] Define whether cancellation may interrupt cleanup.
- [ ] Define whether timeout wraps cancellation or remains separate.
- [ ] Add structured concurrency error rules if the language supports structured concurrency.
- [ ] Add async-aware stack traces where possible.
- [ ] Add async-aware source locations where possible.
- [ ] Avoid losing task boundary information in error reports.

