# Time values and `std/time`

Status: accepted for task 101

## Values

Osiris has three immutable time-related runtime kinds:

- `duration` is a signed 64-bit count of nanoseconds. Its range is
  `-9223372036854775808` through `9223372036854775807` nanoseconds, matching
  Go's `time.Duration`. Construction, addition, subtraction, negation, and
  rounding report `OutOfRange` instead of wrapping or saturating.
- `datetime` is an instant stored as a signed Unix-second count plus a
  nanosecond field in `0..999999999`, together with a `location`. Public
  constructors and decoding restrict the civil year to `0001..9999`.
- `location` is UTC, a named fixed offset, or an IANA timezone name. A
  datetime retains this identity across arithmetic, conversion, the binary
  codec, and JSON serialization.

Durations are false only when they are zero. Datetimes and locations are
always true. Duration equality and ordering compare nanoseconds. Datetime
equality and ordering compare instants; location identity does not make two
representations of the same instant unequal. Locations compare by kind, name,
and fixed offset.

The display forms are `duration("1h2m3s")`, an RFC 3339 Nano datetime followed
by its IANA name when applicable, and `location("name")` or
`fixed_zone("name", offset)`. Inference reports the canonical names
`duration`, `datetime`, and `location`.

## Calendar and epoch rules

Civil construction uses the proleptic Gregorian calendar. `date` accepts a
year in `1..9999`, month in `1..12`, a day valid for that year and month,
hour in `0..23`, minute and second in `0..59`, and nanosecond in
`0..999999999`. Invalid fields fail; they are never normalized. Leap-second
input (`second == 60`) is rejected because the representation follows Unix
time and has no leap-second table.

`unix(seconds, nanoseconds)` accepts a signed integral second count and an
integral nanosecond adjustment. The adjustment is normalized into the second
field before the supported-year check. `unix_milli` and `unix_micro` perform
exact signed integer conversion. Negative epoch values use floor-based Unix
semantics, so the nanosecond field remains non-negative.

`add_date` follows Go's `time.AddDate` calendar normalization after validating
its integral year, month, and day deltas. This includes month-end behavior:
adding one month to October 31 produces December 1. Elapsed-duration
arithmetic uses exact nanoseconds and therefore remains distinct from calendar
arithmetic across daylight-saving transitions.

## Locations and daylight-saving transitions

UTC is canonical. A fixed offset is an exact signed number of seconds strictly
between `-24h` and `+24h`, with a non-empty display name. IANA locations are
loaded by name from the timezone database embedded into the Osiris executable
through Go's `time/tzdata` package. Builds therefore do not require the host's
zoneinfo installation. The database version is the snapshot shipped by the Go
toolchain used to build that Osiris executable and is exposed in build
provenance rather than silently fetched at runtime.

`date` and location-aware parsing reject nonexistent local times in a
daylight-saving gap. When a civil time occurs twice in a fold, Osiris selects
the earlier instant. Conversion with `in_location` never changes the instant.
A missing or malformed IANA name produces `NotFound` or `InvalidArgument`.
Tests use either fixed zones or a named zone from the embedded snapshot and
never depend on a host-only timezone database.

## Parsing and formatting

The module uses Go reference-time layouts. A layout describes the desired
shape by writing the specific reference instant
`Mon Jan 2 15:04:05 MST 2006` in that shape. For example,
`2006-01-02 15:04:05` describes a four-digit year, numeric month and day, and
24-hour clock. The constants `time.rfc3339` and `time.rfc3339_nano` provide
the common layouts.

`parse(layout, text)` parses offsets contained in the text and otherwise uses
UTC. `parse(layout, text, location)` interprets zone-less civil fields in the
given location and applies the gap/fold rule above. `format(datetime, layout)`
formats in the datetime's retained location. Parse failures are structured
`ParseError` values. Parsing never consults the clock.

## Arithmetic and rounding

`add` accepts two durations or a datetime and duration. `sub` accepts two
durations or two datetimes. `round` and `truncate` accept either a duration or
a datetime plus a positive duration quantum; a zero quantum returns the input,
and a negative quantum is `InvalidArgument`. Ties round away from zero for
durations. Datetime rounding is measured from the zero time
`0001-01-01T00:00:00Z`, matching Go's `time.Time.Round` convention, and must
remain in the supported year range.

Every arithmetic path checks its result before constructing a value. No time
operation silently wraps, saturates, loses fractional nanoseconds, or changes
location identity.

Four names have an intentional value/call-position rule because the required
component names and qualified unit names coincide. `time.nanosecond`,
`time.second`, `time.minute`, and `time.hour` are duration constants in value
position; calling one with a datetime returns that component. The rule is
derived from the exact duration value rather than hidden mutable metadata, so
it behaves identically after a codec round trip. `time.microsecond` and
`time.millisecond` are constants only.

## Clock boundary

All operations except `now` are deterministic and pure. `now()` reads the
clock installed on the owning `Runtime`; a new runtime defaults to
`time.Now`. `Runtime.SetClock` replaces that source for a session or test.
The returned datetime is normalized to UTC, contains no monotonic-clock
payload, and is validated against the public year range. Module caching does
not snapshot the current time.

Timers, tickers, sleeping, task deadlines, scheduler integration, mutable
local timezone state, and natural-language or locale-sensitive parsing are
outside this API.

## Serialization

The public binary codec uses dedicated tags:

- duration: signed nanoseconds;
- location: location class, UTF-8 name, and fixed offset when present;
- datetime: signed Unix seconds, nanoseconds, and the encoded location.

Decoding validates canonical nanoseconds, offset limits, IANA availability,
and the supported civil-year range. JSON uses tagged objects with decimal
strings for signed 64-bit fields so JavaScript consumers cannot lose
precision. A datetime's location identity is included, rather than being
reduced to an offset-only timestamp.

## Backends

The interpreter and bytecode VM use the same module values and runtime
operations. The formatter preserves ordinary `std/time` import and call
syntax. The inference pass reports the three exact runtime kinds and the
documented result kinds of module calls.

The native C backend does not implement these values in this release. Any
program that imports or otherwise reaches `std/time` is rejected during native
compilation with a source-aware unsupported-feature diagnostic. Native
compilation must never execute `now`, parse a datetime, or precompute another
time expression.
