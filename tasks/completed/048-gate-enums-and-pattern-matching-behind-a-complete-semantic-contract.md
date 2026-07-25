# Task 048: Gate enums and pattern matching behind a complete semantic contract

**Priority:** P2  
**Dependencies:** 022,025,026,032  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Implement the planned enum/pattern feature only after exhaustive semantics and backend obligations are defined.

## Required work

- [x] Finalize syntax, exhaustiveness, bindings, guards, nested patterns, errors, formatting, and value representation.
- [x] Define VM and native lowering strategy before merging parser-only support.
- [x] Add exhaustiveness/redundancy diagnostics with stable spans.
- [x] Update support matrix and docs.

## Acceptance criteria

- [x] Interpreter and VM are complete in one release unit.
- [x] Native support is complete or the feature is explicitly rejected by native compilation.
- [x] Formatter and LSP understand all forms.

## Validation

- [x] parser/formatter corpus
- [x] exhaustiveness tests
- [x] backend parity

## Non-goals

- Do not merge a syntax-only placeholder into the stable language.

## Completion evidence

- `GOCACHE=/private/tmp/osiris-gocache GOMODCACHE=/private/tmp/osiris-gomodcache go test ./internal/osiris`
- `scripts/check-task-index.sh`
- `cd newdocs && npm run check-reference && npm run build`
- Runtime coverage includes payload constructors and nested patterns, guards and
  defaults, grouped/type/relational/logical patterns, source-located
  exhaustiveness and redundancy diagnostics, interpreter/VM parity, formatter
  round trips, LSP enum-case navigation, and explicit native rejection for
  both unit and payload enums.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.
