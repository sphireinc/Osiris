# Task 158: Add a native GPU device provider

**Priority:** P0
**Dependencies:** 152,153
**Status:** Ready

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
