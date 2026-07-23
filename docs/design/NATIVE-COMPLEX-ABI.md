# Native complex ABI

## Decision

Osiris will add a first-class f64 complex representation to the C runtime
before lowering general `eig`. Complex values are language values, not an
opaque numerical-provider result: they require stable printing, equality,
arithmetic, tensor storage, and ownership rules.

The initial native complex scope is IEEE-754 pairs of f64 values. Arbitrary
precision complex, decimal complex, and mixed exact-number promotion remain
outside this ABI.

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
flattening, element indexing, reshape, conjugation, and elementwise `+`, `-`,
`*`, and `/` with scalar and same/broadcastable complex tensors. Matrix
kernels follow only after those primitive rules have parity tests.

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

Until steps 1 through 3 are complete, native `eig` remains a deliberate
compile-time diagnostic.
