# Task 147: Stabilize native C standard-library parity

**Priority:** P1
**Dependencies:** 146,137
**Status:** Complete

## Objective

Bring native C standard-library support from selected members to complete
parity for the documented `std/text`, `std/crypto`, `std/signal`, `std/list`,
and `std/time` contracts, plus stable data helper integration.

## Exact scope and ownership

- Own native static-module dispatch and generated helper portions of
  `internal/osiris/cbackend_native.go`, its C runtime helpers, and native
  standard-library parity tests.
- Implement every exported pure/bounded member or provide a stable native
  implementation with exact interpreter/VM results; no silent fallback.
- Cover Unicode ownership, cryptographic encodings/digests, deterministic
  signal kernels, list construction/search/transforms, date/time values,
  formatting, and codec/data helpers.
- Add generated-C executable parity and negative source-span tests for any
  deliberately host-only boundary.
- Update public per-symbol backend documentation only after tests pass.

## Matrix transitions

Promote `std/text`, `std/crypto`, `std/signal`, `std/list`, `std/time`, and
data-helper native rows to `stable`.

## Parallelism

Sequential after task 146 and before task 148. Do not run with another task that
edits native module dispatch.

## Completion evidence

- `cbackend_stdlib_runtime.go` supplies bounded, host-independent generated-C
  helpers and `ostd_dispatch` for the parity-tested text, list, signal, crypto,
  and time operations. Dispatch is selected from the explicit native module
  table; unsupported members still fail at the source location.
- Text helpers validate UTF-8 and bound input/result sizes; list helpers retain
  value ownership; signal kernels use deterministic scalar formulas; crypto
  uses owned byte conversion and SHA-256/hex contracts; time helpers retain
  the existing UTC/duration contract.
- `internal/osiris/task147_native_parity_test.go` differentially executes
  trim/split, list reversal, signal statistics, crypto digest, and duration
  formatting through interpreter, VM, and generated C.
- `GOCACHE=/tmp/osiris-task147-cache3 go test ./internal/osiris -run '^TestTask147NativeStandardLibraryParity$' -count=1 -timeout=600s` passed; the native boundary and Task 146 suites remain green.
