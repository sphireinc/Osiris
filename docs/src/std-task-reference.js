// Core standard-library modules are documented one public symbol per entry. The
// shared entry builder only removes repetition from the documentation source;
// every exported operation still has its own syntax, arguments, result, and
// behavior record.
const fn = (name, syntax, parameters, returns, summary, details, example) => ({
  name, syntax, summary, parameters, returns,
  returnValue: `The value returned by ${syntax}.`,
  details: [details], example,
});

const value = (name, syntax, returns, summary, details, example) => ({
  kind: 'constant', name, syntax, summary, parameters: 'No arguments; this is a module value.',
  returns, returnValue: `The stable ${name} value.`, details: [details], example,
});

const regex = [
  value('engine_version', 'engine_version()', 'string', 'Return the bounded RE2 engine contract version.', 'The version is stable for the module version and does not expose host regexp implementation details.', 'import "std/regex" as regex\nemit regex.engine_version()'),
  value('version', 'version', 'string', 'Return the std/regex module version.', 'The module version is independent of the embedded RE2 engine version.', 'import "std/regex" as regex\nemit regex.version'),
  fn('compile', 'compile(pattern)', '`regex` descriptor map', 'pattern is a UTF-8 RE2 expression.', 'Validate a regular-expression pattern before using it in a bounded operation.', 'The descriptor contains the pattern and engine version. It owns no mutable host resource; each operation retains its own bounded execution contract.', 'import "std/regex" as regex\nlet expression = regex.compile("a+")'),
  fn('find', 'find(pattern, input)', 'list<match map>', 'pattern is UTF-8 RE2 syntax and input is a string.', 'Return all bounded matches with byte offsets and numbered captures.', 'Offsets are UTF-8 byte offsets. Match count, input bytes, capture count, and output bytes are bounded; malformed or unsupported syntax returns a structured error.', 'import "std/regex" as regex\nemit regex.find("(?P<word>世界)", "你好世界")'),
  fn('is_match', 'is_match(pattern, input)', 'bool', 'pattern and input are strings.', 'Test whether a bounded RE2 expression matches the input.', 'Use `is_match` in source when `match` would collide with a language keyword. RE2 excludes lookaround, backreferences, and other super-linear constructs.', 'import "std/regex" as regex\nemit regex.is_match("a+", "aaa")'),
  fn('match', 'match(pattern, input)', 'bool', 'pattern and input are strings.', 'Test whether a bounded RE2 expression matches the input.', 'This API-parity spelling is equivalent to `is_match`; use the alias when the parser accepts the member name.', 'import "std/regex" as regex\nemit regex.is_match("a+", "aaa")'),
  fn('replace', 'replace(pattern, input, replacement)', 'string', 'pattern, input, and replacement are strings.', 'Replace bounded matches using the replacement syntax supported by the RE2 engine.', 'Replacement output is bounded and capture references are deterministic. The operation does not mutate the input.', 'import "std/regex" as regex\nemit regex.replace("[0-9]+", "todo 7", "#")'),
  fn('split', 'split(pattern, input)', 'list<string>', 'pattern and input are strings.', 'Split text at bounded regular-expression matches.', 'The number of parts and total output bytes are bounded; empty fields are retained according to the RE2 split contract.', 'import "std/regex" as regex\nemit regex.split(",", "a,,b")'),
];

const compression = [
  value('version', 'version', 'string', 'Return the compression/archive module version.', 'The compression and safe TAR subset use deterministic versioned contracts.', 'import "std/compress" as compress\nemit compress.version'),
  fn('archive_create', 'archive_create(entries)', 'list<byte>', 'entries is a list of `{name, type, data}` maps.', 'Create a deterministic safe USTAR archive.', 'Only regular files and directories are accepted. Paths, duplicate names, entry count, entry bytes, total bytes, and path bytes are bounded; links and special files are rejected.', 'import "std/compress" as compress\nemit compress.archive_create([{name: "a.txt", type: "file", data: [111u8]}])'),
  fn('archive_extract', 'archive_extract(data)', 'list<archive-entry map>', 'data is a byte-compatible list.', 'Extract a safe USTAR archive into owned entry maps.', 'Absolute paths, traversal, links, devices, malformed metadata, duplicate normalized names, and archive limits fail before unsafe output is returned.', 'import "std/compress" as compress\nemit compress.archive_extract(data)'),
  fn('compress', 'compress(data, kind)', 'list<byte>', 'data is bytes and kind is `gzip` or `zlib`.', 'Compress a byte stream with a bounded deterministic codec.', 'Input, output, buffer, and cancellation limits apply. The input list remains caller-owned.', 'import "std/compress" as compress\nemit compress.compress([1u8, 2u8], "gzip")'),
  fn('decompress', 'decompress(data, kind)', 'list<byte>', 'data is compressed bytes and kind is `gzip` or `zlib`.', 'Decompress a bounded gzip or zlib stream.', 'Output bytes and expansion ratio are bounded to prevent decompression bombs; cancellation is checked while reading and writing.', 'import "std/compress" as compress\nemit compress.decompress(data, "gzip")'),
  fn('gzip', 'gzip(data)', 'list<byte>', 'data is a byte-compatible list.', 'Compress bytes with deterministic gzip headers.', 'Equivalent to `compress(data, "gzip")`.', 'import "std/compress" as compress\nemit compress.gzip([1u8, 2u8])'),
  fn('gunzip', 'gunzip(data)', 'list<byte>', 'data is gzip bytes.', 'Decompress a gzip stream under output and ratio limits.', 'Equivalent to `decompress(data, "gzip")`.', 'import "std/compress" as compress\nemit compress.gunzip(data)'),
  fn('zlib', 'zlib(data)', 'list<byte>', 'data is a byte-compatible list.', 'Compress bytes with deterministic zlib framing.', 'Equivalent to `compress(data, "zlib")`.', 'import "std/compress" as compress\nemit compress.zlib([1u8, 2u8])'),
  fn('unzlib', 'unzlib(data)', 'list<byte>', 'data is zlib bytes.', 'Decompress a zlib stream under output and ratio limits.', 'Equivalent to `decompress(data, "zlib")`.', 'import "std/compress" as compress\nemit compress.unzlib(data)'),
];

const jsonValue = (name, syntax, returns, returnValue, summary, parameters, details, example) => ({
  kind: 'constant', name, syntax, summary, parameters, returns, returnValue, details: [details], example,
});

const jsonFn = (name, syntax, parameters, returns, returnValue, summary, details, errors, example, aliases = []) => ({
  name, syntax, summary, parameters, returns, returnValue, details: [details], errors, example,
  ...(aliases.length ? { members: aliases } : {}),
});

const json = [
  jsonValue(
    'version',
    'version',
    'string',
    'The semantic version string for the `std/json` module.',
    'Read the bounded JSON module contract version.',
    'No arguments; `version` is a module constant, not a function call.',
    'The version identifies the language-facing parser and validator contract. It does not identify the host JSON implementation. JSON text is interpreted according to [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259), especially its grammar and value model.',
    'import "std/json" as json\nemit json.version',
  ),
  jsonFn(
    'parse',
    'parse(data)',
    '`data` is one byte-compatible list containing UTF-8 JSON text.',
    'ordered JSON value',
    'The returned value is a JSON-compatible Osiris value: null, bool, string, number, list, or an ordered object representation. The host parser retains number spelling; the language adapter converts numbers to native Osiris numeric values.',
    'Parse bounded JSON text while preserving object member order and number spelling. `parse_json` remains a deprecated member alias.',
    'Parsing follows the JSON grammar and value model in [RFC 8259, §§4–6](https://www.rfc-editor.org/rfc/rfc8259#section-4). Limits cover input bytes, nesting depth, nodes, strings, and object fields. Duplicate keys, trailing data, and malformed literals return structured errors instead of partial values; the host representation retains source member order and number spelling.',
    '`data` must contain exactly one JSON value encoded as a byte-compatible list. The operation can return parse, limit, or cancellation errors.',
    'import "std/json" as json\nlet data = [123u8, 34u8, 120u8, 34u8, 58u8, 49u8, 125u8]\nemit json.parse(data)',
    ['parse_json'],
  ),
  jsonFn(
    'valid',
    'valid(data)',
    '`data` is one byte-compatible list containing UTF-8 JSON text.',
    'bool',
    '`true` means `data` contains exactly one accepted bounded JSON value; `false` means parsing failed.',
    'Check JSON validity without returning the parsed value.',
    'The predicate applies the same [RFC 8259](https://www.rfc-editor.org/rfc/rfc8259) grammar, duplicate-key policy, trailing-data rejection, and resource bounds as `parse`. Use `parse` when callers need the diagnostic error or value.',
    '`data` must be byte-compatible JSON text. Invalid syntax, duplicate keys, trailing data, and limit failures produce `false`; cancellation is reported as an operation error.',
    'import "std/json" as json\nemit json.valid([123u8, 49u8, 125u8])',
  ),
  jsonFn(
    'stringify',
    'stringify(value)',
    '`value` is a JSON-compatible Osiris value.',
    'list<byte>',
    'Compact UTF-8 JSON bytes; object insertion order is preserved.',
    'Serialize one JSON-compatible value as compact JSON.',
    'Strings are escaped, duplicate object keys are rejected, and unsupported runtime resources such as tasks, streams, functions, and modules are not serializable. The result is owned by the caller.',
    'Unsupported value kinds, invalid UTF-8 strings, invalid JSON numbers, and serialization limits return structured errors.',
    'import "std/json" as json\nemit json.stringify({name: "Osiris", ready: true})',
  ),
  jsonFn(
    'pretty',
    'pretty(value[, indent])',
    '`value` is JSON-compatible; optional `indent` is a non-empty string no longer than 16 characters.',
    'list<byte>',
    'Indented UTF-8 JSON bytes using two spaces by default or the supplied indentation string.',
    'Serialize a JSON-compatible value for human-readable output.',
    'Pretty output remains valid JSON and preserves object insertion order. It is presentation output, not a canonical signing representation; use `canonicalize` for deterministic canonical bytes.',
    'Unsupported values, invalid indentation, invalid numbers, and serialization limits return structured errors.',
    'import "std/json" as json\nemit json.pretty({name: "Osiris", ready: true}, "  ")',
  ),
  jsonFn(
    'canonicalize',
    'canonicalize(value)',
    '`value` is JSON-compatible and its numbers fit the supported finite canonicalization range.',
    'list<byte>',
    'Compact canonical JSON bytes with recursively sorted object keys and canonical finite numbers.',
    'Produce deterministic JSON bytes for hashing, cache keys, and artifacts.',
    'The output follows the supported finite-number subset of [RFC 8785](https://www.rfc-editor.org/rfc/rfc8785). It is distinct from ordinary `stringify`: source member order and non-canonical number spellings do not affect the canonical output.',
    'Duplicate keys, non-finite or out-of-range numbers, unsupported values, and serialization limits return structured errors.',
    'import "std/json" as json\nemit json.canonicalize({b: 2, a: 1})',
  ),
  jsonFn(
    'pointer_get',
    'pointer_get(value, pointer)',
    '`value` is JSON-compatible; `pointer` is an empty string or an RFC 6901 JSON Pointer.',
    'ordered JSON value',
    'A cloned value selected by the pointer; the empty pointer selects the complete document.',
    'Read one value by an RFC 6901 JSON Pointer.',
    'Object names decode `~0` as `~` and `~1` as `/`. Array indexes are decimal, have no leading zeroes, and must be in range. The returned value does not alias the input.',
    'Malformed escapes, missing members, scalar traversal, and out-of-range indexes return structured errors.',
    'import "std/json" as json\nemit json.pointer_get({items: ["first"]}, "/items/0")',
  ),
  jsonFn(
    'pointer_set',
    'pointer_set(value, pointer, replacement)',
    '`value` and `replacement` are JSON-compatible; `pointer` is an RFC 6901 JSON Pointer.',
    'ordered JSON value',
    'A new document with the selected member replaced or added; the root pointer replaces the complete document.',
    'Replace or add one value at an RFC 6901 JSON Pointer.',
    'The input and replacement remain unchanged. Object members are replaced or appended, and array `-` appends. Missing intermediate members and invalid array positions are rejected.',
    'Malformed pointers, scalar traversal, and invalid array positions return structured errors.',
    'import "std/json" as json\nemit json.pointer_set({ready: false}, "/ready", true)',
  ),
  jsonFn(
    'pointer_remove',
    'pointer_remove(value, pointer)',
    '`value` is JSON-compatible; `pointer` is an RFC 6901 JSON Pointer to an existing member or array item.',
    'ordered JSON value',
    'A new document without the selected member; removing the root produces JSON `null`.',
    'Remove one value at an RFC 6901 JSON Pointer.',
    'Object order for remaining members is preserved. The operation is non-mutating and returns no partial value when traversal fails.',
    'Malformed pointers, missing members, scalar traversal, and out-of-range indexes return structured errors.',
    'import "std/json" as json\nemit json.pointer_remove({metadata: {debug: true}}, "/metadata")',
  ),
  jsonFn(
    'patch',
    'patch(document, operations)',
    '`document` is JSON-compatible; `operations` is an array of RFC 6902 operation maps.',
    'ordered JSON value',
    'A new document after all operations are applied in array order.',
    'Apply JSON Patch operations atomically.',
    'The supported operations are `add`, `remove`, `replace`, `move`, `copy`, and `test`; paths use RFC 6901. A failed test or malformed operation leaves the input document unchanged.',
    'Missing `op`, `path`, `from`, or `value` fields, unsupported operations, failed tests, and invalid paths return structured errors.',
    'import "std/json" as json\nemit json.patch({name: "old"}, [{op: "replace", path: "/name", value: "new"}])',
  ),
  jsonFn(
    'merge_patch',
    'merge_patch(document, patch)',
    '`document` and `patch` are JSON-compatible values; `patch` follows RFC 7396.',
    'ordered JSON value',
    'A new document after recursive object merge; a non-object patch replaces the complete document.',
    'Apply JSON Merge Patch semantics.',
    'Object members whose patch value is `null` are removed. Nested objects merge recursively, while arrays and scalar patch values replace their targets.',
    'Unsupported values or conversion/size limits return structured errors.',
    'import "std/json" as json\nemit json.merge_patch({name: "Osiris", debug: true}, {debug: null})',
  ),
  jsonFn(
    'parse_properties',
    'parse_properties(data)',
    '`data` is one byte-compatible list containing UTF-8 properties text.',
    'ordered JSON-like value',
    'The returned value is the ordered object/list/scalar value produced by the deterministic properties subset.',
    'Parse the human-authored properties subset into the same ordered value family used by JSON parsing.',
    'This is the configuration-file compatibility parser kept beside the canonical JSON API. Its comment, quoting, array, scalar, duplicate-key, byte, depth, and field rules are deterministic, but it is not itself an RFC 8259 document. Use `parse_json` when interoperable JSON is required.',
    '`data` must be UTF-8 properties text. Malformed syntax, duplicate keys, unsupported nesting, limit violations, and cancellation return structured errors.',
    'import "std/json" as json\nlet data = [110u8, 97u8, 109u8, 101u8, 61u8, 79u8, 115u8, 105u8, 114u8, 105u8, 115u8]\nemit json.parse_properties(data)',
  ),
  jsonFn(
    'schema_property_names',
    'schema_property_names(schema)',
    '`schema` is a schema map containing optional `properties` and nested schema fields.',
    'list<string>',
    'A list of property names in the schema’s deterministic order.',
    'Return the property names declared by a bounded validation schema.',
    'The operation inspects schema structure only; it does not read host configuration or mutate the schema. Names are returned in lexical order for deterministic output. The supported validation vocabulary is intentionally smaller than the full [JSON Schema](https://json-schema.org/specification) dialect family.',
    '`schema` must be a valid schema value. Invalid property maps, non-string names, unsupported nested values, and limit violations return structured errors.',
    'import "std/json" as json\nemit json.schema_property_names({type: "object", properties: {name: {type: "string"}}})',
  ),
  jsonFn(
    'validate',
    'validate(value, schema)',
    '`value` is an ordered JSON-like value; `schema` is a bounded schema map.',
    'bool',
    '`true` means the value satisfies the supported schema subset; `false` means it does not.',
    'Validate an ordered JSON-like value against the supported schema subset.',
    'Validation is a predicate over the bounded schema vocabulary, including types, required properties, additional-property policy, item/property schemas, lengths, counts, and collection limits. JSON Schema terminology is used where it is compatible with [JSON Schema Core 2020-12](https://json-schema.org/draft/2020-12/json-schema-core); this API does not claim full dialect support. Validation does not reorder or mutate `value`.',
    '`value` or `schema` with unsupported kinds, malformed schema fields, invalid limits, or excessive depth return structured errors. A valid value that fails a schema returns `false`.',
    'import "std/json" as json\nemit json.validate({name: "Osiris"}, {type: "object", required: ["name"]})',
  ),
];

const i18n = [
  value('data_version', 'data_version', 'string', 'Return the embedded locale data version.', 'Locale formatting never silently consults the host locale or CLDR installation.', 'import "std/i18n" as i18n\nemit i18n.data_version'),
  fn('locale', 'locale(tag)', 'locale map', 'tag is `en-US`, `de-DE`, `fr-FR`, or an explicit language alias.', 'Select one embedded, versioned locale.', 'Unsupported tags fail explicitly. The returned map identifies both normalized tag and locale-data version.', 'import "std/i18n" as i18n\nemit i18n.locale("en-US")'),
  fn('format_date', 'format_date(tag, rfc3339)', 'string', 'tag is supported and the date is RFC3339 text.', 'Format a date with the embedded locale date pattern.', 'Short, medium, and long host options are fixed by the locale adapter; timezone conversion is not implicit.', 'import "std/i18n" as i18n\nemit i18n.format_date("en-US", "2024-02-29T00:00:00Z")'),
  fn('format_message', 'format_message(tag, id, count)', 'string', 'tag and message id are strings; count is an integer.', 'Format an embedded plural-aware message.', 'Message IDs, placeholders, count selection, argument count, and output bytes are bounded and deterministic.', 'import "std/i18n" as i18n\nemit i18n.format_message("en-US", "items", 2)'),
  fn('format_number', 'format_number(tag, number)', 'string', 'tag is supported and number is finite or an explicitly supported infinity.', 'Format a number with deterministic grouping and locale decimal separators.', 'The language adapter uses two fractional digits as its stable default; the host API exposes explicit min/max fraction options.', 'import "std/i18n" as i18n\nemit i18n.format_number("en-US", 1234.5)'),
];

const db = [
  value('api_version', 'api_version', 'uint32', 'Return the database provider API version.', 'Providers must report the exact version before registration.', 'import "std/db" as db\nemit db.api_version'),
  value('provider', 'provider', 'string', 'Return the deterministic embedded provider name.', 'The shipped fixture is `embedded`; production providers remain explicit capability-gated registrations.', 'import "std/db" as db\nemit db.provider'),
  fn('begin', 'begin(connection)', 'database transaction handle', 'connection is an owned database connection.', 'Begin a snapshot transaction.', 'The transaction owns a pool lease and rolls back on close, cancellation, or scope cleanup. Commit detects stale snapshots.', 'import "std/db" as db\nlet connection = db.open()\nlet transaction = db.begin(connection)\ndefer { close(transaction) }'),
  fn('close', 'close(handle)', 'bool', 'handle is a database connection, rows, statement, or transaction.', 'Close a database resource idempotently.', 'Database handles use the ordinary stream/resource cleanup path. Closing a connection closes outstanding child resources; closing an unfinished transaction rolls back.', 'import "std/db" as db\nlet connection = db.open()\ndefer { db.close(connection) }'),
  fn('columns', 'columns(rows)', 'list<column map>', 'rows is an owned result iterator.', 'Return result column names, normalized types, and nullability.', 'Column metadata is copied and remains valid after the next-row cursor advances.', 'import "std/db" as db\n# emit db.columns(rows)'),
  fn('commit', 'commit(transaction)', 'bool', 'transaction is an owned transaction handle.', 'Publish a transaction snapshot atomically.', 'Commit is cancellable and returns a retryable conflict when another writer committed first.', 'import "std/db" as db\n# emit db.commit(transaction)'),
  fn('exec', 'exec(handle, query[, arguments])', 'result map', 'handle is a connection or transaction; query is parameterized text; arguments is a value list.', 'Execute a bounded parameterized mutation.', 'The result contains `rows_affected`. Query text is bounded and only a stable query identity crosses structured error metadata.', 'import "std/db" as db\nlet connection = db.open()\nemit db.exec(connection, "CREATE TABLE items (id INTEGER)", [])'),
  fn('next', 'next(rows)', 'bool', 'rows is an owned result iterator.', 'Advance to the next database row.', 'Row and byte limits are checked before a row is yielded. End-of-stream closes the row lease.', 'import "std/db" as db\n# emit db.next(rows)'),
  fn('open', 'open([provider])', 'database connection handle', 'provider defaults to `embedded`.', 'Open a capability-authorized deterministic database connection.', 'The language adapter grants only the embedded fixture capabilities. The host driver contract accepts explicit secret references, limits, and cancellation.', 'import "std/db" as db\nlet connection = db.open()\ndefer { db.close(connection) }'),
  fn('prepare', 'prepare(handle, query)', 'database statement handle', 'handle is a connection or transaction; query is parameterized text.', 'Prepare a bounded statement for repeated execution.', 'Statements count against the per-connection limit and must be closed. They retain no caller-owned slices.', 'import "std/db" as db\nlet statement = db.prepare(connection, "SELECT id FROM items")\ndefer { db.close(statement) }'),
  fn('query', 'query(handle, query[, arguments])', 'database rows handle', 'handle is a connection or transaction; arguments is a value list.', 'Execute a bounded parameterized query.', 'Rows own their pool lease until exhaustion or close and return defensive copies of current values.', 'import "std/db" as db\nlet rows = db.query(connection, "SELECT id FROM items", [])\ndefer { db.close(rows) }'),
  fn('rollback', 'rollback(transaction)', 'bool', 'transaction is an owned transaction handle.', 'Discard a transaction snapshot.', 'Rollback is idempotent and releases the transaction lease even after a cancellation path.', 'import "std/db" as db\n# emit db.rollback(transaction)'),
  fn('row_error', 'row_error(rows)', 'error or null', 'rows is an owned result iterator.', 'Return the terminal row iteration error, if any.', 'Errors retain structured operation/provider/retryability fields and redact secrets and raw query text.', 'import "std/db" as db\n# emit db.row_error(rows)'),
  fn('statement_exec', 'statement_exec(statement[, arguments])', 'int64', 'statement is prepared and arguments is a value list.', 'Execute a prepared mutation.', 'The returned count is rows affected; statement lifetime and cancellation remain explicit.', 'import "std/db" as db\n# emit db.statement_exec(statement, [])'),
  fn('statement_query', 'statement_query(statement[, arguments])', 'database rows handle', 'statement is prepared and arguments is a value list.', 'Execute a prepared query.', 'The returned rows own a separate result lease and must be closed independently.', 'import "std/db" as db\n# emit db.statement_query(statement, [])'),
  fn('stats', 'stats(connection)', 'pool-stats map', 'connection is an owned database connection.', 'Inspect max-open, in-use, available, and prepared counts.', 'Stats are a point-in-time copy and contain no credentials or raw handles.', 'import "std/db" as db\n# emit db.stats(connection)'),
  fn('values', 'values(rows)', 'list<value>', 'rows is an owned result iterator after a successful next.', 'Return a defensive copy of the current row.', 'Values preserve null, scalar, byte-list, datetime, decimal, and integer families supported by the provider.', 'import "std/db" as db\n# emit db.values(rows)'),
];

const telemetry = [
  value('version', 'version', 'string', 'Return the telemetry module version.', 'The language adapter creates a bounded runtime-local provider lazily.', 'import "std/telemetry" as telemetry\nemit telemetry.version'),
  fn('close', 'close()', 'bool', 'No arguments.', 'Flush and close the runtime telemetry provider.', 'Close is idempotent. Queued records are drained according to exporter failure policy.', 'import "std/telemetry" as telemetry\ntelemetry.close()'),
  fn('counter_add', 'counter_add(name, delta[, labels])', 'bool', 'name is text, delta is a non-negative integer, and labels is an optional string map.', 'Increment a bounded labeled counter.', 'Series cardinality, label count, key/value bytes, queue behavior, and cancellation are enforced by the provider.', 'import "std/telemetry" as telemetry\ntelemetry.counter_add("requests", 1, {method: "get"})'),
  fn('debug', 'debug(message, source[, fields])', 'bool', 'message and source are strings; fields is an optional map.', 'Emit a debug structured log.', 'Fields are sorted and redacted before enqueueing; records carry the provider clock and inherited context IDs.', 'import "std/telemetry" as telemetry\ntelemetry.debug("detail", "app.osr")'),
  fn('dropped', 'dropped()', 'uint64', 'No arguments.', 'Return the number of records dropped by queue overflow.', 'The counter is monotonic for the provider and never contains record payloads.', 'import "std/telemetry" as telemetry\nemit telemetry.dropped()'),
  fn('error', 'error(message, source[, fields])', 'bool', 'message and source are strings; fields is an optional map.', 'Emit an error structured log.', 'Sensitive keys and values are redacted before export. Ordinary language output is unchanged.', 'import "std/telemetry" as telemetry\ntelemetry.error("failed", "app.osr")'),
  fn('flush', 'flush()', 'bool', 'No arguments.', 'Flush metrics and queued log/span records.', 'Flush observes cancellation and exporter failure policy without changing ordinary program output.', 'import "std/telemetry" as telemetry\ntelemetry.flush()'),
  fn('gauge_set', 'gauge_set(name, value[, labels])', 'bool', 'name is text, value is numeric, and labels is optional.', 'Set a bounded labeled gauge.', 'Gauge series are deterministic and bounded by the configured cardinality and label limits.', 'import "std/telemetry" as telemetry\ntelemetry.gauge_set("load", 0.5)'),
  fn('histogram_observe', 'histogram_observe(name, value[, labels])', 'bool', 'name is text, value is numeric, and labels is optional.', 'Observe one value in a bounded histogram.', 'Bucket boundaries are fixed by the provider configuration and snapshots are sorted by metric name and labels.', 'import "std/telemetry" as telemetry\ntelemetry.histogram_observe("latency", 0.2)'),
  fn('info', 'info(message, source[, fields])', 'bool', 'message and source are strings; fields is an optional map.', 'Emit an informational structured log.', 'The runtime clock is injected and field order is stable. Redaction occurs before queueing.', 'import "std/telemetry" as telemetry\ntelemetry.info("started", "app.osr")'),
  fn('metrics', 'metrics()', 'list<metric map>', 'No arguments.', 'Return a deterministic metric snapshot.', 'Metric points are sorted by name, kind, and normalized labels; no exporter transport is required for a snapshot.', 'import "std/telemetry" as telemetry\nemit telemetry.metrics()'),
  fn('new', 'new()', 'bool', 'No arguments.', 'Initialize the runtime-local bounded telemetry provider.', 'Initialization is lazy and idempotent. The default exporter is deterministic in-memory capture.', 'import "std/telemetry" as telemetry\ntelemetry.new()'),
  fn('snapshot', 'snapshot()', 'list<telemetry item map>', 'No arguments.', 'Return deterministic in-memory exporter items.', 'Only the default memory exporter is exposed by the language adapter; custom exporters remain a host capability.', 'import "std/telemetry" as telemetry\nemit telemetry.snapshot()'),
  fn('span_attribute', 'span_attribute(span, key, value)', 'bool', 'span is an owned span, key is text, and value is JSON-compatible.', 'Set one bounded span attribute.', 'Attributes are normalized, sorted, redacted, and rejected after span end.', 'import "std/telemetry" as telemetry\n# telemetry.span_attribute(span, "route", "/todos")'),
  fn('span_context', 'span_context(span)', 'span-context map', 'span is an owned span.', 'Return trace, span, and parent IDs.', 'The returned identity is propagation-safe and contains no exporter internals.', 'import "std/telemetry" as telemetry\n# emit telemetry.span_context(span)'),
  fn('span_end', 'span_end(span)', 'bool', 'span is an owned span.', 'End and export one span record.', 'Ending is idempotent and emits exactly one record. Closing the handle uses the same operation.', 'import "std/telemetry" as telemetry\n# telemetry.span_end(span)'),
  fn('span_event', 'span_event(span, name)', 'bool', 'span and event name are explicit.', 'Append one bounded span event.', 'Event order, timestamp, field limits, and redaction are deterministic.', 'import "std/telemetry" as telemetry\n# telemetry.span_event(span, "cache-hit")'),
  fn('start_span', 'start_span(name)', 'span handle', 'name is text.', 'Start a bounded root span.', 'The host API also supports parent context and task propagation; the language handle is scope-closable.', 'import "std/telemetry" as telemetry\nlet span = telemetry.start_span("load")\ndefer { telemetry.span_end(span) }'),
  fn('task_id', 'task_id()', 'string', 'No arguments.', 'Return the current telemetry task identity.', 'The language adapter returns the runtime context identity; an empty string means no task identity was attached.', 'import "std/telemetry" as telemetry\nemit telemetry.task_id()'),
  fn('correlation_id', 'correlation_id()', 'string', 'No arguments.', 'Return the current telemetry correlation identity.', 'The identity is separate from trace/span IDs and remains stable across child task propagation.', 'import "std/telemetry" as telemetry\nemit telemetry.correlation_id()'),
  fn('warn', 'warn(message, source[, fields])', 'bool', 'message and source are strings; fields is an optional map.', 'Emit a warning structured log.', 'Warnings use the same deterministic clock, ordering, queue, and redaction contract as other levels.', 'import "std/telemetry" as telemetry\ntelemetry.warn("slow", "app.osr")'),
];

const unsafeMemory = [
  value('version', 'version', 'string', 'Return the bounded `std/unsafe` contract version.', 'This version identifies the handle, view, ownership, and bounds contract; it does not identify a host pointer ABI.', 'import "std/unsafe" as memory\nemit memory.version'),
  value('capability', 'capability', 'string', 'Return the exact host capability required by this module.', 'The value is always `unsafe.memory`. Importing the module fails unless the host manifest grants that capability; source cannot grant it.', 'import "std/unsafe" as memory\nemit memory.capability'),
  value('max_bytes', 'max_bytes', 'u64', 'Return the maximum allocation size.', 'Allocations and copied input are bounded at 64 MiB. The limit is checked before storage is created.', 'import "std/unsafe" as memory\nemit memory.max_bytes'),
  fn('buffer', 'buffer(size)', 'opaque buffer handle', 'size is a non-negative integer no larger than `max_bytes`.', 'Allocate zero-initialized writable bytes.', 'The returned `handle:unsafe_memory_buffer` owns its allocation. It exposes no address and must be released exactly once; repeated release is harmless.', 'import "std/unsafe" as memory\nlet buffer = memory.buffer(8)\ndefer { memory.release(buffer) }'),
  fn('from_bytes', 'from_bytes(bytes)', 'opaque buffer handle', 'bytes is a byte-compatible list and its length is no larger than `max_bytes`.', 'Create an owned buffer by copying bytes.', 'The source list remains caller-owned and later mutations cannot affect the buffer. The copy is bounded and initialized.', 'import "std/unsafe" as memory\nlet buffer = memory.from_bytes([1u8, 2u8, 3u8])\ndefer { memory.release(buffer) }'),
  fn('view', 'view(handle, offset, length, element_size, alignment, byte_order, writable)', 'opaque view handle', 'handle is an owned buffer/view; offsets are bytes; element size and alignment are 1, 2, 4, or 8; byte order is `little`/`big`; writable is bool.', 'Create a checked typed view.', 'The range must be within the parent, length must be an element multiple, and offset/length must satisfy alignment. A view retains the backing allocation and never contains a raw address.', 'import "std/unsafe" as memory\nlet buffer = memory.buffer(8)\nlet words = memory.view(buffer, 0, 8, 2, 2, "little", true)\ndefer { memory.release(words) }\ndefer { memory.release(buffer) }'),
  fn('slice', 'slice(view, offset, length)', 'opaque view handle', 'offset and length are byte offsets compatible with the source view.', 'Create a bounded sub-view retaining the source allocation.', 'Element size, alignment, byte order, and mutability are preserved. Misaligned or out-of-range slices fail before access.', 'import "std/unsafe" as memory\nlet sub = memory.slice(words, 2, 4)\ndefer { memory.release(sub) }'),
  fn('len', 'len(handle)', 'u64', 'handle is an open buffer or view.', 'Return the view length in bytes.', 'The result is metadata only and cannot reveal an address or backing identity.', 'import "std/unsafe" as memory\nemit memory.len(buffer)'),
  fn('elements', 'elements(handle)', 'u64', 'handle is an open buffer or typed view.', 'Return the number of complete elements in the view.', 'This is `len / element_size`; incomplete elements are never exposed.', 'import "std/unsafe" as memory\nemit memory.elements(words)'),
  fn('bytes', 'bytes(handle)', 'list<u8>', 'handle is an open buffer or view.', 'Return a defensive copy of the visible bytes.', 'The returned list does not alias the allocation and is bounded by the view length.', 'import "std/unsafe" as memory\nemit memory.bytes(buffer)'),
  fn('metadata', 'metadata(handle)', 'map', 'handle is an open buffer or view.', 'Return bytes, elements, element size, alignment, byte order, and writable metadata.', 'Metadata contains no address, pointer, stable identity, or host object representation.', 'import "std/unsafe" as memory\nemit memory.metadata(words)'),
  fn('read_u8', 'read_u8(view, index)', 'u8', 'view has element size 1 and index is in range.', 'Read one checked byte.', 'The operation checks lifetime and bounds while holding the backing read lock.', 'import "std/unsafe" as memory\nemit memory.read_u8(buffer, 0)'),
  fn('read_u16', 'read_u16(view, index)', 'u16', 'view has element size 2 and index is in range.', 'Read one checked 16-bit unsigned element.', 'The declared view byte order is used; no alignment cast or pointer arithmetic occurs.', 'import "std/unsafe" as memory\nemit memory.read_u16(words, 1)'),
  fn('read_u32', 'read_u32(view, index)', 'u32', 'view has element size 4 and index is in range.', 'Read one checked 32-bit unsigned element.', 'The operation rejects a view with another element size and rejects stale handles.', 'import "std/unsafe" as memory\nemit memory.read_u32(view, 0)'),
  fn('read_u64', 'read_u64(view, index)', 'u64', 'view has element size 8 and index is in range.', 'Read one checked 64-bit unsigned element.', 'Values are decoded with the view byte order and remain ordinary numeric values only after a successful checked read.', 'import "std/unsafe" as memory\nemit memory.read_u64(view, 0)'),
  fn('write_u8', 'write_u8(view, index, value)', 'bool', 'view has element size 1, is writable, and value fits u8.', 'Write one checked byte.', 'Writes reject immutable views, stale handles, and out-of-range indexes; success returns true.', 'import "std/unsafe" as memory\nmemory.write_u8(buffer, 0, 7u8)'),
  fn('write_u16', 'write_u16(view, index, value)', 'bool', 'view has element size 2, is writable, and value fits u16.', 'Write one checked 16-bit unsigned element.', 'The write uses the view byte order and never exposes a foreign address.', 'import "std/unsafe" as memory\nmemory.write_u16(words, 1, 0x1234u16)'),
  fn('write_u32', 'write_u32(view, index, value)', 'bool', 'view has element size 4, is writable, and value fits u32.', 'Write one checked 32-bit unsigned element.', 'The write is bounded by the view and synchronized with concurrent reads and release.', 'import "std/unsafe" as memory\nmemory.write_u32(view, 0, 7u32)'),
  fn('write_u64', 'write_u64(view, index, value)', 'bool', 'view has element size 8, is writable, and value fits u64.', 'Write one checked 64-bit unsigned element.', 'The value is checked before encoding and the backing storage remains opaque.', 'import "std/unsafe" as memory\nmemory.write_u64(view, 0, 7u64)'),
  fn('copy', 'copy(destination, destination_offset, source, source_offset, length)', 'u64', 'both handles are open; destination is writable; all byte ranges are in bounds.', 'Copy a checked byte range between buffers or views.', 'The source is snapshotted before destination locking, making overlapping same-handle copies safe and avoiding nested-lock deadlocks. The result is the copied byte count.', 'import "std/unsafe" as memory\nemit memory.copy(buffer, 0, buffer, 2, 2)'),
  fn('release', 'release(handle)', 'bool', 'handle is an unsafe memory buffer or view.', 'Release one owned handle.', 'The first release returns true; later releases return false. The final allocation reference detaches storage, and stale use fails rather than accessing freed memory.', 'import "std/unsafe" as memory\nemit memory.release(buffer)'),
];

const extendedNative = 'Native C currently reports a source-spanned unsupported-member diagnostic for this additive standard-library symbol. Interpreter and VM execution provide the portable bounded contract; native support can be added only with parity-tested lowering.';
const nativeLibraryCryptoNative = 'Native C directly lowers this pure member through the portable SHA-512 runtime. It accepts canonical algorithm names, allocates no host resource, returns owned digest data, and reports wrong kinds, wrong arity, unsupported algorithms, invalid byte lists, and the 1 MiB input bound as structured native failures.';
const nativeLibraryCryptoHMACNative = 'Native C directly lowers this pure HMAC member through the portable SHA-256 and SHA-512 runtime. It accepts bounded text or byte lists, normalizes keys to the selected hash block size, returns an owned lowercase hexadecimal string, releases temporary byte staging on every path, requires no host capability, and reports wrong arity, wrong kinds, unsupported algorithms, and the 64 KiB key or 1 MiB data bounds as structured native failures.';
const nativeLibraryCryptoCodecNative = 'Native C directly lowers this pure codec member through the portable C runtime. It accepts bounded strings or byte lists as documented, returns an owned string or byte list, releases temporary byte staging on success and failure, requires no host capability, and reports wrong arity, wrong kinds, malformed encodings, and the 1 MiB bound as structured native failures.';
const nativeLibraryCryptoCompareNative = 'Native C directly lowers this pure comparison member through the portable C runtime. It accepts bounded strings or byte lists, compares every byte in the common length without an early-exit content branch, treats length mismatch as unequal, releases temporary byte staging on every path, and requires no host capability.';
const nativeLibrarySignalNative = 'Native C directly lowers this bounded pure signal member through the portable C signal runtime. Inputs are copied into temporary numeric staging storage, outputs are newly owned tensors, no host capability is required, and wrong arity, non-numeric data, empty-statistics input, invalid modes, and the 65536-sample bound are reported as structured native failures.';
const nativeLibrarySignalTransformNative = 'Native C directly lowers this bounded pure transform through the portable C signal runtime. Real tensors, complex tensors, and numeric or complex lists are copied into temporary OComplex staging storage; the output is a newly owned rank-one complex tensor, no host capability is required, and wrong arity, invalid rank/kinds, allocation failure, and the 4096-sample bound are reported as structured native failures.';
const extendedFn = (name, syntax, parameters, returns, summary, details, example) =>
  ({ ...fn(name, syntax, parameters, returns, summary, details, example), native: extendedNative });
const extendedValue = (name, syntax, returns, summary, details, example) =>
  ({ ...value(name, syntax, returns, summary, details, example), native: extendedNative });
const nativeLibrarySignalFn = (name, syntax, parameters, returns, summary, details, example) =>
  ({ ...extendedFn(name, syntax, parameters, returns, summary, details, example), native: nativeLibrarySignalNative });
const nativeLibrarySignalTransformFn = (name, syntax, parameters, returns, summary, details, example) =>
  ({ ...extendedFn(name, syntax, parameters, returns, summary, details, example), native: nativeLibrarySignalTransformNative });

const extendedCrypto = [
  extendedValue('version', 'version', 'string', 'Return the expanded crypto contract version.', 'The additive digest and encoding surface is versioned independently from the module selector.', 'import "std/crypto" as crypto\nemit crypto.version'),
  extendedValue('max_input_bytes', 'max_input_bytes', 'u64', 'Return the maximum digest input size.', 'Digest and encoding inputs are rejected before allocation when they exceed 1 MiB.', 'import "std/crypto" as crypto\nemit crypto.max_input_bytes'),
  extendedValue('max_output_bytes', 'max_output_bytes', 'u64', 'Return the maximum derived-output size.', 'Derived key output is bounded to 1 MiB.', 'import "std/crypto" as crypto\nemit crypto.max_output_bytes'),
  extendedValue('max_key_bytes', 'max_key_bytes', 'u64', 'Return the maximum HMAC or derivation key size.', 'Keys larger than the fixed 64 KiB contract are rejected before hashing.', 'import "std/crypto" as crypto\nemit crypto.max_key_bytes'),
  extendedValue('max_token_bytes', 'max_token_bytes', 'u64', 'Return the maximum deterministic token size.', 'Token requests are bounded to 4096 bytes.', 'import "std/crypto" as crypto\nemit crypto.max_token_bytes'),
  extendedValue('sha256', 'sha256', 'string', 'Name the SHA-256 algorithm.', 'Pass this stable algorithm identifier to digest, HMAC, and capability checks.', 'import "std/crypto" as crypto\nemit crypto.sha256'),
  extendedValue('sha512', 'sha512', 'string', 'Name the SHA-512 algorithm.', 'The identifier selects the standard SHA-512 digest.', 'import "std/crypto" as crypto\nemit crypto.sha512'),
  extendedValue('sha512_256', 'sha512_256', 'string', 'Name the SHA-512/256 algorithm.', 'The identifier selects the 256-bit SHA-512 truncated digest.', 'import "std/crypto" as crypto\nemit crypto.sha512_256'),
  { ...extendedFn('digest', 'digest(data, algorithm)', 'data is text or bytes; algorithm is `sha256`, `sha512`, or `sha512_256`.', 'hex string', 'Hash bounded input and return lowercase hexadecimal.', 'The operation is deterministic and does not use host entropy or mutable state.', 'import "std/crypto" as crypto\nemit crypto.digest("hello", crypto.sha256)'), native: nativeLibraryCryptoNative },
  { ...extendedFn('digest_bytes', 'digest_bytes(data, algorithm)', 'data is text or bytes; algorithm is a supported name.', 'list<u8>', 'Hash bounded input and return raw digest bytes.', 'The returned list is newly owned and sized to the selected algorithm.', 'import "std/crypto" as crypto\nemit crypto.digest_bytes("hello", crypto.sha256)'), native: nativeLibraryCryptoNative },
  { ...extendedFn('hmac', 'hmac(key, data, algorithm)', 'key and data are bounded text or bytes; algorithm is supported.', 'hex string', 'Compute a keyed HMAC in lowercase hexadecimal.', 'Key and message limits are checked before the HMAC state is created. Native C uses the local SHA-256 and SHA-512 implementations, returns owned output, releases temporary byte staging, and requires no host capability.', 'import "std/crypto" as crypto\nemit crypto.hmac("key", "message", crypto.sha256)'), native: nativeLibraryCryptoHMACNative },
  { ...extendedFn('compare_digest', 'compare_digest(left, right)', 'left and right are text or bytes.', 'bool', 'Compare two digests without early-exit comparison.', 'The comparison is constant-time for equal-length byte strings and returns false for different lengths.', 'import "std/crypto" as crypto\nemit crypto.compare_digest("aa", "aa")'), native: nativeLibraryCryptoCompareNative },
  { ...extendedFn('hex_encode', 'hex_encode(data)', 'data is bounded text or bytes.', 'string', 'Encode bytes as lowercase hexadecimal.', 'The input is copied into an owned textual result under the output bound.', 'import "std/crypto" as crypto\nemit crypto.hex_encode([1u8, 2u8])'), native: nativeLibraryCryptoCodecNative },
  { ...extendedFn('hex_decode', 'hex_decode(text)', 'text is even-length hexadecimal within the input bound.', 'list<u8>', 'Decode hexadecimal into bytes.', 'Malformed digits and odd-length text return an error before a partial result is exposed.', 'import "std/crypto" as crypto\nemit crypto.hex_decode("0102")'), native: nativeLibraryCryptoCodecNative },
  extendedFn('base64_encode', 'base64_encode(data)', 'data is bounded text or bytes.', 'string', 'Encode bytes with standard padded Base64.', 'The standard alphabet and padding are fixed; the operation is deterministic.', 'import "std/crypto" as crypto\nemit crypto.base64_encode("hello")'),
  extendedFn('base64_decode', 'base64_decode(text)', 'text is standard padded Base64 within the input bound.', 'list<u8>', 'Decode standard Base64.', 'Invalid alphabet, padding, and size return an explicit error.', 'import "std/crypto" as crypto\nemit crypto.base64_decode("aGVsbG8=")'),
  extendedFn('base64url_encode', 'base64url_encode(data)', 'data is bounded text or bytes.', 'string', 'Encode bytes with URL-safe unpadded Base64.', 'The URL-safe alphabet is fixed and padding is omitted.', 'import "std/crypto" as crypto\nemit crypto.base64url_encode("hello")'),
  extendedFn('base64url_decode', 'base64url_decode(text)', 'text is URL-safe Base64 within the input bound.', 'list<u8>', 'Decode URL-safe Base64.', 'Both malformed alphabet and oversized decoded output are rejected.', 'import "std/crypto" as crypto\nemit crypto.base64url_decode("aGVsbG8")'),
  extendedFn('derive_key', 'derive_key(key, context, length)', 'key and context are bounded bytes; length is 0..max_output_bytes.', 'list<u8>', 'Derive deterministic key material from explicit inputs.', 'This is a bounded HMAC-based expansion contract, not a password KDF. It has no ambient salt or randomness; callers must supply domain-separated context.', 'import "std/crypto" as crypto\nemit crypto.derive_key("secret", "app:v1", 32)'),
  extendedFn('random_bytes', 'random_bytes(seed, length)', 'seed is bounded text or bytes; length is bounded.', 'list<u8>', 'Generate reproducible pseudo-random bytes from an explicit seed.', 'The name describes byte-shaped output: this function is deterministic and is not a cryptographic entropy source.', 'import "std/crypto" as crypto\nemit crypto.random_bytes("fixture", 16)'),
  extendedFn('token', 'token(seed, length)', 'seed is bounded text or bytes; length is 0..max_token_bytes.', 'string', 'Generate a reproducible URL-safe token.', 'The token uses deterministic expansion and the URL-safe alphabet; use an operating-system entropy provider for secrets.', 'import "std/crypto" as crypto\nemit crypto.token("fixture", 24)'),
  extendedFn('uuid_v5', 'uuid_v5(namespace, name)', 'namespace is a UUID string; name is bounded text.', 'string', 'Create a deterministic RFC 4122 version-5 UUID.', 'The namespace is parsed strictly and SHA-1 name UUID semantics are applied with version and variant bits set.', 'import "std/crypto" as crypto\nemit crypto.uuid_v5("6ba7b810-9dad-11d1-80b4-00c04fd430c8", "example")'),
  { ...extendedFn('algorithm_supported', 'algorithm_supported(name)', 'name is a string.', 'bool', 'Report whether a digest algorithm is in the portable crypto contract.', 'The result is fixed by the language implementation and does not inspect host libraries.', 'import "std/crypto" as crypto\nemit crypto.algorithm_supported(crypto.sha512)'), native: nativeLibraryCryptoNative },
];

const extendedSignal = [
  extendedValue('version', 'version', 'string', 'Return the expanded signal-processing contract version.', 'The additive window, statistics, resampling, and transform operations are versioned.', 'import "std/signal" as signal\nemit signal.version'),
  extendedValue('max_samples', 'max_samples', 'u64', 'Return the maximum signal sample count.', 'Signal inputs and outputs are bounded to 65536 samples.', 'import "std/signal" as signal\nemit signal.max_samples'),
  extendedValue('max_transform_samples', 'max_transform_samples', 'u64', 'Return the maximum transform sample count.', 'The explicit DFT reference transform is bounded to 4096 samples.', 'import "std/signal" as signal\nemit signal.max_transform_samples'),
  extendedValue('pi', 'pi', 'number', 'Return the deterministic mathematical constant π.', 'The value is the language runtime constant, independent of host formatting.', 'import "std/signal" as signal\nemit signal.pi'),
  nativeLibrarySignalFn('rectangular', 'rectangular(length)', 'length is a bounded non-negative integer.', 'tensor[length]', 'Create an all-ones rectangular window.', 'The result is owned, rank one, and deterministic.', 'import "std/signal" as signal\nemit signal.rectangular(4)'),
  nativeLibrarySignalFn('hamming', 'hamming(length)', 'length is a bounded non-negative integer.', 'tensor[length]', 'Create a symmetric Hamming window.', 'The standard 0.54 - 0.46*cos formula is evaluated without host signal libraries.', 'import "std/signal" as signal\nemit signal.hamming(4)'),
  nativeLibrarySignalFn('blackman', 'blackman(length)', 'length is a bounded non-negative integer.', 'tensor[length]', 'Create a symmetric Blackman window.', 'The standard fixed-alpha Blackman formula is used with deterministic edge handling.', 'import "std/signal" as signal\nemit signal.blackman(4)'),
  nativeLibrarySignalFn('bartlett', 'bartlett(length)', 'length is a bounded non-negative integer.', 'tensor[length]', 'Create a Bartlett triangular window.', 'The result is deterministic and handles zero and one-length windows explicitly.', 'import "std/signal" as signal\nemit signal.bartlett(4)'),
  nativeLibrarySignalFn('correlate', 'correlate(left, right)', 'left and right are bounded rank-one numeric signals.', 'tensor', 'Compute full discrete cross-correlation.', 'Inputs are not mutated; output shape and sample order are fixed by the reference implementation.', 'import "std/signal" as signal\nemit signal.correlate([1, 2], [3, 4])'),
  nativeLibrarySignalFn('filter_fir', 'filter_fir(signal, coefficients)', 'both arguments are bounded rank-one numeric signals.', 'tensor', 'Apply a finite impulse-response filter.', 'The operation uses direct bounded convolution-style accumulation and does not spawn workers.', 'import "std/signal" as signal\nemit signal.filter_fir([1, 2, 3], [1, 0.5])'),
  extendedFn('resample', 'resample(signal, length)', 'signal is bounded rank-one numeric data; length is bounded.', 'tensor[length]', 'Resample a signal with deterministic linear interpolation.', 'Endpoints are preserved and intermediate samples use a fixed interpolation rule.', 'import "std/signal" as signal\nemit signal.resample([0, 10], 5)'),
  nativeLibrarySignalFn('normalize', 'normalize(signal, mode)', 'signal is bounded numeric data; mode is `peak`, `minmax`, or `zscore`.', 'tensor', 'Normalize signal amplitudes with an explicit mode.', 'Zero-variance and zero-peak inputs produce zero-valued output rather than NaN or an ambient error; all three modes use the same bounded copied input.', 'import "std/signal" as signal\nemit signal.normalize([2, 4], "minmax")'),
  nativeLibrarySignalFn('mean', 'mean(signal)', 'signal is a non-empty bounded numeric signal.', 'number', 'Return the arithmetic mean.', 'Accumulation is deterministic and empty input returns an explicit error.', 'import "std/signal" as signal\nemit signal.mean([1, 2, 3])'),
  nativeLibrarySignalFn('variance', 'variance(signal)', 'signal is a non-empty bounded numeric signal.', 'number', 'Return population variance.', 'The calculation uses the deterministic two-pass reference formula rather than mean-square.', 'import "std/signal" as signal\nemit signal.variance([1, 2, 3])'),
  nativeLibrarySignalFn('energy', 'energy(signal)', 'signal is bounded numeric data.', 'number', 'Return the sum of squared magnitudes.', 'Inputs are copied for the calculation and output is an ordinary scalar.', 'import "std/signal" as signal\nemit signal.energy([1, 2, 3])'),
  nativeLibrarySignalFn('rms', 'rms(signal)', 'signal is a non-empty bounded numeric signal.', 'number', 'Return root-mean-square amplitude.', 'The result is derived from energy and sample count under the same bounds.', 'import "std/signal" as signal\nemit signal.rms([1, 2, 3])'),
  nativeLibrarySignalTransformFn('fft', 'fft(signal)', 'signal is bounded complex or real rank-one data within max_transform_samples.', 'tensor<complex>', 'Compute the forward discrete Fourier transform.', 'This is an explicit deterministic DFT reference implementation; it does not silently select platform FFT or SIMD behavior. Native C copies real, complex-list, real-tensor, or complex-tensor input into owned OComplex staging storage and returns a newly owned rank-one complex tensor.', 'import "std/signal" as signal\nemit signal.fft([1, 0, 0, 0])'),
  nativeLibrarySignalTransformFn('ifft', 'ifft(spectrum)', 'spectrum is bounded complex rank-one data within max_transform_samples.', 'tensor<complex>', 'Compute the inverse discrete Fourier transform.', 'The inverse uses the same fixed transform convention and returns a newly owned rank-one complex tensor. Native C applies the 1/4096 bound before staging and never retains input storage.', 'import "std/signal" as signal\nemit signal.ifft(signal.fft([1, 0, 0, 0]))'),
];

const extendedList = [
  extendedValue('version', 'version', 'string', 'Return the expanded list contract version.', 'The additive list operations and hard bounds are versioned.', 'import "std/list" as list\nemit list.version'),
  extendedValue('max_items', 'max_items', 'u64', 'Return the maximum list item count.', 'List inputs and materialized outputs are bounded to 100000 items.', 'import "std/list" as list\nemit list.max_items'),
  extendedValue('max_depth', 'max_depth', 'u64', 'Return the maximum flattening depth.', 'Recursive flattening is bounded to 64 levels.', 'import "std/list" as list\nemit list.max_depth'),
  extendedFn('empty', 'empty()', 'no arguments.', 'list', 'Create an empty owned list.', 'The result is independent of every prior list operation.', 'import "std/list" as list\nemit list.empty()'),
  extendedFn('repeat', 'repeat(value, count)', 'value is any value; count is bounded and non-negative.', 'list', 'Create a list containing count copies of a value.', 'Allocation is checked against max_items before storage is created.', 'import "std/list" as list\nemit list.repeat("x", 3)'),
  extendedFn('concat', 'concat(lists...)', 'every argument is a bounded list.', 'list', 'Concatenate lists into a new owned list.', 'Inputs are not mutated and total output size is checked before allocation.', 'import "std/list" as list\nemit list.concat([1, 2], [3])'),
  extendedFn('slice', 'slice(list, start, end)', 'list is bounded; start and end are non-negative indexes.', 'list', 'Copy a half-open list range.', 'Out-of-range endpoints are clamped; end before start is rejected and the source remains unchanged.', 'import "std/list" as list\nemit list.slice([0, 1, 2], 1, 3)'),
  extendedFn('at', 'at(list, index)', 'list is bounded; index is non-negative and in range.', 'value', 'Read one list element with explicit bounds checking.', 'The returned value is the element value and no host slice is exposed.', 'import "std/list" as list\nemit list.at([10, 20], 1)'),
  extendedFn('index_of', 'index_of(list, value)', 'list is bounded; value is comparable.', 'i64', 'Return the first matching index or -1.', 'Equality uses the language value comparison contract.', 'import "std/list" as list\nemit list.index_of(["a", "b"], "b")'),
  extendedFn('last_index_of', 'last_index_of(list, value)', 'list is bounded; value is comparable.', 'i64', 'Return the last matching index or -1.', 'Search order is deterministic from the end of the owned input view.', 'import "std/list" as list\nemit list.last_index_of([1, 2, 1], 1)'),
  extendedFn('contains', 'contains(list, value)', 'list is bounded; value is comparable.', 'bool', 'Test whether a list contains a value.', 'The operation stops at the first match and does not allocate a result list.', 'import "std/list" as list\nemit list.contains([1, 2], 2)'),
  extendedFn('count', 'count(list, value)', 'list is bounded; value is comparable.', 'u64', 'Count matching values.', 'The scan is bounded by max_items and equality remains deterministic.', 'import "std/list" as list\nemit list.count([1, 1, 2], 1)'),
  extendedFn('reverse', 'reverse(list)', 'list is bounded.', 'list', 'Return a reversed copy.', 'The input list is not mutated.', 'import "std/list" as list\nemit list.reverse([1, 2, 3])'),
  extendedFn('sort', 'sort(list)', 'list is bounded and contains mutually orderable values.', 'list', 'Return a deterministically sorted copy.', 'Sorting rejects incomparable mixed values and leaves the source unchanged.', 'import "std/list" as list\nemit list.sort([3, 1, 2])'),
  extendedFn('unique', 'unique(list)', 'list is bounded and values are comparable.', 'list', 'Remove duplicate values while preserving first occurrence order.', 'The operation is stable and bounded; it does not mutate the source.', 'import "std/list" as list\nemit list.unique([1, 1, 2])'),
  extendedFn('partition', 'partition(list, index)', 'list is bounded; index is a non-negative integer no greater than the list length.', 'tuple<list,list>', 'Split a list at an index and return two owned lists.', 'The first result is [0,index), the second is [index,len); indexes beyond the input length and other invalid arguments return a structured error, and the input is never mutated.', 'import "std/list" as list\nemit list.partition([1, 2, 3], 2)'),
  extendedFn('zip', 'zip(left, right)', 'left and right are bounded lists.', 'list<tuple>', 'Pair values at matching positions.', 'The result stops at the shorter input and has no alias to either input.', 'import "std/list" as list\nemit list.zip([1, 2], ["a", "b"])'),
  extendedFn('windows', 'windows(list, width[, step])', 'list is bounded; width and step are positive bounded integers.', 'list<list>', 'Return sliding list windows.', 'Windows are copied, ordered, and bounded before allocation.', 'import "std/list" as list\nemit list.windows([1, 2, 3, 4], 2)'),
  extendedFn('chunks', 'chunks(list, width)', 'list is bounded; width is positive.', 'list<list>', 'Split a list into fixed-width chunks.', 'The final chunk may be shorter; every chunk is independently owned.', 'import "std/list" as list\nemit list.chunks([1, 2, 3], 2)'),
  extendedFn('flatten', 'flatten(list[, depth])', 'list is bounded; optional depth is non-negative.', 'list', 'Flatten nested lists to a bounded depth.', 'Depth and output count are checked recursively, and non-list values remain leaves.', 'import "std/list" as list\nemit list.flatten([[1], [2, [3]]])'),
  extendedFn('all', 'all(list)', 'list is bounded and values are bool.', 'bool', 'Return whether every value is true.', 'Empty lists return true by the standard universal-quantification identity.', 'import "std/list" as list\nemit list.all([true, true])'),
  extendedFn('any', 'any(list)', 'list is bounded and values are bool.', 'bool', 'Return whether any value is true.', 'Empty lists return false and evaluation is short-circuiting.', 'import "std/list" as list\nemit list.any([false, true])'),
  extendedFn('none', 'none(list)', 'list is bounded and values are bool.', 'bool', 'Return whether no value is true.', 'This is the complement of any under the same bounded boolean contract.', 'import "std/list" as list\nemit list.none([false, false])'),
  extendedFn('enumerate', 'enumerate(list)', 'list is bounded.', 'list<tuple<u64,value>>', 'Pair each value with its zero-based index.', 'Pairs are emitted in source order and are newly owned.', 'import "std/list" as list\nemit list.enumerate(["a", "b"])'),
];

const extendedRegex = [
  extendedValue('max_pattern_bytes', 'max_pattern_bytes', 'u64', 'Return the maximum regex pattern size.', 'Patterns are bounded to 16 KiB before compilation.', 'import "std/regex" as regex\nemit regex.max_pattern_bytes'),
  extendedValue('max_input_bytes', 'max_input_bytes', 'u64', 'Return the maximum regex input size.', 'Matching input is bounded to 8 MiB.', 'import "std/regex" as regex\nemit regex.max_input_bytes'),
  extendedValue('max_matches', 'max_matches', 'u64', 'Return the maximum match count.', 'Match-producing operations stop with an explicit limit error rather than growing without bound.', 'import "std/regex" as regex\nemit regex.max_matches'),
  extendedValue('max_output_bytes', 'max_output_bytes', 'u64', 'Return the maximum regex output size.', 'Replacement output is capped before a result is returned.', 'import "std/regex" as regex\nemit regex.max_output_bytes'),
  extendedValue('max_split_parts', 'max_split_parts', 'u64', 'Return the maximum split-part count.', 'Split operations are bounded independently of input length.', 'import "std/regex" as regex\nemit regex.max_split_parts'),
  extendedFn('capture_count', 'capture_count(pattern)', 'pattern is bounded RE2 syntax.', 'u64', 'Return the numbered capture count.', 'Compilation is bounded and rejects unsupported or malformed RE2 syntax.', 'import "std/regex" as regex\nemit regex.capture_count("(a)(?<name>b)")'),
  extendedFn('capture_names', 'capture_names(pattern)', 'pattern is bounded RE2 syntax.', 'list<string>', 'Return capture names in numbered order.', 'Unnamed captures are represented by an empty string so indexes remain stable.', 'import "std/regex" as regex\nemit regex.capture_names("(?P<word>a)")'),
  extendedFn('pattern_info', 'pattern_info(pattern)', 'pattern is bounded RE2 syntax.', 'map', 'Return immutable pattern metadata.', 'The map contains pattern, engine version, capture count/names, literal prefix, and whether the prefix is complete.', 'import "std/regex" as regex\nemit regex.pattern_info("abc.*")'),
  extendedFn('find_indices', 'find_indices(pattern, input)', 'pattern and input are bounded strings.', 'list<map>', 'Return whole-match UTF-8 byte ranges.', 'Each result contains start_byte and end_byte; indexes refer to the original input, not rune counts.', 'import "std/regex" as regex\nemit regex.find_indices("a+", "caa")'),
  extendedFn('find_named', 'find_named(pattern, input)', 'pattern and input are bounded strings.', 'list<map>', 'Return matches with numbered and named capture metadata.', 'Capture text and byte ranges are returned as owned maps under the match and output bounds.', 'import "std/regex" as regex\nemit regex.find_named("(?P<word>\\w+)", "hello")'),
  extendedFn('match_count', 'match_count(pattern, input)', 'pattern and input are bounded strings.', 'u64', 'Count bounded non-overlapping matches.', 'The count operation does not materialize match maps.', 'import "std/regex" as regex\nemit regex.match_count("a", "banana")'),
  extendedFn('replace_first', 'replace_first(pattern, input, replacement)', 'pattern, input, and replacement are bounded strings.', 'string', 'Replace only the first match.', 'Replacement syntax follows the bounded RE2 contract and output bytes are capped.', 'import "std/regex" as regex\nemit regex.replace_first("a", "banana", "o")'),
  extendedFn('replace_n', 'replace_n(pattern, input, replacement, count)', 'pattern, input, replacement are bounded; count is non-negative.', 'string', 'Replace at most count matches.', 'A count of zero returns the input copy; count and output size are bounded.', 'import "std/regex" as regex\nemit regex.replace_n("a", "banana", "o", 2)'),
  extendedFn('split_n', 'split_n(pattern, input, parts)', 'pattern and input are bounded; parts is positive.', 'list<string>', 'Split into at most a bounded number of parts.', 'The result retains deterministic empty-field behavior and enforces max_split_parts.', 'import "std/regex" as regex\nemit regex.split_n(",", "a,b,c", 2)'),
  extendedFn('quote_meta', 'quote_meta(text)', 'text is bounded UTF-8.', 'string', 'Escape regex metacharacters in literal text.', 'The returned expression matches the original text literally.', 'import "std/regex" as regex\nemit regex.quote_meta("a+b")'),
  extendedFn('escape', 'escape(text)', 'text is bounded UTF-8.', 'string', 'Alias quote_meta for familiar escaping terminology.', 'The alias has exactly the same bounds and output as quote_meta.', 'import "std/regex" as regex\nemit regex.escape("a+b")'),
];

const extendedJSON = [
  extendedValue('extensions_version', 'extensions_version', 'string', 'Return the version of the extended JSON operations.', 'The value identifies JSONPath, JSON Lines, structural helpers, and schema validation additions beyond the core JSON module.', 'import "std/json" as json\nemit json.extensions_version'),
  extendedFn('type_of', 'type_of(value)', 'value is a JSON-compatible value.', 'string', 'Return the stable JSON kind name.', 'The result is one of null, bool, number, string, array, or object.', 'import "std/json" as json\nemit json.type_of({ready: true})'),
  extendedFn('keys', 'keys(object)', 'object is a JSON object.', 'list<string>', 'Return object keys in insertion order.', 'The result is an owned list and scalar or array inputs are rejected.', 'import "std/json" as json\nemit json.keys({a: 1, b: 2})'),
  extendedFn('length', 'length(array_or_object)', 'value is an array or object.', 'u64', 'Return array element or object member count.', 'Strings and scalars are rejected to avoid ambiguous byte/rune semantics.', 'import "std/json" as json\nemit json.length([1, 2])'),
  extendedFn('equal', 'equal(left, right)', 'both values are JSON-compatible.', 'bool', 'Compare JSON values structurally.', 'Object order is significant for the ordered representation, while equivalent numeric spellings compare equal.', 'import "std/json" as json\nemit json.equal(1, 1.0)'),
  extendedFn('merge', 'merge(base, overlay)', 'both values are JSON-compatible.', 'JSON value', 'Recursively merge two JSON values.', 'Objects merge by key, overlay values replace scalar/array values, and all inputs remain unchanged.', 'import "std/json" as json\nemit json.merge({a: 1}, {b: 2})'),
  extendedFn('query', 'query(value, path)', 'value is JSON-compatible; path is the bounded JSONPath subset `$`, member, index, and wildcard syntax.', 'list<JSON value>', 'Select values with a bounded JSONPath query.', 'Paths, traversal steps, and result counts are capped. Results are detached and preserve object/array order.', 'import "std/json" as json\nemit json.query({users: [{name: "Ada"}]}, "$.users[*].name")'),
  extendedFn('parse_lines', 'parse_lines(data)', 'data is bounded UTF-8 JSON Lines bytes.', 'list<JSON value>', 'Parse one JSON value per non-empty line.', 'CRLF is accepted, blank lines are skipped, line count is bounded, and errors identify the line without returning partial output.', 'import "std/json" as json\nemit json.parse_lines("{\\"id\\":1}\\n{\\"id\\":2}")'),
  extendedFn('stringify_lines', 'stringify_lines(values)', 'values is a bounded list of JSON-compatible values.', 'list<u8>', 'Serialize values as newline-delimited JSON.', 'Each value is compact, one line is emitted per item, and output bytes are bounded.', 'import "std/json" as json\nemit json.stringify_lines([{id: 1}, {id: 2}])'),
];

const extendedI18n = [
  extendedValue('extensions_version', 'extensions_version', 'string', 'Return the version of the extended locale operations.', 'The value identifies the embedded locale, plural, list, currency, and catalog additions beyond the core locale module.', 'import "std/i18n" as i18n\nemit i18n.extensions_version'),
  extendedFn('supported_locales', 'supported_locales()', 'no arguments.', 'list<string>', 'List supported embedded locale tags.', 'Tags are sorted and come from versioned data; host locale installation is never consulted.', 'import "std/i18n" as i18n\nemit i18n.supported_locales()'),
  extendedFn('plural_category', 'plural_category(locale, number)', 'locale is supported; number is bounded decimal text.', 'string', 'Return the locale plural category for a decimal spelling.', 'Rules are deterministic and preserve visible-fraction information without floating-point conversion.', 'import "std/i18n" as i18n\nemit i18n.plural_category("en-US", "2")'),
  extendedFn('format_list', 'format_list(locale, items[, style])', 'locale is supported; items is a bounded list of strings; style is standard or short.', 'string', 'Format a localized conjunction list.', 'Items are treated as already localized text; punctuation and conjunctions come from embedded locale data.', 'import "std/i18n" as i18n\nemit i18n.format_list("en-US", ["a", "b", "c"])'),
  extendedFn('format_currency', 'format_currency(locale, amount, currency)', 'locale is supported; amount is bounded decimal text; currency is a supported code.', 'string', 'Format a currency amount with deterministic locale punctuation.', 'Amount parsing is exact decimal text, currency symbols come from embedded data, and unsupported currencies fail explicitly.', 'import "std/i18n" as i18n\nemit i18n.format_currency("en-US", "12.50", "USD")'),
  extendedFn('format_percent', 'format_percent(locale, amount)', 'locale is supported; amount is a bounded exact decimal ratio.', 'string', 'Format a decimal ratio as a localized percentage.', 'The input 0.125 becomes 12.50% in en-US. Arithmetic is exact decimal/rational work and uses embedded separators.', 'import "std/i18n" as i18n\nemit i18n.format_percent("en-US", "0.125")'),
  extendedFn('format_relative', 'format_relative(locale, value, unit)', 'locale is supported; value is an integer; unit is minute, hour, day, week, month, or year.', 'string', 'Format a signed relative-time distance.', 'Positive values describe the future and negative values the past. Zero uses a locale-specific now phrase; all phrases are embedded and deterministic.', 'import "std/i18n" as i18n\nemit i18n.format_relative("en-US", -2, "day")'),
  extendedFn('format_date_style', 'format_date_style(locale, rfc3339, style)', 'locale is supported; rfc3339 is an RFC3339 instant; style is short, medium, or long.', 'string', 'Format a date with an explicit locale style.', 'The instant is rendered through a fixed UTC view. No host timezone or locale database is consulted.', 'import "std/i18n" as i18n\nemit i18n.format_date_style("en-US", "2024-01-02T03:04:05Z", "long")'),
  extendedFn('catalog_format', 'catalog_format(locale, catalog, key, number[, arguments])', 'locale is supported; catalog is a JSON-like message object; key/number are bounded strings; arguments is an optional string map.', 'string', 'Format a plural-aware message catalog entry.', 'Catalog entries may be singular strings or plural maps. `{count}` and explicit string arguments are expanded under catalog and output bounds.', 'import "std/i18n" as i18n\nemit i18n.catalog_format("en-US", {items: {one: "one item", other: "{count} items"}}, "items", "2")'),
];

export const extendedReferences = [
  { id: 'crypto', title: 'std/crypto', summary: 'Bounded deterministic digests, keyed hashes, encodings, derivation, and UUIDs.', items: extendedCrypto },
  { id: 'signal', title: 'std/signal', summary: 'Bounded deterministic windows, filters, statistics, resampling, and transforms.', items: extendedSignal },
  { id: 'list', title: 'std/list', summary: 'Bounded owned list construction, indexing, transforms, predicates, and grouping.', items: extendedList },
  { id: 'regex', title: 'std/regex', summary: 'Bounded deterministic RE2 introspection and controlled match operations.', items: extendedRegex },
  { id: 'json', title: 'std/json', summary: 'Bounded JSON queries, structural transforms, JSON Lines, and schema helpers.', items: extendedJSON },
  { id: 'i18n', title: 'std/i18n', summary: 'Embedded locale metadata, plural rules, list formatting, and currency formatting.', items: extendedI18n },
];

export const stdlibReferences = [
  { id: 'regex', title: 'std/regex', summary: 'Bounded deterministic RE2 regular expressions.', items: regex },
  { id: 'compress', title: 'std/compress', summary: 'Bounded deterministic compression and safe archive operations.', items: compression },
  { id: 'json', title: 'std/json', summary: 'Canonical bounded JSON parsing, properties compatibility, schema introspection, and validation. `std/config` remains a deprecated compatibility alias.', items: json },
  { id: 'i18n', title: 'std/i18n', summary: 'Embedded versioned locale formatting.', items: i18n },
];

export const databaseAPIReference = { id: 'db', title: 'std/db', summary: 'Capability-gated parameterized database access with deterministic embedded fixtures.', items: db };
export const telemetryContractReference = { id: 'telemetry', title: 'std/telemetry', summary: 'Bounded structured logs, metrics, traces, and deterministic capture.', items: telemetry };
export const stdlibReferenceReference = { id: 'unsafe', title: 'std/unsafe', summary: 'Explicit unsafe.memory capability with bounded opaque buffers and typed views.', items: unsafeMemory };

const testing = [
  fn('assert', 'assert(condition, message?)', 'condition is bool; message is optional text.', 'bool', 'Record a non-fatal assertion in the current language test.', 'A true condition returns true. A false condition returns false, records a failed assertion, and allows the test body to continue. It fails when called outside `osiris test`.', 'import "std/testing" as testing\nemit testing.assert(2 + 2 == 4, "arithmetic")'),
  fn('assert_equal', 'assert_equal(got, want, message?)', 'got and want are any comparable Osiris values; message is optional text.', 'bool', 'Compare two values and record a non-fatal test assertion.', 'Comparison uses the language value equality contract. A mismatch returns false and marks the test failed without aborting the body. The optional message replaces the default `got ..., want ...` detail.', 'import "std/testing" as testing\nemit testing.assert_equal("hello", "hello")'),
  fn('error', 'error(message...)', 'one or more values rendered with their canonical Osiris representation.', 'null', 'Mark the current test as failed while allowing execution to continue.', 'This is the language equivalent of `testing.T.Error`. It records a failure and does not raise a runtime error. It is valid only inside an Osiris language test.', 'import "std/testing" as testing\nemit testing.error("unexpected result")'),
  fn('errorf', 'errorf(format, values...)', 'format is a string; values are rendered using stable language representations.', 'null', 'Mark the current test as failed with a formatted diagnostic.', 'This is the language equivalent of `testing.T.Errorf`. Formatting is deterministic and the test body continues after the call. Use `%v`, `%s`, `%q`, or `%#q` with rendered Osiris values.', 'import "std/testing" as testing\nemit testing.errorf("Hello(%q) returned an invalid value", "Gladys")'),
  fn('fatal', 'fatal(message...)', 'one or more values rendered with their canonical Osiris representation.', 'does not return', 'Fail the current test and stop the current test body immediately.', 'This is the language equivalent of `testing.T.Fatal`. Fixture cleanup still runs. The runner reports a failed test rather than exposing host control-flow details.', 'import "std/testing" as testing\nemit testing.fatal("required fixture is missing")'),
  fn('fatalf', 'fatalf(format, values...)', 'format is a string; values are rendered using stable language representations.', 'does not return', 'Fail the current test with a formatted diagnostic and stop its body.', 'This is the language equivalent of `testing.T.Fatalf`. Cleanup remains scope-bound and the result contains the formatted failure.', 'import "std/testing" as testing\nemit testing.fatalf("wanted %v", 42)'),
  fn('skip', 'skip(message...)', 'one or more values rendered with their canonical Osiris representation.', 'does not return', 'Skip the current test and stop its body.', 'This is the language equivalent of `testing.T.Skip`. A skipped test is a successful runner outcome with `status: "skip"`; fixture cleanup still runs.', 'import "std/testing" as testing\nemit testing.skip("platform feature unavailable")'),
  fn('skipf', 'skipf(format, values...)', 'format is a string; values are rendered using stable language representations.', 'does not return', 'Skip the current test with a formatted reason.', 'This is the language equivalent of `testing.T.Skipf`. The reason is retained in the result error text and the test is not counted as a failure.', 'import "std/testing" as testing\nemit testing.skipf("requires %v", "gpu")'),
  fn('log', 'log(message...)', 'one or more values rendered with their canonical Osiris representation.', 'null', 'Attach a diagnostic log line to the current test result.', 'Logs are retained in deterministic call order and are emitted in JSON test events as `logs`. Logging does not change pass/fail state.', 'import "std/testing" as testing\nemit testing.log("starting fixture")'),
  fn('logf', 'logf(format, values...)', 'format is a string; values are rendered using stable language representations.', 'null', 'Attach a formatted diagnostic log line to the current test result.', 'The formatted line is retained in deterministic call order and does not affect the test outcome.', 'import "std/testing" as testing\nemit testing.logf("items: %v", 3)'),
  fn('name', 'name()', 'no arguments.', 'string', 'Return the stable name of the current expanded test case.', 'The name includes table-driven case expansion naming and is available only during language-test execution.', 'import "std/testing" as testing\nemit testing.name()'),
  fn('failed', 'failed()', 'no arguments.', 'bool', 'Report whether the current test has recorded a failure.', 'The value becomes true after `assert`, `assert_equal`, `error`, `errorf`, `fatal`, or `fatalf` reports failure. It does not become true for a skip.', 'import "std/testing" as testing\nemit testing.error("bad")\nemit testing.failed()'),
  fn('osiris test', 'osiris test [flags] [paths...]', 'exit status and versioned events', 'paths are optional files or directories; flags are explicit command options.', 'Discover and execute Osiris language tests through the public interpreter, VM, or native backend path. With no path, the current directory is the test root, exactly like `osiris test .`.', 'Exit code 0 includes successful tests, skips, and expected failures. Explicit file and directory paths remain supported; directory discovery is recursive and excludes hidden, generated, `.git`, `.osiris`, and test-artifact directories.', 'osiris test -backend=vm -json'),
  fn('colocated test layout', 'module.osr + module.test.osr', 'assembled package test', 'The production file and preferred `.test.osr` sidecar share one filename stem.', 'Run declarations in a sidecar with the neighboring production source available to the test body.', 'The sidecar is the discovered test file; the ordinary source is prepended once for each isolated declaration. Source locations and result attribution remain attached to the sidecar.', 'todo.osr\ntodo.test.osr\nosiris test'),
  fn('legacy test suffix', 'module.osr + module_test.osr', 'assembled package test', 'The compatibility `_test.osr` sidecar uses the same filename-stem association.', 'Keep legacy test files working while migrating to the preferred `.test.osr` spelling.', 'Suffix matching is lower-case and deterministic. A direct production-file argument selects its `.test.osr` sidecar first, then `_test.osr`.', 'osiris test ./todo.osr'),
  fn('test discovery modes', 'osiris test [path...]', 'sorted test inventory', 'Paths may be omitted, point to a directory, point to a test file, or point to a production file with a sidecar.', 'Use the default current-directory root, recursive directory suites, or precise file selection without requiring a dedicated `tests/` directory.', 'All discovered paths are absolute, cleaned, deduplicated, and lexicographically ordered. Existing `./tests` suites continue to work unchanged.', 'osiris test .\nosiris test ./tests\nosiris test ./todo.test.osr'),
  fn('test package assembly', 'production source + test sidecar', 'isolated test execution', 'The sidecar must have an unambiguous neighboring `.osr` source file when package visibility is required.', 'Make ordinary package declarations available to colocated tests without host-language hooks.', 'Each declaration receives a fresh runtime, deterministic providers, capability policy, timeout, cleanup, and selected backend. Orphan test files remain directly runnable and report normal parse/runtime failures.', 'todo.osr\nfunc add(a, b) { return a + b }\ntodo.test.osr\n// osiris:test name=adds\nemit add(2, 3)'),
  fn('test declarations', '// osiris:test name=<name>', 'language test inventory entry', 'The directive is placed in a discovered `*_test.osr` or `*.test.osr` file.', 'Declare a named language test without changing parser grammar.', 'The source location is retained, declaration order is stable, and a fresh deterministic test runtime is used for each expanded case.', '// osiris:test name="adds values"\nemit 1 + 2'),
  fn('test assertions', '// osiris:assert output[0] == <value>', 'assertion result', 'The expression selects an output index, output count, or exact output membership.', 'Compare canonical emitted values with source-located diagnostics.', 'Assertions record expression, pass state, message, and one-based source range. Failed assertions do not silently coerce values.', '// osiris:assert output[0] == "3u64"\nemit 1 + 2'),
  fn('test fixtures', '// osiris:fixture setup|body|cleanup', 'test result', 'Sections contain public Osiris source.', 'Run setup, body, and cleanup with cleanup attempted on every exit path.', 'Fixture output is isolated from the asserted body output. Cleanup is scope-bound and cancellation-aware.', '// osiris:fixture setup\nlet resource = openread("fixture.txt")\n// osiris:fixture cleanup\nclose(resource)'),
  fn('test cases', '// osiris:case name=<case> key=<value>', 'expanded test result', 'Each case supplies bounded template values.', 'Expand one test into deterministic table-driven cases.', 'Cases are expanded in source order and each receives a fresh runtime, so state cannot leak between rows.', '// osiris:case name=two value=2\nemit {{value}} + 1'),
  fn('test golden', '// osiris:golden path=<file>', 'golden artifact comparison', 'The path is an explicit artifact location.', 'Compare stable JSON output snapshots or update them only under explicit update mode.', 'Golden records include schema version, test name, backend, and canonical outputs. Backend metadata differences do not create false drift for equivalent output.', '// osiris:golden path=goldens/add.json\nemit 1 + 2'),
  fn('test benchmark', '// osiris:benchmark warmup=<n> samples=<n>', 'benchmark record', 'Warmups and measured sample counts are bounded integers.', 'Run a benchmark with schema-versioned JSON timing output.', 'Warmups are not recorded; samples and averages are rounded consistently. Startup/parse caveats are retained in the artifact.', '// osiris:benchmark warmup=1 samples=3\nemit 1 + 2'),
  fn('test fuzz', '// osiris:fuzz seed=<n> runs=<n> corpus=<path>', 'fuzz result and optional failure artifacts', 'The body contains `{{input}}`; runs, seed, corpus, and max size are bounded.', 'Run deterministic seeded fuzz cases with shrinking and replay.', 'Corpus order, generated inputs, shrink deletion order, JSON failure records, and replay source paths are deterministic.', '// osiris:fuzz seed=9 runs=4 max_size=8\nemit len({{input}})'),
  fn('test coverage', '-coverage=<path>', 'coverage JSON artifact', 'The path is an explicit output artifact.', 'Write a source-mapped statement/function/branch coverage report.', 'Interpreter coverage uses public runtime traces. VM/native reports retain stable static maps and explicitly state availability when execution probes are unavailable.', 'osiris test -coverage=coverage.json ./tests'),
  fn('test backend', '-backend=<interpreter|vm|native>', 'backend-specific test results', 'The backend selects a public execution contract.', 'Run tests on the selected interpreter, VM, or generated-C native path.', 'Native compilation or execution failures are reported as failures; native tests are never silently rerouted to the interpreter.', 'osiris test -backend=native ./tests'),
  fn('test deterministic', '-deterministic=<bool>', 'deterministic test policy', 'The flag is true by default.', 'Control deterministic discovery, providers, seeds, artifact names, ordering, and JSON output.', 'Deterministic mode does not claim benchmark timing equality across hosts; it does guarantee the test inputs and result ordering.', 'osiris test -deterministic=true ./tests'),
  fn('test JSON events', '-json', 'one schema-versioned JSON event per declaration', 'The flag selects machine-readable stdout.', 'Emit stable test events while keeping diagnostics on stderr.', 'Events include name, file, backend, status, pass state, location, outputs, assertions, artifacts, and public errors as applicable.', 'osiris test -json ./tests'),
];

const nativeLibraryTextNative = 'Native C lowers this pure, host-independent member through the bounded text runtime. Results are owned values or scalars, inputs are not retained, malformed UTF-8 and invalid arguments are structured errors, and no OS capability is required.';
const nativeLibraryListNative = 'Native C directly lowers this pure, host-independent member through the bounded list runtime. Results are newly tracked list or tuple values, inputs are not mutated or retained, invalid kinds, indexes, comparisons, counts, and list bounds are structured errors, and no OS capability is required.';
export const nativeLibraryTextReference = {
  id: 'text',
  title: 'std/text',
  summary: 'Text operations with explicit value, ownership, and error contracts.',
  items: [
    { ...fn('upper', 'upper(value)', 'value is a UTF-8 string.', 'string', 'Convert text to locale-independent uppercase.', 'The Native C result is newly owned and uses one-to-one mappings, preserving `ß` to match the reference runtime.', 'import "std/text" as text\nemit text.upper("Straße")'), native: nativeLibraryTextNative },
    { ...fn('fields', 'fields(value)', 'value is a UTF-8 string.', 'list<string>', 'Split text on Unicode whitespace and discard empty fields.', 'The list and each returned string are newly owned; malformed UTF-8 fails before partial output is exposed.', 'import "std/text" as text\nemit text.fields("  alpha\\u2003beta")'), native: nativeLibraryTextNative },
    { ...fn('join', 'join(separator, values)', 'separator is a string and values is a list of renderable values.', 'string', 'Join rendered values with a separator.', 'The complete result is allocated once after its bounded size is calculated; source values remain caller-owned.', 'import "std/text" as text\nemit text.join("|", ["a", "b"])'), native: nativeLibraryTextNative },
    { ...fn('compare', 'compare(left, right)', 'left and right are strings.', 'i64', 'Return lexicographic ordering as exactly -1, 0, or 1.', 'Native C compares the common prefix and then lengths, matching the reference string ordering.', 'import "std/text" as text\nemit text.compare("z", "aa")'), native: nativeLibraryTextNative },
    { ...fn('equal_fold', 'equal_fold(left, right)', 'left and right are strings.', 'bool', 'Compare strings using Unicode simple case folding.', 'The bounded portable fold table recognizes final sigma, Kelvin sign, and long s equivalences without allocating a folded copy.', 'import "std/text" as text\nemit text.equal_fold("Σ", "ς")'), native: nativeLibraryTextNative },
    { ...fn('count', 'count(value, needle)', 'value and needle are strings.', 'u64', 'Count non-overlapping substring matches.', 'An empty needle counts code-point boundaries plus one; a missing non-empty needle returns zero without unsafe pointer arithmetic.', 'import "std/text" as text\nemit text.count("aaaa", "aa")'), native: nativeLibraryTextNative },
    { ...fn('index', 'index(value, needle)', 'value and needle are strings.', 'option<u64>', 'Return the first matching code-point index.', 'The option contains a code-point index, returns `some(0)` for an empty needle, and owns its result metadata.', 'import "std/text" as text\nemit text.index("a世b", "b")'), native: nativeLibraryTextNative },
    { ...fn('last_index', 'last_index(value, needle)', 'value and needle are strings.', 'option<u64>', 'Return the final matching code-point index.', 'An empty needle returns the code-point length and is handled before the search loop so the C runtime never scans beyond the terminator.', 'import "std/text" as text\nemit text.last_index("é-é", "é")'), native: nativeLibraryTextNative },
    { ...fn('chunks', 'chunks(value, size)', 'value is UTF-8 text and size is a positive exact integer.', 'list<string>', 'Split text into fixed-width Unicode code-point chunks.', 'Native C directly lowers the bounded operation, copies every chunk into owned string storage, and returns a tracked list without requiring a host capability.', 'import "std/text" as text\nemit text.chunks("a世界b", 2)'), native: nativeLibraryTextNative },
    { ...fn('repeat', 'repeat(value, count)', 'value is a string and count is a non-negative integer.', 'string', 'Repeat text count times.', 'The 1 MiB output bound is checked before allocation and the returned string owns its storage.', 'import "std/text" as text\nemit text.repeat("世", 3)'), native: nativeLibraryTextNative },
  ],
};

const nativeLibraryListFn = (name, syntax, parameters, returns, summary, details, example) =>
  ({ ...extendedFn(name, syntax, parameters, returns, summary, details, example), native: nativeLibraryListNative });

export const nativeListReference = {
  id: 'list',
  title: 'std/list',
  summary: 'List transforms, searches, grouping, construction, and predicates.',
  items: [
    nativeLibraryListFn('reverse', 'reverse(values)', 'values is a bounded list.', 'list', 'Return a reversed copy.', 'The source order is copied in reverse into a newly tracked result; the input container is not mutated.', 'import "std/list" as list\nemit list.reverse([1, 2, 3])'),
    nativeLibraryListFn('at', 'at(values, index)', 'values is a bounded list; index is a non-negative integer in range.', 'value', 'Read one list element.', 'The index is checked before access and the returned value does not expose a C pointer or borrowed slice.', 'import "std/list" as list\nemit list.at([10, 20], 1)'),
    nativeLibraryListFn('index_of', 'index_of(values, target)', 'values is a bounded list; target is comparable.', 'i64', 'Return the first matching index or -1.', 'The bounded equality scan stops at the first match and returns a signed language index.', 'import "std/list" as list\nemit list.index_of([1, 2, 1], 1)'),
    nativeLibraryListFn('last_index_of', 'last_index_of(values, target)', 'values is a bounded list; target is comparable.', 'i64', 'Return the last matching index or -1.', 'The scan is deterministic from the beginning while retaining the last match; the input remains unchanged.', 'import "std/list" as list\nemit list.last_index_of([1, 2, 1], 1)'),
    nativeLibraryListFn('contains', 'contains(values, target)', 'values is a bounded list; target is comparable.', 'bool', 'Test whether a value occurs in a list.', 'The operation short-circuits at the first equality and allocates no result list.', 'import "std/list" as list\nemit list.contains([1, 2], 2)'),
    nativeLibraryListFn('count', 'count(values, target)', 'values is a bounded list; target is comparable.', 'u64', 'Count equal values.', 'The equality scan is bounded and returns an unsigned count without changing the source.', 'import "std/list" as list\nemit list.count([1, 1, 2], 1)'),
    nativeLibraryListFn('sort', 'sort(values)', 'values is a bounded list of mutually orderable values.', 'list', 'Return a stable ascending copy.', 'The working copy is sorted with the language comparison contract; incomparable values return a structured error before partial output.', 'import "std/list" as list\nemit list.sort([3, 1, 2])'),
    nativeLibraryListFn('unique', 'unique(values)', 'values is a bounded list of comparable values.', 'list', 'Remove duplicates while preserving first occurrence order.', 'Equality is checked against the emitted prefix and the returned list owns its element array.', 'import "std/list" as list\nemit list.unique([1, 1, 2])'),
    nativeLibraryListFn('chunks', 'chunks(values, width)', 'values is a bounded list; width is a positive integer no greater than max_items.', 'list<list>', 'Split a list into fixed-width chunks.', 'The input is validated before allocation. Each chunk is copied into its own tracked list, the final chunk may be shorter, and neither the input container nor any source element is mutated. Invalid input, non-positive or oversized widths, and allocation failures produce structured errors without requiring a host capability.', 'import "std/list" as list\nemit list.chunks([1, 2, 3, 4, 5], 2)'),
    nativeLibraryListFn('zip', 'zip(left, right)', 'left and right are bounded lists.', 'list<tuple>', 'Pair values at matching positions.', 'The result stops at the shorter list; each pair is marked as a tuple and both pair storage and the outer list are newly tracked.', 'import "std/list" as list\nemit list.zip([1, 2], ["a", "b"])'),
    nativeLibraryListFn('concat', 'concat(lists...)', 'every argument is a bounded list.', 'list', 'Concatenate zero or more lists.', 'The total item count is checked before one result allocation; inputs are copied and never mutated.', 'import "std/list" as list\nemit list.concat([1], [2, 3])'),
    nativeLibraryListFn('repeat', 'repeat(value, count)', 'value is any value; count is a bounded non-negative integer.', 'list', 'Create count copies of a value.', 'The count bound is checked before allocation and the result has no alias to a caller-owned list container.', 'import "std/list" as list\nemit list.repeat("x", 3)'),
    nativeLibraryListFn('empty', 'empty()', 'no arguments.', 'list', 'Create an empty list.', 'The zero-length result is an owned pure value and rejects unexpected arguments.', 'import "std/list" as list\nemit list.empty()'),
    nativeLibraryListFn('all', 'all(values)', 'values is a bounded list of truthy-compatible values.', 'bool', 'Test whether every value is truthy.', 'Empty input returns true and evaluation short-circuits on the first false value.', 'import "std/list" as list\nemit list.all([true, true])'),
    nativeLibraryListFn('any', 'any(values)', 'values is a bounded list of truthy-compatible values.', 'bool', 'Test whether any value is truthy.', 'Empty input returns false and evaluation short-circuits on the first true value.', 'import "std/list" as list\nemit list.any([false, true])'),
    nativeLibraryListFn('none', 'none(values)', 'values is a bounded list of truthy-compatible values.', 'bool', 'Test whether no value is truthy.', 'Empty input returns true and evaluation stops at the first true value.', 'import "std/list" as list\nemit list.none([false, false])'),
    nativeLibraryListFn('enumerate', 'enumerate(values)', 'values is a bounded list.', 'list<tuple<u64,value>>', 'Pair each value with its zero-based index.', 'Pairs are emitted in source order, marked as tuples, and stored in newly tracked allocations.', 'import "std/list" as list\nemit list.enumerate(["a", "b"])'),
  ],
};

export const languageTestReference = { id: 'testing', title: 'std/testing', summary: 'Go-style test context functions plus deterministic discovery, assertions, fixtures, benchmarks, coverage, fuzzing, and goldens.', items: testing };
