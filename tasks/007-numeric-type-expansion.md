# Numeric type expansion

Source: TYPECHK.txt.

Implement in separate pull requests: parametric complex<f32> and complex<f64>;
one approved derived type (quaternion, dual, or polynomial); shape inference for
matrix/tensor; then sparse storage and named-unit designs. Do not add unsupported
exotic widths merely for syntax completeness.

Acceptance: each selected feature has a focused design, parser/formatter/runtime/
codec/inference tests, documented conversions, and native hooks where relevant.
