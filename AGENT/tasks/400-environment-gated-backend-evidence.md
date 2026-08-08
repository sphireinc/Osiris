# Task 400: Complete environment-gated backend evidence

**Priority:** P1
**Dependencies:** 303,304,307,308,162,163
**Status:** Blocked by execution environment

## Purpose

Track only release evidence that cannot be produced by the current macOS
developer machine. This task is deliberately not a substitute for backend
implementation. WASM closures and host imports, native C parity, LSP parity,
and GPU kernel implementation remain in their owning tasks and must continue
to advance locally when they do not require external hardware or an alternate
operating system.

## Environment-gated scope

### Windows native execution

- Execute generated native C programs on supported Windows runners.
- Exercise HANDLE adapters for processes, files, sockets, roots, and watches.
- Verify standard streams, raw handles, signals, cancellation, and cleanup
  under Windows error and close semantics.
- Run the native C parity and sanitizer-equivalent suites on every supported
  Windows architecture.

### Linux generated-C and sanitizer evidence

- Run the generated-C native parity suite on Linux, including the Linux ASan
  runtime and leak/undefined-behavior checks required by tasks 162 and 163.
- Record compiler, libc, kernel, architecture, sanitizer runtime, and test
  commands in the evidence artifact.
- Include cross-platform differential results; a macOS compile or a Linux
  cross-compile alone is not execution evidence.

### Real GPU-device evidence

- Run the native GPU provider on a machine exposing a supported `MTLDevice`
  or another approved device adapter.
- Exercise discovery, allocation, upload, kernel launch, synchronization,
  readback, cancellation, device loss, and release-on-error.
- Produce device-backed fixtures for scalar/numeric, tensor, reduction,
  matrix/linear-algebra, signal, and mapped-library rows owned by tasks 304,
  307, and 308.
- Keep CPUReferenceProvider fixtures separately labeled; reference execution
  cannot promote hardware GPU matrix cells.

## Required evidence and acceptance criteria

- CI or a designated hardware runner publishes reproducible logs and artifact
  metadata for every environment-gated surface above.
- The evidence identifies the exact backend, operating system, architecture,
  compiler/runtime, device, driver/API version, and capability profile.
- Failures are structured capability or platform diagnostics, never silent
  fallback to another evaluator or CPU reference execution.
- Only after the owning implementation task has executable coverage may task
  310 consume this evidence and promote a matrix cell to `stable`.
- The task is complete only when all environment-gated acceptance checks pass;
  skipped hardware or unavailable runners remain explicitly blocked.

## Current machine check

- The macOS host exposes Metal framework support but no `MTLDevice`; the
  conditional native-device fixture therefore skips and no hardware result is
  counted.
- Colima is installed, but the current Docker daemon is not running and the
  configured socket is unavailable. Linux execution evidence therefore has
  not been claimed from this checkout. A later CI or explicitly started Linux
  runner must provide it.

## Explicitly out of scope

These are not environment blockers and must not be moved here:

- WASM dynamic closures, captured mutable cells, runtime aggregates, or host
  imports (tasks 301 and 302).
- Native C core, numeric, standard-library, host-resource, or sanitizer
  implementation gaps that can be developed and tested on macOS (tasks 162
  and 163).
- LSP semantic, workspace, refactoring, and test-tooling implementation (tasks
  160, 161, and 168).
- GPU provider and kernel implementation that can be validated with a fake
  device or compile-time ABI tests (tasks 303, 304, 307, and 308).

## Suggested execution

Use the repository CI matrix or a dedicated Windows/Linux/GPU runner. Run the
task-specific focused suites first, then the release command:

```sh
OSIRIS_REQUIRE_STABLE_MATRIX=1 ./scripts/check-support-matrix.sh
```

The release command must remain failing while any non-stable cell lacks the
implementation and environment evidence required by task 310.
