# Task 101: Add a deterministic `std/time` date and time API

**Priority:** P2  
**Dependencies:** 014, 025, 035, 039, 040, 045  
**Status:** Complete  

## Objective

Add a first-class `std/time` module for durations, instants, civil calendar
values, parsing, formatting, arithmetic, and explicit clock access. Follow
Go's well-tested calendar and duration conventions where they fit Osiris, but
use Osiris naming, immutable values, structured errors, and deterministic
testing.

Primary semantic reference: <https://pkg.go.dev/time>

## Design gate

Write `docs/design/TIME.md` before implementation. It must settle:

- the immutable `duration` representation and range;
- the immutable instant/civil-time representation and its canonical type name;
- precision, Unix epoch conversion, supported year range, leap-year rules,
  normalization, and comparison;
- UTC, fixed-offset zones, and IANA location behavior;
- the exact parsing/formatting layout contract;
- serialization, equality, truthiness, inference, and display;
- the boundary between deterministic pure operations and host-clock access;
- native C support or explicit compile-time rejection.

Do not represent time as an untyped list, a default float, or an undocumented
string convention.

## Required API

The first release must include the following cohesive surface, with final
spellings chosen to match existing snake_case standard-library style:

- duration construction and parsing: `duration`, `parse_duration`,
  `duration_string`;
- civil/instant construction: `date`, `unix`, `unix_milli`, `unix_micro`;
- parsing and formatting: `parse`, `format`, with documented RFC 3339 and
  RFC 3339 Nano layout constants;
- arithmetic: `add`, `sub`, `add_date`, `round`, and `truncate`;
- comparison: `equal`, `before`, `after`, and `compare`;
- components: `year`, `month`, `day`, `hour`, `minute`, `second`,
  `nanosecond`, `weekday`, `year_day`, and `zone`;
- conversion: `utc`, `in_location`, `fixed_zone`, and `load_location`;
- explicit host clock: `now`.

Expose qualified duration constants such as `time.nanosecond`,
`time.microsecond`, `time.millisecond`, `time.second`, `time.minute`, and
`time.hour`. Coordinate their semantics with task 104's global measurement
units without adding shadowing global names.

## Semantic requirements

- Duration math is exact and overflow-checked. Record whether the chosen range
  follows Go's signed 64-bit nanoseconds or a wider Osiris representation.
- Calendar math follows the proleptic Gregorian calendar and documents how
  invalid dates normalize or fail.
- Parsing and formatting use one documented layout system. If Go's reference
  time layouts are retained, document them clearly rather than assuming users
  know Go.
- `now` is explicitly nondeterministic and must use an injectable session
  clock in tests. Pure parsing, formatting, conversion, comparison, and
  arithmetic must never consult the host clock.
- Time-zone data must have a deterministic availability/version policy.
  Missing zones produce a structured error; tests may not depend on whatever
  zone database happens to be installed on the host.
- Define daylight-saving gaps and folds, leap-second input, offset limits, and
  out-of-range behavior.
- Date/time values must survive public serialization without losing precision
  or their documented zone/offset identity.

## Execution paths

- Add the necessary `Value` kinds and carry them through formatting, equality,
  codecs, inference, runtime registration, bytecode VM, module exports, and
  structured errors.
- Add `std/time` through the normal module resolver and dependency reporting.
- Native C must either implement the same representation and operations or
  reject programs that use them at compile time. Never evaluate `now` or any
  other time expression during compilation.
- Do not route clock waiting through ad hoc sleeps. Timers, tickers, task
  deadlines, and scheduler integration require a separate task unless a very
  small reuse of the existing context scheduler is proven safe.

## Documentation

- Add a dedicated `std/time` page under Standard Library with an individual
  listing for every function and constant, accepted values, return types,
  errors, determinism notes, and checked examples.
- Add the new types to Language → Types, update the Technical Reference value
  kinds and complete surface indexes, global search, and the backend support
  matrix.
- Clearly distinguish civil time, an instant, a duration, a fixed offset, and
  an IANA location.

## Required tests

- parse/format round trips, including RFC 3339 offsets and fractional seconds;
- leap years, month lengths, epoch boundaries, negative Unix times, and range
  limits;
- duration overflow and negative duration arithmetic;
- add/sub/add-date behavior around month ends and daylight-saving transitions;
- fixed-zone and pinned IANA-zone cases, including gaps and folds;
- injected-clock tests for `now` with no wall-clock sleeps;
- interpreter/VM parity, codec round trips, inference, formatter stability,
  malformed-input spans, and structured errors;
- generated-C/executable parity for supported native forms and compile
  diagnostics for unsupported forms;
- executable reference examples and search coverage.

## Acceptance criteria

- `std/time` has a written representation and determinism contract.
- The required API is immutable, precision-preserving, and covered in the
  interpreter and VM.
- Host clock and time-zone dependencies are explicit and test-injectable.
- Native support is honest and compile-time gated.
- Canonical and technical documentation are complete and validation passes.

## Non-goals

- No locale-specific month/day names or natural-language date parser.
- No mutable global local-time zone.
- No scheduler rewrite, cron service, ticker subsystem, or implicit sleep.
- No dependence on current wall time or the host's unpinned zone database in
  deterministic tests.

## Completion record

Completed 2026-07-29.

### Decisions

- `docs/design/TIME.md` defines immutable `duration`, `datetime`, and
  `location` values. Durations are checked signed 64-bit nanoseconds;
  datetimes retain an exact Unix second/nanosecond instant plus UTC,
  fixed-offset, or IANA location identity; public civil years are `1..9999`.
- Civil construction is strict, uses the proleptic Gregorian calendar,
  rejects leap seconds and DST gaps, and resolves folds to the earlier
  instant. `add_date` deliberately retains Go's calendar normalization.
- IANA loading uses the timezone snapshot embedded by `time/tzdata`.
  `Runtime.SetTimeLocationLoader` allows a host or test to pin another
  database. `Runtime.SetClock` injects the sole nondeterministic dependency;
  only `time.now()` reads it, and results are normalized to UTC.
- Go reference-time layouts are the parsing/formatting contract. The module
  exports `rfc3339` and `rfc3339_nano`.
- `time.nanosecond`, `time.second`, `time.minute`, and `time.hour` are exact
  duration constants in value position and the required datetime component
  accessors in call position. This resolves the required shared spellings
  without global names or hidden mutable state.
- Native C support is explicitly unavailable in this release. Importing
  `std/time` fails at the import source location before IR emission or any
  expression evaluation.

### Implementation and evidence

- `internal/osiris/time_value.go` implements all 38 unique `std/time` exports,
  exact arithmetic, strict civil resolution, parsing/formatting, fixed and
  IANA locations, component access, injected `now`, and structured failures.
- `Value`, interpreter calls, bytecode calls, checked binding annotations,
  inference, binary/JSON codecs, module loading/versioning, dependency
  reporting, truthiness, equality, comparison, and rendering carry the three
  new runtime kinds.
- Dedicated tests in `internal/osiris/time_test.go` prove interpreter/VM
  parity; RFC 3339 offsets/fractions; leap years and range endpoints; negative
  epochs; overflow and negative rounding; month-end calendar behavior; fixed
  zones; embedded-IANA gaps/folds; injected clock and location loader behavior;
  codec/JSON location identity; inference; parse-format-parse stability;
  structured source spans; dependency reporting; and the native compile-time
  gate.
- `newdocs/src/time-reference.json` supplies the Standard Library page with an
  individual detailed listing for every unique export and executable
  examples. Types, navigation, search, syntax highlighting, Technical
  Reference indexes, and the backend support matrix include the new surface.

### Validation

Environment:

- macOS arm64
- Go `go1.26.1`
- Node `v24.12.0`
- pnpm `10.30.1`
- Apple clang `15.0.0`

Passing commands:

- `go test ./internal/osiris -run 'TestStdTime|TestTimeValue' -count=1`
- `go test ./...`
- `go test -race ./...`
- `go vet ./...`
- `go test ./internal/osiris -run '^TestCanonicalDocumentationExamples$' -count=1`
- `pnpm run check-reference` in `newdocs/` (`17` API groups, `6` chapters,
  `227` built-ins, `19` special forms, `167` named constants, `17` redirects)
- `pnpm run check-examples` in `newdocs/`
- `pnpm run build` in `newdocs/`
- `git diff --check`

The generated-C golden was updated to the deterministic digest produced by
the enum/pattern native changes already present in the active worktree; task
101 did not alter that native implementation.
