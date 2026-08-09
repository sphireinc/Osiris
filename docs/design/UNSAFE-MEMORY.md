# `std/unsafe` bounded memory contract

The language provides an explicit, capability-gated low-level memory boundary. It is
called `std/unsafe` because it is intended for interoperability and systems
code, but its first implementation deliberately exposes no raw pointer or
address operation. A program must import the module explicitly and its host
must grant the exact `unsafe.memory` capability. A normal runtime has no
ambient authority to import it.

## Threat model and design gate

The facility addresses one concrete gap that safe Osiris values and the task
117 FFI declaration cannot cover: a foreign or device-facing byte region may
need to be inspected as a fixed-width, endian-aware record without copying it
into a general list for every field access. The safe FFI surface remains the
preferred option for declared C functions, structures, and opaque handles.

The boundary assumes that an explicitly authorized host may provide a memory
backing region in a future platform adapter. It does not trust language input,
foreign metadata, or a stale handle. Every operation checks the handle kind,
closed state, integer conversion, byte range, element size, alignment, byte
order, mutability, and lifetime before touching bytes. The current portable
implementation owns a Go byte slice; it provides the same contract without
claiming that a slice is a portable foreign address.

The capability is not transitive. `ffi.fixture`, `os.filesystem`, and other
host grants do not authorize `unsafe.memory`. Host launch/package metadata
must list `unsafe.memory`, and the host applies that list through
`Runtime.GrantCapability`. Osiris source cannot grant, revoke, or widen it.
`UnsafeMemoryCapabilityManifest` is the inventory entry used by host tooling.

## Handles, ownership, and lifetime

The public values are opaque `handle:unsafe_memory_buffer` and
`handle:unsafe_memory_view` values. They contain no integer address, pointer,
or stable identity that can be serialized, hashed, ordered, emitted as a
number, or used in arithmetic. JSON/config conversion rejects them. Diagnostic
text may identify only the handle class and closed state.

`buffer(size)` allocates zero-initialized, writable storage. `from_bytes(data)`
makes an owned copy of a byte-compatible list. Each allocation is bounded by
`max_bytes` (64 MiB). A view retains the backing allocation, so releasing the
parent does not invalidate a live child view. Every handle owns one reference;
`release(handle)` is idempotent and returns `true` only for the first release.
When the final reference is released, the backing bytes are detached and all
subsequent operations fail with a lifetime error.

Operations hold the handle lock and a backing read/write lock for their full
critical section. Concurrent reads, writes, copies, and release therefore
cannot observe partially detached storage. A handle value may be transferred
between tasks because the backing state is synchronized; no borrowed view may
outlive its retained allocation.

## Views and checks

`view(handle, offset, length, element_size, alignment, byte_order, writable)`
creates a checked view. Offsets and lengths are bytes relative to the parent.
`element_size` is 1, 2, 4, or 8; length must be an element multiple;
alignment is 1, 2, 4, or 8 and must divide the offset and length. Byte order
is exactly `little`/`le` or `big`/`be`. A writable view cannot be created from
an immutable view, and every write checks the view's writable flag.

`slice(view, offset, length)` preserves the element, alignment, byte-order,
and mutability contract and requires compatible boundaries. `len` reports
bytes, `elements` reports complete elements, `metadata` returns only those
descriptive properties, and `bytes` returns a defensive copy.

The typed operations use element indexes, never byte addresses:

- `read_u8`, `read_u16`, `read_u32`, `read_u64`
- `write_u8`, `write_u16`, `write_u32`, `write_u64`
- `copy(destination, destination_offset, source, source_offset, length)`

Every index-to-byte multiplication is performed only after non-negative and
bounded integer conversion. `copy` snapshots its source before locking the
destination, so overlapping same-handle copies are deterministic and cannot
deadlock. It still requires a writable destination and rejects either range
that exceeds its view.

## Guarantees that remain mandatory

Unsafe code relaxes only the promise that the data came from ordinary Osiris
values. It does not relax source-spanned diagnostics, capability checks,
bounded allocation, checked bounds, checked alignment, initialization, handle
type identity, ownership cleanup, cancellation/cleanup integration, or
deterministic error behavior. No raw address conversion, arbitrary pointer
arithmetic, unbounded allocation, ambient authority, or implicit FFI alias is
part of this version.

The interpreter and VM share the same module and handle implementation. Native
C and WASM do not silently emulate or execute this host capability: until a
backend has an equivalent bounded ABI, compilation reports a source-spanned
unsupported diagnostic. Optimizers must treat all future foreign-memory
operations as effectful reads/writes and may not remove, duplicate, or reorder
them across an effect barrier.

## API reference

| Member | Signature | Result and checks |
| --- | --- | --- |
| `version` | `version` | Module contract version. |
| `capability` | `capability` | Required exact capability string, `unsafe.memory`. |
| `max_bytes` | `max_bytes` | Maximum allocation size, 64 MiB. |
| `buffer` | `buffer(size)` | Zero-initialized writable buffer handle. |
| `from_bytes` | `from_bytes(bytes)` | Owned copy of a byte-compatible list. |
| `view` | `view(handle, offset, length, element_size, alignment, byte_order, writable)` | Retained typed view handle. |
| `slice` | `slice(view, offset, length)` | Retained compatible sub-view. |
| `len` | `len(handle)` | Byte length as `u64`. |
| `elements` | `elements(handle)` | Complete element count as `u64`. |
| `bytes` | `bytes(handle)` | Defensive byte-list copy. |
| `metadata` | `metadata(handle)` | Bounded metadata map; never an address. |
| `read_u8`/`read_u16`/`read_u32`/`read_u64` | `read_uN(view, index)` | Checked unsigned element read. |
| `write_u8`/`write_u16`/`write_u32`/`write_u64` | `write_uN(view, index, value)` | Checked mutable write; returns `true`. |
| `copy` | `copy(dst, dst_offset, src, src_offset, length)` | Checked byte copy; returns count. |
| `release` | `release(handle)` | Idempotent ownership release; returns transition bool. |

Example:

```osiris
import "std/unsafe" as memory

let buffer = memory.buffer(8)
let words = memory.view(buffer, 0, 8, 2, 2, "little", true)
memory.write_u16(words, 1, 0x1234u16)
emit memory.read_u16(words, 1)
defer { memory.release(words) }
defer { memory.release(buffer) }
```

The example requires a host launch manifest containing `unsafe.memory`; it is
expected to fail before import when that grant is absent.
