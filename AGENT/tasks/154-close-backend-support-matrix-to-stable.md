# Task 154: Close every backend support-matrix row to stable

**Priority:** P0
**Dependencies:** 162,163,164,165,166,167,168
**Status:** Blocked until backend-specific closure tasks 162–168 complete

## Objective

Perform the final cross-backend conformance, documentation, and release gate so
every feature/backend cell in the Backend Support Matrix is `stable`.

## Exact scope and ownership

- Own `docs/src/support-matrix.json`, support-matrix evidence, matrix tests,
  cross-backend differential fixtures, and final task records.
- Reconcile parser, interpreter, VM, native C, WASM, GPU, formatter, and LSP
  claims against the evidence produced by tasks 162–168; no cell may be
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

This is a serialization gate. Tasks 162–168 may proceed in their own backend
chains, but task 154 starts only after every one of those closure records is
complete and must be owned by one integrator.

## Decomposition

- 162–163 close native C core/data and standard-library/host rows.
- 164–165 close WASM core/numeric and library/capability rows.
- 166–167 close GPU core/numeric and library/capability rows.
- 168 closes the LSP column after semantic and workspace IDE work.
- This task performs only cross-backend reconciliation, final evidence checks,
  and the all-stable release decision.
