# Task 148B: Completed advanced native C parity boundaries

**Parent:** 148 — Stabilize native C host capabilities
**Status:** Complete; all four 148B scopes are implemented, documented, and
validated. Windows execution evidence remains a parent-task platform boundary
on this macOS host and is not part of 148B closure.
**Dependencies:** 148
**Priority:** P1

## Purpose

Track the native-C boundaries intentionally excluded from the current task-148
implementation pass. These items require their own design, capability policy,
cross-platform evidence, and differential tests. They must not be counted as
complete merely because the POSIX resource ABI is executable.

## Scope

- Process-group cancellation with owned group/session lifetimes, escalation,
  timeout behavior, and cancellation cleanup.
- Complete inherited-environment overlay semantics for process options,
  including deterministic ordering, explicit deletion, and parity when
  `inherit_env` is combined with an explicit environment map.
- Producer-aware capability checks for generic `readchunk`, `writechunk`, and
  `close`, so file, process, watch, socket, TLS, and HTTP resources do not
  require unrelated network grants.
- Native HTTP redirect policy and generated-language closure-handler
  execution, including limits, cancellation, handler ownership, and error
  propagation.
- Linux generated-C execution and complete cross-platform differential
  evidence. This requires a Linux kernel/userspace runner, sanitizer results,
  and comparison against the macOS/POSIX execution contract.

## Acceptance criteria

- Every item has an explicit capability, ownership, cancellation, and error
  contract in `docs/design/`.
- Process-group and environment behavior has interpreter/VM/native
  differential coverage, including cancellation and cleanup edges.
- Generic resource operations authorize against the resource producer rather
  than an unrelated capability family, with denial diagnostics tested.
- HTTP redirects and generated closure handlers execute natively with bounded
  behavior and parity tests.
- Native generated C compiles and executes under Linux, including sanitizer
  coverage and cross-platform differential fixtures.
- The support matrix and task-148 completion record are updated only after all
  evidence passes; no deferred item is silently promoted by documentation.

## Suggested decomposition

1. 148B-1 — Process groups, cancellation escalation, and environment overlay.
2. 148B-2 — Producer-aware generic resource capability authorization.
3. 148B-3 — Native HTTP redirects and generated closure handlers.
4. 148B-4 — Linux runner, generated-C execution, sanitizer, and differential
   evidence. The focused test and container runner are now present in
   `internal/osiris/task148b_linux_parity_test.go` and
   `scripts/test-native-linux.sh`; this child remains open until the runner
   produces a recorded Linux result.

## 148B-4 evidence contract

The Linux evidence is deliberately deterministic. The focused test compares
interpreter, VM, and generated-C output for scalar control flow, standard
library values, and codec/numeric values against fixed expected output. It
does not use host IDs, clocks, environment variables, network services, or
host-dependent paths. The sanitizer test sets
`OSIRIS_C_SANITIZERS=address` and executes a generated C binary, so it covers
compile, link, process execution, and AddressSanitizer startup rather than
only checking that a flag can be parsed.

From the repository root, run:

```sh
scripts/test-native-linux.sh
```

The runner uses `golang:1.26-bookworm` on `linux/arm64` by default, mounts the
repository at `/workspace`, and runs the exact focused Go tests with isolated
`GOCACHE` and `GOMODCACHE` directories. Override `OSIRIS_LINUX_IMAGE` and
`OSIRIS_LINUX_PLATFORM` when a different Linux toolchain or architecture is
required. A missing Docker daemon is an environmental blocker; the script
exits with status 2 and reports the daemon result instead of claiming Linux
evidence.

The host-side check is useful for confirming the test is well-formed but is
not Linux evidence:

```sh
GOCACHE=/tmp/osiris-task148b-cache go test ./internal/osiris \
  -run '^TestTask148BLinux(GeneratedCDeterministicParity|SanitizerContract)$' \
  -count=1 -timeout=600s
```

On macOS this command skips both tests because generated-C Linux execution
must be performed by the Linux runner. Results should be recorded with the
toolchain line, parity test result, sanitizer test result, and exact image
and platform override in the task completion record.

## Completion record

148B-1 is complete in `internal/osiris/cbackend_process_runtime.go`.
POSIX-spawned children own a `setsid` session/process group; kill, timeout, and
resource-exit cleanup target the group, escalate TERM to KILL after a bounded
100 ms grace period, reap the leader, and release group ownership. `env` is a
deterministic sorted overlay when `inherit_env` is true, a complete replacement
when false, and accepts `null` as explicit deletion. Coverage is in
`internal/osiris/task148b_process_test.go`.

148B-2 is complete in `internal/osiris/cbackend_native.go`. The emitter tracks
resource producers through SSA copies, process members, and HTTP body-stream
members. Generic `readchunk`, `writechunk`, and `close` require the producer's
capability; unknown producers fail with a source-spanned diagnostic. Coverage
is in `internal/osiris/task148b_native_capability_test.go`.

148B-3 is complete in `internal/osiris/cbackend_tls_http_runtime.go`. Native
HTTP follows bounded 301/302/303/307/308 redirects with URL resolution,
method/body and cross-authority credential policy, intermediate stream cleanup,
and cancellation checks. Generated `O_CLOSURE` handlers execute through
`ocall`; response validation, handler errors, cancellation, and connection
cleanup are covered by `TestNativeTLSHTTPRuntimeFollowsBoundedRedirects` and
`TestNativeTLSHTTPRuntimeExecutesGeneratedClosureHandler`.

148B-4 is complete. `scripts/test-native-linux.sh` executed the deterministic
interpreter/VM/generated-C differential fixtures and generated-C AddressSanitizer
fixture in Colima's `linux/arm64` container with Go 1.26.5 and GCC 12.2:

```text
--- deterministic generated-C parity ---
ok github.com/sphireinc/osiris/internal/osiris 3.976s
--- AddressSanitizer generated-C parity ---
ok github.com/sphireinc/osiris/internal/osiris 1.473s
Linux generated-C evidence passed.
```

The complete local focused validation was:

```sh
GOCACHE=/tmp/osiris-148b-process2-cache go test ./internal/osiris \
  -run '^TestTask148BProcess' -count=1 -timeout=600s
GOCACHE=/tmp/osiris-148b-cap-cache go test ./internal/osiris \
  -run '^TestTask148BGeneric' -count=1 -timeout=600s
GOCACHE=/tmp/osiris-148b-http-cache go test ./internal/osiris \
  -run '^TestNativeTLSHTTPRuntime(FollowsBoundedRedirects|ExecutesGeneratedClosureHandler|CompilesAsIsolatedCComponent|ExecutesHTTPProviderContract)$' \
  -count=1 -timeout=600s
scripts/test-native-linux.sh
```

The parent task must no longer describe these four boundaries as deferred.
Parent task 148 may remain open for the separately documented Windows compiler
and execution evidence boundary.
