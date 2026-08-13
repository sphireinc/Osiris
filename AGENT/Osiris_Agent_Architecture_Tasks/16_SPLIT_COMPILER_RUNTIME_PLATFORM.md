# Task 16 — Enforce Compiler / Runtime / Platform / Tooling Separation

## Objective

Establish strict architectural layers so compiler logic, language runtime semantics, OS/platform providers and developer tooling evolve independently.

This task defines dependency contracts. Task 13 performs the Go package decomposition; Task 05 creates the canonical native runtime.

## Required layers

### 1. Compiler layer

Owns:

- source/diagnostics;
- lexer/parser/AST;
- macro expansion;
- resolver/symbol tables;
- canonical type graph/type checker;
- HIR/IR/SSA;
- optimization;
- backend lowering/code generation.

Must not directly perform user program I/O as part of semantics.

### 2. Runtime layer

Owns language execution primitives:

- heap/runtime object lifecycle;
- strings/collections backing primitives;
- errors/unwinding support;
- closures/tasks/runtime synchronization;
- resource handles;
- FFI/unsafe primitives;
- capability/provider dispatch.

Runtime must not import parser/AST/type checker.

### 3. Platform-provider layer

Owns OS/environment-specific implementation:

- filesystem descriptors;
- sockets;
- clocks;
- process creation;
- TLS provider;
- GPU provider;
- platform entropy;
- dynamic library loading.

Expose narrow interfaces to the runtime.

Platform providers must not know Osiris source AST or compiler IR.

### 4. Standard library

Mostly Osiris source as Task 06 defines. Calls runtime primitives through stable modules/intrinsics.

### 5. Tooling

LSP, formatter CLI, debugger, visualization, docs generators consume compiler/runtime APIs. They do not become compiler dependencies.

## Capability architecture

Preserve and strengthen explicit capability injection.

Program execution should receive a runtime/context configured with allowed providers/capabilities. Do not access arbitrary process globals directly from stdlib implementations when a capability interface exists.

This is essential for deterministic testing, sandboxing and embedding.

## No semantic duplication

Examples of forbidden architecture:

- interpreter implements one map equality rule while native runtime implements another;
- C backend contains an independent HTTP policy implementation also present in Go runtime;
- LSP recreates generic resolution rules;
- GPU backend parses type strings independently;
- runtime imports parser to execute a thunk.

Shared semantic decisions live in compiler typed IR or canonical runtime primitives.

## Platform interfaces

Define narrow provider contracts based on capabilities, not giant OS objects.

Example concepts:

```text
ClockProvider
EntropyProvider
FileSystemProvider
NetworkProvider
ProcessProvider
TLSProvider
GPUProvider
DynamicLibraryProvider
```

Avoid one `Host` interface with dozens of unrelated methods if it makes testing/substitution difficult.

## Error translation

Platform errors must be normalized exactly once into structured Osiris runtime errors with stable categories/codes. Backends must not expose raw platform-specific errno/message differences as language semantics unless explicitly documented metadata.

## Testing

- fake platform providers for deterministic tests;
- import-boundary architecture tests;
- native runtime tests with provider substitution;
- capability-denied behavior parity;
- ensure compiler tests do not require network/filesystem except fixture access;
- ensure LSP can use compiler packages without initializing runtime platform facilities.

## Acceptance criteria

- Dependency graph visibly separates compiler, runtime, platform and tooling.
- Runtime has zero AST/parser dependency.
- Compiler semantic packages have zero platform-service dependency.
- Platform code receives no compiler AST/HIR.
- LSP/tooling is downstream of compiler, never upstream.
- Capability/provider behavior is explicit, testable and deterministic.
