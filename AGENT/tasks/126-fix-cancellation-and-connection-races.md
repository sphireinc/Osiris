### P0 — Fix cancellation and connection ownership races

  Change:

  - internal/osiris/tcp.go
      - acceptWithCancellation
      - (*tcpListener).acceptContextWithConfig
      - tcpAcceptContextValue

  - internal/osiris/stream.go
      - readTCPContext
      - writeTCPContext
      - (*streamValue).close
      - closeStreamValue
      - (*streamValue).flush
      - (*streamValue).writeChunk

  - internal/osiris/runtime.go
      - registerContextBuiltins

  Findings:

  1. acceptWithCancellation can successfully accept a connection after the context has been canceled and still return it as a live stream. The successful-connection path does not
     re-check cancellation or close the connection. The result can be a stream that the caller does not expect to own.

     The fix should establish a clear rule:
      - if cancellation wins, close the accepted net.Conn before returning;
      - if acceptance wins, return the stream and transfer ownership explicitly;
      - test the race where cancellation occurs immediately after Accept succeeds.

  2. writechunk, flush, and close are not registered as context-aware builtins. readline, readchunk, and transfer receive context.Context, but writes do not.

     A blocked bufio.Writer.Write, Flush, or the flush performed by (*streamValue).close cannot be interrupted by the session context. This is particularly problematic because
     close flushes before calling conn.Close; a blocked flush can prevent the connection close that would otherwise unblock a pending peer operation.

     Candidate symbols to add or refactor:
      - writeChunkContextValue
      - flushStreamContextValue
      - closeStreamContextValue
      - registerContextBuiltins
      - (*streamValue).writeChunk
      - (*streamValue).flush
      - (*streamValue).close

     The close contract needs to be explicit: graceful flush with a bounded cancellation path, or immediate connection shutdown followed by best-effort buffer cleanup.

  3. closeTCPListenerValue checks listener.listener == nil under one lock and then calls (*tcpListener).close, which locks again. The state can change between the check and the
    close.
## Completion evidence

- Replaced accept cancellation channels with deadline-backed cancellation and closes an accepted connection if cancellation wins the ownership race.
- Made listener close atomic and idempotent, added context-aware stream write, flush, and close paths, and preserved cancellation error classification.
- Added concurrent listener-close and blocked-flush tests; focused package and race tests pass.

     Refactor closeTCPListenerValue and (*tcpListener).close into one atomic idempotent close operation that returns whether this caller performed the close.
