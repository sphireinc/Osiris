# Task 122: Add structured logging and telemetry

**Priority:** P2
**Dependencies:** 014, 035, 045, 046, 116, 119
**Status:** Blocked

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

- [ ] Logs, metrics, and traces have bounded deterministic contracts.
- [ ] Secrets remain redacted across error, task, and exporter boundaries.
- [ ] Test capture is deterministic and ordinary program output is unchanged.
- [ ] Race, performance, docs, and full validation pass.

## Non-goals

- No mandatory vendor backend, ambient network exporter, or unbounded
  high-cardinality labels.
