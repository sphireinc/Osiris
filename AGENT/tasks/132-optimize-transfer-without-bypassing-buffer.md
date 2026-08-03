 ### P2 — Optimize transfer without bypassing buffered-reader ownership

  Change:

  - internal/osiris/stream.go
      - (*streamValue).transferToContext
      - readTCPContext
      - writeTCPContext

  - internal/osiris/tcp.go
      - tcpStreamWithConfig
      - possibly SocketConfig

  transferToContext uses a 32 KiB staging buffer but writes through a 4 KiB bufio.Writer by default. Go’s buffered writer has a direct-write path for sufficiently large writes, so
  this does not necessarily mean eight syscalls per iteration, but the behavior depends on current buffered state.

  A safe TCP fast path must account for bytes already prefetched by bufio.Reader. Directly reading from net.Conn while the buffered reader contains data would reorder or lose
  bytes.

  Potential approach:

  - expose or inspect buffered-reader state;
  - use a direct TCP transfer only when no buffered bytes exist;
  - otherwise preserve the buffered path;
  - align the writer buffer and transfer buffer for explicit bulk configurations;
  - benchmark against the existing 64 KiB bulk benchmark.

  Do not replace the buffered reader blindly with raw net.Conn.Read.
