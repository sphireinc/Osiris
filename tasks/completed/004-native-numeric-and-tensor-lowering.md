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
widths and same-family tagged arithmetic/bitwise result widths. Proven
scalar-only main CFGs additionally unbox same-width unsigned `u8`, `u16`, and
`u32` arithmetic/remainder/bitwise/shift expressions, `u64` and `i64` remainder, and signed `i8`, `i16`, and `i32`
addition, subtraction, multiplication, unary negation, and bitwise `&`, `|`,
`^`, and shifts. Narrow unsigned arithmetic, bitwise operations, and shifts
also lower across `u8`, `u16`, and `u32` widths, producing the wider operand
width (or the left width for shifts). The equivalent proven signed operations
lower across `i8`, `i16`, and `i32` widths. Signed/unsigned mixing, signed division,
decimal/bigint paths, and complete scalar coverage
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

## Current narrowing constraint (2026-07-26)

The unboxed scalar path now has width-aware `u8`, `u16`, and `u32` kinds for
pure scalar main CFGs. Same-width unsigned `+`, `-`, `*`, bitwise operations,
and shifts retain their declared result width. Narrow addition, subtraction,
and multiplication explicitly widen before calculating, then narrow back to
the language width; this avoids C's signed-integer-promotion undefined
behavior while preserving Osiris wrapping semantics. Shift counts are checked
against the actual width. `TestCBackendUnboxedNarrowUnsignedIntegerParity`
proves generated-C locals, executable output, wrapping, and shift behavior.

Signed narrow `+`, `-`, `*`, unary negation, `%`, `&`, `|`, `^`, and shifts now normalize through a
portable `int64_t` intermediate and the `osignedbits` native ABI helper before
being assigned to a narrow C local. This avoids signed C overflow and
implementation-defined out-of-range narrowing while preserving the language's
wrapping result. Signed shifts validate a non-negative count below the source
width and use unsigned-bit shifting plus explicit sign extension for right
shifts. Both signed and unsigned remainder check a zero divisor. Mixed signed/unsigned families and signed division,
and promotion-sensitive paths intentionally remain on the tagged ABI; they
must not be admitted by merely casting C locals.

General signed integer `/` is deliberately still outside the unboxed scalar subset.
`exactSignedDivisionValue` returns a width-preserving integer only when the
quotient has no remainder; otherwise it returns a decimal value. That dynamic
result-kind change cannot be represented by one proven C scalar local, and
blind C truncating division would silently lose the decimal branch. A future
lowering must either prove divisibility and result style at compile time or
introduce an explicit tagged/decimal bridge with parity and ownership tests.
The zero-divisor and `INT64_MIN / -1` boundaries must be handled before any
host division is emitted. The current narrow exception proves two same-width
signed literal operands have a zero remainder and emits the C division only in
that case; `MIN / -1` is guarded to preserve the interpreter's width wrap.
`TestCBackendUnboxedExactSignedLiteralDivisionParity` covers the admitted path,
while `TestCBackendNonExactSignedLiteralDivisionRetainsTaggedDecimalPath`
locks the decimal branch to the tagged ABI.

`benchmarks/native-narrow-integers.osr` is the reproducible parity-checked
workload for the unboxed `u32` scalar CFG path. Run it through the established
harness with `WORKLOAD=benchmarks/native-narrow-integers.osr ./bench.sh`; its
loop bound is intentionally compatible with `BENCH_ITERS` substitution.
The smoke command `RUNS=1 WARMUP=0 BENCH_ITERS=100` produced matching
interpreter/VM/native `4950u32` output. It verifies the harness and native
path only; it is not a regression baseline. Use the documented 15-run command
on a matching host for a performance claim.

`i64` remainder now also lowers directly for proven scalar CFGs. Generated C
checks a zero divisor and handles `INT64_MIN % -1` as zero before using `%`,
avoiding the C undefined operation while matching the big-integer interpreter
result. `TestCBackendUnboxedIntRemainderParity` covers ordinary negative
remainders and that boundary; `TestCBackendUnboxedIntRemainderZeroIsReported`
covers its diagnostic path.

Proven `i64` scalar CFGs now also lower checked `+` and `*` directly through
compiler overflow intrinsics. `TestCBackendUnboxedIntArithmeticParity` proves
normal executable parity; `TestCBackendUnboxedIntOverflowIsReported` proves
that the first overflowing operation terminates with the stable native
diagnostic instead of invoking C signed-overflow behavior.

Proven `i64` scalar `&`, `|`, and `^` now lower directly as fixed-width C
integer operations. Shifts remain on their separate checked path because their
range and signed-right-shift contract is stricter. `TestCBackendUnboxedIntBitwiseParity`
proves generated-C and executable parity for negative operands and masks.

Proven `i64` shifts now lower directly with explicit signed-count validation.
Left shift operates on unsigned bits; right shift explicitly sign-extends from
unsigned bits; both reconstruct the fixed-width signed result through the
native ABI helper. Counts below zero or at least 64 report `shift count exceeds
signed width` rather than reaching C undefined behavior.
`TestCBackendUnboxedIntShiftParity` and
`TestCBackendUnboxedIntShiftRangeIsReported` cover both paths.

Proven `f32`/`f64` scalar CFGs now lower `/` directly with an explicit
zero-divisor diagnostic. The unboxed emitter routes float output through the
existing native `oprintfloat` shortest-roundtrip routine instead of raw C
`printf`, which fixes the visible `-22.0f64 / 7.0f64` final-digit mismatch.
`TestCBackendUnboxedFloatDivisionParity` covers f32 and recurring f64 output;
`TestCBackendUnboxedFloatDivisionZeroIsReported` covers the error boundary.
The same work fixed integral f32 literals from invalid C `2f` to `2.0f`,
covered by the float-width parity test.

Proven `f32`/`f64` scalar `%` now lowers directly through C `fmod` with the
same explicit zero-divisor diagnostic. The f32 path deliberately computes in
f64 before narrowing, matching the interpreter's `math.Mod`-then-width-round
rule rather than using host `fmodf`. `TestCBackendUnboxedFloatRemainderParity`
and `TestCBackendUnboxedFloatRemainderZeroIsReported` provide executable
parity and failure-path evidence.

`benchmarks/native-float-arithmetic.osr` is the reproducible parity-checked
float scalar workload for loop-carried f64 comparison, addition, division,
and remainder. Its loop bound works with `BENCH_ITERS` substitution in the
standard harness. The smoke command `RUNS=1 WARMUP=0 BENCH_ITERS=100
WORKLOAD=benchmarks/native-float-arithmetic.osr ./bench.sh` produced the
matching checksum `83.33333333333334f64` (interpreter 22.136 ms, VM 22.353 ms,
native 11.682 ms; native compile 547.177 ms). This single sample verifies the
workload and direct path only; it is not a performance baseline.

Scalar CFG liveness now ignores dead `undef` loop-carry plumbing while still
rejecting a live undefined value. This permits source loops with an unused
intermediate local to retain direct native lowering instead of falling back to
the tagged ABI. `TestCBackendUnboxedScalarLoopIgnoresDeadUndefCarry` proves
generated-C and executable parity. The named-intermediate float benchmark
continues to pass the same parity smoke after this change.

The tagged native fallback now also implements floating `%` through `fmod`,
preserving f32/f64 result width and reporting zero divisors explicitly rather
than misrouting floats through integer bitwise handling. This keeps dynamic or
aggregate-derived floating operands semantically aligned with the direct
scalar path. `TestCBackendTaggedFloatRemainderParity` and
`TestCBackendTaggedFloatRemainderZeroIsReported` cover that boundary.

Short-circuit `&&` and `||` already lower as direct typed CFG branches rather
than eager generic operations. `TestCBackendUnboxedLogicalBranchParity` now
locks in generated-C shape and interpreter/native output parity for both truth
paths of each operator.

## Scalar std/math checkpoint (2026-07-26)

The unboxed scalar CFG path now recognizes proven static `std/math.sqrt`,
`std/math.cbrt`, `std/math.exp`, `std/math.expm1`, and `std/math.log1p` member calls for f32,
f64, and default floating inputs, plus `std/math.erf` for the same scalar
subset, its separately characterized `std/math.erfc` counterpart, and
`std/math.round` with half-value parity coverage. Floating `std/math.abs` now
also lowers directly through `fabs`. The
member handle stays
compile-time metadata; it is not materialized as an `OValue` or an indirect
call. Generated C checks the same negative-input domain boundary before
calling `sqrt`, and the f32 path computes in double then explicitly narrows,
matching the interpreter's `math.Sqrt`-then-width-rounding contract.

`cbrt` has no negative-input error boundary; its direct f32 path likewise
computes in double and explicitly narrows, matching the interpreter's
`math.Cbrt`-then-width-rounding contract.

`exp` checks C's double-precision result with `isfinite` before any f32
narrowing, then emits the interpreter's stable `exp overflow` error for NaN
or infinity. A finite double result that only overflows on f32 narrowing is
therefore still accepted, exactly as the interpreter's
`math.Exp`-then-width-rounding implementation does.

The one-argument forms of `log`, `log10`, and `log2` now lower directly for
proven floats. Each checks a non-positive input before its matching C
operation so native errors remain `log domain error`, `log10 domain error`,
and `log2 domain error`, respectively; f32 results explicitly narrow after
double computation. Two-argument `log(value, base)` now directly lowers only
when both static operands are f32. It performs the interpreter's `value > 0`,
`base > 0`, and `base != 1` validation, computes `log(value) / log(base)` in
double precision, and narrows once to f32. F64, mixed-width, and unproven
base-log calls reject at native compile time rather than taking the tagged
`ologbase` route: characterization showed Go evaluates `log(81f64, 3f64)` as
`4.000000000000001f64` while the host C libm produced `4f64`. The focused
evidence is `TestCBackendUnboxedFloatLogarithmParity`,
`TestCBackendUnboxedFloatLogarithmDomainIsReported`, and
`TestCBackendFloatBaseLogF64IsRejected`.

Static `std/math.pow` and `std/math.expt` now lower directly only for two f32
literals whose exact-integer branch is conclusively inapplicable. The runtime
attempts exact exponentiation before `math.Pow`, even for f32 spellings such
as `pow(2.0f32, 3.0f32)`, and that branch can return an integer or bigint;
those calls must not enter an unboxed f32 C local. The admitted path calls C
`pow` in double precision and explicitly narrows once to f32. Exact, f64,
mixed-width, dynamic, and unproven power calls reject at native compile time.
`TestCBackendUnboxedFloatPowParity`, `TestCBackendFloatPowF64IsRejected`, and
`TestCBackendExactFloatPowIsRejected` cover the direct and rejecting paths.

Static `std/math.real`, `std/math.imag`, and `std/math.magnitude` now lower
directly for proven real floating scalars. `std/math.conjugate` additionally
lowers for fixed-width signed and unsigned scalar integers, for which it is an
identity. `real`/`conjugate` are identity assignments, `imag` produces
same-width zero, and `magnitude` uses width-aware absolute value. Complex and
aggregate forms remain on their dedicated owned native paths. The executable
parity evidence is `TestCBackendUnboxedFloatComplexProjectionParity`.

Proven unsigned scalar `std/math.abs` and `std/math.magnitude` now lower as
direct identity assignments, preserving all supported `u8`/`u16`/`u32`/`u64`
width metadata without a tagged value. Proven `i8`/`i16`/`i32` inputs now also
lower through unsigned-bit negation plus `osignedbits`, so each minimum-width
value retains the interpreter's wrapping result instead of reaching signed C
overflow. I64 remains tagged until its unary-negation parity boundary is
resolved separately. The executable evidence is
`TestCBackendUnboxedUnsignedMagnitudeParity` and
`TestCBackendUnboxedSignedMagnitudeParity`.

Static `std/math.f32` and `std/math.f64` calls now lower directly when their
single input is a proven scalar float. The generated C performs exactly one
`float` or `double` conversion, preserving the runtime's explicit-width
boundary without materializing an `OValue`. Other numeric families remain on
their representation-aware native paths. The executable parity evidence is
`TestCBackendUnboxedFloatCastParity`.

`expm1` likewise has no interpreter-side domain rejection. Its direct path
uses C `expm1` in double precision and explicitly narrows f32, matching the
numeric-unary interpreter contract without adding an overflow policy that the
language does not have.

`log1p` follows that same numeric-unary contract; values at or below its
mathematical boundary are not converted into a language error by the
interpreter, so the direct path deliberately calls C `log1p` without adding a
domain guard and explicitly narrows f32 afterward.

`erf` is also a non-rejecting numeric-unary operation. The direct path calls C
`erf` in double precision and explicitly narrows f32. `erfc` now has separate
f32/f64 parity characterization and follows the same direct numeric-unary
emission; neither operation’s support was inferred from the other.

Static f32 `std/math.gamma` and `std/math.lgamma` now lower directly through
C `tgamma` and `lgamma`, respectively, followed by the language's one explicit
f32 narrowing. Their numeric-unary contracts introduce no separate domain
error. The native compiler rejects f64, mixed, and unproven calls instead of
routing them through the tagged C helpers, because Go and platform libm have
no bit-exact f64 gamma/log-gamma contract. The focused evidence is
`TestCBackendUnboxedFloatGammaParity` and
`TestCBackendFloatGammaF64IsRejected`.

`round` calls C `round`, whose halfway-away-from-zero behavior matches the Go
interpreter. Both positive and negative half values are covered before the f32
result is explicitly narrowed.

For the proven floating subset, `abs` calls C `fabs` and explicitly narrows
f32. Integer and complex absolute-value semantics remain on their existing
paths because they have distinct width and result-kind contracts.

`TestCBackendUnboxedFloatSqrtParity` proves generated-C shape and executable
interpreter/native parity for f32 and f64 values.
`TestCBackendUnboxedFloatSqrtDomainIsReported` proves the stable `sqrt domain
error` path. `TestCBackendUnboxedFloatCbrtParity` provides the corresponding
positive and negative cbrt parity evidence.
`TestCBackendUnboxedFloatExpm1Parity` provides f32/f64 executable parity for
the new expm1 path. `TestCBackendUnboxedFloatLog1pParity` supplies the same
evidence for log1p. `TestCBackendUnboxedFloatErfParity` supplies f32/f64
evidence for erf, and `TestCBackendUnboxedFloatErfcParity` independently does
so for erfc. `TestCBackendUnboxedFloatRoundParity` covers both signs at the
rounding boundary. `TestCBackendUnboxedFloatAbsParity` proves f32/f64 floating
absolute-value parity. `TestCBackendUnboxedFloatExpParity` proves f32/f64
direct execution parity for exp, while
`TestCBackendUnboxedFloatExpOverflowIsReported` proves its overflow contract.
`TestCBackendUnboxedFloatLogarithmParity` and
`TestCBackendUnboxedFloatLogarithmDomainIsReported` provide executable and
domain-error evidence for the direct one-argument logarithms. Broader math
builtins remain tagged unless their result kind,
rounding, overflow, and error behavior can be proven by an equally narrow
direct contract.

Static `std/math.clamp` now lowers directly when all three operands have the
same proven floating kind (default float, f32, or f64). It emits the
interpreter-equivalent ordered comparisons and returns that same kind; mixed
width operands deliberately retain the tagged ABI because their promoted
result type needs separate proof. `TestCBackendUnboxedFloatClampParity` covers
below-range, above-range, and in-range f32/f64 values and confirms the
generated C has no scalar fallback.

Static variadic `std/math.min` and `std/math.max` now lower as direct ordered
folds when all arguments have one proven floating kind. The generated C starts
with the first argument and uses the same `<`/`>` comparison order as the
interpreter, including its behavior when comparisons with NaN are false.
Mixed-width extrema remain tagged pending a separate promotion proof.
`TestCBackendUnboxedFloatExtremaParity` covers three-argument f32/f64 min and
max execution with no scalar fallback.

Static `std/math.floor` and `std/math.ceil` now lower directly for proven
floating scalar inputs. They call the matching C operation in double precision
and explicitly narrow f32 afterward. `TestCBackendUnboxedFloatFloorCeilParity`
covers positive and negative directional behavior for both f32 and f64.

Static variadic `std/math.hypot` now lowers for same-kind float arguments
(default/f32/f64). The native emitter deliberately follows Osiris's defined
left-to-right f64 sum-of-squares and square-root calculation instead of using
the platform `hypot` function, which has different scaling behavior. It
explicitly narrows the final f32 result; mixed widths remain tagged.
`TestCBackendUnboxedFloatHypotParity` covers f32 and a three-argument f64 case
with no scalar fallback.

Static `std/math.sin`, `std/math.cos`, and `std/math.tan` now lower directly
for proven scalar floats. They use their C double-precision counterparts and
explicitly narrow f32 afterward, matching the interpreter's numeric-unary
contract. `TestCBackendUnboxedFloatTrigonometricParity` covers all three for
positive f32 and negative f64 inputs without scalar fallback.

Static f32 `std/math.asin`, `std/math.acos`, and `std/math.atan` now lower
directly. `asin` and `acos` hold their double result in a scoped temporary and
reject non-finite output as their stable domain error before f32 narrowing;
`atan` remains non-rejecting. The admitted subset is intentionally f32-only:
static f64 inverse-trigonometric calls are rejected at native compile time
because Go's routines and platform C libm differ by a final bit for
characterized values. Rejecting them is required to preserve
interpreter/native parity; they must not fall through to the tagged C helper.
`TestCBackendUnboxedFloatInverseTrigonometricParity`,
`TestCBackendUnboxedFloatInverseTrigonometricDomainIsReported`, and
`TestCBackendFloatInverseTrigonometricF64IsRejected` provide the direct,
error-path, and unsupported-f64 evidence.

Static f32 `std/math.sinh`, `std/math.cosh`, `std/math.tanh`, and
`std/math.asinh` now lower as non-rejecting double-libm computations followed
by explicit f32 narrowing. `std/math.acosh` and `std/math.atanh` use the same
scoped-double pattern as inverse trigonometry and reject non-finite output as
their stable domain error before narrowing. Static f64 hyperbolic calls are
also rejected at native compile time: C libm is not an exact cross-platform
contract for Go's f64 semantics. `TestCBackendUnboxedFloatHyperbolicParity`,
`TestCBackendUnboxedFloatHyperbolicDomainIsReported`, and
`TestCBackendFloatLibmF64IsRejected` prove the admitted f32 and rejected-f64
contracts.

Static floating `std/math.sign`, `std/math.step`, `std/math.heaviside`, and
`std/math.relu` now lower directly for proven f32 and f64 inputs. These are
comparison-only operations, not libm calls: the generated C mirrors the
interpreter's ordered comparisons and exact constants, including step's zero
case and heaviside's `0.5` midpoint. `TestCBackendUnboxedFloatComparisonMathParity`
proves generated-C shape and executable parity across both widths.
The same helpers now admit proven fixed-width signed and unsigned integer
inputs; their result is the language's default float, matching
`scalarUnaryMapped` rather than preserving the source integer width.
`TestCBackendUnboxedIntegerComparisonMathParity` covers this conversion and
the signed/unsigned comparison paths without fallback.

Static f32 `std/math.sigmoid` now lowers with the interpreter's stable branch:
for non-negative inputs it computes `1 / (1 + exp(-x))`; otherwise it computes
`exp(x) / (1 + exp(x))`. The operation explicitly remains f32-only until f64
Go/libm parity is characterized. `TestCBackendUnboxedFloatSigmoidParity`
covers negative, ordinary, and large-positive inputs without a tagged fallback.

Static f32 `std/math.softplus` now mirrors the interpreter's stable contract:
it returns the input only when `x > 20`; at `x == 20` and below it computes
`log1p(exp(x))` in double precision and narrows once. f64 remains unsupported
pending parity characterization. `TestCBackendUnboxedFloatSoftplusParity`
covers negative, ordinary, cutoff, and above-cutoff values without fallback.

`TestCBackendUnboxedFloatMathLoopParity` proves that proven static
`std/math` identity and direct `sqrt` lowering survive a loop-carried f64 CFG:
the generated `main` has no scalar `OValue` fallback locals and the loop body
contains the direct C `sqrt` call. Its executable output matches the
interpreter, so the scalar-math work is not limited to straight-line samples.

## Completion record — 2026-07-28

The original numeric/tensor umbrella has been completed by the focused kernel,
provider, and numeric-semantics tasks 030, 031, and 032. The current ABI has
direct rank-aware f64 and complex tensor kernels, owned shape/data handling,
broadcasting, indexing/slicing/copy-on-write replacement, matrix operations,
and provider-backed decomposition boundaries. Scalar lowering preserves proven
kind/width/overflow behavior and rejects unsupported bigint, decimal, mixed,
or libm-parity-unsafe cases rather than silently changing the language result.

The historic “remaining” notes above predate those successor tasks. They are
not a claim that every mathematical type has a direct C representation; the
documented contract intentionally rejects semantics that cannot yet retain
interpreter parity. Any new element family or numerical algorithm requires a
new scoped task with ownership, error, parity, and benchmark evidence.
