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

`TestTask163NativeListReductionParity` in
`internal/osiris/task162_163_native_parity_test.go` compares integer, `f64`,
and complex sum/product behavior across interpreter, VM, and generated C
execution. Existing task 145, 146, and 147 native parity suites provide the
broader standard-library witnesses.

The same native dispatcher now also admits `std/list.flatten(values, depth)`.
Its C path is implemented by `ostd_list_flatten_append` and
`ostd_list_flatten`, with the same 64-level depth and 100,000-item bounds used
by the staged list API. `TestTask163NativeListFlattenParity` covers depth zero,
one, and two across interpreter, VM, and generated C. The VM shape validator
recognizes this two-argument list member separately from the one-argument
tensor `flatten` builtin, so the differential test reaches execution instead
of failing during preflight.

The native dispatcher and inventory also carry a bounded pure-text slice:
`std/text.trim(value)`, `std/text.trim_space(value)`,
`std/text.lower(value)`, `std/text.contains(value, needle)`,
`std/text.has_prefix(value, prefix)`, and
`std/text.has_suffix(value, suffix)`. `TestTask163NativeTextBoundaryParity`
compares all six signatures and values across interpreter, VM, generated C, and
the executable. `TestTask163NativeTextOwnershipAndCapabilityContract` checks
that string-producing helpers use the generated owned-string boundary and that
the pure operations do not require an OS capability. The exact contracts are
documented in `docs/design/NATIVE-C.md` under “Native std/text boundary
contracts”.

This is not task completion. The task remains **Blocked** pending individual
signature/ownership/capability evidence for the remaining standard-library and
host surfaces, sanitizer and cross-platform generated-C evidence, and
provider-dependent implementations. No support-matrix status is changed by
this progress update.

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

`TestTask163NativeListPartitionParity` covers interpreter, VM, generated-C,
and executable output, including the caught out-of-range error case. The companion
`TestTask163NativeListPartitionOwnershipAndErrorContract` checks the exact
runtime symbols, allocation/tuple boundary, capability absence, inventory
entry, and invalid-input messages. The task remains **Blocked**; this bounded
slice does not close the remaining standard-library, host, sanitizer, or
cross-platform criteria, and no matrix status changed.

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

`TestTask163NativeCryptoSHA512Parity` compares hexadecimal output, raw digest
bytes, and algorithm support across interpreter, VM, generated C, and the
generated executable. `TestTask163NativeCryptoSHA512OwnershipCapabilityAndErrorContract`
checks the generated symbols, ownership boundary, absence of capability macros,
error strings, and individual backend-inventory entries. The task remains
Blocked: provider-dependent standard-library members, host-resource ownership,
sanitizer/cross-platform evidence, and full matrix closure are still open.
