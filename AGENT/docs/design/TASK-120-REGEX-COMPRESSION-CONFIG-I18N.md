# Task 120: regex, compression, configuration, and i18n contract

This document is the implementation contract for the task-120 host API and
the public `std/regex`, `std/compress`, `std/json`, and `std/i18n` modules.
Every operation has its own signature, argument contract, return value, limits,
and cancellation behavior. The public reference page mirrors these symbols
one at a time; a category is never used as a substitute for an API entry.

## `std/regex`

| Symbol | Signature | Contract |
| --- | --- | --- |
| `version` | `string` | Module contract version. |
| `engine_version()` | `string` | Embedded RE2 contract version. |
| `compile(pattern)` | `regex descriptor` | Validates UTF-8 RE2 syntax and returns `{pattern, engine_version}`. |
| `match(pattern, input)` | `bool` | Tests a bounded pattern against UTF-8 input. |
| `is_match(pattern, input)` | `bool` | Grammar-friendly alias of `match`. |
| `find(pattern, input)` | `list<match>` | Returns matches and numbered captures with UTF-8 byte offsets. |
| `replace(pattern, input, replacement)` | `string` | Replaces bounded matches with capture-aware replacement text. |
| `split(pattern, input)` | `list<string>` | Splits at matches and retains empty parts. |

The host functions are `Task120CompileRegex`,
`Task120CompiledRegex.Match`, `Task120CompiledRegex.Find`,
`Task120CompiledRegex.Replace`, and `Task120CompiledRegex.Split`.
`Task120RegexLimits` bounds pattern bytes, input bytes, matches, captures,
output bytes, and split parts. Zero selects documented defaults. RE2 rejects
lookaround, backreferences, and other super-linear constructs; malformed
patterns return `ErrTask120Invalid`. Operations check `context.Context` before
and during bounded work. Capture `StartByte` and `EndByte` are offsets in the
original UTF-8 input, with `-1` for an unmatched optional capture.

## `std/compress`

| Symbol | Signature | Contract |
| --- | --- | --- |
| `version` | `string` | Compression/archive contract version. |
| `compress(data, kind)` | `list<byte>` | `kind` is `gzip` or `zlib`; returns compressed bytes. |
| `decompress(data, kind)` | `list<byte>` | Decompresses under output and ratio limits. |
| `gzip(data)` | `list<byte>` | One-argument gzip shorthand. |
| `gunzip(data)` | `list<byte>` | One-argument gzip decompression shorthand. |
| `zlib(data)` | `list<byte>` | One-argument zlib shorthand. |
| `unzlib(data)` | `list<byte>` | One-argument zlib decompression shorthand. |
| `archive_create(entries)` | `list<byte>` | Encodes file/dir maps into deterministic safe USTAR bytes. |
| `archive_extract(data)` | `list<entry>` | Returns owned `{name, type, data}` entry maps. |

The host operations are `Task120Compress`, `Task120Decompress`,
`Task120CreateArchive`, and `Task120ExtractArchive`. `Task120CompressionKind`
is exactly `gzip` or `zlib`; `Task120CompressionStats` reports input and output
bytes. `Task120CompressionLimits` bounds input, output, expansion ratio, and
buffer bytes. Compression never closes caller-owned readers or writers.

The archive contract is `Task120ArchiveVersion = tar-ustar-safe-1`. A
`Task120ArchiveEntry` is either a file with data or a directory with no data.
`Task120ArchiveLimits` bounds entries, one-entry bytes, total bytes, and path
bytes. Absolute paths, drive-prefixed paths, NUL/invalid UTF-8, `..` escapes,
duplicate normalized names, symlinks, devices, sparse files, and other special
metadata are rejected with `ErrTask120Path` or `ErrTask120Invalid`.
Extraction invokes the caller visitor in archive order and returns owned data.

## `std/json`

| Symbol | Signature | Contract |
| --- | --- | --- |
| `version` | `string` | Bounded JSON utility contract version. |
| `parse(data)` | `config value` | Parses bounded RFC 8259 JSON while preserving object order and number spelling. |
| `parse_json(data)` | `config value` | Compatibility alias for `parse`. |
| `valid(data)` | `bool` | Checks one complete bounded JSON value. |
| `stringify(value)` | `list<byte>` | Emits compact JSON in object insertion order. |
| `pretty(value[, indent])` | `list<byte>` | Emits bounded human-readable JSON. |
| `canonicalize(value)` | `list<byte>` | Emits the supported finite-number RFC 8785 canonical form. |
| `pointer_get(value, pointer)` | `config value` | Reads a detached value using RFC 6901. |
| `pointer_set(value, pointer, replacement)` | `config value` | Adds or replaces a detached value using RFC 6901. |
| `pointer_remove(value, pointer)` | `config value` | Removes a detached member using RFC 6901. |
| `patch(document, operations)` | `config value` | Applies RFC 6902 add/remove/replace/move/copy/test operations transactionally. |
| `merge_patch(document, patch)` | `config value` | Applies RFC 7396 recursive merge semantics. |
| `parse_properties(data)` | `config value` | Parses the deterministic human-authored properties subset. |
| `validate(value, schema)` | `bool` | Validates against the bounded schema map; host validation retains paths. |
| `schema_property_names(schema)` | `list<string>` | Returns schema property names in deterministic order. |

The host functions are `Task120JSONParse`, `Task120JSONStreamParse`,
`Task120JSONValidate`, `Task120JSONValid`, `Task120JSONStringify`,
`Task120JSONPretty`, `Task120JSONPrettyIndent`, `Task120JSONCanonicalize`,
`Task120JSONPointerGet`, `Task120JSONPointerSet`, `Task120JSONPointerRemove`,
`Task120ApplyJSONPatch`, and `Task120ApplyJSONMergePatch`. `Task120ConfigValue`
is null, boolean, number, string, array, or ordered object. `Task120JSONLimits`
bounds bytes, depth, nodes, string bytes, object fields, pointer tokens, patch
operations, and output bytes. Duplicate JSON keys are invalid. JSON Pointer
follows RFC 6901, JSON Patch follows RFC 6902, Merge Patch follows RFC 7396,
and canonicalization implements the documented finite-number subset of RFC
8785. `Task120ConfigSchema` supports `type`, `required`, properties, `items`,
additional-property policy, and min/max item, property, and string-length
bounds. Validation returns stable `Task120ConfigError{Path, Code, Message}`
records such as `$["items"][0]`.

`std/config` remains registered as a deprecated compatibility alias for the
original properties/configuration surface. It is intentionally not the
canonical JSON module and new source should import `std/json`.

## `std/i18n`

| Symbol | Signature | Contract |
| --- | --- | --- |
| `data_version` | `string` | Embedded locale data version. |
| `locale(tag)` | `locale map` | Selects `en-US`, `de-DE`, `fr-FR`, or `en`/`de`/`fr` aliases. |
| `format_number(tag, number)` | `string` | Deterministic grouping and decimal formatting. |
| `format_date(tag, rfc3339)` | `string` | Formats date fields with embedded locale patterns. |
| `format_message(tag, id, count)` | `string` | Selects embedded singular/plural message text. |

The host API is `Task120LocaleFor`, `Task120FormatNumber`,
`Task120FormatDate`, and `Task120FormatMessage`. `Task120LocaleDataVersion`
is embedded in the binary; no ambient host locale, CLDR installation, timezone
database, or environment variable is consulted. `Task120NumberOptions` makes
fraction digits, grouping, and output bytes explicit. `Task120DateOptions`
selects short, medium, or long fixed patterns and an output bound. Message IDs,
argument count, placeholders, and output bytes are bounded. Unsupported tags,
styles, IDs, missing arguments, NaN, malformed dates, and invalid handles
return `ErrTask120Invalid`.

## Verification

`internal/osiris/task120_backend_test.go` covers Unicode captures and byte
offsets, unsupported ReDoS constructs, cancellation, deterministic gzip,
decompression output limits, archive traversal/duplicate/link rejection,
ordered JSON, duplicate-key rejection, schema paths, locale aliases, fixed
locale output, message pluralization, and fuzz non-panics for regex and JSON.
`internal/osiris/task120_123_stdlib_test.go` exercises public module imports
through both `Runtime.Run` and `Runtime.RunVM`.
