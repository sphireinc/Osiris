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

## Current implementation evidence

- `languageServer.signatureHelp`, `lspActiveCall`, `findLSPFunction`, and
  `lspSignatureParameters` now provide structured parameter labels, types,
  active-parameter selection, nested-call handling, quoted-string handling,
  UTF-16 positioning, and stale-version rejection.
- `TestLanguageServerSignatureHelpProtocolUsesUTF16AndRejectsStaleVersion`
  provides framed JSON-RPC evidence for that slice, including a race-enabled
  focused run.
- `TestTask160LSPVersionsCancellationAndTypeHierarchy` and the deterministic
  `task160-semantic.jsonl` fixture cover cancellation, stale versions, save
  revalidation, and type hierarchy behavior.
- `documentAnalysis.members`, `addLSPMember`, and
  `resolveCheckedMemberReferences` now index struct fields, trait methods, and
  `impl` methods by nominal owner after shared type checking. The
  `languageServer.completion`, `hover`, `definition`, and `references`
  symbols use that index for checked receivers such as `user.render`, while
  dynamic receivers retain the existing conservative fallback behavior.
- `TestLanguageServerTypedMembersCompletionNavigationAndUTF16`,
  `TestLanguageServerTypedMemberProtocolUsesUTF16AndRejectsStaleVersion`, and
  `TestLanguageServerTypedMemberCompletionCancellation` cover field/method
  completion, source-backed method navigation, framed protocol handling,
  astral UTF-16 positions, stale versions, and canceled requests. The focused
  normal and race runs both pass.
- `indexLSPImportedModules` now indexes resolvable `std/*` exports with stable
  module-member identities and virtual `osiris://stdlib/<module>` definition
  URIs. `TestTask160StdlibExportsNavigateWithUTF16AndIdentity`,
  `TestTask160StdlibExportsFramedUTF16AndStale`, and
  `TestTask160StdlibExportsFramedCancellation` cover completion, hover,
  definition, references, UTF-16 positions, stale versions, and cancellation.

This evidence is incremental. The task remains **Ready** and is not complete;
generic/macro/module navigation beyond resolvable standard-library exports,
symbols, semantic tokens, diagnostics, generated spans, and the remaining
advertised protocol/error fixtures still need full coverage before task 168
can close the LSP column. Support-matrix statuses were not changed.
