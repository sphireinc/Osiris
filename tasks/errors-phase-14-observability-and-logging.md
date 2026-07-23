# Errors — PHASE 14: OBSERVABILITY AND LOGGING

Source: roadmaps/ERROR-CHK.txt, section PHASE 14: OBSERVABILITY AND LOGGING.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add standard error formatting for humans.
- [ ] Add standard error formatting for machines.
- [ ] Add structured log support for error metadata.
- [ ] Add stack trace capture where appropriate.
- [ ] Add source location capture where appropriate.
- [ ] Add cause-chain formatting.
- [ ] Add correlation ID propagation.
- [ ] Add request ID propagation.
- [ ] Add trace/span integration.
- [ ] Add severity mapping.
- [ ] Add retryability mapping.
- [ ] Add redaction hooks.
- [ ] Add crash-report formatting.
- [ ] Add telemetry-safe error summaries.
- [ ] Avoid logging secrets in messages, causes, metadata, or stack traces.
- [ ] Document logging patterns for expected versus unexpected errors.

