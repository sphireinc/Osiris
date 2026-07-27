# Bounded stream transfer contract

This document defines the contract required before exposing a public
stream-to-stream transfer API. It applies to file and TCP stream resources and
is intentionally independent of a future native runtime.

## Proposed surface

`transfer(source, destination[, limit])` will copy at most `limit` bytes from
the current source position to the current destination position, returning the
number of bytes accepted by the destination. An omitted limit uses a bounded
implementation buffer; it does not mean that the runtime may allocate an
unbounded payload.

The operation does not take ownership of either stream. Callers must close
both resources themselves. Source and destination must be distinct, open,
and readable/writable respectively. The operation flushes the bytes it copied
before reporting success, so a successful byte count is visible to the peer or
underlying file. It never silently closes either endpoint.

## Backpressure and ordering

Transfer uses one fixed-size buffer and writes every partial read in order.
It waits for a full destination write before reading more source data. This
provides natural backpressure and bounded memory. Concurrent writes to the
same destination, or reads from the same source, are serialized; no operation
may interleave bytes with a transfer.

## Cancellation, close, and errors

Each loop checks the current task/session context. TCP reads and writes also
install a temporary context deadline so cancellation wakes blocked operations.
Closing either resource during a transfer must unblock it; the transfer returns
the structured closed-resource, cancellation, timeout, or I/O error produced
by the operation that failed. The returned byte count reflects only fully
accepted destination bytes; it never overstates partial writes.

The implementation must use separate read/write coordination from lifecycle
state. Holding the lifecycle lock for the duration of a copy is forbidden,
because that would prevent `close` from closing the underlying descriptor and
unblocking a blocked transfer.

## Native and platform boundary

The first implementation is portable buffered copying. It makes no `sendfile`,
`splice`, `io_uring`, or no-GC claim. A platform adapter may replace a proven
compatible file-to-socket path only when it preserves the byte count,
cancellation, close, deadline, and structured-error contract above; otherwise
it returns an explicit capability error or uses the buffered fallback.
