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
