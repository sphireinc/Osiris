# Task 10 — Replace Linear "Maps" and Sets with Real Deterministic Hash Collections

## Objective

Replace the current O(n) linear-scan map/set implementation in `internal/osiris/collections.go` with hash-based collections providing expected O(1) lookup/insertion while preserving deterministic language-visible behavior.

## Current problem

Current lookup conceptually performs:

```go
for i := range entries {
    if entries[i].key.Equal(key) {
        return i
    }
}
```

This means map lookup is O(n), repeated insertion can be O(n²), and the type named "map" does not have the performance characteristics programmers expect from a general-purpose language.

## Define observable semantics first

Specify and document:

1. Which types are valid map keys/set elements.
2. Equality semantics for keys.
3. Hash semantics consistent with equality: if `a == b`, hashes MUST match.
4. Iteration order.
5. Mutation semantics if maps/sets remain value-semantic/immutable.
6. Duplicate-key literal behavior.
7. Floating-point key rules, especially `NaN`, `-0.0`, infinities.
8. Exact numeric cross-type equality/hash rules if `1`, `1.0`, rational `1/1`, etc. can compare equal.
9. String normalization behavior: byte/codepoint identity, no locale-dependent hashing.
10. Composite key policy.

## Determinism requirement

Do not inherit Go's randomized map iteration or libc/platform-dependent hash behavior as an Osiris semantic.

Preferred design:

- deterministic stable hash algorithm with fixed specified seed/version for language values;
- open addressing or another cache-friendly table;
- maintain an insertion-order vector/index if language iteration is insertion-ordered;
- alternatively define deterministic table-order iteration only if the exact contract is deliberately stable.

Insertion-order iteration is generally easiest for users and preserves predictable output independently of table capacity.

## Data structure

Implement a runtime collection roughly as:

```text
entries: dense array of key/value records in stable iteration order
index: hash table mapping key hash/probe -> entry index
```

Deletion/tombstone strategy must be defined if mutable deletion exists.

If language maps are persistent/value-semantic, choose copy-on-write/persistent strategy carefully; do not clone the entire table on every operation if that is current behavior in hot workloads. Measure before choosing structural sharing complexity.

## Hash function

Create a central language-value hash API. Do not let map, set, native runtime and VM each invent a hash.

Hashing must:

- be deterministic across supported backends for the same semantic key;
- respect equality;
- reject unhashable keys statically when possible;
- avoid unbounded recursion on cyclic/dynamic objects;
- be reasonably collision resistant for ordinary workloads.

If DoS-resistant keyed hashing is needed for untrusted input, reconcile it with deterministic semantics through a specified runtime mode or stable iteration side structure. Do not silently randomize observable iteration.

## Native runtime

Implement the canonical native map/set table in the C runtime from Task 05. Native generated code should call runtime collection primitives for heap collections.

The Go interpreter/VM must match semantic behavior. Prefer sharing golden hash/equality vectors across Go and C tests.

## Static typing

The type checker must reject invalid map key types before execution in ordinary static source.

Type properties should expose `Hashable`/equality capability structurally or via a well-defined trait, not through a string whitelist.

## Tests

Correctness:

- collisions;
- resize/grow;
- duplicate keys;
- empty collections;
- thousands/millions of keys where feasible;
- deterministic iteration;
- equality/hash edge cases;
- map/set parity across interpreter/VM/native;
- fuzz operations against a reference model;
- malformed/dynamic keys at runtime boundaries.

Performance:

Benchmark lookup/insertion for sizes 8, 64, 1K, 100K. Demonstrate expected scaling rather than merely one fast microbenchmark.

A 100K-entry lookup must not linearly traverse entries.

## Acceptance criteria

- expected O(1) lookup/insertion;
- deterministic documented iteration;
- equality/hash contract is tested cross-backend;
- no ordinary map lookup scans every entry;
- maps/sets are backed by canonical runtime-quality structures rather than lists with collection names.
