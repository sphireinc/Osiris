# Socket networking primitives

Source: ASYNC-PLAN-CHK.txt and IO.txt.

Define minimal TCP client/server capabilities with contexts, deadlines, close
rules, and structured errors. Do not expose polling internals yet.

Acceptance: loopback client/server integration tests, timeout/cancellation
tests, and documentation identify blocking and task-aware operations.

## Active increment: explicit TCP resources

- Added owned TCP listeners plus TCP-backed duplex streams, with
  `tcp_listen`, `tcp_accept`, and `tcp_dial` also exported from `std/net`.
  Connections deliberately reuse `readline`, `readchunk`, `writechunk`,
  `flush`, and idempotent `close`.
- `TestTCPNetworkLoopbackResources` covers loopback accept/dial and
  bidirectional line transfer. It skips only when a host sandbox forbids
  listener sockets; ordinary developer/CI hosts execute it.
- `docs/design/TCP-NETWORKING.md` and the canonical React reference define
  ownership, close behavior, deadlines, blocking behavior, task-aware limits,
  and the native unsupported boundary. Reference search verifies `tcp_dial`.

## Completion evidence — 2026-07-25

- Interpreter and VM both invoke context-aware TCP builtins with their active
  session context. `tcp_accept`, `tcp_dial`, and TCP `readline`/`readchunk`
  convert cancellation into a temporary socket deadline, then return the
  original context cancellation/deadline error.
- TCP listener ownership is synchronized so `close` is idempotent and races
  safely with accept. Network errors normalize to `cancelled`, `timeout`,
  `invalid_argument`, `unavailable`, `network_error`, or `io_error` as
  appropriate through the standard error model.
- `TestTCPCancellationTimeoutAndStructuredErrors` covers loopback accept
  cancellation, deadline expiry, blocked TCP read expiry, and classification;
  `TestTCPNetworkLoopbackResources` covers bidirectional transfer.
- The canonical React `std/net` entries now state task/session cancellation
  and structured error behavior. `docs/design/TCP-NETWORKING.md` remains the
  durable resource contract.

Task 049 is the downstream benchmark-and-tuning follow-up. Do not add socket
options, pooling, batching, or platform polling shortcuts while the task-011
correctness contract remains incomplete.
