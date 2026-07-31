# Task 114: Define deterministic state and mutation

**Priority:** P1
**Dependencies:** 045, 110, 111
**Status:** Blocked

## Objective

Introduce a deliberate, efficient state model for mutable locals and large
value updates without sacrificing determinism, alias safety, or task isolation.

## Required design decisions

- Decide syntax and semantics for mutable local bindings and assignment.
- Specify whether structs/collections can have mutable fields/elements or only
  controlled mutable builders.
- Define value identity, references, borrowing/ownership, escape behavior,
  closure captures, and copy-on-write boundaries.
- Define task transfer, shared state, cancellation, atomics, locks, and data
  race behavior. Deterministic mode must have a testable ordering contract.
- Specify mutation across errors, `try`, loops, streams, FFI handles, and
  resource cleanup.

## Implementation requirements

- Write the durable design in `docs/design/` before adding syntax.
- Extend alias, closure, purity, SSA, liveness, optimizer, and inference
  analyses; mutation must form explicit effect barriers.
- Implement interpreter/VM/native behavior with identical observation order.
- Add bounded in-place paths only when uniqueness/borrowing is proven; retain
  safe copy behavior otherwise.
- Add race diagnostics or explicit synchronization requirements rather than
  allowing accidental shared mutation.

## Tests and documentation

- Parse/format, assignment, loop state, closure capture, escape, alias,
  copy-on-write, task isolation, synchronized sharing, cancellation, and error
  unwinding fixtures.
- Deterministic scheduler tests and `go test -race` stress coverage.
- Optimizer non-reordering and native ownership/sanitizer tests.
- Migration guidance for current loop-shadowing patterns.

## Acceptance criteria

- [ ] The state model is documented before implementation and preserves
      deterministic observable behavior.
- [ ] Mutation is explicit in analysis/IR and cannot be unsafely optimized.
- [ ] Cross-backend, race, aliasing, and cleanup tests prove the contract.
- [ ] Documentation and full validation pass.

## Non-goals

- No unrestricted raw pointers, C-style pointer arithmetic, or undefined data
  races.
