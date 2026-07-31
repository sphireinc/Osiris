# Task 117: Implement the capability-gated C FFI

**Priority:** P1
**Dependencies:** 036, 041, 046, 047, 109, 116
**Status:** Blocked

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

- [ ] The first FFI slice follows the approved capability and ownership design.
- [ ] Interpreter/VM/native behavior and errors are parity-tested.
- [ ] Native dependencies are reproducible, packageable, and security-checked.
- [ ] Callback/handle cleanup is deterministic under failure and cancellation.
- [ ] Bounded buffers, typed views, and opaque handles preserve type, bounds,
      and ownership across every supported execution path.
- [ ] Full validation passes on maintained platforms.

## Non-goals

- No arbitrary Go ABI calls, C++ name mangling/object ABI, unverified library
  search paths, implicit pointer lifetime, integer-address conversion, or raw
  pointer arithmetic.
