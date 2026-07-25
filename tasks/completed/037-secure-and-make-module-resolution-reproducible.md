# Task 037: Secure and make module resolution reproducible

**Priority:** P1  
**Dependencies:** 020,036  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Harden imports, dependency locking, cache integrity, and offline behavior.

## Required work

- [x] Define canonical module identity, version resolution, and checksum rules.
- [x] Prevent path traversal and ambiguous local/remote resolution.
- [x] Support deterministic offline builds from a populated cache.
- [x] Add lockfile lifecycle commands instead of manual editing.

## Acceptance criteria

- [x] Same lockfile/source yields same dependency graph.
- [x] Checksum mismatch and traversal attempts fail safely.
- [x] Offline mode has clear diagnostics.

## Validation

- [x] module resolver tests
- [x] malicious path fixtures
- [x] offline integration test

## Non-goals

- Do not add a network registry without a separate protocol/security design.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

Canonical local identity is cleaned absolute source path plus SHA-256 content
digest; stdlib identity is path plus resolved version. Relative imports cannot
escape their project root, absolute imports remain explicit, and no network
fallback exists. Offline mode (`OSIRIS_OFFLINE=1`) resolves only verified,
lockfile-backed cache entries. `osiris mod sync` creates/refreshes generated
state and `osiris mod clean` removes only generated state for a source project.

`docs/design/MODULE-RESOLUTION.md` and the searchable React Tooling entry
document the contract. Tests cover traversal rejection, cache checksums,
offline hit/miss behavior, and lifecycle cleanup. Full Go tests, task-index
checks, React reference checks/build, and diff checks passed.
