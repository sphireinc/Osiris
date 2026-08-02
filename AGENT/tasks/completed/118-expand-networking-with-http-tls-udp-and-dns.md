# Task 118: Expand networking with HTTP, TLS, UDP, and DNS

**Priority:** P2
**Dependencies:** 011, 012, 035, 045, 046, 116, 117
**Status:** Complete

## Current implementation evidence

The hosted `std/net` module exposes bounded UDP bind/connect/send/receive,
sorted DNS lookup with explicit `system`/`go` resolver policy, verified TLS
client dialing and server configuration, bounded HTTP/1.1 request/get plus
request/response body streams, and cancellable static and dynamic HTTP
responders. UDP peer/truncation metadata and cancellation, DNS ordering and
cancellation, explicit-root TLS verification, deterministic HTTP headers,
redirect/body/header limits, malformed request framing rejection, structured
network errors, native source diagnostics, and sandbox-safe local tests are
covered.

Dynamic HTTP handlers receive bounded deterministic request maps and return
bounded response maps. Captured session state is serialized per handler,
background emits are denied, and cancellation performs graceful shutdown.
Streaming request and response bodies enforce the 8 MiB bound without exposing
overflow bytes. Invalid header names/control characters and unsafe TLS inputs
are rejected before host I/O is exposed.

## Objective

Provide a bounded, cancellable application-networking standard library beyond
the current TCP listen/accept/dial foundation.

## Required surface

- UDP sockets with bind, connect, send/receive, deadlines, peer addresses, and
  truncation reporting.
- DNS lookup with deterministic result ordering, cancellation, explicit
  resolver policy, and structured errors.
- TLS client/server configuration with certificate roots, verification,
  hostname policy, protocol versions, ALPN, and secure defaults.
- HTTP/1.1 client and server primitives for requests, responses, headers,
  streaming bodies, redirects, limits, cancellation, and graceful shutdown.
- Define whether HTTP/2 is included or explicitly deferred.

## Implementation requirements

- Put OS polling behind portable interfaces/build tags.
- Enforce header/body/connection/time/resource limits and reject ambiguous
  framing, invalid names, and unsafe defaults.
- Integrate scope-bound cleanup, task cancellation, streams, structured
  errors, capability reporting, and FFI/native policy.
- Preserve deterministic header/result ordering where observable.
- Native support must be real or an honest source-spanned capability
  diagnostic; never proxy through another Osiris process.

## Tests and documentation

- Local-only UDP, DNS fixture, TLS certificate, HTTP client/server, malformed
  protocol, cancellation, timeout, shutdown, and race tests.
- Security corpus for request smuggling/framing, certificate validation,
  redirect limits, and decompression/body limits.
- Cross-platform tests and exact reference entries with runnable examples.

## Acceptance criteria

- [x] UDP, DNS, TLS, and HTTP contracts are bounded and cancellation-aware.
- [x] Secure defaults and structured error mappings are documented and tested.
- [x] Resource cleanup, race, platform, backend, and docs validation pass.

## Completion evidence

- `GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -count=1 -timeout=600s`
- `GOCACHE=/tmp/osiris-go-cache go test -race ./internal/osiris -count=1 -timeout=600s`
- `CC="$(brew --prefix llvm)/bin/clang" OSIRIS_C_SANITIZERS=address GOCACHE=/tmp/osiris-asan-cache go test ./internal/osiris -run 'Test(NativeRejects|HTTP|UDP|DNS|TLS)' -count=1 -timeout=600s`
- `GOCACHE=/tmp/osiris-go-cache go vet ./...` and `GOCACHE=/tmp/osiris-go-cache go build ./...`
- With Go 1.26.1 and Node 24: `npm run check-reference`, `npm run check-examples`, and `npm run build` in `docs/`.
- The TLS loopback and raw malformed-framing tests skip only when the current sandbox denies socket creation; they run in normal local/CI networking environments. Native C remains an intentional source-spanned unsupported boundary for host networking.

## Non-goals

- No production claim for HTTP/2/3, QUIC, WebSocket, proxy auto-discovery, or
  system credential stores unless separately scoped and tested.
