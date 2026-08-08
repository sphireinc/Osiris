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

## Current implementation evidence

Task 160 now has checked-receiver member completion, hover, definition, and
reference evidence in addition to structured signature help. Task 161 now has
versioned import organization, colocated test inventory/code lenses, open-file
overlay handling, cancellation checks, and lexical/symlink workspace-boundary
tests. The deterministic fixtures are maintained under
`internal/osiris/testdata/lsp/` and are validated by the task-168 evidence
test.

The task remains **Blocked**: these are bounded slices, while the matrix still
has 28 partial LSP cells. Generic, macro, module, symbol, semantic-token,
diagnostic, full rename/refactoring, and complete advertised-protocol
coverage are still required before any LSP cell can be promoted.
