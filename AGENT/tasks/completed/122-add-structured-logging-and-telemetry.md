# Task 122: Add structured logging and telemetry

**Priority:** P2
**Dependencies:** 014, 035, 045, 046, 116, 119
**Status:** Complete

## Objective

Add structured logs, metrics, and traces with deterministic test capture,
context propagation, redaction, and pluggable exporters.

## Required contract

- Structured logger with levels, stable field ordering, component, source,
  timestamp/clock policy, correlation/task/trace IDs, and error records.
- Metrics primitives for counters, gauges, and histograms with explicit label
  limits and deterministic snapshots.
- Trace/span lifecycle with parent context, attributes, events, status,
  cancellation, and cross-task propagation.
- Exporter interface and a built-in deterministic in-memory/JSON exporter;
  external protocols remain separately capability-gated.

## Implementation requirements

- Reuse structured error metadata and injected clocks.
- Enforce redaction, field/label/value/cardinality/queue limits, backpressure,
  and exporter failure policy.
- Logging/telemetry must not change program semantics or introduce
  nondeterministic output into ordinary `emit`.
- Integrate scope-bound span cleanup, scheduler/task context, CLI diagnostics,
  and FFI exporters.

## Tests and documentation

- Stable ordering, clock injection, context propagation, redaction,
  cardinality limits, queue overflow, exporter failure, cancellation, race,
  and benchmark overhead tests.
- Checked standard-library examples and operational guidance.

## Acceptance criteria

- [x] Logs, metrics, and traces have bounded deterministic contracts.
- [x] Secrets remain redacted across error, task, and exporter boundaries.
- [x] Test capture is deterministic and ordinary program output is unchanged.
- [x] Race, performance, docs, and full validation pass.

## Non-goals

- No mandatory vendor backend, ambient network exporter, or unbounded
  high-cardinality labels.

## Completion record — 2026-08-02

`internal/osiris/task122_telemetry.go`, `task122_metrics.go`, and
`task122_trace.go` implement bounded structured logs, counters/gauges/
histograms, parent-aware spans, deterministic memory/JSON exporters, injected
clocks, runtime/task context propagation, redaction, queue backpressure, and
exporter failure policy. `std/telemetry` and `std/log` are registered in the
language module loader. Public module calls are covered for interpreter and VM
in `task120_123_stdlib_test.go`; concurrency, redaction, failure policy,
ordering, cancellation, exporter, and race tests are in
`task122_telemetry_test.go`.

The complete symbol contract is documented in
`AGENT/docs/design/TASK-122-STRUCTURED-LOGGING-TELEMETRY.md` and granular
public entries in `docs/src/std-task-reference.js`. Full Go, race-focused,
vet/build, documentation, and checked-example validation passed on 2026-08-02.
