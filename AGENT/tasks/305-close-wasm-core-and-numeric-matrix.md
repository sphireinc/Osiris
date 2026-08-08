# Task 305: Close WASM core and numeric matrix rows

**Priority:** P0
**Dependencies:** 300,301
**Status:** Blocked

## Objective

Close WASM matrix rows for core syntax, typed functions, state, errors,
modules, patterns, tasks, numeric families, tensors, matrices, codecs, and
linear algebra after instruction-level lowering is complete.

## Acceptance criteria

- Each row has real-engine execution fixtures, interpreter/VM differential
  results, limits/cancellation/ownership evidence, and granular docs.
- Results are read from generated WASM memory/ABI records, never replayed by a
  Go IR evaluator.
- Matrix status changes are limited to rows with maintained evidence.
