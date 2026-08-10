# Task 163: Close native C standard-library and host matrix rows

**Priority:** P0
**Dependencies:** 148,162
**Status:** Blocked

## Objective

Close the native C matrix rows for std/text, std/math, std/crypto, std/list,
std/signal, std/time, std/os, std/net, I/O, process/environment, FFI, and host
capabilities.

## Acceptance criteria

- Every claimed member has an individual signature/ownership/capability test,
  native execution evidence, and granular reference documentation.
- Files, sockets, processes, environment mutation, deadlines, cancellation,
  and FFI handles have explicit ownership and cleanup evidence.
- Native sanitizer and cross-platform compile/execution evidence is recorded;
  matrix cells remain non-stable when a platform cannot provide the provider.

## Current implementation evidence

The native standard-library inventory and generated C dispatcher now admit and
execute `std/list.sum(values)` and `std/list.product(values)` as individual
members. The implementation is split across `nativeStaticModuleMember`,
`nativeModuleMemberStatus`, `ostd_dispatch`, `ostd_list_fold`,
`ostd_list_sum`, and `ostd_list_product`. Numeric and complex reductions retain
the widest floating-point family observed in the input list.

`TestNativeLibraryListReductionParity` in
`internal/osiris/native_core_library_parity_test.go` compares integer, `f64`,
and complex sum/product behavior across interpreter, VM, and generated C
execution. Existing task 145, 146, and 147 native parity suites provide the
broader standard-library witnesses.

The same native dispatcher now also admits `std/list.flatten(values, depth)`.
Its C path is implemented by `ostd_list_flatten_append` and
`ostd_list_flatten`, with the same 64-level depth and 100,000-item bounds used
by the staged list API. `TestNativeLibraryListFlattenParity` covers depth zero,
one, and two across interpreter, VM, and generated C. The VM shape validator
recognizes this two-argument list member separately from the one-argument
tensor `flatten` builtin, so the differential test reaches execution instead
of failing during preflight.

The native dispatcher and inventory now carry a bounded pure-text slice:
`std/text.trim(value)`, `std/text.trim_space(value)`,
`std/text.lower(value)`, `std/text.upper(value)`,
`std/text.fields(value)`, `std/text.join(separator, values)`,
`std/text.contains(value, needle)`, `std/text.has_prefix(value, prefix)`,
`std/text.has_suffix(value, suffix)`, `std/text.compare(left, right)`,
`std/text.equal_fold(left, right)`, `std/text.count(value, needle)`,
`std/text.contains_any(value, code_points)`,
`std/text.index(value, needle)`, `std/text.last_index(value, needle)`, and
`std/text.index_any(value, code_points)`,
`std/text.last_index_any(value, code_points)`, and
`std/text.cut(value, separator)`, `std/text.cut_prefix(value, prefix)`,
`std/text.cut_suffix(value, suffix)`, `std/text.trim_prefix(value, prefix)`,
`std/text.trim_suffix(value, suffix)`, and `std/text.repeat(value, count)`.
The exact admission and dispatch symbols are
`nativeStaticModuleMember`, `nativeModuleMemberStatus`, `ostd_dispatch`,
`ostd_text_case`, `ostd_text_fields`, `ostd_text_join`,
`ostd_text_equal_fold`, `ostd_text_count`, `ostd_text_index`, and the owned
repeat path in `ostd_dispatch`. `TestNativeLibraryTextParity` compares all
twenty-three signatures and values across interpreter, VM, generated C, and the
executable. `TestNativeLibraryTextOwnershipCapabilityAndErrorContract` checks
that string/list/option-producing helpers use the generated owned-value
boundary, malformed and oversized inputs preserve the structured error
contract, and the pure operations do not require an OS capability. The exact
contracts are documented individually in `docs/design/NATIVE-C.md` under
“Native std/text boundary contracts” and in the public `std/text` reference.

This is not task completion. The task remains **Blocked** pending individual
signature/ownership/capability evidence for the remaining standard-library and
host surfaces, sanitizer and cross-platform generated-C evidence, and
provider-dependent implementations. No support-matrix status is changed by
this progress update.

## Current implementation evidence: complete pure `std/signal` surface

The Native C signal cell is now independently closed for the complete exported
module surface. `nativeStaticModuleMember`, `emitCInstr`, and
`nativeModuleMemberStatus` admit and lower the four module values
(`version`, `max_samples`, `max_transform_samples`, and `pi`) together with all
sixteen exported operations, including `resample`. The values use direct
constant lowering; signal inputs are bounded and staged into owned temporary
storage, scalar results stay by value, and tensor results transfer newly owned
storage. No member defines or requires an OS capability.

`TestNativeLibrarySignalPureParity` now exercises the module values, every
window/filter/statistic/transform operation, and resampling across the
interpreter, VM, generated C, and native executable. Its ownership companion
checks every individual inventory member and the direct helper/error boundary;
`TestNativeLibrarySignalResampleParity` and
`TestNativeLibrarySignalResampleOwnershipErrorAndCapabilityContract` retain the
focused interpolation, bound, cleanup, and allocation evidence. The public
support matrix marks only the Native C `std/signal` cell stable; WASM, GPU, and
LSP statuses are unchanged.

Focused normal, race, LLVM AddressSanitizer, and LLVM UndefinedBehaviorSanitizer
execution of the updated Native C signal fixtures passed on the local macOS
host. The granular contracts are documented in `docs/design/NATIVE-C.md` and
the searchable `std/signal` reference.

## Current implementation evidence: complete pure `std/text` surface

The Native C text cell is now independently closed for every exported
`std/text` member. The admission table, backend inventory, dispatcher, and
direct `chunks` lowering cover trimming, case conversion, predicates, Unicode
search, cut/option results, replacement, splitting, slugging, concatenation,
joining, and bounded repetition. `TestNativeLibraryTextParity` now calls the
complete module surface, including `trim`, `trim_space`, `lower`, `contains`,
prefix/suffix predicates, `slug`, `cat`, and `chunks`; its ownership companion
checks every inventory member, owned-result helper, exact error boundary, and
capability absence. The existing trim-set and chunks fixtures remain separate
focused witnesses.

The public support matrix marks only Native C `std/text` stable. The complete
per-symbol signatures and ownership rules are documented in
`docs/design/NATIVE-C.md` and the generated `std/text` reference; WASM, GPU,
and LSP statuses are unchanged.

## Current implementation evidence: `std/text.chunks` direct lowering

The Native C emitter now directly lowers the bounded pure member
`std/text.chunks(value: string, size: integer) -> list<string>` after static
admission. The exact compiler path is `emitCInstr` in
`internal/osiris/cbackend_native.go` calling `ostd_text_chunks` from
`internal/osiris/cbackend_stdlib_runtime.go`; it does not route this call
through `ostd_dispatch`.

`ostd_text_chunks` uses `ostd_text_arg` for the shared 1 MiB UTF-8 input bound,
requires a positive exact integer width, decodes widths by Unicode code point,
and copies every returned chunk through `oownedstring` before constructing the
tracked `olist`. It never retains the input pointer and defines no host
capability. Wrong kinds, malformed UTF-8, non-positive or non-integral widths,
and allocation failures remain structured native errors.

`TestNativeLibraryTextChunksParity` in
`internal/osiris/native_library_text_chunks_test.go` compares interpreter, VM,
generated C, and native executable output for multibyte widths, empty input,
and caught invalid arguments. The companion
`TestNativeLibraryTextChunksOwnershipErrorAndCapabilityContract` checks the
direct call marker, `ostd_text_arg`/`oownedstring`/`olist` ownership boundary,
error markers, capability absence, and the individual
`module/std/text/chunks` inventory entry. The granular public reference entry
is in `docs/src/reference-data.json`, the searchable API entry is in
`docs/src/std-task-reference.js`, and the design contract is in the Native
`std/text` table in `docs/design/NATIVE-C.md`. This evidence is part of the
complete pure `std/text` closure. The public Native C `std/text` cell is
stable; task 163 remains blocked by the other standard-library and
host-resource surfaces.

Focused validation for this slice passed:

```text
GOCACHE=/tmp/osiris-text-chunks-cache go test ./internal/osiris -run '^TestNativeLibraryTextChunks' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  0.918s
GOCACHE=/tmp/osiris-text-chunks-race-cache go test -race ./internal/osiris -run '^TestNativeLibraryTextChunks' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  2.072s
GOCACHE=/tmp/osiris-text-chunks-all-cache go test ./internal/osiris -run '^TestNativeLibraryText' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  5.494s
node docs/scripts/check-reference.mjs
Reference checks passed: 26 API groups, 6 chapters, 315 built-ins, 19 special forms, 375 named constants.
```

The Go toolchain's native `-asan` mode is unavailable on this darwin/arm64
host; the equivalent production LLVM AddressSanitizer command is recorded in
the complete text evidence above and passes locally.

## Current implementation evidence: `std/signal` pure-value slice

The portable Native C runtime now closes the bounded pure signal subset for
these individual members:

- `std/signal.rectangular(length: u64) -> tensor[length]`
- `std/signal.hann(length: u64) -> tensor[length]`
- `std/signal.hamming(length: u64) -> tensor[length]`
- `std/signal.blackman(length: u64) -> tensor[length]`
- `std/signal.bartlett(length: u64) -> tensor[length]`
- `std/signal.correlate(left: signal, right: signal) -> tensor`
- `std/signal.filter_fir(signal: signal, coefficients: signal) -> tensor`
- `std/signal.mean(signal: signal) -> number`
- `std/signal.variance(signal: signal) -> number`
- `std/signal.energy(signal: signal) -> number`
- `std/signal.rms(signal: signal) -> number`
- `std/signal.normalize(signal: signal, mode: string) -> tensor`
- `std/signal.fft(signal: signal) -> tensor<complex>`
- `std/signal.ifft(spectrum: signal) -> tensor<complex>`

`ostd_signal_data` copies rank-one list/tensor samples into temporary numeric
storage. Window construction, convolution/filtering, statistics, and
normalization return scalar values or newly owned `otensordata` results. Native
variance now uses the same two-pass population formula as the reference
runtime. Native normalization implements `peak`, `minmax`, and `zscore`, with
zero peak/span/deviation producing zero-valued output just like the reference
runtime. `ostd_signal_complex_data` separately stages real tensors, complex
tensors, and numeric lists for the bounded explicit DFT. `ostd_signal_fft`
returns `ocomplextensordata` rather than a list, applies the 4096-sample
transform bound before allocation, and releases all temporary input storage.
No operation uses a host capability or retains an input pointer.

`TestNativeLibrarySignalPureParity` compares windows, filtering, correlation,
statistics, normalization, and forward/inverse transforms across interpreter,
VM, generated C, and native execution. `TestNativeLibrarySignalOwnershipCapabilityAndErrorContract`
checks the helper symbols, complex-tensor ownership boundary, capability
absence, structured errors, and each individual backend-inventory entry.
The focused `std/signal.resample` slice is now admitted by
`nativeStaticModuleMember`, reported as implemented by
`nativeModuleMemberStatus`, and routed through `ostd_dispatch` to
`ostd_signal_resample`. The C helper validates rank-one real input and the
`0..65536` input/output bounds, stages input through `ostd_signal_data`, and
matches the reference interpolation behavior for empty input, singleton input,
zero output length, one output element, and endpoint preservation. It releases
staged input before every returned tensor or structured error. The semantic
fixtures `TestNativeLibrarySignalResampleParity` and
`TestNativeLibrarySignalResampleOwnershipErrorAndCapabilityContract` provide
interpreter/VM/generated-C/executable differential evidence, individual
ownership/error checks, no-capability evidence, and the inventory assertion.
Focused normal and race runs passed with the unrelated untracked LSP test file
excluded through a temporary Go overlay (the LSP file itself was not edited):

```text
GOCACHE=/tmp/osiris-signal-resample-overlay-cache go test -overlay=/private/tmp/osiris-signal-overlay.json ./internal/osiris -run 'TestNativeLibrarySignalResample(Parity|OwnershipErrorAndCapabilityContract)$' -count=1 -timeout=1200s
GOCACHE=/tmp/osiris-signal-resample-overlay-race-cache go test -race -vet=off -overlay=/private/tmp/osiris-signal-overlay.json ./internal/osiris -run 'TestNativeLibrarySignalResample(Parity|OwnershipErrorAndCapabilityContract)$' -count=1 -timeout=1200s
```

Both commands completed successfully. A direct no-overlay rerun is currently
blocked by the unrelated untracked `internal/osiris/lsp_call_hierarchy_test.go`,
which uses `upsertDocument` as though it returned a value; that file remains
outside this slice and untouched.
The broader task remains **Blocked**; this bounded member does not close the
remaining standard-library, host, sanitizer, or cross-platform criteria.

This task remains **Blocked**; the signal slice does not close the remaining
standard-library, host, sanitizer, or cross-platform criteria.

## Current implementation evidence: `std/time` pure-value slice

The portable generated-C runtime now implements these individual UTC-only
members:

- `std/time.compare(left: duration | datetime, right: duration | datetime) -> i64`
- `std/time.equal(left: duration | datetime, right: duration | datetime) -> bool`
- `std/time.before(left: duration | datetime, right: duration | datetime) -> bool`
- `std/time.after(left: duration | datetime, right: duration | datetime) -> bool`
- `std/time.year(value: datetime) -> u64`
- `std/time.month(value: datetime) -> u64`
- `std/time.day(value: datetime) -> u64`
- `std/time.weekday(value: datetime) -> string`
- `std/time.year_day(value: datetime) -> u64`

`ostd_time_compare_values` compares same-kind scalar payloads without
subtraction, avoiding overflow at signed 64-bit limits. `ostd_time_predicate`
provides the three boolean relations. `ostd_time_civil` normalizes negative
nanoseconds and uses the portable civil-day conversion; `ostd_time_component`
returns value-owned unsigned components or one of seven static weekday names.
No helper reads the clock, loads timezone data, retains an input pointer, or
requires an OS capability. Wrong arity, wrong kinds, and mixed duration/
datetime comparisons return structured native errors.

`TestNativeLibraryTimePureValueParity` compares the interpreter, VM, generated
C, and native executable for comparison, leap-day accessors, weekday, and
caught invalid-kind/mixed-kind errors. The companion
`TestNativeLibraryTimePureValueOwnershipCapabilityAndErrorContract` checks
the exact C symbols, error strings, absence of `OSIRIS_CAP_*`, and individual
backend-inventory entries. The exact public signatures and native boundary are
documented in `docs/src/time-reference.json` and the task-163 section of
`docs/design/NATIVE-C.md`. The task remains blocked and the support-matrix
status remains partial because location providers, parsing, `now`, timers,
and the remaining stdlib/host families are outside this slice.

## Current implementation evidence: `std/time` clock-component slice

The native dispatcher now admits and executes four additional pure UTC
component accessors individually:

- `std/time.hour(value: datetime) -> u64`
- `std/time.minute(value: datetime) -> u64`
- `std/time.second(value: datetime) -> u64`
- `std/time.nanosecond(value: datetime) -> u64`

The exact C boundary is `ostd_dispatch` -> `ostd_time_component` ->
`ostd_time_civil`. The helper returns a value-owned unsigned scalar after the
existing normalized civil decomposition; it does not allocate, retain the
datetime payload, read the clock, load timezone data, or require an OS
capability. Wrong arity and non-datetime inputs return the individual
structured error strings documented in `docs/src/time-reference.json`.

`TestNativeLibraryTimeClockComponentsParity` in
`internal/osiris/native_library_time_components_test.go` compares interpreter, VM,
generated-C, and native executable output for all four accessors and caught
invalid calls. `TestNativeLibraryTimeClockComponentsContract` checks the C
symbols, per-member inventory entries, error strings, ownership boundary, and
absence of capability macros. The granular public signatures and native
contracts are documented individually in `docs/src/time-reference.json` and
in the `std/time` Native C table in `docs/design/NATIVE-C.md`.

This remains a bounded Task-163 slice, not task completion. The broader
`std/time` row remains partial because parsing, locations, `now`, timers, and
other standard-library and host families remain outside this work.

## Current implementation evidence: `std/time.zone` UTC contract

The generated C backend now admits `std/time.zone` as the smallest safe
addition to the existing two-field UTC datetime ABI. `nativeStaticModuleMember`
and `nativeModuleMemberStatus` record the member individually, while
`ostd_dispatch` calls `ostd_time_zone` after exact arity and datetime-kind
validation. The helper returns a newly constructed two-element list containing
`"UTC"` and `0i64`; it does not retain the datetime, read timezone data, or
define a host capability.

`TestNativeLibraryTimeZoneParity` compares interpreter, VM, generated C, and
native executable output for multiple UTC datetimes plus caught arity and kind
errors. `TestNativeLibraryTimeZoneContract` checks the helper, owned list
boundary, exact error marker, capability absence, and individual inventory
entry. The granular signature and explicit UTC-only limitation are documented
in `docs/design/NATIVE-C.md` and `docs/src/time-reference.json`. The broader
`std/time` cell remains partial until its location-bearing ABI, deterministic
timezone-data strategy, parser, and injected clock contract are implemented.

## Current implementation evidence: `std/time.date`, `add_date`, and `format`

The Native C time ABI now has a bounded UTC date family through
`ostd_time_date`, `ostd_time_add_date`, and `ostd_time_format`. `ODatetime`
stores normalized seconds plus nanoseconds by value. Civil construction accepts
seven exact integer fields or an optional UTC location, rejects non-UTC
locations at the source boundary, validates the full Gregorian range, and
handles negative Unix-era instants with floor-based day arithmetic. Calendar
offsets preserve time-of-day and nanoseconds while correctly normalizing
negative months and month-end overflow; layout formatting supports the
documented reference tokens and returns one owned string.

`TestNativeLibraryTimeDateAddDateFormatParity` compares interpreter, VM,
generated-C, and native executable values for leap dates, negative instants,
month normalization, fractional seconds, layout tokens, and range errors.
`TestNativeLibraryTimeDateAddDateFormatContract` checks the `ODatetime` value
boundary, helper symbols, allocation/capability behavior, and individual
inventory entries; `TestNativeLibraryTimeDateRejectsNonUTCLocationAtSourceBoundary`
proves that unsupported location providers are rejected rather than silently
treated as UTC. Normal and race focused runs pass. This is a bounded time
family slice; parsing, timezone providers, clocks, timers, and broader host
evidence keep task 163 blocked.

## Current implementation evidence: `std/signal.convolve`

The existing generated-C signal dispatcher and `ostd_signal_conv` runtime now
have an individual capability-ledger and differential witness for
`std/signal.convolve(signal, kernel)`. `TestNativeLibrarySignalPureParity`
executes the operation across the interpreter, VM, generated C, and native
executable; `TestNativeLibrarySignalOwnershipCapabilityAndErrorContract`
asserts the direct helper boundary, owned result contract, structured invalid
input behavior, and absence of host capabilities. `nativeModuleMemberStatus`
now records `std/signal.convolve` as implemented, matching the public reference
entry and the existing `nativeStaticModuleMember` admission. This is an
individual standard-library closure slice only; the broader task remains
blocked by the remaining host surfaces and platform evidence.

## Current implementation evidence: `std/time.round` and `std/time.truncate`

The Native C dispatcher now admits the individual pure-value members
`std/time.round(value: duration | datetime, quantum: duration)` and
`std/time.truncate(value: duration | datetime, quantum: duration)`. The
implementation is host-independent for the already admitted UTC datetime
representation: duration values use checked signed integer quotient/remainder
arithmetic, and datetime values use the language's year-1 zero-time origin
modulo the quantum rather than assuming Unix epoch alignment. Negative
quanta, wrong kinds, wrong arity, duration overflow, and native datetime-range
overflow remain explicit errors.

`TestNativeTimeRoundTruncateParity` in
`internal/osiris/native_time_round_test.go` compares interpreter, VM, generated
C, and native execution for positive and negative duration values, datetime
values, zero and arbitrary seven-nanosecond quanta, and caught argument
errors. `TestNativeTimeRoundTruncateOwnershipCapabilityAndErrorContract`
checks the individual allow-list and inventory entries, the checked helper
symbols, exact native error markers, absence of `OSIRIS_CAP_*`, and that the
round/truncate helper region performs no allocation or input retention.
The public signatures and ownership boundary are recorded in
`docs/src/time-reference.json` and the Native C table in
`docs/design/NATIVE-C.md`. Normal, race, and AddressSanitizer runs are
individual evidence for this file; support-matrix statuses and task index
statuses are intentionally unchanged.

## Current implementation evidence: `std/list.partition` slice

The portable native standard-library dispatcher now implements the individual
member `std/list.partition(values, index) -> tuple<list,list>`. The compiler
allow-list and backend inventory admit it explicitly through
`nativeStaticModuleMember` and `nativeModuleMemberStatus`.

`ostd_list_partition` validates a bounded list and non-negative integral index,
rejects an index past the end to match the existing `Task137ListPartition`
contract, copies both ranges into separately tracked `olist` allocations, and
marks the outer two-element list as
the tuple representation. It has no host-capability macro and never retains an
input pointer. Wrong arity and invalid values return structured native errors.

`TestNativeLibraryListPartitionParity` covers interpreter, VM, generated-C,
and executable output, including the caught out-of-range error case. The companion
`TestNativeLibraryListPartitionOwnershipAndErrorContract` checks the exact
runtime symbols, allocation/tuple boundary, capability absence, inventory
entry, and invalid-input messages. The task remains **Blocked**; this bounded
slice does not close the remaining standard-library, host, sanitizer, or
cross-platform criteria, and no matrix status changed.

## Current implementation evidence: pure `std/list` transform and grouping slice

The Native C dispatcher now has individual direct lowerings for the remaining
already-expressible pure list operations:

- `reverse(values: list) -> list`
- `at(values: list, index: i64) -> value`
- `index_of(values: list, target: value) -> i64`
- `last_index_of(values: list, target: value) -> i64`
- `contains(values: list, target: value) -> bool`
- `count(values: list, target: value) -> u64`
- `sort(values: list) -> list`
- `unique(values: list) -> list`
- `zip(left: list, right: list) -> list<tuple>`
- `concat(lists...) -> list`
- `repeat(value: value, count: i64) -> list`
- `empty() -> list`
- `all(values: list) -> bool`
- `any(values: list) -> bool`
- `none(values: list) -> bool`
- `enumerate(values: list) -> list<tuple<u64,value>>`

`ostd_list_reverse`, `ostd_list_at`, `ostd_list_index`,
`ostd_list_sort`, `ostd_list_unique`, and `ostd_list_zip` provide the named C
runtime boundaries; `ostd_dispatch` validates arity and routes variadic
`concat` plus the remaining members. `zip` and `enumerate` mark each pair as
a tuple rather than an ordinary list, matching interpreter and VM rendering.
All outputs are built through tracked `olist` storage, input containers are
not mutated or retained, and these pure members require no host capability.

`TestNativeLibraryListPureParity` in `internal/osiris/native_library_list_test.go`
compares successful values and caught invalid-input results across the
interpreter, VM, generated C, and native executable. The companion
`TestNativeLibraryListPureOwnershipErrorAndCapabilityContract` checks every
individual inventory entry, generated symbols, structured error paths, tuple
markers, allocation boundaries, and capability absence.

Evidence passed for this slice:

```text
GOCACHE=/tmp/osiris-stdlib-list-cache \
go test ./internal/osiris \
  -run '^TestNativeLibraryListPure' \
  -count=1
```

The task remains **Blocked**. This closes only the documented pure list slice;
provider-dependent standard-library members, host-resource ownership,
sanitizer/cross-platform evidence, and the remaining task-163 acceptance
criteria remain open. No support-matrix status was promoted.

## Current implementation evidence: SHA-512 crypto slice

The portable native runtime now implements the following individual members:

- `std/crypto.digest(data: string | list<u8>, algorithm: string) -> string`
- `std/crypto.digest_bytes(data: string | list<u8>, algorithm: string) -> list<u8>`
- `std/crypto.algorithm_supported(name: string) -> bool`

The canonical algorithms are `sha256`, `sha512`, and `sha512_256`. `OSHA512`,
`osh512init`, `osh512write`, and `osh512final` implement the SHA-512 family
without an OpenSSL or operating-system provider. String results use
`oownedstring`; raw bytes use `ostd_bytes_list`; temporary list-to-byte buffers
are released before return. These pure operations require no host capability.
Wrong kinds, wrong arity, invalid byte elements, unknown algorithms, and the
1 MiB input limit remain structured native failures.

`TestNativeLibraryCryptoSHA512Parity` compares hexadecimal output, raw digest
bytes, and algorithm support across interpreter, VM, generated C, and the
generated executable. `TestNativeLibraryCryptoSHA512OwnershipCapabilityAndErrorContract`
checks the generated symbols, ownership boundary, absence of capability macros,
error strings, and individual backend-inventory entries. The task remains
Blocked: provider-dependent standard-library members, host-resource ownership,
sanitizer/cross-platform evidence, and full matrix closure are still open.

## Current implementation evidence: pure crypto codec/comparison slice

The portable native runtime now implements these individual host-independent
members beyond the digest work:

- `std/crypto.hex_encode(data: string | list<u8>) -> string`
- `std/crypto.hex_decode(text: string) -> list<u8>`
- `std/crypto.compare_digest(left: string | list<u8>, right: string | list<u8>) -> bool`

`ostd_crypto_hex` accepts UTF-8 strings or lists containing only `u8` values,
copies list inputs into temporary byte staging, emits lowercase hexadecimal,
and returns an owned string or byte list. It rejects malformed/odd-length
hexadecimal and enforces the documented 1 MiB input/output bound before the
result is exposed. `ostd_crypto_compare_digest` accepts the same byte-compatible
values, compares the common byte range without an early-exit content branch,
folds length mismatch into the result without an 8-bit length truncation, and
releases both temporary buffers on every path. No member requires a host
capability or retains an input pointer.

`TestNativeLibraryCryptoCodecParity` compares successful values and caught
malformed-input behavior across the interpreter, VM, generated C, and native
executable. `TestNativeLibraryCryptoCodecOwnershipErrorAndCapabilityContract`
checks `ostd_crypto_hex`, `ostd_crypto_compare_digest`, `ostd_bytes_list`,
`oownedstring`, structured error strings, length handling, absence of
`OSIRIS_CAP_*`, and the individual backend-inventory entries.

## Current implementation evidence: Base64 codec slice

The portable Native C runtime now admits and implements these individual
host-independent members:

- `std/crypto.base64_encode(data: string | list<u8>) -> string`
- `std/crypto.base64_decode(text: string) -> list<u8>`
- `std/crypto.base64url_encode(data: string | list<u8>) -> string`
- `std/crypto.base64url_decode(text: string) -> list<u8>`

`ostd_crypto_base64_value`, `ostd_crypto_base64_encode`, and
`ostd_crypto_base64_decode` use local RFC 4648 alphabets. Standard encoding
keeps `=` padding; URL-safe encoding omits it. Decoding validates alphabet,
padding, unused trailing bits, the 2 MiB encoded-input bound, and the 1 MiB
decoded-output bound before exposing a result. Byte-compatible inputs are
staged and released; all returned strings and byte lists are newly owned, and
no host capability is defined.

`TestNativeLibraryCryptoBase64Parity` compares standard and URL-safe encoding
and decoding across the interpreter, VM, generated C, and native executable.
`TestNativeLibraryCryptoBase64OwnershipAndErrorContract` checks the helper
symbols, pure capability boundary, individual inventory entries, and the
ownership/error contract. The granular public signatures and native boundary
are documented in `docs/design/NATIVE-C.md` and `docs/src/std-task-reference.js`.

No support-matrix status changed; the task remains Blocked for the broader
standard-library, host, sanitizer, and cross-platform criteria.

## Current implementation evidence: Unicode trim-set slice

The generated-C `std/text` boundary now implements `trim_left` and
`trim_right` against a UTF-8 code-point set rather than a byte set. The
`ostd_text_set_contains` and `ostd_text_trim_set` helpers validate UTF-8,
handle empty and all-removable inputs, return newly owned strings, and release
temporary conversion state on every error path. The operations are pure and
require no host capability.

`TestNativeLibraryTextTrimSetParity` and
`TestNativeLibraryTextTrimSetOwnershipErrorAndCapabilityContract` provide
interpreter/VM/generated-C/native executable parity, Unicode-set behavior,
invalid-kind and invalid-UTF-8 errors, ownership, allocation, capability, and
individual inventory evidence. The exact signatures are documented in
`docs/design/NATIVE-C.md` and the public `std/text` reference. Normal, race,
and LLVM AddressSanitizer focused runs pass. This is an incremental text
surface proof; the task remains blocked for the wider standard-library,
host-resource, sanitizer, and cross-platform acceptance criteria.

## Current implementation evidence: native host inventory reconciliation

The capability ledger now reports the admitted generated-C `std/net` and
`std/os` resource members as implemented individually wherever
`emitNativeResourceCall` has a direct adapter. This includes TCP/UDP/DNS,
TLS/HTTP, process and stream handles, file I/O, rooted filesystem operations,
metadata mutation, deadlines, signals, and snapshot watches. The entries still
retain their capability checks and platform/provider boundaries; this change
only corrects the ledger's previous blanket unsupported result for those direct
resource paths.

`TestBackendCapabilityInventoryIsCompleteAndUnique` now asserts representative
network, file, root, process, stream, and watch members are individually
admitted as native implementations. This is inventory evidence, not a claim
that host-dependent rows are stable: generated execution, ownership and
cancellation, platform evidence, and provider-specific behavior remain part of
the broader task gate.

## Current implementation evidence: `std/list.chunks` pure grouping slice

The generated-C list-shape helper now has a complete chunk-mode contract for
`std/list.chunks(values: list, width: i64) -> list<list>`. The helper validates
the input list bound and positive width before materializing output, copies each
chunk into its own tracked list allocation, permits a shorter final chunk, and
returns an empty outer list for empty input. The source container is never
mutated and no C pointer or host capability escapes the operation.

The affected symbols are `ostd_list_shape` and `ostd_dispatch`; the admission
and ledger symbols are `nativeStaticModuleMember`, `nativeModuleMemberStatus`,
and `BackendCapabilityInventory`. The new chunk branch is isolated from the
existing slice and window branches, so this evidence does not claim either
other shape operation as part of this slice.

`TestNativeLibraryListChunksParity` compares ordinary, empty, short-final,
non-positive-width, oversized-width, and invalid-input behavior through the
interpreter, VM, generated C, and native executable. The companion
`TestNativeLibraryListChunksOwnershipErrorAndCapabilityContract` verifies the
tracked nested list construction, structured error strings, capability absence,
and the individual inventory record. Granular signatures and ownership rules
are documented in `docs/design/NATIVE-C.md`, `docs/src/reference-data.json`,
and `docs/src/std-task-reference.js`.

No support-matrix status changed; the task remains Blocked for its broader
standard-library, host, sanitizer, cross-platform, and provider-dependent
criteria.

The latest bounded `std/time` slice also directly lowers `compare`, `equal`,
`before`, and `after` for valid calls through overflow-safe portable value
comparisons. Normal, race, AddressSanitizer, ownership, and ABI checks passed.
The aggregate time row remains partial for clock reads, parsing, timers,
locations, the location-bearing datetime ABI, and alternate-platform evidence.

## Current implementation evidence: complete pure `std/list` surface

The Native C list dispatcher now has executable coverage for every exported
`std/list` member: `range`, `len`, `take`, `sum`, `product`, `reverse`, `at`,
`index_of`, `last_index_of`, `contains`, `count`, `sort`, `unique`,
`partition`, `slice`, `chunks`, `windows`, `zip`, `concat`, `repeat`, `empty`,
`all`, `any`, `none`, `enumerate`, and `flatten`. The previously incomplete
window path was corrected in `ostd_list_shape`: slice bounds are no longer
applied to window width and step, so overlapping windows are accepted while
both parameters remain independently positive and bounded.

`TestNativeLibraryListPureParity` compares the complete surface across the
interpreter, VM, generated C, and native executable, including successful
overlapping-window output and caught invalid-width behavior.
`TestNativeLibraryListCompleteOwnershipAndCapabilityContract` checks every
individual inventory record, dispatcher/helper symbols, owned nested results,
structured failures, and the absence of host capabilities. The dedicated
flatten and chunk witnesses remain in their respective focused test files.

Evidence passed on macOS:

```text
GOCACHE=/tmp/osiris-native-list-complete-cache2 go test ./internal/osiris \
  -run '^TestNativeLibraryList(CompleteOwnershipAndCapabilityContract|PureParity)$' \
  -count=1 -timeout=600s
ok

GOCACHE=/tmp/osiris-native-list-complete-race go test -race -vet=off ./internal/osiris \
  -run '^TestNativeLibraryList(CompleteOwnershipAndCapabilityContract|PureParity)$' \
  -count=1 -timeout=600s
ok

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
GOCACHE=/tmp/osiris-native-list-complete-asan go test ./internal/osiris \
  -run '^TestNativeLibraryList(CompleteOwnershipAndCapabilityContract|PureParity)$' \
  -count=1 -timeout=600s
ok
```

The `std/list` Native C matrix cell is now marked `stable`; the remaining
task-163 standard-library and host members remain governed by their own
evidence and do not inherit this module's status.

## Current implementation evidence: complete pure `std/crypto` surface

The Native C dispatcher now executes every exported `std/crypto` member,
including module version/bound constants, SHA algorithm constants, seeded
`random`, `hash`, `mix`, `noise`, text and byte digests, HMAC, constant-time
comparison, hexadecimal and standard/URL-safe Base64 codecs, deterministic
key derivation, seeded byte generation, deterministic tokens, UUID v5, and
algorithm queries. The new `OSHA1` implementation is local to the generated
runtime and is used only for UUID v5; no ambient provider or capability is
consulted.

`TestNativeLibraryCryptoCompleteParity` compares the complete module through
the interpreter, VM, generated C, and native executable. Its companion
`TestNativeLibraryCryptoCompleteOwnershipAndCapabilityContract` checks every
inventory member, direct helper and dispatcher symbols, bounded failure
behavior, owned output boundaries, and absence of `OSIRIS_CAP_*` definitions.
The existing focused digest, HMAC, codec, mix, noise, and derivation witnesses
remain as independent regressions.

Evidence passed on macOS:

```text
GOCACHE=/tmp/osiris-native-crypto-complete-cache go test ./internal/osiris \
  -run '^TestNativeLibraryCryptoComplete' -count=1 -timeout=600s
ok

GOCACHE=/tmp/osiris-native-crypto-complete-race go test -race -vet=off ./internal/osiris \
  -run '^TestNativeLibraryCryptoComplete' -count=1 -timeout=600s
ok

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
GOCACHE=/tmp/osiris-native-crypto-complete-asan go test ./internal/osiris \
  -run '^TestNativeLibraryCryptoComplete' -count=1 -timeout=600s
ok

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=undefined \
GOCACHE=/tmp/osiris-native-crypto-complete-ubsan go test ./internal/osiris \
  -run '^TestNativeLibraryCryptoComplete' -count=1 -timeout=600s
ok
```

The `std/crypto` Native C cell is now marked `stable`; provider-dependent
network, filesystem, process, and cross-platform evidence in this task is
not affected by that module-level closure.

## Current implementation evidence: `std/time` duration and Unix slice

The Native C emitter now directly lowers these pure, host-independent members:

- `std/time.duration(value)` for an existing duration, exact signed integer,
  or exact `Second` quantity;
- `std/time.unix(seconds[, nanosecond_adjustment])`;
- `std/time.unix_milli(milliseconds)`;
- `std/time.unix_micro(microseconds)`; and
- the six duration constants `nanosecond`, `microsecond`, `millisecond`,
  `second`, `minute`, and `hour`.

`std/time.hour`, `minute`, `second`, and `nanosecond` retain their language
dual-use contract: value position lowers to the static duration, while call
position lowers to `ostd_time_component` and returns the UTC civil component.
The conversion helpers use checked integer arithmetic, normalize negative
sub-second values into a non-negative nanosecond field, and reject values
outside the supported civil range. They preserve the repaired value-owned
`ODatetime` ABI with `int64_t seconds` and `int32_t nanosecond`; comparison,
rounding, and civil extraction continue to consume that same representation.
No helper reads the clock, allocates result storage, retains an argument
pointer, or defines a host capability macro.

`TestNativeLibraryTimeDurationUnixParity` compares successful conversions,
negative Unix fractions, constants, dual-use component calls, and caught
fractional-input errors across the interpreter, VM, generated C, and native
executable. `TestNativeLibraryTimeDurationUnixContract` checks direct helper
symbols, static-value lowering, the unchanged datetime ABI, capability
absence, and individual inventory entries. The granular signatures and
ownership boundaries are documented in `docs/design/NATIVE-C.md` and
`docs/src/time-reference.json`.

No support-matrix status changed; task 163 remains blocked by its broader
standard-library, host, sanitizer, cross-platform, and provider-dependent
criteria.

## Current implementation evidence: `std/time` direct comparison slice

The Native C emitter now directly lowers valid two-argument calls to the pure
value members `std/time.compare`, `std/time.equal`, `std/time.before`, and
`std/time.after`. `nativeDirectTimeBuiltin` admits only the valid arity, while
the `emitCInstr` branch selects `ostd_time_compare` or `ostd_time_predicate`.
Malformed arity remains on `ostd_dispatch` so structured errors remain
catchable. The comparison helpers were placed in
`cRuntimePortableComponent`; they compare exact duration nanoseconds or
seconds-plus-nanoseconds for UTC datetimes, avoid subtraction overflow, return
scalar values, allocate nothing, and retain no input storage.

`TestNativeTimeComparisonParity` in
`internal/osiris/native_time_comparison_test.go` compares interpreter, VM,
generated-C, and native executable output for duration and UTC datetime
relations, mixed-kind rejection, and wrong arity. The companion contract test
checks direct helper selection, the no-allocation/value-ownership boundary,
capability absence, and the four individual inventory entries.

Focused validation on the current macOS/POSIX host passed:

```text
GOCACHE=/tmp/osiris-time-comparison-normal \
go test ./internal/osiris -run '^TestNativeTimeComparison' -count=1 -timeout=1200s
ok

GOCACHE=/tmp/osiris-time-comparison-race \
go test -race -vet=off ./internal/osiris -run '^TestNativeTimeComparison' -count=1 -timeout=1200s
ok

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
GOCACHE=/tmp/osiris-time-comparison-asan \
go test ./internal/osiris -run '^TestNativeTimeComparison' -count=1 -timeout=1200s
ok
```

This is a bounded host-independent value slice only. The aggregate
`std/time` Native C row remains partial: clock reads, parsing, timers,
IANA/fixed locations, location-bearing datetime ABI, and alternate-platform
execution evidence remain open. No support-matrix or task status changed.

## Pause checkpoint (2026-08-09)

Implementation is paused. Granular Native C evidence has independently closed
the maintained pure list, text, crypto, signal, selected math, codec, and POSIX
environment/identity slices. The task remains blocked by the rest of the
standard-library and host surface: complete time semantics beyond the UTC
projection, networking/TLS/HTTP provider behavior, filesystem/process/watch
parity, FFI/resource ownership, cancellation/deadline semantics, and the
cross-platform generated-C evidence tracked by tasks 401 and 402. No support
matrix status was promoted.

The granular `native-os-environment-and-identity` row is now stable for the
POSIX/macOS execution surface. Environment snapshots and mutation, missing
values, path-separator behavior, hostname, process IDs, and page size have
generated-executable parity under explicit capability grants, with capability
normalization and missing-grant diagnostics covered. Windows/Linux adapters,
filesystem/process/watch breadth, and alternate-platform evidence remain in
the aggregate partial OS row.

The granular `native-posix-filesystem-and-roots` row is now stable for the
macOS/POSIX boundary. Maintained generated-executable fixtures cover owned
file resources, metadata and mutation, rooted filesystem operations,
ownership/timestamp methods, root FS bridging, capability gates, cleanup, and
symlink-safe no-escape resolution. Windows HANDLE adapters, Linux execution
evidence, and unlisted filesystem APIs remain outside the aggregate partial
OS row.

Two additional Native C granular rows are stable on the POSIX/macOS boundary:
`native-posix-process-resources` covers owned process spawn/wait/signal/kill,
stream detachment, environment overlays, process-group cancellation, timeout
escalation, descendant cleanup, and reaping; `native-posix-standard-resources`
covers standard streams/raw handles, signal subscriptions, and bounded watch
snapshots. Their focused normal/race fixtures prove ownership and cleanup.
Windows HANDLE adapters, Linux evidence, and unlisted host semantics remain
outside these rows and the aggregate OS row.

The granular `std-time-utc-projection` row is now stable for Native C. The
maintained UTC and zone fixtures prove value-preserving `time.utc` conversion
and newly owned `time.zone` results, including generated-executable parity,
wrong-kind/arity behavior, no clock or timezone-data access, and no host
capability. This is intentionally narrower than the still-partial aggregate
`std-time` row: IANA/fixed locations, `now`, parsing, timers, and alternate
platform evidence remain open.

The provider-backed `std/math` linear-algebra surface is now closed in the
separate Native C core/data record (task 162). This standard-library task keeps
the provider deployment requirement explicit and does not treat `eig`, `eigh`,
or `svd` as host-independent bundled implementations.

## Current implementation evidence: `std/crypto.mix` deterministic mixer slice

The Native C backend now closes the registered pure member
`std/crypto.mix(value...) -> text`. The exact compiler admission and lowering
symbols are `nativeStaticModuleMember` and the `emitCFunctionBody`/`emitCInstr`
static-module dispatch path in `internal/osiris/cbackend_native.go`. The
individual inventory path is `nativeModuleMemberStatus` in
`internal/osiris/backend_inventory.go`, which now records
`module/std/crypto/mix` as implemented.

The generated runtime symbol `ocrypto_mix` in
`internal/osiris/cbackend_portable_runtime.go` uses the existing canonical
`orender` representation, including tuple delimiters and singleton tuple
commas, then applies the language's fixed 64-bit byte mixer and returns an
owned 16-character lowercase hexadecimal string through `oownedstring`.
Each argument's temporary rendering buffer is released before the next
argument is processed; no input pointer or host resource escapes, and no
capability macro is defined. Zero arguments remain valid. Allocation or
rendering failure is returned through the existing structured native error
path.

`TestNativeLibraryCryptoMixParity` in
`internal/osiris/native_library_crypto_mix_test.go` compares zero-argument,
mixed scalar, byte-list, and tuple calls across the interpreter, VM, generated
C, and native executable. `TestNativeLibraryCryptoMixContract` checks direct
runtime/helper selection, canonical rendering, owned-result construction,
capability absence, and the individual inventory record. The public
`std/crypto.mix` entry now documents its exact text result, canonical argument
rendering, algorithm, ownership, and native capability contract in
`docs/src/reference-data.json`; the generated-C contract is documented
individually in `docs/design/NATIVE-C.md`, and the support-matrix note no
longer lists `mix` as an unsupported Native C member.

Focused validation on the current macOS host passed:

```text
GOCACHE=/tmp/osiris-native-crypto-mix-cache3 \
go test ./internal/osiris -run '^TestNativeLibraryCryptoMix' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  1.082s

GOCACHE=/tmp/osiris-native-crypto-mix-race-cache \
go test -race -vet=off ./internal/osiris -run '^TestNativeLibraryCryptoMix' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  2.147s

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
GOCACHE=/tmp/osiris-native-crypto-mix-asan-cache \
go test ./internal/osiris -run '^TestNativeLibraryCryptoMix' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  1.435s

GOCACHE=/tmp/osiris-native-crypto-mix-compile-cache \
go test ./internal/osiris -run '^$' -count=1
ok   github.com/sphireinc/osiris/internal/osiris  0.331s [no tests to run]

PATH=/Users/JuanSanchez/.nvm/versions/node/v24.12.0/bin:$PATH \
npm --prefix docs run check-reference
Reference checks passed: 26 API groups, 6 chapters, 315 built-ins,
19 special forms, 375 named constants.
Reference navigation checks passed: 27 pages, 1063 unique entry routes.

./scripts/check-repository-hygiene.sh
Repository hygiene checks passed.
```

No support-matrix status changed; task 163 remains blocked by the other
registered standard-library members and by the broader host, sanitizer,
cross-platform, and provider-dependent acceptance criteria.

## Current implementation evidence: `std/crypto.noise` deterministic noise slice

The Native C backend now directly lowers the registered pure member
`std/crypto.noise(value...) -> text`. `nativeStaticModuleMember`,
`nativeModuleMemberStatus`, and the static-module branch in `emitCInstr` admit
the individual member and select `ocrypto_noise` without `ostd_dispatch` or a
reference-runtime fallback. `ocrypto_noise_hash` implements the fixed
byte-level FNV-style hash and `ocrypto_noise` owns and releases each temporary
canonical-render buffer before returning a tracked lowercase hexadecimal text
value.

`TestNativeLibraryCryptoNoiseParity` compares zero-argument, mixed scalar,
byte-list, and tuple calls through the interpreter, VM, generated C, and native
executable. `TestNativeLibraryCryptoNoiseContract` checks direct helper
selection, `orender`/`oownedstring` ownership, no host capability macro, and
the individual `module/std/crypto/noise` inventory entry. The public signature
and exact algorithm are documented in `docs/src/reference-data.json`; the
Native C ownership and capability contract is documented individually in
`docs/design/NATIVE-C.md`.

Focused normal, race, and LLVM AddressSanitizer runs passed on the current
macOS host. This is a bounded pure-member slice; task 163 remains blocked by
the other standard-library and host-resource members, provider availability,
and alternate-platform evidence.

Focused validation for the `std/time.utc` slice passed on the current macOS
host:

```text
GOCACHE=/tmp/osiris-time-utc-focused-cache2 \
go test ./internal/osiris -run '^TestNativeLibraryTimeUTC' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  1.201s

GOCACHE=/tmp/osiris-time-utc-race-cache \
go test -race -vet=off ./internal/osiris -run '^TestNativeLibraryTimeUTC' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  2.215s

CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address \
GOCACHE=/tmp/osiris-time-utc-asan-cache2 \
go test ./internal/osiris -run '^TestNativeLibraryTimeUTC' -count=1 -timeout=1200s
ok   github.com/sphireinc/osiris/internal/osiris  1.428s

GOCACHE=/tmp/osiris-time-utc-compile-cache \
go test ./internal/osiris -run '^$' -count=1
ok   github.com/sphireinc/osiris/internal/osiris  0.657s [no tests to run]

GOCACHE=/tmp/osiris-native-stdlib-focused-cache \
go test ./internal/osiris -run '^TestNativeLibrary(TimeUTC|TimeDuration|TimeDate|TimeClockComponents|Text|List|Signal|Crypto)' -count=1 -timeout=2400s
ok   github.com/sphireinc/osiris/internal/osiris  15.424s

PATH=/Users/JuanSanchez/.nvm/versions/node/v24.12.0/bin:$PATH \
npm --prefix docs run check-reference
Reference checks passed: 26 API groups, 6 chapters, 315 built-ins,
19 special forms, 375 named constants.
Reference navigation checks passed: 27 pages, 1061 unique entry routes.

./scripts/check-repository-hygiene.sh
Repository hygiene checks passed.
```

The broader task remains **Blocked** and the support matrix remains unchanged;
these results prove only this bounded pure member and its focused family
witnesses.

## Current implementation evidence: `std/time.utc` pure conversion slice

The generated-C backend now admits and directly lowers the registered pure
member `std/time.utc(value: datetime) -> datetime`. The exact compiler symbols
are `nativeStaticModuleMember`, `nativeDirectTimeBuiltin`, and the `emitCInstr`
`std/time.utc` branch in `internal/osiris/cbackend_native.go`. Valid calls emit
`ostd_time_utc`; wrong-arity calls remain in `ostd_dispatch` so the language
error can be caught normally. The runtime helper validates `O_DATETIME` and
returns the complete `ODatetime` value by copy. Since the native ABI has no
non-UTC location slot, this preserves the exact seconds and normalized
nanoseconds without consulting timezone data, allocating, retaining an input
pointer, or defining a host capability.

`nativeModuleMemberStatus` now records the individual
`module/std/time/utc` inventory entry as implemented based on this direct
generated-C evidence. This is an implementation addition, not an inventory
rename: the member was registered in `stdlibTimeModule` but previously had no
Native C admission, dispatch branch, or lowering.

`TestNativeLibraryTimeUTCParity` in
`internal/osiris/native_library_time_utc_test.go` compares interpreter, VM,
generated-C, and executable results for negative Unix fractions, leap-day
values, exact equality, wrong arity, and wrong type. The companion
`TestNativeLibraryTimeUTCContract` checks the direct helper/call, value-owned
datetime ABI, structured error marker, absence of `OSIRIS_CAP_*`, and the
individual inventory entry. The focused test is suitable for normal, race,
and LLVM AddressSanitizer runs. The granular public signature is documented in
`docs/src/time-reference.json`; its Native C ownership and capability contract
is documented individually in `docs/design/NATIVE-C.md`.

No support-matrix status changed; task 163 remains blocked by its broader
standard-library, host, sanitizer, cross-platform, and provider-dependent
criteria.
