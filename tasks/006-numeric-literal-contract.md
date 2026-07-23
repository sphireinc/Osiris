# Numeric literal contract and inference

Source: TYPECHK.txt and CHECKLIST.md.

Specify unambiguous literal syntax, promotion, precision loss, and fallback
rules while retaining number as the documented default. Reconcile the stale
checklist statement that integer/float separation has not started.

Acceptance: lexer/parser/runtime/codec/inference agree; invalid suffixes report
source spans; osiris infer explains inferred kind and fallback boundary.
