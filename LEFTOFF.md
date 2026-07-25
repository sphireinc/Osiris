# Native lowering handoff

Last updated: 2026-07-24.

## Current compiler path

Osiris now uses real IR-to-C lowering in `internal/osiris/cbackend_native.go`.
`CompileC` builds IR and emits executable C; it does not evaluate the program
through `Session.ExecProgram` to create an output snapshot. The system C
compiler remains the first native code-generation backend.

Implemented and covered by native executable parity tests:

- tagged C runtime values, strings, lists, one-dimensional f64 tensors, and
  direct tensor kernels;
- scalar branches, loops, direct calls, direct lexical captures, closure
  values, closure capture ownership, and indirect closure calls;
- unboxed scalar main-CFG lowering for the proven u64/i64/f32/f64 subset;
- fixed-width tagged literal/cast preservation for `u8/u16/u32/u64`, `byte`,
  and `i8/i16/i32/i64`, including same-family tagged arithmetic and bitwise
  result-width preservation;
- static `std/math` import/member resolution, including one local module alias;
- static `std/math.pi` and `tau` constants with shortest-roundtrip float
  rendering;
- static `std/math.vec`/`vector` lowering for the contiguous f64 tensor ABI;
- static `std/math.zeros`/`ones`/`full` construction for a one-dimensional
  shape and numeric f64 fill values;
- static one-dimensional f64 `std/math.arange`/`linspace`/`logspace` range
  constructors, including their optional arguments and interpreter defaults;
- rank-aware contiguous f64 tensors: multidimensional `zeros`/`ones`/`full`,
  full `shape` output, and shape-preserving elementwise/scalar kernels;
- recursive rectangular nested-list `tensor` lowering and native `matrix`
  construction (including rows/columns/data);
- native `reshape` with element-count validation and owned rank-aware output;
- direct rank-two native `transpose` and `matmul` kernels;
- native flattened f64 `outer` product kernel;
- direct rank-two f64 `trace` and partial-pivot `det` kernels;
- native partial-pivot f64 `solve` and `inv` for non-empty square matrices;
- native rank-two f64 `cholesky` with interpreter-matched positive-pivot
  validation;
- native rank-two f64 `qr`, returning owned `[Q, R]` tensors;
- direct rank-aware NumPy-style broadcasting for f64 tensor `+`, `-`, `*`,
  and `/` operations;
- rank-aware `at(tensor, [coordinate...])` element indexing alongside the
  existing scalar flat-index form, including owned rank-reducing prefix slices;
- alias-safe copy-on-write tensor `setat` for flat and full-coordinate index
  forms, plus equal-shaped tensor replacement through prefix coordinates;
- native tensor `take` for owned flattened prefix slices;
- native `slice(tensor, start, end[, axis])` for owned half-open ranges along
  any axis with retained remaining dimensions;
- native `identity`/`eye` construction for square and rectangular rank-two
  f64 diagonal tensors;
- variadic scalar `std/math.min`/`max` and ternary `clamp` lowering;
- native `std/math` scalar operations currently including `abs`, casts,
  roots/logs/exponentials, `hypot`, trigonometric, inverse-trigonometric,
  hyperbolic, inverse-hyperbolic, error/gamma, rounding, and cast helpers;
- Unicode code-point string indexing through `at`;
- C executable caching, deterministic source testing, fuzzing, and an
  unboxed-loop benchmark; `./bench.sh` provides a parity-checked, repeated
  interpreter/VM/native execution comparison with a separate native build
  time.

## Last completed increment

Work in progress: the native C ABI now has an owned `O_COMPLEX_TENSOR` form.
`tensor`/`matrix` construction selects it for rectangular data containing a
complex value, while all-real data retains the f64 `O_TENSOR` path. Native
printing, equality, length, and truthiness recognize the new form. This is a
tested checkpoint via `TestCBackendComplexScalarParity`, which now also covers
complex tensor construction, output, `shape`, `flatten`, flat indexing, full
coordinate indexing, owned prefix slices, reshape, and alias-safe copy-on-
write `setat` for full and prefix coordinates, right-aligned broadcast
arithmetic for `+`, `-`, `*`, and `/` across complex tensors, f64 tensors, and
complex-compatible scalars, flattened `take`, owned half-open `slice` on the
leading or explicit axis, `conjugate` for complex scalars and tensors, and
rank-two complex `transpose`, `trace`, and partial-pivot `det`, complex
`sum`/`product`/bilinear `dot`/Euclidean `norm`, mixed complex/f64 `matmul`,
complex `outer`, complex partial-pivot `solve`/`inv`, Hermitian complex `qr`,
and Hermitian positive-definite complex `cholesky`. Native `eig`
now lowers through provider ABI v2, returning complex value/vector tensors.
The test fixture checks a non-real conjugate-pair spectrum by residual. The
remaining eig work is selecting/integrating a production general provider and
broader numerical coverage.

The latest increment adds native f64 complex scalar parity. The tagged C ABI
now carries `O_COMPLEX` values and lowers complex literals, `complex`, `real`,
`imag`, `magnitude`, `abs`, unary negation, arithmetic, equality, truthiness, and
canonical interpreter-matched rendering. It has a native executable parity
test in `TestCBackendComplexScalarParity`.

The preceding increment completes provider-backed `eigh` and `svd`. A
configured provider source links with the executable, is included in cached
build identity, and supplies the versioned f64 decomposition routines.

Focused verification after that change:

```sh
go test ./internal/osiris -run 'TestCBackendComplexScalarParity$' -count=1
git diff --check
```

The focused parity test and the complete `go test ./...` suite passed after
this increment.

## Recommended next work

1. Integrate a production general ABI-v2 numerical provider and add broader
   residual-based eig coverage rather than exact vector equality tests.
2. Continue the in-progress owned rank-aware native complex tensor work with
   reductions and remaining matrix kernels.
3. Extend static module propagation through control-flow block parameters and
   proven module aliases, or emit a deliberate diagnostic where that proof is
   unavailable. CFG parameter propagation is now conservative and proves only
   same-path incoming edges; the remaining case is static module metadata for
   lexical captures passed into nested native functions.
4. Expand `std/math` lowering, starting with `pow`/`expt` only after matching
   the interpreter's exact integer-power and overflow semantics. Unsigned
   overflow promotes to `bigint`, so do not map it blindly to C `pow`.
5. Add static lowering for other standard modules only with an explicit native
   ABI. Dynamic imports, escaping module values, and host/FFI calls remain
   unsupported and should report compile-time diagnostics.
6. Continue `tasks/003-native-loop-optimization.md`: general LICM, provably
   safe bounds-check elimination, profitable unrolling, and benchmark gates.
7. Continue `tasks/004-native-numeric-and-tensor-lowering.md`: unboxed narrow
   integer specialization, complete scalar coverage, higher-rank tensors,
   broadcasting, alias-aware mutation, and kernel benchmarks.
8. Continue `tasks/005-native-tooling-validation.md`: optimization reports,
   benchmark JSON, and CI commands.

## Important implementation notes

- `OValue.fbits` presently carries float width and tagged integer width. Keep
  the width on every helper result; `oabs` was recently fixed for this exact
  reason.
- `emitCFunctionBody` builds `bindings`, `closureArgs`, `builtins`, and
  `modules` in IR order. The static-module resolver depends on that order.
- Direct closure calls use hidden C parameters. Escaped/indirect closures use
  `O_CLOSURE` and `ocall`; do not remove either path.
- `nativeScalarFallbackReason` may still call module resolution “dynamic” in a
  generated comment even when a static member later lowers successfully. This
  is cosmetic only, but it can be refined when the scalar specializer learns
  static modules.
- Preserve existing unrelated worktree changes, especially
  `internal/osiris/.osiris.lock` and the tracked `osiris` binary. Do not edit
  the lock/cache artifact manually.

## Relevant files

- `internal/osiris/cbackend_native.go` — C runtime ABI and IR emission.
- `internal/osiris/cbackend.go` — native compiler/cache integration.
- `internal/osiris/runtime_test.go` — native parity and regression tests.
- `docs/design/NATIVE-COMPILER.md` and `docs/design/NATIVE-ABI.md` — contract.
- `tasks/001-c-backend-real-lowering.md` through
  `tasks/005-native-tooling-validation.md` — primary native backlog.
