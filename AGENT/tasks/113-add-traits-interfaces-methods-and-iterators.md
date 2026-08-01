# Task 113: Add traits, interfaces, methods, and iteration protocols

**Priority:** P1
**Dependencies:** 110, 111, 112
**Status:** Blocked — core language/runtime slice implemented; native ABI and extended LSP tooling remain

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

- [ ] Traits/interfaces and methods have deterministic conformance and dispatch
      rules.
- [ ] `for` can consume the documented standard iteration protocol.
- [ ] Generic bounds and interface values are checker- and backend-safe.
- [ ] Tooling, docs, focused parity tests, and full validation pass.

## Non-goals

- No class inheritance, implicit extension-method search, or unrestricted
  runtime monkey patching.

## Implementation evidence and remaining blockers

Implemented in this pass:

- Nominal `trait`/`interface` declarations, explicit `impl Trait for Type`
  blocks, receiver syntax, deterministic method/coherence diagnostics, trait
  assignments, receiver binding, and dynamic member dispatch.
- Shared iteration boundary for lists, tuples, Unicode strings, tensors, maps,
  sets, and readable streams, including context cancellation at stream reads.
- Generic trait-bound checking, formatter/macro/optimizer/inference/source-map
  propagation, LSP definitions/hover/rename analysis, IR receiver metadata,
  and an explicit native compile-time rejection for dispatch without a vtable
  ABI.
- Design documentation, canonical reference entries, and focused tests for
  interpreter/VM entry parity, standalone receiver methods, nominal bounds,
  conformance diagnostics, and the native IR boundary.

Validation evidence:

- `GOCACHE=/tmp/osiris-go-cache go test ./...`
- `GOCACHE=/tmp/osiris-go-cache go vet ./...`
- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-go-modcache go build ./...`
- `node docs/scripts/check-reference.mjs`
- `git diff --check`

The task remains blocked until the following acceptance items are implemented
and tested: native vtable/witness ownership and codec representation,
completion/signature-help LSP requests, cross-module implementation/tooling
tests, and the full Iterator<T>/Iterable<T> lazy protocol for task/user types
plus explicit downcast/type-test behavior.
