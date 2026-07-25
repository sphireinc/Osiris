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

## Validation and native boundary

Loopback-only integration tests must cover listen/dial/accept, bidirectional
line/chunk transfer, close/unblock behavior, deadline, and cancellation.
The deterministic task-debug scheduler must retain observable ordering and
termination. Networking is interpreter/VM host I/O only; native lowering must
reject it until a separate native runtime ABI and capability policy exist.
