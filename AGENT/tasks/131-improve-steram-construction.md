 ### P2 — Improve stream construction and buffer sizing

  Change:

  - internal/osiris/tcp.go
      - tcpStream
      - tcpStreamWithConfig

  - internal/osiris/stream.go
      - streamValue
      - tcpReader
      - write/flush methods

  tcpStreamWithConfig currently evaluates:

  connection.RemoteAddr().String()
  connection.RemoteAddr().String()

  Cache the address string once.

  It also eagerly allocates both a bufio.Reader and bufio.Writer for every connection, even when a connection is used only for reading or only for writing. A lazy initialization
  design could reduce connection setup allocations, but it needs synchronization with readMu, writeMu, and close.

  SocketConfig.ReadBufferBytes controls the OS socket buffer through SetReadBuffer; it does not control the user-space bufio.Reader size. If larger buffered reads are desired,
  introduce an explicit user-space buffer setting rather than silently reusing the OS buffer field.
