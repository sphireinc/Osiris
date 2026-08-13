# Task 18 — Freeze Feature Expansion and Define a Supported Osiris Core 1 Surface

## Objective

Reduce implementation risk by defining a stable Core 1 language surface and requiring full semantic/backend quality for that surface before accepting additional language features.

This does **not** require deleting valuable advanced work. It requires separating core language commitments from experimental/domain-specific extensions and stopping uncontrolled expansion.

## Core 1 candidate surface

Unless the semantic review identifies a serious reason to adjust it, Core 1 should include:

- primitive scalar types;
- Osiris exact numeric foundation judged stable enough;
- strings;
- lists/maps/sets;
- structs;
- enums/sum types;
- functions and closures;
- `const`, eager `let`, `var`;
- explicit `Lazy<T>`;
- `if`, `match`, loops, return/break/continue;
- `Option<T>` and `Result<T,E>`;
- structured errors/propagation;
- modules/imports;
- generics and traits only if Tasks 03/07 prove them coherent/stable;
- `defer`;
- tasks/concurrency subset whose deterministic semantics are fully specified.

## Extension/domain surfaces

Classify these separately unless already proven Core-quality:

- tensors/linear algebra;
- GPU;
- WASM target-specific facilities;
- FFI;
- unsafe memory;
- databases;
- telemetry;
- HTTP/TLS/networking policy libraries;
- process/OS integration;
- codecs;
- specialized scientific numeric domains.

These may ship and remain useful. Mark status clearly: stable extension, experimental, backend-limited, etc.

## Backend support tiers

Create one authoritative support matrix with levels such as:

```text
Core 1 conforming
Stable extension
Experimental
Unsupported
```

A backend claiming Core 1 conforming MUST run the entire Core 1 conformance suite with identical observable semantics.

Do not label a backend Core-conforming if it silently falls back to interpreter execution.

## Feature freeze policy

Until the following are complete, reject/defer proposals for new syntax/type-system/concurrency features unless required to complete the hardening work:

- semantic contract;
- eager let/explicit lazy;
- strong static typing;
- canonical types;
- staged front end;
- typed HIR/IR;
- VM locals;
- real maps;
- runtime extraction;
- package/layer split;
- Core 1 backend conformance.

Bug fixes, performance work, documentation, tests and implementation completion are allowed.

## Remove accidental complexity

Inventory all current syntax/features. For each ask:

1. Is it orthogonal?
2. Can it be expressed as library code?
3. Does it duplicate another mechanism?
4. Does every Core backend implement it?
5. Is its semantic contract precise?
6. Does it justify permanent language complexity?

Do not delete features just to make a smaller list. Deprecate only when redundancy/semantic weakness is concrete.

## Core conformance command

Add a single command/test target, e.g.:

```text
make test-core
```

or Go equivalent, that runs Core 1 fixtures through all required conforming backends and reports a matrix.

## Documentation

User docs must clearly distinguish:

- language core;
- standard library;
- runtime/platform capabilities;
- experimental/backend-specific extensions.

Avoid presenting every repository capability as equally fundamental to the language.

## Acceptance criteria

- Core 1 is explicitly enumerated and versioned.
- Feature freeze is documented in contributor guidance/roadmap.
- Every conforming backend runs the same Core conformance suite.
- Advanced features have explicit stability/support classifications.
- No significant new language feature lands before the hardening gates are met unless it is necessary to satisfy an existing Core semantic requirement.
