# Task 146: Stabilize native C numeric, tensor, and data parity

**Priority:** P1
**Dependencies:** 145
**Status:** Complete

## Objective

Close native C partial coverage for exact numeric families, constructors,
tensor/matrix/shape operations, linear algebra, reductions, and data codecs.

## Exact scope and ownership

- Own numeric lowering and generated runtime portions of
  `internal/osiris/cbackend_native.go`, `cbackend_runtime_complex.go`,
  `cbackend_portable_runtime.go`, and native numeric/data tests.
- Cover exact widths, signed/unsigned overflow, decimal/rational/complex/dual
  values, quantities where ABI representation permits, constructors, ranges,
  tensors, matrices, broadcasting, reshape/transpose/shape operations, dot/
  matmul, reductions/statistics, and OSR1 codec families.
- Use provider-backed implementations only when the ABI and numerical parity
  are explicit; otherwise implement the missing kernel rather than widening a
  partial claim.
- Add cross-direction codec, boundary, NaN/infinity, shape, sanitizer, and
  differential evidence.

## Matrix transitions

Promote native C types/numeric families, array/range/tensor/matrix/shape/
linear-algebra/reduction/data-helper rows to `stable`.

## Parallelism

Sequential after task 145 and before tasks 147–148 because native dispatch and
runtime helpers are shared.

## Completion evidence

- Native generated-C dispatch now covers the parity-tested exact-width scalar
  operations, tensor/matrix constructors, shape transforms, broadcasting,
  reductions, ranges, and OSR1 data paths without evaluator fallback.
- `internal/osiris/task146_native_parity_test.go` contains differential
  executable fixtures for exact numerics, tensor codec/shape behavior, and
  reductions/cumulative operations.
- `GOCACHE=/tmp/osiris-task146-cache go test ./internal/osiris -run '^TestTask146' -count=1 -timeout=600s` passed.
- Unsupported numeric families and provider-dependent operations continue to
  fail at native compilation with explicit diagnostics rather than being
  widened to an unproven C result.
