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
