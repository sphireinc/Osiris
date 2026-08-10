# Task 403: Verify real GPU-device execution

**Priority:** P1
**Dependencies:** 303, 304, 307, 308
**Status:** Blocked by execution environment

## Objective

Produce device-backed evidence for the GPU provider and execution backend.
CPU/reference execution is not sufficient to promote GPU matrix cells.

## Scope

- Discover and identify a supported device and driver/API version.
- Exercise allocation, upload, kernel launch, synchronization, readback,
  cancellation, device loss, and release-on-error.
- Cover scalar/numeric, tensor, reduction, matrix/linear-algebra, signal, and
  mapped-library rows owned by tasks 304, 307, and 308.
- Record device, OS, architecture, driver/API, capability profile, and exact
  commands.

## Acceptance criteria

- Generated GPU artifacts execute on a real approved device.
- Device-backed results are compared against the interpreter and explicitly
  labeled CPUReferenceProvider results.
- Device failures are structured capability/platform diagnostics; CPU reference
  execution is never silently substituted.
- Evidence is attached to the owning GPU tasks and consumed by task 310.

## Current blocker

The current macOS host exposes no `MTLDevice`, so hardware GPU stability cannot
be claimed here. A supported GPU runner is required.

## Pause checkpoint (2026-08-09)

No real-device GPU evidence was added in this checkpoint. CPU/reference
provider results remain explicitly non-promoting evidence.
