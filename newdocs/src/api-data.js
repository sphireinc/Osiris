import baseLanguageSections from './language-reference.json' with { type: 'json' };
import manual from './legacy-reference.json' with { type: 'json' };
import supplemental from './supplemental-reference.json' with { type: 'json' };
import timeReference from './time-reference.json' with { type: 'json' };
import technicalReference from './technical-reference.json' with { type: 'json' };
import constantReference from './constant-reference.json' with { type: 'json' };
import cliReference from './cli-reference.json' with { type: 'json' };
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
  simd: '# Reserved for a future SIMD value family; not a current runtime kind.',
  quat: '# Reserved for a future quaternion value family; not a current runtime kind.',
  sparse_matrix: '# Reserved for a future sparse-matrix value family; not a current runtime kind.',
  sparse_tensor: '# Reserved for a future sparse-tensor value family; not a current runtime kind.',
  poly: '# Reserved for a future polynomial value family; not a current runtime kind.',
};

function entryFromLegacy(groupID, item) {
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
    returnValue: item.details?.[0],
    details: item.details,
    errors: item.errors,
    native: item.native,
    example: item.example,
    exampleStatus: groupID === 'io' ? 'sandbox-required' : item.example.startsWith('emit ') ? 'checked' : 'illustrative',
  };
}

// This is the canonical React reference data. It was migrated from the old
// static manual as a checked-in snapshot, so the app never fetches or executes
// legacy documentation at runtime.
const supplementalGroups = new Map(supplemental.groups.filter((group) => group.id !== 'constants').map((group) => [group.id, group]));
const combinedGroups = manual.groups.map((group) => ({
  ...group,
  items: [...group.items, ...(supplementalGroups.get(group.id)?.items || [])],
}));
for (const group of supplemental.groups.filter((candidate) => candidate.id !== 'constants')) {
  if (!combinedGroups.some((candidate) => candidate.id === group.id)) combinedGroups.push(group);
}
combinedGroups.push(timeReference);

const apiModules = combinedGroups.map((group) => ({
  id: group.id,
  label: group.title,
  summary: group.summary,
  importLine: group.title.startsWith('std/') ? `import "${group.title}" as ${group.id}` : '# Built-in reference surface',
  sections: orderReferenceSections(
    [{ title: group.title, entries: group.items.map((item) => entryFromLegacy(group.id, item)) }],
    group.id,
  ),
}));

const plannedTypeNames = new Set(['simd', 'quat', 'sparse_matrix', 'sparse_tensor', 'poly']);
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
      name: 'FFI boundary (design only)',
      signature: 'ffi manifest ABI v1 — not implemented',
      description: 'Defines the future C/Go interoperability boundary without exposing arbitrary host calls. General FFI is currently unsupported.',
      parameters: 'A future declaration must be a project-local, ABI-versioned manifest with a digest-pinned allowlisted library, explicit scalar/buffer signatures, ownership, and trusted-host capability grants.',
      returns: 'No current runtime value or callable API. This is a design contract for future implementation.',
      returnValue: 'v1 would support exact scalar widths plus explicit UTF-8 string/byte buffers. Lists, tensors, complex, decimal/bigint, units, errors, tasks, streams, modules, and callbacks are excluded from the direct ABI.',
      details: ['Dynamic search paths, environment-driven loader fallback, arbitrary symbols, network acquisition, callbacks, re-entrancy, and background threads are denied by the design.', 'Every result must declare borrowed, caller-owned, or host-retained ownership. Failed calls never transfer ownership; owned values require a matching declared release function.'],
      example: '{"abi_version":1,"library":"example-math","sha256":"…","capabilities":["ffi.math"],"functions":[{"name":"add_f64","args":["f64","f64"],"result":"f64","ownership":"borrowed"}]}',
    }, {
      name: 'Compatibility and deprecations',
      signature: 'OSIRIS_SUPPRESS_WARNINGS=OSIRIS-Dnnn osiris <command>',
      description: 'Defines how stable language, CLI, ABI, and artifact contracts evolve across releases. Stable changes follow SemVer; partial and experimental support-matrix entries explicitly have narrower guarantees.',
      parameters: 'Set OSIRIS_SUPPRESS_WARNINGS to one comma-separated stable warning code or all for temporary local suppression. Suppression hides a warning only; it never preserves removed behavior.',
      returns: 'Deprecation warnings use stable OSIRIS-Dnnn codes, name the replacement, and state the earliest removal major version.',
      returnValue: 'Stable features are warned for at least one minor release before removal and are removed only in a major release with a migration note.',
      details: ['Release notes use Added, Changed, Deprecated, Removed, Fixed, and Security categories. Breaking releases include before/after migration guidance.', 'There are no active deprecations in 0.1.0. See docs/development/CHANGELOG.md and MIGRATIONS.md in the source repository for the durable policy.'],
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
      description: 'Starts the Osiris Language Server Protocol endpoint over standard input and output. It provides conservative source-aware hover and go-to-definition support for open documents without claiming a full IDE feature set.',
      parameters: 'No command arguments. The client sends JSON-RPC 2.0 messages framed with Content-Length headers. Standard output is reserved for protocol messages; diagnostics and malformed-change notices use the server log stream.',
      returns: 'An LSP session advertising incremental text synchronization, hover, and definition providers.',
      returnValue: 'All LSP positions and ranges use zero-based UTF-16 code units. The implementation converts these to the lexer’s UTF-8 byte-column source map, so astral Unicode characters have correct editor locations.',
      details: [
        'Supported methods are initialize, initialized, shutdown, exit, textDocument/didOpen, textDocument/didChange, textDocument/didClose, textDocument/hover, textDocument/definition, and textDocument/rename. Completion, references, formatting, semantic tokens, and code actions are intentionally not advertised.',
        'didChange accepts LSP incremental changes in order; an omitted range replaces the full document. Invalid ranges, including positions that split a UTF-16 surrogate pair, are ignored without corrupting the existing document.',
        'Malformed or partial source remains open and publishes a diagnostic. Definitions for builtins and generated symbols have no source location and return null rather than a fabricated zero range.',
      ],
      example: 'osiris lsp\n# Client request (Content-Length framing omitted):\n{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}\n# Result capabilities: incremental sync, hover, definition',
    }, {
      name: 'textDocument/rename',
      signature: 'textDocument/rename { textDocument.uri, position, newName }',
      description: 'Produces a scope-aware, formatted dry-run rename edit for one open document.',
      parameters: 'position is zero-based UTF-16 as required by LSP. newName must be a valid Osiris identifier. The selected definition and each reference resolved to that definition are renamed; shadowed bindings and unrelated member names are unchanged.',
      returns: 'A standard WorkspaceEdit containing one whole-document formatted replacement. The server does not mutate its open-document text; the client chooses whether to apply the edit.',
      returnValue: 'Builtins and unresolved names are not renameable. Cross-document rename is intentionally unsupported in this first release, so imports are renamed only within the document that declares and uses the alias.',
      details: ['Rename uses the same lexical analysis as hover and go-to-definition, not a textual search. A local parameter named answer therefore remains untouched when renaming an outer answer binding.', 'The full-document edit keeps parser/formatter output canonical. Clients should apply it atomically and then send their ordinary didChange notification.'],
      example: '{"jsonrpc":"2.0","id":2,"method":"textDocument/rename","params":{"textDocument":{"uri":"file:///tmp/demo.osr"},"position":{"line":2,"character":5},"newName":"result"}}\n# WorkspaceEdit: let answer = 42 / emit answer becomes let result = 42 / emit result',
    }],
  }, {
    title: 'Optimization reporting',
    entries: [{
      name: 'osiris mod',
      aliases: ['mod'],
      signature: 'osiris mod sync [file]\nosiris mod clean <file>',
      description: 'Manages generated local module state without hand-editing .osiris.lock or .osiris/cache. sync resolves imports and refreshes the lock/cache; clean removes only the generated state associated with a source project.',
      parameters: 'sync accepts an optional source path or standard input. clean requires a source file path so Osiris can derive the exact project-root lock and cache paths.',
      returns: 'sync prints the deterministic dependency report and creates verified generated state. clean prints the exact generated paths it removed.',
      returnValue: 'Relative imports are canonicalized beneath their project root. Attempts to escape that root fail. Set OSIRIS_OFFLINE=1 to resolve only lockfile-backed, SHA-256-verified cached modules.',
      details: [
        'Absolute local imports remain explicit and are not treated as relative project paths. No network module registry is consulted.',
        'Offline mode reports a clear error when a requested module is absent from the lock/cache; altered cached module bytes fail their checksum check.',
      ],
      example: 'osiris mod sync app/main.osr\nOSIRIS_OFFLINE=1 osiris run app/main.osr\nosiris mod clean app/main.osr',
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
        'See docs/design/ARTIFACT-FORMATS.md in the source repository for the full compatibility and upgrade policy.',
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
