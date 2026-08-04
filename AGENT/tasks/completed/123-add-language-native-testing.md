# Task 123: Add a language-native testing ecosystem

**Priority:** P1
**Dependencies:** 026, 027, 035, 040, 043
**Status:** Complete

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

- [x] Osiris users can discover and run project tests without writing Go.
- [x] JSON output, coverage, fuzz artifacts, benchmarks, and goldens are stable
      and reproducible.
- [x] Backend selection and failures use public execution contracts.
- [x] CLI, race, docs, packaging, and full validation pass.

## Non-goals

- No distributed test farm, browser runner, or third-party mocking framework in
  the first slice.

## Task-123 implementation slice (2026-08-02)

Implemented within the scoped CLI/runtime files:

- deterministic recursive discovery and a stable source inventory for future
  LSP/code-lens consumers;
- source-preserving `// osiris:` declarations for named tests, canonical output
  assertions, source locations, table-driven cases, skip, expected failure,
  fixtures/cleanup, goldens, benchmarks, and fuzz targets;
- public interpreter, VM, and native C backend selection, per-case timeout and
  cancellation, bounded workers, deterministic result ordering, integration
  capability selection, and inventory-only cache policy;
- versioned JSON test events, stable benchmark JSON, interpreter trace-backed
  coverage with source maps, static VM/native coverage maps, deterministic fuzz
  corpus generation, bounded shrinking, replay sources, and explicit golden
  update mode;
- self-hosted task-123 fixtures and focused Go/CLI tests for inventory,
  parameterization, assertions, fixtures, cleanup, goldens, coverage, VM,
  benchmark bounds, fuzz bounds, timeout, filtering, and JSON inventory/events.

Granular syntax, result fields, artifacts, and CLI flags are documented in
`AGENT/docs/design/TASK-123-LANGUAGE-TESTING.md` and
`AGENT/docs/design/TASK-123-CLI-AND-ARTIFACTS.md`.

The v1 declaration syntax is intentionally source-preserving `// osiris:`
directives; grammar declarations and LSP code lenses remain task-125 scope.
VM/native coverage reports retain stable static source maps and explicitly
report probe availability rather than inventing runtime coverage. Native race
instrumentation remains a toolchain limitation and is surfaced as a clear
backend diagnostic. These are versioned behavior, not silent fallbacks.

## Completion record — 2026-08-02

The workflow, deterministic inventory, bounded parallel execution,
setup/body/cleanup fixtures, table cases, skips, expected failures, goldens,
benchmark JSON, coverage reports, fuzz corpus/shrink/replay, artifacts, cache
policy, timeout/cancellation, JSON events, and CLI flags are implemented in
`internal/osiris/task123_language_test_*.go` and `cmd/osiris/main.go`. Public
interpreter, VM, and native selection paths are explicit and never silently
fall back.

Granular workflow and CLI/API documentation is in
`AGENT/docs/design/TASK-123-LANGUAGE-TESTING.md`,
`AGENT/docs/design/TASK-123-CLI-AND-ARTIFACTS.md`, and the public `osiris test`
entries in `docs/src/std-task-reference.js`. Focused tests, full repository
tests, race-focused tests, vet/build, docs navigation/reference checks, and
checked examples passed on 2026-08-02.

The test-scoped `std/testing` API is also complete: `assert`, `assert_equal`,
`error`, `errorf`, `fatal`, `fatalf`, `skip`, `skipf`, `log`, `logf`, `name`,
and `failed` are implemented with deterministic result logs, structured
assertions, and cleanup-aware fatal/skip control flow. Each symbol is listed
individually in the public reference.
