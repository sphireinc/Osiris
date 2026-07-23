# Async — PHASE 2: STRUCTURED PARALLELISM

Source: roadmaps/ASYNC-PLAN-CHK.txt, section PHASE 2: STRUCTURED PARALLELISM.

## Native compiler integration

This does not block P0 native lowering. If it crosses a native boundary, follow docs/design/NATIVE-COMPILER.md and the runtime ABI.

## Remaining checklist

- [ ] Add an explicit completion-order mode only if it is needed.

