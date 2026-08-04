# Task 166: Close GPU core and numeric matrix rows

**Priority:** P0
**Dependencies:** 159
**Status:** Blocked

## Objective

Close GPU rows for core language operations, typed values, numeric families,
tensor/matrix kernels, reductions, and linear algebra through the real device
provider.

## Acceptance criteria

- Real-device fixtures cover every stable cell, with explicit unavailable-device
  behavior and separately labeled deterministic reference fixtures.
- Kernel ABI, shape/dtype checks, synchronization, cancellation, and cleanup
  are verified under race-capable tests.
- No CPU reference execution is counted as hardware GPU evidence.
