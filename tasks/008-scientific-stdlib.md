# Scientific standard-library gaps

Source: TYPECHK.txt.

Add deterministic random generation with an explicit seed and a minimal signal
module. Defer BLAS/LAPACK linkage until FFI and native ABI work are established.

Acceptance: seeded sequences are reproducible; signal functions have reference
tests and documentation; accelerated implementations expose a clear fallback.
