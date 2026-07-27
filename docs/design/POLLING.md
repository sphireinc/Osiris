# Portable polling foundation

Task 012 introduces an internal polling boundary for the Go-hosted runtime. It
is not a public Osiris API and it is not an epoll, kqueue, IOCP, or io_uring
implementation.

## Contract

`pollContext` is runtime-local and safe for concurrent registration, removal,
notification, waiting, and close. A caller registers a non-zero opaque token
with one or more interests: readable, writable, error, or hang-up. `wait`
returns token/event records, honours its context, accepts a non-negative
duration timeout, and optionally bounds the number of returned events.

Readiness is advisory. An adapter must still perform the operation and handle
ordinary close, EOF, or would-block behavior. Removing a registration prevents
future notifications; already queued events remain observable. Closing the
context wakes waiters, is idempotent, and prevents future registration.

## Current backend

The sole current backend is `portable`. It provides synthetic notifications
for runtime timers/tasks/resource adapters and exposes capabilities as
`{backend: "portable", synthetic_signals: true, handle_readiness: false}`.
Requesting `epoll`, `kqueue`, `iocp`, or any other adapter returns an explicit
unsupported-backend error. This makes platform availability inspectable without
silently selecting a different mechanism.

## Deliberate boundary

TCP streams and listeners still use their context-aware blocking operations
from task 011; they are not registered as poll handles in this increment.
Future build-tagged adapters may implement OS-handle registration behind this
contract, followed by carefully tested integration with network, streams,
timers, and scheduler suspension. Native C lowering remains unsupported until
a separate native runtime ABI and capability policy are defined.
