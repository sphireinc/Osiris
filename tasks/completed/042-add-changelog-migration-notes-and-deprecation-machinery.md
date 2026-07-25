# Task 042: Add changelog, migration notes, and deprecation machinery

**Priority:** P2  
**Dependencies:** 022,035,041  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Give users a predictable path through syntax, CLI, ABI, and format changes.

## Required work

- [x] Adopt changelog categories.
- [x] Define warning lifecycle for deprecated syntax/builtins/flags.
- [x] Add migration notes for breaking changes.
- [x] Tie support matrix stability levels to release policy.

## Acceptance criteria

- [x] Deprecations are testable and time-bounded.
- [x] Breaking releases have migration documentation.
- [x] Warnings have stable codes and suppression rules.

## Validation

- [x] deprecation warning tests

## Non-goals

- Do not retain deprecated behavior indefinitely without policy.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`docs/development/CHANGELOG.md` adopts Added/Changed/Deprecated/Removed/
Fixed/Security categories and maps stable, partial, experimental, and
unsupported support-matrix states to release compatibility promises.
`MIGRATIONS.md` is the required migration-note template for each breaking
release. Stable behavior is deprecated for at least one minor release and is
removed only in a major release with migration guidance.

`DeprecationWarning` supplies stable `OSIRIS-Dnnn` identifiers, a replacement,
an earliest removal major, deterministic rendering, and exact-code or `all`
suppression parsing. Its unit test covers the lifecycle contract and prevents
unrelated code suppression. The React Tooling reference exposes the policy and
suppression syntax. There are intentionally no active warnings in 0.1.0.

Full Go tests, React reference checks/build, task-index checks, and diff checks
passed.
