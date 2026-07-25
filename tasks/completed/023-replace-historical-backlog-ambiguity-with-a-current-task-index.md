# Task 023: Replace historical backlog ambiguity with a current task index

**Priority:** P0  
**Dependencies:** 019,022  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Make `tasks/` navigable and prevent completed or obsolete roadmap items from being reimplemented.

## Required work

- [x] Classify existing tasks as complete, active, blocked, superseded, or historical.
- [x] Add owner/status/dependency metadata conventions.
- [x] Create a generated or manually maintained index ordered by current priority.
- [x] Move source roadmaps to an archive location only after preserving useful design prose.

## Acceptance criteria

- [x] Every task has one status and dependency state.
- [x] Completed work points to tests/commits/docs evidence.
- [x] Codex can select the next unblocked task without reading every file.

## Validation

- [x] task metadata lint

## Non-goals

- Do not rewrite task history or erase design context.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Added canonical `tasks/index.tsv` and human guide `tasks/INDEX.md`. Every
  task 001–048 has a path, status, owner, dependencies, and completion
  evidence, successor, or next gating condition.
- Added `scripts/check-task-index.sh`, run in CI, to reject malformed records,
  duplicate IDs, missing paths, unknown dependencies, and task files omitted
  from the index.
- Archived 46 old phase-breakdown files under `tasks/archive/roadmaps/` without
  changing their contents. The archive is explicitly design source rather than
  an implementation queue.
- Rewrote `tasks/README.md` and `tasks/PRIORITY.md` around the current index:
  task 024 is the next ready task once this task moves to completed.
- Validation passed: `./scripts/check-task-index.sh` and `git diff --check`.
