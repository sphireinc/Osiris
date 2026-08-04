 ### P1 — Reduce avoidable deadline syscalls

  Change:

  - internal/osiris/tcp.go
      - SocketConfig.apply
      - (*tcpListener).acceptContextWithConfig
      - tcpDialWithConfig
      - tcpStreamWithConfig
      - tcpDeadline

  - internal/osiris/stream.go
      - readTCPContext
      - writeTCPContext

  Current overhead includes:

  - acceptContextWithConfig setting and then clearing a listener deadline for each timed accept;
  - readTCPContext setting and clearing a read deadline for each operation with a context deadline;
  - writeTCPContext doing the same for writes;
  - SocketConfig.validate being called by the dial/accept layer, then again by tcpStreamWithConfig, then again by SocketConfig.apply.

  The dial path can validate the same configuration up to three times:

  tcpDialWithConfig
    -> tcpStreamWithConfig
        -> SocketConfig.validate
        -> SocketConfig.apply
            -> SocketConfig.validate

  The accept path has the same layering.

  Refactor into:

  - one validated configuration path;
  - an internal applyValidated method;
  - deadline setup only when the effective deadline changes;
  - no deadline watcher/reset work for contexts that have neither cancellation nor a deadline.

  The default NoDelay and KeepAlive settings also cause socket option syscalls on every endpoint. Whether those should remain explicit defaults or be delegated to platform defaults
 needs benchmark and compatibility evidence before changing.
## Completion evidence

- Socket configuration is validated once and applied through the validated path; dial and accept no longer repeat configuration validation.
- Operations with no context deadline take a direct path without deadline syscalls. Cancellation-only operations install only a cancellation callback; deadline contexts use the one effective deadline and restore it afterward.
- Focused deadline, TCP, and race tests pass.
