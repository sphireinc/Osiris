# Task 308: Close GPU library and capability matrix rows

**Priority:** P0
**Dependencies:** 307,153
**Status:** Blocked

## Objective

Close GPU rows for signal processing, mapped standard-library operations,
capability negotiation, artifact ABI, and host/device resource policy.

## Acceptance criteria

- Every claimed mapped library member has a real-device result fixture,
  provider capability test, bounded memory evidence, and granular docs.
- Device absence, capability mismatch, cancellation, and device loss are
  structured and deterministic; no silent reference fallback is permitted.

## Current blocker

The native adapter does not yet provide real-device signal, mapped-library,
capability-wide, or host-resource fixtures. Those operations remain
capability-gated and no GPU support-matrix cells are promoted by the
reference-provider suite. Device-loss and broader capability-policy evidence
still require a real adapter run or a deterministic adapter test harness.

The deterministic fake-native harness now verifies capability rejection and
the explicit `cpu-reference` device identity. This proves provider policy and
ownership wiring only; it does not claim signal, mapped-library, or
host-resource hardware support and does not promote matrix rows.
