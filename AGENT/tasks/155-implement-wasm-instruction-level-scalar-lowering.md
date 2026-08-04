# Task 155: Implement WASM instruction-level scalar lowering

**Priority:** P0
**Dependencies:** 149,150
**Status:** Ready

## Objective

Replace the inspection-only scalar WASM body with actual instruction-level
lowering for scalar constants, unary operations, binary operations,
comparisons, branches, loops, returns, and deterministic cancellation checks.
The generated module must compute scalar results in WASM linear memory or
locals; result values must not be manufactured by replaying Osiris IR in a Go
runner.

## Exact scope

- Own `emitWASMInspectionBody`, `buildWASMModule`, scalar WASM instruction
  encoding, validation, and scalar execution fixtures.
- Define signed/unsigned integer widths, f32/f64 conversion, boolean, null,
  comparison, overflow, division-by-zero, and trap-to-structured-error rules.
- Export a versioned `run` ABI that writes bounded result records into linear
  memory and returns a status code.
- Enforce instruction, memory, output, and cancellation limits inside the
  generated execution contract.
- Do not add aggregate, closure, host-provider, or GPU behavior here; those
  belong to tasks 156 and 157.

## Acceptance criteria

- Scalar/control-flow fixtures execute through wazero and match interpreter and
  VM results without calling `executeWASMProgram` for semantics.
- Malformed modules, traps, limits, and cancellation have source-aware,
  deterministic errors.
- Generated WAT, binary ABI metadata, and `docs/design/WASM.md` document every
  exported function, memory field, status code, and scalar encoding.
- Focused normal, race, sanitizer-equivalent, and docs checks pass.
