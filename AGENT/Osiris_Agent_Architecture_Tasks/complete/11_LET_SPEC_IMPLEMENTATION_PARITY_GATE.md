# Task 11 — Close the `let` Specification/Implementation Parity Bug

## Objective

Treat the existing disagreement over `let` evaluation as a semantic compatibility defect and close it across **specification, interpreter, VM, native backend, examples and conformance tests**.

Task 01 performs the implementation redesign and adds explicit lazy values. This task is the independent parity gate that proves no stale lazy-`let` behavior remains anywhere.

## Known contradiction

Documentation such as `docs/design/STATE-AND-MUTATION.md` and `docs/src/language-reference.json` describes ordinary `let` as evaluated once when the declaration executes.

The current interpreter instead stores an initializer expression/environment and evaluates on demand. `internal/osiris/lazy_loop_test.go` contains tests intentionally depending on unused ordinary `let` initializers not executing.

These cannot both define the language.

The required final contract is:

```text
ordinary let = eager immutable binding
explicit lazy { ... } = deferred Lazy<T>
```

## Repository-wide audit

Search for every reference to lazy/eager binding semantics, including:

- docs;
- parser comments;
- AST comments;
- `binding` types;
- evaluator promise/cache code;
- VM bind opcodes;
- native lowering;
- examples;
- tests named lazy/deferred/memoized;
- benchmark assumptions;
- LSP hover/documentation strings;
- language reference generators.

For every occurrence, classify it as ordinary eager `let` or explicit `Lazy<T>` behavior.

No ambiguous "bindings may evaluate when read" wording may remain.

## Build a semantic probe suite

Create backend-neutral Osiris programs that distinguish evaluation strategies:

### Error timing

```osiris
emit "before"
let x = 1 / 0
emit "after"
```

Expected: `before` is emitted, declaration fails, `after` is not emitted.

### Effect timing

Use a controlled deterministic side-effect primitive/test hook to prove initializer effect occurs at declaration.

### Scope capture

```osiris
var x = 1
let y = x
x = 2
emit y
```

Expected `y` is the value produced when declaration executed, not a later read through captured environment.

### Loop declarations

Ensure initializer executes once per control-flow execution of the declaration, not once per AST node globally.

### Explicit lazy

Equivalent cases with `lazy { ... }` must demonstrate deferred exactly-once behavior.

## Backend matrix

Run each semantic probe through every backend that claims support:

- interpreter;
- VM;
- native C/AOT;
- WASM if it claims the involved Core 1 features;
- any other execution mode exposed as semantically equivalent.

If a backend does not support explicit laziness yet, it must reject the construct at compile time with a documented unsupported-backend diagnostic. It may not silently change semantics.

## Cache audit

Search evaluator caches for keys based on expression + scope. Ensure ordinary `let` no longer relies on those caches. If evaluator memoization remains for pure-expression optimization, it must not alter observable evaluation order or suppress required errors/effects.

Memoization as an optimization must be semantics-preserving, not the language's implicit evaluation strategy.

## Documentation wording

State explicitly:

> When execution reaches a `let` declaration, Osiris evaluates the initializer exactly once before creating the immutable binding. If evaluation fails, the binding is not created and control follows normal error propagation.

And for lazy:

> `lazy { expr }` constructs a `Lazy<T>` without evaluating `expr`; `force` evaluates it at most once.

## Acceptance criteria

- No ordinary-`let` lazy test remains.
- Semantic probes pass identically on all claiming backends.
- Docs, formatter, examples, LSP text and runtime agree.
- A repository search finds no implementation path storing an ordinary `let` initializer AST for later demand evaluation.
- Add a regression test specifically named/documented as preventing recurrence of the former spec/interpreter contradiction.

## Completion record — 2026-08-13

Completed the independent eager-`let` parity gate.

- Audited source, runtime, VM, native lowering, tests, public/design docs,
  LSP-facing reference data, examples/workloads, and task guidance. Ordinary
  `let` is consistently documented as eager; `lazy { ... }` is the only
  deferred, type-visible form.
- Added Core probes for eager effect timing, capture timing, loop declaration
  execution, same-scope redeclaration, native error timing, and explicit
  Lazy<T> once-only forcing. The native Core runner now executes error fixtures
  and asserts both a nonzero result and committed output before the failure.
- Added `TestFormerLazyLetContradictionCannotRecur`, which proves interpreter,
  VM, and generated native C emit `"before"`, fail at the declaration, and do
  not reach `"after"`.
- Enforced same-scope redeclaration diagnostics in the frontend while retaining
  child-scope shadowing and initializer-before-binding visibility.
- Replaced the stale `lazy_eval.go` naming with `evaluator_iterative.go`,
  removed its artificial AST-backed binding-cycle test, and made its stored
  value/cell behavior explicit. Heap inspection and module exports also read
  stored bindings first, preventing tooling from re-evaluating a `let`.
- Updated the Core deviation ledger and support matrix: CS1-D02 and CS1-D03
  are resolved; evaluation-order determinism remains CS1-D01 for Task 18.

Validation completed:

```sh
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run 'Test(CoreSemanticsConformance|FormerLazyLetContradictionCannotRecur|EagerLoopBindingBoundedStackSubprocess|EagerLoopBindingParityAndScopeRetention|OrdinaryLetInitializersAreEager|ExplicitLazy)' -count=1
GOCACHE=/tmp/osiris-go-cache go test ./...
GOCACHE=/tmp/osiris-go-cache go test -race ./internal/osiris -run 'Test(FormerLazyLetContradictionCannotRecur|CoreSemanticsConformance|ExplicitLazy|EagerLoopBindingParityAndScopeRetention)$' -count=1
GOCACHE=/tmp/osiris-go-cache go vet ./...
./scripts/check-docs.sh
git diff --check
```
