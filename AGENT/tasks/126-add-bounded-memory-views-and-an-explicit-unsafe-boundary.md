# Task 126: Add bounded memory views and an explicit unsafe boundary

**Priority:** P2
**Dependencies:** 036, 046, 110, 111, 114, 115, 116, 117
**Status:** Blocked

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

- [ ] Ordinary Osiris code remains memory-safe and receives no implicit raw
      memory access.
- [ ] Safe buffers, typed views, handles, and ownership cover the documented
      common interoperability cases.
- [ ] Every unsafe primitive is explicit, capability-gated, auditable, and
      justified by a use case the safe forms cannot cover.
- [ ] Alias, lifetime, task, optimizer, cleanup, and backend contracts are
      documented and proven by focused, race, and sanitizer tests.
- [ ] Full validation and documentation checks pass.

## Non-goals

- No ambient raw pointers, C++ object ABI, arbitrary Go ABI access, stable
  pointer-address serialization, or claim that unsafe behavior is portable
  when the underlying platform contract is not.
