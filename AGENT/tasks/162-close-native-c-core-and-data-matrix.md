# Task 162: Close native C core and data matrix rows

**Priority:** P0
**Dependencies:** 148
**Status:** Ready

## Objective

Close only the native C matrix rows for core syntax/state, errors/results,
modules/patterns/tasks, numeric families, tensors, matrices, codecs, and
linear algebra. This task does not own native standard-library host I/O.

## Acceptance criteria

- Each owned matrix cell is backed by executable interpreter/VM/native
  differential fixtures, normal/race/sanitizer evidence, and exact docs.
- Unsupported shapes are source-spanned compile errors; no interpreter, VM, or
  shell fallback is allowed.
- The task updates only its owned native matrix cells after evidence is present.

## Current implementation evidence

The native implementation now has focused differential coverage for the
currently executable core/data slice in
`internal/osiris/native_core_library_parity_test.go`:

- `TestNativeCoreDataParityExpanded` covers payload-bearing Results,
  Option extraction, codec round-tripping, tensor construction, reductions,
  dot product, and shape reporting through the interpreter, VM, and generated C
  executable.
- `otensorsum`, `otensorproduct`, `otensordot`, and `otensornorm` preserve the
  numeric family and floating-point width required by their input values.
- Native Result rendering includes payloads, so generated output distinguishes
  `ok(7u64)` from an empty `ok`.
- Tuple construction and indexing are now a bounded native data slice:
  `irBuilder.evalExpr`, `emitCInstr`, `otuple`, `oprint_tuple_value`,
  `oprint_tuple_aware`, `atValue`, and `lenValue` preserve item order,
  direct tuple rendering, zero-based bounds checks, and tuple length. The same
  witness covers mixed-width items and nested tuple indexing through the
  interpreter, VM, and generated-C executable.
- The tuple witness passes the normal focused run, `go test -race -vet=off`,
  and the address-sanitized generated-C run using the installed LLVM clang.
  These are slice-level proofs, not full task closure evidence.
- `irBuilder.lowerStmt` now lowers `let (a, b) = tuple` through the existing
  `const`/`index`/`copy` IR sequence. `TestNativeCoreTupleDestructuringParity`
  proves flat and nested bindings through the interpreter, VM, and generated-C
  artifact (`oat` calls).
- Named product lowering now carries declaration-order field metadata through
  `IRProgram.Products`, `product`/`product_update` instructions, and a tagged
  `O_PRODUCT` runtime representation. `TestNativeCoreNamedProductParity`
  proves struct construction, dot-member reads, copy-on-write `with` updates,
  standalone rendering, string-field indexing, missing-field handling, and
  generated executable parity. `atValue` and `oat_product_or_value` share the
  string field-name contract; numeric positional product indexing remains
  explicitly rejected. Invalid field values remain source-spanned diagnostics
  in `TestNativeCoreNamedProductDiagnostic`, while
  `TestNativeCoreNamedProductRenderingAndIndexContract` checks the generated
  product renderer, temporary lookup-key release, and capability boundary.
- Transparent aliases and single-field newtypes now have a bounded native ABI:
  `IRProgram.Aliases` records alias targets, alias constructors lower to a
  value copy, and newtype constructors lower to `newtype`/`onewtype` over the
  existing tagged product representation. `TestNativeCoreAliasesAndNewtypesParity`
  proves interpreter/VM/generated-C/native executable parity for construction,
  rendering, `.value`, and `["value"]` access. The ownership witness checks
  the no-capability boundary, tracked product construction, and temporary key
  release; `TestNativeCoreNewtypeDiagnostic` checks source-spanned rejection
  of an invalid target value.
- Evidence for this slice is recorded by the normal differential run
  `go test ./internal/osiris -run '^TestNativeCore(AliasesAndNewtypes|NewtypeDiagnostic)$'`,
  the IR-contract run
  `go test ./internal/osiris -run '^TestNativeCoreAliasesAndNewtypesIRContract$'`,
  the focused race run with `-race -vet=off`, and the generated-C AddressSanitizer
  run with `CC=$(brew --prefix llvm)/bin/clang OSIRIS_C_SANITIZERS=address`.
- The tuple-destructuring slice was checked with the focused normal run and
  `git diff --check`; no backend matrix status was changed.

- Scalar `complex<f32>` now closes one concrete numeric family boundary. The
  existing `complexValueStyled`, `complexComponentBits`, `complexResultBits`,
  and `formatComplexStyled` contracts are retained through `cLiteral`,
  `emitCInstr`, `ocomplexbits`, `ocomplexcall`, `obin`, `oreal`, and `oimag`.
  Width-tagged complex literals lower to the inline tagged `OComplex` ABI;
  they do not allocate and do not define a host capability. The dedicated
  `internal/osiris/native_core_numeric_test.go` witnesses cover literal
  parsing/lowering, interpreter/VM/generated-C/native executable parity,
  rendering, binary arithmetic, projections, IR shape, ownership/capability
  boundaries, and a source-spanned diagnostic for unsupported complex scalar
  math. Normal, race, and LLVM AddressSanitizer evidence is required for this
  slice before broader numeric status changes.

- Scalar dual values now use the complete inline Native C ABI. `emitCInstr`
  lowers `dual`, `primal`, and `tangent` with source-spanned arity diagnostics;
  `odualcall` constructs the two-component `ODual`; `odualparts` admits mixed
  scalar operands; `odoubledualbin` implements addition, subtraction,
  multiplication, and division rules; `ounary` negates both components; and
  `oprintvalue` renders the language form `dual(primal, tangent)`. The
  `TestNativeCoreDualScalarParity` differential witness covers construction,
  mixed arithmetic, unary negation, rendering, and projections through the
  interpreter, VM, generated C, and native executable. The dedicated lowering
  and arity tests check the allocation-free/no-capability boundary and
  source-spanned malformed-call rejection; `TestNativeCoreDualScalarRuntimeErrorContract`
  checks matching division-by-zero behavior. Normal, `-race -vet=off`, and
  Homebrew LLVM AddressSanitizer evidence pass for all dedicated dual tests.
  This closes the dual-scalar slice; wider numeric families and cross-platform
  evidence remain separate gaps.

- Named-product/struct patterns now have a coordinated full-stack slice. The
  source form `Point { x: x, y: y }` is parsed as `structPattern` with named
  recursive field patterns. `validateEnumPattern` rejects unknown product
  types, newtypes, duplicate fields, and missing fields with source spans;
  `matchPattern` provides the shared interpreter/VM semantics; formatter,
  cloning, macro freshening, alias binding, and `patternBindingNames` preserve
  nested bindings. Native `lowerNativeEnumPatternBranch` and
  `bindNativeEnumPatternValue` lower `matchproduct`/`productfield` CFG
  operations, while `emitCInstr` and the generated runtime use `oproductis`
  and `oproductfield` without allocations or capabilities.
- `TestNativeCoreProductPatternParity` proves complete and partial named
  field matching, recursive bindings, failed alternatives, and executable
  interpreter/VM/generated-C/native parity. The dedicated format/clone and IR
  contract tests cover AST preservation and instruction shape;
  `TestNativeCoreProductPatternDiagnostic` covers missing-field source
  diagnostics; and `TestNativeCoreProductPatternOwnershipAndCapabilityContract`
  checks the no-capability/no-lookup-allocation boundary.

This evidence is intentionally incremental. The task remains **Ready** for
the Native C priority lane, but is not complete:
remaining wider numeric and complex-width surfaces, full sanitizer/race
evidence for the complete product family, and cross-platform generated-C
evidence still require implementation or execution before any matrix cell can
be changed.

The remaining wide numeric/provider and alternate-platform boundaries are
tracked separately; this product-pattern slice does not promote the broad
patterns-and-matching matrix cell by itself.

### Bounded fixed-width bit-operation slice (2026-08-09)

The complete pure unsigned bit family in `std/math` now has a direct generated-C
ABI: `count_ones`, `count_zeros`, `leading_zeros`, `trailing_zeros`,
`bit_width`, `bit_floor`, `bit_ceil`, `has_single_bit`, and `byte_swap`.
`ostd_math_bit_input` validates the explicit `u8`/`u16`/`u32`/`u64` tag and
width and masks the copied machine value. The operation-specific helpers
preserve the reference width and zero behavior; `bit_ceil` checks the declared
width before constructing its result, and `byte_swap` rejects non-byte-aligned
widths. No helper allocates or retains input storage, and no capability macro
is emitted.

`TestNativeCoreBitOperationsParity`,
`TestNativeCoreBitOperationsLoweringOwnershipAndCapabilityContract`, and
`TestNativeCoreBitOperationsErrorContract` in
`internal/osiris/native_core_bitwise_test.go` provide differential, ownership,
capability, inventory, and runtime-error evidence. This is an incremental
core/data proof only. The granular `std-math-bit-operations` support row is now
stable for this bounded family; the broad numeric row remains open for the
wider numeric families and external platform evidence recorded elsewhere.

### Bounded pure integer-math slice (2026-08-09)

The locally executable fixed-width unsigned `std/math` reduction surface now
has a direct Native C ABI:

- `emitCInstr` recognizes `gcd` and `lcm` static math members, enforces their
  two-argument shape, and emits direct calls instead of `ostd_dispatch` or
  `ocall`.
- `ostd_math_unsigned` validates `O_UINT` values with explicit widths from 1
  through 64 bits.
- `ostd_math_gcd` uses the Euclidean remainder loop and returns the wider
  input width through `ouintbits`.
- `ostd_math_lcm` divides before multiplying, detects machine-word overflow,
  checks the declared-width limit, and returns a structured failure rather
  than wrapping an unrepresentable result.

`TestNativeCoreIntegerMathParity` and
`TestNativeCoreIntegerMathLoweringContract` in
`internal/osiris/native_core_numeric_test.go` cover mixed-width inputs,
zero behavior, interpreter/VM/generated-C/native executable parity, direct
helper selection, and the no-capability boundary. The exact API and remaining
signed/arbitrary-precision boundary are documented in
`docs/design/NATIVE-C.md` under “Native fixed-width integer math”. This is an
incremental proof only; the granular `std-math-unsigned-reductions` row is
stable while the broad `std-math-scalar` row and task remain open.

### Bounded fixed-width unsigned `divmod` slice (2026-08-09)

`std/math.divmod` now has a direct Native C path for explicit fixed-width
unsigned values. `emitCInstr` selects `ostd_math_divmod`, which validates the
`O_UINT` tags and widths, rejects a zero divisor, computes quotient and
remainder at the wider input width, and returns an owned two-item list. The
helper does not retain input storage or define a host capability. Signed and
arbitrary-precision reference forms remain outside this bounded ABI.

`TestNativeCoreUnsignedDivmodParity`,
`TestNativeCoreUnsignedDivmodLoweringContract`, and
`TestNativeCoreUnsignedDivmodErrorContract` in
`internal/osiris/native_core_divmod_test.go` provide interpreter/VM/generated-C
parity, direct-lowering and inventory evidence, ownership/capability checks,
and zero-divisor/unsupported-kind runtime errors. The exact contract is
documented in `docs/design/NATIVE-C.md` under “Native fixed-width unsigned
`divmod`”. The granular `std-math-unsigned-divmod` support row is stable; the
broader `std-math-scalar` row and this task remain open.

### Bounded scalar extrema inventory closure (2026-08-09)

The capability ledger now agrees with the existing direct generated-C
lowerings for `std/math.min`, `std/math.max`, and `std/math.clamp`. The three
members use `ominmax`/`oclamp` for the documented numeric subset and do not
fall through to module dispatch or define a host capability. Existing normal
executable parity covers mixed numeric extrema and f32/f64 clamp behavior;
`TestNativeCoreMathExtremaInventoryAndLoweringContract` adds direct-helper,
inventory, and no-dispatch evidence. The granular `std-math-extrema` row is
stable; the task remains Ready while broader numeric families, complete
sanitizer coverage, and alternate-platform generated-C evidence remain open.

### Integer-math inventory reconciliation (2026-08-09)

The backend capability ledger now reports the already-tested direct lowerings
for `std/math.gcd` and `std/math.lcm` as implemented. The entries were
previously omitted even though `emitCInstr` selected `ostd_math_gcd` and
`ostd_math_lcm`, causing inventory consumers to under-report the native
surface. `TestNativeCoreIntegerMathLoweringContract` now asserts both
individual inventory entries in addition to direct-helper, no-dispatch, and
no-capability evidence. This closes an inventory inconsistency only; signed,
arbitrary-precision, full numeric-family, sanitizer, and alternate-platform
criteria remain open.

### Scalar math inventory reconciliation (2026-08-09)

The backend ledger now recognizes the direct Native C scalar lowerings already
covered by the emitter for `sqrt`, `expt`, the real trigonometric and
hyperbolic families, `cbrt`, `expm1`, `log1p`, `erf`, `erfc`, `gamma`, `lgamma`,
`floor`, `ceil`, and `round`. The status remains a documented subset: each
operation retains its existing numeric-kind and domain gates, and unsupported
widths or families still fail at native compile time.
`TestNativeCoreMathScalarInventoryCoversDirectLowerings` prevents the ledger
from regressing to unsupported for these direct paths; this does not promote
the aggregate numeric matrix cell.

### Bounded real scalar math slice (2026-08-09)

The Native C emitter now directly lowers the matching-width real scalar
operations `atan2`, `exp2`, and `trunc`, for both explicit `f32`/`f64` values
and the default floating `number` form. `emitCInstr` admits their builtin
identities and validates arity; `nativeScalarBuiltinResultKind` proves the
result width; and `emitScalarBuiltin` emits the direct C operation. `exp2`
retains the reference overflow checks before and after `f32` narrowing, while
`atan2` and `trunc` preserve the existing IEEE and signed-zero behavior.

`nativeBuiltinStatus` and `nativeModuleMemberStatus` now expose individual
inventory entries for all three operations. `TestNativeCoreMathScalarParity`
in `internal/osiris/native_core_math_scalar_test.go` compares builtin and
qualified calls across the interpreter, VM, generated C, and native
executable. `TestNativeCoreMathScalarLoweringOwnershipAndCapabilityContract`
checks direct C symbols, no allocation or input retention in the generated
main, no dispatcher or host capability, and the individual inventory entries.
`TestNativeCoreMathScalarErrorsRemainStructured` verifies matching overflow
errors, and `TestNativeCoreMathScalarArityDiagnostic` verifies malformed-call
rejection. The public contracts are documented individually in
`docs/src/supplemental-reference.json` and the Native C ownership/error
contract is in `docs/design/NATIVE-C.md` under “Native scalar math:
`atan2`, `exp2`, and `trunc`”.

This is a bounded numeric slice only. The task remains open for the wider
numeric-family, sanitizer, and alternate-platform criteria; support-matrix
and task-completion statuses are unchanged.

### Bounded real remainder math slice (2026-08-09)

The Native C scalar emitter now directly lowers `std/math.fmod`,
`std/math.remainder`, and `std/math.fma` for matching-width default floating,
`f32`, and `f64` arguments. `emitCInstr` recognizes the builtin identities and
validates arity; `nativeScalarBuiltinResultKind` proves the result width; and
`emitScalarBuiltin` emits `fmod`, `remainder`, or fused `fma`. The first two
check a zero divisor before calling C, preserving the reference structured
`invalid_argument` error. The `f32` paths evaluate already-rounded operands
through double and narrow once at the result boundary, while `fma` retains one
fused rounding step.

`nativeBuiltinStatus` and `nativeModuleMemberStatus` expose individual
implemented entries. `TestNativeCoreMathRemainderParity` covers interpreter,
VM, generated C, and native executable differential behavior;
`TestNativeCoreMathRemainderLoweringOwnershipAndCapabilityContract` checks
direct calls, allocation and input ownership, dispatcher absence, and host
capability absence; and the zero-divisor and arity tests cover structured
runtime and malformed-call failures. This evidence does not change aggregate
matrix or task status and does not claim mixed-width, complex, dynamic,
sanitizer, or alternate-platform parity.

The bounded `std-math-remainder` row is now stable for Native C. The existing
direct emitter and contract tests prove generated-executable parity for
`fmod`, `remainder`, and `fma` across matching-width default, `f32`, and `f64`
floating scalars; zero divisors retain structured invalid-argument failures;
arity and mixed-width shapes fail during lowering; and the generated main has
no allocation, dispatch, evaluator fallback, or host capability. This row is
deliberately separate from the still-partial aggregate scalar-math row and
does not claim complex, integer, dynamic, sanitizer, or alternate-platform
parity.

### Direct numeric and tensor capability-ledger reconciliation (2026-08-09)

The capability ledger now reports individual entries for the direct Native C
lowerings already present in `cbackend_native.go` without changing that
emitter or any generated runtime implementation. The reconciled builtin
surface includes:

- numeric constructors and projections: `uint`, `byte`, `bytes`, `complex`,
  `dual`, `primal`, `tangent`, `conjugate`, and `magnitude`;
- scalar math and reductions: `atan2`, `exp2`, `trunc`, `copysign`,
  `nextafter`, `isfinite`, `isinf`, `isnan`, `isnormal`, `signbit`, `degrees`,
  `radians`, `sign`, `step`, `heaviside`, `relu`, `sigmoid`, `softplus`,
  `mean`, `std`, `var`, `median`, `quantile`, `percentile`, `argmin`,
  `argmax`, `any`, `all`, `cumsum`, `cumprod`, and `phase`;
- tensor constructors and portable operations: `zeros`, `ones`, `full`,
  `identity`, `eye`, `arange`, `linspace`, `logspace`, `tensor`, `matrix`,
  `at`, `setat`, `reshape`, `transpose`, `matmul`, `outer`, `trace`, `det`,
  `solve`, `inv`, `cholesky`, `qr`, `shape`, `flatten`, `squeeze`,
  `expand_dims`, `broadcast_to`, `clip`, `where`, `concat`, `stack`, `sum`,
  `product`, `dot`, and `norm`;
- fixed-width integer helpers already emitted directly: `gcd`, `lcm`, and the
  unsigned bit-operation family.

`TestNativeCoreDirectLoweringInventory` asserts both root builtin and
`std/math` member records. `TestNativeCoreDirectLoweringContract` compiles
representative constructor, projection, tensor, linear-algebra, and reduction
programs and checks the generated `main` for the exact direct helper calls,
absence of `ostd_dispatch`, `ocall`, and host-capability macros. The public
signatures and per-operation Native C boundaries are documented individually
in `docs/src/supplemental-reference.json`; the implementation-level ownership,
shape, and error contracts are in `docs/design/NATIVE-C.md` under the numeric
constructor, scalar reduction, tensor constructor, and tensor operation
sections.

The initial ledger reconciliation intentionally excluded provider-dependent
`eig`, `eigh`, and `svd` because their emitter branches require configured
numerical-provider ABIs. That boundary was subsequently closed by the
provider-backed section below; the stable claim is limited to the configured
provider ABI and does not imply a bundled general-purpose solver.

### Configured numerical-provider linear algebra closure (2026-08-09)

The bounded provider-backed linear-algebra surface is now covered directly.
`TestCBackendEighProviderParity`, `TestCBackendSVDProviderParity`,
`TestCBackendEigProviderParity`, and `TestCBackendEigProviderComplexResidual`
execute generated C with the deterministic diagonal provider and compare
interpreter/VM/native results, including complex eigenvector residuals.
`TestNativeCoreMathLinalgProviderLoweringOwnershipAndCapabilityContract`
verifies the direct `oeigh`, `osvd`, and `oeig` helpers, ABI-provider lookup,
owned tensor/list result construction, and missing-provider diagnostics;
`TestCBackendProviderABIMismatchDiagnostic` covers an incompatible provider.
The focused normal and race suites pass. The public Native C linear-algebra
cell is now stable for this explicitly configured ABI surface; a bundled
general-purpose solver and alternate-platform production provider remain out of
scope and are recorded as deployment requirements.

### Native OSR1 Result, Option, and Error codec slice (2026-08-09)

The generated C codec now preserves the structured OSR1 records for
`O_ERROR` (`0x0a`), `O_RESULT` (`0x16`), and `O_OPTION` (`0x17`). The codec
implementation is split across `ocodecerror`, `ocodecerrorDecode`,
`ocodecencodeinto`, and `ocodecdecodefrom`, with bounded recursive traversal,
canonical map-key ordering, strict version/tag/length checks, and tracked
copies for decoded strings, boxes, maps, stack records, and error records.
Result success records carry one nested value; failed records carry one tagged
error record; Options carry a presence byte and only encode a payload when
present. The existing `ODatetime` seconds/nanosecond representation is
unchanged.

`TestNativeCoreCodecResultOptionErrorParity` compares fixed OSR1 byte records,
successful and failed Results, present and absent Options, decoded Error field
access, and round trips across the interpreter, VM, generated C, and native
executable. `TestNativeCoreCodecResultOptionErrorRejectsMalformedRecords`
checks invalid presence flags, unsupported Error versions, truncated records,
and malformed framing. Both tests require no host capability and exercise the
same code under normal and race builds. The public codec contract is recorded
individually in `docs/src/reference-data.json` and the Native C design chapter.
`TestNativeResultOptionErrorConstructorContract` adds exact constructor arity
and wrong-kind execution checks plus direct-helper, payload-box, and ownership
markers. The granular `native-result-option-error-codec` row is stable; task
162 still requires the broader wide numeric, provider-backed linear algebra,
and alternate-platform surfaces.

The broader bounded `native-osr1-codecs` row is now stable for Native C. It
combines the existing scalar/cross-direction codec contract with the
structured Error/Result/Option records: generated C preserves canonical OSR1
tags, validates framing and recursive bounds, copies decoded storage into
tracked ownership, and rejects malformed records and constructor shapes
without dispatch or host capability. The aggregate `data-helpers` row remains
partial for data-pipeline helpers and codec tags outside this maintained OSR1
tree.

### Qualified list range and numeric reductions (2026-08-09)

The pure qualified calls `std/list.range`, `std/list.sum`, and
`std/list.product` now bypass the general native module dispatcher. In
`emitCInstr`, the emitter validates the one-argument shape and selects the
owned helpers `ostd_list_range`, `ostd_list_sum`, and `ostd_list_product`
directly. The existing helpers retain the reference contracts: range produces
the bounded floating sequence from zero, sum/product preserve the widest
explicit floating width and complex family, and empty reductions return their
mathematical identities. Wrong kinds, invalid range values, non-numeric list
items, and malformed arity remain structured native failures.

`TestNativeListRangeAndReductionParity` and
`TestNativeListRangeAndReductionUseOwnedDirectHelpers` in
`internal/osiris/native_list_range_reduction_test.go` cover interpreter/VM/
generated-C/native executable parity, empty identities, complex values,
structured failures, direct helper selection, dispatcher absence at the call
site, and the no-capability boundary. The focused normal and race commands
are:

```text
GOCACHE=/tmp/osiris-list-range-reduction go test ./internal/osiris \
  -run '^TestNativeListRangeAndReduction' -count=1 -timeout=600s
GOCACHE=/tmp/osiris-list-range-reduction-race go test -race -vet=off \
  ./internal/osiris -run '^TestNativeListRangeAndReduction' -count=1 -timeout=600s
```

This is a bounded direct-call optimization and evidence slice. It does not
promote the aggregate array, range, or reduction matrix cells: tensor and
array constructors, wider reduction families, alternate-platform execution,
and the remaining qualified `std/list` members retain their existing
boundaries.

### Sanitizer floating-point parity boundary (2026-08-09)

Generated-C sanitizer builds now retain the production `-O3 -flto` boundary in
`CompileCExecutableCachedWithOptions` and `compileCArtifact`. The cache
manifest records those flags, and `TestCBackendSanitizerFlagsAreCacheKeyed`
asserts both the requested sanitizer and LTO. This is required because the
Native C runtime must have the same floating-point contraction behavior under
instrumentation as it has in the production executable.

The portable `osolve` elimination update now calls C `fma` explicitly. This
turns the fused rounding step into a source-level ABI contract rather than a
compiler heuristic and removes the one-ULP solve rendering mismatch previously
seen under sanitizer optimization. The complete Native C suite passed on the
current macOS host under normal, `-race -vet=off`, LLVM ASan, and LLVM UBSan
runs. The exact commands and runtimes were:

```text
go test ./internal/osiris -run '^TestCBackend' -count=1 -timeout=3600s
ok   github.com/sphireinc/osiris/internal/osiris  99.456s

go test -race -vet=off ./internal/osiris -run '^TestCBackend' -count=1 -timeout=3600s
ok   github.com/sphireinc/osiris/internal/osiris  100.295s

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
go test ./internal/osiris -run '^TestCBackend' -count=1 -timeout=3600s
ok   github.com/sphireinc/osiris/internal/osiris  121.230s

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=undefined \
go test ./internal/osiris -run '^TestCBackend' -count=1 -timeout=3600s
ok   github.com/sphireinc/osiris/internal/osiris  121.941s
```

This closes the local sanitizer-evidence gap for the generated-C core/data
slice. It does not provide Linux, Windows, or provider-backed numerical
execution evidence, so the aggregate task and support-matrix status remain
open.

### Native unreachable terminator lowering (2026-08-09)

`emitCTerm` now handles the IR `irUnreachable` terminator explicitly. Generated
function bodies return the structured native failure `ofail("unreachable")`,
while the generated program entry point returns a non-zero status. This keeps
the Native C emitter total for valid IR and preserves a failure boundary rather
than converting an unreachable terminator into an emitter-only error.
`TestNativeCUnreachableTerminatorHasExplicitFailureLowering` checks both
function and entry-point output directly.

### Native tensor shape/value and scalar classification slices (2026-08-09)

The Native C emitter now has direct, owned lowering for the bounded real
numeric tensor operations `squeeze`, `expand_dims`, `broadcast_to`, `clip`,
`where`, `concat`, and `stack`. `emitCInstr` validates their individual
arities and calls `otensor_squeeze`, `otensor_expand_dims`,
`otensor_broadcast_to`, `otensor_clip`, `otensor_where`, `otensor_concat`, or
`otensor_stack` directly. The portable helpers normalize rectangular numeric
lists and scalars where permitted, preserve row-major shape and width metadata,
validate axes and broadcast compatibility, and transfer only newly owned
results. `TestNativeTensorShapeValueOperationsParity` and its invalid-shape
cases compare the interpreter, VM, and generated C executable and reject
axis, broadcast, concatenation, and stack mismatches. The focused normal,
race, and LLVM ASan suites passed on the current macOS host.

The scalar math slice adds direct Native C support for `copysign`, `nextafter`,
`isfinite`, `isinf`, `isnan`, `isnormal`, `signbit`, `degrees`, and `radians`.
`nativeScalarBuiltinResultKind` and `emitScalarBuiltin` preserve f32/f64
widths, including width-specific `nextafter` and normal-number thresholds.
`TestNativeCoreMathClassificationAndAngleParity`, boundary, lowering, and
error tests cover interpreter/VM/generated-C parity, signed zero, NaN,
infinity, subnormal values, f32 narrowing, and invalid calls. The focused
normal, race, and LLVM ASan suites passed on the current macOS host.

This evidence did not close the complete task. At that checkpoint, the
tensor-only `repeat`, `tile`, `split`, `gather`, `scatter`, and `pad` cluster,
along with `clone` and `view`, remained unlowered; complex tensor variants and
the broader module/pattern/task/data surfaces remained separate gaps.
Linux/Windows execution evidence and provider-independent cross-platform
evidence remain environment-gated by task 400. The later task-162 bounded
shape/data section below records the six newly implemented real-tensor paths;
`clone` and `view` remain unsupported.

### Bounded Native Result/Option combinator and try/catch slice (2026-08-09)

This slice closes the locally implementable pure combinator gap without
changing support-matrix statuses. `emitCInstr` now lowers the existing `async`
IR forms `map`, `map_error`, and `flat_map` to the synchronous helpers
`omapresult`, `omaperror`, and `oflatmap`; `isok` and `iserr` now have direct
tagged-ABI predicates alongside the existing Option predicates. The helpers
preserve Result short-circuit behavior, invoke callbacks only on the matching
branch, convert Option callbacks for `flat_map`, and preserve structured
callback failures through the existing `ohaderror`/`olasterror` state used by
`otry`.

`TestNativeResultOptionCombinatorAndTryParity` provides interpreter/VM/
generated-C/native executable differential coverage for successful and failed
map, error mapping, Result and Option flat mapping, predicates, `unwrap`,
`let?`, and caught structured errors. The malformed/callback test covers
source-level arity diagnostics and callback failures. The ownership/no-fallback
test checks `omapresult`, `omaperror`, `oflatmap`, `oresultbox`, `ooptionbox`,
`otrack`, `otry`, and `oresultbind` in generated C and rejects
`ostd_dispatch`/capability injection. All behavior is pure and local; no
Linux/Windows/provider evidence is included.

The remaining structured-error boundary is explicit: `wrap`, cause/metadata
mutation, full error-field preservation, dynamic callback ABI shapes, and
asynchronous task/host failure paths remain outside this bounded slice. The
aggregate errors/results/options row and task 162 remain open; no support
matrix or task-completion status was changed.

### Native structured-error wrapping (2026-08-09)

`wrap(cause, kind, message)` now has a direct Native C lowering through
`owrap`. It accepts a structured Error or `null` cause, validates the kind and
message strings, creates a new outer Error, and tracks an independent copy of
the cause value. The direct `cause(error)` accessor uses `ocause` to return the
tracked cause or `null` without a dispatcher. `TestNativeStructuredErrorWrapParity`
compares nested cause construction, cause extraction, and invalid-input behavior
across interpreter, VM, generated C, and the native executable;
`TestNativeStructuredErrorWrapLoweringOwnsCauseWithoutFallback` checks the
direct helpers, cause ownership, inventory admission, and absence of capability
or dispatcher fallback. Full metadata/debug/source-location mutation and
asynchronous error paths remain separate gaps; no matrix or task status changed.

### Native tensor shape/data cluster (2026-08-09, task-162 bounded slice)

The next local Native C slice directly lowers the real-tensor subset of the
global `repeat`, `tile`, `split`, `gather`, `scatter`, and `pad` builtins.
`emitCInstr` validates the two-, three-, or optional-four-argument forms and
emits `otensor_repeat`, `otensor_tile`, `otensor_split`, `otensor_gather`,
`otensor_scatter`, or `otensor_pad`. Each helper validates integral shape,
count, width, and index contracts before publishing an owned result. Source
buffers and index/value staging buffers are copied or released; no helper
retains an input pointer, invokes `ostd_dispatch`, or defines a host
capability. Repeat/tile use leading-dimension repetition, split returns
owned chunks, gather adopts the index tensor shape, scatter applies updates
in input order, and pad uses the leading dimension with an optional scalar
fill.

`TestTask162NativeTensorShapeDataParity` provides interpreter/VM/generated-C
executable differential evidence. `TestTask162NativeTensorShapeDataInvalidShapes`
covers negative and zero counts, out-of-range indices, scatter cardinality,
and negative padding widths through matching caught errors.
`TestTask162NativeTensorShapeDataLoweringOwnershipAndNoDispatch` checks the
direct helper symbols, generated ownership markers, no-dispatch/no-callable
fallback boundary, and only the six reconciled builtin inventory entries.
The existing `std/math` tensor-shape section in `docs/design/NATIVE-C.md`
records the exact real-tensor contract and retains complex tensors,
string-polymorphic repeat/split, `clone`, and `view` as unsupported remainder.

This is a bounded local slice, not task closure. Wider complex tensor forms,
clone/view alias semantics, exact reduction/constructor gaps, provider and
alternate-platform evidence, and the broader Native C matrix remain open.

## Pause checkpoint (2026-08-09)

Implementation is paused with task status still `Ready`; the task remains
locally actionable and is not complete. Proven slices include core control
flow/cleanup, Result/Option combinators, tuples/products/aliases/newtypes,
selected numeric families, real tensor shape/data operations, codecs, and
provider-backed linear-algebra contracts. Remaining work includes full
structured-error field/cause metadata mutation beyond the new `wrap` slice,
unsupported complex
and alias/view tensor semantics, remaining constructor and reduction families
beyond the covered list range/sum/product slice,
async task failure behavior, and the required sanitizer, Linux, provider, and
cross-platform evidence. No matrix status changed.

Native structured-error `wrap` and `cause` now have direct generated-C
lowering, owned cause storage, type/arity validation, and focused normal,
race, and lowering-ownership coverage. The native inventory test is aligned
with the existing direct tensor-shape lowering for `tile`, `gather`, `scatter`,
and `pad`; `clone` and `view` remain explicitly unsupported. These are bounded
local slices only and do not promote the task or its aggregate matrix cells.
