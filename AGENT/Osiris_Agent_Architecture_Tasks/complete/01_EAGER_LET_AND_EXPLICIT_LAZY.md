# Task 01 — Make `let` Eager and Add Explicit Lazy Values

## Objective

Change Osiris so that ordinary `let` declarations use conventional eager evaluation. Add an explicit lazy-expression facility with source syntax:

```osiris
let value = lazy { expensive() }
```

The ordinary declaration:

```osiris
let value = expensive()
```

MUST execute `expensive()` exactly once, immediately when control reaches the declaration. The binding stores the resulting value, not the initializer AST and lexical environment.

The explicit lazy form MUST defer evaluation until demanded, evaluate at most once, cache either the successful value or the failure according to the final semantic contract, and be represented by a distinct type rather than pretending to be the underlying value.

## Required semantic decisions

Implement the following semantics unless the semantic-contract task establishes a stricter compatible rule:

1. `let x = expr` is eager, immutable binding.
2. `var x = expr` remains eager, mutable binding.
3. `const x = expr` remains compile-time/constant according to existing constant rules.
4. `lazy { expr }` produces a value of type `Lazy<T>` where `T` is the static type of `expr`.
5. A lazy value SHALL NOT be implicitly forced by arbitrary variable access. Requiring explicit force is preferred because invisible effects are precisely what this task is eliminating.
6. Add one canonical forcing operation. Prefer language syntax `force(value)` initially unless there is already a clean callable/trait convention. `force : Lazy<T> -> T`.
7. A `Lazy<T>` evaluates its body at most once.
8. Recursive forcing of the same lazy value while it is already being evaluated MUST produce a deterministic structured error such as `LazyCycle` rather than deadlock or recurse forever.
9. Concurrency behavior MUST be deterministic and race-free. Two tasks forcing the same lazy object concurrently must coordinate so that the body executes once. Other forces receive the same completed result.
10. Decide and document whether a failed force is memoized. Preferred rule: **memoize the failure** so that a lazy initializer has exactly-once evaluation semantics rather than retry semantics. Retrying belongs in an explicit retry API.
11. Cancellation MUST not leave the lazy object permanently stuck in a pending state. Define whether cancellation is the memoized result or resets the lazy state; preferred rule is to memoize a language-level failure only after evaluation has actually committed, while context cancellation cleans up pending state safely. Cover this with tests.

## Parser and AST work

Inspect `internal/osiris/lexer.go`, `parser.go`, AST node declarations, formatter, parser round-trip tests, and documentation tooling.

Add `lazy` as a reserved keyword only if doing so does not break an existing identifier compatibility rule. Add a dedicated AST expression node, not a magic builtin call synthesized by the parser. Example shape:

```go
type lazyExpr struct {
    Body Expr
    Span SourceSpan
}
```

The actual AST naming may follow repository conventions.

The parser MUST accept:

```osiris
lazy { expression }
```

Do not make this a statement-only construct. It is an expression and may appear wherever an expression of type `Lazy<T>` is valid.

Reject malformed forms with source-local diagnostics. Update formatter and format round-trip tests.

## Type-system work

Add canonical `Lazy<T>` type support. Do not model it by string concatenation such as `"lazy<" + typeName + ">"` as a permanent implementation.

Rules:

- `lazy { expr }` has type `Lazy<T>`.
- `force(lazyT)` has type `T`.
- `Lazy<T>` is not assignment-compatible with `T`.
- `T` is not assignment-compatible with `Lazy<T>` without an explicit lazy constructor.
- Generic substitution/inference must preserve `Lazy<T>` structurally.
- Equality/hashability/serialization rules for `Lazy<T>` must be explicit. Preferred initial rule: lazy values are not value-equality comparable, not map keys, and not serializable unless forced.
- FFI/native ABI exposure of `Lazy<T>` should initially be rejected unless a stable representation is explicitly implemented.

## Interpreter migration

Inspect `internal/osiris/session.go`, `runtime.go`, `evaluator_iterative.go`, binding/scope structures, evaluator cache/promise machinery, and `eager_loop_test.go`.

Current ordinary `let` bindings store initializer AST plus scope and defer evaluation. Remove that behavior from ordinary lexical bindings.

After this task, a normal immutable runtime binding should conceptually contain:

```go
type binding struct {
    value Value
    // metadata as necessary
}
```

not an unevaluated `Expr` and captured scope.

Evaluate the initializer before extending the scope with the new binding. This preserves normal lexical behavior and prevents the initializer from referring to the just-created binding unless recursion is explicitly supported through another mechanism.

Move promise/memoization machinery that is genuinely useful for laziness into a dedicated lazy runtime object rather than keeping it as the default variable-binding model.

Do not preserve the old hidden laziness through an evaluator cache.

## VM work

Ensure bytecode for:

```osiris
let x = f()
```

performs the call/evaluation before the bind/store instruction. This should be explicit in bytecode tests.

Add bytecode operations for lazy creation/forcing only if necessary. Prefer a runtime primitive with a well-defined lazy object rather than smuggling AST nodes into VM values. The VM must never require raw AST execution to force a lazy expression. Compile the lazy body into a zero-argument closure/thunk with captured lexical values, then store that closure inside the runtime `Lazy<T>` object.

This is critical: **compiled laziness must remain compiled**. No VM-to-interpreter fallback is allowed.

## Native backend work

Implement explicit laziness in the native backend using the canonical runtime ABI. Compile a lazy body into a closure/thunk and allocate a runtime lazy object with a state machine such as:

```text
UNFORCED -> EVALUATING -> VALUE
                      \-> ERROR
```

Use synchronization only if lazy objects can be shared across concurrently executing tasks. The implementation must not use process-global mutable state.

## Tests that MUST be added/changed

Replace tests that intentionally depend on unused ordinary `let` initializers not executing.

Required tests:

```osiris
let x = 1 / 0
emit 7
```

MUST fail before `emit 7`.

```osiris
let x = side_effect()
emit "after"
```

MUST show the side effect before `after`.

```osiris
let x = lazy { 1 / 0 }
emit 7
```

MUST emit 7 if `x` is never forced.

```osiris
let x = lazy { expensive() }
let a = force(x)
let b = force(x)
```

MUST execute `expensive()` once and produce equal `a`/`b` values.

Also add:

- closure capture tests;
- lazy body capturing immutable and mutable lexical values according to snapshot/reference rules;
- failure memoization tests;
- recursive force/cycle tests;
- concurrent force tests;
- cancellation tests;
- interpreter/VM/native parity tests;
- parser/formatter tests;
- source diagnostic tests.

## Documentation changes

Update at minimum:

- `docs/design/STATE-AND-MUTATION.md`
- `docs/src/language-reference.json`
- generated/public docs derived from those sources
- language examples and any lazy-evaluation documentation

Delete claims that ordinary `let` is lazy. Explicitly state evaluation order.

## Acceptance criteria

This task is complete only when:

- ordinary `let` is eager in every supported execution mode;
- unused `let` initializers can no longer suppress errors/effects;
- explicit `Lazy<T>` exists and is statically visible;
- forcing is explicit and exactly-once;
- no backend forces a lazy value by invoking the AST interpreter;
- old evaluator binding laziness is removed rather than merely bypassed;
- all changed semantics are covered by backend-parity tests and docs.

## Completion record — 2026-08-13

Completed the eager-binding and explicit-laziness implementation.

- Ordinary `let` initializers now evaluate before their name enters scope in
  Session execution and evaluator-only block execution. Immutable bindings
  retain eager `Value`s; they do not retain initializer AST/environment pairs.
- `lazy { expression }` is a reserved-keyword AST expression with
  parse-format-parse support. The type checker represents `Lazy<T>`
  structurally and types `force(Lazy<T>)` as `T` without an implicit coercion.
- Interpreter lazy objects coordinate concurrent forces, cache successful
  values and language failures, detect `LazyCycle`, and reset an interrupted
  attempt after cancellation.
- The VM compiles lazy bodies to bytecode chunks and forces them through VM
  execution only. Native C lowering compiles them to captured zero-argument
  closures and uses an `OLazy` per-object state machine. Neither backend
  delegates force to the AST interpreter.
- Added eager-let, parser/formatter, static-type, interpreter, VM, native-C,
  Core-fixture, cycle, failure, concurrent-force, capture, and cancellation
  coverage. The Core laziness fixture now requires interpreter, VM, and native
  execution.
- Updated the state design, language reference, support matrix, and Core
  deviation ledger. Native task execution is currently cooperative/sequential;
  a future physically concurrent native task runtime must add per-`OLazy`
  synchronization rather than process-global state.

Validation completed:

```sh
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run 'Test(ExplicitLazy|OrdinaryLetInitializersAreEager|NativeCLowersLazyToCompiledOnceThunk|VMExplicitLazyCompilesAndForcesExactlyOnce|VMLazySharesOneConcurrentForceAndCachesFailure|LazyStaticType|CoreSemanticsConformance|ParserExplicitLazyExpressionAndSpans|FormatterLanguageCorpus)$' -count=1
GOCACHE=/tmp/osiris-go-cache go test ./...
GOCACHE=/tmp/osiris-go-cache go test -race ./internal/osiris -run 'Test(ExplicitLazy|VMExplicitLazyCompilesAndForcesExactlyOnce|VMLazySharesOneConcurrentForceAndCachesFailure)$' -count=1
GOCACHE=/tmp/osiris-go-cache go vet ./...
./scripts/check-docs.sh
git diff --check
```

`./scripts/test-native-sanitizers.sh` was invoked and correctly skipped on
Darwin because its generated-executable ASan runtime is unsupported there.
