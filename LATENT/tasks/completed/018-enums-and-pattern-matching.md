# Enums and pattern matching

Add closed, first-class enum types and complete the existing expression-valued
`match` / `switch` facility into a predictable pattern-matching language. This
work is deliberately sequenced after the native tagged-value ABI: enum cases
must lower to general generated C values and comparisons, not be interpreted
at compile time or emitted as a snapshot.

## Current state

Osiris already has `match subject { pattern => expression }` and its `switch`
alias. It supports literal, wildcard (`_`), binding, and fixed-length list
patterns. A nonmatching expression reports `no match`. It does not yet support
declared enum cases, grouped PHP-style alternatives, a named `default` arm, or
C#-style type, relational, logical, and guarded patterns.

## Language contract

### Enums

```osiris
enum Suit { Hearts, Diamonds, Clubs, Spades }
enum Result { Ok(value), Err(message) }
enum Status: string { Draft = "draft", Published = "published" }

let card = Suit.Spades
let outcome = Result.Ok(42)
```

- An enum declaration creates one closed nominal type and ordered case names.
- Pure cases are canonical singleton values: `Suit.Spades == Suit.Spades` is
  true and it never equals a case from a different enum.
- Payload cases are immutable tagged values. Their payload arity is checked at
  construction and is visible only through a matching pattern.
- A backed enum permits exactly `string` or an explicitly supported integer
  backing family. Backed values are unique within their enum. `name` and
  `value` reflection, serialization, and casts require a separate documented
  API; no implicit conversion to the backing primitive is allowed.
- Enum declarations are module-scoped, exported explicitly, and may not be
  reopened. Case names are accessed only as `Enum.Case`.

### Match and switch

`match` and `switch` remain expressions with identical semantics. `switch` is
an intentional spelling alias, not a fall-through statement. The subject is
evaluated once, arms are considered left to right, and only the selected body
is evaluated.

```osiris
let label = match suit {
  Suit.Hearts, Suit.Diamonds => "red",
  Suit.Clubs, Suit.Spades => "black",
}

let text = switch result {
  Result.Ok(value) when value >= 0 => "ok",
  Result.Err(message)              => message,
  default                          => "unexpected",
}
```

- Comma-separated patterns in one arm are alternatives, like PHP `match`; an
  arm is selected when any alternative matches.
- `default` is a reserved spelling of the terminal wildcard. Only one is
  allowed, it cannot be grouped with other patterns, and it must be last.
- Without `default`, match is exhaustive only when the compiler can prove that
  all enum cases are covered. Otherwise the runtime returns an explicit
  `unmatched match case` error; it never falls through.
- Pattern matching is strict: scalar literal patterns use Osiris `Equal`
  semantics, never coercive string/number equality.
- C#-inspired patterns are accepted in this order: enum-case/payload patterns;
  type patterns (`number n`, `string text`); relational patterns (`< 0`,
  `>= limit`); logical patterns (`p or q`, `p and q`, `not p`); list patterns;
  and a `when` guard. A guard runs after structural matching and can reference
  variables bound by that pattern.
- Duplicate bindings in a single successful pattern require equality, matching
  the current list-pattern behavior. Bindings do not escape the selected arm.

## Implementation slices

1. Add enum declarations/case references to the lexer, AST, parser, formatter,
   source maps, macro cloning, name resolution, and type inference. Add a
   nominal enum `Value` representation and tests for identity, construction,
   equality, errors, imports, and serialization.
2. Add grouped arms and `default` to existing match syntax, preserving lazy
   left-to-right arm evaluation. Improve unmatched-case diagnostics with the
   subject rendering.
3. Add enum payload, type, relational, logical, and guarded patterns. Add
   static exhaustiveness/unreachable-arm diagnostics for known enum subjects.
4. Lower enum values and proven match dispatch into the C ABI. The fallback
   tagged runtime must handle every defined pattern; direct enum tag switches
   are an optimization, not a semantic fork. Add interpreter/VM/native parity
   tests, including payload bindings and guards.
5. Document syntax, error behavior, public reflection/serialization rules, and
   native lowering status in `newdocs/`.

## Acceptance

- Parser/formatter round trips cover pure, payload, and backed declarations.
- Every supported pattern has interpreter, VM, and C-executable parity tests.
- Nonexhaustive dynamic matches fail deterministically; statically provable
  nonexhaustive enum matches diagnose at compile time.
- Generated C contains neither captured interpreter output nor a Go runtime
  dependency for supported enum/match programs.

## Native tagged enum checkpoint — 2026-07-28

The first native slice replaces the blanket enum rejection with a real `O_ENUM`
tagged C ABI value: nominal type name, case name, and owned payload values.
IR now records enum declarations as compiler metadata and emits `enum` values
for qualified unit cases and payload constructors; it never calls the
interpreter while compiling. Generated C implements enum truthiness, recursive
nominal/payload equality, and interpreter-matched rendering. The scoped
`TestCBackendEnumValueParity` executable test covers unit and payload values,
equality, and output against the interpreter. Unit-case arms plus final
`default` dispatch, and payload-case arms with direct bindings, now lower into
ordinary C CFG branches using `oenumis` and `oenumpayload`; executable parity
is covered by `TestCBackendUnitEnumMatchParity` and
`TestCBackendPayloadEnumMatchBindingParity`. Guarded payload arms now execute
their guard only after direct bindings are installed and resume the next arm
on a false guard; `TestCBackendGuardedEnumMatchParity` locks that behavior.
Grouped enum alternatives with the same direct payload-binding layout now
lower into a stable sequence of tagged C branches and share one selected body;
`TestCBackendGroupedEnumMatchParity` provides executable parity evidence.

### Recursive native pattern checkpoint — 2026-07-28

The generated-C path now lowers nested enum/list payloads, literals,
relational comparisons, `number`/other supported type patterns, conjunction,
negation, grouped alternatives, guards, and repeated-binding equality as CFG
branches. `olistis`/`olistitem` provide checked structural extraction and
`omatchtype` implements the supported tagged-C kind checks; duplicate binding
comparisons lower through strict `oequal`. The source evaluator remains absent
from compilation. Executable interpreter/VM/native parity is covered by
`TestCBackendNestedListEnumMatchParity`,
`TestCBackendRepeatedPatternBindingParity`, and
`TestCBackendRelationalTypeAndLogicalPatternParity`, in addition to the
earlier enum, guard, and grouped tests.

This task remains active: complete native type-kind coverage and a final
acceptance audit of reflection, serialization, diagnostics, parse-format-parse
stability, and public examples are still required before it can close.

### Backed declaration checkpoint — 2026-07-28

`enum Name: string` and `enum Name: <integer-family>` now parse only explicit,
unique matching literals. Backed cases cannot carry payloads, and unbacked
cases cannot introduce a value. The backing data is declaration metadata: the
runtime value remains a nominal enum and does not silently coerce to its
primitive. Formatter output round-trips this syntax. Interpreter, VM, and C
executable parity for string and `i32` declarations is covered by
`TestBackedEnumDeclarationsRoundTripAndNativeParity`; invalid-family, duplicate
value, missing-value, payload, and unbacked-value diagnostics are covered by
`TestBackedEnumDeclarationDiagnostics`.

## Completion record — 2026-07-28

The native audit is complete. `O_ENUM` now retains nominal type/case names and
owned payloads through construction, matching, recursive equality, truthiness,
and recursive rendering. Payload equality now includes the C-ABI closure kind
by callable identity, matching the runtime's function-name identity rule.
Native type patterns cover every value kind represented by the C ABI
(`number`, `integer`/`int`, `string`, `list`, `tensor`, `bool`, `null`,
`enum`, and `function`); runtime kinds with no native representation remain
explicit native compilation boundaries rather than evaluator fallbacks.

The final audit added `TestCBackendEnumNestedPayloadAndFunctionKindParity` for
nested enum rendering, function payload equality, and a function type pattern;
`TestEnumJSONSerializationIsNominalAndRecursive` fixes the public JSON envelope
as `{"enum":{"type":"…","case":"…","payload":[…]}}`; and the static non-exhaustive
enum test now also exercises `CompileC`. Existing focused tests cover pure,
payload, backed, list, literal, relational, type, logical, grouped, repeated
binding, guarded, and default-arm behavior across interpreter, VM, and native
C, as well as formatter round trips and source-located diagnostics.

Validation passed on darwin/arm64 (Apple M1 Pro), Go 1.26.1, Apple clang
15.0.0:

```sh
go test ./internal/osiris -run \
  'Test(CBackendEnum(NestedPayloadAndFunctionKindParity|ValueParity|UnitEnumMatchParity|PayloadEnumMatchBindingParity|GuardedEnumMatchParity|GroupedEnumMatchParity)|EnumJSONSerializationIsNominalAndRecursive|BackedEnumDeclarationsRoundTripAndNativeParity|BackedEnumDeclarationDiagnostics|CBackend(NestedListEnumMatchParity|RepeatedPatternBindingParity|RelationalTypeAndLogicalPatternParity)|EnumMatchRejects)' \
  -count=1
```

Passed (`ok github.com/sphireinc/osiris/internal/osiris`, 2.889s). With Node
24.12.0, `npm run check-reference` passed all 14 API groups, 6 chapters, and
17 redirects. `npm run check-examples` also passed once `/opt/homebrew/bin`
was placed before `/usr/local/go/bin` on `PATH`, ensuring the checker used the
installed Go 1.26.1 toolchain rather than the older Go 1.26.0 binary.
