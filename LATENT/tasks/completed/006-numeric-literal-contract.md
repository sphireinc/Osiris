# Numeric literal contract and inference

Source: TYPECHK.txt and CHECKLIST.md.

Specify unambiguous literal syntax, promotion, precision loss, and fallback
rules while retaining number as the documented default. Reconcile the stale
checklist statement that integer/float separation has not started.

Acceptance: lexer/parser/runtime/codec/inference agree; invalid suffixes report
source spans; osiris infer explains inferred kind and fallback boundary.

## Completion evidence

- `docs/design/NUMERIC-LITERALS.md` defines defaults, bases, separators,
  suffixes, inference spelling, conversion/error behavior, and native boundary.
- `newdocs/` contains the canonical public numeric-literal entry with a
  checked example.
- `TestNumericLiteralContractAcrossParserInferenceAndCodec` covers default,
  width-qualified, decimal, bfloat, and complex literals through parsing,
  runtime rendering, inference, and codec round trips.
- `TestInvalidNumericSuffixHasParserLocation` locks invalid-suffix diagnostics
  to the numeric token location.
