# Task 303: Add a native GPU device provider

**Priority:** P0
**Dependencies:** 152,153
**Status:** Blocked — deferred behind Native C/LSP stabilization and device evidence

## Objective

Implement a real device-backed GPU provider instead of treating
`CPUReferenceProvider` as GPU execution. The provider must discover devices,
select capabilities, allocate device memory, and report an explicit backend
ABI for the host platform.

## Exact scope

- Own `GPUProvider` implementations, device discovery, capability negotiation,
  backend selection, native build/link configuration, and platform adapters.
- Select one supported host API first (Metal on macOS or an explicitly
  available Vulkan/CUDA path), with a versioned adapter boundary for others.
- Preserve `GPUBuffer` ownership, event synchronization, cancellation, limits,
  and release-on-error semantics.
- CPU reference execution remains available only when explicitly requested by
  `AllowCPUReference`; it must never be reported as a hardware GPU.

## Acceptance criteria

- A real device can be enumerated, selected, allocated, launched, synchronized,
  read, and released through the public provider interface.
- Device absence produces a structured capability error, not silent CPU
  fallback.
- Hardware-provider tests and deterministic fake-provider tests cover ownership
  and cancellation; docs describe platform prerequisites and exact capability
  fields.

## Current implementation evidence

- `internal/osiris/gpu_native_provider.go` now owns native device selection,
  provider lifetime, device-buffer references, launch events, readback, and
  close-after-last-buffer semantics.
- `internal/osiris/gpu_native_darwin.go` and
  `internal/osiris/gpu_metal_darwin.m` define the versioned macOS Metal
  adapter. It performs real MSL dispatch for the bounded elementwise and
  scalar-math surface and reports `native-device`/`float32` capabilities.
- `gpu_native_stub.go` returns `ErrGPUCapabilityUnavailable` on hosts without
  the adapter; it never constructs a CPU reference device.
- `gpu_backend_test.go` covers discovery policy, default native selection,
  deterministic adapter ownership/cancellation, ownership after release, and
  a conditional real-device execution fixture.

The current execution environment exposes Metal framework support but did not
return an `MTLDevice` to the test process. The conditional hardware fixture was
therefore skipped and no hardware result is being counted as evidence. Task
303 remains blocked until a device-backed CI or developer-host run proves
enumeration, allocation, dispatch, synchronization, readback, and release.

The environment-independent seam is covered by
`TestNativeGPUProviderLifecycleWithDeterministicAdapter` and
`TestNativeGPUProviderFakeCapabilityAndABIBoundaries`. These tests use opaque
fake handles and do not promote any support-matrix cell.
