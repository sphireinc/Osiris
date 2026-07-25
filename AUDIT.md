# Osiris repository audit

Audit date: 2026-07-24

## Executive summary

Osiris is far beyond a toy interpreter. It has a broad language surface, a VM,
substantial compiler analyses, a real C-lowering backend, scientific and complex
numeric work, an LSP, structured errors, concurrency primitives, documentation,
and extensive tests. The greatest risk is no longer feature scarcity; it is
maintaining coherent contracts across many execution paths while the native ABI
and language surface continue expanding.

The supplied snapshot contains intentional uncommitted work. The task pack does
not modify or overwrite that work. It prioritizes reproducibility, correctness,
ABI hardening, parity automation, release engineering, and consolidation before
large new feature areas.

## Observed risks

1. `go.mod` requests Go 1.26. The audit environment could not download that
   toolchain, so no test or vet run was possible. CI also resolves its version
   directly from `go.mod`; release reproducibility therefore depends on a
   toolchain that must be explicitly verified.
2. The worktree contains many modified and untracked files, including native
   linalg/complex lowering, docs, a generated binary, module lock/cache state,
   and a second documentation application. Baseline/reconciliation work should
   precede broad refactoring.
3. The implementation is concentrated in several very large files
   (`runtime.go`, `runtime_test.go`, `numeric.go`, `module.go`,
   `cbackend_native.go`, `optimize.go`). This increases review and merge risk.
4. The native backend has grown into a sizeable embedded C runtime. Ownership,
   cleanup, overflow, error, and provider ABI behavior need dedicated
   validation beyond functional parity.
5. Two documentation systems exist (`docs/` and `newdocs/`), while `node_modules`
   and built assets are present in the snapshot. A source-of-truth and generated
   artifact policy is needed.
6. Existing task files mix active work, historical roadmaps, and partially
   completed plans. A current status index should prevent Codex from repeating
   finished work.
7. CI currently runs only `go test ./...`. It lacks formatting, vet/static
   analysis, race, cross-platform, native compiler, docs, and sanitizer gates.
8. Public CLI and artifact formats need explicit stability/versioning rules
   before users depend on them.

## Recommended sequencing

- Wave 0: tasks 019-024 establish a trustworthy baseline and repository policy.
- Wave 1: tasks 025-034 harden the compiler/runtime/native ABI and parity.
- Wave 2: tasks 035-042 improve tooling, modules, LSP, docs, and releases.
- Wave 3: tasks 043-048 address performance, architecture, security, and future
  language evolution after the prior gates exist.

Do not run multiple tasks that touch `cbackend_native.go`, `runtime.go`, or
`module.go` concurrently unless each agent has an explicit file boundary.
