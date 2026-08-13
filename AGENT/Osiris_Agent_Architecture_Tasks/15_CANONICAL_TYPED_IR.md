# Task 15 — Establish One Canonical Typed HIR/IR as the Semantic Center of the Compiler

## Objective

Stop allowing each execution backend to independently interpret raw AST semantics. Establish a canonical typed high-level IR (HIR), followed by a lower IR/SSA representation, as the authoritative compiler representation consumed by interpreter/VM/native/WASM/GPU lowering as appropriate.

## Target pipeline

```text
Source
 -> Lexer
 -> Surface AST
 -> Macro expansion
 -> Name/module resolution
 -> Type checking/inference
 -> Typed HIR              [language semantics normalized here]
 -> Lower IR / CFG / SSA   [control/data-flow lowering]
 -> optimizer
 -> backend
```

The interpreter may initially execute typed HIR rather than lower SSA if that keeps it simple, but it must not remain a separate raw-AST semantic implementation indefinitely.

## Typed HIR requirements

Every HIR expression/statement must carry:

- canonical `TypeID` for value-producing nodes;
- resolved `SymbolID`s instead of textual identifier lookup;
- source span/origin;
- normalized semantic operation;
- effect/capability metadata where needed;
- resolved function/trait/generic targets where compile-time resolution is possible.

HIR should remove syntax sugar that backends should not need to understand.

Examples:

- shorthand function bodies normalized;
- compound propagation constructs lowered to explicit semantic nodes;
- high-level pattern syntax represented as typed patterns/decision structures;
- calls identify target signature/instantiation;
- locals identify symbols, not names.

Do not over-lower HIR into machine-like operations; preserve enough structure for useful diagnostics and high-level optimization.

## Lower IR / SSA

Reuse and evolve `internal/osiris/ir.go`, `ssa.go`, `cfg.go`, `dom.go`, `loop.go` rather than inventing an unrelated third compiler IR.

The lower IR must have:

- typed values;
- explicit basic blocks;
- explicit terminators;
- explicit calls;
- explicit memory/cell operations for mutable state;
- explicit error/control-flow edges as appropriate;
- explicit task/runtime operations;
- verifier enforcing structural/type invariants.

No lower-IR instruction should need to ask the parser/type checker what an operand means.

## Interpreter strategy

Create a typed-HIR interpreter as the reference semantic engine, or progressively adapt the current evaluator to consume HIR.

Benefits required:

- interpreter sees the same resolved types/symbols as compilers;
- macro/type semantics occur once;
- runtime errors are not substitutes for static checking;
- differential tests compare execution engines from common semantic input.

Do not lower HIR back into AST for the interpreter.

## VM strategy

Bytecode compiler consumes HIR or lower IR. It must not independently resolve names, infer types or expand macros.

Task 04's local slots should derive naturally from resolved symbols/HIR function layouts.

## Native strategy

Native C codegen consumes verified lower IR/SSA. Backend-specific validation should check capability/support constraints, not redo language semantics.

Move optimizations that are language/backend independent to IR passes before C emission.

## WASM/GPU

Consume the same typed IR subset. Unsupported operations are rejected based on explicit IR capabilities. Do not implement separate AST walkers with subtly different semantics.

## IR verification

Create strong verifiers executed after construction and after transforms:

- all value IDs defined before use according to SSA rules;
- type IDs valid;
- branch conditions bool;
- call signature matches;
- return type matches;
- block terminators valid;
- no unresolved symbol/type variables;
- mutable cell operations use compatible types;
- backend-independent capability/effect invariants.

Verifier failure is an internal compiler error, never a user diagnostic.

## Serialization/debugging

Keep `osiris ir`, CFG/SSA visualization tools working. Update output to display friendly source names/types through metadata while internal identity remains IDs.

If IR serialization is introduced, version it explicitly. Do not accidentally make debug text a stable binary interface.

## Migration plan

1. Define HIR package/types.
2. Lower one small Core subset from typed front end to HIR.
3. Convert interpreter/VM paths for that subset.
4. Expand HIR coverage feature-by-feature with conformance tests.
5. Convert native backend to lower from canonical IR.
6. Move shared optimizations before backend boundaries.
7. Convert remaining alternative backends.
8. Delete backend AST semantic walkers/fallbacks.

Use feature-complete vertical slices; do not leave two permanent semantic pipelines.

## Tests

- golden HIR for representative programs;
- HIR verifier negative unit tests;
- AST spelling differences desugaring to equivalent HIR;
- same HIR used for interpreter/VM/native tests;
- generic instantiation/trait resolution encoded once;
- source span preservation;
- differential execution from same typed input;
- repository guards against backend imports of parser internals if architecture permits.

## Acceptance criteria

- There is exactly one post-type-check semantic representation.
- Backends do not independently expand macros, resolve names or infer ordinary types.
- Interpreter no longer defines separate raw-AST semantics for migrated Core 1 constructs.
- IR/HIR is typed with canonical TypeIDs/SymbolIDs.
- Verifier catches malformed compiler output before backend execution.
