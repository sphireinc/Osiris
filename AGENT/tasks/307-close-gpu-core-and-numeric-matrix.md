# Task 307: Close GPU core and numeric matrix rows

**Priority:** P0
**Dependencies:** 304
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

## Current blocker

The native provider has real Metal lowering for elementwise and scalar math,
but the core numeric matrix still lacks device-backed fixtures for
constructors, ranges, shape transforms, reductions, dot/matmul, and linear
algebra. These rows remain experimental/unsupported as represented by the
existing matrix; deterministic CPU reference tests do not close them.

The current local progress is limited to bounded ABI and capability validation
through a deterministic fake-native provider. No constructor, reduction,
matrix, or linear-algebra row is promoted by these tests; real-device
execution remains required for matrix closure.
