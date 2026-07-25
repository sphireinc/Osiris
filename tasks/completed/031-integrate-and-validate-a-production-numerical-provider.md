# Task 031: Integrate and validate a production numerical provider

**Priority:** P1  
**Dependencies:** 028,030  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Replace or supplement the test provider with a documented production path for eig/eigh/SVD and related routines.

## Required work

- [x] Select provider strategy and licensing constraints.
- [x] Define ABI version negotiation and capability discovery.
- [x] Handle missing provider, symbol/version mismatch, provider errors, and deterministic diagnostics.
- [x] Add residual/orthogonality/reconstruction tests across representative matrices.

## Acceptance criteria

- [x] Build works with provider enabled and disabled.
- [x] Provider identity participates in native cache keys.
- [x] Numerical validation uses tolerances and difficult cases.

## Validation

- [x] provider integration tests
- [x] cache invalidation test
- [x] matrix residual suite

## Non-goals

- Do not vendor incompatible licensed binaries.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record — 2026-07-24

- Selected an explicit, non-vendored system-provider strategy. The production
  macOS adapter in `internal/osiris/providers/accelerate_lapack_provider.c`
  adapts Accelerate LAPACK to ABI v2; Linux deployments provide an equivalent
  adapter and explicit LAPACK/BLAS linker arguments.
- Added repeatable `-linalg-provider-flag` CLI arguments and
  `CCompileOptions.LinalgProviderFlags`. Provider source SHA-256 and all link
  flags participate in the content-addressed executable cache.
- Added capability/version-error coverage with an incompatible ABI fixture;
  absent providers remain compile-time diagnostics and unresolved symbols
  remain deterministic linker errors. No interpreter fallback exists.
- Added Accelerate end-to-end residual/orthogonality/reconstruction coverage
  for `eigh`, `svd`, and complex `eig`, plus cache invalidation tests for both
  provider source contents and provider link flags.
- Documented licensing, adapter selection, ABI capability discovery, and
  macOS/Linux build examples in `docs/design/NATIVE-LINALG-ABI.md`.
- Verified with:

  ```sh
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./internal/osiris -run 'TestCBackend(ProviderABIMismatchDiagnostic|AccelerateProviderResiduals|ExecutableCacheIncludesProvider(LinkFlags|SourceContent))$|TestCBackendProviderRequired$' -count=1
  GOCACHE=/private/tmp/osiris-bench-gocache GOMODCACHE=/private/tmp/osiris-bench-gomodcache go test ./...
  scripts/check-task-index.sh
  git diff --check
  ```
