# Task 04 — Replace VM Local Name Lookups with Compiler-Assigned Integer Slots

## Objective

Remove string-keyed lexical local lookup from the VM hot path. Compile parameters and local variables to integer slots determined before bytecode execution.

Names must remain available for diagnostics, debug information, stack traces, REPL inspection and tooling, but runtime instructions for ordinary lexical locals SHALL address them by integer slot/index.

## Current issue

The current VM frame uses maps such as:

```go
type vmFrame struct {
    parent  *vmFrame
    locals  map[string]Value
    mutable map[string]bool
}
```

and performs chained lexical name lookup. This adds hashing, allocation, string comparisons and unpredictable memory access to basic variable operations.

## Target model

During name resolution/lowering, assign each lexical binding a stable symbol identity. During bytecode compilation, assign each runtime local a slot within its frame/function.

Conceptually:

```go
type LocalSlot uint32

type vmFrame struct {
    parent *vmFrame // only if required for closures/upvalues
    locals []Value
}
```

Mutability metadata should be compiler-known. If runtime mutability checks remain necessary for defensive validation, encode compact metadata/bitsets rather than `map[string]bool`.

Bytecode should contain instructions such as:

```text
LOAD_LOCAL  r3, slot7
STORE_LOCAL slot7, r2
```

Parameters should occupy predetermined slots.

## Lexical scopes

Do not allocate a new string map simply because source introduces a lexical block.

Prefer a function-level local-slot table with compiler-determined live ranges. Initial implementation may allocate one slot per lexical declaration for the whole function; slot reuse can be added later.

Shadowing is solved by symbol identity:

```osiris
let x = 1
{
    let x = 2
    emit x
}
emit x
```

The two `x` declarations receive different `SymbolID`s and different slots even though source spellings match.

## Closures and captured locals

Captured variables require explicit lowering.

Classify locals as:

- ordinary stack/frame local;
- captured immutable value;
- captured mutable cell/upvalue.

A closure must not perform string lookup against an ancestor frame.

Introduce upvalue/capture descriptors, for example:

```go
type Capture struct {
    SourceSlot LocalSlot
    Kind CaptureKind
}
```

and instructions/closure metadata that address captures by index.

Preserve current Osiris semantics around task snapshots and mutable captures. Do not accidentally turn snapshot semantics into shared mutable state.

## Debug metadata

Create a side table mapping bytecode location + function + slot to source information:

```text
slot 7 -> SymbolID 42 -> source name "count" -> declaration span
```

This table is not consulted for ordinary execution.

Use it for:

- stack traces;
- debugger local inspection;
- disassembly;
- error messages;
- profiler labels.

## Modules/globals/builtins

This task targets lexical locals. Module/global symbols may use a separate indexed global table. Builtins should also eventually resolve to stable callable IDs rather than repeated string lookup, but do not expand scope unnecessarily unless required by the implementation.

Dynamic namespaces, reflection-like dictionaries, module dictionaries, or explicit `dynamic` values may legitimately remain hash maps.

## Compiler changes

The bytecode compiler must consume resolved symbols rather than raw identifier strings wherever possible.

Build a function-local layout pass:

1. enumerate parameters;
2. enumerate local declarations;
3. identify captures;
4. assign slots/capture indices;
5. emit bytecode using those indices;
6. emit debug metadata separately.

Do not perform slot allocation opportunistically while emitting instructions if doing so makes branch/scoping behavior order-dependent.

## VM changes

- Replace `locals map[string]Value` with slices/arrays.
- Replace local lookup opcodes/paths with direct indexing.
- Detect compiler-produced invalid slot indices defensively and report an internal VM error.
- Do not turn user program mistakes into runtime "unknown local" errors; unresolved names should have been rejected earlier.

## Performance tests

Add microbenchmarks for:

- tight loop reading one local;
- tight loop updating one `var`;
- function call with N parameters;
- nested scopes with shadowed names;
- closure read;
- mutable capture;
- task snapshot of captured frame.

Record before/after allocations/op and ns/op.

Do not set an arbitrary percentage target if profiles vary by host, but this task is unsuccessful if local access remains map-based or if performance regresses materially without a documented reason.

## Correctness tests

Cover:

- shadowing;
- recursion;
- nested closures;
- mutable captures;
- task snapshot semantics;
- break/continue scopes;
- errors/defer unwinding;
- debugger/disassembler names;
- large numbers of locals;
- unused locals;
- same identifier spelling in distinct functions/scopes.

## Acceptance criteria

- Ordinary lexical VM loads/stores contain no string-map lookup.
- Names are kept only in compiler/debug metadata, not execution addressing.
- Closures access captures by index, not ancestor string lookup.
- Tests demonstrate exact semantic parity with the interpreter/native backend for scope/capture behavior.
- Benchmarks are added and results documented in the task completion note.
