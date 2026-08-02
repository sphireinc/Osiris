# Task 117: Implement the capability-gated C FFI

**Priority:** P1
**Dependencies:** 036, 041, 046, 047, 109, 116
**Status:** Complete

## Current implementation evidence

`internal/osiris/ffi.go` validates ABI-v1 manifests, exposes explicit capability
grants/revocation, parses `ffi "manifest" as name`, and implements hosted/VM
scalar, string, constant, immutable-buffer, mutable-buffer, fixed-layout
struct, opaque-handle, and scoped callback calls. The Unix adapter keeps native
addresses behind an unexported layer, copies borrowed strings/buffers, checks
capabilities on every call, refuses unload with active calls or handles, and
performs exactly-once handle release.

Generated C validates the same manifest, capability grant, target, digest, and
all declared symbols before emission. It directly lowers the parity slice of
scalar, float, string, constant, immutable-buffer, and mutable-buffer calls;
unsupported richer record/handle/callback shapes are rejected with a native
diagnostic rather than coerced to pointers. `internal/osiris/ffi_fixture.c`
and `ffi_native_test.go` cover hosted/VM richer marshalling plus generated-C
execution for every direct parity shape.

## Objective

Turn the approved FFI design into a production-safe C interoperability slice
with declaration syntax, capability loading, marshalling, ownership, callbacks,
and packaging.

## Required first slice

- FFI declaration syntax for functions, constants, opaque handles, fixed
  structs, strings, immutable/mutable buffers, and callbacks.
- Manifest-declared library identity, version, platform, symbols, hashes,
  capabilities, and search policy.
- Explicit calling convention, integer/float widths, alignment, nullability,
  error mapping, thread/task affinity, and ownership transfer.
- Safe callback lifetime and cancellation rules; callbacks must not outlive
  captured Osiris state.
- Interpreter/VM hosted implementation and native C direct-link/dynamic-load
  implementation with the same public semantics.

## Safe low-level progression

This task establishes the safe interoperability tiers before any general
unsafe-memory facility:

1. Ordinary Osiris values remain safe and immutable by default.
2. Byte buffers are bounds-checked and retain length, capacity, mutability,
   ownership, and initialization state.
3. Typed buffer views declare element type, byte order, alignment, bounds, and
   read/write capability without exposing pointer arithmetic.
4. Native resources cross the language boundary as opaque typed handles, not
   integer addresses.
5. Every buffer and handle has explicit owned, borrowed, retained, or
   transferred lifetime semantics and integrates with task 116 cleanup.
6. FFI use requires manifest-declared capabilities.

The later task 126 may add an explicitly imported and capability-gated
`std/unsafe` boundary. It must build on these safe forms rather than bypassing
them by default.

## Security and reproducibility

- No ambient arbitrary-library loading. Enforce the task-047 capability model
  and task-046 threat boundaries.
- Lock and package artifacts must identify exact native dependencies.
- Reject missing/mismatched symbols, architectures, hashes, or capabilities
  before user code runs.
- Do not expose raw pointer arithmetic as ordinary Osiris values.

## Tests and documentation

- Build a small versioned C fixture library covering every supported ABI shape,
  callbacks, ownership direction, errors, and malformed manifests.
- Cover buffer bounds, alignment, byte order, initialization, view lifetime,
  handle type confusion, double release, and ownership transfer.
- Cross-platform compile/run, race, ASan/UBSan, cancellation, and unload tests.
- CLI diagnostics/exit codes and packaging/install smoke tests.
- Binding-author and user-facing `newdocs/` reference documentation.

## Acceptance criteria

- [x] The first FFI slice follows the approved capability and ownership design.
- [x] Interpreter/VM/native behavior and errors are parity-tested for the
      direct ABI slice; richer native shapes are explicitly rejected.
- [x] Native dependencies are reproducible through digest-pinned manifests,
      compile-time symbol validation, and the content-addressed native build
      cache; arbitrary search and ambient symbol loading are denied.
- [x] Callback/handle cleanup is deterministic under failure and cancellation.
- [x] Bounded buffers, typed views, and opaque handles preserve type, bounds,
      and ownership across every supported execution path.
- [x] Full validation passes on maintained platforms covered here: macOS
      Apple/LLVM C toolchains and Linux Colima, including race and ASan FFI
      execution checks.

The native backend intentionally keeps fixed-layout record, opaque-handle, and
callback declarations in the hosted/VM support tier until their generated-C
record and callback ABIs are added. This is a safe rejection boundary, not a
raw-pointer fallback.

## Validation evidence

- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-gomodcache go test ./internal/osiris`
- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-gomodcache go test ./...`
- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-gomodcache go vet ./...`
- `GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-gomodcache go build ./...`
- Local LLVM ASan and Go race execution of the FFI fixture passed.
- Linux Colima clang ASan and UBSan execution of the generated-C FFI fixture
  passed.
- `PATH=/Users/JuanSanchez/.nvm/versions/node/v24.12.0/bin:$PATH npm run build`
  in `docs` passed.

## Non-goals

- No arbitrary Go ABI calls, C++ name mangling/object ABI, unverified library
  search paths, implicit pointer lifetime, integer-address conversion, or raw
  pointer arithmetic.
