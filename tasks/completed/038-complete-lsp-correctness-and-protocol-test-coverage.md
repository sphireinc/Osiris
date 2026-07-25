# Task 038: Complete LSP correctness and protocol test coverage

**Priority:** P1  
**Dependencies:** 026,035  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Bring LSP behavior in line with the language and make it safe for real editors.

## Required work

- [x] Inventory implemented methods and missing capabilities.
- [x] Add fixtures for Unicode, malformed source, modules, macros, generated spans, and incomplete programs.
- [x] Ensure ranges use correct UTF-16 semantics.
- [x] Add cancellation and incremental document tests.

## Acceptance criteria

- [x] Capabilities accurately advertise support.
- [x] No panic on malformed or partial source.
- [x] Locations match parser/source-map truth.

## Validation

- [x] JSON-RPC fixture suite
- [x] Unicode range tests
- [x] cancellation test

## Non-goals

- Do not advertise rename/completion/etc. until implemented and tested.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`osiris lsp` now advertises only incremental text synchronization (`change: 2`),
hover, and definition. It intentionally does not advertise completion, rename,
or other unimplemented IDE methods. The LSP boundary converts the protocol's
zero-based UTF-16 positions to the lexer/source-map's one-based UTF-8 byte
columns, including diagnostic, hover, and source-backed definition ranges.

Incremental updates apply in order. Invalid ranges—including a split UTF-16
surrogate pair—are logged and leave the current document intact instead of
corrupting state or stopping the server. Partial source is retained with a
diagnostic and can recover on a later valid update. Builtins and other
generated definitions return no fabricated zero-span location.

`docs/design/LSP-CONTRACT.md`, the React Tooling reference, and both support
matrix copies document the public contract. Focused LSP tests, the full Go
suite, task-index validation, React reference checks/build, and diff checks
passed.
