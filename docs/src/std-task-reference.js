// Task 120, 121, and 122 are documented one public symbol per entry.  The
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

export const task120References = [
  { id: 'regex', title: 'std/regex', summary: 'Bounded deterministic RE2 regular expressions.', items: regex },
  { id: 'compress', title: 'std/compress', summary: 'Bounded deterministic compression and safe archive operations.', items: compression },
  { id: 'json', title: 'std/json', summary: 'Canonical bounded JSON parsing, properties compatibility, schema introspection, and validation. `std/config` remains a deprecated compatibility alias.', items: json },
  { id: 'i18n', title: 'std/i18n', summary: 'Embedded versioned locale formatting.', items: i18n },
];

export const task121Reference = { id: 'db', title: 'std/db', summary: 'Capability-gated parameterized database access with deterministic embedded fixtures.', items: db };
export const task122Reference = { id: 'telemetry', title: 'std/telemetry', summary: 'Bounded structured logs, metrics, traces, and deterministic capture.', items: telemetry };
export const task126Reference = { id: 'unsafe', title: 'std/unsafe', summary: 'Explicit unsafe.memory capability with bounded opaque buffers and typed views.', items: unsafeMemory };

const testing = [
  fn('osiris test', 'osiris test [flags] [paths...]', 'exit status and versioned events', 'paths are optional files or directories; flags are explicit command options.', 'Discover and execute Osiris language tests through the public interpreter, VM, or native backend path. With no path, the current directory is the test root, exactly like `osiris test .`.', 'Exit code 0 includes successful tests, skips, and expected failures. Explicit file and directory paths remain supported; directory discovery is recursive and excludes hidden, generated, `.git`, `.osiris`, and test-artifact directories.', 'osiris test -backend=vm -json'),
  fn('colocated test layout', 'module.osr + module.test.osr', 'assembled package test', 'The production file and preferred `.test.osr` sidecar share one filename stem.', 'Run declarations in a sidecar with the neighboring production source available to the test body.', 'The sidecar is the discovered test file; the ordinary source is prepended once for each isolated declaration. Source locations and result attribution remain attached to the sidecar.', 'todo.osr\ntodo.test.osr\nosiris test'),
  fn('legacy test suffix', 'module.osr + module_test.osr', 'assembled package test', 'The compatibility `_test.osr` sidecar uses the same filename-stem association.', 'Keep task-123 test files working while migrating to the preferred `.test.osr` spelling.', 'Suffix matching is lower-case and deterministic. A direct production-file argument selects its `.test.osr` sidecar first, then `_test.osr`.', 'osiris test ./todo.osr'),
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

export const task123Reference = { id: 'testing', title: 'osiris test', summary: 'Language-native discovery, assertions, fixtures, benchmarks, coverage, fuzzing, and goldens.', items: testing };
