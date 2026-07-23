# Concurrency and I/O semantics

Source: ASYNC-PLAN-CHK.txt, IO.txt, and LANG.txt.

Document blocking behavior, suspension points, cancellation safe points,
timeout outcomes, task error propagation, and result ordering. Align claims
with the current goroutine-based implementation.

Acceptance: one authoritative docs page and examples cover spawn, await,
parallel, cancel, timeout, streaming I/O, and deterministic mode.
