# Task 06 — Move Standard-Library Policy and Algorithms into Osiris

## Objective

Restructure the standard library so Osiris dogfoods itself.

Use this architectural rule:

```text
Native/runtime primitives: C runtime ABI (and narrow compiler intrinsics)
Standard-library policy and algorithms: Osiris source
```

Do not keep implementing every ordinary library operation as a Go builtin or a native-runtime builtin.

## Principle

The runtime exists to expose capabilities that cannot be expressed efficiently/safely in ordinary Osiris without privileged access to representation, OS APIs or architecture-specific instructions.

The standard library should contain ordinary reusable language-level behavior.

Example split:

Runtime primitive:

```text
string byte length
allocate string backing storage
read file descriptor
monotonic clock
socket send/recv
hash primitive
```

Osiris stdlib:

```text
string trim/split/join policy
list transforms
sorting/search algorithms
higher-level filesystem helpers
HTTP request composition
retry policy
formatting helpers
collection adapters
```

## Create a real stdlib source tree

Introduce a dedicated Osiris-source standard library, e.g.:

```text
stdlib/
  core/
  text/
  collections/
  math/
  io/
  fs/
  net/
  task/
  time/
  encoding/
  ...
```

The compiler/toolchain must know how to locate/version these modules without treating them as magical parser constructs.

## Inventory existing builtins

Search `internal/osiris/runtime.go`, `extended_*.go`, filesystem/network/database/crypto/time modules, native runtime builtin tables and docs.

Produce a machine-readable inventory with fields:

```text
name
module
current implementation
requires privileged/runtime access? yes/no
allocation/representation dependency
platform dependency
determinism/effect class
migration target: runtime | stdlib | compiler intrinsic
```

Do not migrate blindly.

## Classification rules

Keep in the runtime only when at least one is true:

- needs raw object representation;
- needs allocation/lifetime primitive unavailable to source;
- performs OS syscall/platform API;
- requires FFI/unsafe operation;
- requires architecture-specific accelerated primitive;
- must participate directly in scheduler/runtime unwinding;
- cannot be implemented without circular dependency on the stdlib itself.

Move to Osiris when it is ordinary algorithm/policy over public language values.

Compiler intrinsics must be exceptionally rare and documented. Do not turn the compiler into a second standard library.

## Bootstrap layering

Avoid circular dependencies by define layers:

```text
Core runtime ABI
  ↓
core/prelude Osiris modules
  ↓
collections/text/math
  ↓
io/fs/net/etc.
  ↓
higher-level libraries
```

Core/prelude modules must not import higher layers.

Create an import-cycle checker if one does not already reliably exist.

## Native and VM loading

All execution modes should run the same Osiris stdlib sources or compiled artifacts.

- VM may compile/cache stdlib bytecode.
- Native compiler should compile/link stdlib modules normally or ship precompiled archives.
- Interpreter can parse/compile them through the same front end.

Do not maintain separate Go-native and C-native implementations of an algorithm solely for each backend.

## Performance

Moving code to Osiris is only acceptable if the compiler can optimize it adequately.

For hot primitives, expose minimal runtime operations and write loops/policy in Osiris. Use benchmarks to decide whether a routine must remain native.

Do not prematurely move cryptographic primitives or specialized vector kernels into source if doing so destroys constant-time/security/performance properties. High-level crypto composition may still live in Osiris while primitives remain provider-backed.

## Public API stability

Preserve existing `std/...` API names where sensible. The implementation location changing must not force user source changes unless the API itself is being deliberately corrected.

Add deprecation adapters where necessary, with a removal milestone.

## Tests

- stdlib unit tests written in Osiris;
- same tests on interpreter/VM/native;
- bootstrap/import-cycle tests;
- test that stdlib does not depend on compiler-internal packages;
- performance tests for migrated hot routines;
- platform-provider substitution tests;
- docs examples executed as tests.

## Acceptance criteria

- A substantial majority of non-privileged standard-library algorithms are Osiris source.
- Runtime builtin tables shrink to genuine primitives.
- Each public API has one semantic implementation rather than Go + C duplicates.
- Osiris can compile/run meaningful portions of its own stdlib across supported backends.
- Standard library layering and bootstrap dependencies are explicit and documented.
