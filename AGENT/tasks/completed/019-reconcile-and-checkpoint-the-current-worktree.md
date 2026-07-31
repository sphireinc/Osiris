# Task 019: Reconcile and checkpoint the current worktree

**Priority:** P0  
**Dependencies:** None  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Create a reviewed baseline for the supplied uncommitted native/compiler/docs work before new implementation begins.

## Required work

- [x] Inventory every modified/untracked path and classify it as source, generated artifact, cache, documentation, or intentional local-only file.
- [x] Run focused tests documented in `LEFTOFF.md` in an environment with the required Go toolchain.
- [x] Split unrelated changes into coherent commits or produce a written commit plan without discarding work.
- [x] Confirm whether the tracked `osiris` binary and `.osiris.lock` changes belong in version control.

## Acceptance criteria

- [x] A clean or intentionally documented worktree exists.
- [x] Every retained change has a test result or a documented reason it could not be tested.
- [x] No source work is lost and no cache is manually edited.

## Validation

- [x] `go test ./...`
- [x] focused native parity tests from `LEFTOFF.md`
- [x] `git diff --check`

## Non-goals

- Do not add new language features.
- Do not reformat the repository wholesale.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

### Toolchain and validation

- `go version`: `go1.26.1 darwin/arm64`; this satisfies the current `go.mod`
  requirement in this workspace.
- `GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...`: passed.
- Focused native validation passed:
  `TestCBackendComplexScalarParity`, `TestCBackendComplexCholeskyParity`,
  `TestCBackendEigProviderParity`, and
  `TestCBackendEigProviderComplexResidual`.
- `git diff --check`: passed.

### Worktree inventory

| Classification | Retained paths | Disposition |
| --- | --- | --- |
| Native/compiler source and tests | `internal/osiris/{alias,cbackend_native,infer,language_suite_test,linear,module,numeric,optimize,purity,runtime,runtime_test,tensor}.go`, `internal/osiris/testdata/linalg_provider_diagonal.c` | Intentional native-complex/provider lowering work. Covered by the validations above; checkpoint as one native ABI/lowering commit after review. |
| Native/docs/task records | `NATIVE.txt`, `docs/design/NATIVE-{COMPLEX-ABI,LINALG-ABI}.md`, `tasks/004-native-numeric-and-tensor-lowering.md`, `tasks/PRIORITY.md`, `tasks/README.md`, `LEFTOFF.md` | Intentional documentation and handoff work. Commit with the native lowering slice, except the handoff may remain local if the reviewer prefers. |
| Documentation source | `docs/design/DOCUMENTATION-STYLE.md`, `docs/design/ENUMS-AND-PATTERNS.md`, `tasks/018-enums-and-pattern-matching.md` | Intentional design/planning work. Separate documentation/design commit; no language implementation is implied. |
| Candidate React documentation source | `newdocs/{index.html,package.json,package-lock.json,vite.config.js,src/**}` | Intentional source, not generated output. Keep isolated until Task 039 chooses the canonical public documentation source. `newdocs/node_modules/` and `newdocs/dist/` are ignored. |
| Audit/task governance | `AGENTS.md`, `AUDIT.md`, `tasks/tasks/019-048` | Intentional audit task pack. Keep as the active task source; Task 023 will create the user-facing current index. |
| User-supplied visual assets | `logo_black_bg.png`, `logo_transparent.png` | Intentional local-only pending provenance/usage decision; do not stage or delete in this task. |
| Generated tracked artifacts | `osiris`, `internal/osiris/.osiris.lock` | Both are tracked in the current repository, but their present modifications are generated local state. Do not manually edit, stage, or discard them. Task 021 must set policy; the recommended policy is to exclude normal build/lock churn from source checkpoints unless explicitly released or reproducibly generated. |

### Coherent checkpoint plan

1. Native ABI/lowering: C runtime, interpreter/VM parity, provider fixture,
   native design documents, and the native task record.
2. Design-only: enum/pattern proposal and documentation-style contract.
3. Documentation application: `newdocs/` source and its package lock, only
   after Task 039 establishes whether it replaces `docs/`.
4. Governance: audit, root agent instructions, current task index, CI, and
   generated-artifact policy as the Wave 0 tasks complete.

No files were discarded or manually regenerated. The tracked binary and lock
file remain intentionally unstaged local backup state pending the explicit
generated-artifact policy task.
