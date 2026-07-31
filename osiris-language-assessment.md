# Osiris: Current Position, Major Gaps, and the Path to General-Purpose Maturity

> **Executive assessment**
>
> Osiris is best described today as an **ambitious deterministic technical-computing language** - not yet a general-purpose replacement for C, C++, or Go.
>
> It already offers several capabilities that are more modern or more specialized than those languages, including exact numeric families, physical quantities, tensors, linear algebra, structured errors, `Result`/`Option`, payload-carrying enums with pattern matching, deterministic tasks, macros, codecs, and extensive compiler-analysis tooling.

---

## At a Glance

| Area | Osiris Today | C / C++ / Go |
|---|---|---|
| **Type system** | Runtime value model with a limited set of checked binding annotations | Mature static type systems |
| **User-defined data** | Enums with payloads; no general structs | Structs; C++ classes; Go structs and interfaces |
| **Numeric computing** | Exceptionally rich built-in numeric, tensor, unit, and scientific support | Usually requires libraries |
| **Memory** | High-level immutable values; no raw memory access | C manual memory, C++ RAII, Go garbage collection |
| **Concurrency** | Tasks, `spawn`, `await`, `parallel`, cancellation, streams, race, and select | Mature thread and goroutine ecosystems across production backends |
| **Native compilation** | Growing but feature-specific C backend | Complete production compilation |
| **Interoperability** | FFI is designed but not implemented | Mature C ABI and foreign-language integration |
| **Ecosystem** | Small built-in library and repository-local modules | Large package ecosystems and mature tooling |

---

# The Largest Missing Language Features

## 1. A Complete Static Type System

This is probably the single biggest gap.

Current annotations such as:

```osiris
let size u32 = 3
let name string = "Europa"
```

check values against a closed list of built-in types.

Osiris does not yet have:

- Typed function parameters and return types
- User-defined struct types with declarative codec/tooling field tags
- Type aliases and newtypes
- Generic functions and generic containers
- Interfaces, traits, or protocols
- Compile-time method checking
- Typed collections such as `list<u32>`
- Function types such as `func(u32) -> string`
- Complete compile-time checking before execution

Compared with Go, something equivalent to the following is missing:

```go
type Spacecraft struct {
    Name   string
    Health float64
}

type Reporter interface {
    Report() string
}

func Map[T, U any](values []T, convert func(T) U) []U
```

Enums give Osiris strong sum types, but they do not replace general product types, interfaces, and generics.

> **Why this matters:** Without a coherent static type system, larger programs become harder to model, verify, refactor, and optimize before execution.

---

## 2. General User-Defined Data Structures

The runtime has lists, tensors, enums, errors, tasks, and other specialized values, but no ordinary language-level:

- Structs
- Maps or dictionaries
- Sets
- Tuples with distinct static element types
- Methods or receiver functions
- Encapsulation or private fields
- Custom collection types
- Iterator or generator protocols

A substantial application needs a convenient way to represent entities such as:

- Users
- Requests
- Configurations
- Syntax trees
- Database rows
- Application state

> **Key limitation:** Osiris can represent specialized computational values well, but it cannot yet model ordinary application-domain data with the same ease.

---

## 3. A Defined State and Mutation Model

Osiris favors immutable lexical bindings and copy-on-write operations such as `setat`.

Loop state is expressed through shadowing:

```osiris
let total = 0

for value in values {
  let total = total + value
}
```

That approach is deterministic, but the language lacks a broader model for:

- Mutable local variables
- Mutable struct fields
- References or borrowing
- Object identity
- Shared mutable state
- Atomics and synchronization primitives
- Controlled in-place updates for large structures

Osiris does not necessarily need C-style unrestricted mutation, but it does need a deliberate answer for efficient stateful programs.

> **Design challenge:** Preserve determinism while making stateful workloads practical and efficient.

---

## 4. Complete Backend Parity

The interpreter is currently the most complete implementation.

The public VM supports a broad surface, but some control-flow programs still delegate to the interpreter. Native C supports a tested subset, while WASM and GPU generation are narrower or experimental.

Recent backend work is recorded in:

- `tasks/completed/107-fix-lazy-loop-binding-stack-overflow.md`
  Removed the deep lazy-loop stack overflow across supported execution paths.

- `tasks/completed/108-make-emit-consistent-in-sequential-blocks.md`
  Made `emit` consistent across sequential executable block contexts.

- `tasks/completed/109-compile-mission-control-showcase-through-native-c.md`
  Added a broad, parity-tested native integration slice while retaining
  explicit compile-time rejection outside the supported surface.

- `tasks/115-close-general-purpose-backend-parity-gaps.md`
  Tracks the remaining stable VM and native C parity audit without treating
  one showcase program as the language boundary.

In Go, users generally expect any valid program to compile. In Osiris, users must still consult:

```text
newdocs/src/support-matrix.json
```

before relying on native, WASM, or GPU execution.

> **Production expectation:** A valid Osiris program should behave consistently across every officially supported backend.

---

## 5. Resource Management

Osiris explicitly has no:

- `defer`
- `finally`
- Destructors
- Finalizers

Streams can be closed manually, but the language lacks a universal scope-bound cleanup mechanism.

It needs something equivalent in purpose to:

| Language | Resource-management mechanism |
|---|---|
| C++ | RAII |
| Go | `defer` |
| Java / C# | `try/finally` or `using` |
| Rust | Deterministic `Drop` |

This matters for:

- Files
- Sockets
- Streams
- Native buffers
- Database transactions
- Locks
- Future FFI handles

> **Core requirement:** Cleanup must be reliable even when a function returns early, fails, or propagates an error.

---

## 6. Real Foreign-Function Interoperability

The repository has a thoughtful FFI design, but it remains design-only.

Osiris cannot yet conveniently call arbitrary C or Go libraries.

Missing pieces include:

- FFI declaration syntax
- Manifest and capability loading
- Stable public C ABI
- Struct and buffer marshalling
- Callbacks
- Ownership transfer
- Native library packaging
- Bindings generation

Without FFI, every missing library must be reimplemented inside either:

- The Osiris Go runtime
- The generated C runtime

> **Strategic consequence:** FFI is the bridge between a young language and the existing software ecosystem.

---

## 7. A Broader Standard Library

The existing library is unusually strong in mathematics, but narrow for general application development.

Major missing areas include:

- HTTP clients and servers
- TLS
- UDP, DNS, and richer socket APIs
- Process creation and management
- Environment and command-line APIs
- Regular expressions
- Compression and archive formats
- Database access
- Logging and structured telemetry
- Configuration formats and schema validation
- Filesystem watching
- OS signals beyond the current specialized surface
- Internationalization and locale support

For example, `std/net` currently exposes essentially:

- TCP listen
- TCP accept
- TCP dial

> **Current profile:** Strong technical-computing foundation, limited application-platform surface.

---

## 8. A Language-Level Testing Ecosystem

There is extensive Go-based testing for the implementation, but no Osiris-project equivalent of:

```bash
go test ./...
```

The language needs:

- A standard test declaration or convention
- Assertions and test fixtures
- Test discovery
- Parameterized tests
- Benchmarks
- Coverage
- Fuzzing
- Golden or snapshot testing
- Mock clocks, filesystems, networks, and task schedulers

This is essential for people building applications **in Osiris**, rather than developing Osiris itself.

> **Maturity signal:** A language becomes practical when its users can confidently test their own software without leaving the language ecosystem.

---

## 9. Package Ecosystem and Project Tooling

Osiris already has:

- Imports
- Semantic versions
- Lock files
- Dependency reports
- Cache synchronization

What it lacks is the mature ecosystem surrounding established languages:

- A public package index
- Package publishing and signing
- Package discovery
- Workspaces and multi-module projects
- Dependency license and vulnerability auditing
- Widely used build conventions
- Reproducible third-party binary dependencies
- A meaningful community library ecosystem

> **Important distinction:** Dependency mechanics exist, but the surrounding distribution and trust ecosystem is still missing.

---

## 10. Production-Grade Editor and Debugging Support

The tooling is impressive for a young language.

Osiris already includes:

- Formatter
- REPL
- Debugger
- Source maps
- IR inspection
- SSA inspection
- CFG inspection
- Inference
- Alias analysis
- Language Server Protocol support

The LSP remains relatively narrow, however.

It still needs stronger support for:

- Completion
- Find-all-references
- Signature help
- Semantic tokens
- Cross-file rename and refactoring
- Workspace-wide analysis
- Code actions and automatic fixes
- Inlay type hints
- Integrated test and benchmark discovery

> **Tooling outlook:** The compiler-analysis foundation is unusually strong; the next step is turning those capabilities into a polished day-to-day developer experience.

---

# Bottom Line

Osiris is already richer than C in several areas:

- Safe high-level values
- Pattern matching
- Structured errors
- Scientific types
- Deterministic data processing

It is also more numerically ambitious than Go's core language.

However, C, C++, and Go remain vastly ahead in:

1. Static type abstraction
2. User-defined data modeling
3. Complete native execution
4. Resource management and FFI
5. Standard libraries and packages
6. Testing, tooling, compatibility, and production maturity

---

# Highest-Value Next Milestone

The most valuable next language-design milestone would be a coherent user-defined type system containing:

- Structs with Go-style field tags
- Typed functions
- Function types
- Typed collections
- Generics
- An interface or trait mechanism

After that, the next priorities should be:

1. Complete native-backend parity
2. Deterministic resource management
3. Production-grade FFI
4. A language-native testing system
5. Broader standard-library coverage

> **Final assessment**
>
> Osiris already has the foundation of a compelling deterministic scientific and technical-computing language. The transition to a genuinely general-purpose language depends less on adding more specialized primitives and more on completing the abstractions, runtime guarantees, interoperability, and ecosystem required for large application development.

---

# Implementation Backlog

This assessment is split into scoped implementation tasks so each change can
prove its language contract, backend behavior, tests, and documentation:

- **Static types and application data:** tasks 110–114
- **Backend parity, cleanup, and FFI:** tasks 115–117
- **Application standard library:** tasks 118–122
- **Testing, packages, and editor tooling:** tasks 123–125
- **Bounded memory and explicit unsafe access:** task 126

The canonical status and dependency graph lives in `tasks/index.tsv`.
