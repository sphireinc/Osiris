# Task 112: Add generics and typed collections

**Priority:** P1
**Dependencies:** 110, 111
**Status:** Blocked

## Objective

Add generic functions and parameterized built-in/container types so reusable
algorithms can retain compile-time element and result types.

## Required language contract

- Define type-parameter syntax, explicit and inferred type arguments, bounds,
  substitution, inference failure, and ambiguity diagnostics.
- Add parameterized forms for `list<T>`, `map<K, V>`, `set<T>`, `option<T>`,
  `result<T, E>`, `task<T>`, streams, tensors where applicable, and function
  types.
- Define map/set key eligibility, equality and hashing, duplicate handling,
  deterministic iteration/rendering/serialization order, and collection
  update semantics.
- Specify invariance/covariance decisions explicitly; do not infer unsafe
  container conversions.
- Define generic recursion, closures, enum/struct use, default values, and
  separate module identity.
- Select and document native implementation strategy: monomorphization,
  dictionary passing, or a proven hybrid. Artifact/cache keys must include all
  semantic type arguments.

## Implementation requirements

- Extend the shared checker rather than adding backend-local template logic.
- Preserve deterministic specialization order, diagnostics, IR names, C
  symbols, cache keys, and generated artifacts.
- Prevent infinite specialization and enforce bounded compiler resource limits.
- Retain concrete type arguments through runtime reflection/inference surfaces
  where the public contract exposes them.

## Tests and documentation

- Generic identity/map/fold, higher-order, nested-container, map/set ordering
  and key validity, recursive, and cross-module fixtures.
- Negative tests for unsatisfied bounds, ambiguous inference, arity,
  specialization cycles, and type erasure.
- Interpreter/VM/native parity and generated-IR/C specialization assertions.
- Compiler scale/limit tests and cache reproducibility checks.
- Canonical reference documentation and checked examples.

## Acceptance criteria

- [ ] Generic functions and typed collection forms have one precise checker
      contract.
- [ ] Specialization/dispatch is deterministic, bounded, and cache-safe.
- [ ] Concrete element/result types survive every promised execution and codec
      boundary.
- [ ] Focused positive/negative/cross-backend tests and full validation pass.

## Non-goals

- No higher-kinded types, specialization by arbitrary values, C++-style
  overload resolution, or implicit container casts.
