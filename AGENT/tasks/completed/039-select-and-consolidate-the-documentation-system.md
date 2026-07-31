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

## Source-of-truth clarification — 2026-07-25

- `newdocs/` is not merely the publishing shell: its checked-in React data,
  components, examples, and `src/support-matrix.json` are the public language
  reference source of truth.
- Removed the legacy static-reference import command so routine public-doc
  changes cannot be authored in `docs/` and copied forward. `docs/` is frozen
  compatibility material plus durable non-public design documentation.
- The Go support-matrix contract test and task index now name the React-owned
  matrix as canonical.

## Search coverage hardening — 2026-07-26

- `newdocs/scripts/check-reference.mjs` now verifies every published API entry
  is returned when queried by its own documented name, in addition to the
  existing representative cross-reference queries. This directly tests the
  same global-search index consumed by the React UI.
- Direct Node 24 reference checks, checked examples, and a production Vite
  build pass; the local `npm run` wrapper remains pinned to an obsolete Node
  18 executable, so direct Node invocation is the documented local workaround.

## Per-entry contract audit — 2026-07-27

- Replaced the combined `match / switch`, `readline / readchunk`, and
  `writechunk / flush / close` cards with seven independently searchable
  reference entries. Each states its signature, parameters, return type and
  observable return value, behavior notes, error conditions, and an example.
- The common React entry adapter now supplies a visible parameter contract for
  every migrated function entry. The reference check rejects function entries
  without parameters or behavioral detail, and searches an error-only phrase
  to prove the UI index includes error as well as signature, return, detail,
  and example metadata.
- Validation passed: `node newdocs/scripts/check-reference.mjs`,
  `node newdocs/scripts/check-examples.mjs`, direct Node Vite production build,
  `./scripts/check-docs.sh`, and `git diff --check`.

## Language-page data consolidation — 2026-07-27

- Moved the public language page’s callable and control-flow reference cards
  into `api-data.js` as canonical `languageSections`; the React view now
  renders those entries through the shared `ApiListing` component.
- Split Result/Option conversion, must-handle metadata, error redaction, and
  language constructs into 16 individual entries. They are all globally
  searchable and subject to the same signature, parameter, return-value,
  behavior, and example checks as module APIs.
- `language-reference.json` is now the shared React and Go-test source for
  those entries. `TestCanonicalDocumentationExamples` executes every complete
  language snippet with an `emit`, while retaining the explicit exclusion for
  I/O and legacy illustrative fragments that cannot run in isolation.

## Export-coverage and family-search audit — 2026-07-27

- `check-reference.mjs` now reads every current stdlib export table and fails
  if a public callable is absent from the canonical React reference. The check
  is based on source exports rather than a stale hand-maintained count.
- Detailed numeric-constructor family cards now expose their concrete members
  as searchable aliases. This makes `i32`, `u64`, `uint`, `byte`, `uf16`, and
  bfloat-family names discoverable while retaining one accurate shared
  signature, return contract, behavior description, and example per family.
- The checker verifies each alias through the same search index rendered by the
  React UI. Node reference checks, executable examples, and a Vite production
  build pass.
