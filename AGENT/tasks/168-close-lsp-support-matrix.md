# Task 168: Close LSP support-matrix rows

**Priority:** P1
**Dependencies:** 160,161
**Status:** Blocked

## Objective

Close the LSP column of the support matrix only after semantic IDE,
workspace, refactoring, formatting, and test-tooling behavior is complete.

## Acceptance criteria

- Every LSP-advertised feature has protocol fixtures, cancellation/race
  evidence, source-position tests, and granular documentation.
- The support-matrix test verifies that each stable LSP cell names an existing
  fixture and documentation route.
- Unsupported protocol methods remain explicit and are not marked stable.
