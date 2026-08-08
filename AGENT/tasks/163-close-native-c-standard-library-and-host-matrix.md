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
