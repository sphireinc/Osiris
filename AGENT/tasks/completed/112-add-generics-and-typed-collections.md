# Task 112: Add generics and typed collections

**Priority:** P1
**Dependencies:** 110, 111
**Status:** Complete

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

- [x] Generic functions and typed collection forms have one precise checker
      contract.
- [x] Specialization/dispatch is deterministic, bounded, and cache-safe.
- [x] Concrete element/result types survive every promised execution and codec
      boundary.
- [x] Focused positive/negative/cross-backend tests and full validation pass.

## Completion evidence

- `internal/osiris/parser.go` parses bounded generic declarations, explicit
  type arguments, recursive parameterized types, and preserves metadata through
  macro/optimizer expansion and formatting.
- `internal/osiris/typecheck.go` performs shared substitution and inference,
  reports arity/conflict/bound failures, infers nested list/result/option/task
  forms, and enforces invariant parameterized assignability.
- `internal/osiris/collections.go`, `value.go`, `runtime.go`, and `codec.go`
  implement ordered persistent maps and sets with scalar key eligibility,
  duplicate semantics, deterministic rendering/iteration, and validated codec
  tags.
- `internal/osiris/ir.go` and `cbackend_native.go` use the documented hybrid
  strategy: generic bodies use the tagged native ABI while compile-time generic
  assertions are not emitted as impossible runtime checks; generic declaration
  metadata is retained in generated artifacts.
- `internal/osiris/generics_test.go` covers identity, explicit/inferred calls,
  bounds, higher-order calls, recursion, interpreter/VM parity, map/set order
  and key validity, codec round trips, and native artifact generation.
- `AGENT/docs/design/GENERICS.md` is the canonical language/runtime contract.
- Validation: `GOCACHE=/tmp/osiris-go-cache go test ./...` passed.

## Non-goals

- No higher-kinded types, specialization by arbitrary values, C++-style
  overload resolution, or implicit container casts.
