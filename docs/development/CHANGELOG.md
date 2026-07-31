# Changelog

Osiris uses Keep-a-Changelog-style sections: **Added**, **Changed**,
**Deprecated**, **Removed**, **Fixed**, and **Security**. Every released
version has a dated section. Unreleased changes are recorded under
`[Unreleased]`; release tooling moves them into the next SemVer version.

## [Unreleased]

### Added

- Release provenance via `osiris version`, unsigned archive dry runs, and a
  compatibility/deprecation policy.
- Optional typed function parameters, checked result annotations, exact
  function types, and a shared pre-execution checker for runtime and artifact
  entry points.

### Changed

- `emit` is valid throughout synchronous executable blocks. Interpreter and
  VM failures preserve earlier committed output, while concurrent task bodies
  must return values for the synchronous caller to emit.

## Compatibility rules

- `stable` support-matrix entries follow the current major version's public
  compatibility policy.
- `partial` entries name their supported subset; unsupported behavior is not a
  compatibility promise.
- `experimental` and `unsupported` entries may change without deprecation,
  though migrations are provided when practical.
- A stable removal requires a major release and a migration note. A stable
  deprecation is warned for at least one minor release before removal.
