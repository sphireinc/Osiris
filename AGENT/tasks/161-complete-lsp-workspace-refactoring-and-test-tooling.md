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
