# Native lowering handoff

Last updated: 2026-07-30.

## Hosted filesystem I/O boundary — 2026-07-30

- Task 102 adds 17 bounded file, directory, metadata, path-mutation, and
  seekable-stream built-ins shared by the interpreter and VM. Ownership,
  cancellation, memory ceilings, symlinks, portability, and structured errors
  are fixed in `docs/design/FILESYSTEM-IO.md`.
- Native C does not claim filesystem ownership/error parity. IR retains the
  builtin source span and `CompileC` rejects each new filesystem name there;
  there is no compile-time evaluator fallback.
- Focused normal/race tests, full normal/race/vet suites, Linux and Windows
  build-only checks, 32 sandboxed I/O reference examples, exact reference
  indexing, and the production Vite build pass with Go 1.26.1 and Node 24.

## Expanded mathematical and bit surface — 2026-07-30

- Task 100 adds 35 shared global/`std/math` operations covering real
  arithmetic, floating decomposition and classification, angle conversion,
  exact integer helpers, fixed-width unsigned bit inspection, and complex
  phase.
- `exp`, `log`, `sqrt`, `pow`/`expt`, `sin`, `cos`, `tan`, `sinh`, `cosh`,
  and `tanh` now accept their mathematically valid complex forms through one
  interpreter/VM implementation. Principal branches and signed-zero sides of
  the negative-real cut are characterized in
  `internal/osiris/math_ext_test.go`.
- No new operation is claimed by native C. New callables reject as unsupported
  at compile time, and complex transcendental inputs are tracked through SSA
  copies, CFG block arguments, closures, and direct function parameters so
  they cannot fall into a real-only tagged C helper. Existing parity-proven
  real scalar lowerings remain intact.
- `newdocs/` has an individual detailed listing for every addition, an exact
  181-name `std/math` export index, checked examples, search coverage, and an
  updated partial-native support statement. Normal, race, vet, reference,
  example, and production-build validations pass with Go 1.26.1 and Node 24.

## Unicode-aware `std/text` expansion — 2026-07-29

- Task 103 adds 24 qualified comparison, predicate, search, cut, tokenization,
  trim, and bounded replacement functions while preserving the original ten
  exports.
- Search results use `option<u64>` and Unicode code-point indexes. Empty
  needles, Unicode code-point sets, simple case folding, split limits, and
  absent cuts have explicit contracts shared by the interpreter and VM.
- Native C rejects `std/text` imports at the import source span until the
  backend has a parity-safe UTF-8 representation and ownership contract. The
  compiler never evaluates text calls to synthesize output.
- `internal/osiris/text_test.go` holds table-driven parity/error/inference/IR
  evidence and bounded cut/split fuzz properties. The canonical page documents
  all 34 exports, and the Technical Reference export index is checked exactly
  against the module table.

## Global constants and measurement units — 2026-07-29

- Task 104 expands the immutable named catalog to 159 canonical concepts and
  375 public spellings, including exact metric/customary/astronomical units,
  dimensionless angle scales, binary64 mathematical constants, and pinned
  2022 CODATA physical values.
- Exact quantity definitions are parsed directly into `big.Rat`; aliases share
  one definition and retain identical inference and codec behavior. The
  detailed and technical references are exact-set audited against the ordered
  implementation catalog.
- Native C now lowers default-number globals as compile-time IR constants with
  executable parity. Dimensioned named quantities remain an explicit
  compile-time rejection until the C ABI can retain arbitrary rational
  magnitudes and dimension vectors.
- Focused evidence is in `internal/osiris/named_test.go`; full normal, race,
  vet, reference, checked-example, and production-docs validations pass.

## Native enum and match completion — 2026-07-28

- Task 018 replaced the former blanket enum
  rejection has been replaced by a tagged `O_ENUM` C ABI value carrying the
  nominal type name, case name, and owned payload list.
- IR treats declarations as compile-time metadata and lowers qualified unit
  cases and payload constructors into `enum` instructions. It does not call
  the interpreter during compilation. Generated C implements truthiness,
  recursive nominal/payload equality, and rendering.
- `match` lowers unit enum arms plus a final `default`, and payload enum arms
  with recursive enum/list, literal, relational, type, conjunction, negation,
  grouped, and guarded patterns into ordinary C CFG branches. The tagged C
  helpers are `oenumis`/`oenumpayload`, `olistis`/`olistitem`, and
  `omatchtype`; repeated pattern bindings emit strict `oequal` checks. Focused
  executable parity evidence is
  `TestCBackendEnumValueParity`, `TestCBackendUnitEnumMatchParity`, and
  `TestCBackendPayloadEnumMatchBindingParity`. Guarded payload arms now bind
  before evaluating their guard and resume the next arm when it is false;
  `TestCBackendGuardedEnumMatchParity` covers this branch behavior.
- `TestCBackendNestedListEnumMatchParity`,
  `TestCBackendRepeatedPatternBindingParity`, and
  `TestCBackendRelationalTypeAndLogicalPatternParity` cover recursive list
  extraction, duplicate-binding equality, and the non-enum C#-style forms.
- Backed `string` and fixed-width integer declarations now validate literal
  values, uniqueness, and no-payload rules, and survive parse-format-parse;
  they remain nominal enum values in every execution engine with no implicit
  primitive conversion. `TestBackedEnumDeclarationsRoundTripAndNativeParity`
  and `TestBackedEnumDeclarationDiagnostics` cover the boundary.
- Task 018 is complete. The final C-ABI audit added recursive rendering for
  enum payloads, closure-identity equality inside enum payload equality, and
  executable parity for nested enum/function payloads and function patterns.
  JSON serialization retains the explicit nominal `{enum:{type,case,payload}}`
  envelope; no implicit backing-value conversion or evaluator fallback exists.

## Documentation and network queue update — 2026-07-25

- `newdocs/` is the canonical public React language reference, including its
  API data, examples, and support matrix. `docs/` is frozen compatibility plus
  durable design material; do not reintroduce a static-to-React migration path.
- Tasks 011, 012, and 049 are complete. Task 049's baseline harness, portable
  configuration boundary, bounded transfer, and same-host default-versus-bulk
  comparison are recorded. The explicit bulk override was approximately 3.8%
  slower with identical allocation counts, so the low-latency defaults remain
  unchanged.
- `scripts/bench-tcp-loopback.sh` prints a compact median/range/throughput/
  allocation table and retains raw Go rows in `TCP_BENCH_OUT`. Use
  `TCP_BENCH_SHOW_RAW=1` only when stdout copies of those rows are needed.
  The completed comparison command was:

  ```sh
  TCP_RUNS=15 TCP_BENCH_TIME=1s \
    TCP_BENCH_OUT=artifacts/tcp-loopback-bulk.txt \
    ./scripts/bench-tcp-loopback.sh
  ```

  The default median was 237,576 ns/op and the explicit bulk median was
  246,530 ns/op (approximately 3.8% slower), so `DefaultSocketConfig` remains
  unchanged.
- `TestSocketConfigBulkBatchAppliesToBothEndpoints` locks the benchmark's
  explicit policy to both endpoints' observable 64 KiB writer buffers; its
  focused normal and race checks pass (loopback remains unavailable here).

## Current compiler path

Osiris now uses real IR-to-C lowering in `internal/osiris/cbackend_native.go`.
`CompileC` builds IR and emits executable C; it does not evaluate the program
through `Session.ExecProgram` to create an output snapshot. The system C
compiler remains the first native code-generation backend.

The native C pipeline performs a narrow proof-gated LICM pass before emission:
in canonical while loops, numeric constant and invariant arithmetic/comparison
IR moves from the body to a unique preheader only when it cannot depend on
loop-carried state or introduce an error. Calls, allocation, division,
indexing, mutation, and aliases are deliberately not hoisted. The next task-003
slice completed range-proven bounds-check elimination followed by bounded,
profitability-gated unrolling.

The first bounds-elimination proof is now implemented for canonical `for`
loops over a directly constructed list. The loop header establishes the index
range before body entry, so only that exact `at(iter, index)` becomes
`olistatunchecked`; all dynamic/direct indexing remains checked. Evidence is
`TestCBackendProofGatedForListBoundsEliminationParity` and
`TestCBackendBoundsEliminationLeavesDirectAtChecked`.

Implemented and covered by native executable parity tests:

- tagged C runtime values, strings, lists, one-dimensional f64 tensors, and
  direct tensor kernels;
- scalar branches, loops, direct calls, direct lexical captures, closure
  values, closure capture ownership, and indirect closure calls;
- unboxed scalar main-CFG lowering for the proven u8/u16/u32/u64/i8/i16/i32/i64/f32/f64 subset, including defined u64/i64 remainder;
- scalar CFG lowering computes liveness across block parameters, so dead
  undef-initialized loop carries do not force tagged fallback while a live
  undef remains unsupported;
- fixed-width tagged literal/cast preservation for `u8/u16/u32/u64`, `byte`,
  and `i8/i16/i32/i64`, including same-family tagged arithmetic and bitwise
  result-width preservation;
- direct checked `i64` scalar `+`, `-`, `*`, and remainder lowering, including
  defined `INT64_MIN % -1` behavior and stable overflow/zero-divisor diagnostics;
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

The most recent scalar increment adds direct lowering for proven static
`std/math.sqrt`, `std/math.cbrt`, `std/math.exp`, `std/math.log`, `std/math.log10`,
`std/math.log2`, `std/math.expm1`, `std/math.log1p`, and
`std/math.erf`/`std/math.erfc` calls on f32, f64, and default float scalar
CFGs, plus direct floating `std/math.abs` and `std/math.round` with explicit
signed-half parity. The member identity remains
compile-time metadata, output remains unboxed, negative sqrt inputs retain the
`sqrt domain error` contract, cbrt retains its valid-negative-input contract,
expm1/log1p/erf/erfc retain their ordinary numeric-unary contract, and f32
explicitly rounds after C `sqrt`/`cbrt`/`exp`/`expm1`/`log1p`/`erf`/`erfc`/`round`.
Direct `exp` checks the double-precision C result for finiteness before f32
narrowing, so its `exp overflow` boundary exactly follows the interpreter.
Focused evidence is
`TestCBackendUnboxedFloatSqrtParity` and
`TestCBackendUnboxedFloatSqrtDomainIsReported`, plus
`TestCBackendUnboxedFloatCbrtParity` and
`TestCBackendUnboxedFloatExpm1Parity`, plus
`TestCBackendUnboxedFloatLog1pParity` and
`TestCBackendUnboxedFloatErfParity`, plus
`TestCBackendUnboxedFloatErfcParity` and
`TestCBackendUnboxedFloatRoundParity`, plus
`TestCBackendUnboxedFloatAbsParity`, plus
`TestCBackendUnboxedFloatExpParity` and
`TestCBackendUnboxedFloatExpOverflowIsReported`, plus
`TestCBackendUnboxedFloatLogarithmParity` and
`TestCBackendUnboxedFloatLogarithmDomainIsReported`.

One-argument floating `std/math.log`, `std/math.log10`, and `std/math.log2`
now lower directly with their individual non-positive domain diagnostics and
explicit f32 narrowing. Two-argument `log(value, base)` now lowers directly
when both static operands are f32: it validates `value > 0`, `base > 0`, and
`base != 1`, computes `log(value) / log(base)` in C double precision, then
narrows once to f32 just as the interpreter does. F64 and unproven/mixed base
logs instead fail native compilation. A characterization test found that Go
and the host C libm differ in the final f64 result bit (`4.000000000000001f64`
versus `4f64` for `log(81, 3)`), so accepting that lowering would violate
native/interpreter parity. Evidence includes
`TestCBackendFloatBaseLogF64IsRejected`.

Static f32 `std/math.gamma` and `std/math.lgamma` now lower directly through
`tgamma`/`lgamma` followed by one f32 narrowing. F64, mixed, and unproven
calls reject at native compile time rather than relying on tagged C libm.
`TestCBackendUnboxedFloatGammaParity` characterizes f32 executable parity;
`TestCBackendFloatGammaF64IsRejected` locks the f64 safety boundary.

Static `std/math.pow` and `std/math.expt` now lower only when both operands
are f32 literals and `exactPowValue` proves that their exact-integer branch is
inapplicable. The direct C path calls `pow` in double precision and narrows
once to f32. Integer-like f32 values can still produce an exact integer or
bigint in Osiris, so exact, f64, mixed-width, dynamic, and unproven calls
reject instead of changing the result kind. Evidence:
`TestCBackendUnboxedFloatPowParity`, `TestCBackendFloatPowF64IsRejected`, and
`TestCBackendExactFloatPowIsRejected`.

Proven real floating `std/math.real`, `std/math.imag`, and
`std/math.magnitude` now lower without tagged values: identity, same-width
zero, and width-aware absolute value. `std/math.conjugate` also lowers as an
identity for fixed-width signed/unsigned scalar integers. Complex and aggregate
forms remain on their owned native paths. Evidence:
`TestCBackendUnboxedFloatComplexProjectionParity`.

Proven unsigned scalar `std/math.abs` and `std/math.magnitude` now lower as
identity assignments while retaining their exact width. Proven `i8`/`i16`/`i32`
values lower through unsigned-bit negation and `osignedbits`, preserving the
language's minimum-width wrap. I64 remains tagged pending its separate unary
negation parity boundary. Evidence:
`TestCBackendUnboxedUnsignedMagnitudeParity` and
`TestCBackendUnboxedSignedMagnitudeParity`.

Proven scalar floating `std/math.f32` and `std/math.f64` casts now lower as a
single direct C conversion. Other input families retain their tagged,
representation-aware paths. Evidence: `TestCBackendUnboxedFloatCastParity`.

Static `std/math.clamp` also now lowers directly for same-kind float triples
(default/f32/f64); mixed widths remain tagged. Its evidence is
`TestCBackendUnboxedFloatClampParity`.

Same-kind floating variadic `std/math.min`/`max` now lower as direct ordered
folds, preserving interpreter comparison order; mixed widths remain tagged.
Their evidence is `TestCBackendUnboxedFloatExtremaParity`.

Static float `std/math.floor`/`ceil` now lower directly with explicit f32
narrowing; their positive/negative evidence is
`TestCBackendUnboxedFloatFloorCeilParity`.

Same-kind floating variadic `std/math.hypot` now lowers as Osiris's own f64
sum-of-squares-plus-sqrt contract (not platform `hypot`) with final f32
narrowing; mixed widths remain tagged. Its evidence is
`TestCBackendUnboxedFloatHypotParity`.

Static float `std/math.sin`/`cos`/`tan` now lower directly with f32 narrowing;
their evidence is `TestCBackendUnboxedFloatTrigonometricParity`.

Static f32 `std/math.asin`/`acos`/`atan` now lower directly. `asin` and `acos`
check their double result for finiteness before narrowing and retain the stable
domain error; `atan` is non-rejecting. f64 inverse trigonometry deliberately
fails native compilation: the Go and C implementations differ by one final bit
for characterized values, so either direct or tagged C libm would break
parity. Evidence is
`TestCBackendUnboxedFloatInverseTrigonometricParity` and
`TestCBackendUnboxedFloatInverseTrigonometricDomainIsReported`, plus
`TestCBackendFloatInverseTrigonometricF64IsRejected`.

Static f32 `std/math.sinh`/`cosh`/`tanh`/`asinh` now lower directly, and f32
`acosh`/`atanh` lower with their stable finite-result domain checks. The f64
family fails native compilation until exact Go/libm parity is proven. Evidence
is `TestCBackendUnboxedFloatHyperbolicParity` and
`TestCBackendUnboxedFloatHyperbolicDomainIsReported`, plus
`TestCBackendFloatLibmF64IsRejected`.

Static floating `std/math.sign`/`step`/`heaviside`/`relu` now lower directly
for f32 and f64 through exact comparisons and constants, not platform libm.
Their evidence is `TestCBackendUnboxedFloatComparisonMathParity`.
Proven integer inputs now lower too and produce the interpreter's default-float
result; `TestCBackendUnboxedIntegerComparisonMathParity` covers that rule.

Static f32 `std/math.sigmoid` now lowers through the interpreter's stable
two-branch exp formulation. f64 remains unsupported pending parity evidence.
Its evidence is `TestCBackendUnboxedFloatSigmoidParity`.

Static f32 `std/math.softplus` now mirrors the interpreter's strict `x > 20`
cutoff and its `log1p(exp(x))` path below it. f64 remains unsupported pending
parity evidence. Its evidence is `TestCBackendUnboxedFloatSoftplusParity`.

`TestCBackendUnboxedFloatMathLoopParity` additionally proves that a static
`std/math.sqrt` call survives a loop-carried f64 scalar CFG as direct C, not a
tagged fallback.

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
6. Task 003 is complete; defer general LICM, fusion, and tiling to a newly
   scoped optimization task rather than extending its bounded proof slice.
7. Continue `tasks/004-native-numeric-and-tensor-lowering.md`: mixed-width
   integer operations, complete scalar coverage, higher-rank tensors,
   broadcasting, alias-aware mutation, and kernel benchmarks.
8. Continue `tasks/005-native-tooling-validation.md`: optimization reports,
   benchmark JSON, and CI commands.

## Important implementation notes

- `OValue.fbits` presently carries float width and tagged integer width. Keep
  the width on every helper result; `oabs` was recently fixed for this exact
  reason.
- General signed integer division remains tagged by design. Exact division
  returns a width-styled integer, while a remainder returns a decimal, so an
  unboxed C local cannot represent its dynamic result kind. Same-width signed
  literal operands with a proven zero remainder now lower directly (including
  a `MIN / -1` wrap guard); do not lower other cases as truncating C `/`
  without a divisibility proof or an explicit decimal bridge.
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
