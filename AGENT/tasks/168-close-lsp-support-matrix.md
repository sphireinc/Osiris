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
has 23 partial LSP cells. Generic, macro, module, symbol, semantic-token,
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

## Current implementation evidence: declaration navigation

The standard `textDocument/declaration` capability is now advertised and
handled through `languageServer.capabilities`, `languageServer.handleIDERequest`,
and `languageServer.declarationContext`. It validates the required open
document, optional version assertion, and UTF-16 position before resolving the
analyzer-backed declaration. Source-backed declarations return the same
deterministic `Location` as definition navigation; generated, virtual, and
unresolved identities return `null` without fabricated ranges.

`TestLSPDeclarationNavigationMatchesSourceDefinition`,
`TestLSPDeclarationNavigationValidatesRequests`,
`TestLSPDeclarationNavigationCancellationIsTransactional`, and the declaration
request in `lsp-protocol.jsonl` provide normal, malformed, stale, UTF-16,
cancellation, and deterministic framed evidence. The exact request, result,
error, and cancellation contract is documented in
`docs/design/LSP-DECLARATION.md` and the granular generated reference entry.

This closes one advertised method boundary only; the aggregate LSP matrix
remains blocked until every advertised feature has complete evidence.

## Current implementation evidence: tooling and protocol cell

The `tooling-and-analysis` LSP cell is now independently closed. The
capability response and `TestLSPProtocolFixtureAdvertisesAndExercisesAllLSPMethods`
fixture cover every advertised request provider, deterministic framed output,
malformed request handling, and the exact initialize capability set. The
focused protocol, semantic-token, workspace-symbol, code-action, formatting,
diagnostic, hierarchy, and colocated-test fixtures add UTF-16, stale-version,
and cancellation evidence for the corresponding editor operations. The public
support matrix marks only this LSP cell stable; all language-feature and
stdlib-specific LSP cells remain partial until their construct-specific
semantic evidence is complete.

## Current implementation evidence: typed functions and static checking

The `typed-functions-and-static-checking` LSP cell is now independently closed.
The semantic analyzer and protocol handlers use the shared checked symbol
identity for typed parameters/results, generic type parameters, typed fields,
and trait methods. `textDocument/hover`, `textDocument/definition`,
`textDocument/references`, and `textDocument/completion` return the same
source-backed symbols, while `textDocument/signatureHelp` returns the checked
callable signature and active parameter. The evidence covers astral-Unicode
UTF-16 positions, malformed requests, stale versions, deterministic concurrent
responses, and cancellation before publication through
`TestLSPSemanticGenericTypeParametersNavigateWithUTF16AndSharedChecker`,
`TestLSPSemanticGenericTypeParametersFramedMalformedStaleAndCancellation`,
`TestLanguageServerTypedMembersCompletionNavigationAndUTF16`,
`TestLanguageServerTypedMemberProtocolUsesUTF16AndRejectsStaleVersion`,
`TestLanguageServerTypedMemberCompletionCancellation`,
`TestLanguageServerSignatureHelpProtocolUsesUTF16AndRejectsStaleVersion`,
`TestLanguageServerSignatureHelpValidatesMalformedRequestsAndCancellation`, and
`TestLanguageServerSignatureHelpIsDeterministicAndConcurrent`.

The public matrix promotes only this typed-function LSP cell. The other
construct-specific LSP rows remain partial until their own maintained fixtures,
source-position checks, cancellation behavior, and documentation are complete.

## Current implementation evidence: deterministic bindings and mutation

The `deterministic-state-and-mutation` LSP cell is now independently closed for
scalar bindings. The analyzer preserves the parsed distinction between
`constant binding`, `immutable binding`, and `mutable binding`, indexes
assignment references against the original definition, and reports
source-spanned diagnostics for assignments to `const` and `let`. The direct
analyzer test and deterministic framed protocol test cover declaration details,
valid mutable reassignment, immutable-target errors, repeated output, and the
shared UTF-16/stale/malformed/cancellation request boundary:
`TestLSPAnalyzerPreservesBindingMutabilityAndRejectsImmutableAssignments` and
`TestLSPBindingContractsProtocolIsDeterministicAndReportsImmutableAssignments`.

Aggregate rows involving native aggregate field/index mutation remain explicitly
partial in their Native C cells; this promotion concerns the LSP analysis and
diagnostic surface only.

## Current implementation evidence: patterns and matching

The `patterns-and-matching` LSP cell is now independently closed for the
supported language pattern surface. Enum and named-product/struct patterns use
the shared analyzer identities for type, case, field, and nested binding
navigation. Exact source spans are converted to UTF-16, unknown fields/cases
produce diagnostics at their pattern tokens, and the framed protocol fixture
proves deterministic definitions plus stale and malformed request rejection:
`TestLSPStructPatternNavigationAndNestedBindings`,
`TestLSPEnumPatternTypeNavigationUsesExactSourceSpan`, and
`TestLSPPatternProtocolFixtureIsDeterministicAndValidatesStaleRequests`.

This does not change Native C pattern status; that column still requires its
own generated-execution and ownership evidence.

## Current implementation evidence: modules, packages, exports, and macros

The `modules-packages-and-macros` LSP cell is now independently closed for
explicit export navigation. Source-backed user modules, standard-library
exports, verified repository-provider selectors, package/version metadata, and
hygienic macro declarations/parameters share source or virtual symbol identity
across completion, hover, definition, and references. The maintained framed
fixtures cover UTF-16 positions, stale and malformed requests, cancellation,
workspace boundaries, deterministic framing, and verified provider state via
`TestLSPSemanticUserModuleNavigationSharesIdentityAndUTF16`,
`TestLSPSemanticUserModuleNavigationRejectsStaleMalformedAndCancelled`,
`TestLSPSemanticUserModuleNavigationFramedFixtureIsDeterministic`,
`TestLSPRepositoryModuleNavigationUsesVerifiedProviderSource`, and
`TestLSPSemanticMacroNavigationFramedUTF16MalformedStaleAndCancellation`.

Unresolved, outside-root, capability-gated, and source-less virtual modules
remain explicit non-results rather than speculative navigation.

## Current implementation evidence: standard text exports

The `std-text` LSP cell is now independently closed. Completion derives the
entire exported member inventory from the live `std/text` runtime module rather
than a duplicated list, and the inventory test fails if any exported function or
constant disappears from editor results. Existing semantic and framed tests
cover the representative member's completion, hover, definition, references,
Unicode/UTF-16 positions, stale and malformed requests, cancellation, and
deterministic output; `TestLSPStdlibTextExportInventoryMatchesRuntimeModule`
covers every exported member.

The same runtime-derived inventory boundary now covers `std/math`,
`std/crypto`, `std/signal`, `std/list`, and `std/time` through
`TestLSPStdlibExportInventoriesMatchRuntimeModules`, closing those LSP module
cells without duplicating export lists in the analyzer. Existing representative
navigation tests continue to cover virtual identities, UTF-16 positions, stale
and malformed requests, cancellation, and deterministic framing.

## Current implementation evidence: types and numeric families

The `types-and-numeric-families` LSP cell is now independently closed for
source-backed type identity. The analyzer records exact references from struct
field annotations and ordinary typed function parameters/returns in addition
to generic type parameters; aliases and newtypes resolve to their declarations,
while numeric constructor expressions remain literal/value analysis rather than
false unresolved type symbols. `TestLSPSourceBackedTypeReferencesCoverAliasesNewtypesAndTypedSignatures`
checks the source spans and declaration identities. The framed
`TestLSPTypesAndNumericFamiliesProtocolIsDeterministicAndValidatesRequests`
fixture checks deterministic type-definition results, UTF-16 surrogate-split
rejection, malformed requests, stale versions, and pre-cancelled requests.

## Current implementation evidence: synchronous control flow and lexical bindings

The granular `lsp-core-synchronous-control-flow` row is now stable. The
analyzer records source-backed definitions and references for synchronous
function declarations and parameters, `const`/`let`/`var` bindings,
assignments, `while` and `for` scopes, `defer`, `return`, `break`, `continue`,
block expressions, and calls. Function parameters are included in the
document definition set as well as their private function scope, so hover,
definition, references, and protocol consumers share one identity.

`TestLSPCoreSynchronousControlFlowTracksLexicalTargets` checks that every
fixture definition and representative reference has a non-empty source span.
`TestLSPCoreSynchronousControlFlowProtocolIsDeterministicAndValidatesRequests`
checks framed UTF-16 definition output, repeatability, malformed JSON-RPC
parameters, stale versions, and cancellation. This promotion is deliberately
bounded: asynchronous tasks, streams, and host I/O remain partial in the
broader task/stream row.

## Current implementation evidence: document highlights

The granular `lsp-document-highlights` row is now stable. The implementation
symbols are `languageServer.documentHighlightsContext`,
`languageServer.documentHighlights`, `lspDocumentHighlightParams`,
`lspDocumentHighlight`, and the shared `documentAnalysis.findAt` identity
resolver. Definitions and assignments are emitted as `write` highlights;
resolved reads are emitted as `read` highlights. The method removes duplicate
ranges, sorts them deterministically, converts source spans to UTF-16, and
checks document versions and cancellation before publishing the complete
array.

`TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests`,
`TestLSPDocumentHighlightsAreDeterministicAndConcurrent`, and
`TestLSPDocumentHighlightsCancellationAndStaleVersionAreTransactional`, with
`lsp-document-highlight.jsonl`, cover exact wire fields, astral-character
positions, malformed/out-of-document/surrogate-splitting requests, stale
versions, concurrent repeatability, and pre-/mid-request cancellation. This
promotion is method-specific and does not promote the broader tooling or
control-flow rows.

## Current implementation evidence: standard-library host inventories

The LSP rows `lsp-stdlib-network-inventory` and `lsp-stdlib-os-inventory` are
now stable for export inventory and source-independent navigation. The shared
`stdlibModule`, `lspExportedModuleMembers`, `indexLSPImportedModules`, and
`resolveLSPImportedModules` path enumerates every live `std/net` and `std/os`
member for completion without opening a socket, process, file, signal, or
watch resource. The generic protocol tests continue to prove deterministic
ordering, virtual identities, UTF-16 positions, stale/malformed errors, and
cancellation.

`TestLSPStdlibExportInventoriesMatchRuntimeModules` now includes both modules
alongside the previously closed stdlib inventories. The promotion is strictly
editor-inventory behavior; Native C host execution remains in the aggregate
networking and OS rows and retains its platform/capability boundaries.

## Current implementation evidence: declaration navigation

The granular `lsp-declaration-navigation` row is now stable. `handleIDERequest`
routes `textDocument/declaration` through `declarationContext`, which shares
the source-backed `documentAnalysis.findAt` identity resolver with definition
navigation and preserves exact UTF-16 declaration ranges. The dedicated
`lsp-declaration.jsonl` fixture proves deterministic repeated responses,
missing/stale request rejection, and the exact declaration location after an
astral character; direct tests cover negative, out-of-document, and
surrogate-splitting positions plus transactional cancellation.

## Current implementation evidence: implementation navigation

The granular `lsp-implementation-navigation` row is now stable. The
`implementationsContext` resolver distinguishes callable functions from trait
and concrete implementation methods. Concrete methods are indexed with
`kind: "method"`; a request targeting a trait method therefore returns the
implementing method locations rather than the trait declaration. The resolver
keeps deterministic URI/source ordering, de-duplicates identities, and checks
cancellation throughout traversal and before publication.

`TestLSPImplementationProtocolFixtureIsDeterministicAndValidatesRequests`
provides dedicated framed evidence for deterministic UTF-16 locations,
malformed and stale requests, and exact concrete implementation selection.
`TestLSPTypeNavigationContextResultsAreDeterministicAndUTF16Safe`,
`TestLSPTypeNavigationCancellationIsTransactional`, and
`TestLSPTypeNavigationRequestValidationAndContextDispatch` add direct normal,
race, cancellation, surrogate-boundary, and validation coverage.

## Current implementation evidence: declaration-level symbol filtering

The document/workspace symbol providers now share the explicit
`isLSPDocumentOrWorkspaceSymbol` contract and omit function parameters from
top-level declaration results. The exact-shape direct test and
`lsp-workspace-symbols.jsonl` fixture prove the five-symbol result, UTF-16
coordinates, deterministic ordering, cancellation, stale-version rejection,
and consistent workspace filtering. This is granular evidence only; it does
not promote the broad LSP tooling row.

## Current implementation evidence: executable test results

The granular `lsp-test-results` row is now stable. The
`osiris/testResults` handler validates the open-document URI and optional
version, accepts only the supported `interpreter`, `vm`, and `native` backend
selectors, reuses shared colocated/default-root discovery, and executes with
the current open-document overlays. `lspTestResultFromResult` strips internal
error interfaces and evaluator values while preserving status, skip and
expected-failure flags, outputs, assertions, logs, UTF-16 source locations,
error text, and `durationNS`.

`TestLSPTestResultsProtocolFixtureIsDeterministicAndValidated` and
`TestLSPTestResultsExecutesOverlayWithStableFailureProjection`, with
`lsp-test-results.jsonl`, cover exact JSON-safe result fields, passing and
skipped declarations, overlay precedence, stale-version and unsupported
backend errors, and cancellation without partial publication. The fixture
comparison treats `durationNS` as non-deterministic runtime measurement; all
semantic result fields remain deterministic. This method-specific promotion
does not close the broader LSP workspace/refactoring matrix.

## Current implementation evidence: affected-document rename transactions

`languageServer.renameWorkspaceEditContext` now treats a multi-document rename
as one analyzed generation. It snapshots the merged open-overlay/workspace
index, uses that snapshot for all edit ranges and document versions, and
performs the locked `languageServer.validateRenameDocumentSnapshot` check for
every affected URI immediately before publication. A replaced importer or
source-backed user module therefore produces `-32602` with no partial edit;
unaffected documents may change without invalidating the result. Matrix
statuses remain unchanged.

The normal/stale framed fixture is
`internal/osiris/testdata/lsp/lsp-workspace-rename-transaction.jsonl`.
`TestLSPWorkspaceRenameTransactionProtocolFixtureIsDeterministic` verifies
repeatable wire output, `TestLSPWorkspaceRenameTransactionRejectsChangedAffectedDocument`
and `TestLSPWorkspaceRenameTransactionConcurrentMutation` cover deterministic
and concurrent stale affected-document replacement, and
`TestLSPWorkspaceRenameTransactionCancellationIsWireLevel` verifies
`-32800` cancellation without a result. Normal and race-detector focused
commands pass; this is granular refactoring evidence only and does not close
the remaining partial LSP cells or promote the aggregate matrix.

The strict validation slice now has explicit task evidence. The
`typeHierarchy/prepare` dispatcher requires a position and rejects malformed,
negative, out-of-document, and UTF-16-surrogate-splitting positions with
`-32602`. The `textDocument/formatting` dispatcher requires a non-empty
`textDocument.uri`, so missing and wrongly typed document parameters fail at
the protocol boundary rather than being misreported as document lookup
failures. Direct and framed tests cover deterministic valid responses,
malformed and stale requests, and cancellation without partial results.

The related navigation boundary tests apply strict UTF-16 validation to
`textDocument/prepareRename` and `callHierarchy/prepare`, including malformed,
negative, out-of-document, and surrogate-splitting positions. These are
method-level request-contract proofs only; they do not promote the aggregate
LSP column or close the still-partial semantic, workspace, refactoring,
repository-module, test-discovery, and host-backed rows.

## Pause checkpoint (2026-08-09)

Implementation is paused. The matrix currently has 25 stable and 23 partial
LSP cells. Remaining acceptance work is the complete advertised semantic and
workspace surface. The newly verified `prepareRename` and
`callHierarchy/prepare` paths now reject missing, malformed, out-of-document,
negative, and UTF-16-surrogate-split positions with deterministic `-32602`
responses and honor cancellation. Remaining work is broader code actions and
refactoring, repository/module edge cases, test discovery parity, client-side
edit transactions, strict validation for other advertised methods, and
complete cancellation, stale-version, malformed-request, and deterministic-
fixture evidence. The aggregate LSP status remains blocked and no cell was
promoted during this checkpoint.

Test-code-lens coverage now proves UTF-16 range conversion for colocated test
directives, astral-character handling, open-document overlays, stale versions,
deterministic repeated responses, and cancellation without partial results.
This is additional protocol evidence only; the aggregate remains 25 stable and
23 partial, with semantic, workspace, refactoring, repository, test-discovery,
and broader method-validation work still open.
