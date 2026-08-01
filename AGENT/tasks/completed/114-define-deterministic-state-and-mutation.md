# Task 114: Define deterministic state and mutation

**Priority:** P1
**Dependencies:** 045, 110, 111
**Status:** Completed

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

- [x] The state model is documented before implementation and preserves
      deterministic observable behavior.
- [x] Mutation is explicit in analysis/IR and cannot be unsafely optimized.
- [x] Cross-backend, race, aliasing, and cleanup tests prove the contract.
- [x] Documentation and full validation pass.

## Completion evidence

- Design: `docs/design/STATE-AND-MUTATION.md` defines `const` static
  initializers, eager immutable `let` bindings, `var`, atomic assignment,
  value-semantic aggregate updates, closure cells, task snapshots, cancellation,
  error, loop, and cleanup behavior.
- Implementation: parser, formatter, type checking, interpreter, VM, SSA IR,
  native C, macro expansion, alias/purity/inference/shape analysis, optimizer,
  source maps, tracing, LSP, GPU planning, and visualization preserve the
  explicit constant and assignment contracts. Field and index mutation remain
  rejected by design; `with` and value-returning builders are the aggregate
  update boundary. `var` remains contextual so the statistical `var(...)`
  builtin is not broken.
- Tests: `internal/osiris/state_mutation_test.go` covers const/let/var
  metadata, static-initializer rejection, parse/format, loop state,
  interpreter/VM/IR/native parity, closure cells, task isolation, failed-RHS
  atomicity, and aggregate alias safety. Full `go test ./...`,
  `go test -race ./...`, `go vet ./...`, `go build ./...`,
  `node docs/scripts/check-reference.mjs`, and `git diff --check` pass.

## Non-goals

- No unrestricted raw pointers, C-style pointer arithmetic, or undefined data
  races.
