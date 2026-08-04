# Task 125: Complete the LSP and IDE experience

**Priority:** P2
**Dependencies:** 038, 110, 111, 112, 113, 123
**Status:** Complete

## Objective

Turn compiler analysis into production-grade editor support across workspaces,
types, modules, tests, and refactoring.

## Required LSP surface

- Context-aware completion for names, fields, methods, modules, types,
  patterns, imports, and named arguments where supported.
- Signature help with active parameter/type argument.
- Find references, implementations, definitions, type definitions, document
  symbols, and workspace symbols.
- Semantic tokens, inlay type/parameter hints, folding, selection ranges, and
  call/type hierarchy where protocol support is mature.
- Cross-file rename/refactoring with conflict analysis and formatted edits.
- Code actions/quick fixes for imports, type mismatches, exhaustive matches,
  deprecated APIs, and safe migrations.
- Test/benchmark discovery and run/debug metadata from task 123.

## Implementation requirements

- Use the shared parser/checker/module graph; do not reimplement semantics in
  protocol handlers.
- Support incremental UTF-16 edits, cancellation, stale-result suppression,
  workspace configuration, bounded indexing, and deterministic response order.
- Preserve privacy: no source/network telemetry without explicit user action.
- Add capability/version negotiation and stable malformed-request errors.

## Tests and documentation

- Protocol transcripts and multi-file workspace fixtures for every feature,
  including malformed messages, concurrent edits, cancellation, Unicode,
  dependency changes, and large-workspace limits.
- Editor-neutral setup/reference documentation and feature matrix.

## Acceptance criteria

- [ ] The listed navigation, completion, semantic, refactoring, and testing
      features work across files and modules.
- [ ] Responses are cancellation-aware, deterministic, UTF-16-correct, and
      bounded.
- [ ] Protocol fixtures, race/stress tests, docs, and full validation pass.

## Non-goals

- No editor-specific proprietary extension, cloud indexing service, or
  compiler semantics duplicated inside the LSP.

## Completion evidence

- Added bounded workspace indexing and shared compiler-analysis-backed
  completion, signatures, navigation, symbols, semantic tokens, inlay hints,
  folding, selection ranges, formatting, diagnostics, rename edits, call and
  type hierarchy, and language-test metadata.
- Added protocol negotiation, malformed-request diagnostics, UTF-16-aware
  document updates, cancellation/stale-result handling, deterministic edits,
  and multi-file workspace fixtures.
- Verification: focused LSP/task-125 tests, compile-only package tests, and
  focused race tests passed.
