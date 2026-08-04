# Task 137: Expand core standard-library modules

**Priority:** P1
**Dependencies:** 100, 103, 120, 123
**Status:** Complete

## Objective

Substantially expand `std/list`, `std/signal`, `std/crypto`, `std/regex`,
`std/json`, and `std/i18n` from their current minimal surfaces into cohesive,
production-useful Osiris libraries. Every new public symbol must have explicit
arguments, return values, limits, errors, determinism guarantees, interpreter/
VM parity, and granular public reference documentation.

## Required surface

- `std/list`: construction, slicing, searching, stable ordering, mapping,
  filtering, folding, flattening, deduplication, partitioning, zipping,
  windowing, chunking, and bounded sequence predicates.
- `std/signal`: window families, correlation, filtering, resampling,
  normalization, energy/statistics, FFT/IFFT-compatible transforms where the
  existing numeric representation can preserve exact documented semantics, and
  bounded signal-shape validation.
- `std/crypto`: versioned hash/digest families, constant-time comparisons,
  encoding/decoding helpers, deterministic keyed derivation, UUID/token helpers,
  and explicit random/seeding contracts without ambient mutable state.
- `std/regex`: compiled descriptors, capture/name inspection, match indexes,
  find-all helpers, split/replace variants, escaping, quote/meta helpers, and
  explicit pattern/input/output limits.
- `std/json`: type predicates, object/array access and construction,
  pointer/query helpers, merge/patch utilities, canonical/pretty encoders,
  schema helpers, and bounded streaming-oriented fragments compatible with the
  existing JSON value model.
- `std/i18n`: locale discovery, language fallback, plural categories, list and
  relative-time formatting, currency/percent formatting, date/time styles,
  message catalogs, and deterministic locale metadata.

## Acceptance criteria

- [x] Existing APIs remain compatible and all new exports are versioned,
      bounded, deterministic, and cancellation-aware where work is iterative.
- [x] Interpreter and VM behavior matches for every new supported symbol.
- [x] Native backend support is implemented or explicitly rejected with a
      source-spanned capability/support diagnostic; no evaluator fallback is
      introduced.
- [x] Focused unit, error-boundary, limit, cancellation, and interpreter/VM
      parity tests cover every public family.
- [x] Every implementation export has one granular reference entry containing
      syntax, parameters, return type/value, errors, limits, behavior, and an
      example.
- [x] Reference and navigation checks discover every module and symbol without
      fixed-count or manually duplicated sidebar assumptions.
- [x] Race-sensitive focused tests, vet/build, documentation checks, checked
      examples, and production docs build pass. The repository-wide suite has
      one unrelated pre-existing failure in
      `TestCloseDispatchesTheClosableResourceProtocol/vm` (`expected stream,
      got struct`); the task-137 package and focused tests pass independently.

## Completion evidence

Implementation and validation evidence:

- Added bounded, owned list construction/access/search/order/shape/predicate
  operations and metadata through `task137ListModule`.
- Added deterministic signal windows, correlation, FIR filtering, resampling,
  normalization, statistics, DFT/IDFT, and sample limits through
  `task137SignalModule`.
- Added SHA-256/SHA-512/SHA-512/256 digest/HMAC, constant-time comparison,
  hex/Base64 codecs, deterministic derivation/token/UUID helpers, and explicit
  algorithm metadata through `task137CryptoModule`.
- Added bounded RE2 capture inspection, byte indexes, named matches, bounded
  replacement/splitting, pattern metadata, and escaping through
  `task137RegexModule`.
- Added JSONPath queries, pointer existence, structural equality/merge, JSON
  Lines, schema validation helpers, and module adapters through
  `Task137JSONModule`.
- Added six embedded locale datasets, plural/list/currency/percent/relative
  time/date style formatting, and plural message catalog formatting through
  `Task137I18nModule`.
- Added interpreter/VM smoke parity coverage in
  `internal/osiris/task137_stdlib_test.go` plus focused family tests.
- Added one reference entry per module export, generated export coverage in
  `docs/scripts/check-reference.mjs`, and generated navigation merging so new
  symbols cannot be omitted from the sidebar.
- Verified with:
  `go test ./internal/osiris -run '^TestTask137' -race -count=1`,
  `go vet ./internal/osiris`, `go build ./...`, `git diff --check`,
  `docs/scripts/check-reference.mjs`, `docs/scripts/check-navigation.mjs`,
  and the Node 24 production docs build.
