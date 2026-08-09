# Task 168: Close LSP support-matrix rows

**Priority:** P1
**Dependencies:** 160,161
**Status:** Blocked

## Objective

Close the LSP column of the support matrix only after semantic IDE,
workspace, refactoring, formatting, and test-tooling behavior is complete.

## Acceptance criteria

- Every LSP-advertised feature has protocol fixtures, cancellation/race
  evidence, source-position tests, and granular documentation.
- The support-matrix test verifies that each stable LSP cell names an existing
  fixture and documentation route.
- Unsupported protocol methods remain explicit and are not marked stable.

## Current implementation evidence

Task 160 now has checked-receiver member completion, hover, definition, and
reference evidence in addition to structured signature help. Task 161 now has
versioned import organization, colocated test inventory/code lenses, open-file
overlay handling, cancellation checks, and lexical/symlink workspace-boundary
tests. The deterministic fixtures are maintained under
`internal/osiris/testdata/lsp/` and are validated by the task-168 evidence
test.

The pull-diagnostics slice is independently covered by
`languageServer.pullDiagnosticsContext`,
`TestLSPWorkspacePullDiagnosticsHaveExactShapeUTF16StaleCancellationAndDeterminism`,
and `lsp-workspace-diagnostics.jsonl`. The evidence proves the exact full-report
shape, defensive-copy behavior, deterministic ordering, UTF-16 conversion,
stale/malformed request errors, and cancellation handling. This is incremental
evidence only and does not promote the LSP matrix.

The inlay/folding/selection slice is independently covered by
`languageServer.inlayHintsContext`, `languageServer.foldingRangesContext`, and
`languageServer.selectionRangesContext`. The corresponding
`TestLSPWorkspaceIDEHintsFoldingAndSelectionHaveExactUTF16Shapes` test and
`lsp-workspace-ide-ranges.jsonl` fixture prove exact wire fields, checked type hints,
multiline and consecutive-comment folds, UTF-16 selection boundaries,
deterministic ordering, malformed and stale `-32602` errors, and cancellation
`-32800`. This is incremental evidence only and does not promote the LSP
matrix.

The source-backed rename slice is independently covered by
`languageServer.prepareRenameContext` and
`languageServer.renameWorkspaceEditContext`. It proves occurrence-local
prepare ranges, analyzer-identity filtering for locals, fields, methods,
generic and macro symbols, and module aliases, plus UTF-16 non-overlapping
edits, deterministic versioned document changes, malformed/stale errors, and
cancellation. Its focused normal/race tests and `lsp-workspace-rename.jsonl`
fixture are tracked without changing matrix statuses.

The user-module navigation slice adds source-backed imports for files resolved
inside configured workspace roots. `lspExportedModuleMembers` reads only
explicit `export` references, shared `symbolDef` pointers connect importer and
defining-document analyses, and `languageServer.references` returns a
deterministic cross-document set. The normal/race-oriented focused tests and
`lsp-semantic-user-modules.jsonl` fixture cover completion, hover, definition,
references, astral UTF-16 positions, stale and malformed protocol errors, and
framed determinism. This evidence does not promote the LSP matrix and does not
claim virtual stdlib, unresolved repository, unparseable, or out-of-root
modules.

The task remains **Blocked**: these are bounded slices, while the matrix still
has 28 partial LSP cells. Generic, macro, module, symbol, semantic-token,
diagnostic, full rename/refactoring, and complete advertised-protocol
coverage are still required before any LSP cell can be promoted.

The advertised call- and type-hierarchy boundaries now have explicit
malformed-request proofs: incoming/outgoing call requests and supertype/subtype
requests require non-empty `item.name` and `item.uri`, return `-32602` for an
absent/empty identity, and preserve their valid result paths. The framed
protocol fixture covers all four malformed item requests with normal and
race-focused execution. This closes only the hierarchy item-validation gap;
the remaining semantic, workspace, refactoring, and release-matrix gaps remain
open.

The call-hierarchy slice now also has identity-accurate execution semantics.
`callHierarchyDefinition` resolves a prepared definition by URI and stable
source identity; `incomingCallsContext` groups only references targeting that
definition by their containing callable; and `outgoingCallsContext` limits the
scan to the selected callable rather than the entire document. The methods
aggregate and sort UTF-16 ranges deterministically and return `-32800` when
cancellation occurs before or during traversal. The semantic and framed tests
`TestLSPCallHierarchyFiltersByDefinitionAndPreservesUTF16Ranges` and
`TestLSPCallHierarchyProtocolFixtureIsDeterministicAndExact` prove valid
results, malformed `-32602`, cancellation `-32800`, astral UTF-16 positions,
and deterministic wire output through `lsp-call-hierarchy.jsonl`. This closes
one locally verifiable LSP slice only; no support-matrix status is changed.

The code-action request path now has an explicit transactional boundary.
`languageServer.codeActionsAtVersionContext` validates the required UTF-16
range against the current document before scanning diagnostics or building
organize-import edits, propagates cancellation through the scan and edit
construction, and returns `-32800` without exposing partial actions. The
normal and race tests `TestLSPCodeActionValidatesUTF16RangesBeforeProducingEdits`,
`TestLSPCodeActionRangeValidationIsConcurrentAndDeterministic`,
`TestLSPCodeActionValidationProtocolFixtureIsDeterministic`, and
`TestLSPCodeActionCancellationIsWireLevelAndTransactional` cover malformed
ranges, deterministic framed output, and cancellation. The exact request,
range, edit, stale-version, and cancellation contract is documented in
`docs/design/LSP.md` and the generated LSP reference. This is additional
bounded evidence; the LSP aggregate remains blocked until every matrix row
has the complete advertised-feature and release-gate evidence.

The formatting path now has the same context-aware transaction boundary.
`languageServer.formattingAtVersionContext` checks cancellation before taking
the open-document snapshot and after the synchronous formatter has built the
full-document edit. `TestLSPFormattingCancellationIsTransactional` proves
that cancellation returns `-32800`, produces no partial edit, and leaves the
overlay unchanged. The individual request, version, result, and cancellation
contract is documented in `docs/design/LSP.md` and the
`textDocument/formatting` generated reference entry. This remains bounded
evidence and does not promote the LSP aggregate cell.

Type hierarchy traversal now also propagates request cancellation through
`prepareTypeHierarchyContext`, `typeSupertypesContext`, and
`typeSubtypesContext`. Indexed-document and statement scans check cancellation
before resolving implementation targets and after sorting, so canceled
requests return `-32800` without a partial hierarchy result.
`TestLSPTypeHierarchyCancellationIsTransactional` covers the direct context
boundary and the JSON-RPC prepare path. This is still incremental evidence;
the aggregate LSP support-matrix row remains blocked.

Signature-help requests now have the same explicit request boundary:
`handleIDERequest` routes through `signatureHelpContext`, which validates the
required UTF-16 position, stale document version, and cancellation before,
during, and after call scanning. Invalid requests return `-32602`; cancellation
returns `-32800` without a partial result. The normal and concurrent tests
`TestLanguageServerSignatureHelpValidatesMalformedRequestsAndCancellation` and
`TestLanguageServerSignatureHelpIsDeterministicAndConcurrent`, plus the
`lsp-signature-help.jsonl` protocol fixture, provide exact wire and race
evidence. This closes one advertised-method boundary only; the aggregate LSP
cell remains blocked until the remaining matrix-wide acceptance criteria are
complete.

The document/workspace symbol providers now also check cancellation after
collection and sorting, so a request canceled at the publication boundary
returns `-32800` rather than a partial array. `TestLSPSymbolRequestsCancelAfterCollectionWithoutPartialResults`
proves the boundary for both `textDocument/documentSymbol` and
`workspace/symbol`, including an empty-analysis path. The exact behavior is
documented in `docs/design/LSP-SYMBOL-REQUESTS.md` and the public LSP chapter.
This closes one cancellation boundary only and does not promote the aggregate
LSP matrix cell.

The core navigation request boundary now also covers
`textDocument/hover`, `textDocument/definition`,
`textDocument/completion`, and `textDocument/prepareRename`. Their context-aware
implementations check cancellation before and after semantic lookup and before
publishing the final result; completion checks during item collection. The
normal and race tests `TestLSPCoreNavigationContextResultsAreDeterministic`,
`TestLSPCoreNavigationCancellationDoesNotPublishPartialResults`, and
`TestLSPCoreNavigationWireDispatchHonorsCancellation` prove deterministic
results, exact `-32800` cancellation errors, and unchanged overlays. The
individual request and result contracts are documented in `docs/design/LSP.md`
and the generated API reference. This remains bounded evidence and does not
promote the LSP aggregate until every advertised feature is covered.

The semantic-token protocol contract is now independently closed at the
request boundary by `languageServer.semanticTokensContext`,
`scanSemanticTokensContext`, and `encodeSemanticTokensContext`. The handler
requires an open URI, rejects malformed and stale document assertions with
`-32602`, and validates every generated token's legend index, source-line span,
UTF-16 boundary, and deterministic source order before publication. Cancellation
is checked through lexing, sorting, delta encoding, and the final publication
boundary; canceled requests return `-32800` without a partial result.
`TestLSPWorkspaceSemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism`,
`TestLSPSemanticTokenEncodingHonorsCancellationBeforePublication`, and
`TestLSPSemanticTokenProtocolFixtureHasExactValidationContract` provide normal
and race-ready evidence, while
`internal/osiris/testdata/lsp/lsp-workspace-semantic-tokens.jsonl` records valid,
malformed, stale, and unknown-document framed requests. This is a disjoint
semantic-token slice only; task and support-matrix statuses remain unchanged.

The document lifecycle now restores disk-backed workspace state after
`textDocument/didClose` removes an overlay through
`languageServer.restoreClosedWorkspaceDocument`. The normal and race test
`TestLSPDidCloseRestoresCurrentDiskDocument` proves that the current saved
file replaces the pre-open workspace snapshot without retaining overlay text;
the exact bounded parse, cancellation, workspace-boundary, and deletion
behavior is documented in the LSP workspace file-watching section. This is
incremental lifecycle evidence only and does not promote the aggregate LSP
matrix cell.

The incremental document-sync boundary now has an explicit transactional
implementation in `languageServer.applyDocumentChangesContext`. Ordered
multi-change batches are analyzed privately, cancellation and stale/concurrent
snapshots are checked before publication, and malformed batches cannot mutate
the open overlay. `lsp_document_change_test.go` and
`lsp-document-changes.jsonl` provide normal/race and framed notification
evidence for this method. This closes only the document-sync boundary; the
aggregate LSP support-matrix cell remains partial.

Workspace indexing now has a private-generation publication boundary through
`languageServer.refreshWorkspace` and `relinkLSPDocumentsContext`. Cancellation
or a concurrent open-overlay update cannot replace the last complete workspace
index. `lsp_workspace_refresh_test.go` covers cancellation preservation and
overlay precedence in normal and race-focused runs. This closes only the
workspace-refresh transaction boundary and does not promote the aggregate LSP
cell.

The stdio request lifecycle now has a real `$/cancelRequest` boundary through
`languageServer.beginLSPRequest`, `languageServer.cancelLSPRequest`, and
`languageServer.handleLSPCancelRequest`. Request handlers run with registered
child contexts while the reader remains able to receive cancellation
notifications; response frames are serialized and released in request order.
`shutdown`, `exit`, input errors, and session cancellation drain active request
goroutines. `TestLSPCancelRequestCancelsRegisteredRequest` and
`TestLSPCancelRequestIsNotificationAndNormalRequestsRemainOrdered` provide
focused normal/race evidence for active-context cancellation, notification
semantics, and ordered normal responses. The exact parameters, no-response
rule, `-32800` result behavior, and lifecycle drain rules are documented in
`docs/design/LSP.md` and the granular `$/cancelRequest` reference entry. This
is additional bounded evidence; the aggregate LSP support-matrix cell remains
partial.

Workspace-level test tooling now has a deterministic metadata boundary as
well. `languageServer.workspaceTestInventoryContext` accepts a configured
workspace-directory URI, discovers `.test.osr` and `_test.osr` files under the
root with hidden-directory, symlink-entry, size, and count safeguards, and
uses open overlays while parsing declarations. `osiris/testInventory` routes
through the request context, so cancellation returns `-32800` without a
partial inventory. `TestLSPWorkspaceRootInventoryUsesDeterministicDefaultDiscovery`
proves module ownership, sorted nested files, overlay precedence, exclusion
rules, wire dispatch, and cancellation under normal and race execution. This
closes one local test-tooling boundary only; the LSP support-matrix cells
remain unchanged and the aggregate task remains open.

## Current implementation evidence: document highlights

The standard `textDocument/documentHighlight` boundary is now implemented by
`languageServer.handleIDERequest` and
`languageServer.documentHighlightsContext`. It strictly decodes required
`textDocument.uri`, optional overlay version, and position fields; resolves
the existing analyzer `symbolDef` identity; returns same-document definition,
assignment, and read occurrences with `write`/`read` kinds; converts parser
spans to UTF-16; removes duplicate ranges; and sorts results deterministically.
Cancellation is checked before lookup, during reference traversal, after
sorting, and before publication. The optional version is checked both before
collection and against the live document at the publication boundary.

`TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests`
and the `lsp-document-highlight.jsonl` fixture cover capability advertisement,
exact UTF-16 ranges after an astral scalar, malformed, stale, surrogate-
splitting, and out-of-document requests. The concurrent and transactional
tests cover normal/race determinism and cancellation without partial output.
The granular contract is documented in `docs/design/LSP-HIGHLIGHTS.md` and
the generated API reference. This closes one advertised method boundary only;
support-matrix statuses and the aggregate task remain unchanged.
