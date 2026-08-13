# Task 17 — Runtime Data Structures and VM Locals Performance Closure Gate

## Objective

After Tasks 04 and 10, perform a deliberate performance/integration pass proving the new map/set implementation and VM slot-local model are actually used across production paths and that old slow paths are removed.

This task is not permission for speculative optimization. It is a closure gate targeting known architectural bottlenecks.

## Required audits

### VM lexical access

Search for every VM/interpreter bytecode path that performs:

- `map[string]Value` lookup for lexical locals;
- `map[string]bool` mutability lookup;
- parent-frame name-chain search;
- runtime resolution of an identifier that should have been resolved by compiler.

For VM code, ordinary lexical operations must use integer slots/capture indices.

### Map/set operations

Search for every collection lookup that linearly scans entries. Replace production map/set operations with the deterministic hash table from Task 10.

Small-list utilities may legitimately linear scan; a language `Map` may not.

### Allocation profile

Profile:

- function calls;
- local variable reads/writes;
- tight numeric loops;
- map lookup/insertion;
- closure creation/calls;
- task snapshot;
- common string/list operations.

Use Go benchmarks for interpreter/VM and native benchmarks for AOT.

## Native performance objective

For statically typed primitive loops compiled natively, inspect generated code/C and make sure the runtime architecture does not force:

- heap allocation per scalar local;
- hash lookup for local variables;
- boxed generic `Value` dispatch for every integer/float operation;
- repeated runtime type-name parsing/checking.

The goal is C-like generated code for C-like source operations, subject to Osiris safety semantics.

Example expectation:

```osiris
var sum: i64 = 0
for i in range(0, n) {
    sum = sum + i
}
```

should lower to primitive integer locals/loop control, not a chain of generic dynamic runtime calls.

## Benchmark corpus

Create stable benchmark programs covering:

1. scalar loop;
2. function call/local access;
3. nested lexical scopes;
4. closure reads/writes;
5. map build + random lookup;
6. set membership;
7. list traversal;
8. string processing;
9. task fan-out/fan-in;
10. numeric/tensor representative kernel.

Record interpreter, VM and native results separately. Compare previous baseline where available.

## Regression guards

Add benchmarks to CI only as informational unless stable performance infrastructure exists; avoid flaky hard ns thresholds.

Add structural tests that are deterministic:

- bytecode local operands are numeric slot IDs;
- VM frame local storage is slice/array based;
- map implementation uses hash table indexing;
- native scalar IR/C contains typed primitive operations for representative programs;
- no interpreter fallback in VM/native path.

## Cleanup

Delete obsolete compatibility code:

- legacy VM local map helpers;
- old linear map lookup used by language `Map`;
- duplicate map-key semantic functions if canonical hash/equality API replaces them;
- stale benchmarks testing removed architecture.

## Acceptance criteria

- Known hot paths use the new structures, not merely coexist with them.
- Native scalar loops have a plausible direct path to C-class performance.
- VM lexical access complexity is O(1) direct index.
- Map/set expected lookup complexity is O(1).
- Benchmarks and structural guard tests document/prevent regression.
