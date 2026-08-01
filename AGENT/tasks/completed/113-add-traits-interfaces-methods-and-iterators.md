# Task 113: Add traits, interfaces, methods, and iteration protocols

**Priority:** P1
**Dependencies:** 110, 111, 112
**Status:** Completed

## Objective

Add compile-time behavioral abstraction through explicit interfaces/traits,
receiver methods, and a standard iteration protocol.

## Required language contract

- Choose and document nominal versus structural conformance.
- Define interface/trait declarations, method signatures, implementations,
  receiver syntax, visibility, conflicts, and coherence/orphan rules.
- Define static and dynamic dispatch, interface values, equality, `null`,
  downcasts/type tests, and structured dispatch failures.
- Define `Iterator<T>`/`Iterable<T>` semantics used by `for`, including
  termination, errors, cancellation, laziness, and ownership.
- Specify how built-in lists, strings, tensors, streams, maps, sets, structs,
  enums, tasks, and user types implement protocols.

## Implementation requirements

- Integrate conformance into the shared checker and LSP.
- Preserve deterministic method selection and module ordering.
- Add IR/bytecode/native representations for method calls and interface
  values without compile-time execution.
- Document and test native vtable/witness ownership and codec policy.
- Keep existing `for` behavior compatible while routing it through the common
  iteration contract where possible.

## Tests and documentation

- Static/dynamic dispatch, embedded captures, generic bounds, cross-module
  implementations, iterator errors/cancellation, and conflict tests.
- Interpreter/VM/native parity plus generated artifact assertions.
- LSP completion, go-to-definition, references, rename, and signature help.
- Checked public examples and technical-reference entries.

## Acceptance criteria

- [x] Traits/interfaces and methods have deterministic conformance and dispatch
      rules.
- [x] `for` can consume the documented standard iteration protocol.
- [x] Generic bounds and interface values are checker- and backend-safe.
- [x] Tooling, docs, focused parity tests, and full validation pass.

## Non-goals

- No class inheritance, implicit extension-method search, or unrestricted
  runtime monkey patching.

## Implementation evidence

Implemented in this pass:

- Nominal `trait`/`interface` declarations, explicit `impl Trait for Type`
  blocks, receiver syntax, deterministic method/coherence diagnostics, trait
  assignments, receiver binding, and dynamic member dispatch.
- Shared lazy iteration cursor for lists, tuples, Unicode strings, tensors,
  maps, sets, readable streams, and one-shot tasks, including context
  cancellation before every cursor step and stream read. User structs can
  provide `iter()` and `next() -> Option<T>`.
- Generic trait-bound checking, formatter/macro/optimizer/inference/source-map
  propagation, LSP definitions/hover/rename/references/completion/signature
  help analysis, IR receiver metadata, and primitive native witness-table
  dispatch with owned tagged receiver captures. Capturing receiver methods and
  product-type receiver layouts remain source-spanned compile-time boundaries.
- Design documentation, canonical reference entries, and focused tests for
  interpreter/VM entry parity, standalone receiver methods, nominal bounds,
  conformance diagnostics, cross-module implementations, lazy user iterators,
  native witness executable parity, and LSP protocol features.

Validation evidence:

- `GOCACHE=/tmp/osiris-go-cache go test ./...`
- `GOCACHE=/tmp/osiris-go-cache go test -race ./...`
- `GOCACHE=/tmp/osiris-go-cache go vet ./...`
- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-go-modcache go build ./...`
- `node docs/scripts/check-reference.mjs`
- `git diff --check`

Native interface values retain their concrete tagged representation; equality
and codec behavior are therefore concrete-value based rather than witness
pointer based. Typed `match` patterns remain the checked type-test/downcast
surface, with no unchecked cast added.
