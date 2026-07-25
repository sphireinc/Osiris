# Task 041: Create a release, packaging, and installation pipeline

**Priority:** P1  
**Dependencies:** 020,024,035,36  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Produce repeatable signed/versioned releases for supported platforms.

## Required work

- [x] Define semantic versioning and compatibility policy.
- [x] Embed version/commit/toolchain information.
- [x] Build platform archives and checksums.
- [x] Create install/uninstall documentation and smoke tests.
- [x] Decide policy for C compiler/provider prerequisites.

## Acceptance criteria

- [x] Release artifacts are reproducible enough to verify.
- [x] `osiris version` reports actionable provenance.
- [x] Fresh-machine smoke tests pass on supported platforms.

## Validation

- [x] release dry run
- [x] archive smoke tests

## Non-goals

- Do not publish from untrusted pull-request contexts.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`osiris version` reports version, commit, build date, Go toolchain, and target.
Release builds inject the first three fields using Go link flags; development
defaults remain explicit. `scripts/release-dry-run.sh` builds unsigned,
trimpath archives for darwin/arm64, darwin/amd64, linux/amd64, and
windows/amd64, then writes SHA-256 checksums. It never signs or publishes.

`scripts/test-release-archive.sh` verifies every checksum, extracts the host
archive, checks injected provenance, and runs an example. CI runs that smoke
test. `docs/development/RELEASES.md` documents SemVer, installation,
uninstallation, verification, and the policy that only native build/compile
needs a C compiler (with explicit providers for affected numerical features).
The searchable React Tooling reference documents `osiris version`.

Release smoke, React reference checks/build, full Go tests, task-index checks,
and diff checks passed.
