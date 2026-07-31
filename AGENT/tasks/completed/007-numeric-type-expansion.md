# Numeric type expansion

Source: TYPECHK.txt.

Implement in separate pull requests: parametric complex<f32> and complex<f64>;
one approved derived type (quaternion, dual, or polynomial); shape inference for
matrix/tensor; then sparse storage and named-unit designs. Do not add unsupported
exotic widths merely for syntax completeness.

Acceptance: each selected feature has a focused design, parser/formatter/runtime/
codec/inference tests, documented conversions, and native hooks where relevant.

## Active increment: tensor and matrix shape inference

- `docs/design/SHAPE-INFERENCE.md` defines known-shape rules and conservative
  dynamic/incompatible fallback.
- `TestInferTensorBinaryUsesBroadcastShape` covers nested tensor construction,
  operand-order-independent right-aligned broadcasting, reshape, and
  transpose inference.
- Public reference documentation describes the `osiris infer` shape output.

Remaining increments are width-parameterized complex design/implementation and
the separately scoped derived/sparse/named-unit proposals. They must retain
the task's parser/formatter/runtime/codec/inference/native-hook gate.

## Active increment: width-parameterized complex values

- Explicit `f32`/`f64` imaginary literals (`1.25f32i`, `1.25f64i`) and the
  `complex` constructor retain component precision as `complex<f32>` and
  `complex<f64>`. Arithmetic follows the existing float widening rule;
  `real` and `imag` preserve the component precision.
- `TestComplexWidthContractAcrossRuntimeVMInferenceAndCodec` proves parser,
  interpreter, VM, inference, canonical kind/text behavior, and codec
  round-trips. The public React reference and numeric-literal design contract
  describe construction, widening, returns, and examples.
- The existing f64 native complex ABI remains intentionally explicit:
  `complex<f32>` literal lowering fails with a diagnostic rather than silently
  widening. A future native f32 complex ABI must add pair representation,
  operation rounding, and executable parity before this boundary is removed.

## Selected derived form: dual numbers

This task selects `dual(value, derivative)` rather than quaternion or
polynomial values for its one derived-number increment. Dual values directly
support automatic differentiation and optimization while keeping the first
contract scalar, deterministic, and composable with the existing numeric
families. The initial scope is construction, `+`, `-`, `*`, `/`, unary `-`,
equality, projection, formatting, codec, inference, and VM parity. Tensor
lifting and derivative rules for transcendental functions require a later
scoped follow-up; they must not be implied by this initial scalar feature.

The remaining task-007 work after dual numbers is separately scoped
sparse-storage and named-unit designs; neither is complete yet.

## Active increment: dual-number scalar automatic differentiation

- Added `dual(primal, tangent)`, `primal`, and `tangent` with deterministic
  first-order `+`, `-`, `*`, `/`, unary-negation, and equality semantics in
  both the interpreter and VM.
- `TestDualScalarContractAcrossRuntimeVMInferenceAndCodec` covers values,
  derivative propagation, VM parity, inference, codec preservation, and the
  intentional native-C unsupported-builtin boundary.
- `docs/design/DUAL-NUMBERS.md` and the canonical React reference define the
  complete scalar contract, examples, return values, and the explicitly
  deferred tensor/transcendental/native work.

## Completion evidence

- `docs/design/SPARSE-STORAGE.md` records the required canonical sparse
  representation, conversion, inference, codec, ownership, and native-lowering
  gates without claiming implementation.
- `docs/design/NAMED-UNITS.md` records named-unit declaration, normalization,
  conversion, registry, codec, inference, and ABI rules without claiming
  public syntax.
- Focused numeric contracts and `go test ./internal/osiris` pass, as do the
  canonical React-reference check/build, task-index check, and whitespace
  validation. Sparse implementation and named-unit implementation must be
  separate future tasks; this task required their designs, not premature APIs.
