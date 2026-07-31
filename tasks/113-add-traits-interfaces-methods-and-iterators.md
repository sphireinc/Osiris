# Task 113: Add traits, interfaces, methods, and iteration protocols

**Priority:** P1
**Dependencies:** 110, 111, 112
**Status:** Blocked

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
