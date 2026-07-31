# Minimal TCP networking design

Task 011 introduces only explicit TCP resources. It does not expose raw file
descriptors, polling registrations, UDP, TLS, DNS caching, HTTP, callbacks, or
background accept loops.

## Resource model

`tcp_listen(address)` returns an owned listener. `tcp_accept(listener[, timeout_ms])`
blocks until it returns one owned duplex connection or a structured timeout /
cancelled error. `tcp_dial(address[, timeout_ms])` creates one owned duplex
connection. Connections use the existing `readline`, `readchunk`, `writechunk`,
`flush`, and `close` operations; their stream mode is `tcp`.

Listeners and connections are closed explicitly. `close` is idempotent: it
returns true exactly once, unblocks a pending local read/accept, and all later
operations fail with a closed-resource error. Closing a listener does not close
already accepted connections. Closing a connection flushes buffered writes only
when safe; a write/flush error is returned rather than hidden.

## Contexts, deadlines, and errors

All blocking operations inherit the current task/session context. An optional
non-negative integer timeout installs a temporary deadline for that one dial or
accept operation and is cleared before returning a reusable connection.

Go network errors are normalized through the existing structured error model:
deadline expiry maps to `Timeout`, context cancellation to `Cancelled`, refused
or unreachable addresses to `Unavailable`, malformed addresses to
`InvalidArgument`, and closed local resources to `IOError`. Error metadata may
contain a redacted operation/address, never a live connection object or secret
payload.

## Portable tuning boundary

New accepted and dialed connections enable `TCP_NODELAY` and TCP keepalive by
default. This is the low-latency default for the current explicit stream API.
The runtime's typed `SocketConfig` can explicitly override those values and set
non-negative connect/read/write deadlines, keepalive period, kernel buffer
sizes, and a bounded user-space write batch size. It is an internal runtime
contract until Osiris has a typed record/configuration value surface; the
language deliberately does not accept arbitrary raw socket options.

No deadline is installed implicitly. Context cancellation remains the portable
operation-cancellation guarantee. Deadline, native event-engine, zero-copy,
and platform-specific options must return a documented capability result or
structured unsupported error when they eventually become public; they must not
quietly alter portable stream behavior.

## Validation and native boundary

Loopback-only integration tests must cover listen/dial/accept, bidirectional
line/chunk transfer, close/unblock behavior, deadline, and cancellation.
The deterministic task-debug scheduler must retain observable ordering and
termination. Networking is interpreter/VM host I/O only; native lowering must
reject it until a separate native runtime ABI and capability policy exist.

## Runtime and ownership boundary

The current TCP implementation is hosted by Go. Its streams, buffered readers
and writers, task state, error values, and temporary transfer buffers are
ordinary Go-managed objects. Consequently Osiris cannot honestly promise a
no-GC runtime, deterministic reclamation, RAII destruction, pinned user
buffers, or the absence of garbage-collector pauses for host TCP programs.
Explicit `close` is a resource-lifecycle operation, not a proof that every
allocation involved in an operation has been reclaimed immediately.

A future native TCP runtime is a separate product boundary, not an optimization
flag for this implementation. It would need all of the following before making
such claims:

- compiler-lowered async state machines at actual suspension points;
- explicit ownership and lifetime rules for streams, tasks, and buffers,
  including pinning/retention while the operating system owns an I/O request;
- a portable completion/readiness abstraction with build-tagged epoll, kqueue,
  IOCP, and any optional `io_uring` adapters;
- cancellation, deadline, close, backpressure, error, and byte-count behavior
  proven equivalent to the portable contract above;
- worker scheduling and work stealing separated from a bounded blocking-task
  pool, so disk, DNS, TLS, and host calls cannot stall network workers; and
- allocation, tail-latency, throughput, and cleanup evidence on each supported
  platform.

Until those requirements and their tests exist, documentation and benchmark
results must describe the Go-hosted runtime precisely and must not imply
zero-copy, no-GC, RAII, or a native event-engine guarantee.
