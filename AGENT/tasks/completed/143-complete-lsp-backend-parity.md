# Task 143: Complete LSP and IDE parity with the language

**Priority:** P1
**Dependencies:** 139,140,141,142,125,136
**Status:** Complete

## Objective

Make the LSP a complete projection of parser, checker, formatter, interpreter,
VM, module, test, and backend semantics rather than a partial editor overlay.

## Exact scope and ownership

- Own `internal/osiris/lsp.go`, `internal/osiris/lsp_ide.go`, protocol fixtures,
  and LSP documentation.
- Cover diagnostics, incremental UTF-16 updates, document/workspace symbols,
  hover, definitions, type definitions, implementations, references,
  completion, signature help, rename, semantic tokens, inlay hints, folding,
  selection ranges, code actions, call/type hierarchy, formatting, code lens,
  test discovery, backend status, and capability diagnostics.
- Consume compiler/checker/formatter APIs; do not duplicate language rules.
- Surface stable support-matrix status and source-spanned backend rejection.
- Add bounded workspace, cancellation, malformed-request, and multi-file tests.

## Acceptance criteria

- LSP capabilities advertise only implemented, tested methods, and every
  advertised method has protocol fixtures and deterministic results.
- All parser/interpreter/VM/formatter conformance forms have useful diagnostics
  and symbol/navigation behavior.
- Colocated tests, modules, generics, traits, methods, aliases, and newtypes
  work across workspace boundaries.
- LSP support is `stable` in the final matrix.

## Parallelism

This task owns LSP files and can run concurrently with native, WASM, and GPU
implementation tasks after parser and formatter contracts are available.

## Completion evidence

- `internal/osiris/lsp.go` returns stable `-32602` invalid-parameter
  responses, validates required URIs, and continues after malformed requests.
- `internal/osiris/lsp_task143_test.go` and
  `internal/osiris/testdata/lsp/task143-protocol.jsonl` cover malformed
  requests, invalid UTF-16 positions, cancellation, bounded workspaces, and
  deterministic protocol responses.
- `docs/design/LSP.md` documents advertised methods, request shapes, results,
  cancellation, workspace limits, and source-spanned diagnostics.
- Focused LSP, language-server, Task 125, and race tests passed.
