# Task 119: Add process, environment, and OS integration

**Priority:** P2
**Dependencies:** 014, 035, 045, 046, 102, 116, 117
**Status:** Blocked

## Objective

Add safe application-facing APIs for command-line arguments, environment,
child processes, general OS signals, and filesystem watching.

## Required surface

- Read-only command-line argument and selected environment access with
  explicit capability policy and deterministic test injection.
- Child process spawn, stdin/stdout/stderr pipes, exit status, working
  directory, environment, cancellation, timeout, and process-tree cleanup.
- General signal subscription/notification distinct from `std/signal`
  numerical processing.
- Filesystem watch events with normalized paths, stable event categories,
  overflow/rescan behavior, cancellation, and platform capability reporting.

## Implementation requirements

- Never invoke a shell implicitly; arguments remain an explicit vector.
- Redact secrets from diagnostics/traces and bound captured process output.
- Put platform differences behind build-tagged adapters and documented
  portable semantics.
- Integrate tasks, streams, scope-bound cleanup, structured errors, CLI JSON
  diagnostics, and native/host capability gates.
- Provide deterministic fake environment/process/watch providers for tests.

## Tests and documentation

- Quoting/injection, environment isolation, pipe backpressure, cancellation,
  timeout, exit mapping, signal, watch overflow, symlink/path, race, and
  cross-platform tests.
- Security tests for capability denial and secret redaction.
- Complete standard-library and technical-reference pages.

## Acceptance criteria

- [ ] APIs are explicit, cancellable, resource-bounded, and shell-safe.
- [ ] Platform differences and capability gates are documented and tested.
- [ ] Deterministic fakes support reliable language-level tests.
- [ ] Full normal/race/platform/docs validation passes.

## Non-goals

- No terminal UI, privilege escalation, service manager, container runtime, or
  unrestricted ambient environment mutation.
