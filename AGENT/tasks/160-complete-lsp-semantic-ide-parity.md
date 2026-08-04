# Task 160: Complete LSP semantic IDE parity

**Priority:** P1
**Dependencies:** 143
**Status:** Ready

## Objective

Complete semantic editor functionality for the full Osiris language surface:
completion, hover, signature help, definitions, references, document symbols,
workspace symbols, diagnostics, and semantic tokens.

## Exact scope

- Own LSP semantic indexes, source-span/UTF-16 conversion, completion,
  signature help, hover, definition/reference search, symbols, tokens, and
  diagnostics for functions, types, generics, traits, impl methods, modules,
  patterns, macros, and standard-library exports.
- Reuse compiler/parser/typechecker/module APIs; do not duplicate semantic
  rules inside the LSP.
- Add framed JSON-RPC fixtures for valid, malformed, incremental, and
  cancellation cases.

## Acceptance criteria

- Every advertised LSP method has protocol fixtures and exact response-shape
  documentation.
- Unsupported requests return explicit protocol errors; no AST form is silently
  ignored.
- UTF-16 positions, generated macro spans, module boundaries, and stale
  document versions are tested and race-safe.
