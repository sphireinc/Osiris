# Task 157: Implement WASM host ABI, results, and capability execution

**Priority:** P0
**Dependencies:** 156,151
**Status:** Blocked

## Objective

Move standard-library, filesystem, environment, database, telemetry, testing,
regex, i18n, and capability-gated behavior from the custom Go payload runner
into explicit WASM imports and result-memory exchanges.

## Exact scope

- Own WASM import declarations, host ABI versioning, result/error records,
  capability validation, virtual providers, cancellation, and ownership.
- Define one import contract per supported capability; no ambient browser,
  Node, process, filesystem, network, or clock access may be inferred.
- Ensure host calls copy inputs and outputs, preserve structured errors, and
  cannot retain linear-memory pointers after a call.
- Add deterministic fake-provider fixtures for every declared profile module,
  plus rejection fixtures for undeclared modules and capabilities.

## Acceptance criteria

- WASM execution uses generated imports and ABI records for all claimed library
  behavior; `executeWASMProgram` is no longer the semantic backend.
- Capability revocation, cancellation, malformed host records, memory bounds,
  and provider cleanup are tested under normal and race execution.
- WASM support-matrix rows are promoted only with executable evidence and
  granular docs for each import and result shape.
