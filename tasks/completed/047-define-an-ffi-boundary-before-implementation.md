# Task 047: Define an FFI boundary before implementation

**Priority:** P2  
**Dependencies:** 028,036,046  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Create a safe, versioned design for C/Go interoperability without prematurely adding host calls.

## Required work

- [x] Specify type mapping, ownership, errors, callbacks, threading, determinism, and capability restrictions.
- [x] Define manifest/declaration format and ABI versioning.
- [x] Address dynamic library search and supply-chain risks.
- [x] Prototype only enough to validate the design.

## Acceptance criteria

- [x] Design covers all value families or explicitly excludes them.
- [x] Failure and cleanup paths are unambiguous.
- [x] Security review approves the loading model.

## Validation

- [x] design examples
- [x] prototype ABI tests if implemented

## Non-goals

- Do not expose arbitrary host symbols by default.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`FFI-BOUNDARY.md` establishes ABI v1 as a design-only capability-gated,
digest-pinned manifest contract. Exact scalar widths and explicit UTF-8/buffer
ownership are the only proposed direct value mappings; all aggregate, numeric
metadata, error/task/stream/module, callback, thread, and re-entrant behavior
is explicitly excluded pending a later ABI revision. Loading is allowlisted
absolute-path plus SHA-256 verification from trusted host configuration—never
arbitrary symbols, search paths, environment fallback, or network acquisition.

The React Tooling reference makes the unsupported status and future contract
searchable without presenting it as a callable feature. No host-call prototype
was added, by design. Full Go tests, React checks/build, task-index checks,
and diff checks passed.
