# Task 14 — Define the Osiris Core Semantic Contract Before Adding Features

## Objective

Freeze significant new language-feature work and write an executable semantic contract for Osiris Core. The contract must define at least the following 20 fundamental rules precisely enough that two independent backend implementers would produce the same observable behavior.

This task should begin before the other architecture migrations because it defines what they must preserve.

## Required semantic rules

### Rule 1 — Source evaluation order

Define left-to-right/right-to-left order for function arguments, operator operands, collection literals, field/index expressions and chained calls. Preferred rule: deterministic left-to-right evaluation unless a construct explicitly states otherwise.

### Rule 2 — `const`, `let`, `var`

Define initialization timing, immutability/mutability, shadowing and assignment. Ordinary `let` MUST be eager. Failed initializer means binding is not created.

### Rule 3 — Explicit laziness

Define `lazy { expr }`, `Lazy<T>`, explicit forcing, exactly-once behavior, failure/cancellation/cycle/concurrency semantics.

### Rule 4 — Lexical scope and shadowing

Define block/function/module scope, declaration visibility, same-scope redeclaration, shadowing and initializer visibility.

### Rule 5 — Function calls

Define argument evaluation, arity, parameter binding, defaults/variadics if present, return evaluation and recursion visibility.

### Rule 6 — Closures

Define what closures capture: immutable values, mutable cells, function/module references. Define lifetime and mutation visibility.

### Rule 7 — Static typing

Define that normal programs are statically typed, when annotations are required, inference boundaries and explicit dynamic/foreign escape hatches.

### Rule 8 — Numeric literal typing and conversions

Define default literal types, widening, narrowing, overflow, signed/unsigned interaction, exact numeric behavior and explicit conversion rules.

### Rule 9 — Equality, identity and ordering

Define equality by type family, cross-numeric equality if any, floating `NaN`/signed zero, aggregate equality, function/task/resource equality and ordering availability.

### Rule 10 — Hashability and deterministic collections

Define valid map/set keys, hash/equality contract, duplicate keys and map/set iteration order.

### Rule 11 — Aggregate/value semantics

Define whether lists/maps/sets/structs are immutable values, copy/share behavior, mutation APIs and aliasing guarantees.

### Rule 12 — Control-flow conditions and loops

Define bool/truthiness policy, loop variable scope, break/continue behavior, loop result semantics if any and mutation visibility.

### Rule 13 — Pattern matching

Define pattern binding scopes, arm order, guards, exhaustiveness, unreachable patterns and enum payload semantics.

### Rule 14 — Errors / `Result` / propagation

Define structured error identity, `Result`, `Option`, `?`, `try/catch`, cause chains, matching, redaction and whether panics/internal faults are language errors.

### Rule 15 — `defer` and cleanup

Define registration timing, LIFO order and behavior on return, error, break/continue, cancellation and cleanup failure. Define multiple-cleanup-error policy.

### Rule 16 — Tasks and closure capture

Define task creation, what lexical state is snapshotted/shared, result ordering, cancellation, task failure and task object lifetime.

### Rule 17 — Parallel/race/select determinism

Define which outcome ordering is guaranteed, what may physically execute concurrently, restrictions on effects and tie-breaking rules.

### Rule 18 — Modules/import initialization

Define module identity, import cycles, initialization order, one-time initialization, exported symbol visibility and failure behavior.

### Rule 19 — Resource/FFI/unsafe ownership

Define language resource handle lifetime, ownership transfer, explicit unsafe boundaries, FFI argument/result ownership and cleanup guarantees.

### Rule 20 — Backend equivalence

Define which behavior is language-observable and therefore MUST be identical across interpreter/VM/native/WASM. Unsupported features must fail at compile time; no backend may silently switch semantics or delegate to another execution engine.

## Add additional rules where necessary

At minimum also consider:

21. string/text encoding and indexing;
22. array/list bounds behavior;
23. integer division/modulo for negatives;
24. floating reproducibility expectations;
25. tensor/quantity semantics;
26. macro expansion/hygiene phase;
27. generic/trait coherence;
28. cancellation boundaries for blocking host calls;
29. process exit/uncaught error semantics;
30. determinism guarantees around clocks/random/I/O.

## Deliverables

Create:

```text
docs/design/SEMANTICS-CORE.md
spec/core/
```

For each rule include:

- normative wording using MUST/SHALL/MAY consistently;
- 2–5 minimal source examples;
- expected result/output/error;
- edge cases;
- backend obligations.

Create machine-executable conformance fixtures where possible:

```text
spec/core/bindings/*.osr
spec/core/evaluation_order/*.osr
...
```

with expected output/error metadata.

## Spec/versioning

Assign a semantic version identifier such as `Core Semantics 1` or align with Osiris language versioning. Compiler/runtime should be able to report language semantic version.

Changes to established Core semantics after release require explicit versioning/migration, not accidental test edits.

## No implementation-defined loopholes

Use "implementation-defined" only for matters genuinely irrelevant to portable program behavior. Do not use it to avoid deciding:

- argument order;
- map iteration;
- overflow;
- task result tie-breaking;
- cleanup order.

## Acceptance criteria

- At least the 20 required rules are normative and example-backed.
- Every rule that can be tested has backend-neutral conformance fixtures.
- Existing contradictory docs/tests are reconciled.
- Feature PRs must cite semantic rules they add/change.
- No significant new language feature is accepted until Core semantic suite is green on the required backend matrix.

## Completed — 2026-08-12

Delivered:

- Added `docs/design/SEMANTICS-CORE.md` as the normative Core Semantics 1
  contract, including Rules 1–30, examples, backend obligations, and a
  migration-gate table.
- Added the public `spec/core/` fixture corpus and a strict backend-neutral
  fixture metadata format exercised by `TestCoreSemanticsConformance`.
- Added `internal/osiris.CoreSemanticsVersion`; `osiris version` now reports
  the language semantic version.
- Reconciled public reference data so legacy implementation status cannot be
  read as an alternate language contract.
- Added two minimal expected-observation examples, edge cases, and backend
  obligations for every Core Semantics rule.
- Ran documentation checks, focused legacy/Core conformance, CLI version
  checks, and the full Go test suite successfully.

Known implementation deviations (intentionally not implemented in Task 14):

1. The interpreter currently schedules ordinary binary expressions and some
   builtin/list evaluation concurrently, violating Rule 1’s source order and
   Rule 17’s deterministic tie-breaking target.
2. Ordinary unannotated `let` is currently implicit lazy/memoized evaluation,
   while Rules 2–3 require eager `let` plus explicit `lazy {}` / `force`.
3. Current scope chaining permits same-scope redeclaration, contrary to Rules
   2 and 4.
4. The checker still permits implicit dynamic compatibility, contrary to Rule
   7; native enum/pattern and full backend-matrix coverage also remain pending.

These are intentional later-task implementation requirements, not exceptions
to Core Semantics 1. Their evidence and exact ownership are maintained in the
`Implementation deviation ledger` of `docs/design/SEMANTICS-CORE.md`. This
task is complete as the specification/contract milestone; Task 18 owns the
final all-backend Core 1 conformance gate.
