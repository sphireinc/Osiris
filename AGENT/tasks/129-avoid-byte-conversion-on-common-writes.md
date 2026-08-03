 ### P1 — Avoid byte conversion allocations on common writes

  Change:

  - internal/osiris/numeric.go
      - bytesFromValue

  - internal/osiris/stream.go
      - (*streamValue).writeChunk
      - writeChunkValue

  For string input, bytesFromValue executes:

  return []byte(v.str), nil

  That creates a fresh byte slice for every writechunk call. Lists and tensors also go through packBytes, which creates a staging representation.

  Candidate design:

  - fast-path string values through bufio.Writer.WriteString;
  - retain the existing byte-count behavior;
  - use a direct or borrowed representation for already-byte-compatible list/tensor inputs where safe;
  - keep a copying path for values whose backing storage may be mutated or whose lifetime does not cover the write.

  This is especially relevant to BenchmarkTCPLoopbackRoundTrip64B, where payload conversion can be a significant portion of the per-message allocation cost.
