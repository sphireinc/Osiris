# GPU execution contract

Osiris has two deliberately separate GPU execution providers:

1. `NativeGPUProvider` uses a host device adapter. The current adapter is
   Metal on macOS when the process can obtain an `MTLDevice`.
2. `CPUReferenceProvider` is a deterministic software reference. It is useful
   for tests and explicitly requested fallback, but it is never hardware GPU
   evidence and its device kind is `cpu-reference`.

`CompileGPU` validates the shared program and emits a versioned artifact. The
OpenCL text in `GPUArtifact.Source` is an inspection/interop representation;
it is not used as an execution result. Execution always goes through a
selected `GPUProvider`.

## Public symbols

### `CompileGPU(ctx, program) (GPUArtifact, error)`

Compiles a bounded program into a `GPUArtifact`. It runs the shared GPU
capability checker before collecting kernels, rejects malformed or unsupported
source forms, and validates the resulting artifact. It does not select a
device or silently execute through the interpreter.

### `ValidateGPUArtifact(artifact) error`

Checks the schema version, kernel/value ABI versions, source and kernel count
limits, unique kernel names, operation arity, value types, and ownership
specifications. A provider must reject an artifact that this validator does
not accept.

### `NewNativeGPUProvider(limits) (*NativeGPUProvider, error)`

Discovers the platform-native adapter. It returns a structured
`ErrGPUCapabilityUnavailable` error when no supported device can be obtained;
it never returns a CPU reference device. The current supported adapter is:

- macOS + cgo + Metal framework;
- device identifier `metal-0`;
- device kind `metal`;
- host value ABI `f64`, transferred to shared Metal `float` buffers;
- native capabilities `native-device`, `float32`, `tensor`, and `math`.

The provider does not claim `float64` or `deterministic` capability. Metal
execution currently performs the bounded kernel subset in 32-bit device
precision and converts results back to the host `f64` ABI. Matrix, reduction,
constructor, range, reshape, transpose, and signal operations remain
capability-gated until their device kernels have real-device differential
fixtures.

### `NewMetalProvider(limits) (*NativeGPUProvider, error)`

Explicit alias for the currently supported native adapter. It has the same
discovery and unavailable-device behavior as `NewNativeGPUProvider`.

### `NewDeterministicCPUProvider(limits) *CPUReferenceProvider`

Creates the opt-in deterministic reference provider. Its device is named
`cpu-reference-0` and is intentionally distinguishable from a hardware
device. `NewCPUReferenceProvider` is retained as a compatibility alias.

### `ExecuteGPU(ctx, artifact, provider, request) (GPUValue, error)`

Validates the artifact, negotiates limits and the requested device, allocates
owned input buffers, launches the requested kernel, waits for its event,
reads the owned output, validates it, and releases all buffers on every exit
path.

When `provider` is nil, execution tries `NewNativeGPUProvider` first. If no
native device is available:

- `GPUDeviceRequest.AllowCPUReference == false` returns
  `ErrGPUCapabilityUnavailable`;
- `AllowCPUReference == true` explicitly selects the deterministic CPU
  reference provider.

There is no implicit interpreter, VM, or CPU fallback when a native provider
was supplied.

## Artifact and value ABI

The artifact schema, kernel ABI, and value ABI are versioned independently.
The v1 value ABI is:

| Field | Contract |
| --- | --- |
| `GPUValue.ABI` | `GPUValueABIVersion` |
| `GPUValue.Type` | `GPUValueFloat64` (`f64`) |
| `GPUValue.Shape` | nil for a scalar, otherwise row-major dimensions |
| `GPUValue.Data` | flat row-major payload with one element per shape product |

Kernel inputs are `GPUBorrowed`; kernel outputs are `GPUOwned`. Artifact
validation rejects an input spec with any other ownership, rejects an output
spec that is not owned, and rejects unsupported ABI/type versions before a
provider sees the kernel. Providers copy caller-owned input data into
provider-owned allocations during `Allocate`. Callers release returned
buffers with `GPUBuffer.Release`.

The native Metal adapter currently lowers these operations to actual MSL
compute kernels:

- `elementwise.add`, `elementwise.sub`, `elementwise.mul`,
  `elementwise.div`;
- `math.pow` and `math.hypot`;
- `math.abs`, `math.sqrt`, `math.exp`, `math.log`, `math.log10`,
  `math.log2`, `math.sin`, `math.cos`, `math.tan`, `math.asin`,
  `math.acos`, `math.atan`, `math.floor`, `math.ceil`, and `math.round`.

Elementwise scalar/tensor broadcasting is represented by indexed input
counts. The provider rejects an operation outside this list with
`ErrGPUCapabilityUnavailable`; it does not run the reference kernel instead.

## Device memory and ownership

`GPUBuffer` owns either a reference value or a native device allocation. A
native buffer stores only an opaque handle inside the runtime; language values
never receive a Metal pointer. `Read` waits for the buffer event and performs
device-to-host readback into a fresh host value. It never exposes the backing
buffer.

`Release` is idempotent. On a native output it waits for any in-flight command
before releasing the Metal buffer, which makes cancellation cleanup safe even
when the foreground `GPUEvent.Wait` returned early. A provider retains its
Metal device while allocated buffers exist; `NativeGPUProvider.Close` rejects
new work and closes the device after the last owned buffer is released.

Foreign, nil, released, reference-only, and provider-closed buffers are
rejected with `ErrGPUOwnership` at native launch. Allocation, output creation,
readback, and release failures are returned as `GPUError` values with a
stable stage and cause.

## Synchronization and cancellation

`GPUEvent.Wait(ctx)` waits for the provider event or returns
`ErrGPUCancelled` wrapping `ctx.Err()` if the caller's context is cancelled.
The native Metal adapter commits a command buffer and waits for completion in
the provider event worker. Releasing an output still drains that command
before dropping the native handle, so cancellation cannot leak or prematurely
invalidate device resources.

Cancellation before discovery, allocation, or launch is rejected before a
native call. A device command that reports an error becomes
`ErrGPUExecution`; a context cancellation remains `ErrGPUCancelled` at the
public wait boundary.

## Limits and capability errors

`GPULimits` bounds elements, host buffer bytes, rank, kernel count, and
artifact/source size. The selected device can further reduce those limits;
it cannot increase them. `ValidateGPUValue` and providers check shape,
payload, rank, element, and byte limits before launch and after readback.

The stable error classes are:

- `ErrGPUArtifactMalformed` — invalid schema, ABI, shape, or kernel contract;
- `ErrGPUCapabilityUnavailable` — no device, missing capability, or an
  operation not implemented by the selected provider;
- `ErrGPUResourceLimit` — an artifact, value, or output exceeds a bound;
- `ErrGPUOwnership` — a buffer or provider lifecycle violation;
- `ErrGPUExecution` — device allocation, compilation, dispatch, or readback
  failure;
- `ErrGPUCancelled` — cancellation before or during synchronization.

## Reference provider and evidence policy

The reference provider implements the broader deterministic f64 kernel set:
elementwise arithmetic, dot/matmul, constructors, ranges, shape transforms,
reductions, deterministic math, and the pure signal kernels. These fixtures
are valuable interpreter/VM comparison oracles, but they must be labeled
reference-provider tests. They cannot promote a GPU support-matrix cell to
hardware `stable`.

Native-device tests are conditional on an actual adapter. A machine with
Metal framework support but no runtime `MTLDevice`, a machine built without
cgo, and non-macOS hosts exercise the structured unavailable-device path.
Those runs prove policy and compilation only; they are not hardware
execution evidence. Device-backed execution, broader lowering, kernel breadth,
differential fixtures, and matrix closure remain open for future implementation.

The deterministic fake-native adapter used by lifecycle tests models the
provider boundary without claiming a device: it supplies opaque buffer
handles, records allocation/release and provider-close ordering, and exposes a
small capability set. Its successful arithmetic result proves only that the
Go-side ABI, ownership, and cleanup protocol are wired correctly. It cannot
prove Metal/MSL compilation, device scheduling, synchronization behavior, or
hardware performance.
