# Scientific standard-library gaps

Source: TYPECHK.txt.

Add deterministic random generation with an explicit seed and a minimal signal
module. Defer BLAS/LAPACK linkage until FFI and native ABI work are established.

Acceptance: seeded sequences are reproducible; signal functions have reference
tests and documentation; accelerated implementations expose a clear fallback.

## Completion evidence

- Added pure `random(seed[, counter])`: explicit seed/counter inputs, no
  mutable process state, stable `[0, 1)` f64-backed result, and interpreter/VM
  reproducibility coverage in `TestSeededRandomIsReproducibleAcrossRuntimeAndVM`.
- Added `std/signal.hann(length)` and `std/signal.convolve(signal, kernel)` as
  deterministic scalar reference kernels. Their rank/value/shape behavior and
  interpreter/VM parity are covered by
  `TestSignalModuleReferenceKernelsAcrossRuntimeAndVM`.
- The canonical React reference documents all three functions with signatures,
  parameters, result shapes, return values, examples, and an explicit absence
  of SIMD/FFT/GPU/native-provider acceleration. Search validation includes
  `convolve`.
