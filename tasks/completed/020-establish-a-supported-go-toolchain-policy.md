# Task 020: Establish a supported Go toolchain policy

**Priority:** P0  
**Dependencies:** 019  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Make local builds and CI reproducible by selecting and documenting a real, supported Go version strategy.

## Required work

- [x] Determine whether Go 1.26 is intentional, available, and required by source usage.
- [x] If it is not generally available, select the minimum supported released version and remove accidental future-version dependence.
- [x] Add a toolchain/version check with a clear error message.
- [x] Document upgrade policy and developer setup.

## Acceptance criteria

- [x] Fresh checkout builds without implicit access to an unavailable toolchain.
- [x] CI and local instructions use the same supported version.
- [x] Any version downgrade is backed by compilation/tests.

## Validation

- [x] `go version` policy test or script
- [x] `go test ./...` on minimum version
- [x] CI validation

## Non-goals

- Do not change language semantics.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

### Decision

Go 1.26 is intentional: the `go 1.26` directive was introduced by the public
release commit `4f9a63e` (2026-05-21), and this workspace provides the released
`go1.26.1 darwin/arm64` toolchain. No downgrade was made: there is no evidence
that a lower release line is the maintained contract, and a downgrade could
silently alter the standard-library/compiler baseline.

### Implementation

- Added `scripts/check-go-version.sh`, which accepts released Go 1.26.x only
  and emits a clear install/remediation error for every other state.
- `build.sh` runs that check before building.
- CI runs the same check immediately after `actions/setup-go` reads `go.mod`.
- Added `docs/development/GO-TOOLCHAIN.md` with setup and upgrade policy.
- No `toolchain` directive was added; builds do not request implicit downloads.

### Evidence

- `./scripts/check-go-version.sh`: passed with `go1.26.1`.
- `GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache ./build.sh`: passed.
- `GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...`: passed.
- `git diff --check`: passed.
