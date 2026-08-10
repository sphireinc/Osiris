# Task 402: Verify Windows Native C resource adapters

**Priority:** P1
**Dependencies:** 148, 163
**Status:** Blocked by execution environment

## Objective

Execute and validate the Windows HANDLE/Winsock adapters already emitted by
Native C for files, roots, processes, sockets, streams, signals, and watches.

## Scope

- Run generated C on supported Windows architectures.
- Exercise HANDLE ownership, close/error behavior, standard streams, raw
  handles, rooted resources, process cancellation, sockets, and watches.
- Verify environment overlays, inherited handles, signal behavior, and cleanup
  under Windows semantics.
- Record compiler, SDK, architecture, runtime, capability profile, and exact
  commands.

## Acceptance criteria

- Every Windows adapter fixture completes with generated-executable evidence.
- Close, cancellation, ownership, and capability-denial behavior is checked
  under Windows rather than inferred from POSIX.
- Windows failures remain explicit structured/platform diagnostics with no
  shell or interpreter fallback.
- Evidence is attached to task 148/163 and consumed by task 154.

## Current blocker

The current development host is macOS and cannot execute Windows HANDLE
artifacts. Windows CI or a dedicated Windows runner is required.

## Pause checkpoint (2026-08-09)

No Windows HANDLE/Winsock execution evidence was added in this checkpoint.
POSIX/macOS generated-C results remain insufficient for this task.
