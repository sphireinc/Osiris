# FFI boundary v1

Osiris FFI is an explicit, manifest-backed boundary. A program names a
manifest, the host grants its declared capability, and the runtime exposes
only the functions, constants, and value shapes declared by that manifest:

```osiris
ffi "/absolute/project/math.ffi.json" as math
let total = ffi_call(math, "add_i64", 20i64, 22i64)
emit total
ffi_close(math)
```

The manifest is validated before the library is opened. Hosted execution and
the VM use the Go adapter; generated C validates the same manifest, resolves
every declared symbol at compile time, and emits a direct declaration-driven
adapter. Neither path accepts an integer address, an arbitrary symbol, a
search-path fallback, or a raw pointer value.

## Manifest and capabilities

An ABI-v1 manifest declares an absolute library path, version, target platform
and architecture, lowercase SHA-256 digest, capability names, absolute search
policy, functions, constants, structs, handles, and callbacks. Each function
also declares argument/result types, calling convention, nullability, thread
affinity, and ownership.

The trusted host grants capabilities before compilation or execution:

```go
runtime := osiris.NewRuntime()
runtime.GrantCapability("ffi.math")
```

The language cannot grant itself a capability. Revocation is checked before
each hosted/VM call. Native compilation requires the capability grant and
re-validates the manifest digest, target, and declared symbols before emitting
C. Native executables embed the manifest’s resolved library path and declared
symbols; they do not load an ambient library name.

Unknown manifest fields, missing or mismatched hashes, unsupported targets,
non-absolute paths, undeclared symbols, duplicate declarations, missing grants,
and invalid ABI metadata are rejected before user code runs.

## Supported value families

| Family | Boundary contract |
| --- | --- |
| `bool`, fixed-width signed/unsigned integers, `f32`, `f64` | Homogeneous scalar calls, up to four arguments, with declared result width. |
| NUL-free `string` | Arguments are copied to temporary NUL-terminated storage. Borrowed C results are copied immediately into Osiris-owned strings. |
| `const_buffer` | One or two bounded byte lists are copied into temporary storage and passed as `(pointer, length)`. |
| `buffer` | One bounded mutable byte list is copied in, passed as `(pointer, length)`, and copied back only after a successful call. Its length cannot change. |
| `struct:Name` | Hosted/VM fixed-layout records are encoded using the manifest’s field offsets, size, alignment, and byte order. Generated C rejects a struct declaration that has no native record lowering. |
| `handle:Name` | Hosted/VM opaque values carry only a type identity and host-owned foreign value. The declared release symbol runs at most once. Library close is refused while a handle remains active. |
| `callback:Name` | Hosted/VM supports the scoped synchronous `i64 -> i64` callback ABI. Registration ends immediately after the foreign call and cancellation invalidates the callback state. |
| constants | `ffi_const` returns the manifest’s statically validated value without a native call. |

Lists, maps, tensors, complex values, decimal/bigint, units, errors, tasks,
streams, modules, closures as ordinary return values, variadic functions, C++
ABIs, and retained foreign pointers are outside ABI-v1. They must be adapted by
a separately declared, bounded ABI rather than coerced through a pointer.

## Ownership, cancellation, and cleanup

Every function result declares `borrowed`, `owned`, `transferred`, or
`retained`. The current direct string and buffer adapters accept borrowed input
and copy results. Opaque handles are owned tokens with deterministic,
idempotent close behavior. A library cannot be unloaded while calls or handles
are active; close the handle before its library. Task-bound calls stay
synchronous on the current task. Cancellation is checked before and after a
call, but cannot interrupt a foreign function unless a future manifest ABI
declares a cancellation operation. Callback state is scoped to the call and
never outlives captured Osiris state.

Foreign code never receives an Osiris or Go pointer. Temporary buffers and
strings are released at the adapter boundary. Mutable buffers preserve bounds
and length, and post-call copy-back is rejected if the shape is no longer
valid.

## Example manifest

```json
{
  "abi_version": 1,
  "library": "/opt/osiris/libmath.dylib",
  "version": "1.0.0",
  "platform": "darwin",
  "architecture": "arm64",
  "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
  "capabilities": ["ffi.math"],
  "search_policy": "absolute",
  "functions": [
    {"name":"add_i64","symbol":"math_add_i64","args":["i64","i64"],"result":"i64","ownership":"borrowed","calling_convention":"c","thread_affinity":"any"}
  ],
  "constants": [{"name":"version_major","type":"u32","value":"1"}]
}
```

The C fixture in `internal/osiris/testdata/ffi_fixture.c` exercises scalar,
float, string, immutable/mutable buffer, fixed-record, opaque-handle, and
callback declarations. `internal/osiris/ffi_native_test.go` covers hosted and
generated-C execution for the scalar/string/constant/buffer parity slice and
the hosted-only richer value families.
