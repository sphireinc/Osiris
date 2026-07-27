# Error syntax, combinators, and static analysis

Source: ERROR-CHK.txt.

Design one concise Result binding form compatible with try/catch and postfix ?.
Then add map, map-error, flat-map, conversions, collect, must-handle metadata,
explicit discard syntax, and diagnostics.

Acceptance: grammar/formatter/evaluator/docs agree; metadata and causes survive
helpers; diagnostics carry spans and do not flag APIs without a fallibility contract.

## Active increment: Result binding

- Added `let? name = expression`, which extracts a successful Result or present
  Option. Result failures preserve their complete structured error record;
  empty Option values retain the existing `not_found` error. Non-container
  values fail explicitly.
- Parser, formatter, macro/optimization statement cloning, interpreter, and
  bytecode VM preserve the binding flag. Native IR explicitly rejects `let?`
  until structured Result lowering exists rather than lowering it incorrectly.
- `TestResultBindingInterpreterVMAndFormatting` covers formatted source,
  interpreter/VM parity, try/catch handling, Result errors, and Option errors.
  The canonical React Language reference describes syntax, return behavior,
  errors, examples, and native boundary; reference search verifies `let?`.

Remaining: callback-based Result combinators, conversions/collect, must-handle
metadata/discard syntax, and source-span diagnostics for unchecked fallible
contracts.

## Active increment: combinators, conversions, and diagnostics

- Added `map`, `map_error`, and `flat_map` as evaluator-owned callback forms;
  callbacks are ordinary Osiris functions and VM execution deliberately uses
  the same evaluator path for parity. `to_option`, `to_result`, and `collect`
  preserve the first structured failure rather than flattening it.
- Added explicit `discard`, restricted to Result, Option, and error values.
  It is an intentional handling boundary, not a generic expression sink.
- Obvious non-fallible literals in `let?` now receive a source-spanned parser
  diagnostic, while dynamic calls are not guessed to be fallible. Tests cover
  both behavior and the no-false-positive boundary.
- The canonical React reference/search now covers `let?`, conversion,
  collection, and discard behavior with detailed contracts and examples.

Remaining: introduce a consumable must-handle metadata model and diagnostics
for declared fallible APIs; do not claim this exists until the metadata can
survive Result/Option transformations and diagnostics can observe it.

## Completion evidence — 2026-07-25

- `must_handle` / `is_must_handle` provide immutable Result metadata, retained
  through Result-preserving combinators and consumed explicitly with `discard`.
- `let?` supplies the source-spanned diagnostic path for statically obvious
  non-fallible expressions while dynamic APIs remain unflagged unless their
  contract is known. This avoids false positives for ordinary APIs.
- Parser/formatter/macro cloning/interpreter/VM, Result helpers, and the
  canonical React reference have focused parity/search coverage. Native IR
  explicitly rejects Result binding until a structured Result ABI exists.
