# Task 119: Add process, environment, and OS integration

**Priority:** P2
**Dependencies:** 014, 035, 045, 046, 102, 116, 117
**Status:** Complete

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

- [x] APIs are explicit, cancellable, resource-bounded, and shell-safe.
- [x] Platform differences and capability gates are documented and tested.
- [x] Deterministic fakes support reliable language-level tests.
- [x] Full normal/race/platform/docs validation passes.

## Implementation evidence

- Added the capability-gated `std/os` module with read-only `args`, isolated
  environment reads/name enumeration, shell-free process spawn/wait/kill,
  bounded signal event streams, and normalized snapshot filesystem watches.
- Added `Runtime.SetCommandLineArgs` and `Runtime.SetEnvironment`, including
  authoritative empty injections, plus deterministic in-package process and
  snapshot provider seams used by the language-runtime tests.
- Child processes use owned stdin/stdout/stderr streams, explicit vector
  arguments, opt-in environment inheritance, platform process-group cleanup,
  structured exit results, and context/timeout cancellation. Watch overflow
  reserves a queue slot for a rescan marker; signal queues are bounded.
- Added Unix, Windows, and fallback process adapters; integrated structured
  host errors, closable process values, backend capability inventory, and
  source-spanned native-C rejection for hosted `std/os` members.
- Added `AGENT/docs/design/OS-INTEGRATION.md`, the `std/os` reference page,
  technical runtime-kind/module entries, and support-matrix evidence.

## Completion evidence

- `GOCACHE=/tmp/osiris-go-cache go test ./...`
- `GOCACHE=/tmp/osiris-go-cache go vet ./...`
- `GOCACHE=/tmp/osiris-go-cache go build ./...`
- `GOCACHE=/tmp/osiris-race-cache go test -race ./internal/osiris -count=1`
- `GOOS=linux GOARCH=arm64 GOCACHE=/tmp/osiris-linux-cache go test -c ./internal/osiris -o /tmp/osiris-linux-arm64.test`
- `GOOS=windows GOARCH=amd64 GOCACHE=/tmp/osiris-windows-cache go test -c ./internal/osiris -o /tmp/osiris-windows-amd64.test.exe`
- `CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address GOCACHE=/tmp/osiris-asan-cache go test ./internal/osiris -run 'Test(NativeRejectsHostedOS|OS)' -count=1 -timeout=600s`
- In `docs/` with Node 24: `npm run check-reference`, `npm run check-examples`,
  and `npm run build`.

## Non-goals

- No terminal UI, privilege escalation, service manager, container runtime, or
  unrestricted ambient environment mutation.
