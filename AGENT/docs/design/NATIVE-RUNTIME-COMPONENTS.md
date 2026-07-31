# Native C runtime components

`emitCIRWithOptions` emits exactly one C translation unit.  Its runtime must
therefore be assembled in a fixed order; individual components are not linked
as a second runtime or selected dynamically.

## Ordered assembly

`cRuntimePrelude` emits these stable components, in order:

| Order | Component | Responsibility |
| --- | --- | --- |
| 1 | base value and memory runtime | `OValue` tags, scalar constructors, native arena allocation, strings, lists, closures, and calls. |
| 2 | base numeric and error runtime | fixed-width scalar operations, printing, truthiness, errors, and numeric conversion. |
| 3 | base f64 tensor runtime | rectangular construction, shape/index operations, f64 kernels, and tensor ownership. |
| 4 | base f64 linalg runtime | f64 matrix kernels and versioned numerical-provider calls. |
| 5 | complex tensor component | `O_COMPLEX_TENSOR` construction, rank-aware operations, complex matrix kernels, and ABI-v2 `eig`. |

The first four boundaries currently occupy the established base prelude in
`cbackend_native.go`; the complex component is physically isolated in
`cbackend_runtime_complex.go`.  This is deliberate: complex helpers wrap and
override established f64 helpers, so they must follow them in the translation
unit.  Subsequent extraction can split the base prelude at these documented
boundaries without changing the emitted order.

## Invariants

- Components append source only; none may replace or reorder another emitted
  component.
- Cross-component C symbols use the existing `o` prefix and have one
  definition.  The complex component calls renamed `*_real` base helpers and
  owns the wrapper definitions.
- The emitted string, not its Go file layout, is the cache/source-map input.
  A representative SHA-256 generated-C golden in
  `TestCBackendRuntimeComponentGolden` detects ordering or whitespace drift.
- Generated C remains a single source of ownership rules and one process
  lifetime arena, as specified in `NATIVE-ABI.md`.
