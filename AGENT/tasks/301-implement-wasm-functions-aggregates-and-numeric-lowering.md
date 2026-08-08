# Task 301: Implement WASM functions, aggregates, and numeric lowering

**Priority:** P0
**Dependencies:** 300
**Status:** Blocked

## Objective

Lower Osiris functions, recursion, closures, mutable cells, lists, tuples,
enums, tensors, exact represented numeric families, and codecs into executable
WASM instructions and owned linear-memory records.

## Exact scope

- Own `wasm_provider.go` IR-to-WASM lowering for calls, captures, cells,
  aggregates, patterns, tensor operations, and numeric operations.
- Define stable layouts for strings, byte lists, tuples, enums, tensors,
  complex values, fixed-width integers, quantities, Result, and Option.
- Implement bounds checks, copy/borrow rules, recursion limits, and cleanup for
  every generated aggregate allocation.
- Differentially test interpreter, VM, native-compatible fixtures, and WASM
  for functions, closures, mutation, patterns, exact arithmetic, tensors, and
  codecs.

## Acceptance criteria

- `ExecuteWASM` obtains all semantic outputs from the generated module's
  memory/result ABI, not from a Go IR evaluator.
- Aggregate and numeric round trips are deterministic and bounded.
- Unsupported values fail at compile time with source spans and no fallback.
- Every layout and ownership rule is documented granularly in
  `docs/design/WASM.md` and the public reference.
