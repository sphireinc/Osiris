# FFI boundary v1

The repository implements manifest validation, capability grants, a pinned
dynamic loader, bounded scalar calls, and idempotent opaque-handle lifetime
tokens in `internal/osiris/ffi.go`. The declaration form is:

```osiris
ffi "/absolute/project/math.ffi.json" as math
emit ffi_call(math, "add_i64", 20i64, 22i64)
ffi_close(math)
```

The host must grant the manifest capability before running the program:
`runtime.GrantCapability("ffi.math")`. Native C compilation rejects these
host-loader calls with a source-spanned diagnostic until a generated-C adapter
is configured; it never proxies through the interpreter.

## Manifest and loading

An FFI declaration is an explicit project-local manifest with `abi_version: 1`,
a logical library identity, exact SHA-256 digest, exported declaration names,
and requested capabilities. Dynamic library search paths, environment-driven
loader fallback, arbitrary symbol lookup, and network acquisition are denied.
The host config supplies an allowlisted absolute library path matching the
manifest digest. `LoadFFIManifest` rejects mismatch, unknown fields, platform
or architecture mismatches, missing capabilities, unsupported types, and
duplicate symbols before any library is opened.

## Values and ownership

| Family | v1 status |
| --- | --- |
| null, bool, fixed-width signed/unsigned integers, f32/f64 | supported only with exact declared width |
| UTF-8 strings and byte buffers | supported with `(pointer,length)` and explicit borrowed/owned result convention |
| lists, maps, tensors, complex, decimal/bigint, units, errors, tasks, streams, modules, functions/closures | excluded from v1 direct ABI; adapt through a separately versioned owned buffer codec if needed |

Every result is declared `borrowed`, `owned`, `transferred`, or `retained`.
`FFIHandle.Close` is idempotent and clears the opaque value before invoking a
release hook. Failed calls must not transfer ownership. No callback may retain
a borrowed value.

## Errors, callbacks, threading, determinism

The maintained direct adapter supports up to four homogeneous fixed-width
integer or floating-point arguments and scalar results. It resolves every
declared symbol before returning a library handle, uses `RTLD_NOW|RTLD_LOCAL`,
and unloads exactly once. Strings, buffers, opaque handles, constants, and
callbacks remain schema-validated but are rejected by the call adapter until
their dedicated marshalling/lifetime implementations land; they are not
silently coerced to pointers. Calls are synchronous and cancellation cannot
interrupt a foreign function that did not declare a cancellation ABI.

## Capability and security policy

Capabilities are least-privilege (`ffi.math`, `ffi.crypto`, etc.), granted by
trusted host configuration—not source code. FFI is not available to modules
obtained from untrusted paths. A security review must approve every loading
model, manifest schema revision, and new value family before implementation.

## Example declaration (non-executable)

```json
{"abi_version":1,"library":"example-math","sha256":"…","capabilities":["ffi.math"],"functions":[{"name":"add_f64","args":["f64","f64"],"result":"f64","ownership":"borrowed"}]}
```
