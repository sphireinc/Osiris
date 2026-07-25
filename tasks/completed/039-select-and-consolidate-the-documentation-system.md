# Task 039: Select and consolidate the documentation system

**Priority:** P0  
**Dependencies:** 021,022  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Choose `docs/` or `newdocs/` as the public source of truth and create a maintainable publishing flow.

## Required work

- [x] Compare accessibility, search, build complexity, offline use, and GitHub Pages deployment; the user selected the componentized React/Vite site in `newdocs/` as the public source.
- [x] Migrate unique content from the losing implementation.
- [x] Remove checked-in dependency/build noise according to policy.
- [x] Automate navigation/content index generation.

## Acceptance criteria

- [x] One canonical source is documented.
- [x] All existing public pages have redirects or migrated equivalents.
- [x] Docs build deterministically in CI.

## Validation

- [x] docs build
- [x] link checker
- [x] navigation test

## Non-goals

- Do not redesign branding and rewrite all prose in the same task.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Chose `newdocs/` as the public source of truth and documented its React
  component/data model in `newdocs/README.md`.
- Migrated the static manual into checked-in `legacy-reference.json`: 10 API
  groups, five prose chapters, types, and 163 API entries retaining signature,
  description, return type/value notes, details, and examples. The support
  matrix is also a React route backed by checked-in JSON.
- Added global search over API names, signatures, descriptions, details,
  return metadata, and examples. Hash routes make every navigation item and
  compatibility URL addressable.
- Added generated compatibility redirects for all 17 former `docs/*.html`
  URLs, plus a GitHub Pages workflow that builds and publishes `newdocs/dist`.
- Added `npm run check-reference`, invoked by CI, to validate migrated entry
  metadata, representative search queries, support-matrix presence, navigation
  coverage, and all legacy redirects.
- Validation passed: `npm run migrate-reference`, `npm run check-reference`,
  `npm run build`, `./scripts/check-docs.sh`, and `git diff --check`.
