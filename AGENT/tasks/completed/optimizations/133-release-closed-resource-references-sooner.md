 ### P2 — Release closed resource references sooner

  Change:

  - internal/osiris/stream.go
      - (*streamValue).close
      - streamValue

  (*streamValue).close marks the stream closed and closes the underlying file/connection, but leaves conn, reader, writer, and file references attached to the stream object. A
  retained language Value therefore retains the buffered reader/writer and their backing connection references after close.

  After the close transition has safely detached the resources, clear the fields that are no longer needed. This must be coordinated with concurrent operations that have already
  copied local references.

  The Session structure has no stream registry or deterministic session-level cleanup hook, so explicit close remains the only visible ownership release path. That makes this
  cleanup and cancellation behavior important for abandoned streams.

  ## Recommended implementation sequence

  1. Fix accepted-connection cancellation ownership and make close behavior interruptible.
  2. Remove the context.Background() watcher-goroutine path and consolidate deadline handling.
  3. Add context-aware write, flush, and close dispatch.
  4. Reuse chunk/transfer staging buffers.
  5. Add string write fast paths.
  6. Consolidate configuration validation and measure socket-option/deadline syscall costs.
  7. Consider lazy stream buffers and post-close reference detachment.

  Validation should use the existing TCP parity/cancellation tests plus new race tests, allocation benchmarks, and repeated canceled-operation stress. The current worktree contains
 unrelated modifications, including internal/osiris/runtime.go, session.go, and value.go; none were changed during this analysis.
## Completion evidence

- Close now establishes the closed state under the write ownership lock, flushes safely, detaches connection/file/reader/writer/closer/handle and scratch references, and then releases the underlying resources.
- The lock order prevents close/write deadlock; scratch detachment has its own synchronization so concurrent reads remain race-free.
- Focused close, transfer, and race tests pass.
