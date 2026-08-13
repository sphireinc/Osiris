# Task 07 — Harden the Type System to Match the Rest of the Compiler

## Objective

Bring the type system to the same engineering maturity as Osiris's backend, diagnostics and tooling infrastructure. This task is broader than "remove dynamic": establish correctness properties, canonical typing rules, diagnostics, trait/generic behavior, representation decisions and tests appropriate for a serious statically typed language.

Task 03 defines the user-visible static-typing direction. Tasks 08/09 define the canonical internal representation. This task closes the remaining correctness and usability gaps.

## Required type-system specification

Write a type-system design document covering at minimum:

- primitive scalar types;
- literal typing/defaulting;
- integer signedness/width;
- floating families;
- exact numeric types;
- complex/fixed/modular/interval types;
- quantities/units;
- strings/text;
- lists, arrays, maps, sets;
- tuples if present;
- structs;
- enums/sum types;
- type aliases;
- newtypes;
- functions/closures;
- `Option<T>`;
- `Result<T,E>`;
- `Lazy<T>`;
- tasks/futures;
- tensors including dimensions/dtype policy;
- generic type parameters;
- trait/interface constraints;
- `dynamic`/FFI boundary if retained;
- bottom/never type if used;
- null/nil policy;
- error-recovery compiler sentinel type (not user-visible).

For each type category document equality, assignment compatibility, conversions, hashability, ordering, serialization eligibility, FFI eligibility and runtime representation class where relevant.

## Conversion lattice

Create a single conversion/coercion policy. Do not let individual builtins or backends invent their own rules.

Classify conversions:

- identity;
- lossless implicit widening;
- explicit checked conversion;
- explicit lossy conversion;
- invalid conversion.

Numeric promotion must be deterministic. Avoid C's surprising integer promotions.

Physical quantities/units must not silently coerce dimensionally incompatible values.

## Control-flow typing

Implement or verify typed control-flow joins:

- `if` branches;
- `match` arms;
- early return;
- `try`/error propagation;
- loops that produce values if supported.

Introduce/use a `Never`/bottom type if it materially simplifies branches that always return/throw, but do not add theory for theory's sake.

## Traits and generics

Define exact distinction between:

- nominal declared type;
- structural generic instantiation;
- trait implementation;
- trait bound;
- overload/operator resolution.

Resolution must be deterministic and produce ambiguity errors instead of choosing arbitrary implementations.

Prevent orphan/coherence problems by specifying implementation rules. Choose a simple rule initially, such as an `impl` is legal only when the current module/package owns either the trait or the target nominal type.

If current language semantics differ, document and test the intended alternative explicitly.

## Type aliases and newtypes

Guarantee:

- aliases share identity with the aliased type after resolution;
- newtypes have distinct nominal identity;
- conversion between newtype and representation is explicit according to rules;
- diagnostics print source-friendly alias/newtype names without making printed strings equal compiler identity.

## Type checking phases

Separate:

1. type declaration collection;
2. alias/newtype resolution;
3. generic signature checking;
4. body inference/checking;
5. trait obligation resolution;
6. finalization ensuring no unresolved inference variables remain.

Do not allow unresolved type variables to reach typed IR.

## Error recovery

Introduce explicit `ErrorType`/poisoned expression nodes for diagnostic continuation.

Never use `dynamic` as an error recovery type.

Prevent cascades: if `a` is already an invalid identifier, `a.foo.bar + 1` should not produce five misleading independent type errors.

## Compile-time/runtime boundary

Audit runtime `Value` kind switches. Identify checks that represent:

- legitimate dynamic/host input checks;
- language runtime errors (bounds, division, checked cast);
- checks that should be statically impossible.

For the latter, make the compiler guarantee them and turn runtime violation into an internal assertion/error.

## Tests

Create a dedicated type-system conformance tree with positive and negative cases. Add golden diagnostics for important failures.

Property/fuzz tests should cover:

- type parser/printer round trip, if textual type syntax exists;
- canonical type interning equality;
- substitution;
- generic instantiation;
- conversion lattice properties;
- alias cycles;
- recursive types;
- trait ambiguity/coherence;
- numeric promotion combinations.

## Acceptance criteria

- There is a written complete type-system contract.
- Every language type has canonical compiler representation and defined compatibility rules.
- Trait/generic resolution is deterministic.
- No unresolved inference variable or ordinary `dynamic` fallback reaches typed IR.
- Runtime type mismatches from well-typed ordinary source are considered compiler/runtime bugs.
- Type diagnostics are specific, source-located and tested.
