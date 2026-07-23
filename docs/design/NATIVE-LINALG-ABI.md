# Native numerical-library ABI

## Decision

Osiris will not add ad hoc C implementations of production eigenvalue and SVD
algorithms. The native C backend will instead use an optional, explicitly
versioned numerical-provider ABI for the heavy decompositions:

- `eigh` for real symmetric f64 matrices;
- `svd` for real f64 matrices;
- eventually `eig`, only after Osiris has a native complex tensor ABI.

The ordinary C backend and its supported f64 tensor kernels remain dependency
free. A program that requires a provider operation must either link a selected
provider or receive a compile-time diagnostic. It must never fall back to the
Go interpreter at runtime.

## Why a provider boundary

LAPACK-class routines need careful workspace management, convergence behavior,
scaling, platform-specific optimized libraries, and numerical test suites.
Reimplementing them casually would be slower and less reliable than the system
and vendor libraries that already exist. The boundary lets Osiris retain its
own value semantics while adapting providers such as Accelerate, OpenBLAS with
LAPACK, MKL, or a future bundled implementation.

## Osiris-level contract

All initial provider inputs and outputs are contiguous row-major f64 tensors.
The generated C adapter owns conversion to a provider's layout and owns all
temporary workspace. Provider-owned pointers may not escape the call.

`eigh(matrix)` accepts a square matrix that has passed Osiris's existing
symmetric-matrix validation. It returns `[values, vectors]`:

- `values` has shape `[n]`, sorted in ascending order;
- `vectors` has shape `[n, n]`, with one eigenvector per column;
- the adapter maps non-convergence and invalid input to native Osiris errors.

`svd(matrix)` preserves the existing public shape contract, returning
`[U, singular_values, VT]`:

- `U` has shape `[rows, columns]`;
- `singular_values` has shape `[columns]`, ordered non-increasingly;
- `VT` has shape `[columns, columns]`.

This is intentionally an Osiris contract, not a direct copy of a LAPACK API.
Providers with compact or column-major outputs are normalized by the adapter.
For columns that have no defined left singular vector, the adapter must use the
same zero-column convention as the current interpreter.

General `eig(matrix)` is not eligible for this f64-only provider ABI. A real
non-symmetric matrix can have complex eigenvalues or eigenvectors. Until
complex scalar and tensor ownership, printing, equality, and lowering are
defined, native lowering must diagnose `eig` rather than drop imaginary data.
A later complex-capable provider operation must return a documented complex
values tensor and complex vector matrix.

## C provider surface

The generated native runtime will look up one versioned table supplied by the
embedding application or a selected build-time library:

```c
#define OSIRIS_LINALG_ABI_VERSION 1u

typedef enum {
    O_LINALG_OK = 0,
    O_LINALG_INVALID_ARGUMENT,
    O_LINALG_NOT_CONVERGED,
    O_LINALG_NO_MEMORY,
    O_LINALG_UNAVAILABLE
} OLinalgStatus;

typedef struct {
    unsigned abi_version;
    OLinalgStatus (*eigh_f64)(size_t n, double *row_major_matrix,
                              double *values, double *row_major_vectors);
    OLinalgStatus (*svd_f64)(size_t rows, size_t columns,
                             double *row_major_matrix,
                             double *row_major_u,
                             double *singular_values,
                             double *row_major_vt);
} OLinalgProviderV1;
```

The matrix argument is mutable provider workspace. The generated adapter makes
an owned copy of the Osiris input before calling either operation. A provider
must not retain any pointer it receives. It may use internal threading, but it
must not call back into the Osiris runtime.

The first integration may use a build-time selected provider and a generated
`const OLinalgProviderV1 *` symbol. Dynamic loading, environment-dependent
provider discovery, and silent fallback are deliberately out of scope.

## Current implementation status

The C backend now recognizes these operations as provider-gated. Without a
configured provider integration, `eigh` and `svd` produce a compile-time
diagnostic that names this ABI document; `eig` produces a separate diagnostic
that names the missing complex tensor ABI. The provider source/linker selection
is now available through `osiris build -linalg-provider provider.c`. The
selected C source is compiled and linked with generated C. `eigh` now calls the
provider's `eigh_f64` entry after Osiris validates a square symmetric f64
tensor; `svd` now calls `svd_f64` and returns the normalized `[U, s, VT]`
tensor list. Provider source contents are included in cached-build identities.

## Error and determinism rules

- No provider table: compile-time diagnostic naming the required provider.
- ABI version mismatch: executable startup failure before user code runs.
- Invalid argument: native Osiris argument diagnostic.
- No convergence: native Osiris numerical-convergence diagnostic.
- Provider allocation failure: native out-of-memory diagnostic.
- Output ordering is normalized by the adapter where the public contract
  requires it.

Eigenvector signs and bases inside degenerate eigenspaces are not stable API
values. Validation must use reconstruction, orthogonality, singular-value
ordering, and residual tolerances, not bytewise equality with one provider.

## Implementation sequence

1. Design the complex native ABI before considering general `eig` lowering.
2. Add benchmark and CI matrices for no-provider diagnostics and each selected
   provider.
