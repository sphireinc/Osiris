# Task 123: Add a language-native testing ecosystem

**Priority:** P1
**Dependencies:** 026, 027, 035, 040, 043
**Status:** Ready

## Objective

Give Osiris projects a first-class `osiris test` workflow with language-level
test discovery, assertions, fixtures, parameterization, benchmarks, coverage,
fuzzing, and golden testing.

## Required user experience

- Define test declaration/file conventions and deterministic discovery order.
- Add assertion helpers that retain structured errors and precise source
  locations.
- Support setup/cleanup fixtures using task 116 when available, table-driven
  tests, skips, expected failures, and bounded parallel execution.
- Add benchmark declarations with warmup/sample metadata compatible with the
  existing benchmark JSON format.
- Add statement/branch/function coverage with stable source maps.
- Add bounded fuzz targets with seed corpus, reproducible failure artifacts,
  shrinking policy, and replay.
- Add golden/snapshot helpers with explicit update mode and stable encoding.

## CLI and tooling

- `osiris test [packages...]` must define exit codes, stdout/stderr, JSON event
  schema, filtering, timeouts, deterministic mode, race/backend selection, and
  cache behavior.
- Integrate LSP discovery/code lenses later through task 125; expose a stable
  internal test inventory now.
- Tests must run through public interpreter/VM/native paths rather than private
  evaluator hooks where backend selection is requested.

## Tests and documentation

- Self-hosted fixture projects covering discovery, assertions, failures,
  cleanup, parameterization, benchmarks, coverage, fuzz replay, goldens,
  malformed files, cancellation, and JSON output.
- CI examples and complete `newdocs/` testing chapter.

## Acceptance criteria

- [ ] Osiris users can discover and run project tests without writing Go.
- [ ] JSON output, coverage, fuzz artifacts, benchmarks, and goldens are stable
      and reproducible.
- [ ] Backend selection and failures use public execution contracts.
- [ ] CLI, race, docs, packaging, and full validation pass.

## Non-goals

- No distributed test farm, browser runner, or third-party mocking framework in
  the first slice.
