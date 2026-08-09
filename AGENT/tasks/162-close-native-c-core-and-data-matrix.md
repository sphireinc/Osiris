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
core/data proof only; the task remains open for the wider numeric families and
the external platform evidence recorded elsewhere.

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
incremental proof only; no support-matrix or task-completion status changed.

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
`divmod`”. No aggregate support-matrix or task-completion status changed.

### Bounded scalar extrema inventory closure (2026-08-09)

The capability ledger now agrees with the existing direct generated-C
lowerings for `std/math.min`, `std/math.max`, and `std/math.clamp`. The three
members use `ominmax`/`oclamp` for the documented numeric subset and do not
fall through to module dispatch or define a host capability. Existing normal
executable parity covers mixed numeric extrema and f32/f64 clamp behavior;
`TestNativeCoreMathExtremaInventoryAndLoweringContract` adds direct-helper,
inventory, and no-dispatch evidence. This is a ledger/data-surface slice only;
the task remains Ready while broader numeric families, complete sanitizer
coverage, and alternate-platform generated-C evidence remain open.

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

### Direct numeric and tensor capability-ledger reconciliation (2026-08-09)

The capability ledger now reports individual entries for the direct Native C
lowerings already present in `cbackend_native.go` without changing that
emitter or any generated runtime implementation. The reconciled builtin
surface includes:

- numeric constructors and projections: `uint`, `byte`, `bytes`, `complex`,
  `dual`, `primal`, `tangent`, `conjugate`, and `magnitude`;
- scalar math and reductions: `atan2`, `exp2`, `trunc`, `sign`, `step`,
  `heaviside`, `relu`, `sigmoid`, `softplus`, `mean`, `std`, `var`, `median`,
  `quantile`, `percentile`, `argmin`, `argmax`, `any`, `all`, `cumsum`,
  `cumprod`, and `phase`;
- tensor constructors and portable operations: `zeros`, `ones`, `full`,
  `identity`, `eye`, `arange`, `linspace`, `logspace`, `tensor`, `matrix`,
  `at`, `setat`, `reshape`, `transpose`, `matmul`, `outer`, `trace`, `det`,
  `solve`, `inv`, `cholesky`, `qr`, `shape`, `flatten`, `sum`, `product`,
  `dot`, and `norm`;
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

Provider-dependent `eig`, `eigh`, and `svd` are deliberately not included in
this ledger reconciliation. Their emitter branches require configured
numerical-provider ABIs and need separate provider-backed execution evidence.
No aggregate support-matrix cell or task status changed.

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
The bounded slice is complete; task 162 still requires the broader wide
numeric, provider-backed linear algebra, and alternate-platform surfaces.
