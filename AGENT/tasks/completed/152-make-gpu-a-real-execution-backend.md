# Task 152: Replace GPU planning with a real execution backend

**Priority:** P1
**Dependencies:** 139,140,141,036
**Status:** Complete

## Objective

Turn GPU artifact planning into a real, bounded execution backend with a
versioned kernel/value ABI and deterministic host fallback policy.

## Exact scope and ownership

- Own `internal/osiris/gpu.go`, new GPU runtime/provider files, GPU artifact
  tests, and GPU CLI integration.
- Define device selection, capability negotiation, memory ownership, kernel
  launch, synchronization, error transport, cancellation, and numerical
  tolerance contracts.
- Compile and execute only validated IR; a kernel comment or plan is not an
  execution result.
- Provide deterministic CPU reference comparisons and explicit diagnostics for
  device features unavailable under the selected profile.
- Add artifact schema, malformed input, resource-limit, and repeated-run tests.

## Matrix transitions

Establish the stable GPU runtime foundation required by later GPU language and
numeric tasks.

## Parallelism

This task owns GPU files and may run concurrently with native, WASM, formatter,
LSP, and REPL work.

## Completion evidence

- `gpu_abi.go`, `gpu_runtime.go`, and `gpu_cpu_provider.go` define a versioned
  artifact, provider negotiation, owned buffers/events, bounded launch limits,
  cancellation, and deterministic errors.
- The CPU reference provider executes the versioned kernel contract as an
  explicit provider, not a hidden interpreter fallback.
- `gpu_backend_test.go` covers artifact round trips, malformed artifacts,
  provider ownership/limits, cancellation, repeatability, and execution.
- `docs/design/GPU.md` documents the ABI, provider model, ownership, limits,
  cancellation, and unsupported capability profile.
