# Task 167: Close GPU library and capability matrix rows

**Priority:** P0
**Dependencies:** 166,153
**Status:** Blocked

## Objective

Close GPU rows for signal processing, mapped standard-library operations,
capability negotiation, artifact ABI, and host/device resource policy.

## Acceptance criteria

- Every claimed mapped library member has a real-device result fixture,
  provider capability test, bounded memory evidence, and granular docs.
- Device absence, capability mismatch, cancellation, and device loss are
  structured and deterministic; no silent reference fallback is permitted.
