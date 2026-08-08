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
- `TestTask161OrganizeImportsProducesMinimalVersionedEdits` and
  `task161-import-actions.jsonl` provide unit and framed protocol evidence;
  stale document versions are rejected before edits are produced.
- Existing workspace inventory, versioned rename, formatting, and colocated
  test-discovery fixtures remain in `TestTask161VersionedWorkspaceEditsAndColocatedInventory`
  and `task161-workspace.jsonl`.
- `languageServer.codeLensesContext` now consumes the same
  `testInventoryContext` metadata as `osiris/testInventory`, so a production
  document resolves its deterministic colocated sidecar and emits run or
  benchmark commands from the sidecar overlay when it is open.
- `testInventoryContext` checks cancellation before and after discovery,
  prefers an open sidecar overlay over disk, resolves module ownership from
  the selected test file, and rejects a sidecar that escapes configured roots
  lexically or through symlink resolution via `workspacePathAllowed` and
  `pathWithin`.
- `TestTask161ColocatedInventoryDrivesCodeLensesAndUsesOverlay` and
  `TestTask161TestInventoryHonorsCancellationAndSymlinkRootBoundary` cover
  versioned open overlays, benchmark command selection, cancellation, and the
  symlink root boundary. `task161-test-inventory.jsonl` provides deterministic
  framed protocol coverage for both inventory and code lenses.
- Granular method documentation was added for `textDocument/codeLens` and
  `osiris/testInventory` in `docs/design/LSP.md` and `docs/src/api-data.js`.
- `textDocument/semanticTokens/full` now uses the repository lexer for stable
  keyword, operator, literal, comment, definition, and resolved-reference
  classification. Results are capped, sorted deterministically, encoded as
  the exact LSP delta array, and cancellation-aware through the full scan.
- `TestTask161SemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism`
  and `testdata/lsp/task161-semantic-tokens.jsonl` cover exact JSON shape,
  astral-Unicode UTF-16 lengths, comments versus strings, stale versions,
  cancellation, and repeated-response determinism.

The implementation passes `gofmt` and `git diff --check`. The semantic-token
slice is integrated with the standard-library semantic-index changes in
`lsp.go`, and the combined focused package tests pass with an explicit
writable Go cache.

Focused evidence for this slice:

```text
GOCACHE=/tmp/osiris-task161-cache go test ./internal/osiris -run 'TestTask161(ColocatedInventory|TestInventory|VersionedWorkspace|OrganizeImports)|TestTask160And161ProtocolFixturesAreDeterministic' -count=1
ok

GOCACHE=/tmp/osiris-task161-race-cache go test -race -vet=off ./internal/osiris -run 'TestTask161(ColocatedInventory|TestInventory|VersionedWorkspace|OrganizeImports)|TestTask160And161ProtocolFixturesAreDeterministic' -count=1
ok

GOCACHE=/tmp/osiris-task161-matrix-cache ./scripts/check-support-matrix.sh
passed
```

This evidence is incremental. The task remains **Blocked** and is not
complete; full multi-file rename, workspace/module parity, formatting and
test-tooling behavior, cancellation, root-boundary cases, and complete
documentation coverage remain open.
