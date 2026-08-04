# Task 124: Build repository-oriented modules, workspaces, and supply-chain tooling

**Priority:** P2
**Dependencies:** 036, 037, 041, 046
**Status:** Complete

## Objective

Turn existing module/version/lock/cache mechanics into a Go-style,
repository-oriented package system. A module must be usable directly from its
source repository without registration in a central Osiris registry. GitHub
is the default documented host and first complete provider, not a mandatory
language-level monopoly.

## Module identity and resolution

- Use repository-shaped canonical module paths and package import paths:

  ```text
  github.com/acme/orbit
  github.com/acme/orbit/telemetry
  ```

- Resolve `github.com/<owner>/<repository>` directly from GitHub in the first
  complete provider. Define a provider interface and repository metadata
  discovery mechanism so other Git hosts and controlled vanity paths can be
  added without changing import syntax or module identity.
- Treat semantic-version Git tags such as `v1.2.3` as released versions.
  GitHub Releases are presentation metadata; their associated immutable Git
  tag and commit are authoritative. Define major-version path behavior.
- Permit `@latest`, exact semantic versions, compatible version selectors, and
  explicit commits. Untagged commits receive a deterministic pseudo-version.
- Record the resolved module path, selector, version/tag, full commit SHA,
  canonical source-tree digest, dependency graph, and provider identity in
  generated lock state.
- Define deterministic version selection and conflict behavior, including a
  migration from the existing exact/caret/tilde resolver where necessary.

## CLI and project files

- Add a human-authored, versioned module manifest distinct from generated
  `.osiris.lock` and `.osiris/cache` state. Update generated-file and commit
  policy deliberately rather than silently repurposing either format.
- Implement and document:

  ```text
  osiris mod init github.com/acme/orbit
  osiris get github.com/osiris-labs/tensors
  osiris get github.com/osiris-labs/tensors@v1.4.2
  osiris get github.com/osiris-labs/tensors@latest
  osiris mod tidy
  osiris mod download
  osiris mod verify
  ```

- `osiris get` resolves and adds, upgrades, downgrades, or removes module
  requirements through explicit selectors.
- `osiris mod tidy` derives requirements from imports, adds missing direct and
  transitive dependencies, and removes unused requirements deterministically.
- `osiris mod download` populates the verified offline cache; `osiris mod
  verify` checks cached source against lock data without executing package
  code.
- Preserve or migrate existing `osiris mod sync`, `mod clean`, `deps`, offline
  cache, `replace`, and exclusion behavior with stable diagnostics.
- Add workspace/multi-module manifests with deterministic members, local
  replacements, and shared resolution.
- Provide stable text and JSON output for listing, graphing, updating,
  auditing, vendoring, and workspace operations.

## Discovery, publishing, and authentication

- Publishing a source module means pushing the declared module path and a
  valid semantic-version Git tag to its repository; no `osiris publish`
  service or central registration is required.
- Package discovery is separate from resolution. Define Go-style searchable
  documentation/index metadata that can be built from public repositories,
  while ensuring `osiris get` works without prior index registration.
- GitHub-backed discovery may use repository metadata/topics through a
  replaceable provider. Do not make availability of a discovery website or API
  a prerequisite for builds.
- Public repositories require no credentials. Private GitHub repositories use
  explicit Git/GitHub authentication without copying tokens into manifests,
  lockfiles, caches, diagnostics, telemetry, or generated artifacts.
- Local development uses workspace membership or a versioned `replace`
  directive without changing source import paths.
- Package metadata covers deprecation, provenance, licenses, platform
  constraints, and separately verified native artifacts.

## Security and reproducibility

- Preserve verified offline cache behavior and lockfile canonicalization.
- Defend against repository redirects/renames, dependency confusion, tag
  retargeting, force-pushes, path traversal, malicious submodules, mutable
  release assets, compromised mirrors, typosquatting metadata, and secret
  leakage.
- Never treat arbitrary GitHub Release assets as source packages. Native
  release artifacts require separate platform metadata, checksums, signatures,
  provenance, and capability declarations.
- Add license, vulnerability/advisory, provenance, dependency-policy, and
  native-binary audits.

## Tests and documentation

- Use local fake Git/HTTP/provider fixtures for normal tests; do not require
  live GitHub access in the default suite.
- Cover tagged releases, pseudo-versions, repository redirects, private-auth
  redaction, manifest/tidy changes, transitive selection, replacements,
  workspaces, offline/verify, tag mutation, malicious archives/submodules,
  advisory/license audits, concurrency, and reproducibility.
- Add opt-in GitHub integration coverage, release pipeline integration, and
  complete package-author/user documentation.

## Acceptance criteria

- [ ] Repository paths are canonical module identities; GitHub is the first
      complete provider and builds do not require central registration.
- [ ] The documented `osiris get` and `osiris mod` workflow maintains the
      manifest, lock, cache, and imports deterministically.
- [ ] Git tags/commits resolve reproducibly and locked/offline builds verify
      exact source and native dependencies.
- [ ] Workspaces, local replacements, public/private repositories, and
      discovery-versus-resolution behavior are documented and tested.
- [ ] Supply-chain audits and attack cases are tested.
- [ ] CLI, packaging, docs, and full validation pass.

## Non-goals

- No central package-upload registry, mandatory discovery service, execution
  of dependency code during resolution, GitHub-only language grammar, or
  community-governance mandate.

## Completion evidence

- Implemented canonical repository module identities, provider-based GitHub
  resolution, semantic tags, commit selectors, pseudo-versions, immutable
  source-tree digests, and provider/provenance lock metadata.
- Added `osiris.mod`, `osiris.work`, repository provider, workspace,
  replacement, tidy/download/verify/audit/vendor, and CLI workflows while
  retaining the existing local-module cache behavior.
- Added deterministic fake provider fixtures and coverage for manifests,
  repository imports, lock/cache verification, offline behavior, workspace
  resolution, traversal rejection, and CLI workflows.
- Verification: focused internal repository/module tests, focused CLI tests,
  compile-only package tests, and `git diff --check` passed.
