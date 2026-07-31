# Task 118: Expand networking with HTTP, TLS, UDP, and DNS

**Priority:** P2
**Dependencies:** 011, 012, 035, 045, 046, 116, 117
**Status:** Blocked

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

- [ ] UDP, DNS, TLS, and HTTP contracts are bounded and cancellation-aware.
- [ ] Secure defaults and structured error mappings are documented and tested.
- [ ] Resource cleanup, race, platform, backend, and docs validation pass.

## Non-goals

- No production claim for HTTP/2/3, QUIC, WebSocket, proxy auto-discovery, or
  system credential stores unless separately scoped and tested.
