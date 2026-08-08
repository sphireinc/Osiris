# Task 162: Close native C core and data matrix rows

**Priority:** P0
**Dependencies:** 148
**Status:** Ready

## Objective

Close only the native C matrix rows for core syntax/state, errors/results,
modules/patterns/tasks, numeric families, tensors, matrices, codecs, and
linear algebra. This task does not own native standard-library host I/O.

## Acceptance criteria

- Each owned matrix cell is backed by executable interpreter/VM/native
  differential fixtures, normal/race/sanitizer evidence, and exact docs.
- Unsupported shapes are source-spanned compile errors; no interpreter, VM, or
  shell fallback is allowed.
- The task updates only its owned native matrix cells after evidence is present.

## Current implementation evidence

The native implementation now has focused differential coverage for the
currently executable core/data slice in
`internal/osiris/task162_163_native_parity_test.go`:

- `TestTask162NativeCoreDataParityExpanded` covers payload-bearing Results,
  Option extraction, codec round-tripping, tensor construction, reductions,
  dot product, and shape reporting through the interpreter, VM, and generated C
  executable.
- `otensorsum`, `otensorproduct`, `otensordot`, and `otensornorm` preserve the
  numeric family and floating-point width required by their input values.
- Native Result rendering includes payloads, so generated output distinguishes
  `ok(7u64)` from an empty `ok`.
- Tuple construction and indexing are now a bounded native data slice:
  `irBuilder.evalExpr`, `emitCInstr`, `otuple`, `oprint_tuple_value`,
  `oprint_tuple_aware`, `atValue`, and `lenValue` preserve item order,
  direct tuple rendering, zero-based bounds checks, and tuple length. The same
  witness covers mixed-width items and nested tuple indexing through the
  interpreter, VM, and generated-C executable.
- The tuple witness passes the normal focused run, `go test -race -vet=off`,
  and the address-sanitized generated-C run using the installed LLVM clang.
  These are slice-level proofs, not full task closure evidence.

This evidence is intentionally incremental. The task remains **Ready** for
the Native C priority lane, but is not complete:
named product-type declarations and native lowering, remaining wide numeric
and dual/complex-width surfaces, sanitizer/race evidence, and cross-platform
generated-C evidence still require implementation or execution before any
matrix cell can be changed.
