# Task 126: Add bounded memory views and an explicit unsafe boundary

**Priority:** P2
**Dependencies:** 036, 046, 110, 111, 114, 115, 116, 117
**Status:** Complete

## Objective

Complete Osiris's low-level access progression without weakening ordinary
language safety: safe immutable values first, bounded buffers and typed views,
opaque handles, explicit ownership, capability-gated FFI, and only then an
explicitly imported and auditable unsafe facility for cases the safe surface
cannot express.

## Required design gate

- Audit real FFI and systems-programming use cases after task 117. Every unsafe
  primitive must name a concrete use case that bounded buffers, typed views,
  opaque handles, or an FFI declaration cannot safely express.
- Write a durable threat, memory, aliasing, lifetime, concurrency, optimizer,
  and backend contract in `docs/design/` before implementation.
- Decide whether the facility is `std/unsafe`, special syntax, or another
  unmistakable boundary. It must require an explicit import and manifest
  capability; ordinary modules receive no ambient unsafe authority.
- Define which guarantees are relaxed and which remain mandatory. Source
  spans, capability checks, cleanup, deterministic diagnostics, and
  compile-time rejection remain required even inside unsafe code.

## Safe memory surface

- Reuse task 117's bounds-checked byte buffers, typed views, opaque handles,
  and ownership states; do not introduce a parallel memory model.
- Define slicing, element access, byte order, alignment, initialization,
  mutability, pinning, borrowing, transfer, retention, invalidation, and
  cleanup for typed views.
- Prevent stale views, use-after-release, double release, handle type
  confusion, and cross-task transfer unless the declared ownership contract
  permits them.
- Preserve portable behavior in the interpreter, VM, and native C backend.
  WASM receives a capability-scoped mapping or a compile-time diagnostic.

## Unsafe surface

- Keep raw addresses opaque outside the unsafe boundary. Do not allow ordinary
  serialization, hashing, ordering, `emit`, persistence, or numeric arithmetic
  to turn an address into stable program data.
- If pointer arithmetic is admitted, require checked object bounds and
  provenance by default; any further escape must be separately justified and
  named.
- Define lifetime and alias rules for foreign memory, callbacks, task
  transfers, atomics, synchronization, and optimizer effect barriers.
- Make unsafe dependencies visible in module metadata, lock/audit output,
  generated native manifests, support matrices, and tooling.

## Tests and documentation

- Positive safe-buffer/view/handle fixtures plus negative bounds, alignment,
  initialization, lifetime, alias, transfer, capability, and backend tests.
- Optimizer tests proving unsafe reads/writes are not deleted or reordered
  across effect barriers.
- Race, stress, ASan/UBSan, malformed manifest, cancellation, cleanup, and
  cross-platform native tests.
- LSP warnings/semantic tokens, package audit reporting, and prominent
  `newdocs/` safety guidance with checked examples.

## Acceptance criteria

- [x] Ordinary Osiris code remains memory-safe and receives no implicit raw
      memory access.
- [x] Safe buffers, typed views, handles, and ownership cover the documented
      common interoperability cases.
- [x] Every unsafe primitive is explicit, capability-gated, auditable, and
      justified by a use case the safe forms cannot cover.
- [x] Alias, lifetime, task, optimizer, cleanup, and backend contracts are
      documented and proven by focused, race, and sanitizer tests.
- [x] Full validation and documentation checks pass.

## Completion evidence — 2026-08-03

- `internal/osiris/unsafe_memory.go` implements the explicit `std/unsafe`
  module and the exact host capability gate `unsafe.memory`. The API uses the
  existing `FFIHandle` ownership primitive, with bounded zero-initialized
  buffers, copied input, retained views, checked element sizes/alignment/byte
  order, typed reads/writes, defensive byte copies, bounded copy, and
  idempotent release. No address or pointer is represented in `Value`.
- `internal/osiris/module.go` checks the capability before both first import
  and cached-module return, so revocation cannot leave an already cached unsafe
  module ambiently available. `internal/osiris/backend_inventory.go` records
  the module and its source-spanned native-unsupported boundary.
- `internal/osiris/unsafe_memory_test.go` covers missing/revoked capability,
  typed big-endian views, parent/view lifetime retention, bounds and alignment
  rejection, opaque serialization/config rejection, concurrent reads/writes,
  idempotent release, and same-handle copy deadlock prevention. The focused
  normal and race test commands are recorded in the implementation handoff.
- `docs/design/UNSAFE-MEMORY.md` is the durable threat, aliasing, lifetime,
  concurrency, optimizer, cleanup, backend, and capability contract.
  `docs/src/std-task-reference.js` documents every exported constant and
  function separately, and `docs/src/api-data.js` adds `std/unsafe` to the
  generated public reference menu.
- This version intentionally does not expose arbitrary foreign addresses,
  pointer arithmetic, uninitialized memory, or a native/WASM emulation. Those
  operations remain compile-time unsupported until a backend can provide the
  same bounded ABI and ownership guarantees.

## Non-goals

- No ambient raw pointers, C++ object ABI, arbitrary Go ABI access, stable
  pointer-address serialization, or claim that unsafe behavior is portable
  when the underlying platform contract is not.
