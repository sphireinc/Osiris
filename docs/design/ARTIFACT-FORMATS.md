# Osiris serialized and artifact formats

This inventory distinguishes process-boundary data from in-memory compiler
structures. A version field means the reader rejects incompatible data rather
than guessing. It does not promise indefinite compatibility.

| Format | Current version / magic | Persistence promise | Compatibility rule |
| --- | --- | --- | --- |
| Native executable cache manifest | JSON `format_version: 1` | Internal, local build cache | Cache reuse requires exact manifest equality for source hash, provider hash, compiler path/identity, flags, and native target. Missing, corrupt, old, or mismatched manifests fail clearly. |
| Module lockfile | JSON `version: 2` | Internal generated dependency state | Version 1 upgrades losslessly to v2 because v2 only adds optional metadata; other versions are rejected. A cached module is SHA-256 checked against the lock entry before use. |
| Module source cache | SHA-256-named `.osr` file | Internal generated cache | The lock entry is the authority. Missing data or a digest mismatch fails rather than evaluating altered cached source. |
| GPU plan JSON | JSON `schema_version: 1` | Experimental artifact | Consumers must check the schema version; unknown additive fields may be ignored. |
| Structured error value codec | tag `0x0a`, payload version `1` | Runtime binary codec | The decoder rejects unknown payload versions. |
| WebAssembly output | Standard WebAssembly magic and version `1` | Experimental snapshot artifact | It is a standard WASM binary, but Osiris does not promise snapshot/back-end compatibility beyond its experimental contract. |
| VM bytecode | none | In-memory only | `vmChunk` is compiled and immediately executed in the current Go process. It is not serialized, cached, or a public artifact format. |
| IR, SSA, CFG, DOM, alias, and inference reports | none | Textual inspection only | Their renderings are deliberately diagnostic, not a stable interchange format. |

## Native cache invalidation

The native cache key includes the manifest format version, generated C source
SHA-256, numerical-provider source SHA-256, resolved C compiler path and its
version output, exact compiler/linker flags, and the native OS/architecture
target. Generated C already reflects parsed source, static module contents,
and lowering decisions; provider source and flags are included separately
because they affect linked code without changing generated C.

The cache directory is explicitly caller-selected and intended for one local
native target. Do not share it as a cross-machine binary distribution format.

## Upgrade policy

Increment a format version whenever a reader could otherwise misinterpret old
bytes. Add a compatibility reader only when a supported migration is required;
otherwise return an error that names both the observed and expected versions.
The known v1→v2 lock migration is lossless and is rewritten as v2 on the next
save. Do not silently overwrite an otherwise incompatible lockfile or reuse an
unvalidated cache executable.
