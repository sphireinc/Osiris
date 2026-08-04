# Task 162: Close native C core and data matrix rows

**Priority:** P0
**Dependencies:** 148
**Status:** Blocked

## Objective

Close only the native C matrix rows for core syntax/state, errors/results,
modules/patterns/tasks, numeric families, tensors, matrices, codecs, and
linear algebra. This task does not own native standard-library host I/O.

## Acceptance criteria

- Each owned matrix cell is backed by executable interpreter/VM/native
  differential fixtures, normal/race/sanitizer evidence, and exact docs.
- Unsupported shapes are source-spanned compile errors; no interpreter, VM, or
  shell fallback is allowed.
- The task updates only its owned native matrix cells after evidence is present.
