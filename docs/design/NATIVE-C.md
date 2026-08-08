# Native C backend contract

## Host capability contract

`CCompileOptions.NativeCapabilities` is the only native-build capability grant.
The list is normalized (empty names removed, duplicates removed, and sorted),
included in native executable cache identity, and used when validating
capability-gated FFI manifests. The compiler does not infer grants from the
build host.

The native C surface lowers a capability-gated POSIX subset of `std/os` and
`std/net`. The direct path helpers and bounded `read_file`/`write_file` calls
retain their owned 1 MiB byte contract. The integrated resource ABI additionally
covers `open`, `create`, `open_file`, `close`, `read`, `write`, `read_at`,
`write_at`, `seek_file`, `sync`, deadlines, `stat`, `lstat`, `read_dir`,
`open_root`, `root_open`, `root_close`, process handles and their streams,
TCP/UDP/DNS sockets, plain HTTP, provider-backed TLS, and snapshot watch
streams. Every admitted member has an explicit capability and direct C
runtime symbol; missing grants produce source-spanned compile diagnostics.

`std/os.getenv` is also lowered for the explicit `os.environment` capability.
It copies the requested key, never returns a borrowed C environment pointer,
and produces the generated Option ABI (`some(string)` or `none()`).

`std/os.env_names` and `std/os.environ` copy and key-sort bounded environment
snapshots. `std/os.setenv` and `std/os.unsetenv` are separate
`os.environment_mutate` operations; they mutate only the generated process and
are never inferred from the host capability set.

Native POSIX filesystem mutation also covers `mkdir`, `mkdir_all`, `remove`,
`remove_all`, `truncate`, `rename`, `link`, `symlink`, `chmod`, `readlink`, and
`write_string`,
with `os.file_mutate`, `os.symlink`, or `os.file_write` grants as documented in
the host-resource ABI. Rooted file creation, read/write, metadata, directory
mutation, rename, chmod, and close use the same confined descriptor model.

`std/os.hostname`, `std/os.getpid`, `std/os.getppid`, `std/os.getuid`,
`std/os.geteuid`, `std/os.getgid`, `std/os.getegid`, and
`std/os.getpagesize` require `os.system_identity`. Native tests assert only
portable predicates (positive IDs, non-empty hostname, and positive page
size), because process identity is intentionally host-specific.

Environment and identity operations remain capability-gated and host-specific.
Adding a capability name does not silently enable an operation whose native
lowering is absent. Process-group cancellation, deterministic environment
overlays, producer-aware generic resource authorization, native HTTP redirects,
generated closure handlers, and Linux generated-C/ASan evidence are covered by
the completed 148B contract. The remaining explicit boundaries are unsupported
flag-derived file grants and callback-backed raw-handle methods, Windows
execution evidence, and TLS when the OpenSSL provider is not available to the
build.

The native backend lowers the typed Osiris IR to generated C. It never invokes
the interpreter or VM as a fallback. Every IR operation that is outside the
generated-C ABI fails during compilation with the source file and span that
introduced it.

## Result and Option ABI

Generated values use a tagged `OValue`. `O_RESULT` and `O_OPTION` each own a
single heap-allocated payload box. The `fbits` field is the presence bit:

- `ok(value)` stores `value` in an `O_RESULT` box with `fbits = 1`.
- `err(error)` stores the structured error in an `O_RESULT` box with
  `fbits = 0`.
- `some(value)` stores `value` in an `O_OPTION` box with `fbits = 1`.
- `none()` stores a null sentinel in an `O_OPTION` box with `fbits = 0`.

The shared `oresultbind` helper handles both Result and Option. Successful
values are returned from the box; failed Results preserve their error record,
and empty Options produce a `not_found` error. The same ABI supports
`unwrap`, `expect`, `or_else`, `to_option`, `to_result`, `collect`,
`issome`, and `isnone` for the parity-tested forms.

## Errors and ownership

`OError` carries the public kind, message, user message, operation, resource,
source, location, cause, stack, metadata, retryability, severity, and redaction
fields. Payload boxes and aggregate buffers are registered with the generated
runtime allocation tracker. Cleanup paths release or invalidate owned handles
before returning through normal, error, cancellation, and task edges.

### Capability-gated native FFI lifecycle

The direct native FFI adapter has one admission boundary and one ownership
boundary:

- `LoadFFILibrary` decodes a strict manifest, requires an absolute library
  path, verifies the SHA-256 digest before `dlopen`, checks the host-granted
  capabilities, and resolves every declared function and handle-release
  symbol before returning a live library.
- `FFILibrary.CallContext` checks the library state, declaration, current
  capability grants, argument shape, and context cancellation before entering
  the foreign call. A canceled context prevents entry. A synchronous foreign
  call that has already entered C cannot be interrupted by Go cancellation;
  callback adapters observe cancellation and return a neutral callback result.
- `FFILibrary.Close` is idempotent but will not unload a library while a call
  or an owned opaque handle remains live. Capability revocation blocks future
  calls and constants, while cleanup of already-owned handles remains allowed
  so revocation cannot strand native resources. Close marks the library as
  closing before leaving the Go-side mutex; concurrent callers wait for that
  one unload attempt, and a native close error leaves the library retryable.
- Opaque handles are borrowed under a mutex for the complete synchronous
  foreign call. Close cannot release a borrowed value, and a losing close
  caller waits for the one release attempt and observes the same release
  error. Library-owned release callbacks cannot be replaced by a caller;
  release is attempted exactly once and active-handle accounting is retired
  even when the foreign release function reports an error.
- Native symbol caches are protected independently from the library lifecycle.
  The `FFILibrary` call count prevents `dlclose` while a cached symbol is in
  use; after close, the library's native adapter is cleared and calls fail
  before reaching the adapter.

These rules deliberately make cancellation cooperative at the FFI boundary:
Osiris does not spawn an untracked goroutine to make an arbitrary C function
appear cancellable, because unloading its library while that function runs
would be unsafe.

## Tasks

The supported native task contract is deterministic and cooperative:

- `spawn` creates an owned deferred thunk;
- the first `await` evaluates it and later awaits reuse the terminal result;
- `cancel` transitions a pending task to a structured `cancelled` failure;
- `parallel` preserves source order and stops at the first terminal failure;
- `try`/`catch` receives the structured error produced by the task boundary.

Host-backed asynchronous scheduler work, cancellation of an already-running
external syscall, and APIs without a native ABI remain compile-time capability
boundaries. Synchronous native file, socket, process, TLS/HTTP, and watch
operations use owned resources and bounded polling; they do not invoke the
interpreter or VM as a fallback.

## Modules, patterns, and diagnostics

Standard-library imports are resolved as static module identities before C
emission. Member calls are selected from the explicit native module table;
dynamic module values and unsupported members produce source-spanned native
diagnostics. Enum, list, type, relational, logical, grouped, guarded, and
repeated-binding patterns lower to generated control-flow tests and bindings.

The maintained native parity tests include generated-source ABI markers,
interpreter/VM/native differential fixtures, malformed and unsupported
diagnostics, cleanup edges, task failure/cancellation, and undefined-behavior
sanitizer runs.

## Core and data parity witnesses (tasks 162 and 163)

The following native C symbols are part of the executable parity surface and
are documented individually here because they have different contracts:

- `emitCInstr` lowers the typed IR instruction that constructs and emits a
  value. The generated program owns the result payload and prints the complete
  Result value, including a payload such as `ok(7u64)` or `err(...)`.
- `oresultbind` implements the shared `Result`/`Option` extraction boundary.
  `ok` and `some` return their owned payload; `err` and `none` retain their
  structured failure semantics. It is used by `unwrap`, `expect`, `or_else`,
  `to_option`, `to_result`, `collect`, `issome`, and `isnone` in the admitted
  forms.
- `otensorsum` reduces every tensor element and returns a scalar whose numeric
  family and maximum floating-point width match the input tensor.
- `otensorproduct` performs the analogous product reduction and preserves the
  input floating-point width and complex/scalar family.
- `otensordot` computes the dot product for compatible one-dimensional tensor
  values. Its result width is the wider operand width, and complex operands
  produce a complex result.
- `otensornorm` computes the Euclidean norm for a numeric tensor and preserves
  the tensor's floating-point width in the returned scalar.

The focused differential witness
`TestTask162NativeCoreDataParityExpanded` in
`internal/osiris/task162_163_native_parity_test.go` executes the same core/data
fixture through the interpreter, VM, and generated C executable. It covers
payload-bearing Results, Option extraction, codec round-tripping, tensor
construction, reductions, dot product, and shape reporting.

## Native `std/list` reduction contracts (task 163)

`std/list.sum(values)` accepts one list of numeric or complex values and returns
the additive identity for an empty list (`0` at the default floating width).
`std/list.product(values)` accepts the same shape and returns the multiplicative
identity for an empty list (`1` at the default floating width). Both functions reject a non-list,
non-numeric element, or an incorrect argument count with a structured native
failure; neither calls back into the interpreter or VM.

The C implementation is split into these symbols:

- `nativeStaticModuleMember` admits `std/list.sum` and
  `std/list.product` during native compilation.
- `nativeModuleMemberStatus` and `BackendCapabilityInventory` expose the
  individual member status used by support audits.
- `ostd_list_fold` performs the shared numeric/complex fold and preserves the
  maximum floating-point width observed in its input list.
- `ostd_list_sum` and `ostd_list_product` provide the named runtime entry
  points used by the standard-library dispatcher.
- `ostd_dispatch` validates the argument count and routes the two members to
  their dedicated fold operation.

`TestTask163NativeListReductionParity` in
`internal/osiris/task162_163_native_parity_test.go` compares integer, `f64`,
and complex sum/product results across the interpreter, VM, and generated C
paths. The broader standard-library witnesses remain in
`internal/osiris/task145_native_parity_test.go`,
`internal/osiris/task146_native_parity_test.go`, and
`internal/osiris/task147_native_parity_test.go`.

### `std/list.flatten`

`std/list.flatten(values, depth)` accepts exactly two arguments: a list and a
non-negative integer depth in the inclusive range `0..64`. It returns a new
list containing the input elements with at most `depth` nested list layers
removed. At depth zero, nested lists are preserved as elements; an empty nested
list contributes no output once its containing layer is traversed. The input
list and the returned list are never mutated.

The native path is implemented by `nativeStaticModuleMember`,
`nativeModuleMemberStatus`, `ostd_list_flatten`,
`ostd_list_flatten_append`, and `ostd_dispatch`. The C helper bounds both the
top-level input and flattened output at 100,000 elements and bounds recursive
descent at 64 layers. Invalid argument counts, non-list input, negative or
non-integral depth, excessive depth, and limit violations produce a structured
native failure; the helper does not call the interpreter or VM.

`TestTask163NativeListFlattenParity` compares depth-one, depth-two, and
depth-zero behavior through the interpreter, VM, and generated-C executable.

### `std/list.partition`

`std/list.partition(values, index) -> tuple<list, list>` accepts exactly a
bounded list and a non-negative integer index. It returns a two-element tuple:
the first element contains the copied half-open range `[0,index)`, and the
second contains `[index,len(values))`. An index greater than the input length is
rejected under the established `std/list.slice` contract; the input is never
mutated. The two returned lists and the tuple wrapper are independently owned
native allocations.

The native symbols are `nativeStaticModuleMember`,
`nativeModuleMemberStatus`, `ostd_list_partition`, `olist`, and `ostd_dispatch`.
`ostd_list_partition` rejects non-lists, negative or non-integral indexes, and
indexes beyond the input length, and incorrect arity with the structured native message
`partition expects a list and non-negative index` or
`partition expects list and index`. It is pure and requires no host capability.

`TestTask163NativeListPartitionParity` and
`TestTask163NativeListPartitionOwnershipAndErrorContract` in
`internal/osiris/task162_163_native_parity_test.go` cover interpreter/VM/native
execution, the caught out-of-range error, tuple indexing, allocation symbols, capability absence,
and the invalid-input error contract.

## Native tuple construction and indexing (task 162 bounded slice)

This slice closes tuple expressions and is complemented by the bounded named
product slice below. The supported tuple operations are explicit:

- `irBuilder.evalExpr` recursively lowers each tuple item and emits one
  `IRInstr{Op: "tuple"}`. Item order and item evaluation order are preserved.
- `irBuilder.evalExpr` lowers `object[index]` to
  `IRInstr{Op: "index"}`. The operation evaluates the object and index once
  and retains the normal `at` bounds and non-negative-integer diagnostics.
- `emitCInstr` maps `tuple` to `otuple` and `index` to `oat`; it does not call
  the interpreter, VM, or a source evaluator.
- `oprint_tuple_value` recursively renders tuple aggregates with parentheses
  and list aggregates with brackets. `oprint_tuple_aware` preserves the normal
  generated-output spacing/newline contract while making direct tuple emits
  match the interpreter and VM.
- `otuple` copies the bounded item array through the existing allocation
  tracker and marks the aggregate with the tuple discriminator. Tuple storage
  is layout-compatible with the native aggregate array; the marker is kept
  for backend operations that need to distinguish tuple and list values.
- `atValue` and `lenValue` accept `kindTuple` with the same one-based contract
  used by the existing value model: indexing is zero-based, negative or
  out-of-range indices return an error, and length returns the number of
  elements.

`TestTask162NativeCoreDataParityExpanded` covers mixed-width numeric, string,
nested tuple construction, direct tuple rendering, constant indexing, and
tuple length through the interpreter, VM, and generated-C executable. Tuple
pattern matching remains a separate boundary until its ownership and generated
representation are implemented.

### Native named products (task 162 bounded slice)

The Native C backend now supports ordinary `struct` declarations, construction,
dot-member reads, and copy-on-write `with` updates for the statically checked
field subset. `Runtime.BuildIR` records declaration-order `IRProductType` and
`IRProductField` metadata. `irBuilder.lowerStmt` accepts product declarations;
`irBuilder.evalExpr` emits `product` and `product_update` instructions; and
`emitCInstr` lowers those instructions without invoking another evaluator.

The generated runtime uses tagged `O_PRODUCT` values with an owned type name
and field map. `oproduct` copies field values into tracked storage,
`oproductupdate` copies the original map before replacing named fields, and
`omember_any`/`omapget` resolve dot-member reads. Construction and updates are
copy-on-write: an existing product is never mutated and source expression
pointers are not retained.

`TestTask162NativeNamedProductParity` compares declaration, construction,
member reads, and updates through interpreter, VM, generated C, and the native
executable. `TestTask162NativeNamedProductDiagnostic` preserves source-spanned
rejection for invalid field values. Product rendering as a standalone emitted
value, bracket indexing, aliases, newtypes, and product pattern matching remain
explicit follow-up boundaries.

### Native tuple destructuring (task 162 bounded slice)

Tuple destructuring in a `let` binding is now lowered through the existing
tuple index ABI:

- `irBuilder.lowerStmt` evaluates the source expression once, creates a
  zero-based unsigned index constant for each binding, emits `index`, and then
  emits `copy` for the destination binding in source order.
- The generated C path therefore uses `oat`, the same helper used by direct
  tuple indexing. A non-tuple source, a non-integral index, or an out-of-range
  element returns the normal structured native error rather than silently
  binding a null value.
- Binding names are inserted into the IR environment only after their item is
  extracted, preserving left-to-right visibility and matching the interpreter
  and VM tuple-bind operations.

`TestTask162NativeTupleDestructuringParity` is the differential witness for
flat and nested tuple bindings across interpreter and VM, and checks the
generated-C artifact for the corresponding `oat` operations.

These witnesses do not close either task by themselves. Unverified product
rendering/indexing, wide numeric, dual/complex-width, provider-dependent host,
and cross-platform execution surfaces must remain non-stable until they have
executable evidence. Unsupported native members must continue to produce
source-spanned diagnostics rather than being admitted by inventory metadata
alone.

## Native `std/text` boundary contracts (task 163)

The following six members are a bounded, host-independent native C slice:

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/text.trim` | `trim(value: string) -> string` | none | New owned UTF-8 string with Unicode whitespace removed at both ends. |
| `std/text.trim_space` | `trim_space(value: string) -> string` | none | Same owned result contract as `trim`; the alias is independently admitted and tested. |
| `std/text.lower` | `lower(value: string) -> string` | none | New owned UTF-8 string using the native Unicode lower-case table; the input is not mutated. |
| `std/text.contains` | `contains(value: string, needle: string) -> bool` | none | Boolean search result; no borrowed result escapes the call. |
| `std/text.has_prefix` | `has_prefix(value: string, prefix: string) -> bool` | none | Boolean prefix predicate; both arguments remain caller-owned. |
| `std/text.has_suffix` | `has_suffix(value: string, suffix: string) -> bool` | none | Boolean suffix predicate; both arguments remain caller-owned. |

`ostd_dispatch` validates arity and runtime kinds before calling the helpers.
`ostd_text_trim` and `ostd_text_case` construct owned result strings through
`oownedstring`; predicates return scalar values and do not retain input
pointers. Invalid UTF-8 and wrong argument kinds produce structured native
errors. Since this slice performs no host I/O, it does not accept or infer an
OS capability. `TestTask163NativeTextBoundaryParity` compares the interpreter,
VM, generated C, and generated executable, while
`TestTask163NativeTextOwnershipAndCapabilityContract` checks the generated
ownership boundary and the backend inventory entry.

## Native `std/crypto` SHA-512 contracts (task 163 bounded slice)

This slice closes the pure, host-independent digest members for the standard
SHA-512 family. It does not enable HMAC, random bytes, key derivation, or any
other host/provider surface.

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/crypto.digest` | `digest(data: string \| list<u8>, algorithm: string) -> string` | none | Lowercase hexadecimal string, newly allocated and owned by the result. |
| `std/crypto.digest_bytes` | `digest_bytes(data: string \| list<u8>, algorithm: string) -> list<u8>` | none | Newly allocated raw digest bytes; 64 bytes for `sha512`, 32 bytes for `sha256` and `sha512_256`. |
| `std/crypto.algorithm_supported` | `algorithm_supported(name: string) -> bool` | none | Boolean result; no input pointer or host resource is retained. |

The accepted canonical algorithm names are `sha256`, `sha512`, and
`sha512_256`. The C implementation uses `OSHA256` for SHA-256 and the local
`OSHA512` implementation for SHA-512 and SHA-512/256. The latter uses the
FIPS 180-4 round constants, 128-byte block schedule, 128-bit length encoding,
and the SHA-512/256 initialization vectors; it does not call an OpenSSL,
CommonCrypto, or operating-system provider.

Input accepts UTF-8 strings or lists containing only `u8` values and is capped
at 1 MiB before a temporary byte conversion is allocated. A list input is
copied into a temporary buffer and released before the result is returned. A
hex result is constructed through `oownedstring`; a raw result is copied into
an owned Osiris list through `ostd_bytes_list`. No borrowed C pointer escapes
the dispatcher, and no capability macro is required.

Wrong arity or value kinds, invalid byte-list elements, inputs over 1 MiB, and
unknown algorithm names return a structured native failure. In particular,
`algorithm_supported` returns `false` for unknown names rather than attempting
provider discovery. `TestTask163NativeCryptoSHA512Parity` compares digest
strings, raw bytes, and the capability predicate across the interpreter, VM,
generated C, and generated executable. `TestTask163NativeCryptoSHA512OwnershipCapabilityAndErrorContract`
checks the generated ownership symbols, pure-capability boundary, error
messages, and individual backend-inventory entries.

## Linux generated-C evidence

Linux execution evidence is maintained separately from the macOS/POSIX test
run. `internal/osiris/task148b_linux_parity_test.go` compares deterministic
interpreter, VM, and generated-C results with fixed output contracts and runs
an AddressSanitizer-generated executable. It intentionally excludes host IDs,
clocks, environment state, network services, and host-dependent paths so the
same expected output is meaningful across Linux and macOS.

Run the evidence in a Linux kernel/userspace with:

```sh
scripts/test-native-linux.sh
```

The script uses `golang:1.26-bookworm` and `linux/arm64` by default, and can be
overridden with `OSIRIS_LINUX_IMAGE` and `OSIRIS_LINUX_PLATFORM`. It reports
the Go/compiler versions, runs deterministic generated-C parity, then runs
the same generated-C path with `OSIRIS_C_SANITIZERS=address`. A Docker daemon
that is unavailable or does not report a Linux engine is an environmental
blocker; no Linux execution claim is made from the macOS host-side skipped
tests. The 148B evidence was executed successfully in Colima with Go 1.26.5,
Debian GCC 12.2, deterministic interpreter/VM/native comparison, and the
generated-C AddressSanitizer fixture.
