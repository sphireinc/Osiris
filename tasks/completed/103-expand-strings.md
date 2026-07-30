# Task 103: Expand `std/text` string operations

**Priority:** P2  
**Dependencies:** 025, 032, 039, 040  
**Status:** Complete  

## Objective

Expand the existing `std/text` module with practical comparison, search,
cutting, splitting, and trimming operations inspired by Go's `strings`
package. Keep one Osiris text module; do not introduce a competing
`std/strings` namespace.

Primary inventory: <https://pkg.go.dev/strings>

## Existing surface to preserve

Audit the runtime, module export table, Unicode indexing behavior, native
backend, and canonical `std/text` page before editing. Do not duplicate
`trim`, `upper`, `lower`, `replace`, `repeat`, `split`, `join`, `chunks`,
`slug`, or `cat`.

## Required first-release additions

Add these qualified `std/text` functions, adjusting a spelling only when an
existing Osiris convention requires it:

- comparison and predicates: `compare`, `equal_fold`, `contains`,
  `contains_any`, `has_prefix`, and `has_suffix`;
- search and counting: `count`, `index`, `index_any`, `last_index`, and
  `last_index_any`;
- cutting: `cut`, `cut_prefix`, and `cut_suffix`;
- tokenization: `fields`, `split_after`, `split_n`, and `split_after_n`;
- trimming: `trim_space`, `trim_left`, `trim_right`, `trim_prefix`, and
  `trim_suffix`;
- replacement: `replace_n`.

## Semantic contract

- Osiris strings are immutable UTF-8 text. Search indexes count Unicode code
  points, matching existing string indexing, rather than exposing Go byte
  offsets.
- A missing index returns `none()` and a present index returns `some(u64)`;
  do not introduce a `-1` sentinel.
- `cut` returns `some([before, after])` when the separator is present and
  `none()` otherwise. Prefix/suffix cuts return an `option<string>`.
- Define empty-needle behavior for every search/count/split operation.
- `split_n` and `split_after_n` define `n < 0`, `n == 0`, and positive `n`
  explicitly, using an Osiris integer width that cannot overflow host
  allocation sizes silently.
- `contains_any`, `index_any`, and `last_index_any` treat the second argument
  as a set of Unicode code points, not a regex or a byte set.
- Case-insensitive equality uses documented Unicode simple folding and is
  locale-independent.
- Invalid arity and kinds produce stable structured errors.

## Execution paths

- Add exports to `std/text`; avoid unnecessary global aliases.
- Share semantics between interpreter and bytecode VM.
- Extend inference for `bool`, `u64`, `option`, `list`, and `string` results.
- Native lowering may use C helpers only if they implement the same UTF-8
  code-point semantics and ownership. Otherwise reject the call at compile
  time; byte-oriented `strstr`/`strlen` behavior is not sufficient evidence.

## Documentation

- Add an individual canonical `std/text` entry for every function with
  accepted values, return type/value, Unicode and empty-input behavior,
  structured errors, and a checked example.
- Update global search, backend support, and the Technical Reference complete
  surface indexes.
- Include examples with ASCII, multibyte Unicode, empty strings, repeated
  separators, and absent matches.

## Required tests

- table-driven ordinary, empty, and boundary cases for every function;
- multibyte Unicode cases proving code-point indexes rather than byte offsets;
- Unicode simple-fold cases for `equal_fold`;
- split limits, trailing separators, repeated separators, and empty needles;
- option result shapes for search and cut operations;
- interpreter/VM parity and inference assertions;
- native generated-C/executable parity where supported, plus clear compile
  diagnostics elsewhere;
- executable documentation examples and search coverage;
- fuzz properties for functions with natural invariants, such as successful
  `cut` reconstruction and split/join round trips.

## Acceptance criteria

- The required operations are available through `std/text` with one explicit
  UTF-8/code-point contract.
- Missing results use `option` rather than undocumented integer sentinels.
- Interpreter, VM, inference, and every advertised native path agree.
- Each public function is individually documented and searchable in both the
  canonical expanded reference and technical surface audit.
- Focused, fuzz/property, documentation, and full validation pass.

## Non-goals

- No duplicate `std/strings` module.
- No locale-sensitive collation, normalization, grapheme-cluster segmentation,
  regex engine, mutable builder, streaming reader, or callback-based
  `IndexFunc`/`Map` family in this task.
- No byte-offset API disguised as text indexing.

## Completion record

Completed 2026-07-29.

### Decisions

- All additions remain qualified exports of `std/text`; no global aliases or
  competing module were added. The existing module version and its ten prior
  exports remain compatible.
- Strings remain immutable UTF-8 text. `index`, `index_any`, `last_index`, and
  `last_index_any` convert internal UTF-8 positions to zero-based Unicode
  code-point indexes and return `option<u64>`.
- Empty substrings follow one consistent boundary model: `contains` succeeds;
  `count` returns one plus the code-point length; first index is zero; last
  index is the code-point length; `cut` returns `some(["", value])`; and split
  operations divide at code-point boundaries.
- The `*_any` and left/right trim operations interpret their second string as
  a Unicode code-point set. `equal_fold` uses locale-independent Unicode
  simple folding and deliberately does not add normalization or collation.
- Limited split/replacement calls accept signed, unsigned, and big integers,
  plus exact integral default-number literals so ordinary `-1` expresses the
  documented unlimited mode. Limits must fit the host allocation index width;
  replacement/repeat result growth is checked before allocation.
- New and preserved `std/text` operations report stable structured
  `invalid_argument`, `type_error`, and `out_of_range` failures. Runtime and VM
  attach the call source span.
- Importing `std/text` is explicitly unsupported by native C and now fails at
  the import source location. No byte-oriented C helper or compile-time
  evaluator fallback was introduced.

### Implementation and evidence

- `internal/osiris/text.go` implements the 24 new comparison, search, cut,
  tokenization, trim, and bounded-replacement operations with shared Unicode,
  option, argument, and allocation helpers.
- `internal/osiris/module.go` publishes all 34 `std/text` exports. The
  interpreter and bytecode VM invoke the same immutable function values.
  Inference now reports `bool`, `i64`, `u64`, `option`, `list`, and `string`
  results for qualified calls.
- `internal/osiris/text_test.go` has table-driven interpreter/VM checks for
  every addition plus empty, repeated-separator, trailing, absent, and
  multibyte cases; structured kind/span checks; inference; generated-IR
  assertions; and the source-located native gate.
- `FuzzStdTextCutReconstruction` proves successful cuts reconstruct the source.
  `FuzzStdTextSplitJoinRoundTrip` proves non-empty-separator split/join
  round-trips. Three-second runs executed 260,711 and 257,392 inputs
  respectively without a failure.
- The canonical `std/text` page now gives every existing and new export an
  individual signature, accepted values, return type, Unicode/empty-input
  behavior, structured errors, and executable example. The Technical
  Reference has an exact implementation-checked `std/text` export index, and
  search checks cover the new vocabulary.

### Validation

Environment:

- macOS arm64
- Go `go1.26.1`
- Node `v24.12.0`
- pnpm `10.30.1`
- Apple clang `15.0.0`

Passing commands:

- `go test ./internal/osiris -run 'TestStdText' -count=1`
- `go test ./internal/osiris -count=1`
- `go test ./internal/osiris -run '^$' -fuzz '^FuzzStdTextCutReconstruction$' -fuzztime=3s`
- `go test ./internal/osiris -run '^$' -fuzz '^FuzzStdTextSplitJoinRoundTrip$' -fuzztime=3s`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `pnpm run check-reference` in `newdocs/` (`17` API groups, `6` chapters,
  `227` built-ins, `19` special forms, `375` named constants, `17` redirects)
- `pnpm run check-examples` in `newdocs/`
- `pnpm run build` in `newdocs/`
- `git diff --check`
