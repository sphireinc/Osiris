# Task 159: Integrate GPU device execution and memory synchronization

**Priority:** P0
**Dependencies:** 158
**Status:** Blocked

## Objective

Connect GPU artifacts and language operations to the native device provider,
including kernel dispatch, mapped scalar/tensor values, asynchronous events,
limits, and deterministic error propagation.

## Exact scope

- Own `ExecuteGPU`, kernel ABI validation, device buffer transfers, event
  waits, cancellation, and provider-specific launch adapters.
- Implement scalar, tensor, reduction, signal, and mapped standard-library
  kernels claimed by the GPU artifact ABI.
- Verify shape, dtype, rank, element, and buffer-byte limits before launch and
  after readback.
- Ensure every input and output is released exactly once on success, failure,
  cancellation, and device loss.

## Acceptance criteria

- Interpreter/VM/GPU differential fixtures execute on a real device where the
  provider is available and fail deterministically otherwise.
- No GPU test calls `CPUReferenceProvider` unless it explicitly tests the
  opt-in reference mode.
- ABI, kernel layouts, synchronization, fallback policy, and ownership are
  documented per symbol in `docs/design/GPU.md`.
