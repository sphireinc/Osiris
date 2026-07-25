# Task 024: Expand CI into a trustworthy validation pipeline

**Priority:** P0  
**Dependencies:** 020,021  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Add layered CI gates for correctness, tooling quality, native compilation, docs, and portability.

## Required work

- [x] Add fast formatting/test/vet checks.
- [x] Add race and bounded stress jobs.
- [x] Compile and execute representative native C programs with at least GCC and Clang where available.
- [x] Build documentation and validate examples/links.
- [x] Add Linux/macOS/Windows coverage according to supported features.

## Acceptance criteria

- [x] Required branch checks are documented.
- [x] Fast PR feedback remains reasonably bounded.
- [x] Native and documentation regressions fail CI.
- [x] Platform-specific exclusions are explicit.

## Validation

- [x] workflow syntax validation
- [x] intentional failure smoke tests

## Non-goals

- Do not add flaky wall-clock benchmarks as required PR gates.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Expanded CI into fast formatting, vet, unit-test, repository-contract, race,
  GCC/Clang native-executable, React-reference, and host-platform lanes.
- Added `docs/development/CI.md` documenting required checks and explicit
  exclusions: native execution is Linux-only until a Windows C toolchain
  contract exists, while Go tests run on Linux, macOS, and Windows.
- Added the Pages workflow for the React reference and
  `scripts/check-ci-workflows.sh`, which validates workflow YAML and required
  job/command/deployment contracts before CI runs.
- Validation passed: workflow contract check, YAML parsing, intentional broken
  native-job smoke failure, bounded race suite, GCC and Clang native parity
  suites, React reference check/build, `go vet ./...`, `go test ./...`, and
  `git diff --check`.
