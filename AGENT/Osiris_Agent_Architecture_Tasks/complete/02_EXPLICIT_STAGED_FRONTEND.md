# Task 02 — Introduce an Explicit Staged Front End

## Objective

Refactor the compiler front end so source processing is an explicit sequence of compiler passes with well-defined input/output representations. Stop treating the pre-macro and post-macro AST as if they were semantically interchangeable.

The target pipeline is:

```text
Source text
  -> Lex
  -> Parse (surface AST)
  -> Macro declaration collection / expansion
  -> Name and module resolution
  -> Desugaring / normalization
  -> Static type checking + inference
  -> Typed HIR
  -> Lowered IR / SSA
```

The exact split between desugaring and resolution may be adjusted, but the pass boundaries MUST be explicit and testable.

## Problem to eliminate

The current interpreter path type-checks, expands macros, then type-checks again. This is conservative but makes the front end conceptually ambiguous:

- Which tree is authoritative?
- Which pass owns name resolution?
- Can diagnostics refer to macro-generated locations?
- Does the LSP reason about pre-expansion or post-expansion declarations?
- Can a backend receive a tree that has not passed the same semantic stages as another backend?

The new architecture must answer those questions structurally.

## Required representations

Create named compiler-stage types. Do not pass a naked `*Program` everywhere and rely on comments to state its phase.

Suggested conceptual API:

```go
type ParsedProgram struct { ... }
type ExpandedProgram struct { ... }
type ResolvedProgram struct { ... }
type TypedProgram struct { ... }
```

These may wrap shared immutable nodes initially, but the Go type system should prevent accidentally passing a parsed tree to an IR builder expecting a typed tree.

Prefer immutable pass outputs or copy-on-write replacement over mutating one global AST in place.

## Pass manager / driver

Create one front-end driver used by interpreter, VM, IR CLI, native compiler, WASM compiler, GPU compiler where applicable, LSP semantic analysis, and tests.

Conceptual API:

```go
result, err := frontend.Compile(ctx, frontend.Request{
    Source: source,
    Mode: frontend.ModeTyped,
    Capabilities: caps,
})
```

or explicit pass calls if that is clearer.

Do not allow each backend to independently reproduce parser -> macro -> checker ordering.

The driver must preserve:

- source file identity;
- exact source spans;
- macro expansion origin chains;
- diagnostics from all passes;
- module/package context;
- compiler options/capabilities.

## Macro phase requirements

Macros must have a defined semantic phase.

At minimum:

1. Parse macro declarations and invocations as surface syntax.
2. Collect macro definitions according to documented scope/module rules.
3. Expand macros before ordinary static type checking of generated expressions.
4. Preserve source provenance: diagnostics inside expanded code must identify both the generated construct and the invocation/origin where useful.
5. Prevent arbitrary later compiler passes from invoking macro expansion again.

If macro definitions themselves require validation, create a macro-specific validation phase rather than running the full ordinary type checker over the unexpanded program simply because it was convenient.

## Name resolution

Introduce an explicit resolution result rather than repeatedly searching textual scopes by name in later compiler passes.

Each resolved identifier should carry or refer to a stable symbol identity such as:

```go
type SymbolID uint32
```

The symbol table should distinguish locals, parameters, functions, types, traits, modules, imports, constants, builtins, etc.

Later compiler passes should resolve by `SymbolID`, not redo lexical name lookup from strings.

The parser still stores source spelling. The resolver converts spelling + scope to identity.

## Desugaring / normalized HIR input

Identify syntax sugar that should not burden every backend. Examples may include shorthand function bodies, propagation syntax, high-level loop forms, pattern syntax, or other conveniences.

Lower those constructs exactly once into a smaller semantic representation before typed HIR/IR generation.

Do not prematurely erase source spans or user-facing names needed by diagnostics.

## Type checking

The static checker should operate on a resolved/normalized representation, not an arbitrary raw parsed tree.

It must annotate or produce typed nodes explicitly. Do not require later passes to recompute expression types.

## LSP integration

The LSP needs both surface and semantic views.

- Formatting/syntax highlighting can consume parsed syntax.
- Go-to-definition/references should consume resolver symbols.
- Hover/type information should consume typed results.
- Macro-related navigation should use expansion-origin metadata.

Do not duplicate a separate quasi-compiler inside `lsp.go`/`lsp_ide.go`.

## Error handling

Define a compiler diagnostics accumulator with stable ordering. A pass can stop when continuing would be unsound, but recoverable front-end errors should allow collection of multiple diagnostics when practical.

No pass should emit diagnostics directly to stdout/stderr.

## Migration approach

1. Introduce pass-result wrapper types around current AST to establish compile-time phase boundaries.
2. Centralize current parse/macro/typecheck sequencing in one front-end package/API.
3. Convert all execution backends and CLI analysis commands to call the central pipeline.
4. Introduce symbol resolution identities.
5. Move static typing to resolved nodes.
6. Add typed HIR as specified in Task 15.
7. Delete duplicate ordering logic and pre/post expansion checker hacks once no callers require them.

Do not attempt a big-bang AST rewrite.

## Required tests

Add tests proving:

- parser output cannot be sent directly to backend lowering through public/internal APIs;
- macro expansion occurs exactly once;
- a generated identifier resolves using documented hygiene/scope rules;
- diagnostics preserve invocation/origin spans;
- every backend receives equivalent typed input for the same source program;
- LSP definition/hover behavior uses the same symbols/types as the compiler;
- invalid macro output fails in the semantic phase with deterministic diagnostics;
- no stale pre-expansion type result survives after macro expansion changes code.

## Acceptance criteria

Complete when:

- there is one explicit front-end pipeline;
- phases have distinct Go-level types/APIs;
- ordinary type checking occurs against the post-expansion resolved representation;
- name identity is not repeatedly reconstructed by string lookup in downstream passes;
- all backends/tooling consume the same front-end results;
- the old "type-check raw AST, expand, type-check again" architecture is removed unless a narrowly named macro-validation pass genuinely requires an early check.

## Completion record — 2026-08-13

Completed the staged-front-end transition without prematurely implementing the
typed-HIR/type-graph work assigned to later tasks.

- Added named `ParsedProgram`, `ExpandedProgram`, `ResolvedProgram`, and
  `TypedProgram` stages plus the single `Runtime.Frontend` driver.
- The driver performs parse input handling, macro expansion exactly once,
  structural validation, deterministic declaration resolution, and ordinary
  static checking in that order. The pre-expansion `Session` type-check pass
  and evaluator-time macro expansion were removed.
- `ResolvedProgram` allocates stable `SymbolID`s for declarations and resolved
  identifier uses. Checked bindings retain their declaration ID; this is an
  explicit bridge pending Task 15's typed-HIR rewrite.
- VM execution, VM module compilation, IR building, native C/WASM through IR,
  GPU validation/planning, inference, alias analysis, purity analysis, and
  interpreter sessions now pass through the common frontend route. New
  `RunVMFromTyped` and `BuildIRFromTyped` APIs make the semantic boundary
  explicit; raw compatibility APIs immediately invoke it.
- LSP keeps the parsed tree for formatting and macro-call navigation, while
  its diagnostics/hover types and declaration IDs use the shared typed
  frontend result. This preserves existing macro source ranges until Task 15
  adds node-addressable expansion-origin chains.
- Added frontend-stage tests for expansion, post-expansion diagnostics,
  declaration/reference identities, VM/IR/native consumption, and LSP/compiler
  SymbolID/type agreement. Added `docs/design/FRONTEND-PIPELINE.md` as the
  durable boundary contract.

Validation completed:

```sh
GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run 'Test(Frontend|Macro|LSP.*Macro|BytecodeVMParity|VMParityCorpus|CoreSemanticsConformance)' -count=1
GOCACHE=/tmp/osiris-go-cache go test ./...
GOCACHE=/tmp/osiris-go-cache go test -race ./internal/osiris -run 'Test(Frontend|LSP.*Macro|VMParityCorpus|CoreSemanticsConformance)' -count=1
GOCACHE=/tmp/osiris-go-cache go vet ./...
./scripts/check-docs.sh
git diff --check
```
