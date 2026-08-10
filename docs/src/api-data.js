import baseLanguageSections from './language-reference.json' with { type: 'json' };
import manual from './reference-data.json' with { type: 'json' };
import supplemental from './supplemental-reference.json' with { type: 'json' };
import timeReference from './time-reference.json' with { type: 'json' };
import technicalReference from './technical-reference.json' with { type: 'json' };
import constantReference from './constant-reference.json' with { type: 'json' };
import cliReference from './cli-reference.json' with { type: 'json' };
import stdOSReference from './std-os-reference.js';
import { stdlibReferences, databaseAPIReference, telemetryContractReference, languageTestReference, stdlibReferenceReference, extendedReferences, nativeListReference } from './std-task-reference.js';
import { orderReferenceSections } from './reference-order.js';

export const languageSections = orderReferenceSections(
  [...baseLanguageSections, ...supplemental.languageSections],
  'language',
);

const typeExamples = {
  number: 'let foo number = 4\nlet bar = number(4)\nemit foo # 4\nemit bar # 4',
  bool: 'let enabled bool = true\nemit enabled # true',
  byte: 'let value = byte(65)\nemit value # 65u8',
  decimal: 'let value = decimal("1.25")\nemit value # 1.25d',
  rational: 'let value = rational("3/4")\nemit value # 0.75d',
  rat: 'let value = rat("3/4")\nemit value # 0.75d',
  complex: 'let value = complex(3, 4)\nemit value # 3+4i',
  bigint: 'let value = bigint("9007199254740993")\nemit value # 9007199254740993n',
  bigfloat: 'let value = bigfloat("1.25")\nemit value # 1.25bf',
  interval: 'let value = interval(1, 2)\nemit value # [1, 2]',
  modint: 'let value = modint(7, 3)\nemit value # 1 mod 3',
  fixed: 'let value = fixed(1.5)\nemit value # 1.5fixed',
  result: 'let value = ok(4)\nemit value # ok(4)',
  option: 'let value = some(4)\nemit value # some(4)',
  f32: 'let value f32 = f32(4)\nemit value # 4f32',
  f64: 'let value f64 = f64(4)\nemit value # 4f64',
  bf: 'let value = bf16(4)\nemit value # 4bf16',
  i: 'let value = i64(4)\nemit value # 4i64',
  u: 'let value = u64(4)\nemit value # 4u64',
  uf: 'let value = uf64(4)\nemit value # 4uf64',
  tensor: 'let value tensor = tensor([1, 2])\nemit value # tensor([1, 2])',
  matrix: 'let value = matrix([[1, 2], [3, 4]])\nemit value # matrix([[1, 2], [3, 4]])',
  vector: 'let value = vector(1, 2, 3)\nemit value # tensor([1, 2, 3])',
  dual: 'let value = dual(3, 1)\nemit value # dual(3, 1)',
  duration: 'import "std/time" as time\nlet value duration = time.parse_duration("1.5s")\nemit time.duration_string(value) # "1.5s"',
  datetime: 'import "std/time" as time\nlet value datetime = time.date(2024, 2, 29, 0, 0, 0, 0)\nemit time.format(value, time.rfc3339) # "2024-02-29T00:00:00Z"',
  location: 'import "std/time" as time\nlet value location = time.fixed_zone("EST", -18000)\nemit value # fixed_zone("EST", -18000)',
  quantity: '# Quantities are constructed with an explicit unit-bearing expression.\nlet distance = 3 * m\nemit distance # 3m',
  null: 'let empty null = null\nemit empty # null',
  string: 'let label string = "ready"\nemit label # "ready"',
  list: 'let values list = [1, 2, 3]\nemit values # [1u64, 2u64, 3u64]',
  error: 'let problem = error(InvalidArgument, "bad input")\nemit problem.kind # "invalid_argument"',
  stream: 'let input = openread("input.txt")\n# close(input) after use',
  task: 'let pending = spawn(40 + 2)\nemit await(pending) # 42u64',
  function: 'func double(value) => value * 2\nlet callable function = double\nemit callable(3) # 6u64',
  module: 'import "std/math" as math\nemit math.pi',
  enum: 'enum Status { Ready, Done }\nlet status enum = Status.Ready\nemit status # Status.Ready',
  tcp_listener: 'import "std/net" as net\nlet listener = net.tcp_listen("127.0.0.1:0")\n# close(listener) after use',
  simd: 'import "std/math" as math\nlet lanes = math.simd<4, number>(4, [1, 2, 3, 4])\nemit lanes # simd<4, number>(1, 2, 3, 4)',
  quat: 'import "std/math" as math\nlet rotation = math.quat<number>([1, 0, 0, 0])\nemit math.quat_norm(rotation) # 1',
  sparse_matrix: 'import "std/math" as math\nlet matrix = math.sparse_matrix<number>([2, 2], [[0, 1, 3]], "csr")\nemit at(matrix, [0, 1]) # 3',
  sparse_tensor: 'import "std/math" as math\nlet tensor = math.sparse_tensor<number>([2, 2, 2], [[0, 1, 1, 7]])\nemit shape(tensor) # [2u64, 2u64, 2u64]',
  poly: 'import "std/math" as math\nlet polynomial = math.poly<number>([1, 2, 3])\nemit math.poly_eval(polynomial, 2) # 17',
};

function entryFromReference(groupID, item) {
  return {
    kind: item.kind || 'function',
    name: item.name,
    anchor: item.anchor,
    aliases: item.members || [],
    signature: item.syntax,
    description: item.summary,
    parameters: item.parameters || (
      item.members?.length
        ? `Supported family members: ${item.members.map((member) => `\`${member}\``).join(', ')}.`
        : `Arguments and their order are defined by \`${item.syntax}\`.`
    ),
    returns: item.returns,
    returnValue: item.returnValue || item.details?.[0],
    details: item.details,
    errors: item.errors,
    native: item.native,
    example: item.example,
    exampleStatus: groupID === 'io' ? 'sandbox-required' : item.example.startsWith('emit ') ? 'checked' : 'illustrative',
  };
}

// This is the canonical React reference data. It is checked in so the app
// never fetches or executes documentation data at runtime.
const supplementalGroups = new Map(supplemental.groups.filter((group) => group.id !== 'constants').map((group) => [group.id, group]));
// Manual entries provide the baseline; supplemental entries may refine them.
// Merge by public name so a refined entry cannot reach the strict ordering
// check twice and create duplicate routes/search identities.
const mergeReferenceItems = (baseItems = [], supplementalItems = []) => {
  const byName = new Map(baseItems.map((item) => [item.name, item]));
  for (const item of supplementalItems) byName.set(item.name, item);
  return [...byName.values()];
};
const combinedGroups = manual.groups.map((group) => ({
  ...group,
  items: mergeReferenceItems(group.items || [], supplementalGroups.get(group.id)?.items || []),
}));
for (const group of supplemental.groups.filter((candidate) => candidate.id !== 'constants')) {
  if (!combinedGroups.some((candidate) => candidate.id === group.id)) combinedGroups.push(group);
}
combinedGroups.push(timeReference);

const generatedModuleReferenceGroups = [
  ...stdlibReferences,
  ...extendedReferences,
  nativeListReference,
  databaseAPIReference,
  telemetryContractReference,
  languageTestReference,
  stdlibReferenceReference,
];
for (const reference of generatedModuleReferenceGroups) {
  if (!combinedGroups.some((group) => group.id === reference.id)) {
    combinedGroups.push({ id: reference.id, title: reference.title, summary: reference.summary, items: [] });
  }
}
const generatedModuleReferences = new Map();
for (const reference of generatedModuleReferenceGroups) {
  const current = generatedModuleReferences.get(reference.id) || [];
  const byName = new Map(current.map((item) => [item.name, item]));
  for (const item of reference.items) byName.set(item.name, item);
  generatedModuleReferences.set(reference.id, [...byName.values()]);
}

const moduleItems = (group) => {
  if (group.id === 'os') return stdOSReference;
  if (!generatedModuleReferences.has(group.id)) return group.items;
  const byName = new Map((group.items || []).map((item) => [item.name, item]));
  for (const item of generatedModuleReferences.get(group.id)) byName.set(item.name, item);
  return [...byName.values()];
};

const apiModules = combinedGroups.map((group) => ({
  id: group.id,
  label: group.title,
  summary: group.summary,
  importLine: group.title.startsWith('std/') ? `import "${group.title}" as ${group.id}` : '# Built-in reference surface',
  sections: orderReferenceSections(
    [{ title: group.title, entries: moduleItems(group).map((item) => entryFromReference(group.id, item)) }],
    group.id,
  ),
}));

const plannedTypeNames = new Set();
const typeDefinitions = [...manual.types, ...supplemental.types];
const typeEntry = (type, planned = false) => ({
  kind: 'type',
  name: type.name,
  aliases: type.name === 'i' ? ['int'] : [],
  signature: type.name,
  description: planned ? `${type.summary} This name is reserved documentation, not an implemented runtime kind.` : type.summary,
  returns: 'not applicable (type definition)',
  returnValue: planned ? 'No current expression, constructor, annotation, or backend produces this runtime kind.' : 'Use this type name to understand the value family returned by language forms and constructors.',
  example: typeExamples[type.name] || `# ${type.name}: ${type.summary}`,
});

const constantEntry = (entry, errorKind = false, section = {}) => ({
  kind: 'constant',
  name: entry.names[0],
  anchor: entry.anchor,
  aliases: entry.names.slice(1),
  signature: entry.names.join(' | '),
  description: entry.description,
  returns: errorKind ? 'error-kind string' : entry.type,
  returnValue: errorKind ? `The canonical error-kind code \`${entry.code}\`.` : entry.value,
  details: errorKind
    ? [
        `The constant evaluates to \`${entry.code}\`.`,
        `Use \`${entry.names[0]}\` with \`error\`, \`wrap\`, \`matches\`, or \`is\` instead of comparing display text.`,
      ]
    : [
        entry.value,
        section.exactness ? `Representation: ${section.exactness}` : undefined,
        section.source ? `Source: ${section.source}` : undefined,
      ].filter(Boolean),
  example: errorKind
    ? `let problem = error(${entry.names[0]}, "example failure")\nemit problem.kind # "${entry.code}"`
    : entry.example,
  exampleStatus: 'checked',
});

const constantModules = [constantReference.global, constantReference.errors].map((reference) => ({
  id: reference.id,
  label: reference.title,
  summary: reference.summary,
  importLine: reference.importLine,
  sections: orderReferenceSections(
    reference.sections.map((section) => ({
      title: section.title,
      entries: section.entries.map((entry) => constantEntry(entry, reference.id === 'error-constants', section)),
    })),
    reference.id,
  ),
}));

const typeModule = {
  id: 'types',
  label: 'Types',
  summary: 'Language value families and their runtime roles.',
  importLine: '# Types are language-level value families',
  sections: orderReferenceSections([{
    title: 'Numeric literals',
    entries: [{
      name: 'numeric literals',
      signature: '42  0xffu8  -7i16  1.25  1.25f32  1.25bf16  1.25d  2i',
      description: 'Unsuffixed integers are uint64; unsuffixed fractional or exponent values are the default number family. A suffix selects an exact numeric family and remains visible to inference, formatting, codecs, and supported native lowering.',
      parameters: 'Bases: `decimal`, `0b` binary, `0o` octal, and `0x` hexadecimal. `_` separates digits. Integer widths are `8`, `16`, `32`, `64`, `128`, `256`, `512`, and `1024`: use `uN`, `iN`, or packed `ufN`; `u` means `u64`, `b` means `u8`, and `ufnum` selects the native packed family. Floating suffixes are `f8` through `f1024` at those widths and `bf8` through `bf1024`; constructors are narrower than literal syntax. `d` selects decimal and `i` selects a pure imaginary value.',
      returns: 'A runtime numeric value with its declared family and width metadata.',
      returnValue: '`osiris infer` reports `u64`, `u8`, `i16`, `number`, `f32`, `bf16`, `decimal`, or `complex` as appropriate.',
      details: ['Unary - applies after literal parsing; -7i16 is a signed integer expression.', 'Malformed suffixes and digits are parser errors at the numeric token. Explicit suffixes are never silently reinterpreted as another family.'],
      example: 'let count = 42\nlet mask = 0xffu8\nlet ratio = 1.25f32\nlet exact = 1.25d\nemit count, mask, ratio, exact\n# 42u64, 255u8, 1.25f32, 1.25d',
      exampleStatus: 'checked',
    }, {
      name: 'tensor and matrix shape inference',
      signature: 'tensor(values)  matrix(rows, columns, values)  reshape(tensor, shape)  transpose(matrix)',
      description: 'The infer command retains statically derivable tensor dimensions from rectangular literals and shape constructors. Tensor arithmetic uses right-aligned broadcasting, matching runtime behavior regardless of operand order.',
      parameters: 'Literal dimensions and rectangular nested lists preserve shapes. Dynamic dimensions or incompatible broadcast shapes remain tensor with an unspecified shape rather than receiving a guessed result.',
      returns: 'osiris infer reports tensor[dimensions], for example tensor[2x2], when shape proof succeeds.',
      returnValue: 'Shape inference is conservative analysis; runtime and native lowering remain responsible for exact validation.',
      details: ['tensor([10, 20]) + tensor([[1, 2], [3, 4]]) infers tensor[2x2].', 'reshape with a literal shape and rank-two transpose retain their resulting dimensions.'],
      example: 'let vector = tensor([10, 20])\nlet matrix = tensor([[1, 2], [3, 4]])\nlet sum = vector + matrix\n# osiris infer reports: main.sum: tensor[2x2]',
      exampleStatus: 'checked',
    }],
  }, {
    title: 'Runtime type reference',
    entries: typeDefinitions.filter((type) => !plannedTypeNames.has(type.name)).map((type) => typeEntry(type)),
  }, {
    title: 'Reserved future type names',
    entries: typeDefinitions.filter((type) => plannedTypeNames.has(type.name)).map((type) => typeEntry(type, true)),
  }], 'types'),
};

const toolingModule = {
  id: 'tooling',
  label: 'Tooling',
  summary: 'Compiler inspection and deterministic optimization reporting commands.',
  importLine: '# Commands run through the osiris CLI',
  sections: orderReferenceSections([{
    title: 'Release provenance',
    entries: [{
      name: 'FFI manifest ABI v1',
      signature: 'FFIManifest.Validate(grants) / LoadFFIManifest(path, grants)',
      description: 'Validates a project-local, digest-pinned C ABI manifest before any native library is opened. Hosted, VM, and generated-C execution use the same declaration-driven scalar, string, constant, and bounded-buffer ABI; richer fixed-record, opaque-handle, and scoped-callback forms are explicitly bounded by backend support.',
      parameters: 'The manifest declares an absolute library path, SHA-256 digest, platform/architecture, capabilities, C symbols, exact scalar/string/buffer/opaque-handle types, calling convention, thread affinity, and ownership.',
      returns: 'A validated manifest or a security/ABI diagnostic. No raw pointer or integer address is returned.',
      returnValue: 'The direct parity slice supports exact scalar widths, borrowed NUL-free strings, manifest constants, borrowed copy-in `const_buffer`, and bounded mutable `buffer` arguments with integer/bool/void results. Hosted and VM additionally support fixed-layout structs, opaque typed handles, and a scoped synchronous i64 callback ABI.',
      details: ['String arguments are copied to temporary NUL-terminated C storage and string results are copied immediately. Immutable and mutable buffers cross as explicit pointer/length pairs; mutable buffers are copied back only after a successful call.', 'Dynamic search paths, environment-driven loader fallback, arbitrary symbols, network acquisition, retained callbacks, re-entrancy, and background threads are denied by the design. Every result declares ownership. A library cannot close while a call or opaque handle remains active.'],
      example: '{"abi_version":1,"library":"example-math","sha256":"…","capabilities":["ffi.math"],"functions":[{"name":"add_f64","args":["f64","f64"],"result":"f64","ownership":"borrowed"}]}',
    }, {
      name: 'std/net expanded host networking',
      signature: 'udp_bind / udp_connect / udp_send / udp_receive / dns_lookup / tls_dial / http_get / http_request / http_serve',
      description: 'Provides bounded, cancellable hosted networking beyond TCP. HTTP/1.1 is supported; HTTP/2, QUIC, WebSockets, proxy discovery, and insecure TLS modes are not included.',
      parameters: 'UDP is capped at 1 MiB per packet; HTTP headers are capped at 256 entries and bodies at 8 MiB; redirects are capped at five. `udp_receive` returns bytes, peer address, and truncation status. `dns_lookup(host, policy)` accepts the explicit `system` or `go` resolver policy and sorts/deduplicates addresses.',
      returns: 'Owned UDP or TLS stream resources, deterministic DNS address lists, HTTP response maps with sorted header pairs, or a cancellable HTTP server.',
      returnValue: 'Resource-returning operations transfer one explicit handle to the caller, which should register `defer { close(handle) }` immediately after successful acquisition. `http_serve(listener, handler)` passes a bounded request map to a serialized handler and accepts a bounded response map; the static body form remains available for fixtures.',
      details: ['TLS uses certificate and hostname verification, TLS 1.2 minimum, explicit roots when configured, and HTTP/1.1 ALPN. Invalid header names/control characters, oversized bodies, and unsafe TLS configuration fail before host I/O is exposed.', 'Native C rejects these host resources with a source-spanned capability diagnostic.'],
      example: 'import "std/net" as net\nlet socket = net.udp_bind("127.0.0.1:0")\ndefer { close(socket) }\n# udp_receive(socket, 65536, 1000) -> (bytes, peer, truncated)\n# http_serve(listener, handler) -> handler(request) -> {status, headers, body}',
    }, {
      name: 'Compatibility and deprecations',
      signature: 'OSIRIS_SUPPRESS_WARNINGS=OSIRIS-Dnnn osiris <command>',
      description: 'Defines how stable language, CLI, ABI, and artifact contracts evolve across releases. Stable changes follow SemVer; partial and experimental support-matrix entries explicitly have narrower guarantees.',
      parameters: 'Set OSIRIS_SUPPRESS_WARNINGS to one comma-separated stable warning code or all for temporary local suppression. Suppression hides a warning only; it never preserves removed behavior.',
      returns: 'Deprecation warnings use stable OSIRIS-Dnnn codes, name the replacement, and state the earliest removal major version.',
      returnValue: 'Stable features are warned for at least one minor release before removal and are removed only in a major release with a migration note.',
      details: ['Release notes use Added, Changed, Deprecated, Removed, Fixed, and Security categories. Breaking releases include before/after migration guidance.', 'There are no active deprecations in 0.1.0. Release policy is maintained with the project’s published version history.'],
      example: 'OSIRIS_SUPPRESS_WARNINGS=OSIRIS-D001 osiris run program.osr\n# Suppresses only warning OSIRIS-D001 for this invocation environment.',
    }, {
      name: 'osiris version',
      aliases: ['version'],
      signature: 'osiris version',
      description: 'Prints the provenance of the installed CLI for release verification and actionable bug reports.',
      parameters: 'No arguments. Release builds inject version, commit, and build date at link time; development builds state unknown metadata explicitly.',
      returns: 'Plain text containing version, commit, build timestamp, Go toolchain, and OS/architecture target.',
      returnValue: 'Use it after verifying a release archive checksum to confirm the intended executable is on PATH.',
      details: ['Unsigned dry-run archives use -trimpath and SHA-256 checksums. Signing and publishing are deliberately separate trusted-release operations.', 'run and vm need no C compiler. build/compile require a compatible system C compiler; some numerical decompositions also require an explicitly configured provider.'],
      example: 'osiris version\n# osiris 0.1.0\n# commit: abcdef123456\n# built: 2026-07-24T00:00:00Z\n# go: go1.26.x\n# target: linux/amd64',
    }],
  }, {
    title: 'Editor protocol',
    entries: [{
      name: 'osiris lsp',
      aliases: ['lsp'],
      signature: 'osiris lsp',
      description: 'Starts the Osiris Language Server Protocol endpoint over standard input and output. It provides source-aware declaration and definition navigation, document highlights, completion, signature help, symbols, semantic tokens, diagnostics, workspace queries, hierarchy queries, formatting, code actions, code lenses, test inventory, executable test results, and refactoring for open documents.',
      parameters: 'No command arguments. The client sends JSON-RPC 2.0 messages framed with Content-Length headers. Standard output is reserved for protocol messages; diagnostics and malformed-change notices use the server log stream.',
      returns: 'An LSP session advertising incremental synchronization plus the implemented semantic, workspace, refactoring, formatting, hierarchy, and test-tooling providers. Unknown methods return JSON-RPC -32601 rather than an empty success.',
      returnValue: 'All LSP positions and ranges use zero-based UTF-16 code units. The implementation converts these to the lexer’s UTF-8 byte-column source map, so astral Unicode characters have correct editor locations.',
      details: [
        'Supported methods include document and workspace symbols, hover, declaration and definition, type definition, implementations, references, completion, signature help, rename, semantic tokens, inlay hints, folding and selection ranges, code actions, formatting, pull diagnostics, code lenses, call and type hierarchy, osiris/testInventory, and osiris/testResults. textDocument/declaration and textDocument/definition share the analyzer source identity while preserving their standard navigation method names. textDocument/inlayHint returns checked type labels in a requested UTF-16 range; textDocument/foldingRange returns deterministic region and consecutive-comment folds; textDocument/selectionRange returns one identifier-or-point, line, and document selection tree per input position. All three validate document versions, malformed ranges/positions, UTF-16 boundaries, cancellation, and deterministic ordering. textDocument/diagnostic returns exactly {kind: "full", items: [...]} with sorted defensive snapshots, UTF-16 ranges, stale-version rejection, malformed-request errors, and cancellation behavior. osiris/testResults executes shared discovered tests against open-document overlays and returns per-test status, outputs, assertions, logs, locations, errors, and runtime duration; durationNS is intentionally a measurement and may vary between runs. Exact result shapes are documented in docs/design/LSP.md.',
        'didChange is a notification with no JSON-RPC response. It applies multiple LSP incremental changes in order to a private snapshot; an omitted range replaces the complete document. Invalid ranges, cancellation, and duplicate or older non-zero versions reject the complete batch before publication, leaving the prior overlay and version unchanged.',
        'Optional textDocument.version assertions on version-sensitive requests reject stale edits with -32602. Rename and organize-import actions return versioned WorkspaceEdit.documentChanges sorted deterministically by URI and descending edit position.',
        'source.organizeImports sorts a leading import block by module path and alias, returns only the changed import spans, preserves the rest of the document, and refuses to move imports that follow another declaration. The action is versioned and UTF-16-correct; textDocument/formatting remains the whole-document formatter.',
        'Workspace roots are deduplicated and sorted; indexing is bounded, does not follow symlink directories, preserves malformed files with diagnostics, and gives open-document overlays precedence. Flat textDocument/documentSymbol and workspace/symbol results include declaration-level definitions but omit function-parameter definitions; parameters remain available through lexical navigation, completion, references, rename, and signature help. Test inventory uses the shared colocated/default-root discovery parser and includes package/module ownership metadata.',
        'Relative user-module imports are source-backed only when resolveModulePath finds a readable, parseable file inside a configured workspace root. Explicit export statements provide shared symbolDef identities across importer and defining documents for completion, hover, definition, and cross-document references. Virtual stdlib exports, unresolved repository modules, unparseable files, and out-of-root paths remain unclaimed. The user-module protocol fixture and LSP tests cover UTF-16, stale, malformed, cancellation, and deterministic framed behavior.',
        'Repository imports may resolve through the runtime RepositoryProvider registry and its verified lock/cache boundary when a provider and selector are available. The LSP passes cancellation through resolution, parses selected source without executing it, indexes explicit exports, and uses deterministic osiris://repository virtual definition URIs. Provider failures, unavailable cached modules, malformed source, and canceled resolution remain explicit non-results; lsp-workspace-repository.jsonl and the provider-backed semantic test cover selector propagation and lock-state evidence.',
        'textDocument/codeLens and osiris/testInventory resolve the same name.test.osr/name_test.osr sidecar. An open sidecar overlay wins over disk, benchmark metadata selects osiris.benchmark, configured roots reject lexical or symlink-resolved escapes, and cancellation returns -32800 before partial metadata is published. osiris/testResults uses that inventory, executes interpreter, VM, or Native C tests when requested, projects structured results into JSON-safe records, and preserves the overlay snapshot during execution.',
        'textDocument/semanticTokens/full returns exactly {data: number[]} using the advertised legend and LSP delta encoding. Keywords, operators, literals, comments, definitions, and resolved references receive stable classifications; source spelling preserves UTF-16 lengths for astral Unicode and escaped strings. Missing, malformed, unknown-document, and stale requests return -32602. Generated spans are validated against UTF-16 boundaries and legend indices before publication. Results are deterministic, capped at 10,000 tokens, and cancellation-aware through lexing, encoding, and final publication; cancellation returns -32800 without partial data.',
        'textDocument/signatureHelp returns one deterministic checked signature with declared parameter labels and a zero-based activeParameter. It validates required textDocument and position fields, UTF-16 boundaries, and stale textDocument.version values with -32602; cancellation is checked before, during, and after the scan and returns -32800 without partial output or overlay mutation. lsp-signature-help.jsonl covers valid opening-parenthesis and argument positions, malformed and stale requests, and repeated framed-response equality.',
        'textDocument/references requires a valid open-document URI and UTF-16 position, rejects missing, out-of-document, surrogate-splitting, and stale requests with -32602, and returns definition/reference locations ordered by URI and UTF-16 range. The context-aware scan checks cancellation before, during, and after collection and returns -32800 without partial locations. The exact contract is documented in docs/design/LSP-REFERENCES.md and exercised by lsp-references.jsonl.',
        'textDocument/documentHighlight requires a strict textDocument object and UTF-16 position, rejects missing fields, out-of-document and surrogate-splitting positions, unknown documents, and stale versions with -32602, and returns only same-document occurrences of the shared analyzer identity. Definitions and assignment targets are write highlights; resolved reads are read highlights. Ranges are deduplicated, UTF-16-correct, and deterministically sorted. Cancellation returns -32800 before a private result is published. The exact contract is documented in docs/design/LSP-HIGHLIGHTS.md and exercised by lsp-document-highlight.jsonl.',
        'Named product patterns are source-backed semantic nodes: Point { x: x, label: label } resolves the nominal Point type, each declared field, and each nested arm-local binding through shared analyzer identities. Enum patterns likewise resolve a narrow reference for Status and a qualified case reference for Status.Ready, so type and case cursors navigate independently. Unknown product types or fields produce source-spanned diagnostics; valid nested patterns preserve UTF-16 hover, definition, and reference ranges without executing code. Malformed or partial source remains open and publishes a diagnostic. Definitions for builtins and generated symbols have no source location and return null rather than a fabricated zero range. Canceled requests use -32800 and canceled analysis cannot replace a newer document.',
        'Request lifecycle is concurrent at the input boundary: each request receives a private cancellation context and is registered by JSON-RPC id until its response is published. A $/cancelRequest notification cancels the matching active request, has no response of its own, and ignores unknown ids. Response frames remain serialized and are released in request order; shutdown waits for earlier requests, while exit and session cancellation drain active request goroutines. The exact lifecycle and transaction boundary are documented in docs/design/LSP.md.',
      ],
      example: 'osiris lsp\n# Client request (Content-Length framing omitted):\n{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n# Result capabilities: incremental sync, hover, definition, references, completion, signature help, rename',
    }, {
      name: '$/cancelRequest',
      signature: '$/cancelRequest { id }',
      description: 'Cancels an active JSON-RPC request without creating a response for the cancellation notification.',
      parameters: 'id is the exact JSON-RPC request identifier previously sent by the client. The notification is valid only for an active request; unknown identifiers are ignored and malformed parameters are logged.',
      returns: 'No response. The canceled request, when it reaches a context-aware transaction boundary, returns JSON-RPC error -32800 with no partial result.',
      returnValue: 'The stdio reader routes the notification to the active-request registry while request analysis may be running. The matching child context is canceled, response writes remain serialized and request-ordered, and shutdown/exit drain active request lifecycles before the session ends.',
      details: ['Cancellation is cooperative: pure synchronous work is not interrupted in the middle of a mutation. Context-aware handlers check before lookup, during bounded traversal where applicable, and immediately before publishing a complete result.', 'The wire and lifecycle contract is covered by lsp_request_cancellation_test.go and documented in docs/design/LSP.md.'],
      example: '{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":17}}',
    }, {
      name: 'textDocument/hover',
      signature: 'textDocument/hover { textDocument.uri, position }',
      description: 'Resolves the source-aware symbol or reference under a UTF-16 cursor position and returns Markdown hover content.',
      parameters: 'textDocument.uri is required and must identify an open document. position is zero-based UTF-16; astral characters consume two code units and positions splitting a surrogate pair are invalid.',
      returns: 'A Hover object with Markdown contents and an optional UTF-16 range, or null when no analyzed source identity is under the cursor.',
      returnValue: 'The wire request uses languageServer.hoverContext. It checks cancellation before lookup, after analysis, and before publication. Cancellation returns -32800 with no partial Hover and does not mutate the document overlay.',
      details: ['Definitions without a source-backed span, including generated builtins and virtual standard-library members, do not produce fabricated zero ranges. Hover identity is shared with definition, completion, and references.', 'The compatibility languageServer.hover wrapper uses a background context for existing in-process callers; LSP clients always receive the request context. The deterministic normal fixture and lsp_core_navigation_cancellation_test.go cover repeated results, UTF-16 conversion, and late cancellation.'],
      example: '{"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"textDocument":{"uri":"file:///tmp/demo.osr"},"position":{"line":2,"character":5}}}',
    }, {
      name: 'textDocument/definition',
      signature: 'textDocument/definition { textDocument.uri, position }',
      description: 'Returns the source-backed definition targeted by the analyzed symbol under a UTF-16 cursor position.',
      parameters: 'textDocument.uri is required and must identify an open document. position is zero-based UTF-16 and must map to a valid source position.',
      returns: 'A Location, or null when the analyzed target is unresolved, generated, virtual, or has no valid source span.',
      returnValue: 'The wire request uses languageServer.definitionContext. Cross-document range conversion is completed in private storage before the final cancellation check; cancellation returns -32800 with no partial Location.',
      details: ['The target URI and range use the defining document’s current open overlay when available, so UTF-16 conversion is correct across files. Definition identity comes from the analyzer rather than textual matching.', 'The compatibility languageServer.definition wrapper preserves direct internal callers while Serve passes its request context to definitionContext. The normal protocol fixture and lsp_core_navigation_cancellation_test.go cover deterministic results and canceled dispatch.'],
      example: '{"jsonrpc":"2.0","id":7,"method":"textDocument/definition","params":{"textDocument":{"uri":"file:///tmp/demo.osr"},"position":{"line":3,"character":12}}}',
    }, {
      name: 'textDocument/declaration',
      signature: 'textDocument/declaration { textDocument.uri, position, version? }',
      description: 'Navigates from a UTF-16 cursor position to the source-backed declaration of the analyzed symbol or reference.',
      parameters: 'textDocument.uri is required and must identify an open document. An optional textDocument.version must match the current overlay. position is required, zero-based UTF-16, and may not be outside the document or split an astral-character surrogate pair.',
      returns: 'A single Location or null. Osiris uses one source-backed analyzer identity for declarations and definitions, so a valid declaration result has the same URI and range as textDocument/definition.',
      returnValue: 'The request is evaluated by languageServer.declarationContext, which validates the UTF-16 position before delegating to the shared source-definition resolver and checks cancellation before publication. The compatibility languageServer.declaration wrapper uses a background context for direct in-process callers. A canceled request returns -32800 with no partial location.',
      details: ['Generated, virtual, unresolved, and zero-span identities return null instead of a fabricated location. The defining document’s current open overlay is used for cross-document UTF-16 range conversion.', 'Missing or malformed parameters, unknown documents, stale versions, negative or out-of-document positions, and surrogate-splitting positions return JSON-RPC -32602. TestLSPDeclarationNavigationMatchesSourceDefinition, TestLSPDeclarationNavigationValidatesRequests, TestLSPDeclarationNavigationCancellationIsTransactional, and the declaration request in lsp-protocol.jsonl cover the exact result, validation, cancellation, and deterministic framing contract. The complete method contract is documented in docs/design/LSP-DECLARATION.md.'],
      example: '{"jsonrpc":"2.0","id":36,"method":"textDocument/declaration","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":7},"position":{"line":3,"character":12}}}',
    }, {
      name: 'textDocument/typeDefinition',
      signature: 'textDocument/typeDefinition { textDocument.uri, position, version? }',
      description: 'Resolves the nominal source-backed type declaration for the symbol or reference under a UTF-16 cursor position.',
      parameters: 'textDocument.uri is required and must identify an open document. An optional textDocument.version must match the current overlay. position is required, zero-based UTF-16, and may not be outside the document or split an astral-character surrogate pair.',
      returns: 'A Location for the resolved struct, enum, trait, type alias, or newtype declaration, or null when no source-backed type identity is available.',
      returnValue: 'The request is evaluated by languageServer.typeDefinitionContext. It validates the URI, stale version, and UTF-16 position before semantic lookup; cancellation is checked before lookup, after reference resolution, during candidate collection, after deterministic sorting, and immediately before returning the location. A canceled request returns -32800 with no partial result.',
      details: ['A cursor directly on a source-backed nominal type reference returns that reference target’s declaration. A typed field or method result is resolved through its declared type name. Generated, virtual, unresolved, and invalid spans return null rather than a fabricated range.', 'The compatibility languageServer.typeDefinition wrapper uses a background context for direct internal callers; JSON-RPC requests always use the live request context. URI ordering and source-span ordering make duplicate declarations deterministic. Missing position, malformed parameters, stale versions, out-of-document positions, and surrogate-splitting positions return -32602.', 'TestLSPTypeNavigationContextResultsAreDeterministicAndUTF16Safe, TestLSPTypeNavigationCancellationIsTransactional, and TestLSPTypeNavigationRequestValidationAndContextDispatch cover normal, cancellation, malformed, stale, UTF-16, and repeated-request behavior.'],
      example: '{"jsonrpc":"2.0","id":30,"method":"textDocument/typeDefinition","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":7},"position":{"line":4,"character":17}}}',
    }, {
      name: 'textDocument/implementation',
      signature: 'textDocument/implementation { textDocument.uri, position, version? }',
      description: 'Finds source-backed function and trait-method definitions implementing the analyzed callable under a UTF-16 cursor position.',
      parameters: 'textDocument.uri is required and must identify an open document. An optional textDocument.version must match the current overlay. position is required, zero-based UTF-16, and must map to a valid source position without splitting an astral-character surrogate pair.',
      returns: 'A deterministically ordered Location array. The array is empty when the cursor has no analyzed callable identity or no source-backed implementations.',
      returnValue: 'The request is evaluated by languageServer.implementationsContext. It checks cancellation before document lookup, after reference resolution, during every document and definition traversal, after complete sorting, and before publication. Cancellation returns -32800 with no partial locations and does not mutate the document overlay.',
      details: ['Results include only valid source spans whose definitions are classified as function or concrete method, depending on the callable identity under the cursor. Trait-method requests select implementing method definitions rather than returning the trait declaration itself. Duplicate URI/span identities are removed. Locations are ordered by URI, start line, start character, end line, and end character so repeated requests have byte-for-byte equivalent JSON results.', 'The compatibility languageServer.implementations wrapper uses a background context for direct internal callers; JSON-RPC requests use the live request context. Missing position, malformed parameters, stale versions, out-of-document positions, and surrogate-splitting positions return -32602.', 'TestLSPImplementationProtocolFixtureIsDeterministicAndValidatesRequests, TestLSPTypeNavigationContextResultsAreDeterministicAndUTF16Safe, TestLSPTypeNavigationCancellationIsTransactional, and TestLSPTypeNavigationRequestValidationAndContextDispatch cover normal, race, cancellation, malformed, stale, UTF-16, duplicate, and deterministic-order behavior.'],
      example: '{"jsonrpc":"2.0","id":31,"method":"textDocument/implementation","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":7},"position":{"line":1,"character":20}}}',
    }, {
      name: 'textDocument/documentHighlight',
      signature: 'textDocument/documentHighlight { textDocument: { uri, version? }, position: { line, character } }',
      description: 'Returns deterministic read/write highlights for every occurrence of the analyzed symbol under the UTF-16 cursor in the current document.',
      parameters: 'textDocument and position are required objects. textDocument.uri must identify an open document; an optional textDocument.version must equal the current overlay version. position.line and position.character are required non-negative integers in zero-based UTF-16 units and may not split an astral-character surrogate pair.',
      returns: 'A DocumentHighlight array sorted by UTF-16 range. Each item contains range and kind, where kind is write for a definition or assignment target and read for a resolved use.',
      returnValue: 'languageServer.documentHighlightsContext resolves the shared analyzer symbol identity, filters occurrences to the requested document, deduplicates ranges, converts UTF-8 source spans to UTF-16, checks cancellation during collection and before publication, and rechecks the optional version against the live overlay. No match returns an empty array; cancellation returns -32800 with no partial result.',
      details: ['Malformed or missing required fields, unknown documents, stale versions, out-of-document positions, and surrogate-splitting positions return JSON-RPC -32602. Generated or virtual definitions never create fabricated ranges.', 'The direct documentHighlights wrapper uses a background context for in-process callers; JSON-RPC dispatch uses the live request context. TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests, TestLSPDocumentHighlightsAreDeterministicAndConcurrent, and TestLSPDocumentHighlightsCancellationAndStaleVersionAreTransactional cover capability advertisement, deterministic normal/race behavior, exact UTF-16 ranges, read/write kinds, malformed and stale validation, and cancellation. The full contract is documented in docs/design/LSP-HIGHLIGHTS.md.'],
      example: '{"jsonrpc":"2.0","id":32,"method":"textDocument/documentHighlight","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":7},"position":{"line":3,"character":12}}}',
    }, {
      name: 'textDocument/completion',
      signature: 'textDocument/completion { textDocument.uri, position }',
      description: 'Returns deterministic source, typed-member, and standard-module completion items for a UTF-16 cursor position.',
      parameters: 'textDocument.uri is required and must identify an open document. position is zero-based UTF-16. A cursor after a typed receiver dot requests checked members; other positions enumerate lexical definitions.',
      returns: 'An alphabetically sorted completion-item array. Each item includes its label, LSP kind, optional detail, and source identity data when applicable.',
      returnValue: 'The wire request uses languageServer.completionContext. Cancellation is checked around typed-member lookup, module statements and members, lexical definitions, sorting, and publication; a canceled request returns -32800 and a nil result rather than partial items.',
      details: ['Completion uses the same analyzer identity as hover and definition. Standard-library module members are virtual and are not treated as editable source; source-backed user-module members may carry a data URI and name for client follow-up.', 'The compatibility languageServer.completion wrapper uses a background context for direct callers. Serve passes the live request context. Repeated normal calls and canceled framed dispatch are covered by lsp_core_navigation_cancellation_test.go and the deterministic LSP protocol fixture.'],
      example: '{"jsonrpc":"2.0","id":11,"method":"textDocument/completion","params":{"textDocument":{"uri":"file:///tmp/demo.osr"},"position":{"line":3,"character":4}}}',
    }, {
      name: 'textDocument/formatting',
      signature: 'textDocument/formatting { textDocument.uri, version? }',
      description: 'Formats one open Osiris document and returns a client-applied full-document edit.',
      parameters: 'textDocument.uri is required. An optional textDocument.version is compared with the current open-document version before formatting.',
      returns: 'An array containing zero edits when the document is already formatted, or one full-document TextEdit with UTF-16 range coordinates and the formatted source.',
      returnValue: 'Formatting never mutates the server overlay. A stale version or malformed request returns -32602; cancellation returns -32800 without a partial edit. The pure formatter is checked for cancellation before and after its synchronous pass.',
      details: ['The edit range spans the complete current document, so clients should apply it atomically. `textDocument/codeAction` is the narrower operation for import-only edits.', 'TestLSPFormattingCancellationIsTransactional covers cancellation, exact -32800 behavior, and the unchanged open document. The framed lsp-formatting.jsonl fixture covers exact edits, repeated responses, UTF-16 astral ranges, already-formatted empty results, stale versions, malformed typed parameters, and the current missing-textDocument -32603 dispatch gap. The complete request/result/error contract is documented in docs/design/LSP.md.'],
      example: '{"jsonrpc":"2.0","id":26,"method":"textDocument/formatting","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":4}}}',
    }, {
      name: 'typeHierarchy/prepare',
      signature: 'typeHierarchy/prepare { textDocument: { uri, version? }, position: { line, character } }',
      description: 'Prepares a source-backed nominal type item for type-hierarchy traversal at a UTF-16 cursor position.',
      parameters: 'textDocument.uri and position are required; an optional textDocument.version must match the open overlay. The cursor uses zero-based UTF-16 code units.',
      returns: 'An array containing one type-hierarchy item with name, numeric kind, uri, range, selectionRange, and data, or an empty array when the cursor is not on a nominal type.',
      returnValue: 'The item is resolved from the analyzer definition or reference identity. Ranges are source-backed and UTF-16-correct; the server does not fabricate locations for unresolved or generated types.',
      details: ['Malformed position types and stale versions return -32602; canceled requests return -32800 without a partial item array. A cursor inside an astral literal is converted safely and produces no type item.', 'The current value-shaped position field means an omitted position is interpreted as line 0, character 0 rather than rejected; this is a documented implementation gap. Surrogate-split rejection is not claimed for this method.', 'lsp-type-hierarchy.jsonl and TestLSPTypeHierarchyProtocolFixtureIsExactDeterministicAndValidated cover exact item fields, deterministic repeated responses, UTF-16 coordinates, malformed and stale requests.'],
      example: '{"jsonrpc":"2.0","id":23,"method":"typeHierarchy/prepare","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":7},"position":{"line":1,"character":8}}}',
    }, {
      name: 'typeHierarchy/supertypes',
      signature: 'typeHierarchy/supertypes { item: { name, uri, range, selectionRange, data? } }',
      description: 'Returns the traits implemented by a selected source-backed type.',
      parameters: 'item is required and must contain non-empty name and uri fields; the item is normally returned by typeHierarchy/prepare.',
      returns: 'A deterministic array of type-hierarchy items for matching `impl Trait for Type` declarations, sorted by identity.',
      returnValue: 'Each result uses the trait declaration identity and exact UTF-16 source ranges. Missing or empty item identity returns -32602; cancellation returns -32800 without a partial array.',
      details: ['The server matches the selected type identity rather than scanning names in text. Unknown but well-formed items produce an empty array. The framed type-hierarchy fixture covers an exact User-to-Render result and malformed identity validation.'],
      example: '{"jsonrpc":"2.0","id":24,"method":"typeHierarchy/supertypes","params":{"item":{"name":"User","uri":"file:///tmp/demo.osr","range":{"start":{"line":1,"character":7},"end":{"line":1,"character":11}},"selectionRange":{"start":{"line":1,"character":7},"end":{"line":1,"character":11}}}}}',
    }, {
      name: 'typeHierarchy/subtypes',
      signature: 'typeHierarchy/subtypes { item: { name, uri, range, selectionRange, data? } }',
      description: 'Returns the source-backed types implementing a selected trait.',
      parameters: 'item is required and must contain non-empty name and uri fields; the item is normally returned by typeHierarchy/prepare.',
      returns: 'A deterministic array of implementing type items sorted by name, URI, and source range.',
      returnValue: 'Each result contains the implementation type declaration identity and exact UTF-16 ranges. Missing or empty item identity returns -32602; cancellation returns -32800 without a partial array.',
      details: ['Multiple implementations are deduplicated and ordered independently of source traversal order. The framed type-hierarchy fixture proves Admin then User ordering, repeated-response equality, malformed identity validation, and cancellation.'],
      example: '{"jsonrpc":"2.0","id":25,"method":"typeHierarchy/subtypes","params":{"item":{"name":"Render","uri":"file:///tmp/demo.osr","range":{"start":{"line":0,"character":6},"end":{"line":0,"character":12}},"selectionRange":{"start":{"line":0,"character":6},"end":{"line":0,"character":12}}}}}',
    }, {
      name: 'workspace/didChangeWatchedFiles',
      signature: 'workspace/didChangeWatchedFiles { changes: [{ uri, type }] }',
      description: 'Refreshes the bounded disk-backed workspace index after a client filesystem-watcher batch.',
      parameters: 'Each change requires a valid file URI and type 1 (created), 2 (changed), or 3 (deleted). The complete batch is validated before refresh; this notification has no JSON-RPC response.',
      returns: 'No result. Open-document overlays remain authoritative; created, changed, and deleted workspace source files are reflected by later navigation and workspace-symbol requests.',
      returnValue: 'The server performs one cancellation-aware refresh and does not create a background watcher or retain operating-system file handles.',
      details: ['Malformed entries are logged and do not cause a partial refresh. Symlink entries and files outside configured workspace roots remain excluded, and the refresh is bounded by the same workspace-file limits as initialize indexing.'],
      example: '{"jsonrpc":"2.0","method":"workspace/didChangeWatchedFiles","params":{"changes":[{"uri":"file:///workspace/greetings.osr","type":2}]}}',
    }, {
      name: 'callHierarchy/incomingCalls',
      signature: 'callHierarchy/incomingCalls { item: { name, uri, range, selectionRange } }',
      description: 'Finds source-backed call sites that reach the selected function or trait method.',
      parameters: 'item is required and must contain non-empty name and uri fields plus the ordinary call-hierarchy ranges. The item is normally returned by callHierarchy/prepare.',
      returns: 'An array of incoming-call records, each with a from call-hierarchy item and UTF-16 fromRanges.',
      returnValue: 'Only resolved function and trait-method references are returned. Results are deterministic and an unknown source document produces an empty array after the item identity has been validated.',
      details: ['Malformed JSON or an absent/empty item name or uri returns JSON-RPC -32602; malformed identity is never converted into a successful empty result.', 'The server does not execute code or infer calls textually. It uses analyzer reference targets and source-backed ranges. The framed protocol fixture covers valid and malformed requests.'],
      example: '{"jsonrpc":"2.0","id":21,"method":"callHierarchy/incomingCalls","params":{"item":{"name":"greet","uri":"file:///tmp/greetings.osr","range":{"start":{"line":2,"character":5},"end":{"line":2,"character":10}},"selectionRange":{"start":{"line":2,"character":5},"end":{"line":2,"character":10}}}}}',
    }, {
      name: 'textDocument/prepareRename',
      signature: 'textDocument/prepareRename { textDocument.uri, position }',
      description: 'Checks whether the UTF-16 position identifies a source-backed symbol that the analyzer can rename safely, including an exported member imported from a local workspace module.',
      parameters: 'textDocument.uri is required; an optional textDocument.version rejects stale requests before analysis. position is zero-based UTF-16 and must land on the identifier occurrence to be renamed.',
      returns: 'The exact occurrence range under the cursor, or null when the symbol is unresolved, virtual, generated, or not proven editable by the shared analyzer.',
      returnValue: 'Local bindings, struct fields and methods, generic type parameters, macro declarations and parameters, imported module aliases, and members exported by source-backed local user modules are source-backed when their analyzer identity is known. Virtual osiris://stdlib exports are navigable but are not editable rename targets.',
      details: ['Malformed positions and stale versions return JSON-RPC -32602. A canceled request returns -32800 and no partial result. The returned range is the cursor occurrence, so a reference in a different line or document is not incorrectly reported as the declaration range. Open documents overlay workspace-indexed module files.'],
      example: '{"jsonrpc":"2.0","id":2,"method":"textDocument/prepareRename","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":4},"position":{"line":2,"character":5}}}\n# Result: {"start":{"line":2,"character":5},"end":{"line":2,"character":11}}',
    }, {
      name: 'textDocument/rename',
      signature: 'textDocument/rename { textDocument.uri, position, newName }',
      description: 'Produces a source-backed, scope-aware, versioned WorkspaceEdit for every proven definition and reference of one symbol across the indexed workspace.',
      parameters: 'textDocument.uri is required; an optional textDocument.version rejects stale requests. position is zero-based UTF-16. newName must be a valid Osiris identifier.',
      returns: 'A WorkspaceEdit with deterministic, versioned documentChanges. The server does not mutate open-document text; the client chooses whether to apply the edit.',
      returnValue: 'Builtins, generated symbols, unresolved names, virtual standard-library exports, and stale version assertions are not renameable. Same-spelled shadowed bindings and unrelated member names are never renamed by textual fallback. An exported member of a source-backed local user module is renameable from either its definition or an importer reference; the edit includes the explicit export and resolved references in both documents.',
      details: ['Rename uses the same lexical analysis as hover and go-to-definition, not a textual search. A local parameter named answer therefore remains untouched when renaming an outer answer binding. Struct fields and methods, generic parameters, macro symbols and parameters, imported module aliases, and exported user-module members use the same proven symbol identity.', 'Edits are UTF-16-correct, deduplicated, non-overlapping, and sorted from the end of each document toward the beginning. documentChanges are sorted by URI and carry the captured document version; workspace-indexed files use their indexed version until an open overlay replaces them. Malformed identifiers or positions return -32602; cancellation returns -32800. Clients should apply edits atomically and then send their ordinary didChange notification.', 'The multi-document result is built from one merged open-overlay/workspace snapshot. languageServer.validateRenameDocumentSnapshot performs a locked final identity/version check for every affected URI; if an affected module or importer changes while the scan is in flight, the request returns -32602 with no WorkspaceEdit. lsp-workspace-rename-transaction.jsonl and the normal, concurrent-mutation, and wire-cancellation tests cover this transaction boundary.'],
      example: '{"jsonrpc":"2.0","id":2,"method":"textDocument/rename","params":{"textDocument":{"uri":"file:///tmp/demo.osr"},"position":{"line":2,"character":5},"newName":"result"}}\n# WorkspaceEdit: let answer = 42 / emit answer becomes let result = 42 / emit result',
    }, {
      name: 'textDocument/codeAction',
      signature: 'textDocument/codeAction { textDocument, range, context }',
      description: 'Returns diagnostic quick fixes and a minimal source.organizeImports edit for a leading import block.',
      parameters: 'textDocument.uri is required. An optional textDocument.version is checked before producing edits; range is UTF-16 and diagnostics are the client-provided diagnostics for the requested span.',
      returns: 'An array of code actions. Deprecated diagnostics produce a quickfix; out-of-order imports produce one versioned WorkspaceEdit with one edit per changed import statement.',
      returnValue: 'Import edits are sorted by module path and alias, non-overlapping, UTF-16-correct, and ordered from the end of the document toward the beginning. Imports after another declaration are not moved. The server never mutates the open overlay.',
      details: ['The range is required and must be a valid, non-reversed UTF-16 range in the current open document; missing, reversed, or out-of-document ranges return JSON-RPC -32602 before edits are built.', 'Use textDocument/formatting when the entire document should be reformatted. source.organizeImports intentionally changes only import statement spans so clients can apply it independently.', 'Cancellation is checked before validation, during diagnostic/import scanning, and after edit assembly. A canceled request returns -32800 and no partial action or edit.', 'A stale optional version assertion returns JSON-RPC -32602 and no action. The framed boundary cases are recorded in lsp-workspace-code-actions.jsonl.'],
      example: '{"jsonrpc":"2.0","id":3,"method":"textDocument/codeAction","params":{"textDocument":{"uri":"file:///tmp/demo.osr","version":4},"range":{"start":{"line":0,"character":0},"end":{"line":1,"character":0}},"context":{"diagnostics":[]}}}',
    }, {
      name: 'textDocument/codeLens',
      signature: 'textDocument/codeLens { textDocument: { uri, version? } }',
      description: 'Returns deterministic run or benchmark commands for tests in the opened file or its colocated sidecar.',
      parameters: 'The optional textDocument.version must equal the open overlay version. A production source resolves name.test.osr first, then name_test.osr; a direct test URI uses that file.',
      returns: 'CodeLens objects with zero-based LSP ranges and osiris.test or osiris.benchmark commands carrying the absolute test file and declaration name.',
      returnValue: 'The server never executes tests or mutates overlays. Open sidecar text takes precedence over disk, and cancellation returns JSON-RPC -32800.',
      details: ['A production document with no valid sidecar returns an empty array.', 'Configured workspace roots reject sidecars that escape lexically or through symlinks.'],
      example: '{"jsonrpc":"2.0","id":4,"method":"textDocument/codeLens","params":{"textDocument":{"uri":"file:///tmp/greetings.osr","version":7}}}',
    }, {
      name: 'osiris/testInventory',
      signature: 'osiris/testInventory { textDocument: { uri, version? } }',
      description: 'Returns schema-versioned test metadata for a test file, its deterministic colocated sidecar, or every test below a configured workspace directory.',
      parameters: 'The URI may identify name.test.osr/name_test.osr directly, a neighboring production source, or a configured workspace directory. The optional version asserts the current open overlay before single-file discovery; a workspace-directory URI has no document version.',
      returns: 'An inventory containing schema_version, discovered test files, declaration names, one-based source locations, skip/expected-failure/benchmark/fuzz flags, package ownership for single-file requests, and nearest osiris.mod module ownership when available.',
      returnValue: 'Sidecar overlays take precedence over disk. Configured roots are enforced after symlink resolution; canceled requests return JSON-RPC -32800 and do not return partial metadata.',
      details: ['This endpoint shares discovery rules with osiris test, including name.test.osr before name_test.osr for a source file. A configured workspace-directory request recursively discovers both suffixes in sorted absolute-path order, skips hidden directories plus .git and .osiris, rejects symlink entries, and applies the workspace file and per-file-size limits.', 'An open test-file overlay wins over disk for both single-file and workspace-directory requests. Workspace-directory requests require the directory to be inside a configured root; an unconfigured or outside directory is rejected without reading it.', 'The language server reports metadata only; execution remains the responsibility of osiris test or a client command.'],
      example: '{"jsonrpc":"2.0","id":5,"method":"osiris/testInventory","params":{"textDocument":{"uri":"file:///tmp/greetings.osr","version":7}}}',
    }, {
      name: 'osiris/testResults',
      signature: 'osiris/testResults { textDocument: { uri, version? }, backend?, filter? }',
      description: 'Executes the selected language tests against the current open-document snapshot and returns JSON-safe per-test results.',
      parameters: 'textDocument.uri identifies a test file, a neighboring production source, or a configured workspace directory. The optional version must match the open overlay for a single-document request. backend is interpreter, vm, or native; filter is the deterministic test-name filter.',
      returns: 'An array of records containing name, file, backend, status, passed, optional skip/expected-failure/unexpected-pass flags, outputs, assertions, logs, source location, error text, and durationNS. Assertions contain expression, passed, location, and optional message.',
      returnValue: 'Open sidecar and source overlays take precedence over disk and no partial result array is returned after cancellation. Locations use zero-based UTF-16 ranges. durationNS is a runtime measurement and is not expected to be byte-for-byte stable across runs.',
      details: ['Stale versions, malformed textDocument values, unsupported backends, invalid filters, and discovery failures return structured request or runner errors. Cancellation returns JSON-RPC -32800.', 'The endpoint shares colocated/default-root discovery with osiris test and osiris/testInventory. It does not expose evaluator values or internal error interfaces over JSON-RPC.', 'The lsp-test-results.jsonl fixture covers passing and skipped tests, open overlays, locations, stale-version rejection, unsupported backend rejection, and deterministic semantic comparison with durationNS ignored.'],
      example: '{"jsonrpc":"2.0","id":6,"method":"osiris/testResults","params":{"textDocument":{"uri":"file:///tmp/greetings.test.osr","version":7},"backend":"interpreter"}}',
    }],
  }, {
    title: 'Optimization reporting',
    entries: [{
      name: 'osiris mod',
      aliases: ['mod'],
      signature: 'osiris mod init|tidy|download|verify|sync|clean|graph|list|audit|vendor|workspace [arguments…]',
      description: 'Manages the human-authored osiris.mod/osiris.work manifests and generated .osiris.lock/.osiris/cache state without requiring a central package registry.',
      parameters: 'init creates a canonical module manifest; tidy derives requirements; download populates verified cache; verify checks lock/cache integrity; sync and clean retain the legacy local-module workflow; graph/list/audit/vendor/workspace expose deterministic supply-chain operations.',
      returns: 'A deterministic manifest, lock/cache update, dependency report, audit report, or vendor tree according to the selected subcommand.',
      returnValue: 'Repository imports use canonical paths such as github.com/acme/orbit. Tags and full commits are resolved by a replaceable provider, while lock state records the provider, commit, source digest, and dependency graph. Set OSIRIS_OFFLINE=1 to resolve only verified cached modules.',
      details: [
        'Absolute local imports remain explicit and are not treated as relative project paths. `osiris get module@selector` adds or updates one requirement without executing dependency code.',
        'Offline mode reports a clear error when a requested module is absent from the lock/cache; altered cached module bytes fail their checksum check. Private credentials never enter manifests, locks, caches, diagnostics, or telemetry.',
      ],
      example: 'osiris mod init github.com/acme/orbit\nosiris get github.com/acme/tensors@v1.4.2\nosiris mod tidy\nosiris mod verify',
    }, {
      name: 'osiris get',
      aliases: ['get'],
      signature: 'osiris get module[@selector]',
      description: 'Adds, upgrades, downgrades, or removes one repository module requirement using an explicit selector.',
      parameters: 'The module path is canonical repository syntax. The optional selector is `latest`, a semantic version or compatible selector, or a full commit SHA; a leading `-` removes the requirement.',
      returns: 'The updated osiris.mod requirement and deterministic lock/cache resolution.',
      returnValue: 'Resolution records the immutable commit, tag/pseudo-version, source-tree digest, provider identity, and verified cache entry. The command does not execute imported dependency code.',
      details: ['GitHub is the first complete provider, but provider identity is replaceable for other Git hosts and controlled vanity paths.', 'Use `OSIRIS_OFFLINE=1` with a previously downloaded lock/cache state for offline reproducible builds.'],
      example: 'osiris get github.com/acme/orbit@latest\nosiris get github.com/acme/orbit@v1.2.3\nosiris get -github.com/acme/orbit',
    }, {
      name: 'Compilation artifacts and caches',
      signature: 'osiris build -cache-dir directory -o program source.osr',
      description: 'Builds a native executable using an explicit local content-addressed cache. Cache reuse is guarded by a versioned manifest so incompatible or corrupted artifacts fail rather than execute.',
      parameters: '-cache-dir selects a local cache. The cache is native-target specific; do not use it as a cross-machine binary-distribution directory. -o names the output executable.',
      returns: 'A native executable and, in the cache directory, an executable plus a JSON manifest with format_version 1.',
      returnValue: 'The native cache key includes generated C, C compiler path and identity, exact flags, OS/architecture target, and configured numerical-provider source and flags. A changed semantic input creates a distinct key.',
      details: [
        'Generated .osiris.lock files use format version 2 and SHA-256-check cached module contents before use. They are local generated state, not a hand-edited dependency manifest.',
        'GPU -json plans use schema_version 1. VM bytecode, IR, SSA, CFG, and related analysis structures are in-memory or diagnostic-only and do not claim a persistent artifact format.',
        'The compatibility and upgrade policy is maintained with the project’s published toolchain contracts.',
      ],
      example: 'osiris build -cache-dir .osiris/native-cache -o program program.osr\n./program\n# A later identical build validates and reuses the matching cache entry.',
    }, {
      name: 'osiris CLI diagnostics',
      signature: 'osiris --diagnostics=json <command> [arguments…]',
      description: 'Runs a command normally but renders a failure as a versioned JSON diagnostic on standard error. Successful command payloads remain on standard output and retain their command-specific format.',
      parameters: '--diagnostics=json must appear before the command name. It is intentionally distinct from command-owned result flags such as osiris opt -json, gpu -json, deps -json, trace -json, and heap -json.',
      returns: 'Exit status 0 on success; JSON only on standard error for a failing invocation.',
      returnValue: 'Diagnostic schema v1 contains schema_version, command when available, category, exit_code, and message. Categories are usage (64), compile (65), input (66), runtime (70), and internal (71).',
      details: [
        'Use this mode for editors, task runners, and CI. Ignore additive unknown fields and branch on schema_version before depending on a future schema.',
        'The CLI emits no terminal colour or control sequences, so interactive and redirected output have the same content.',
        'A bare osiris prints help and exits 0. Invalid commands and flags are usage failures; source parse/lowering failures are compile failures; program execution failures are runtime failures.',
      ],
      example: 'osiris --diagnostics=json run broken.osr 1>result.txt 2>diagnostic.json\n# diagnostic.json: {"schema_version":1,"command":"run","category":"compile","exit_code":65,"message":"…"}',
    }, {
      name: 'osiris opt',
      aliases: ['opt'],
      signature: 'osiris opt [-w] [-report|-json] [file]',
      description: 'Formats the conservative source-optimizer result, writes it back with -w, or reports the ordered optimizer pipeline. The report is designed for review and automation; it does not claim speculative transformations ran.',
      parameters: 'file is an Osiris source path, or standard input when omitted. -report writes stable text. -json writes a versioned JSON object. -w requires a file path and cannot be combined with report output.',
      returns: 'Formatted Osiris source, a text report, or an OptimizationReport JSON document.',
      returnValue: 'The JSON report has version, source_name, optimized_source, and ordered passes. Each pass exposes name, status, before/after source, statement counts, and a reason when it made no change or is unavailable.',
      details: [
        'simplify-and-propagate and dead-statement-elimination are conservative source passes. They report no_change when no eligible transformation is proven safe.',
        'loop-invariant-code-motion, bounds-check-elimination, loop-unrolling, and scalarization are explicitly reported as not_implemented until the compiler has the needed effect, alias, range, or profitability proof.',
        'JSON field and pass ordering are deterministic. Consumers should branch on version before relying on new fields or statuses.',
      ],
      example: 'osiris opt -report program.osr\nosiris opt -json program.osr\n# {"version":1,"source_name":"program.osr","optimized_source":"…","passes":[…]}',
    }],
  }, ...cliReference.sections], 'tooling'),
};

export const modules = [...apiModules, typeModule, ...constantModules, toolingModule];

export const chapters = manual.sections.map((chapter) => ({
  ...chapter,
  cards: chapter.cards || [],
}));

const languageSearchModule = {
  id: 'language',
  label: 'Language',
  summary: 'Core syntax and semantics.',
  sections: languageSections,
};

const technicalSearchModule = {
  id: 'technical',
  label: technicalReference.title,
  summary: technicalReference.summary,
  sections: [{
    title: 'Technical reference',
    entries: technicalReference.sections.map((section) => ({
      name: section.title,
      signature: section.syntax?.map((item) => item.code).join('\n') || `#technical-${section.id}`,
      description: section.summary,
      searchText: [
        section.summary,
        ...(section.paragraphs || []),
        ...(section.tables || []).flatMap((table) => [table.title, ...table.rows.flat()]),
        ...(section.indexes || []).flatMap((index) => [
          index.title,
          ...(index.source ? technicalReference[index.source] : index.items),
        ]),
      ].join(' '),
    })),
  }],
};

export function searchEntries(query) {
  const needle = query.trim().toLowerCase();
  if (!needle) return [];
  return [...modules, languageSearchModule, technicalSearchModule].flatMap((module) => module.sections.flatMap((section) => section.entries.map((entry) => ({ module, section, entry })))).filter(({ module, section, entry }) => [module.label, module.summary, section.title, entry.name, entry.aliases?.join(' '), entry.signature, entry.description, entry.searchText, entry.parameters, entry.returns, entry.returnValue, entry.details?.join(' '), entry.errors, entry.native, entry.example].filter(Boolean).join(' ').replaceAll('`', '').toLowerCase().includes(needle));
}
