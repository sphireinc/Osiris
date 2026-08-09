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

## Native OSR1 codec for tagged values

`oencode` and `odecode` use the versioned `OSR1` value codec. The generated C
codec preserves the individual wire tags for `O_ERROR` (`0x0a`), `O_RESULT`
(`0x16`), and `O_OPTION` (`0x17`) instead of rejecting or flattening these
values.

| Function | Parameters | Result | Ownership and failure behavior |
| --- | --- | --- | --- |
| `encode(value)` | One value from the supported OSR1 tree | `string` containing the four-byte `OSR1` header followed by one tagged value | Traversal is limited to `OCODEC_DEPTH` (256) and `OCODEC_LIMIT` (64 MiB); Result/Option/Error payloads recurse through the same encoder; allocation or unsupported-value failure becomes a structured native error. The map key order is canonicalized lexicographically. |
| `decode(bytes)` | One OSR1 `string` or byte-compatible `list<u8>` | One decoded value, including `error`, `ok`, `err`, `some`, and `none` wrappers | Validates the header, exact tag, version, presence byte, length, recursion depth, stack/metadata records, and trailing bytes before publishing a result. Decoded strings, boxes, maps, stack records, and error strings are copied into tracked native storage; the input byte list may then be released. |

The `0x0a` Error body is version `1`, followed by the exact field sequence from
`encodeErrorRecord`: kind, code, message, user message, operation, resource,
retryability, severity, correlation ID, component, remediation, timestamp,
location, cause, stack, metadata, and debug metadata. Native errors encode
their stable code as the kind and use the canonical `error` severity when the
compact C ABI has no corresponding optional field; decoded optional wire
fields are still consumed and validated, while the fields represented by
`OError` remain available to generated code. `0x16` Result writes a presence
byte and then either one nested value or one tagged Error. `0x17` Option writes
a presence byte and writes a nested value only for `some`. No codec path invokes
the interpreter, VM, host provider, or capability dispatcher.

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

### Native scalar `complex<f32>` (bounded slice)

Explicit `complex<f32>` values use the same tagged scalar ABI as the default
and `complex<f64>` families. The component width is metadata on
`OValue.fbits`; the payload remains the inline `OComplex` pair, so this slice
introduces no heap allocation and no host capability.

- `complexValueStyled` rounds both components to IEEE binary32 before storing
  an explicit `complex<f32>` value. `complexComponentBits` and
  `complexResultBits` preserve that width through construction and binary
  promotion; mixed default-width operands still promote according to the
  shared scalar rule.
- `formatComplexStyled` renders both components with the `f32` suffix, and
  `Value.Kind` reports `complex<f32>`. `realValue` and `imagValue` retain the
  source complex component width for their scalar results.
- `cLiteral` lowers a width-tagged complex constant to
  `ocomplexbits(real, imag, 32)` (or the corresponding explicit width) rather
  than rejecting `complex<f32>`. `emitCInstr` therefore preserves the literal
  and lowering contract without a fallback evaluator.
- `ocomplexbits` stores the tagged width in generated C,
  `ocomplexcall` preserves the wider component width rule for constructed
  values, `obin` preserves it through `+`, `-`, `*`, and `/`, and `oreal`/
  `oimag` return width-tagged scalar components.
- `nativeCInstrError` carries the source span for a neighboring unsupported
  shape. Complex arguments to scalar-only native math such as `sqrt` are
  rejected at C lowering with `complex sqrt is not supported yet`; they are
  never silently evaluated by the interpreter or VM.

`TestNativeCoreComplexF32Parity` in
`internal/osiris/native_core_numeric_test.go` compares suffixed literals,
typed construction, arithmetic, rendering, and real/imaginary projections
through the interpreter, VM, generated C, and native executable.
`TestNativeCoreComplexF32LoweringContract` checks the IR width marker,
`ocomplexbits` lowering, no capability macro, and the allocation-free scalar
constructor boundary. `TestNativeCoreComplexUnsupportedShapeDiagnostic`
checks the source-spanned rejection contract for complex scalar math.

### Native scalar `dual` (bounded slice)

The Native C backend uses the language’s scalar dual contract directly:
`dual(primal, tangent)` stores two `double` components in the inline `ODual`
member of `OValue`. It is not a heap object, does not retain either input, and
does not grant or infer a host capability.

The individual symbols and contracts are:

- `emitCInstr` admits the `dual`, `primal`, and `tangent` builtins. It validates
  arity during lowering and reports malformed calls through
  `nativeCInstrError`, preserving the source file, line, and column.
- `odualcall` validates the two runtime arguments with `onum` and constructs
  the inline `ODual`; it returns a structured native failure for a non-numeric
  argument rather than calling an evaluator fallback.
- `odualparts` accepts either a dual or an ordinary numeric scalar, treating a
  scalar as zero tangent. This is the shared mixed-scalar boundary used by
  `odoubledualbin`, so `dual(3, 1) + 2` and `2 + dual(3, 1)` have the same
  derivative semantics as the interpreter and VM.
- `odoubledualbin` implements `+`, `-`, `*`, and `/` with the product and
  quotient rules. A zero primal denominator and an unknown operator are
  structured native failures.
- `ounary` negates both dual components for unary `-`; logical negation still
  uses the shared truthiness contract.
- `oprimal` and `otangent` return scalar values by value. `oprimal` extracts
  the primal component, while `otangent` accepts ordinary numeric scalars as
  zero-tangent values.
- `oeq` compares both components exactly, and `otruthy` follows the same
  non-zero primal-or-tangent rule as the reference runtime.
- `oprintvalue` renders a dual as `dual(primal, tangent)`, matching
  `Value.String()` and avoiding the previous non-language `primal+tangentε`
  spelling.

`TestNativeCoreDualScalarParity` compares construction, rendering, mixed
scalar and dual arithmetic, unary negation, and projections through the
interpreter, VM, generated C, and native executable. The dedicated
`TestNativeCoreDualScalarLoweringOwnershipAndCapabilityContract` checks the
inline helper regions for allocation, input retention, and capability macros.
`TestNativeCoreDualScalarArityDiagnostic` checks the source-spanned
malformed-call boundary, while `TestNativeCoreDualScalarRuntimeErrorContract`
checks matching division-by-zero behavior from the interpreter and generated
executable. Normal, race, and Homebrew LLVM AddressSanitizer runs cover all
four dedicated dual tests. The broader task remains open for other numeric
families, but this dual-scalar slice no longer belongs in the remaining dual
gap list.

### Native numeric constructors and projections

The capability ledger records the following individual direct Native C
lowerings. Each row describes one callable; the rows are intentionally kept
separate because their accepted value families, result representation, and
allocation behavior differ.

| Symbol | Accepted arguments | Result and direct helper | Boundary |
| --- | --- | --- | --- |
| `uint(value)` | One numeric value | Width-tagged unsigned scalar from `ouintcast` | Numeric conversion is checked by the generated helper; invalid kinds become structured native failures. |
| `byte(value)` | One numeric value | Byte-width unsigned scalar from `obytecast` | The conversion is performed by value and does not retain the input. |
| `bytes(value)` | One bytes-compatible value | Owned byte sequence from `obytes` | The generated helper owns the result storage; malformed input is a structured failure. |
| `complex(value)` | One complex-compatible value, or real and imaginary values | Inline `OComplex` from `ocomplexcall` | Components retain the language width metadata; no host capability is required. |
| `conjugate(value)` | Complex scalar/tensor or an admitted real numeric value | Conjugated value from `oconjugate` | Complex tensors receive newly owned data; scalar numeric values remain inline. Unsupported families fail explicitly. |
| `magnitude(value)` | Complex-compatible or admitted scalar numeric value | Magnitude from `omagnitude` | Scalar paths stay unboxed; complex results are checked by the generated helper and do not cross a host boundary. |
| `dual(primal, tangent)` | Two numeric values | Inline `ODual` from `odualcall` | The two components are copied by value and never retained. The full dual arithmetic and projection contract is documented above. |
| `primal(value)` | One numeric or dual value | Scalar projection from `oprimal` | Ordinary numeric values are returned unchanged; dual values return their primal component. |
| `tangent(value)` | One numeric or dual value | Scalar projection from `otangent` | Ordinary numeric values produce a zero tangent; malformed values produce a structured failure. |

`emitCInstr` validates arity before selecting these helpers. The scalar
specializer may emit an even narrower unboxed assignment for `conjugate` and
`magnitude` when it has proved the input kind; the inventory entry covers that
proven subset as well as the tagged helper path. No entry here claims arbitrary
precision, future numeric families, or dynamic closure execution.

### Native scalar reductions and element-wise operations

These scalar and collection operations already have direct helper selection in
the emitter. Their individual contracts are:

| Symbol | Parameters | Return value | Direct helper |
| --- | --- | --- | --- |
| `mean(values)` | One bounded numeric list/tensor | Numeric mean | `omean` |
| `std(values)` | One bounded numeric list/tensor | Numeric standard deviation | `ostd` |
| `var(values)` | One bounded numeric list/tensor | Numeric variance | `ovar` |
| `median(values)` | One bounded numeric list/tensor | Numeric median | `omedian` |
| `quantile(values, probability)` | Values and a probability | Numeric quantile | `oquantile` |
| `percentile(values, percentile)` | Values and a percentile | Numeric percentile | `opercentile` |
| `argmin(values)` | One bounded comparable collection | Index value | `oargmin` |
| `argmax(values)` | One bounded comparable collection | Index value | `oargmax` |
| `any(values)` | One collection | Boolean predicate result | `oany` |
| `all(values)` | One collection | Boolean predicate result | `oall` |
| `cumsum(values)` | One numeric list/tensor | Cumulative numeric collection | `ocumsum` |
| `cumprod(values)` | One numeric list/tensor | Cumulative numeric collection | `ocumprod` |
| `phase(value)` | One complex-compatible value | Floating phase angle | `ophase` |

The direct paths validate the documented arity and value family before calling
their helpers. Returned lists/tensors are newly represented by the generated
runtime; input storage is borrowed for the call and no host capability is
introduced. Unsupported dynamic or provider-specific forms remain native
compile diagnostics. The inventory entries are asserted by
`TestNativeCoreDirectLoweringInventory`, and the generated-main helper calls
are checked by `TestNativeCoreDirectLoweringContract` in
`internal/osiris/native_core_capability_ledger_test.go`.

### Native tensor constructors

| Symbol | Parameters | Result | Direct helper and boundary |
| --- | --- | --- | --- |
| `zeros(shape...)` | One shape list or one or more dimensions | Zero-filled tensor | `ozeros`; dimensions are checked before allocation. |
| `ones(shape...)` | One shape list or one or more dimensions | One-filled tensor | `oones`; the result owns its data. |
| `full(shape..., value)` | Shape plus a numeric fill value | Filled tensor | `ofull`; fill conversion and size limits are checked. |
| `identity(rows[, columns])` | One square size or row and column sizes | Identity matrix | `oidentity`; checked dimensions and owned matrix storage. |
| `eye(rows[, columns])` | One square size or row and column sizes | Identity matrix | Uses the same direct `oidentity` ABI and validation. |
| `arange(stop)` | Stop, or start/stop, or start/stop/step | Rank-one numeric tensor | `oarange`; zero steps and excessive output are structured failures. |
| `linspace(start, stop[, count])` | Floating bounds and optional count | Rank-one numeric tensor | `olinspace`; count and allocation limits are checked. |
| `logspace(start, stop, count[, base])` | Floating bounds, count, and optional base | Rank-one numeric tensor | `ologspace`; invalid bases and counts are structured failures. |
| `tensor(data[, shape])` | Nested data, optionally with an explicit shape | Tensor value | `otensorcall`; rectangularity, shape, and element-family checks remain explicit. |
| `matrix(data[, shape][, columns])` | Matrix data in one of the documented forms | Rank-two tensor | `omatrixcall`; the helper preserves the matrix shape contract. |

These constructors may allocate result storage because they create owned
aggregates; the ledger does not describe them as allocation-free. They do not
retain input pointers after construction and do not define a host capability.

### Native tensor indexing, transforms, and portable linear algebra

| Symbol | Parameters | Result | Direct helper |
| --- | --- | --- | --- |
| `at(value, index)` | Aggregate and scalar or tensor index | Selected value | `oat` |
| `setat(value, index, replacement)` | Aggregate, index, replacement | Updated aggregate/value | `osetat`; the original aggregate is not mutated through the borrowed input pointer. |
| `reshape(value, shape)` | Tensor and compatible shape | Re-shaped tensor | `oreshape` |
| `transpose(value)` | Rank-two tensor | Transposed tensor | `otranspose` |
| `matmul(left, right)` | Two compatible rank-two tensors | Matrix product | `omatmul` |
| `outer(left, right)` | Two numeric or complex vectors/tensors | Outer-product tensor | `oouter` |
| `trace(value)` | Rank-two tensor | Diagonal sum | `otrace` |
| `det(value)` | Square rank-two tensor | Determinant scalar | `odet` |
| `solve(matrix, right)` | Square matrix and vector/matrix right-hand side | Solution tensor | `osolve`; singularity and size are structured failures. |
| `inv(matrix)` | Non-empty square matrix | Inverse matrix | `oinv`, using the native solve boundary. |
| `cholesky(matrix)` | Positive-definite square matrix | Lower-triangular matrix | `ocholesky`; invalid definiteness is a structured failure. |
| `qr(matrix)` | Rank-two matrix | `[Q, R]` result | `oqr`; orthogonalization buffers are owned and released by the helper. |
| `shape(value)` | Tensor or shape-bearing aggregate | Dimension list | `oshape` |
| `flatten(value)` | Tensor/aggregate | Flattened aggregate | `oflatten` |
| `sum(value)` | Numeric tensor | Numeric scalar | `otensorsum` |
| `product(value)` | Numeric tensor | Numeric scalar | `otensorproduct` |
| `dot(left, right)` | Compatible one-dimensional tensors | Numeric scalar | `otensordot` |
| `norm(value)` | Numeric tensor | Numeric norm scalar | `otensornorm` |

The emitter rejects malformed arity and statically unsupported shapes rather
than routing them through the interpreter or VM. The portable operations above
are ledgered individually because each helper has a different shape and
ownership contract. `eig`, `eigh`, and `svd` remain outside this slice: their
emitter branches require configured numerical-provider ABIs and need separate
provider-backed execution evidence before they can be recorded as implemented.

### Native scalar extrema (bounded slice)

The Native C backend directly lowers these `std/math` members for the
documented numeric subset:

- `math.min(values...) -> number`: a non-empty variadic numeric fold;
- `math.max(values...) -> number`: a non-empty variadic numeric fold;
- `math.clamp(value, low, high) -> number`: a three-argument numeric clamp.

`min` and `max` use the generated `ominmax` helper and preserve the reference
numeric family for mixed integer/floating inputs. `clamp` uses `oclamp` and
requires the value, lower bound, and upper bound to be mutually compatible;
invalid arity or value families remain source-spanned native diagnostics.
Inputs are borrowed only for the duration of the call, no host capability is
defined, and the result is a newly represented value. The existing executable
parity coverage in `TestCBackendStaticMathExtremaParity`,
`TestCBackendUnboxedFloatClampParity`, and
`TestCBackendUnboxedFloatExtremaParity` is supplemented by
`TestNativeCoreMathExtremaInventoryAndLoweringContract`, which prevents the
backend capability ledger from under-reporting these direct lowerings.

### Native scalar math: `atan2`, `exp2`, and `trunc`

The Native C backend directly lowers the following pure real scalar members of
`std/math`:

| Symbol | Parameters | Return value | Native behavior and failure contract |
| --- | --- | --- | --- |
| `std/math.atan2(y, x)` | Two floating scalars of the same width: `f32`, `f64`, or the default `number` width | Same floating width | Computes the quadrant-aware angle with C `atan2`; `f32` inputs are evaluated in `double` and narrowed once, matching the interpreter's width boundary. |
| `std/math.exp2(x)` | One `f32`, `f64`, or default `number` scalar | Same floating width | Computes `2**x`. Finite overflow is checked in the mathematical result and again after `f32` narrowing; finite underflow, NaN, and infinite inputs retain the reference IEEE behavior. Overflow is a structured native failure. |
| `std/math.trunc(x)` | One `f32`, `f64`, or default `number` scalar | Same floating width | Removes the fractional part toward zero through C `trunc`, preserving the width and IEEE signed-zero/special-value behavior. |

The direct scalar path is deliberately narrow: both arguments to `atan2` must
have the same statically known floating kind, and all three operations reject
complex, mixed-width, integer-only, and dynamically unproven shapes during C
lowering with a native compile diagnostic. Qualified module calls and their
builtin spellings resolve to the same emitter symbols.

`emitCInstr` validates the operation identity and arity before selecting the
unboxed scalar path. `nativeScalarBuiltinResultKind` proves the result width,
`emitScalarBuiltin` emits the direct C operation, and `nativeBuiltinStatus` plus
`nativeModuleMemberStatus` expose the implemented inventory entries. The
generated expressions borrow their input scalar locals, allocate no storage,
retain no input pointers, define no `OSIRIS_CAP_*` host capability, and do not
cross `ostd_dispatch`, `ocall`, the interpreter, or the VM.

`TestNativeCoreMathScalarParity` compares builtin and qualified calls through
the interpreter, VM, generated C, and native executable. The companion
ownership/capability test checks direct C symbols, the inventory, the absence
of allocation and dispatch in the generated main, while the error and arity
tests verify structured `exp2` overflow and malformed-call diagnostics.

### Native scalar remainder math: `fmod`, `remainder`, and `fma`

The Native C backend directly lowers the following pure real scalar members of
`std/math`:

| Symbol | Parameters | Return value | Native behavior and failure contract |
| --- | --- | --- | --- |
| `std/math.fmod(dividend, divisor)` | Two floating scalars of the same width: `f32`, `f64`, or default `number`; `divisor` must not be zero | Same floating width | Checks the divisor, then calls C `fmod`. `f32` inputs are evaluated in double and narrowed once, matching `math.Mod` followed by the language result-width conversion. |
| `std/math.remainder(dividend, divisor)` | Two floating scalars of the same width: `f32`, `f64`, or default `number`; `divisor` must not be zero | Same floating width | Checks the divisor, then calls C `remainder`, whose nearest-even quotient and IEEE exceptional behavior match `math.Remainder`. `f32` results are narrowed once. |
| `std/math.fma(x, y, addend)` | Three floating scalars of the same width: `f32`, `f64`, or default `number` | Same floating width | Calls C `fma` for one fused rounding step. `f32` inputs are evaluated as already-rounded doubles and narrowed only after the fused result, matching `math.FMA` and the language result-width conversion. |

The direct scalar path is deliberately narrow: all arguments must have the
same statically known floating kind. Complex, mixed-width, integer-only, and
dynamically unproven shapes remain native compile diagnostics rather than
silently crossing into a generic evaluator or host-dependent fallback. A zero
`fmod` or `remainder` divisor, including signed zero, returns the structured
native error used by the reference runtime.

`emitCInstr` validates each operation identity and arity before selecting the
unboxed scalar path. `nativeScalarBuiltinResultKind` proves the result width,
and `emitScalarBuiltin` emits the direct C operation. `nativeBuiltinStatus` and
`nativeModuleMemberStatus` expose the three individual inventory entries. The
generated expressions borrow scalar locals, allocate no storage, retain no
input pointers, define no `OSIRIS_CAP_*` host capability, and do not cross
`ostd_dispatch`, `ocall`, the interpreter, or the VM.

`TestNativeCoreMathRemainderParity` compares builtin and qualified calls
through the interpreter, VM, generated C, and native executable.
`TestNativeCoreMathRemainderLoweringOwnershipAndCapabilityContract` checks
the direct C calls, inventory, ownership, allocation, dispatcher, and
capability boundaries. The zero-divisor contract test checks structured errors
on all three executable layers, and the arity test verifies source-level
malformed-call diagnostics.

### Native fixed-width integer math (bounded slice)

The Native C backend directly lowers the pure `std/math` integer reductions
`gcd` and `lcm` for fixed-width unsigned arguments. These are separate
contracts rather than one generic “math” category:

| Symbol | Parameters | Return value | Native behavior and failure contract |
| --- | --- | --- | --- |
| `std/math.gcd(left, right)` | `left: u8 \| u16 \| u32 \| u64`, `right: u8 \| u16 \| u32 \| u64` | `uN`, where `N` is the wider input width | Euclidean reduction over the two unsigned values. The result is non-negative and retains the wider input width. Zero arguments are valid; `gcd(0, 0)` returns zero. |
| `std/math.lcm(left, right)` | `left: u8 \| u16 \| u32 \| u64`, `right: u8 \| u16 \| u32 \| u64` | `uN`, where `N` is the wider input width | Computes `abs(left / gcd(left, right) * right)` without an intermediate multiplication overflow. If either input is zero, it returns zero. If the result is not representable in `uN`, the generated program returns a structured native failure instead of wrapping. |

The exact generated-C symbols are:

- `emitCInstr` recognizes the `gcd` and `lcm` builtin identities produced by
  static `std/math` member resolution, validates the two-argument shape, and
  emits a direct helper call. The call does not use `ostd_dispatch`, `ocall`,
  the interpreter, or the VM.
- `ostd_math_unsigned` checks the tagged `OValue` kind and width metadata. It
  accepts only `O_UINT` values with an explicit width from 1 through 64 bits,
  returning the raw machine-word value and width to the reduction helpers.
- `ostd_math_gcd` performs the Euclidean remainder loop and returns
  `ouintbits(result, max(left_width, right_width))`. It allocates no storage
  and grants no host capability.
- `ostd_math_lcm` divides by the gcd before multiplying, checks both the
  machine-word multiplication and the declared-width limit, and returns
  `ouintbits(result, max(left_width, right_width))`. Non-unsigned inputs and
  unrepresentable results produce structured failures.

The bounded scope intentionally does not claim native parity for signed,
arbitrary-precision, floating-point, or future numeric families for these
functions. Those shapes remain part of the wider numeric work and are
not silently coerced into the unsigned ABI.

### Native fixed-width unsigned `divmod` (bounded slice)

The Native C backend directly lowers `std/math.divmod` for two explicit
fixed-width unsigned values. It returns the same two-item list as the
reference runtime: quotient first and remainder second. The result values use
the wider input width, so mixed-width calls do not silently narrow either
component.

- `emitCInstr` recognizes the static `std/math.divmod` builtin, validates its
  two-argument shape, and emits `ostd_math_divmod` directly. It does not use
  `ostd_dispatch`, `ocall`, or an evaluator fallback.
- `ostd_math_unsigned` admits only `O_UINT` values with explicit widths from 1
  through 64 bits. `ostd_math_divmod` rejects other kinds and a zero divisor
  with structured native failures.
- The helper performs unsigned quotient/remainder operations, constructs two
  width-tagged scalar values, and passes them to `olist`. The helper does not
  retain input pointers or grant a host capability; the returned list owns its
  copied item buffer through the normal aggregate tracker.

The bounded native contract is intentionally narrower than the reference
function, which also accepts signed and arbitrary-precision integers. Those
forms remain outside this slice and are not coerced into the unsigned ABI.
`TestNativeCoreUnsignedDivmodParity` in
`internal/osiris/native_core_divmod_test.go` compares mixed-width quotient,
remainder, list indexing, and a 64-bit case across the interpreter, VM,
generated C, and native executable. The lowering and error-contract tests
check direct helper selection, inventory status, capability absence, helper
ownership, zero-divisor behavior, and explicit rejection of an unsupported
signed input.

`TestNativeCoreIntegerMathParity` in
`internal/osiris/native_core_numeric_test.go` compares mixed-width `gcd`,
`lcm`, and zero behavior through the interpreter, VM, generated C, and native
executable. `TestNativeCoreIntegerMathLoweringContract` checks the direct
helper symbols, the absence of `std/math` dispatcher and indirect-call
boundaries in `main`, and the no-capability contract.

The focused differential witness
`TestNativeCoreDataParityExpanded` in
`internal/osiris/native_core_library_parity_test.go` executes the same core/data
fixture through the interpreter, VM, and generated C executable. It covers
payload-bearing Results, Option extraction, codec round-tripping, tensor
construction, reductions, dot product, and shape reporting.

### Native fixed-width bit operations

The Native C backend directly lowers the complete pure unsigned bit-operation
family in `std/math`. Each function accepts exactly one explicit fixed-width
unsigned scalar (`u8`, `u16`, `u32`, or `u64`). The input is masked to its
declared width before the operation, so unused machine-word bits cannot affect
the result. These functions allocate no storage, retain no input pointer, and
require no host capability.

| Symbol | Parameter | Return value | Exact behavior and failure contract |
| --- | --- | --- | --- |
| `std/math.count_ones(value)` | `value: uN` | `u64` | Counts set bits in the declared `N`-bit representation. |
| `std/math.count_zeros(value)` | `value: uN` | `u64` | Counts clear bits in the declared `N`-bit representation, including leading zeroes. |
| `std/math.leading_zeros(value)` | `value: uN` | `u64` | Counts clear bits from bit `N-1` toward bit `0`; zero returns `N`. |
| `std/math.trailing_zeros(value)` | `value: uN` | `u64` | Counts clear bits from bit `0` upward; zero returns `N`. |
| `std/math.bit_width(value)` | `value: uN` | `u64` | Returns the mathematical bit length; zero returns `0`. |
| `std/math.bit_floor(value)` | `value: uN` | `uN` | Returns zero for zero, otherwise the greatest power of two no greater than the input. |
| `std/math.bit_ceil(value)` | `value: uN` | `uN` | Returns the least power of two at least as large as the input. A result outside `uN` is a structured out-of-range failure, never a wrap. |
| `std/math.has_single_bit(value)` | `value: uN` | `bool` | Returns true exactly for positive powers of two. |
| `std/math.byte_swap(value)` | `value: uN` | `uN` | Reverses the bytes in the declared representation. A non-byte-aligned width is rejected with a structured unsupported error. |

The generated-C symbols are intentionally one-to-one with those contracts:

- `ostd_math_bit_input` validates the `O_UINT` tag and explicit width, then
  masks the copied machine value. It does not borrow or retain the input.
- `ostd_math_count_ones`, `ostd_math_count_zeros`,
  `ostd_math_leading_zeros`, `ostd_math_trailing_zeros`, and
  `ostd_math_bit_width` return a newly tagged scalar value with width `64`.
- `ostd_math_bit_floor`, `ostd_math_bit_ceil`, and `ostd_math_byte_swap`
  preserve the input width in `ouintbits`; `ostd_math_has_single_bit` returns
  an inline boolean.
- `emitCInstr` checks arity before emitting the direct helper call. The calls
  do not use `ostd_dispatch`, `ocall`, the interpreter, or the VM. Invalid
  kinds, an unrepresentable ceiling, and a non-byte-aligned byte swap return a
  generated native failure without allocating an intermediate aggregate.

`TestNativeCoreBitOperationsParity` in
`internal/osiris/native_core_bitwise_test.go` compares all nine operations,
width-sensitive zero behavior, and 16/64-bit byte reversal through the
interpreter, VM, generated C, and native executable.
`TestNativeCoreBitOperationsLoweringOwnershipAndCapabilityContract` checks
the direct helper symbols, the no-dispatch/no-capability boundary, helper
allocation absence, and inventory entries. `TestNativeCoreBitOperationsErrorContract`
checks invalid input and unrepresentable results from both the reference
runtime and the generated executable.

## Native `std/list` reduction contracts

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

`TestNativeLibraryListReductionParity` in
`internal/osiris/native_core_library_parity_test.go` compares integer, `f64`,
and complex sum/product results across the interpreter, VM, and generated C
paths. The broader standard-library witnesses remain in
`internal/osiris/native_option_parity_test.go`,
`internal/osiris/native_numeric_parity_test.go`, and
`internal/osiris/native_stdlib_parity_test.go`.

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

`TestNativeLibraryListFlattenParity` compares depth-one, depth-two, and
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

`TestNativeLibraryListPartitionParity` and
`TestNativeLibraryListPartitionOwnershipAndErrorContract` in
`internal/osiris/native_core_library_parity_test.go` cover interpreter/VM/native
execution, the caught out-of-range error, tuple indexing, allocation symbols, capability absence,
and the invalid-input error contract.

## Native `std/list` pure transform and grouping contracts

The following members are direct, host-independent Native C lowerings. Every
result is constructed through the tracked `olist` allocator; inputs are read
only, and no member defines an `OSIRIS_CAP_*` host capability. List inputs and
materialized outputs use the `std/list` bound of 100,000 items.

### `std/list.reverse`

`reverse(values: list) -> list` returns a new list in reverse source order. A
non-list or an input above the item bound returns a structured error. The C
entry point is `ostd_list_reverse` and it copies element values into a fresh
array before constructing the result.

### `std/list.at`

`at(values: list, index: i64) -> value` returns one zero-based element. The
index must be integral, non-negative, and smaller than the list length; invalid
indexes return `list index out of range`. The `ostd_list_at` path returns the
already-owned element value and does not expose a C pointer or borrowed slice.

### `std/list.index_of`

`index_of(values: list, target: value) -> i64` scans from the beginning and
returns the first equal index, or `-1i64` when no value matches. Equality uses
the shared `oequal` contract and equality failures are returned as structured
errors. The implementation is `ostd_list_index` with the first-match mode.

### `std/list.last_index_of`

`last_index_of(values: list, target: value) -> i64` returns the final equal
index, or `-1i64`. The scan remains deterministic and bounded; it does not
mutate or sort the input. It uses the same `ostd_list_index` helper with
last-match mode.

### `std/list.contains`

`contains(values: list, target: value) -> bool` returns whether any element is
equal to `target`. It short-circuits on the first match and materializes no
result list. The dispatcher derives the boolean from the bounded index search.

### `std/list.count`

`count(values: list, target: value) -> u64` counts equal elements without
changing their order. The count is bounded by the input item limit and is
returned as an ordinary unsigned language value.

### `std/list.sort`

`sort(values: list) -> list` returns a stable ascending copy using the language
`<` comparison contract. Incomparable values fail before a partial result is
returned; the source list remains unchanged. `ostd_list_sort` uses bounded
insertion sorting and allocates one working element array plus one tracked
result list.

### `std/list.unique`

`unique(values: list) -> list` keeps the first occurrence of each equal value,
preserving source order. `ostd_list_unique` performs deterministic equality
checks against the already-emitted prefix and returns an independently owned
list.

### `std/list.zip`

`zip(left: list, right: list) -> list<tuple>` pairs matching positions and
stops at the shorter input. Each pair is a tracked tuple (not an ordinary
list), and the outer result is a new list. `ostd_list_zip` constructs and
marks each pair with the tuple discriminator before returning.

### `std/list.concat`

`concat(lists...) -> list` concatenates zero or more bounded lists into one
owned result. The total item count is checked before allocation, and non-list
arguments return a structured error. The dispatcher copies each input range
once into the result staging array.

### `std/list.repeat`

`repeat(value: value, count: i64) -> list` creates `count` copies of one value.
Count must be integral, non-negative, and no greater than 100,000; the bound
is checked before allocation. The result has no alias to a caller-owned list
container.

### `std/list.empty`

`empty() -> list` returns a zero-length owned list and rejects arguments. It is
pure and does not allocate a host resource.

### `std/list.all`

`all(values: list) -> bool` returns true for an empty list and otherwise
requires every element to be truthy under the language value contract. The
runtime short-circuits on the first false value.

### `std/list.any`

`any(values: list) -> bool` returns false for an empty list and short-circuits
on the first truthy element. It does not materialize an intermediate list.

### `std/list.none`

`none(values: list) -> bool` returns true for an empty list and false as soon
as a truthy element is found. It is the bounded complement of `any` under the
same truthiness rules.

### `std/list.enumerate`

`enumerate(values: list) -> list<tuple<u64,value>>` pairs each source value
with its zero-based `u64` index. Pair values are marked as tuples, emitted in
source order, and stored in newly tracked list allocations.

`TestNativeLibraryListPureParity` in
`internal/osiris/native_library_list_test.go` compares all sixteen members across the
interpreter, VM, generated C, and native executable. The companion
`TestNativeLibraryListPureOwnershipErrorAndCapabilityContract` checks the C
entry points, individual backend inventory records, invalid-input behavior,
tracked list construction, and absence of host capability definitions.

### `std/list.chunks`

`chunks(values: list, width: i64) -> list<list>` validates a bounded input list
and a positive width no greater than the `std/list` item limit of 100,000. It
returns chunks in source order; every chunk is a separately tracked list copy,
and the final chunk may contain fewer than `width` values. Empty input returns
an empty outer list. The source list and its element values are never mutated
or retained through a borrowed C slice.

The generated-C implementation uses `ostd_list_shape` in chunk mode and routes
through `ostd_dispatch`. It validates the list bound and width before any
materialized output allocation, frees staging storage when a nested allocation
fails, and returns structured errors for non-list input, wrong arity, a
non-integral or non-positive width, an oversized width, or allocation failure.
The operation is pure and does not define or require an `OSIRIS_CAP_*` host
capability. `nativeStaticModuleMember`, `nativeModuleMemberStatus`, and the
individual `BackendCapabilityInventory` entry identify the admitted operation.

`TestNativeLibraryListChunksParity` in
`internal/osiris/native_library_list_chunks_test.go` compares normal, empty,
short-final-chunk, invalid-width, oversized-width, and invalid-input behavior
across the interpreter, VM, generated C, and native executable. The companion
`TestNativeLibraryListChunksOwnershipErrorAndCapabilityContract` checks the
generated helper and dispatcher symbols, tracked list construction, structured
error strings, capability absence, and the individual inventory record.

## Native tuple construction and indexing (bounded slice)

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

`TestNativeCoreDataParityExpanded` covers mixed-width numeric, string,
nested tuple construction, direct tuple rendering, constant indexing, and
tuple length through the interpreter, VM, and generated-C executable. Tuple
pattern matching remains a separate boundary until its ownership and generated
representation are implemented.

### Native named products (bounded slice)

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

Standalone product output and field indexing are now part of the same bounded
ABI. `oprint_product_value` renders a product as
`Point { x: 1u64, y: 2u64 }`, preserving declaration/map order and recursively
using the tuple-aware renderer for nested aggregate fields. `oat_product_or_value`
accepts a string field name, copies the temporary lookup key, resolves the
field through `omapget`, and releases that temporary key before returning.
Missing fields return `product field not found`; non-string product indices
return `product index expects a string field name`. The interpreter and VM use
the same string-field contract through `atValue`, while numeric positional
product indexing remains intentionally unsupported.

`TestNativeCoreNamedProductParity` compares declaration, construction,
member reads, standalone rendering, string-field indexing, updates, and missing
field handling through interpreter, VM, generated C, and the native executable.
`TestNativeCoreNamedProductRenderingAndIndexContract` checks the generated
ABI symbols, temporary-key ownership boundary, and absence of host capability
requirements. `TestNativeCoreNamedProductDiagnostic` preserves
source-spanned rejection for invalid field values.

### Native aliases and newtypes (bounded slice)

Type aliases and single-field newtypes now use explicit IR contracts rather
than the former declaration rejection:

- `Runtime.BuildIR` records `IRProgram.Aliases` for `type Count = u64` and
  records a newtype as an `IRProductType` with `Newtype: true` and one field
  named `value`.
- `irBuilder.lowerStmt` treats both declarations as compile-time/runtime
  metadata. An alias constructor such as `Count(7u64)` evaluates its one
  argument once and emits a value `copy`; it does not allocate or retain a
  second runtime wrapper.
- A newtype constructor such as `MissionID(7u64)` emits the `newtype` IR
  instruction. `emitCInstr` lowers it to `onewtype`, which copies the value
  into the tracked product map with the nominal type name and `value` field.
- `omember_any` and `oat_product_or_value` expose `.value` and `["value"]`
  through the same owned product lookup contract as ordinary products.
- `oprint_product_value` recognizes the explicit newtype marker and renders
  `MissionID(7u64)`; ordinary one-field structs retain the
  `Type { field: value }` form.
- `OProduct.newtype` is metadata only. It does not add a host capability,
  borrow an input pointer, or change copy-on-write ownership of the field map.

`TestNativeCoreAliasesAndNewtypesParity` compares transparent alias values,
nominal newtype construction, rendering, member access, and string indexing
through interpreter, VM, generated C, and the native executable.
`TestNativeCoreAliasesAndNewtypesIRContract` additionally verifies that the
alias is represented as a transparent `copy` and the newtype as a one-field
`newtype` instruction before C emission.
`TestNativeCoreAliasesAndNewtypesOwnershipContract` checks `onewtype`, the
product access helpers, temporary lookup-key release, and the no-capability
boundary. `TestNativeCoreNewtypeDiagnostic` checks the source-spanned
type error for a value that cannot initialize the declared target type.

### Native named-product patterns (coordinated slice)

Named-product patterns use the source form
`Point { x: x, y: y }`. The type name is nominal, each listed field is
matched recursively, and omitted fields are unconstrained. The shared parser
node is `structPattern` with `structPatternField` entries; duplicate fields are
rejected while parsing. `validateEnumPattern` checks that the type is an
ordinary declared struct and that every named field exists, producing a
source-spanned diagnostic such as `struct Point has no field "missing" in
pattern` before interpreter, VM, or Native C execution.

`matchPattern` applies the same nominal-type and field lookup contract in the
interpreter and VM. `clonePattern`, macro freshening, alias binding, and
`patternBindingNames` recurse through every field subpattern, so nested binds,
grouped alternatives, guards, and macro expansion retain the same bindings as
enum and list patterns. `formatPattern` emits a stable named-field form.

Native lowering adds `matchproduct` and `productfield` IR instructions inside
`lowerNativeEnumPatternBranch` and `bindNativeEnumPatternValue`. The generated
C runtime implements them with `oproductis` and `oproductfield`: matching
checks the nominal product tag and the existence of every requested field,
then field values are read without allocating lookup keys. `emitCInstr` emits
only string literals and bounded field arrays; no host capability is defined,
no source pointer is retained, and no per-field allocation is introduced.

`TestNativeCoreProductPatternParity` compares full interpreter, VM,
generated-C, and native-executable behavior for complete, partial, nested,
binding, and failed product patterns. The dedicated format/clone, IR contract,
diagnostic, and ownership/capability tests cover the remaining contracts.
Product patterns do not change the separate boundaries for unsupported future
numeric types, provider-dependent values, or cross-platform generated-C
execution.

### Native tuple destructuring (bounded slice)

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

`TestNativeCoreTupleDestructuringParity` is the differential witness for
flat and nested tuple bindings across interpreter and VM, and checks the
generated-C artifact for the corresponding `oat` operations.

These witnesses do not close either task by themselves. Unverified product
rendering/indexing, wide numeric, dual/complex-width, provider-dependent host,
and cross-platform execution surfaces must remain non-stable until they have
executable evidence. Unsupported native members must continue to produce
source-spanned diagnostics rather than being admitted by inventory metadata
alone.

## Native `std/text` boundary contracts

The following bounded, host-independent members are a native C slice:

| Symbol | Signature | Capability | Result and ownership |
| --- | --- | --- | --- |
| `std/text.trim` | `trim(value: string) -> string` | none | New owned UTF-8 string with Unicode whitespace removed at both ends. |
| `std/text.trim_space` | `trim_space(value: string) -> string` | none | Same owned result contract as `trim`; the alias is independently admitted and tested. |
| `std/text.trim_left` | `trim_left(value: string, code_points: string) -> string` | none | Remove leading Unicode code points that occur in `code_points`; the result is newly owned and an empty set leaves the input unchanged. |
| `std/text.trim_right` | `trim_right(value: string, code_points: string) -> string` | none | Remove trailing Unicode code points that occur in `code_points`; the result is newly owned and an empty set leaves the input unchanged. |
| `std/text.lower` | `lower(value: string) -> string` | none | New owned UTF-8 string using the native Unicode lower-case table; the input is not mutated. |
| `std/text.contains` | `contains(value: string, needle: string) -> bool` | none | Boolean search result; no borrowed result escapes the call. |
| `std/text.has_prefix` | `has_prefix(value: string, prefix: string) -> bool` | none | Boolean prefix predicate; both arguments remain caller-owned. |
| `std/text.has_suffix` | `has_suffix(value: string, suffix: string) -> bool` | none | Boolean suffix predicate; both arguments remain caller-owned. |
| `std/text.upper` | `upper(value: string) -> string` | none | New owned UTF-8 string using the locale-independent native case table; one-to-one mappings preserve `ß`, matching the reference runtime. |
| `std/text.fields` | `fields(value: string) -> list<string>` | none | New owned list of non-empty fields split on Unicode whitespace; field strings do not retain the input allocation. |
| `std/text.join` | `join(separator: string, values: list<value>) -> string` | none | New owned string formed from rendered list values and separators; inputs remain caller-owned. |
| `std/text.compare` | `compare(left: string, right: string) -> i64` | none | Lexicographic UTF-8 byte comparison returning exactly `-1`, `0`, or `1`. |
| `std/text.equal_fold` | `equal_fold(left: string, right: string) -> bool` | none | Unicode simple-fold equality for the portable case-equivalence table; no result allocation escapes. |
| `std/text.contains_any` | `contains_any(value: string, code_points: string) -> bool` | none | Return whether any decoded character in `value` occurs in `code_points`; an empty set returns `false`. |
| `std/text.cut` | `cut(value: string, separator: string) -> option<tuple<string,string>>` | none | Split at the first separator and return owned before/after values; an absent separator returns `none`. |
| `std/text.cut_prefix` | `cut_prefix(value: string, prefix: string) -> option<string>` | none | Remove an exact prefix and return the owned remainder; a non-matching prefix returns `none`. |
| `std/text.cut_suffix` | `cut_suffix(value: string, suffix: string) -> option<string>` | none | Remove an exact suffix and return the owned remainder; a non-matching suffix returns `none`. |
| `std/text.count` | `count(value: string, needle: string) -> u64` | none | Count non-overlapping substring matches; an empty needle counts code-point boundaries plus one. |
| `std/text.index` | `index(value: string, needle: string) -> option<u64>` | none | Return the first matching code-point index, or `none`; empty needles return `some(0)`. |
| `std/text.last_index` | `last_index(value: string, needle: string) -> option<u64>` | none | Return the final matching code-point index, or `none`; an empty needle returns the text length in code points. |
| `std/text.index_any` | `index_any(value: string, code_points: string) -> option<u64>` | none | Return the first code-point index whose decoded character occurs in `code_points`; an empty set or absent character returns `none`. |
| `std/text.last_index_any` | `last_index_any(value: string, code_points: string) -> option<u64>` | none | Return the final code-point index whose decoded character occurs in `code_points`; an empty set or absent character returns `none`. |
| `std/text.repeat` | `repeat(value: string, count: integer) -> string` | none | New owned string repeated `count` times; negative counts and results above the 1 MiB text bound fail before allocation. |
| `std/text.trim_prefix` | `trim_prefix(value: string, prefix: string) -> string` | none | Remove one exact prefix when present and otherwise return an owned copy of the original value. |
| `std/text.trim_suffix` | `trim_suffix(value: string, suffix: string) -> string` | none | Remove one exact suffix when present and otherwise return an owned copy of the original value. |
| `std/text.replace` | `replace(value: string, old: string, replacement: string) -> string` | none | Replace bounded non-overlapping occurrences and return one owned string. |
| `std/text.replace_n` | `replace_n(value: string, old: string, replacement: string, limit: integer) -> string` | none | Replace at most the requested bounded number of occurrences. |
| `std/text.split` | `split(value: string, separator: string) -> list<string>` | none | Split on a separator with deterministic empty-field behavior. |
| `std/text.split_after` | `split_after(value: string, separator: string) -> list<string>` | none | Split after each separator while retaining the separator in the preceding field. |
| `std/text.split_n` | `split_n(value: string, separator: string, limit: integer) -> list<string>` | none | Split into at most the requested number of fields. |
| `std/text.split_after_n` | `split_after_n(value: string, separator: string, limit: integer) -> list<string>` | none | Split after separators into at most the requested number of fields. |
| `std/text.chunks` | `chunks(value: string, size: integer) -> list<string>` | none | Split on positive Unicode code-point widths; each returned string is newly owned and the final chunk may be shorter. |

`std/text.chunks` is directly emitted as `ostd_text_chunks` after the static
module admission check; the other entries use the bounded dispatcher where
their argument shape is intentionally runtime-polymorphic. `ostd_dispatch`
validates arity and runtime kinds before calling those helpers.
`ostd_text_trim`, `ostd_text_case`, `ostd_text_join`, and `ostd_text_repeat`
construct owned result strings through `oownedstring`; `ostd_text_fields`
constructs a newly owned list whose members are copied strings. Predicates and
comparisons return scalar values, while `ostd_text_index` returns a newly owned
option containing a code-point index. `ostd_text_count` and the search helpers
validate UTF-8 before returning, handle empty needles without walking beyond a
terminator, and use non-overlapping matching. Invalid UTF-8, wrong argument
kinds, negative repeat counts, and oversized results produce structured native
errors. `ostd_text_chunks` checks the shared 1 MiB input bound and positive
exact width, decodes UTF-8 by code point, and copies every chunk through
`oownedstring` before constructing the tracked result list. Since this slice
performs no host I/O, it does not accept or infer an OS capability.
`TestNativeLibraryTextParity` compares the interpreter, VM, generated C, and
generated executable for the existing text members; the focused
`TestNativeLibraryTextChunksParity` covers Unicode widths, empty input, and
caught invalid arguments, while
`TestNativeLibraryTextChunksOwnershipErrorAndCapabilityContract` checks the
direct call site, ownership boundary, error strings, capability absence, and
inventory entry.

## Native `std/signal` pure-value contracts (bounded slice)

This section documents the portable Native C subset for the bounded,
host-independent signal operations listed below. Each operation accepts the
same rank-one numeric signal contract as the interpreter/VM implementation;
the Native C runtime copies list or tensor samples into temporary `double`
storage before calculating and returns a newly owned rank-one tensor for data
results. No operation reads a clock, starts a worker, consults a host signal
library, or defines an `OSIRIS_CAP_*` macro.

| Symbol | Signature | Result and exact contract | Ownership, errors, and capability |
| --- | --- | --- | --- |
| `std/signal.rectangular` | `rectangular(length: u64) -> tensor[length]` | All samples are `1`; length may be `0..65536`. | New tensor storage; invalid, fractional, negative, or oversized lengths fail before allocation. |
| `std/signal.hann` | `hann(length: u64) -> tensor[length]` | Symmetric Hann window using `0.5 - 0.5*cos(2*pi*i/(n-1))`; lengths `0` and `1` are handled deterministically. | New tensor storage; bounded length; no host capability. |
| `std/signal.hamming` | `hamming(length: u64) -> tensor[length]` | Symmetric Hamming window using the fixed `0.54 - 0.46*cos(...)` formula. | New tensor storage; bounded length; no host capability. |
| `std/signal.blackman` | `blackman(length: u64) -> tensor[length]` | Symmetric fixed-alpha Blackman window using `0.42 - 0.5*cos(x) + 0.08*cos(2*x)`. | New tensor storage; bounded length; no host capability. |
| `std/signal.bartlett` | `bartlett(length: u64) -> tensor[length]` | Symmetric triangular window with explicit zero/one-length handling. | New tensor storage; bounded length; no host capability. |
| `std/signal.convolve` | `convolve(signal: signal, kernel: signal) -> tensor` | Full discrete convolution; output length is `len(signal)+len(kernel)-1`, or zero for an empty input. | Both inputs are copied and never mutated; non-rank-one/non-numeric data and allocation failures return structured errors before partial output; no host capability. |
| `std/signal.correlate` | `correlate(left: signal, right: signal) -> tensor` | Full discrete cross-correlation; output length is `len(left)+len(right)-1`, or zero for an empty input. | Both inputs are copied and never mutated; non-rank-one/non-numeric data fails before a partial result. |
| `std/signal.filter_fir` | `filter_fir(signal: signal, coefficients: signal) -> tensor` | Direct bounded finite-impulse-response accumulation; output length equals the signal length. | Inputs are copied; output is newly owned; malformed numeric data and allocation failure are structured errors. |
| `std/signal.resample` | `resample(signal: signal, length: u64) -> tensor[length]` | Deterministic linear interpolation over a rank-one real signal. The first and last input samples are preserved; an empty input produces zero-filled output, a singleton input is repeated, zero output length returns an empty tensor, and one output element is the first input sample. Input and output lengths are each limited to `0..65536`. | `ostd_signal_data` stages and validates the bounded input, while `ostd_signal_resample` releases that temporary storage on every success and failure path before transferring a newly allocated tensor to `otensordata`. Invalid rank, non-numeric values, negative/fractional/oversized output lengths, and oversized input fail with structured errors. No capability is defined. |
| `std/signal.mean` | `mean(signal: signal) -> number` | Arithmetic mean of a non-empty signal. | Scalar by value; empty input and invalid samples fail; no allocation escapes. |
| `std/signal.variance` | `variance(signal: signal) -> number` | Population variance using the documented two-pass mean/delta formula. | Scalar by value; empty input and invalid samples fail; no capability. |
| `std/signal.energy` | `energy(signal: signal) -> number` | Sum of squared real sample values; empty input returns zero. | Scalar by value; temporary copied samples are released before return. |
| `std/signal.rms` | `rms(signal: signal) -> number` | `sqrt(energy / sample_count)` for a non-empty signal. | Scalar by value; empty input and invalid samples fail; no capability. |
| `std/signal.normalize` | `normalize(signal: signal, mode: string) -> tensor` | `peak`, `minmax`, and `zscore` modes; zero peak/span/deviation produce zero samples rather than NaN or an error. | Input is copied, output owns its tensor storage, mode is validated before transformation, and unknown modes fail. |
| `std/signal.fft` | `fft(signal: list<number> \| tensor<number> \| tensor<complex>) -> tensor<complex>` | Forward deterministic DFT with the `max_transform_samples` bound of 4096. Real inputs are promoted to zero-imaginary complex samples; complex lists and complex tensors preserve both components. | `ostd_signal_complex_data` copies input into temporary `OComplex` storage and `ocomplextensordata` takes ownership of the newly allocated result. Invalid rank, non-numeric values, oversized input, and allocation failure return structured errors. No capability is defined. |
| `std/signal.ifft` | `ifft(spectrum: list<number> \| tensor<number> \| tensor<complex>) -> tensor<complex>` | Inverse deterministic DFT using the same 4096-sample bound and normalizing by input length. | The same copy/ownership boundary as `fft`; empty input returns an owned rank-one complex tensor with zero elements. Input pointers are never retained and no host capability is required. |

The dispatcher enforces exact arity for every member. `ostd_signal_data` is the
single staging boundary for list/tensor inputs; `ostd_signal_window`,
`ostd_signal_resample`, `ostd_signal_conv`, `ostd_signal_fir`, `ostd_signal_stats`, and `ostd_signal_normalize` own their
temporary allocations and transfer only complete results to `otensordata`.
`TestNativeLibrarySignalPureParity` compares interpreter, VM, generated C, and
native executable output for windows, filtering, correlation, statistics, and
all normalization modes. `TestNativeLibrarySignalOwnershipCapabilityAndErrorContract`
checks the helper symbols, no-capability boundary, exact error strings, and
each individual backend-inventory entry. The focused
`TestNativeLibrarySignalResampleParity` fixture adds generated-C and executable
evidence for interpolation, empty and singleton inputs, zero- and one-element
outputs, and caught invalid bounds and signal shapes. Its companion
`TestNativeLibrarySignalResampleOwnershipErrorAndCapabilityContract` checks
the resampler's admission, staging, cleanup, owned tensor transfer, error
strings, and individual inventory entry.

`ostd_signal_complex_data` is the separate complex staging boundary: it accepts
rank-one real tensors, rank-one complex tensors, and numeric or complex lists,
copies every sample into an `OComplex` buffer, and rejects all other shapes
before the DFT begins. `ostd_signal_fft` performs the bounded O(n²) transform
without a platform FFT provider or worker task, then transfers the result to
`ocomplextensordata`. The dispatcher enforces exactly one argument for both
members. `TestNativeLibrarySignalPureParity` covers real, complex, forward, and
inverse results; `TestNativeLibrarySignalOwnershipCapabilityAndErrorContract`
checks the helper symbols, complex-tensor ownership, exact invalid-input error,
capability absence, and individual inventory entries.

The broader `std/signal` row remains partial because other provider-independent
additions and the aggregate standard-library evidence requirements are outside
this bounded slice.

## Native `std/crypto` SHA-512 contracts (bounded slice)

This slice closes the pure, host-independent digest members for the standard
SHA-512 family. HMAC is documented separately below; random bytes, key
derivation, and other host/provider surfaces remain outside this slice.

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
provider discovery. `TestNativeLibraryCryptoSHA512Parity` compares digest
strings, raw bytes, and the capability predicate across the interpreter, VM,
generated C, and generated executable. `TestNativeLibraryCryptoSHA512OwnershipCapabilityAndErrorContract`
checks the generated ownership symbols, pure-capability boundary, error
messages, and individual backend-inventory entries.

## Native `std/crypto.hmac` contract

`std/crypto.hmac(key: string | list<u8>, data: string | list<u8>, algorithm: string) -> string`
computes lowercase hexadecimal HMAC-SHA-256, HMAC-SHA-512, or HMAC-SHA-512/256.
The algorithm constants `crypto.sha256`, `crypto.sha512`, and
`crypto.sha512_256` lower to owned strings and are accepted anywhere a
canonical algorithm name is required.

Keys are capped at 65536 bytes and message data at 1 MiB before hashing. Text
uses its UTF-8 bytes. List inputs are staged into temporary byte storage using
the established byte-packing contract, which accepts unsigned values and
retains their low byte; that staging is released on both success and failure.
The HMAC pads a normalized key to the selected SHA block size, performs the
inner and outer hash locally through `OSHA256` or `OSHA512`, and returns a new
owned hexadecimal string. No borrowed input pointer or host capability escapes
the operation.

Wrong arity, non-string algorithms, non-unsigned list elements, unsupported
algorithm names, oversized keys, and oversized data return structured native
errors before a partial result is exposed. `ostd_crypto_hmac` and
`ostd_dispatch` are the runtime boundaries. `TestNativeLibraryCryptoHMACParity`
compares all three algorithms, algorithm constants, byte-list packing, and
caught failures across interpreter, VM, generated C, and the executable.
`TestNativeLibraryCryptoHMACOwnershipErrorAndCapabilityContract` checks the
hash helpers, temporary byte staging, owned result boundary, error strings,
constant lowering, inventory entry, and absence of `OSIRIS_CAP_*` definitions.

## Native `std/crypto` codec and comparison contracts (bounded slice)

This separate slice covers the admitted, host-independent hexadecimal,
Base64, and digest-comparison members. The Base64 implementation is local and
does not call an operating-system or third-party provider.

| Symbol | Signature | Capability | Result, validation, and ownership |
| --- | --- | --- | --- |
| `std/crypto.hex_encode` | `hex_encode(data: string \| list<u8>) -> string` | none | Lowercase hexadecimal. Strings are treated as their UTF-8 bytes; byte lists are copied into temporary staging. The owned result is bounded to a 1 MiB input. |
| `std/crypto.hex_decode` | `hex_decode(text: string) -> list<u8>` | none | Decodes upper- or lowercase hexadecimal into a newly owned `list<u8>`. Odd length, invalid digits, and decoded output over 1 MiB fail before a partial result is exposed. |
| `std/crypto.base64_encode` | `base64_encode(data: string \| list<u8>) -> string` | none | RFC 4648 standard alphabet with `=` padding, returned as a newly owned string. Input is capped at 1 MiB. |
| `std/crypto.base64_decode` | `base64_decode(text: string) -> list<u8>` | none | Decodes the padded standard alphabet into newly owned bytes. Invalid alphabet, padding, trailing bits, input over 2 MiB, or output over 1 MiB fail before a partial result is exposed. |
| `std/crypto.base64url_encode` | `base64url_encode(data: string \| list<u8>) -> string` | none | RFC 4648 URL-safe alphabet without padding, returned as a newly owned string. Input is capped at 1 MiB. |
| `std/crypto.base64url_decode` | `base64url_decode(text: string) -> list<u8>` | none | Decodes the unpadded URL-safe alphabet into newly owned bytes. Padding, invalid alphabet, trailing bits, input over 2 MiB, or output over 1 MiB fail explicitly. |
| `std/crypto.compare_digest` | `compare_digest(left: string \| list<u8>, right: string \| list<u8>) -> bool` | none | Compares all bytes in the common length and folds length mismatch into the result without an early-exit content comparison. Temporary list staging is released on success and failure. |

The C symbols are `ostd_crypto_hex`, `ostd_crypto_base64_value`,
`ostd_crypto_base64_encode`, `ostd_crypto_base64_decode`,
`ostd_crypto_compare_digest`, `ostd_bytes_list`, `oownedstring`, and
`ostd_dispatch`. `ostd_crypto_hex`
validates the 1 MiB input/output contract before allocation;
`ostd_crypto_compare_digest` uses an equality length flag rather than
narrowing the length difference to an 8-bit value, so lengths such as 0 and
256 cannot compare equal accidentally. The Base64 helpers validate padding and
unused trailing bits, and copy all byte-compatible list inputs before release.
Neither helper retains a borrowed input pointer or emits an `OSIRIS_CAP_*`
definition.

`TestNativeLibraryCryptoCodecParity` compares each member and caught malformed
input across interpreter, VM, generated C, and the generated executable.
`TestNativeLibraryCryptoCodecOwnershipErrorAndCapabilityContract` checks the
hexadecimal and comparison symbols, while
`TestNativeLibraryCryptoBase64Parity` and
`TestNativeLibraryCryptoBase64OwnershipAndErrorContract` cover the four
Base64 members, their allocation boundary, malformed inputs, capability
absence, and individual backend-inventory status. The granular public
signatures and native notes are also recorded in `docs/src/std-task-reference.js`.

## Native `std/time` pure-value contracts

The existing UTC `std/time` generated-C subset has these individually
documented host-independent contracts:

| Symbol | Signature | Result | Capability and ownership |
| --- | --- | --- | --- |
| `std/time.round` | `round(value: duration \| datetime, quantum: duration) -> duration \| datetime` | Same kind as `value` | Non-negative quantum; duration ties round away from zero, while UTC datetimes round from the language's year-1 zero time. Checked scalar return; no capability, allocation, or input-pointer escape. |
| `std/time.truncate` | `truncate(value: duration \| datetime, quantum: duration) -> duration \| datetime` | Same kind as `value` | Non-negative quantum; duration values truncate toward zero and UTC datetimes truncate from the year-1 zero time. Checked scalar return; no capability, allocation, or input-pointer escape. |
| `std/time.compare` | `compare(left: duration \| datetime, right: duration \| datetime) -> i64` | `-1i64`, `0i64`, or `1i64` | Both values must have the same kind. The scalar is returned by value; no capability or allocation is used. |
| `std/time.equal` | `equal(left: duration \| datetime, right: duration \| datetime) -> bool` | Equality predicate | Compares exact duration nanoseconds or UTC datetime nanoseconds. No input pointer escapes. |
| `std/time.before` | `before(left: duration \| datetime, right: duration \| datetime) -> bool` | Strict-less predicate | Same-kind exact comparison; no capability or allocation. |
| `std/time.after` | `after(left: duration \| datetime, right: duration \| datetime) -> bool` | Strict-greater predicate | Same-kind exact comparison; no capability or allocation. |
| `std/time.year` | `year(value: datetime) -> u64` | Civil year `1..9999` | UTC civil extraction by value; no capability or allocation. |
| `std/time.month` | `month(value: datetime) -> u64` | Month `1..12` | UTC civil extraction by value; no capability or allocation. |
| `std/time.day` | `day(value: datetime) -> u64` | Day `1..31` | UTC civil extraction by value; no capability or allocation. |
| `std/time.hour` | `hour(value: datetime) -> u64` | Hour `0..23` | UTC civil extraction by value; no capability or allocation. |
| `std/time.minute` | `minute(value: datetime) -> u64` | Minute `0..59` | UTC civil extraction by value; no capability or allocation. |
| `std/time.second` | `second(value: datetime) -> u64` | Second `0..59` | UTC civil extraction by value; leap seconds are not represented; no capability or allocation. |
| `std/time.nanosecond` | `nanosecond(value: datetime) -> u64` | Nanosecond `0..999999999` | UTC fractional-second extraction by value; no capability or allocation. |
| `std/time.weekday` | `weekday(value: datetime) -> string` | `Sunday` through `Saturday` | Returns one of seven static strings; no host locale or allocation is consulted. |
| `std/time.year_day` | `year_day(value: datetime) -> u64` | One-based `1..365` or `1..366` | UTC civil extraction by value; no capability or allocation. |
| `std/time.utc` | `utc(value: datetime) -> datetime` | The same instant and normalized nanosecond in the native UTC representation | `ostd_time_utc` validates `O_DATETIME` and returns the complete `ODatetime` value by copy. Native datetimes have no non-UTC location slot, so this direct lowering performs no timezone lookup, allocation, capability check, or input-pointer retention. Wrong arity remains on `ostd_dispatch` so `try/catch` observes `utc expects 1 datetime`. |

The duration and Unix-conversion members are also lowered individually. Their
argument and ownership boundaries are explicit:

| Symbol | Signature | Result | Capability and ownership |
| --- | --- | --- | --- |
| `std/time.duration` | `duration(value: duration \| integer \| quantity[Second]) -> duration` | Exact signed nanoseconds | Accepts an existing duration, an exact signed integer, or a `Second` quantity whose nanosecond product is integral and fits `i64`; returns an `O_DURATION` by value without a capability or retained input pointer. |
| `std/time.unix` | `unix(seconds: integer[, nanosecond_adjustment: integer]) -> datetime` | UTC `datetime` | Checked integer normalization carries nanoseconds into seconds with floor semantics, preserves a non-negative `ODatetime.nanosecond`, and rejects civil-range or signed-overflow results. |
| `std/time.unix_milli` | `unix_milli(milliseconds: integer) -> datetime` | UTC `datetime` | Checked multiplication by `1_000_000`, followed by the same value-owned seconds/nanoseconds normalization as `unix`; no allocation or host capability. |
| `std/time.unix_micro` | `unix_micro(microseconds: integer) -> datetime` | UTC `datetime` | Checked multiplication by `1_000`, followed by the same value-owned seconds/nanoseconds normalization as `unix`; no allocation or host capability. |
| `std/time.nanosecond` | `nanosecond` or `nanosecond(value: datetime)` | `duration` or `u64` | Value position emits `1ns` as a static duration; call position extracts the UTC fractional component. |
| `std/time.microsecond` | `microsecond -> duration` | Exact `1_000ns` duration | Static `INT64_C(1000)` payload; no allocation or capability. |
| `std/time.millisecond` | `millisecond -> duration` | Exact `1_000_000ns` duration | Static `INT64_C(1000000)` payload; no allocation or capability. |
| `std/time.second` | `second` or `second(value: datetime)` | `duration` or `u64` | Value position emits `1s`; call position extracts the UTC second component. |
| `std/time.minute` | `minute` or `minute(value: datetime)` | `duration` or `u64` | Value position emits `1m`; call position extracts the UTC minute component. |
| `std/time.hour` | `hour` or `hour(value: datetime)` | `duration` or `u64` | Value position emits `1h`; call position extracts the UTC hour component. |

`ostd_time_duration`, `ostd_time_unix`, and `ostd_time_unix_scaled` use
checked scalar conversion and return only value-owned `OValue` payloads. The
repaired `ODatetime` representation remains `int64_t seconds` plus normalized
`int32_t nanosecond`; comparison, civil extraction, rounding, and formatting
continue to consume that same representation. `TestNativeLibraryTimeDurationUnixParity`
compares interpreter, VM, generated C, and native executable results, while
`TestNativeLibraryTimeDurationUnixContract` checks the direct helper symbols,
static constant lowering, capability absence, ABI shape, and individual
inventory entries.

The native UTC date family adds these individually tested pure operations:

| Symbol | Signature | Result and exact contract | Ownership, errors, and capability |
| --- | --- | --- | --- |
| `std/time.date` | `date(year, month, day, hour, minute, second, nanosecond[, location]) -> datetime` | Builds a Gregorian UTC instant for years `1..9999`; all seven fields must be exact integers and the optional location must be `utc_location`. | The `ODatetime` payload is returned by value; invalid civil fields, non-UTC locations, overflow, and wrong arity are structured/source-spanned errors. No capability or input allocation is retained. |
| `std/time.add_date` | `add_date(value: datetime, years: integer, months: integer, days: integer) -> datetime` | Applies checked calendar offsets while preserving time-of-day and nanoseconds, including negative month/day offsets and Go-compatible month normalization. | Returns a value-owned `ODatetime`; year-range and arithmetic overflow fail before publication, with no host capability. |
| `std/time.format` | `format(value: datetime, layout: string) -> string` | Formats UTC civil fields, month/weekday names, fractional seconds, and UTC offset tokens from the documented reference-layout subset. | Builds one bounded owned string; invalid kinds/layout values return structured errors, and no host locale, clock, or capability is consulted. |

`ostd_time_compare_values` validates same-kind duration/datetime pairs without
subtracting them, so signed 64-bit extrema cannot overflow. The public
predicates are implemented by `ostd_time_predicate`; the three-way result uses
`ostd_time_compare`. `ostd_time_civil` normalizes negative nanosecond values
before converting the stored instant with `ocivil_from_days`, and
`ostd_time_component` derives the requested year, month, day, hour, minute,
second, nanosecond, weekday, or one-based year-day value. All invalid kinds and mixed comparisons return the
structured native errors documented by the dispatcher; wrong arity is rejected
before entering the helper.

`ostd_time_round` dispatches to checked integer helpers for duration and UTC
datetime payloads. The datetime helper computes the year-1 zero-time remainder
modulo the quantum without overflowing the Unix-nanosecond representation, so
arbitrary positive duration quanta retain the language rule instead of silently
rounding relative to the Unix epoch. The helpers return only `OValue` scalars,
retain no argument pointer, and allocate nothing.

These members do not read the clock, load timezone data, retain input storage,
or define an `OSIRIS_CAP_*` macro. The native inventory records each member
individually. `TestNativeLibraryTimePureValueParity` compares interpreter, VM,
generated C, and native executable outputs, including caught mixed-kind and
wrong-kind errors. `TestNativeLibraryTimePureValueOwnershipCapabilityAndErrorContract`
checks the exact helper symbols, error strings, capability absence, and every
inventory entry. `TestNativeLibraryTimeClockComponentsParity` and
`TestNativeLibraryTimeClockComponentsContract` separately cover the four
clock-component signatures. The broader `std/time` row remains partial because parsing,
locations, `now`, timers, and other members are not part of this slice.

## Linux generated-C evidence

Linux execution evidence is maintained separately from the macOS/POSIX test
run. `internal/osiris/native_host_deferred_linux_parity_test.go` compares deterministic
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
