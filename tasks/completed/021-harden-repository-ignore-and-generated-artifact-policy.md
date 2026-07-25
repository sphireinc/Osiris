# Task 021: Harden repository ignore and generated-artifact policy

**Priority:** P0  
**Dependencies:** 019  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Prevent caches, dependencies, local binaries, and generated docs from polluting commits.

## Required work

- [x] Audit `.gitignore`, tracked binaries, `.osiris` artifacts, `newdocs/node_modules`, `newdocs/dist`, benchmark scratch, and OS metadata.
- [x] Choose whether generated docs are committed or built in CI.
- [x] Add a repository hygiene check script suitable for CI.
- [x] Document exceptions.

## Acceptance criteria

- [x] A fresh install/build leaves only expected changes.
- [x] `node_modules` and local caches cannot be accidentally added.
- [x] Tracked generated artifacts have an explicit regeneration command.

## Validation

- [x] hygiene script test
- [x] fresh build status check

## Non-goals

- Do not delete intentional source assets without replacing their workflow.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

### Policy

- Authored `docs/` content remains tracked. Generated `newdocs/dist/` is never
  committed; a future publishing workflow builds it only after Task 039 selects
  the public documentation source.
- `newdocs/` source and its package lock may be tracked. `node_modules/`,
  `dist/`, `.osiris/`, locks, Go caches, coverage/profile output, temporary
  files, and OS metadata are local-only.
- The legacy tracked `osiris` binary and `internal/osiris/.osiris.lock` were
  removed from the Git index without deleting their working copies. `./build.sh`
  regenerates the binary; module/dependency operations regenerate lock/cache
  state. Neither should be hand-edited or recommitted as ordinary source.

### Implementation and evidence

- Expanded `.gitignore` and added `docs/development/GENERATED-ARTIFACTS.md`.
- Added executable `scripts/check-repository-hygiene.sh`; CI runs it after the
  shared Go-version check.
- `./scripts/check-repository-hygiene.sh`: passed before and after a build.
- `GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache ./build.sh`: passed.
- `GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...`: passed.
- `git diff --check`: passed.
