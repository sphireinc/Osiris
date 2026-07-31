# FFI boundary v1 (design only)

No general FFI is implemented or enabled by default. This document specifies
the required contract for a future C ABI; Go interoperability must use the
same manifest/capability model rather than bypassing it.

## Manifest and loading

An FFI declaration is an explicit project-local manifest with `abi_version: 1`,
a logical library identity, exact SHA-256 digest, exported declaration names,
and requested capabilities. Dynamic library search paths, environment-driven
loader fallback, arbitrary symbol lookup, and network acquisition are denied.
The host config supplies an allowlisted absolute library path matching the
manifest digest. A future loader must reject mismatch before loading.

## Values and ownership

| Family | v1 status |
| --- | --- |
| null, bool, fixed-width signed/unsigned integers, f32/f64 | supported only with exact declared width |
| UTF-8 strings and byte buffers | supported with `(pointer,length)` and explicit borrowed/owned result convention |
| lists, maps, tensors, complex, decimal/bigint, units, errors, tasks, streams, modules, functions/closures | excluded from v1 direct ABI; adapt through a separately versioned owned buffer codec if needed |

Every result is declared `borrowed`, `owned_by_caller`, or `retained_by_host`.
Owned buffers require a manifest-declared matching release function. Failed
calls must not transfer ownership. No callback may retain a borrowed value.

## Errors, callbacks, threading, determinism

Calls return a status code plus an optional owned error buffer; status-to-
`OsirisError` mapping preserves a public message and source call-site while
redacting host debug data. Callbacks, re-entrant calls, background threads,
and long-lived pointers are excluded from v1. Calls are synchronous and only
allowed when the requested capability and deterministic-mode policy permit
them. Cancellation is cooperative: a future ABI must accept a cancellation
token or explicitly declare non-cancelable behavior.

## Capability and security policy

Capabilities are least-privilege (`ffi.math`, `ffi.crypto`, etc.), granted by
trusted host configuration—not source code. FFI is not available to modules
obtained from untrusted paths. A security review must approve every loading
model, manifest schema revision, and new value family before implementation.

## Example declaration (non-executable)

```json
{"abi_version":1,"library":"example-math","sha256":"…","capabilities":["ffi.math"],"functions":[{"name":"add_f64","args":["f64","f64"],"result":"f64","ownership":"borrowed"}]}
```
