# Native complex ABI

## Decision

Osiris will add a first-class f64 complex representation to the C runtime
before lowering general `eig`. Complex values are language values, not an
opaque numerical-provider result: they require stable printing, equality,
arithmetic, tensor storage, and ownership rules.

The initial native complex scope is IEEE-754 pairs of f64 values. The language
also has an interpreter/VM `complex<f32>` family, but native compilation
rejects an f32 complex literal until a separate f32 pair ABI and operation
rounding contract are implemented; it must not silently widen it to f64.
Arbitrary precision complex, decimal complex, and mixed exact-number promotion
remain outside this ABI.

## Scalar representation

Generated C will extend `OValue` with `O_COMPLEX` and the following payload:

```c
typedef struct { double real; double imag; } OComplex;
```

`O_COMPLEX` values are passed by value, require no allocation, and preserve
negative zero, NaN, and infinity exactly as their f64 components do. A real
numeric value may be promoted to complex only at a documented complex
operation boundary; ordinary f64 arithmetic remains unboxed f64 arithmetic.

The public renderer must use the same shortest-roundtrip component formatting
as f64 values. The canonical form is `a+bi` or `a-bi`; zero imaginary parts
remain complex when the value's kind is complex so a native result cannot lose
its semantic family during printing or equality.

Equality is componentwise IEEE equality, matching the current scalar numeric
rules. Consequently NaN components are not equal to themselves.

## Complex tensors

The initial complex tensor form is contiguous, rank-aware, row-major storage:

```c
typedef struct {
    OComplex *data;
    size_t len;
    size_t *shape;
    size_t rank;
} OComplexTensor;
```

It has the same owned shape and process-lifetime arena rules as the f64 tensor
ABI. A tensor has one element family: an f64 tensor does not silently become a
complex tensor because one operation happened to produce an imaginary value.
Explicit constructors or operations allocate the complex result.

The first tensor operations are construction, printing, equality, shape,
flattening, element indexing, reshape, copy-on-write mutation, prefix/range
slicing, conjugation, and elementwise `+`, `-`, `*`, and `/` with scalar and
same/broadcastable complex tensors. Matrix kernels follow only after those
primitive rules have parity tests.

## Provider extension

The next numerical-provider ABI version may add a general real-matrix
eigenvalue operation that writes complex results:

```c
OLinalgStatus (*eig_f64)(size_t n, double *row_major_matrix,
                         OComplex *values,
                         OComplex *row_major_vectors);
```

The generated adapter owns all three output buffers. It returns
`[complex_values, complex_vectors]`, with values shaped `[n]` and vectors
shaped `[n, n]`. It must not discard imaginary components or coerce them to
real values. Provider ordering is not stable; validation uses residuals and
subspace checks rather than exact vector equality.

## Implementation sequence

1. Add interpreter/VM parity tests for f64 complex rendering, equality, and
   scalar arithmetic where the language already exposes complex values.
2. Add `O_COMPLEX` scalar support to the native tagged ABI and direct scalar
   helpers, retaining f64 unboxed paths for purely real code.
3. Add the owned rank-aware complex tensor ABI with primitive operations.
4. Extend the numerical-provider table to version 2 with `eig_f64`.
5. Add provider-backed `eig` residual tests covering conjugate-pair spectra.

## Current implementation

Steps 1 and 2 are now complete for the native scalar ABI: `O_COMPLEX` stores
f64 real and imaginary components, and native lowering covers complex
literals, `complex`, `real`, `imag`, `conjugate`, `magnitude`, unary negation, arithmetic,
`abs`, equality, truthiness, and interpreter-matched rendering. Purely real paths
remain on their existing unboxed f64 lowering path.

Step 3 is in progress. The C backend now implements owned rank-aware complex
tensors for rectangular `tensor`/`matrix` construction, rendering, equality,
length/truthiness, `shape`, `flatten`, flat and coordinate `at`, rank-reducing
prefix indexing, `reshape`, copy-on-write `setat`, flattened `take`,
`conjugate`, and
half-open `slice` along the leading or an explicit axis. Elementwise `+`, `-`,
`*`, and `/` accept complex tensors, f64 tensors, and complex-compatible
scalars with right-aligned broadcast shapes. These operations have native
executable parity coverage.

Rank-two `transpose` and `matmul` now accept complex tensors and mixed
complex/f64 operands, returning owned complex tensors.
`trace` accepts a complex rank-two tensor and returns an `O_COMPLEX` scalar.
`det` accepts a complex square tensor and uses partial-pivot Gaussian
elimination with complex-magnitude pivot selection. These kernels preserve the
existing f64 fast paths for all-real tensors.
`sum`, `product`, and `dot` accept owned complex tensors; `dot` is bilinear
(it does not conjugate either input), matching `matmul`. `norm` returns the
real Euclidean magnitude `sqrt(sum(real² + imag²))`. All four retain their
direct f64 kernels when every operand is real.
`outer` likewise accepts real and complex tensors, flattens its inputs as the
existing public operation does, and returns an owned complex result whenever
either input is complex.
`solve` and `inv` accept complex square matrices and real or complex vector /
matrix right-hand sides. They use complex-magnitude partial pivoting and return
owned complex tensors whenever either input is complex.
`qr` accepts complex matrices and uses classical Gram-Schmidt with a Hermitian
inner product (`conjugate(Q)ᵀ A`), producing complex `Q` and `R` tensors.
`cholesky` accepts Hermitian positive-definite complex matrices and returns a
lower-triangular complex factor `L` such that `A = L · conjugate(L)ᵀ`.
Native `eig` now uses provider ABI v2 and returns owned complex tensors for
both values and vectors. A configured general provider is required; without
one it remains a deliberate compile-time diagnostic. The native suite includes
a non-real rotation-matrix residual check; production-provider selection and
broader numerical coverage remain.

## Kernel inventory and mixed-family rules

The native inventory is intentionally explicit. `f64` means the existing
real tensor helper remains selected when every input is real; `promotes` means
one complex tensor or complex scalar produces a newly owned complex tensor.

| Operation family | Native complex behavior | Conditions and result |
| --- | --- | --- |
| `tensor`, `matrix`, `vector` | supported | Rectangular data containing a complex value allocates `O_COMPLEX_TENSOR`; all-real data remains `O_TENSOR`. |
| `len`, `shape`, `flatten`, printing, equality, truthiness | supported | Shape is preserved; printing retains the complex family including `+0i`; equality is componentwise IEEE f64 equality. |
| `at`, `reshape`, `setat`, `take`, `slice` | supported | Flat/full/prefix indexing and owned copy-on-write mutation use the same rank/error rules as f64 tensors. A prefix replacement must be a complex tensor with the exact trailing shape. |
| `conjugate` | supported | Scalars and tensors return a fresh complex value/tensor with negated imaginary components. |
| elementwise `+`, `-`, `*`, `/` | promotes | f64 tensors and numeric scalars are converted to `(real, 0)` only at this operation boundary. Shapes use right-aligned broadcasting. |
| `sum`, `product`, `dot`, `norm`, `outer` | supported | `dot` is bilinear (no conjugation), `norm` is real `sqrt(sum(real² + imag²))`, and `outer` promotes if either side is complex. |
| `transpose`, `matmul`, `trace`, `det` | supported | Matrix operations preserve complex components; determinant uses complex-magnitude pivoting. |
| `solve`, `inv`, `qr`, `cholesky` | supported | Solve/inverse use complex pivoting; QR uses a Hermitian inner product; Cholesky requires Hermitian positive-definite input and satisfies `A = L · conjugate(L)ᵀ`. |
| `eig` | provider-backed | Input is currently a real f64 square matrix; ABI-v2 returns complex eigenvalues/vectors. No production provider is bundled. |
| `eigh`, `svd`, ranges, identity/eye | real-only or real-producing | Existing f64 behavior remains. Real results can subsequently participate in a documented promoting operation. |

### Error and IEEE behavior

- Complex division rejects a zero squared magnitude denominator. Tensor
  elementwise division applies that rule per element and reports `division by
  zero`; shape mismatch, invalid coordinate rank, out-of-range coordinates,
  and incompatible prefix replacement shapes retain the f64 tensor messages.
- Finite, infinite, NaN, and signed-zero components use f64 arithmetic and
  rendering. NaN compares unequal (including to itself); signed zero is kept
  in payloads and printing follows the existing f64 formatter.
- Every promoting kernel allocates independent data and copied shape metadata.
  No complex kernel mutates an f64 or complex input, including failed
  replacement and matrix-work-buffer paths.

Numerical factorization/vector tests use residual tolerances rather than exact
entry equality. `TestCBackendComplexTensorResiduals` bounds solve, Hermitian
Cholesky reconstruction, and QR reconstruction residuals by `1e-10`; the
provider `eig` residual test uses the same principle.
