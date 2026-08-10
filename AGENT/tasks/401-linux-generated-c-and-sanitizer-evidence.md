# Task 401: Verify Linux generated-C and sanitizer evidence

**Priority:** P1
**Dependencies:** 162, 163
**Status:** Blocked by execution environment

## Objective

Run the already-implemented Native C core, standard-library, and host slices
under a real Linux kernel/userspace environment and record reproducible
generated-executable evidence.

## Scope

- Execute the Native C differential suites on supported Linux architectures.
- Run Linux AddressSanitizer, UndefinedBehaviorSanitizer, and leak checks where
  the toolchain supports them.
- Record compiler, libc, kernel, architecture, sanitizer runtime, capability
  profile, and exact commands.
- Compare Linux generated-C output with the interpreter and VM without treating
  a cross-compile as execution evidence.

## Acceptance criteria

- Linux generated executables run successfully for every owning task-162 and
  task-163 fixture selected by the release manifest.
- Sanitizer runs produce reproducible logs with no unexplained failures.
- Platform-specific failures are structured diagnostics and never hidden
  fallback to the interpreter, VM, or another host.
- Evidence is attached to the owning task records and consumed by task 154
  without promoting unsupported implementation rows.

## Current blocker

The current development host is macOS. Colima/Docker availability alone does
not constitute Linux generated-executable evidence; a running Linux VM,
container runtime, or CI Linux runner must execute the binaries.

## Pause checkpoint (2026-08-09)

No Linux binary execution or Linux sanitizer evidence was added in this
checkpoint. macOS/LLVM sanitizer results remain host-local evidence and do not
satisfy this task.
