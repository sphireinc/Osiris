# Module resolution and reproducibility

## Canonical identity

Standard-library modules use their `std/...` path plus resolved version.
Local-file modules use their cleaned absolute path plus SHA-256 content digest:
`/absolute/module.osr@<sha256>`. A version selector is checked against the
module package declaration after the content identity is established.

Relative imports are resolved against the importing file and must remain below
the registry project root. A relative path that escapes that root is rejected.
Absolute paths are explicit local imports and retain their absolute identity;
there is no implicit local/remote fallback and no network registry.

## Lock and cache

`.osiris.lock` is generated state. Its entries record canonical source path,
content SHA-256, cached-source path, and optional package/version metadata.
Cache fallback recomputes the SHA-256 before returning source. A mismatch,
missing cache entry, or unsupported lock version fails clearly.

`osiris mod sync source.osr` resolves the dependency graph and refreshes the
generated state. `osiris mod clean source.osr` removes exactly that project’s
generated lock/cache state. Do not hand-edit either location.

## Offline mode

Set `OSIRIS_OFFLINE=1` to require cache-only local resolution. The resolver
does not read a module source file in this mode; it requires a matching lock
entry and verified cache bytes. This makes an offline build deterministic from
a populated cache and produces `offline module ... is not present in the
lockfile/cache` when state is missing.

## Determinism

The dependency report preserves source import order for each node and visits
the same canonical identities from the same root source and lock/cache state.
Cycles are reported using that canonical traversal. No wall-clock, network,
or directory-enumeration result participates in resolution.
