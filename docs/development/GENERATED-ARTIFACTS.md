# Generated artifact policy

Repository source must be reviewable and reproducible without local build
products. Generated local artifacts are ignored and must not be committed.

## Ignored artifacts

| Artifact | Regeneration command | Policy |
| --- | --- | --- |
| `osiris` | `./build.sh` | Local developer binary. Ignored; release binaries belong in release assets, not the source tree. |
| `.osiris/` and `*.osiris.lock` | Module/dependency commands create them | Local module cache and lock state. Never hand-edit or commit. |
| `newdocs/node_modules/` | `cd newdocs && npm install` | Local dependency installation. Ignored. |
| `newdocs/dist/` | `cd newdocs && npm run build` | Generated public documentation output. Ignored; CI and the publishing workflow build it from tracked source. |
| Go caches, test profiles, coverage, temporary files | Go/test tools | Local-only. Ignored. |

`newdocs/` is the public documentation source of truth. Its React components,
reference data, package manifest, and lockfile are tracked; its dependency and
build-output directories may not. The tracked `docs/` files are legacy source
material and compatibility-route input only; do not add new public-reference
prose there.

`scripts/check-repository-hygiene.sh` enforces this policy locally and in CI.
When a future release intentionally needs a generated artifact, publish it as
a release asset or add an explicit, reproducible exception to this document
and the hygiene script.
