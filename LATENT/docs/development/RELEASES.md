# Releases and installation

Osiris follows semantic versioning. A released `MAJOR.MINOR.PATCH` version
promises compatible CLI and language behavior within its major line; documented
JSON schemas and artifact formats carry their own explicit versions. Pre-release
versions are not a compatibility promise.

## Install

Download the archive for the host target, verify its SHA-256 entry in
`SHA256SUMS`, extract it, and place `osiris` on `PATH`. On Windows use
`osiris.exe` from the zip archive. Confirm the binary before use:

```sh
osiris version
osiris run examples/named-values.osr
```

Remove the installed binary to uninstall. Local generated module state lives
beside projects (`.osiris.lock` and `.osiris/cache`) and is removed separately
with `osiris mod clean source.osr` when no longer needed.

## Native-build prerequisites

The distributed CLI itself is self-contained. `osiris run` and `osiris vm` do
not need a C compiler. `osiris build`/`compile` invoke a compatible system C
compiler; advanced numerical decompositions additionally require an explicitly
configured compatible provider. The exact native subset remains governed by
the support matrix and compile-time lowering diagnostics.

## Release dry run

Maintainers run `./scripts/release-dry-run.sh` with explicit
`OSIRIS_VERSION`, `OSIRIS_COMMIT`, and `SOURCE_DATE_EPOCH` for reproducible
metadata. It builds unsigned local archives only; signing and publication are
intentionally outside this script and must not run on untrusted pull requests.
