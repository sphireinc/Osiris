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

This evidence is incremental. The task remains **Blocked** and is not
complete; full multi-file rename, workspace/module parity, formatting and
test-tooling behavior, cancellation, root-boundary cases, and complete
documentation coverage remain open.
