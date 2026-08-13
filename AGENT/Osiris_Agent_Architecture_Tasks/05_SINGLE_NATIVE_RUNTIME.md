# Task 05 — Replace Generated C Runtime Duplication with One Canonical Native Runtime

## Architectural decision

Use a **portable C runtime library with a stable C ABI** as the canonical production runtime for native Osiris.

This is the best fit for the stated goal of raw C-like bare-metal performance because it provides:

- direct native code and data representation control;
- predictable ABI interoperability;
- no mandatory GC or heavyweight host runtime;
- static linking for single-file/native deployments;
- straightforward use from generated C today and from a future LLVM/custom machine-code backend later;
- broad OS/architecture portability;
- easy FFI interop with existing C ecosystems;
- independence from Go's runtime in compiled Osiris executables.

The Go compiler/tooling remains in Go. Do **not** rewrite the compiler merely to make the runtime C.

## Important interpretation of "one runtime"

Today Osiris effectively contains runtime semantics in several places: Go interpreter/runtime code plus large generated/native C runtime fragments. The immediate goal is to establish **one canonical production runtime implementation and ABI**, not to force every compiler data structure into C.

Native, VM and interpreter semantics must be specified once. Runtime primitives that represent Osiris values/resources should converge on the canonical runtime contract. During migration, Go interpreter adapters may exist temporarily, but they must not remain an independently evolving second specification.

Long-term options for tooling execution are:

- a thin cgo bridge to `libosiris_rt` where using the native runtime is appropriate;
- a process/dynamic ABI bridge for tools that must remain cgo-free;
- a deliberately labeled reference interpreter implementation used for validation, generated/tested against the same semantic contract.

Do not let "one runtime" produce pervasive cgo calls in compiler hot paths. The compiled program's runtime performance is the priority.

## Target artifact

Create a standalone runtime tree, for example:

```text
runtime/
  include/
    osiris/runtime.h
    osiris/value.h
    osiris/error.h
    osiris/task.h
    ...
  src/
    value.c
    memory.c
    collections.c
    error.c
    task.c
    ...
  platform/
    posix/
    windows/
    darwin/
  tests/
```

Build outputs:

```text
libosiris_rt.a
libosiris_rt.so / dylib / dll   (optional/shared distribution)
```

The default native compiler should statically link `libosiris_rt.a` unless the user explicitly chooses dynamic linking.

## Stop embedding an entire runtime in generated source

Inspect `internal/osiris/cbackend_native.go`, `cbackend_portable_runtime.go`, `cbackend_stdlib_runtime.go`, networking/TLS/process runtime files and other `cbackend_*runtime*` sources.

Separate:

A. code generation for the user program;
B. runtime implementation;
C. platform provider implementations.

Generated C for a program should contain program-specific code and calls into runtime headers/APIs. It should not contain thousands of lines implementing lists, values, errors, networking, tasks, TLS, etc.

## ABI design principles

Design a small versioned C ABI.

Rules:

1. Prefix exported symbols, e.g. `osr_`.
2. Do not expose unstable compiler-internal structs in public runtime headers.
3. Use explicit-width integer types.
4. Define ownership for every pointer/handle argument/result.
5. Define error propagation consistently.
6. Avoid C global mutable runtime state. Use an explicit runtime/context object where state is necessary.
7. Make allocator choice explicit/configurable if practical.
8. Preserve deterministic semantics independent of libc hash/randomization behavior.
9. Keep platform-specific behavior behind provider/vtable interfaces rather than `#ifdef` scattered throughout semantic code.
10. Version the ABI and runtime manifest so compiler/runtime incompatibility fails clearly.

Suggested root object:

```c
typedef struct osr_runtime osr_runtime;

typedef struct {
    uint32_t abi_version;
    /* allocator, capability/provider hooks */
} osr_runtime_config;

osr_status osr_runtime_init(osr_runtime **out, const osr_runtime_config *cfg);
void osr_runtime_destroy(osr_runtime *rt);
```

Do not blindly adopt this exact shape if current runtime semantics suggest a cleaner API, but preserve explicit context and lifecycle.

## Value representation and performance

This task is an opportunity to establish a native value ABI that does not require every statically typed scalar operation to box into a universal `Value`.

Rules:

- Native generated code should use raw C scalars for statically known primitive locals/parameters wherever ABI-safe.
- Heap/runtime objects are used for strings, collections, closures, tasks, dynamic values, big numbers, tensors as required.
- Avoid mandatory boxing of `i32`, `u64`, `f64`, `bool`, etc. in native hot loops.
- Keep checked arithmetic/bounds semantics exactly as specified.
- Resource handles must have explicit ownership/lifetime.

## Memory management

Do not add a tracing GC simply because Go has one.

Choose runtime memory management based on Osiris language semantics and performance. A pragmatic initial design is:

- value-semantic scalars inline;
- immutable/ref-counted heap objects where sharing is required;
- explicit owned handles for external resources;
- compiler/runtime-managed temporary lifetimes;
- region/arena allocation for clearly scoped compiler-generated temporaries where safe;
- cycle strategy explicitly documented if ref-counted object graphs can cycle.

Before changing memory representation, write ownership invariants and tests. Do not introduce unsafe use-after-free behavior merely to chase benchmark numbers.

## Runtime capabilities

Move only primitives requiring runtime privilege/representation into the runtime, such as:

- allocation/object lifecycle;
- string backing/storage primitives;
- collection backing tables;
- task/scheduler primitives;
- clocks/random/OS handles;
- filesystem/network/process primitives;
- FFI/unsafe memory bridge;
- big-number/provider primitives where not practical in Osiris;
- tensor/GPU provider bridge;
- structured error ABI support.

Task 06 moves higher-level algorithms/policy into Osiris source.

## Build system

Add reproducible runtime builds for supported platforms.

The Osiris compiler must be able to locate a runtime via:

1. bundled toolchain runtime;
2. explicit `--runtime-path`/toolchain configuration if needed;
3. version/ABI validation.

Do not shell out to compile the full runtime on every program build. Build/install/cache the runtime library separately.

## Testing

Required:

- runtime C unit tests independent of compiler;
- ASan/UBSan builds on supported CI hosts;
- native conformance suite linked against runtime library;
- ABI version mismatch tests;
- static-link test;
- dynamic-link test if supported;
- FFI ownership/lifecycle tests;
- stress tests for collections/tasks/errors/resource cleanup;
- performance benchmark comparison against current generated runtime path.

## Migration sequence

1. Inventory all generated runtime fragments and runtime APIs used by emitted C.
2. Write ABI design document.
3. Extract foundational value/error/memory primitives into `libosiris_rt`.
4. Change codegen to include runtime headers and link library.
5. Extract collections/tasks/OS/FFI/platform facilities in vertical slices.
6. Keep temporary generated compatibility shims only while a slice is migrating.
7. Delete old runtime generators after all users migrate.
8. Add repository guard tests preventing reintroduction of large runtime source blobs into program emission.

## Acceptance criteria

- Native compilation links one reusable Osiris runtime library.
- Generated program C no longer embeds the canonical runtime implementation.
- Runtime has a documented/versioned C ABI.
- Statically known primitive native code is not forced through a universal boxed-value API.
- Native executables do not depend on the Go runtime.
- Sanitizer/conformance/parity suites pass.
- Old duplicate generated runtime implementation code is deleted.
