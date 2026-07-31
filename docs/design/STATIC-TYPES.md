# Static types and typed functions

## Status

This document defines the static type contract introduced by task 110. Osiris
remains compatible with existing unannotated programs, but an annotation is a
checked boundary rather than a comment or an implicit conversion.

## Grammar

Function declarations use the same Go-style, colon-free annotations as
bindings:

```text
type         := canonicalType | functionType
functionType := "func" "(" [ type { "," type } ] ")" type
parameter    := identifier [ type ]
parameters   := [ parameter { "," parameter } ]
functionDecl := "func" identifier "(" parameters ")" [ type ]
                ( "=>" expression | block )
letDecl      := "let" [ "?" ] identifier [ type ] "=" expression
```

Examples:

```osiris
func increment(value u32) u32 => value + u32(1)

func apply(value u32, callback func(u32) string) string {
  return callback(value)
}

let callback func(u32) string = render
```

Parameter commas remain mandatory. `func f(left right)` therefore treats
`right` as a type spelling and reports it as unknown; it does not declare two
untyped parameters.

The canonical type names are:

```text
number u8 u16 u32 u64 i8 i16 i32 i64 f32 f64
string bool null list tensor enum function
error result option stream task module
decimal fixed interval modint complex bigint bigfloat quantity dual
duration datetime location
```

`function` means any callable. A function type such as
`func(u32, string) bool` requires that exact parameter and result signature.
Generic application and user-defined type parameters are reserved for task
112; an unknown annotation such as `T` is currently a source-located error.

## Checking and inference

The checker runs after parsing and again after macro expansion. It does not
evaluate user expressions to discover types. Function signatures in a lexical
statement sequence are declared before bodies are checked, so direct recursion
and mutual recursion do not depend on execution order.

An omitted parameter or return annotation is `dynamic`. Existing unannotated
functions therefore retain their runtime behavior. A value that crosses from a
dynamic expression into a typed binding, argument, or return is checked at
runtime as a compatibility guard. Known mismatches are rejected statically.

An expression-bodied function returns its expression. A block-bodied function
returns an explicit `return`, its tail expression, or `null` on fallthrough.
Every reachable explicit return and every possible tail/fallthrough outcome is
checked against an annotated result.

The expected type context may fit an unsuffixed non-negative integer literal:

```osiris
func choose(flag bool) u32 => if(flag, 1, 2)
```

Both literals above become `u32`. This fitting applies to direct outcomes of
`if`, `match`/`switch`, `try`/`catch`, and block tails. It is the same narrow
literal rule used by `let value u32 = 3`; it is not a cast. A suffixed value or
an existing value of another width is incompatible:

```osiris
func bad() u32 => 3u64       # type error
let source = u64(3)
let target u32 = source      # type error
let converted u32 = u32(source)
```

## Assignability

- Numeric widths and explicit float styles are exact. There are no implicit
  widening or narrowing conversions.
- `string`, `bool`, and `null` are distinct. `null` is assignable only to
  `null`; Osiris does not implicitly make another type nullable.
- `result` and `option` check the container kind. Payload type arguments arrive
  with typed collections and generics in task 112.
- `enum` checks the enum runtime kind. Enum values still retain their nominal
  declared identity for equality and pattern matching; user-defined nominal
  annotations are part of task 111.
- `tensor` checks the tensor runtime kind. The existing shape checker continues
  to reject statically known incompatible shapes; shape parameters are not
  erased or invented by this first annotation grammar.
- `task` checks a task value. The eventual task result remains dynamic until
  generic task types are available.
- Function signatures are invariant and exact. For example,
  `func(u32) string` is not assignable to `func(u64) string` or
  `func(u32) function`.
- `function` accepts any callable, including a callable with an exact
  signature. The reverse direction requires an exact known signature or a
  runtime signature guard at a dynamic boundary.

Explicit constructors perform conversions under their own documented range and
precision rules. An annotation never requests a conversion.

## Typed semantic data

`Runtime.CheckProgram` produces a deterministic `CheckedProgram` containing
typed bindings and function signatures with source spans. Inference, LSP
analysis, VM preparation, IR lowering, and artifact entry points all invoke
this checker rather than maintaining backend-specific type rules.

Function `Value` objects retain the canonical signature alongside their
closure environment and callable identity. IR functions retain parameter and
result types, and lowering emits entry and result assertions wherever a
dynamic compatibility boundary can still reach the function.

## Diagnostics and backend behavior

Type errors use the structured `type_error` kind and point to the incompatible
argument, return, binding, or declaration. Diagnostic order follows source
order.

The interpreter checks the source and macro-expanded program before executing
the first main-program statement. The VM and artifact paths check their
expanded program before bytecode or IR emission. WASM snapshot generation and
GPU planning also reject known type errors before producing an artifact.

Native C preserves exact callable signatures in closure metadata and checks
typed entries and returns in generated C. If a runtime kind has no portable C
representation, native compilation rejects its assertion at the annotated
source span. It does not defer that unsupported boundary to execution.
