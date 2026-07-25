# Native numeric and tensor lowering

Source: OPTCHK.md, TYPECHK.txt, and ASYNC-PLAN-CHK.txt.

Lower u8/u16/u32/u64, i64, f32, and f64 first. Then add one contiguous
elementwise f64 tensor kernel with one-time shape checks, alias safety, and a
scalar fallback. Decimal, complex, and big-number paths follow as runtime calls.

Acceptance: parity tests cover every supported scalar and kernel; generated
output reports fallback reasons; no generic dispatch remains inside supported
inner loops.

Progress: scalar-only CFGs lower u64/i64/f32/f64 arithmetic to direct C local
operations and have native/interpreter parity coverage. Tagged native lowering
now preserves `u8/u16/u32/u64`, `byte`, and `i8/i16/i32/i64` literal/cast
widths and same-family tagged arithmetic/bitwise result widths; unboxed
narrow-integer specialization, mixed integer families, decimal/bigint paths,
and complete scalar coverage
remain open. The first contiguous f64 tensor
kernel now lowers list-backed one-dimensional tensors with one shape check and
direct elementwise arithmetic, including numeric scalar broadcasting in either
operand position; higher-rank/tensor broadcasting, alias safety, scalar
fallback comments in generated C explain when the tagged ABI is selected;
native one-dimensional `shape`/`flatten`/`sum`/`product`/`dot`/`norm` parity
is covered. Native `std/math.zeros`, `ones`, and `full` now construct this
same tensor form for one scalar dimension or a one-element shape list;
`arange`, `linspace`, and `logspace` also lower natively to this form.
Native scalar `min`/`max`/`clamp` parity is also covered. The contiguous f64
ABI now carries rank and shape metadata, and multidimensional fill
construction, full-shape reporting, equality, and same-shape/scalar kernels
have parity coverage. Rectangular nested-list `tensor` construction and all
supported `matrix` constructor forms now use the same ABI. Direct f64
elementwise kernels now implement general NumPy-style tensor broadcasting;
native `at(tensor, [coordinate...])` supports validated rank-aware element
lookup and rank-reducing prefix slices while scalar `at` retains flat indexing.
Rank-two `transpose` and
`matmul` use direct native kernels. Tensor `setat` now performs alias-safe
copy-on-write replacement for flat, full-coordinate, and prefix-coordinate
indexes; prefix replacement requires an equal-shaped trailing tensor. Broader
mutation and benchmark coverage remain open. Native `take` copies a flattened
tensor prefix into a one-dimensional output tensor. Native `identity(size)` and
`identity(rows, columns)`, together with the `eye` alias, construct owned
rank-two f64 diagonal tensors with direct fill kernels. `slice(tensor, start,
end[, axis])` now copies a half-open range along any tensor axis while
retaining all other dimensions. Rank-two `trace` and square-matrix `det` now
lower to direct f64 reduction kernels, with determinant using partial pivoting.
Complex rank-two `trace` and square-matrix `det` now lower as well; the latter
uses complex-magnitude partial pivoting while preserving the real fast path.
Complex tensor `sum`, `product`, bilinear `dot`, and Euclidean `norm` now
lower directly, preserving the corresponding f64 fast paths.
Complex `outer` now lowers directly as a rank-two bilinear tensor product.
Complex `solve` and `inv` now lower through complex-magnitude partial-pivot
Gauss-Jordan elimination, including real/complex RHS mixing.
Complex `qr` now lowers through Hermitian classical Gram-Schmidt.
Complex `cholesky` now lowers for Hermitian positive-definite matrices.
Rank-two `solve` and `inv` now lower through partial-pivot Gauss-Jordan
elimination for non-empty square f64 matrices and tensor vector/matrix RHSs.
Rank-two `cholesky` now lowers to an interpreter-matched lower-triangular f64
kernel with positive-pivot validation. Rank-two `qr` now lowers to the
interpreter-matched classical Gram-Schmidt kernel and returns owned `[Q, R]`
tensors. `outer` now lowers for f64 tensors of any rank after flattening both
operands into a direct rank-two product kernel. The remaining `eigh` and `svd`
work is implemented through the optional versioned numerical-provider ABI in
`docs/design/NATIVE-LINALG-ABI.md`; native `eig` remains blocked on a real
complex tensor ABI specified in `docs/design/NATIVE-COMPLEX-ABI.md`. The
native tagged ABI now has f64 complex scalar parity for literals,
construction, arithmetic, equality, real/imaginary projection, conjugation,
magnitude, absolute value, truthiness, and rendering. Owned rank-aware complex
tensors now cover construction, shape/flattening, indexing, reshape,
copy-on-write mutation, slicing, broadcasting, and conjugation; reductions,
`outer`, complex `solve`/`inv`/`qr`/`cholesky`, and selected matrix kernels
now have direct parity coverage. Production-provider numerical coverage
remains. Native
`eig` now lowers through ABI-v2 `eig_f64` into complex value/vector tensors;
the bundled provider is a diagonal/rotation residual fixture, not a general
eigensolver.
