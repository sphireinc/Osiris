# Task 09 — Eliminate Compiler Decisions Based on Type Strings

## Objective

After Task 08 establishes canonical `TypeID`s, remove all important compiler/backend behavior that depends on formatting, parsing, splitting, prefix-checking, suffix-checking, or canonicalizing type strings.

This task is an audit and enforcement pass. It is separate from building the type graph because string-based decisions tend to survive migrations in utility code and backends.

## Search scope

Search the full repository for patterns including:

```text
.name comparisons on static types
"dynamic"
"list<"
"map<"
"result<"
"option<"
strings.HasPrefix(type...)
strings.HasSuffix(type...)
strings.Split(type...)
strings.Trim...
fmt.Sprintf used to construct semantic types
parseType called on a type produced by compiler formatting
canonicalTypeName
normalizeTypeName
```

Also inspect code generators and tests for hidden textual dispatch.

## Classification

Every use of a type string must be categorized:

### Allowed

- parsing source type syntax;
- diagnostics;
- formatter output;
- documentation;
- serialization of a stable explicitly versioned metadata format where the string is data, not semantic runtime identity;
- human-facing debug/disassembly text.

### Forbidden

- type equality;
- assignability;
- overload selection;
- numeric promotion;
- generic substitution;
- trait resolution;
- ABI selection;
- native C type lowering;
- VM opcode selection;
- WASM lowering;
- tensor/quantity semantic checking;
- deciding whether a type is collection/function/result/etc.;
- hashing semantic types;
- runtime representation selection.

Replace forbidden uses with `TypeID`/`Kind`/typed node queries.

## API rule

Create type-query helpers where needed:

```go
func (tc *TypeContext) Kind(id TypeID) Kind
func (tc *TypeContext) AsFunction(id TypeID) (FunctionType, bool)
func (tc *TypeContext) IsInteger(id TypeID) bool
func (tc *TypeContext) MapParts(id TypeID) (key, value TypeID, ok bool)
```

Do not scatter raw arena indexing and kind assertions everywhere if a small query API makes invariants clearer.

## Backend migration

Native code generation must map canonical types to ABI/native representation through a structured lowering table/switch.

Bad:

```go
if strings.HasPrefix(t.String(), "List<") { ... }
```

Required concept:

```go
switch tc.Kind(t) {
case KindList:
    elem := tc.ListElement(t)
    ...
}
```

Apply equivalently to VM/WASM/GPU.

## Guardrails

Add static repository tests/lints if practical that fail when compiler-semantic packages introduce known forbidden patterns. A lightweight AST-based Go test can inspect semantic packages for calls such as `strings.HasPrefix` on known type-display APIs.

Do not create a brittle grep that blocks legitimate diagnostic formatting without context; target semantic packages/functions.

## Acceptance criteria

- Semantic type behavior is invariant under type pretty-printer changes.
- Backends consume type IDs/nodes directly.
- String parsing occurs only at source/metadata boundaries.
- There is no parse-format-parse cycle inside compiler semantic processing.
- Add at least one regression test that changes/uses an alias display name while proving codegen/type behavior remains identical.
