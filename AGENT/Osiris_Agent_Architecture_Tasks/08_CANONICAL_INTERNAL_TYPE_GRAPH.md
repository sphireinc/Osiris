# Task 08 — Introduce a Canonical Internal Type Graph with Stable Type IDs

## Objective

Replace ad-hoc compiler type structures based primarily on textual names with an interned canonical type graph.

Every semantic type known to the compiler SHALL have stable structural/nominal identity represented by a `TypeID` (or equivalent compact handle). Equality of semantic types SHALL be identity/structural canonical equality, not formatted-string equality.

## Target architecture

Create a type arena/interner owned by a compilation context:

```go
type TypeID uint32

type Kind uint8

const (
    KindInvalid Kind = iota
    KindBool
    KindInt
    KindFloat
    KindString
    KindList
    KindMap
    KindFunction
    KindStruct
    KindEnum
    KindTypeParam
    KindGenericInstance
    KindOption
    KindResult
    KindLazy
    KindTask
    KindTensor
    KindQuantity
    KindDynamic
    KindNever
    KindError
    // ... actual Osiris kinds
)

type Type struct {
    Kind Kind
    // canonical payload by kind
}
```

Do not copy this blindly if Go representation can be cleaner. The requirements are stable IDs, explicit kinds, canonical payloads and no semantic reliance on display strings.

## Interner behavior

Structural types with identical canonical components should intern to the same `TypeID` within a compilation universe.

Examples:

```text
List<u32> == List<u32>
(u32, String) -> bool == same signature
```

Nominal types such as separately declared structs/newtypes remain distinct even if fields/underlying representations are identical. Give declarations stable `SymbolID`/declaration IDs that participate in the type node.

## Compilation-context ownership

Avoid process-global mutable interners. Use an explicit `TypeContext`/`Compilation` object so:

- parallel compilations are safe;
- tests are isolated;
- memory can be reclaimed when compilation/session ends;
- incremental compiler/LSP can deliberately retain/reuse arenas.

Builtin primitive IDs may be preallocated deterministically inside each context or represented by reserved IDs.

## Required node payloads

Model information structurally, for example:

- integer: signedness + width;
- float: representation/width;
- function: parameter `TypeID`s + result + effect/calling data if semantic;
- list/set: element `TypeID`;
- map: key/value IDs;
- struct/enum/newtype: declaration identity + generic args;
- generic parameter: owner + index/identity;
- generic instance: base declaration + argument IDs;
- quantity: numeric representation + canonical dimension/unit type data;
- tensor: element dtype + rank/dimension metadata according to language semantics;
- Result/Option/Lazy/Task: contained IDs.

Do not store `"List<u32>"` and parse it later.

## Type formatting

Create one pure formatter:

```go
func (tc *TypeContext) Format(id TypeID, mode FormatMode) string
```

Formatting is for diagnostics/docs/debugging only.

Support source-friendly names/aliases where useful, but ensure changing pretty-printing never changes compilation behavior.

## Recursive types

Design interning to support legal recursive nominal types without infinite structural construction. Nominal declaration IDs should break cycles naturally. Reject illegal unguarded alias cycles explicitly.

## Generic substitution

Implement substitution over `TypeID`s:

```go
func Substitute(tc *TypeContext, id TypeID, subst Substitution) TypeID
```

Memoize where helpful. Do not serialize a type to text, perform string replacement and parse it again.

## Integration plan

1. Introduce `TypeID`, `TypeContext`, primitive types and formatter alongside existing `staticType`.
2. Convert type checker internal expression results to `TypeID`.
3. Convert function signatures, bindings, generics, traits and declarations.
4. Convert IR/HIR type annotations.
5. Convert backend type switches.
6. Convert LSP hover/signature logic to format `TypeID`.
7. Remove old semantic `staticType.name` machinery once no longer needed.

Temporary adapters may parse legacy type syntax at source/declaration boundaries, but semantic passes may not depend on them after migration.

## Tests

- interning identity;
- nominal distinction;
- alias identity;
- newtype distinction;
- generic substitution;
- nested type equality;
- recursive declarations;
- concurrency safety if contexts are used in parallel;
- memory/lifetime behavior for discarded compilation contexts;
- formatter golden tests;
- test proving two differently formatted aliases still resolve to same canonical type when semantically identical;
- test proving changing display formatting cannot alter type checking.

## Acceptance criteria

- All semantic compiler type equality uses `TypeID`/canonical nodes.
- User-facing names are metadata, not identity.
- No backend performs type semantics by parsing a pretty-printed type.
- Generic substitution and compatibility operate on type graph nodes.
- The old string-centric type model is deleted or reduced to source parsing/formatting adapters only.
