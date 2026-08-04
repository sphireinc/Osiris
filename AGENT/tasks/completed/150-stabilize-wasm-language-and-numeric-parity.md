# Task 150: Stabilize WASM language and numeric parity

**Priority:** P1
**Dependencies:** 149,140,141
**Status:** Complete

## Objective

Lower the complete non-host language and numeric surface to WASM with exact
interpreter/VM parity.

## Exact scope and ownership

- Own WASM lowering/runtime files introduced by task 149 and WASM conformance
  fixtures; do not modify native C or GPU files.
- Cover control flow, functions/closures, typed/generic values, constants and
  mutation, errors/results/options, modules/macros, enums/patterns, tasks and
  cancellation where the WASM ABI supports them, exact numeric families,
  constructors, ranges, tensors, matrices, shape operations, linear algebra,
  reductions, and codecs.
- Define deterministic host-import behavior for capabilities and source-span
  rejection for forms not permitted by the selected WASM profile.
- Add differential execution tests against interpreter and VM, including
  ownership, limits, cancellation, and malformed inputs.

## Matrix transitions

Promote WASM core syntax, typed checking, state, types/numeric families,
errors/results/options, modules/macros, patterns, tasks, math, arrays, ranges,
tensor/matrix/shape/linear algebra/reductions, and data-helper rows to `stable`.

## Parallelism

Sequential after task 149. May run alongside native stdlib/host and GPU work.

## Completion evidence

- The WASM lowering/runtime covers constants, state, control flow,
  calls/closures, lists/tuples/enums, Result/Option propagation, patterns,
  deterministic modules, and the selected numeric/data profile.
- The runtime preserves owned values and enforces instruction, memory, output,
  artifact, and context-cancellation limits without interpreter fallback.
- `internal/osiris/wasm_test.go` includes language/numeric parity,
  repeatability, malformed-artifact/limit, capability-rejection, and
  source-span diagnostics.
- The focused WASM suite passes under the normal backend gate.
