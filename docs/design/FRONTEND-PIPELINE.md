# Explicit frontend pipeline

Osiris source has one semantic pipeline:

```text
source -> ParseSurface -> macro expansion -> validation -> resolution -> type check -> TypedProgram -> IR/VM/native/WASM/GPU
```

`ParsedProgram`, `ExpandedProgram`, `ResolvedProgram`, and `TypedProgram` are
distinct Go-level stages. Backend-facing APIs consume `TypedProgram` through
`RunVMFromTyped` and `BuildIRFromTyped`; compatibility entry points accepting
`*Program` immediately invoke the same frontend driver.

Macro expansion happens once, before ordinary validation and type checking.
The parser remains the surface syntax authority for formatting and syntax
highlighting. The expanded tree is the semantic authority for execution and
artifact generation. Diagnostics retain the source span carried by syntax;
macro invocation navigation remains a surface-LSP responsibility until the
typed-HIR work adds node-addressable expansion-origin chains.

Resolution allocates stable `SymbolID` values for surface declarations. The
initial resolver records declaration identity and attaches IDs to checked
bindings without rewriting the AST. Task 15 owns replacement of the shared AST
with typed HIR and full identifier-reference rewriting. LSP type diagnostics
already consume the shared frontend result; its source-navigation view retains
surface macro locations during the transition.

There is no raw-AST type check before expansion and no evaluator-time macro
expansion. Module VM loading uses the same session-aware frontend path.
