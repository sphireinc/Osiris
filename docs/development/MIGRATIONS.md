# Migration notes

Breaking releases add a section here before publication. Each section names
the first affected version, explains the old and new behavior, provides a
before/after source or command example, and lists a rollback/compatibility
option where one exists.

## Deprecation warnings

Warnings use stable codes of the form `OSIRIS-Dnnn`. Their message names the
replacement and the earliest removal major version. Users may suppress a
specific warning with `OSIRIS_SUPPRESS_WARNINGS=OSIRIS-Dnnn` (comma-separated)
or all deprecation warnings with `OSIRIS_SUPPRESS_WARNINGS=all`. Suppression
does not retain removed behavior and should be temporary.

There are no active source or CLI deprecations in 0.1.0. This file is the
required destination for the first breaking migration.

## Unreleased: optional typed function signatures

Function parameters and results may now use the same colon-free annotations as
typed bindings:

```osiris
# Existing source remains valid and dynamic.
func render(value) => cat(value)

# Opt in to checked boundaries.
func render_id(value u32) string => cat(value)
let callback func(u32) string = render_id
```

No source rewrite is required. Unannotated positions retain their prior
runtime behavior. Once annotated, numeric widths and callable signatures are
exact; use an explicit constructor such as `u32(value)` when conversion is
intended.

## Unreleased: committed output and concurrent `emit`

Failed interpreter and VM runs previously returned no output even when an
earlier `emit` had completed. They now return and print that committed prefix
before reporting the failure. Consumers that assumed failed runs always had
empty standard output must read standard output and standard error
independently.

Output from concurrent bodies was previously captured and, depending on the
construct, merged after completion. `emit` is now rejected in `spawn`,
`parallel`, `race`, `select`, and `timeout` execution, including calls to
emitting functions. Return the task value and emit it from the synchronous
caller:

```osiris
# Before: no longer accepted.
let task = spawn({ emit compute(); 0 })
emit await(task)

# After.
let task = spawn({ compute() })
emit await(task)
```

There is no compatibility flag for concurrent output. This avoids shared
output races and leaves room for a future task-local capture and deterministic
merge contract.
