 ### P1 — Reuse TCP read staging buffers

  Change:

  - internal/osiris/stream.go
      - streamValue
      - (*streamValue).readChunk
      - (*streamValue).readChunkContext
      - optionally (*streamValue).transferToContext

  readChunk and readChunkContext each allocate:

  buf := make([]byte, n)
  out := make([]Value, read)

  The []Value result is required by the current language representation, but the temporary byte buffer can be reused because readMu serializes reads on a stream.

  Candidate change:

  - add a bounded scratch byte buffer to streamValue, or use a carefully bounded sync.Pool;
  - reuse it in both context and non-context chunk readers;
  - retain the existing []Value conversion semantics;
  - avoid retaining an accidentally huge buffer indefinitely.

  transferToContext separately allocates a 32 KiB buffer per transfer invocation. That buffer is a good candidate for a bounded pool, provided the pool does not retain unbounded
 user-controlled sizes.
## Completion evidence

- Added a bounded per-stream scratch buffer for reads up to 64 KiB; larger reads remain one-shot allocations and are not retained.
- Added a bounded pooled 32 KiB transfer buffer and retained the required owned `[]Value` result conversion.
- Added close/read race coverage and passed focused package and race tests.
