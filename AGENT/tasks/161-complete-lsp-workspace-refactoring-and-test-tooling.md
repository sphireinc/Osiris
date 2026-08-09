# Task 161: Complete LSP workspace, refactoring, and test tooling

**Priority:** P1
**Dependencies:** 160,124,125,136
**Status:** Blocked

## Objective

Finish workspace-aware IDE behavior: rename, code actions, formatting,
workspace/file watching, repository modules, test discovery, and test metadata.

## Exact scope

- Own LSP workspace lifecycle, document synchronization, rename, code actions,
  formatting integration, repository module resolution, and colocated/default
  test-root discovery.
- Return deterministic edits with version checks and reject edits against stale
  documents.
- Expose test names, source locations, package/module ownership, and failure
  diagnostics to IDE clients.
- Add multi-file workspace fixtures, symlink/root-boundary cases, cancellation
  tests, and malformed-client requests.

## Acceptance criteria

- Rename and code-action edits are complete, non-overlapping, UTF-16-correct,
  and transactionally rejected when a document version is stale.
- Workspace and test-discovery results match CLI behavior exactly.
- All advertised features have granular protocol and documentation coverage.

## Current implementation evidence

- `languageServer.codeActionsAtVersion` now implements the bounded
  `source.organizeImports` action through `organizeImportEdits`.
- Leading imports are sorted deterministically by module path and alias,
  edits are minimal, non-overlapping, UTF-16-correct, emitted in descending
  source order, and carry the open document version.
- `TestLSPWorkspaceOrganizeImportsProducesMinimalVersionedEdits` and
  `lsp-workspace-import-actions.jsonl` provide unit and framed protocol evidence;
  stale document versions are rejected before edits are produced.
- Existing workspace inventory, versioned rename, formatting, and colocated
  test-discovery fixtures remain in `TestLSPWorkspaceVersionedWorkspaceEditsAndColocatedInventory`
  and `lsp-workspace.jsonl`.
- `languageServer.codeLensesContext` now consumes the same
  `testInventoryContext` metadata as `osiris/testInventory`, so a production
  document resolves its deterministic colocated sidecar and emits run or
  benchmark commands from the sidecar overlay when it is open.
- `testInventoryContext` checks cancellation before and after discovery,
  prefers an open sidecar overlay over disk, resolves module ownership from
  the selected test file, and rejects a sidecar that escapes configured roots
  lexically or through symlink resolution via `workspacePathAllowed` and
  `pathWithin`.
- `TestLSPWorkspaceColocatedInventoryDrivesCodeLensesAndUsesOverlay` and
  `TestLSPWorkspaceTestInventoryHonorsCancellationAndSymlinkRootBoundary` cover
  versioned open overlays, benchmark command selection, cancellation, and the
  symlink root boundary. `lsp-workspace-test-inventory.jsonl` provides deterministic
  framed protocol coverage for both inventory and code lenses.
- Granular method documentation was added for `textDocument/codeLens` and
  `osiris/testInventory` in `docs/design/LSP.md` and `docs/src/api-data.js`.
- `textDocument/semanticTokens/full` now uses the repository lexer for stable
  keyword, operator, literal, comment, definition, and resolved-reference
  classification. Results are capped, sorted deterministically, encoded as
  the exact LSP delta array, and cancellation-aware through the full scan.
- `TestLSPWorkspaceSemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism`,
  `TestLSPSemanticTokenEncodingHonorsCancellationBeforePublication`, and
  `TestLSPSemanticTokenProtocolFixtureHasExactValidationContract` cover exact
  JSON shape, astral-Unicode UTF-16 lengths, malformed and stale requests,
  cancellation during encoding, and repeated-response determinism in
  `testdata/lsp/lsp-workspace-semantic-tokens.jsonl`.
- `languageServer.pullDiagnosticsContext` now serves pull diagnostics as a
  context-aware, defensive snapshot. Items are sorted by UTF-16 source range
  and stable metadata; canceled requests cannot return a partial report.
- `TestLSPWorkspacePullDiagnosticsHaveExactShapeUTF16StaleCancellationAndDeterminism`
  and `lsp-workspace-diagnostics.jsonl` cover the exact `{kind, items}` response,
  required diagnostic fields, an astral-character UTF-16 range, repeated
  response equality, stale and malformed `-32602` errors, and cancellation
  `-32800`.
- `languageServer.inlayHintsContext`, `languageServer.foldingRangesContext`,
  and `languageServer.selectionRangesContext` now implement the advertised
  range-navigation methods with request-context cancellation checks. Inlay
  hints validate the required range, use checked type strings, emit the exact
  LSP fields, and sort by UTF-16 position and label. Folding ranges include
  multiline definition regions and consecutive comment blocks with stable
  ordering. Selection ranges validate UTF-16 boundaries and preserve request
  order while nesting identifier-or-point, line, and document ranges.
- `TestLSPWorkspaceIDEHintsFoldingAndSelectionHaveExactUTF16Shapes` covers exact
  response fields, astral-safe identifier ranges, comment/function folding,
  malformed and reversed parameters, stale versions, and pre/post-operation
  cancellation. `lsp-workspace-ide-ranges.jsonl` provides deterministic framed
  protocol coverage for all three methods.

Source-backed rename is now covered independently by
`languageServer.prepareRenameContext`,
`languageServer.renameWorkspaceEditContext`, and the identity helpers
`sourceBackedRenameTarget`, `languageServer.sourceBackedRenameOccurrence`,
`sameRenameDefinition`, and `renameSpanEdit`.
The implementation returns the occurrence range requested by
`prepareRename`, emits non-overlapping UTF-16 edits in descending source
order, deduplicates definition/reference spans, captures deterministic
versioned `documentChanges`, and checks cancellation before, during, and
after the workspace scan. The focused semantic test covers locals with
shadowing, struct fields, methods, generic parameters, macros, and imported
  module aliases. `lsp-workspace-rename.jsonl` provides framed prepare/rename,
malformed, stale-version, and deterministic response evidence.

The bounded cross-document user-module slice is implemented by
`TestLSPWorkspaceSourceBackedUserModuleRenameFromImporter` in
`internal/osiris/lsp_workspace_cross_document_rename_test.go`. An exported
member from a workspace-local `.osr` module can now be prepared and renamed
from an importer reference or from the indexed definition. The resulting
versioned `WorkspaceEdit` includes the module definition, its explicit
`export` occurrence, and the importer reference, while preserving UTF-16
ranges, URI ordering, open-document overlays, and source-span validation.
Virtual standard-library members, unresolved modules, out-of-root modules,
and user modules that cannot be parsed remain outside this slice.

Workspace module navigation now uses the same indexed document set as workspace
symbols and test tooling. `refreshWorkspace` parses bounded `.osr` files,
`resolveLSPImportedModules` accepts only readable local files inside configured
roots, and `relinkLSPDocumentsContext` performs a two-pass identity rebuild.
An open document overlays its on-disk module, while an unresolved repository
reference or out-of-root path remains absent from semantic results. The user
module fixture `lsp-semantic-user-modules.jsonl` is deterministic and exercises
framed completion, definition, references, stale versions, and malformed
requests; the companion Task 160 tests cover UTF-16 and cancellation.

The implementation passes `gofmt` and `git diff --check`. The semantic-token
slice is integrated with the standard-library semantic-index changes in
`lsp.go`, and the combined focused package tests pass with an explicit
writable Go cache.

Focused evidence for this slice:

```text
GOCACHE=/tmp/osiris-lsp-workspace-cache go test ./internal/osiris -run 'TestLSPWorkspace(ColocatedInventory|TestInventory|VersionedWorkspace|OrganizeImports)|TestLSPProtocolFixturesAreDeterministic|TestLSPSemanticToken' -count=1
ok

GOCACHE=/tmp/osiris-lsp-workspace-race-cache go test -race -vet=off ./internal/osiris -run 'TestLSPWorkspace(ColocatedInventory|TestInventory|VersionedWorkspace|OrganizeImports)|TestLSPProtocolFixturesAreDeterministic|TestLSPSemanticToken' -count=1
ok

GOCACHE=/tmp/osiris-lsp-workspace-matrix-cache ./scripts/check-support-matrix.sh
passed
```

This evidence is incremental. The task remains **Blocked** and is not
complete; rename for source-backed user modules is now bounded and locally
verified, but full multi-file refactoring, broader workspace/module parity,
formatting and test-tooling behavior, cancellation, root-boundary cases, and
complete documentation coverage remain open.

## Current implementation evidence: watched-file and repository-module slices

`workspace/didChangeWatchedFiles` now validates every URI and change type in a
notification batch before performing one bounded `refreshWorkspace` pass.
Created, changed, and deleted `.osr` files are reflected in the disk-backed
workspace index; open overlays remain authoritative, symlink entries remain
excluded, and no background watcher or operating-system handle is retained.
`TestLSPWorkspaceWatchedFilesRefreshesBoundedIndex` and
`TestLSPWatchedFileChangesRejectMalformedEntries` provide the semantic and
validation evidence, with the request shape recorded in
`internal/osiris/testdata/lsp/lsp-workspace-watch.jsonl` and documented
individually in `docs/design/LSP.md` and `docs/src/api-data.js`.

Repository imports now use the runtime's verified `RepositoryProvider` registry
when a matching provider and selector are available. The LSP passes request
cancellation through registry materialization, reuses verified lock/cache
source bytes, parses source without executing it, and indexes only explicit
exports under deterministic `osiris://repository/...` virtual URIs.
`TestLSPRepositoryModuleNavigationUsesVerifiedProviderSource` and
`lsp-workspace-repository.jsonl` cover selector propagation, completion,
definition identity, and lock-state creation. Provider failures, missing
offline cache entries, malformed source, and canceled resolution remain
explicit non-results.

These slices strengthen the task but do not yet close its full acceptance gate;
support-matrix statuses remain unchanged until the complete workspace,
refactoring, formatting, and test-tooling audit is finished.

## Current implementation evidence: code-action validation and cancellation

The `textDocument/codeAction` request path now requires a non-null UTF-16 range
and validates its endpoints against the current open overlay before producing
any action. `languageServer.codeActionsAtVersionContext` carries the request
context through diagnostic scanning and `organizeImportEditsContext`, then
checks it again after assembling the versioned edit. A canceled request
returns `-32800` without exposing a partial action; the existing synchronous
server transaction does not mutate document text.

`TestLSPCodeActionValidatesUTF16RangesBeforeProducingEdits`,
`TestLSPCodeActionRangeValidationIsConcurrentAndDeterministic`,
`TestLSPCodeActionValidationProtocolFixtureIsDeterministic`, and
`TestLSPCodeActionCancellationIsWireLevelAndTransactional` cover valid
organize-import output, missing/reversed/out-of-document ranges, concurrent
validation, deterministic framed responses, and cancellation before edit
publication. The fixture is
`internal/osiris/testdata/lsp/lsp-workspace-code-actions.jsonl`; granular
method behavior is documented in `docs/design/LSP.md` and `docs/src/api-data.js`.

Focused evidence:

```text
GOCACHE=/tmp/osiris-lsp-final-cache go test ./internal/osiris -run '^TestLSPCodeAction' -count=1 -timeout=120s
ok

GOCACHE=/tmp/osiris-lsp-final-race-cache go test -race -vet=off ./internal/osiris -run '^TestLSPCodeAction' -count=1 -timeout=180s
ok

node --check docs/src/api-data.js
git diff --check
passed
```

The stdio request lifecycle now implements real per-request `$/cancelRequest`
handling. `languageServer.beginLSPRequest` registers a child context by the
JSON-RPC request id, `handleLSPCancelRequest` cancels the matching active entry
without producing a response, and `dispatchLSPRequest` runs the context-aware
request handlers while the reader remains able to receive cancellation
notifications. Response writes are serialized and released in request order;
`shutdown`, `exit`, input errors, and session cancellation drain active request
goroutines before returning. Unknown cancellation ids are ignored and
malformed notifications are logged.

`TestLSPCancelRequestCancelsRegisteredRequest` proves cancellation reaches an
active request context through a framed notification, while
`TestLSPCancelRequestIsNotificationAndNormalRequestsRemainOrdered` proves that
the notification has no response and normal request/shutdown responses retain
their order. These tests are run normally and under the race detector. The
wire contract is documented granularly in `docs/design/LSP.md` and the
`$/cancelRequest` reference entry in `docs/src/api-data.js`.

## Current implementation evidence: formatting cancellation boundary

`languageServer.formattingAtVersionContext` now receives the request context
from `handleIDERequest`, validates cancellation before reading the document and
after the complete `FormatSource` edit is assembled, and returns `-32800`
without mutating the open overlay or exposing a partial result. The legacy
`formattingAtVersion` helper remains a background-context wrapper for internal
callers. `TestLSPFormattingCancellationIsTransactional` covers the canceled
request, exact error code, and unchanged document state. The synchronous
formatter remains intentionally non-preemptive during its pure formatting pass;
the post-pass check is the transaction boundary. This is a bounded slice and
does not close the broader task or promote the support matrix.

## Current implementation evidence: core navigation cancellation boundary

`languageServer.hoverContext`, `languageServer.definitionContext`, and
`languageServer.completionContext` now receive the request context from the
stdio dispatcher. Each checks cancellation before document lookup, after
semantic lookup, during collection where applicable, and immediately before
publication. The existing `prepareRenameContext` path remains transactional
and is covered by the same wire-level cancellation contract.

`TestLSPCoreNavigationContextResultsAreDeterministic`,
`TestLSPCoreNavigationCancellationDoesNotPublishPartialResults`, and
`TestLSPCoreNavigationWireDispatchHonorsCancellation` cover deterministic
normal results, `-32800` cancellation responses, malformed publication
boundaries, and unchanged overlays. The exact method parameters, result
shapes, and cancellation behavior are documented in `docs/design/LSP.md` and
the generated API reference. This is incremental evidence; the workspace,
refactoring, and complete advertised-protocol acceptance criteria remain open.

## Current implementation evidence: signature-help request boundary

`textDocument/signatureHelp` now uses a context-aware path from
`handleIDERequest`. It requires a position, validates UTF-16 line and
character boundaries against the open document, rejects stale versions with
`-32602`, checks cancellation before and during call scanning and after the
result is assembled, and returns `-32800` without a partial signature array.
The synchronous `signatureHelp` wrapper remains available to internal callers.

`TestLanguageServerSignatureHelpValidatesMalformedRequestsAndCancellation`
and `TestLanguageServerSignatureHelpIsDeterministicAndConcurrent` cover the
transaction boundary and repeated concurrent output. The framed fixture
`internal/osiris/testdata/lsp/lsp-signature-help.jsonl`, validated by
`TestLPSSignatureHelpFixtureIsDeterministicAndValidatesRequests`, covers valid
UTF-16 positions, malformed parameters, stale versions, and deterministic wire
responses. Granular parameter, result, stale-version, and cancellation
documentation is in `docs/design/LSP.md` and the generated API reference.

## Current implementation evidence: final symbol-request cancellation boundary

`textDocument/documentSymbol` and `workspace/symbol` now perform a final
request-context check after collection and deterministic sorting. Cancellation
at that publication boundary returns `-32800` with no partial symbol array;
empty analyses take the same path. The focused
`TestLSPSymbolRequestsCancelAfterCollectionWithoutPartialResults` fixture
drives both wire methods and cancels exactly at the post-collection boundary.
The contract is documented in `docs/design/LSP-SYMBOL-REQUESTS.md` and the
workspace-symbol section of `docs/design/LSP.md`. This is incremental evidence
only; the broader workspace/refactoring acceptance gate remains open.

## Current implementation evidence: closed-overlay restoration

`languageServer.restoreClosedWorkspaceDocument` now restores the current
disk-backed `.osr` document after `textDocument/didClose` removes an editor
overlay. It validates the workspace boundary and file size, builds parse and
analysis state privately, checks cancellation before publication, and removes
deleted/out-of-root entries instead of retaining stale snapshots.
`TestLSPDidCloseRestoresCurrentDiskDocument` verifies that a changed on-disk
file replaces the old overlay through the lifecycle boundary; the existing
watched-file refresh test and normal/race focused runs cover the adjacent
workspace-index behavior. The public contract is documented in the LSP
workspace file-watching section. This is a bounded lifecycle slice and does
not by itself close the task's full workspace/refactoring/test-tooling gate.

## Current implementation evidence: transactional incremental changes

`languageServer.applyDocumentChangesContext` now applies a complete
`textDocument/didChange` batch to private text in order, checks cancellation
between changes, rejects stale versions and concurrent snapshots at commit,
and publishes only a fully analyzed replacement overlay. Invalid ranges,
cancellation, or concurrent updates leave the prior document unchanged while
the notification remains response-free. The focused normal/race tests in
`lsp_document_change_test.go` and the `lsp-document-changes.jsonl` fixture
cover ordered multi-change application, UTF-16 ranges, malformed batches,
stale versions, cancellation, and exact notification behavior. This is an
incremental lifecycle/refactoring slice; the broader task gate remains open.

## Current implementation evidence: transactional workspace refresh

`languageServer.refreshWorkspace` and `relinkLSPDocumentsContext` now build
private workspace and analysis generations, resolve modules through a private
server view, and publish only after traversal, analysis, and cancellation
checks complete. A canceled or stale concurrent refresh preserves the previous
index and open overlays. `TestLSPWorkspaceRefreshCancellationPreservesIndex`
and `TestLSPWorkspaceRefreshConcurrentOverlayWins` provide focused normal and
race evidence for those publication boundaries. This is an incremental
workspace lifecycle slice; the broader task gate remains open.

## Current implementation evidence: workspace-root test discovery metadata

`languageServer.testInventoryContext` now accepts a configured workspace
directory URI in addition to an open source or test document. The new
`languageServer.workspaceTestInventoryContext` path shares the language-test
parser with the CLI, recursively discovers both `.test.osr` and `_test.osr`
files in deterministic absolute-path order, applies the existing workspace
file and per-file-size bounds, skips hidden/metadata directories, excludes
symlink entries, and uses an open test overlay instead of disk text. It reads
no test body for execution and preserves the existing module metadata field
when the requested root has an `osiris.mod` manifest.

The `osiris/testInventory` wire handler now passes its request context through
to the context-aware inventory path. A canceled root traversal returns
`-32800` before partial metadata is published; a root outside configured
workspace roots is rejected without reading it. The focused
`TestLSPWorkspaceRootInventoryUsesDeterministicDefaultDiscovery` test covers
module metadata, sorted nested discovery, overlay precedence, hidden and
symlink exclusion, wire dispatch, and cancellation. The request and response
fields are documented individually in `docs/design/LSP.md` and the
`osiris/testInventory` reference entry in `docs/src/api-data.js`.

This is bounded workspace/test-tooling evidence only; it does not close the
remaining refactoring, repository, or complete LSP acceptance gate.

Focused verification for this slice:

```text
GOCACHE=/tmp/osiris-lsp-root-inventory-cache go test ./internal/osiris -run '^TestLSPWorkspaceRootInventoryUsesDeterministicDefaultDiscovery$' -count=1 -timeout=300s
ok

GOCACHE=/tmp/osiris-lsp-root-inventory-race-cache go test -race -vet=off ./internal/osiris -run '^TestLSPWorkspaceRootInventoryUsesDeterministicDefaultDiscovery$' -count=1 -timeout=600s
ok
```

## Current implementation evidence: document highlights

`languageServer.handleIDERequest` now dispatches the standard
`textDocument/documentHighlight` request, while
`languageServer.documentHighlightsContext` resolves the shared analyzer
identity, limits results to the requested document, emits read/write kinds,
deduplicates and sorts UTF-16 ranges, and rechecks cancellation and the live
document version before publication. `lspDocumentHighlightParams` preserves
the distinction between missing required fields and valid zero-based
coordinates; `lspDocumentHighlight` is the exact wire result shape.

`TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests`,
`TestLSPDocumentHighlightsAreDeterministicAndConcurrent`, and
`TestLSPDocumentHighlightsCancellationAndStaleVersionAreTransactional` cover
capability advertisement, malformed and stale requests, astral Unicode
conversion, read/write classification, deterministic concurrent results, and
early/late cancellation. The framed cases are in
`internal/osiris/testdata/lsp/lsp-document-highlight.jsonl`; the granular
request/result contract is in `docs/design/LSP-HIGHLIGHTS.md` and the
`textDocument/documentHighlight` API reference entry.

This is a bounded IDE capability slice only; task and support-matrix statuses
remain unchanged.
