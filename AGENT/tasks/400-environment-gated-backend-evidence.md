# Task 400: Complete environment-gated backend evidence

**Priority:** P1
**Dependencies:** 401,402,403
**Status:** Blocked by execution environment

## Purpose

Track only release evidence that cannot be produced by the current macOS
developer machine. This task is deliberately not a substitute for backend
implementation. WASM closures and host imports, native C parity, LSP parity,
and GPU kernel implementation remain in their owning tasks and must continue
to advance locally when they do not require external hardware or an alternate
operating system.

## Environment-gated scope

The concrete platform evidence is split into task 401 (Linux generated-C and
sanitizers), task 402 (Windows Native C resource adapters), and task 403 (real
GPU-device execution). This umbrella task owns coordination, evidence format,
and release-gate consumption; implementation remains in the owning 100/300
series tasks.

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

- The macOS host exposes Metal framework support but no `MTLDevice`; task 403
  therefore remains blocked and no hardware result is counted.
- Linux generated-C execution and Windows HANDLE execution are not claimed
  from this checkout; tasks 401 and 402 require their respective runners.

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

## Pause checkpoint (2026-08-09)

No environment-gated evidence was produced during this checkpoint. The
current macOS host still cannot provide Linux execution, Windows HANDLE
execution, or real GPU-device execution; tasks 401, 402, and 403 remain
blocked and the umbrella task remains open.
