# GPU execution contract

Osiris GPU artifacts describe executable kernels with a versioned value and
ownership ABI. `CompileGPU` validates IR and produces an artifact; it does not
evaluate the source and does not treat generated OpenCL text as an execution
result.

## Public symbols

- `CompileGPU(ctx, program)` emits and validates a bounded `GPUArtifact`.
- `ValidateGPUArtifact(artifact)` rejects malformed schema, ABI, source,
  kernel, and resource-limit data.
- `NewDeterministicCPUProvider(limits)` returns the portable reference provider
  used when no GPU device is available.
- `DefaultGPUCapabilityProfile()` documents the stable deterministic-f64-tensor
  language profile and its explicit host-only boundaries.
- `ValidateGPUProgram(program)` rejects mapped calls that cannot be represented
  by that profile before an artifact is emitted.
- `GPUProvider.SelectDevice`, `Allocate`, `Launch`, and `GPUEvent.Wait` define
  device negotiation, owned buffers, execution, and synchronization.

## ABI and ownership

The artifact schema and kernel/value ABIs are independently versioned. The
current portable value ABI is `f64` scalar or row-major tensor data. Input
buffers are `borrowed`; kernel outputs are `owned`. Providers copy input data
on launch and callers must release output buffers through `GPUBuffer.Release`.
Device capabilities include `float64`, `tensor`, and `deterministic`.

The deterministic CPU provider implements elementwise add/subtract/multiply/
divide, dot, matrix multiplication, vector and tensor constructors, ranges,
reshape/transpose/flatten, reductions, real deterministic math, and the pure
`std/signal` Hann and full-convolution kernels. These operations use the same
pure value-model contracts as the interpreter and VM reference tests, but the
provider executes the versioned GPU operation directly; it does not run source
through either evaluator. It checks cancellation during long kernels and
preserves deterministic kernel and result ordering. Hardware providers may
negotiate stricter limits but cannot weaken the artifact ABI or ownership rules.

The v1 value ABI is deliberately finite f64 scalar or row-major tensor data.
Complex, fixed-width integer, packed/bfloat, decimal/rational, bigint, future
numeric, structured Result/Option, module loading, pattern matching, tasks,
streams, and host I/O have explicit capability errors. Mapped pure calls from
`std/math` and `std/signal` are selected as kernel operations; an arbitrary
stdlib member is never silently omitted from the artifact.

## Limits and errors

`GPULimits` bounds elements, buffer bytes, rank, kernels, and source/artifact
size. `ErrGPUArtifactMalformed`, `ErrGPUCapabilityUnavailable`,
`ErrGPUResourceLimit`, `ErrGPUOwnership`, `ErrGPUExecution`, and
`ErrGPUCancelled` identify stable failure classes through `GPUError`.
Malformed artifacts, incompatible shapes, unsupported operations, ownership
violations, and cancellation are explicit errors; no interpreter or planner
fallback is permitted.
