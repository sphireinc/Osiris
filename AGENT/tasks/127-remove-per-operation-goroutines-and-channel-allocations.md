 ### P1 — Remove per-operation goroutines and channel allocations

  Change:

  - internal/osiris/tcp.go
      - acceptWithCancellation
      - (*tcpListener).acceptContextWithConfig
      - tcpDeadline

  - internal/osiris/stream.go
      - readTCPContext
      - writeTCPContext

  Current behavior for each blocking operation:

  - allocates two channels;
  - starts a goroutine;
  - performs the network operation;
  - closes a channel;
  - waits for the watcher goroutine to stop.

  This happens even for context.Background(), whose Done() channel is nil and can never cancel. In the common non-cancelable path, the watcher goroutine and both channels are
  unnecessary.

  Recommended design order:

  1. Fast path for ctx == nil, ctx.Done() == nil, and no context deadline: call the underlying operation directly.
  2. If a context deadline or explicit timeout is available, use the socket deadline without starting a watcher goroutine.
  3. For cancellation-only contexts, use a cancellation callback or a shared polling/cancellation mechanism rather than one goroutine per read/write/accept.
  4. Preserve the current error precedence deliberately: distinguish an operation that completed successfully from one interrupted by cancellation immediately afterward.

  The main performance target is transferToContext, where one read watcher and one write watcher can be created for every buffer iteration.

  Relevant existing evidence points:

  - BenchmarkTCPLoopbackCancelledAccept
  - BenchmarkTCPLoopbackRoundTrip64B
  - BenchmarkTCPLoopbackRoundTrip64KiB
  - TestTCPCancellationTimeoutAndStructuredErrors

  Add allocation and goroutine-count assertions around the context.Background() fast path.
