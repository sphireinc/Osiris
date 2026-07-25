# Concurrency and I/O semantics

Source: ASYNC-PLAN-CHK.txt, IO.txt, and LANG.txt.

Document blocking behavior, suspension points, cancellation safe points,
timeout outcomes, task error propagation, and result ordering. Align claims
with the current goroutine-based implementation.

Acceptance: one authoritative docs page and examples cover spawn, await,
parallel, cancel, timeout, streaming I/O, and deterministic mode.

## Completion evidence

- The canonical React reference now has the `Tasks, cancellation, and streams`
  chapter. It documents blocking/suspension boundaries, cancellation safe
  points, timeout outcome/error behavior, task error propagation, and ordered
  parallel output.
- Checked examples cover `spawn`/`await`, `cancel`, `parallel`, `timeout`,
  async file wrappers, explicit stream handles, and runtime task-debug
  deterministic mode. The wording explicitly distinguishes cooperative
  cancellation from forced interruption and asynchronous task wrappers from
  nonblocking host I/O.
- Existing task, timeout, async-I/O, stream, and scheduler tests are the
  implementation evidence; the React reference check now requires six
  chapters and validates the canonical application build.
