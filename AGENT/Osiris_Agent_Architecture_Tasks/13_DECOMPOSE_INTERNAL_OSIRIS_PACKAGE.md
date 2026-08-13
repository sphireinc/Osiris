# Task 13 — Decompose the `internal/osiris` Monolith Without Changing Semantics

## Objective

Break the current giant `internal/osiris` Go package into explicit architectural packages with controlled dependencies. This task is primarily code organization and dependency enforcement; do not combine it with unnecessary semantic rewrites.

## Target package topology

A pragmatic target is approximately:

```text
internal/source        source files/spans/diagnostics primitives
internal/syntax        lexer/parser/surface AST/formatter
internal/frontend      macro expansion, resolution, front-end orchestration
internal/types         canonical type graph/type checking support
internal/hir           typed high-level IR
internal/ir            lower IR/SSA/CFG analyses
internal/interp        reference interpreter
internal/vm            bytecode compiler/VM
internal/runtime       shared Go-side runtime adapters/value APIs where still needed
internal/backend/c     C/native code generation
internal/backend/wasm  WASM lowering/provider glue
internal/backend/gpu   GPU lowering/provider glue
internal/stdlib        builtin/runtime-primitive registration/bootstrap support
internal/tooling/lsp   LSP
internal/tooling/debug debugger/trace/heap tools as appropriate
```

Do not create dozens of one-file packages. The goal is architectural boundaries, not maximum package count.

Task 16 defines the broader compiler/runtime/platform layering; use it as the dependency policy while performing this migration.

## Required dependency direction

At minimum enforce:

```text
source
  ↑
syntax
  ↑
frontend/types
  ↑
hir
  ↑
ir
  ↑
interp/vm/backends
```

Runtime/platform services must not import compiler front-end packages merely to execute values.

Tooling may depend on published compiler APIs, but compiler packages must not import LSP/tooling.

Backends must not import each other.

## Migration strategy

Do not move all files in one commit.

### Stage 1 — dependency inventory

Generate/document imports and logical ownership for every `internal/osiris/*.go` file.

Identify shared foundational types currently causing package entanglement: `Value`, source spans, errors, AST nodes, type representations, capabilities, module IDs.

### Stage 2 — extract leaf/tool packages

Move packages with relatively clean dependencies first, e.g. source/diagnostic primitives or isolated analyses/tooling.

### Stage 3 — syntax/front end/types

Move lexer/parser/AST, then type system/front-end passes using Task 02/08 architecture.

### Stage 4 — IR and execution backends

Move IR/SSA/CFG, VM, interpreter and native/WASM/GPU backend code.

### Stage 5 — runtime/stdlib/platform

Extract runtime and platform provider interfaces while coordinating with Tasks 05/06/16.

### Stage 6 — delete compatibility facade

A temporary `internal/osiris` facade may re-export APIs to keep CLI/tests compiling during migration. It MUST be deleted or reduced to a deliberately tiny public orchestration facade before completion.

## Avoid import cycles

Do not solve cycles by:

- moving everything into a new `common` package;
- introducing interface abstractions for every struct regardless of need;
- using global callback registries;
- duplicating core type definitions.

Instead identify the lower-level concept and move it to the correct layer.

A small `source` or `core` package is acceptable only for genuinely foundational dependency-free types.

## API minimization

As files move, reduce exported identifiers. Package boundaries should expose narrow APIs.

Examples:

- parser exposes `Parse` and syntax types, not internal lexer mutable state;
- type system exposes `TypeID` queries, not arbitrary mutation of arena internals;
- IR exposes builders/verified structures, not backend-specific hooks;
- runtime exposes primitive interfaces, not compiler AST.

## Tests

Move tests with their owning packages. Create integration tests for cross-package compiler pipeline separately.

Add an architecture test that checks forbidden import directions. This can use `go list -json` or parse Go imports.

Examples of forbidden edges:

- `internal/types -> internal/vm`
- `internal/ir -> internal/backend/c`
- `internal/frontend -> internal/tooling/lsp`
- `internal/runtime -> internal/syntax`

## CLI

Keep `cmd/osiris/main.go` thin. Move command implementation logic to appropriate packages. The CLI should orchestrate APIs rather than own compiler/runtime behavior.

## Acceptance criteria

- `internal/osiris` is no longer the home of essentially the whole system.
- Package dependency direction is documented and enforced by tests.
- No cycles or junk-drawer `common` package are introduced.
- Compiler, runtime, backends and tooling can be understood from package topology.
- All existing semantics/conformance tests pass after migration.
