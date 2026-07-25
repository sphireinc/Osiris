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

ABI version 2 extends that prefix without changing the v1 `eigh` or `svd`
entries. A provider that advertises version 2 must use this layout and provide
`eig_f64`:

```c
typedef struct { double real; double imag; } OComplex;

typedef struct {
    unsigned abi_version; /* exactly 2 */
    OLinalgStatus (*eigh_f64)(size_t, double *, double *, double *);
    OLinalgStatus (*svd_f64)(size_t, size_t, double *, double *, double *,
                             double *);
    OLinalgStatus (*eig_f64)(size_t n, double *row_major_matrix,
                             OComplex *values,
                             OComplex *row_major_vectors);
} OLinalgProviderV2;
```

`eig_f64` accepts a mutable owned row-major f64 copy. It writes `n` complex
eigenvalues and an `n × n` row-major complex eigenvector matrix, with one
eigenvector per column. The generated adapter owns all buffers and returns
`[values, vectors]` as complex tensors even when every imaginary component is
zero. A v1 provider remains valid for `eigh` and `svd`, but cannot service
`eig`; an `eig` program requires a configured ABI-v2 provider.

The matrix argument is mutable provider workspace. The generated adapter makes
an owned copy of the Osiris input before calling either operation. A provider
must not retain any pointer it receives. It may use internal threading, but it
must not call back into the Osiris runtime.

The first integration may use a build-time selected provider and a generated
`const OLinalgProviderV1 *` symbol. Dynamic loading, environment-dependent
provider discovery, and silent fallback are deliberately out of scope.

## Production provider selection

The repository ships no numerical-library binary. The supported production
strategy is an explicit adapter source plus explicit linker arguments, which
keeps licensing, deployment, and CPU-library selection under the embedding
application's control.

On macOS, the maintained adapter is
`internal/osiris/providers/accelerate_lapack_provider.c`. It adapts Apple
Accelerate LAPACK `dsyevd`, `dgesdd`, and `dgeev` to ABI v2, including
row-major/column-major conversion and LAPACK's paired real representation of
complex eigenvectors. It is linked explicitly:

```sh
osiris build -o program \
  -linalg-provider internal/osiris/providers/accelerate_lapack_provider.c \
  -linalg-provider-flag=-framework \
  -linalg-provider-flag=Accelerate \
  program.osr
```

On other systems, supply an adapter with the same Osiris table and the local
LAPACK/OpenBLAS link arguments, for example repeated
`-linalg-provider-flag=-llapack` and `-linalg-provider-flag=-lblas` where
those libraries are installed. Osiris does not assume a particular package
manager, library path, or binary license.

The table's `abi_version` and non-null function entries are capability
discovery: v1 can serve `eigh`/`svd`; exactly v2 plus `eig_f64` is required
for `eig`. A missing source is a compile error; a missing symbol is a linker
error; a null/incompatible function entry becomes the deterministic native
provider diagnostic. Provider source content and every explicit provider link
argument participate in the executable cache key.

## Current implementation status

The C backend now recognizes these operations as provider-gated. Without a
configured provider integration, `eigh`, `svd`, and `eig` produce a compile-time
diagnostic that names this ABI document. `eig` specifically requires ABI v2.
The provider source/linker selection is now available through
`osiris build -linalg-provider provider.c`. The selected C source is compiled
and linked with generated C. `eigh` now calls the
provider's `eigh_f64` entry after Osiris validates a square symmetric f64
tensor; `svd` now calls `svd_f64` and returns the normalized `[U, s, VT]`
tensor list. `eig` now calls v2 `eig_f64` and returns complex value/vector
tensors. Provider source contents are included in cached-build identities.
The diagonal test provider is an ABI fixture only, not a general eigensolver;
it includes a rotation-matrix conjugate-pair fixture whose native test checks
eigenpair residuals rather than vector byte identity.

The Accelerate adapter is a production path, and its integration test checks
eigh reconstruction and orthogonality, SVD reconstruction and ordering, and
complex eig residuals at `1e-10`. It skips only when that macOS framework is
not available; the portable ABI fixture remains part of the regular suite.

## Error and determinism rules

- No provider table: compile-time diagnostic naming the required provider.
- ABI version mismatch or missing capability: deterministic native provider
  diagnostic when the requested operation is reached; no interpreter fallback.
- Invalid argument: native Osiris argument diagnostic.
- No convergence: native Osiris numerical-convergence diagnostic.
- Provider allocation failure: native out-of-memory diagnostic.
- Output ordering is normalized by the adapter where the public contract
  requires it.

Eigenvector signs and bases inside degenerate eigenspaces are not stable API
values. Validation must use reconstruction, orthogonality, singular-value
ordering, and residual tolerances, not bytewise equality with one provider.

## Implementation sequence

1. Add residual-based tests for non-real conjugate-pair spectra against a
   general selected provider; do not compare unstable eigenvectors bytewise.
2. Add benchmark and CI matrices for no-provider diagnostics and each selected
   provider.
