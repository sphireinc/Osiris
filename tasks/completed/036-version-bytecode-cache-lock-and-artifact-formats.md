# Task 036: Version bytecode, cache, lock, and artifact formats

**Priority:** P1  
**Dependencies:** 019,035  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Prevent opaque incompatibility as the compiler evolves.

## Required work

- [x] Inventory serialized/internal formats.
- [x] Add magic/version fields and compatibility errors where absent.
- [x] Define invalidation rules tied to compiler, ABI, provider, flags, and source dependencies.
- [x] Add corruption and downgrade/upgrade tests.

## Acceptance criteria

- [x] Old/incompatible artifacts fail clearly rather than misexecute.
- [x] Cache keys cover semantic inputs.
- [x] Format versions are documented.

## Validation

- [x] corruption fixtures
- [x] cache key tests
- [x] compatibility tests

## Non-goals

- Do not promise long-term stability for formats explicitly marked internal.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

Documented the serialized/internal-format inventory in
`docs/design/ARTIFACT-FORMATS.md`. VM bytecode, IR, SSA, CFG, and analysis
reports are explicitly in-memory or diagnostic-only and make no persistence
promise. The formats that cross a cache/artifact boundary now carry or verify
versions:

- native executable cache manifests use `format_version: 1` and must exactly
  match source, provider source hash, compiler path/identity, flags, and native
  OS/architecture target before a cache hit is reused;
- generated `.osiris.lock` writes format version 2, upgrades the known
  lossless v1 predecessor, and rejects all other versions; cached module source
  is SHA-256 checked against its lock entry before fallback use;
- GPU plan JSON emits `schema_version: 1`;
- structured-error encoding and standard WASM versioning were inventoried as
  existing versioned formats.

Missing, corrupt, old, or semantically mismatched native manifests now fail
clearly rather than copying an unchecked executable. This is intentionally
strict: generated lock/cache state has no silent upgrade path.

Evidence:

- `TestCCompileManifestRejectsCorruptAndIncompatibleFormats` covers malformed
  and old native-manifest input.
- `TestModuleLockUpgradesV1AndRejectsUnknownFormat` and
  `TestModuleRegistryRejectsCorruptCachedSource` cover lock downgrade and
  cached-module corruption.
- `TestGPUArtifactJSONIsVersioned` checks the public GPU JSON envelope.
- Existing cache tests prove generated source, provider source contents, and
  provider link flags affect native executable keys.
- `go test ./...`, `npm run check-reference`, `npm run build`,
  `scripts/check-task-index.sh`, and `git diff --check` passed.
