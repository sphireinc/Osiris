# Task 310: Close every backend support-matrix row to stable

**Priority:** P0
**Dependencies:** 154,305,306,307,308,400
**Status:** Blocked until task 154 and deferred backend tasks 300–308 and 400 complete

## Objective

Perform the final cross-backend conformance, documentation, and release gate so
every feature/backend cell in the Backend Support Matrix is `stable`.

## Exact scope and ownership

- Own `docs/src/support-matrix.json`, support-matrix evidence, matrix tests,
  cross-backend differential fixtures, and final task records.
- Reconcile parser, interpreter, VM, native C, WASM, GPU, formatter, and LSP
  claims against the evidence produced by task 154, tasks 300–308, and task 400; no cell may be
  changed to `stable` from prose alone.
- Require every feature row currently marked `partial`, `experimental`, or
  `unsupported` to have maintained tests, deterministic limits, diagnostics,
  cancellation/ownership evidence, and documented behavior.
- Add CI checks that fail if a new feature/backend cell is introduced without a
  stable implementation, tests, docs, and exact backend ownership.
- Verify REPL/file parity, parse/format/parse stability, interpreter/VM/native/
  WASM/GPU differential results, LSP protocol fixtures, sanitizer coverage,
  and production documentation examples.
- Update `AGENT/tasks/README.md`, `AGENT/tasks/INDEX.md`, and the completion
  records only after all prior tasks have complete evidence.

## Acceptance criteria

- Every cell in `docs/src/support-matrix.json` is exactly `stable`.
- `internal/osiris/support_matrix_test.go` rejects any non-stable cell for the
  completed parity program.
- Full Go tests, race tests, native sanitizer tests, WASM/GPU artifact and
  execution tests, formatter tests, LSP protocol tests, docs reference checks,
  docs navigation checks, examples, vet, and builds pass.
- No backend invokes another backend's evaluator as a hidden fallback.
- All completed parity tasks are physically moved to `AGENT/tasks/completed/`
  and indexed with complete status/evidence.

## Parallelism

This is a serialization gate. Native C and LSP work proceeds through task 154;
WASM and GPU work proceeds through tasks 300–308; environment-gated evidence
proceeds through task 400. Task 310 starts only after every dependency has a
complete record and must be owned by one integrator.

## Decomposition

- 154 closes the Native C and LSP columns after tasks 160–163 and 168.
- 305–306 close WASM core/numeric and library/capability rows.
- 307–308 close GPU core/numeric and library/capability rows.
- 400 supplies Windows/Linux/real-device evidence that is unavailable on the
  current machine.
- This task performs only cross-backend reconciliation, final evidence checks,
  and the all-stable release decision after the Native C/LSP gate and deferred
  backend/environment tasks are complete.
