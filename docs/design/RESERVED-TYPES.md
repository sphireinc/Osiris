# Reserved numeric and sparse type contract

Task 134 makes the five previously reserved names real shared-runtime values.
This contract intentionally describes the portable interpreter/VM surface;
artifact backends that do not carry these values must reject them explicitly.

## Generic forms and scalar family

The canonical forms are:

- `poly<T>`: an ordered coefficient vector in ascending power order.
- `quat<T>`: four components in `(w, x, y, z)` order.
- `simd<N, T>`: exactly `N` lanes.
- `sparse_matrix<T>`: a rank-two sparse value.
- `sparse_tensor<T>`: a rank-one-or-higher sparse value.

`T` is one of `number`, `f32`, or `f64` (or `bool` for a SIMD mask).
`number` is the default binary64
family; `f32` rounds after every scalar operation and `f64` retains binary64.
No implicit family conversion occurs inside a container. `N` is a literal
lane count and must be one of 2, 4, 8, or 16. `simd<N, bool>` is reserved for
masks and is constructed by `simd_mask` or the explicit boolean form; the arithmetic constructors accept
only the three numeric families.

## Algebraic values

`poly<T>(coefficients)` accepts a list or one-dimensional tensor of numeric
coefficients. Trailing zeroes are removed, so the zero polynomial has degree
`-1` and renders with an empty coefficient list. Addition, subtraction, and
multiplication require equal coefficient families. `poly_degree` and
`poly_eval` expose degree and Horner evaluation. Scalar division, quotient /
remainder, and roots are deliberately not part of this first deterministic
surface; unsupported operators report an error. Polynomial equality compares
normalized coefficients.

`quat<T>(w, x, y, z)` also accepts one four-element list. Multiplication is
the Hamilton product with the stated component order. `quat_conjugate`,
`quat_norm`, and `quat_inverse` are available; inverse and division reject a
zero norm. Quaternion ordering is undefined, while equality is componentwise.

## SIMD values

`simd<N,T>(lanes)` requires exactly `N` lanes. `+`, `-`, `*`, and `/` are
lane-wise and require equal shapes/families. `at` reads a lane and `setat`
returns an updated value without mutating its input. `simd_mask` creates a
boolean mask and `simd_select(mask, yes, no)` selects lane-wise. Alignment is
not observable and no platform vector ABI is promised; the portable ABI is an
owned ordered lane array.

## Sparse values

`sparse_matrix<T>([rows, columns], entries[, format])` takes entries of the form
`[row, column, value]`; format is `csr` or `csc` and is retained as metadata.
`sparse_tensor<T>(shape, entries[, format])` takes entries of the form
`[coordinate..., value]`; its format is `coo`. Dimensions and coordinates are non-negative and
bounded by the declared shape. Duplicate coordinates are coalesced in source
order by scalar addition; exact zero entries are removed. Stored entries are
then sorted lexicographically by coordinate, independent of format. Missing
coordinates read as the family zero. `shape`, `nnz`, `at`, `setat`,
`to_dense`, and `to_sparse` are shared operations. Updates return a new value,
and iteration yields canonical `[coordinate, value]` pairs.

The codec stores the family, shape, format, and canonical entries in versioned
`OSR1` tags `0x22` through `0x26`. JSON uses explicit family envelopes.

## Backend boundary

Interpreter and bytecode VM share the same tagged values and operations. The
current native C, WASM, and GPU artifact paths do not carry this contract;
they must reject a program that reaches any of these families with a source
spanned unsupported-type diagnostic. No backend may evaluate the program at
compile time to manufacture output. A future optimized SIMD ABI or sparse
allocator may be added only with ownership, aliasing, and executable parity
evidence.
