# Osiris LSP contract

The Osiris language server is a stdio JSON-RPC 2.0 endpoint. Requests use
`Content-Length` framing, and all document positions and ranges are zero-based
UTF-16 code-unit coordinates. The parser and checker remain the source of
language truth; the LSP layer converts their source spans and checked types to
protocol values.

## Advertised surface

The initialize response advertises only the methods implemented by the server:

- incremental open/change/save/close synchronization;
- diagnostics, hover, document/workspace symbols, definitions, type
  definitions, implementations, references, completion, signature help, and
  rename;
- semantic tokens, inlay hints, folding ranges, selection ranges, code actions,
  formatting, and code lenses;
- call hierarchy and type hierarchy;
- workspace file-change notifications for bounded disk-index refreshes;
- the Osiris test-inventory request `osiris/testInventory`.

The deterministic end-to-end protocol fixture at
`internal/osiris/testdata/lsp/lsp-protocol.jsonl` sends every advertised
request and checks that each request receives a stable response. It also checks
initialize capabilities, diagnostic publication, malformed parameters, and an
unknown method without ending the session.

## Request lifecycle and cancellation

`$/cancelRequest` is a JSON-RPC notification. Its parameters contain the exact
request identifier to cancel:

```json
{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":17}}
```

The stdio reader recognizes this notification while another request is being
analyzed. Each request receives a private child context and is registered by
its JSON-RPC identifier until its response has been published. A matching
`$/cancelRequest` calls that context's cancellation function; an unknown
identifier is ignored, and a malformed cancellation notification is logged.
The cancellation notification itself never receives a response. A canceled
handler returns JSON-RPC error `-32800` with no partial result when its next
transaction boundary observes the canceled context.

Request analysis may overlap with input reading so a cancellation can reach an
active request instead of waiting for that request to finish. Response frames
are written under one output lock and are released in request order, which
keeps framed output intact and deterministic while preserving the protocol's
request/notification ordering contract. Synchronization notifications remain
serialized with the reader. `shutdown` waits for all earlier request responses;
ordinary `exit` waits for already-read requests to drain, while session-context
cancellation and input errors cancel active requests and wait for their
goroutines before returning.

The lifecycle contract is covered by
`internal/osiris/lsp_request_cancellation_test.go`, including cancellation of
an active registered request, notification-without-response behavior, and
ordered normal responses. Context-aware provider methods still decide the
precise safe publication boundary; cancellation does not interrupt a pure
synchronous operation in the middle of a mutation.

## Positions, changes, and diagnostics

Parser spans use the lexer’s UTF-8 byte columns. The server converts them to
UTF-16 only at the protocol boundary, including diagnostics, navigation
locations, semantic-token deltas, inlay hints, code lenses, and edits. An
incremental change is applied in the order received; a change without a range
replaces the complete document. All changes in one `textDocument/didChange`
notification are first applied to a private snapshot, and the open overlay is
replaced only after every change and the resulting analysis succeed. A
position that is outside the document or splits an astral character’s
surrogate pair is rejected. Cancellation, a malformed range, or a stale
non-zero document version discards the complete batch and leaves the prior
overlay and version unchanged. `textDocument/didOpen`, `textDocument/didChange`,
`textDocument/didSave`, and `textDocument/didClose` are notifications and never
receive JSON-RPC responses; invalid notification payloads are logged and the
server continues reading framed messages.

`textDocument/didSave` is a notification and therefore never receives a JSON-RPC
response. The server revalidates the current open-document overlay and publishes
fresh diagnostics. A client may include the optional saved `text`; when omitted,
the current overlay is revalidated. Revalidation keeps the document's existing
version. If a newer change or close wins while analysis is running, the saved
result is discarded instead of replacing newer state.

Malformed source is retained as the open document and produces a source
diagnostic. Generated or builtin definitions without a source span return no
location rather than a fabricated zero range.

`textDocument/diagnostic` is a pull request with the exact result shape
`{"kind":"full","items":[...]}`. Each item contains `range`, `severity`,
`source`, and `message`; ranges are zero-based UTF-16 positions converted from
the parser/checker span. Items are returned as a defensive snapshot, sorted by
start and end position and then severity, source, and message. The server does
not expose the mutable diagnostic slice held by an open document. An optional
`textDocument.version` assertion is checked before retrieval. Missing or
malformed document parameters return `-32602`, a stale version returns
`-32602`, and cancellation returns `-32800` without a partial report. The
deterministic fixture `internal/osiris/testdata/lsp/lsp-workspace-diagnostics.jsonl`
covers the full result shape, an astral-character UTF-16 range, malformed and
stale requests, and repeated-response equality.

## Completion and signature help

`textDocument/completion` returns deterministic, alphabetically sorted items.
At a module selector such as `text.` it enumerates the imported module's
exports; elsewhere it enumerates analyzed lexical definitions. Items use LSP
completion kinds for functions, types, variables, and modules and include the
definition detail where the analyzer has it. The request position is converted
from UTF-16 before the module selector is inspected, so an astral character
before the cursor does not shift the lookup boundary.

Source-backed completion items include a `data` object with the defining
document `uri` and exported symbol `name`. This identity payload is available
for local user-module and ordinary source definitions; virtual stdlib members
remain intentionally source-less.

When the selector is a checked nominal receiver such as `user.` and the
binding has type `User`, completion enumerates only the members indexed for
`User`. Struct fields use LSP kind `Field` and show `field <type>` detail;
trait and implementation methods use kind `Method` and show their complete
function signature. Member completion is intentionally type-directed: a
dynamic or untyped receiver falls back to lexical/module completion and does
not receive speculative members. The receiver type is taken from the shared
checker, so completion remains consistent with the type used by navigation.

The same checked member index powers `textDocument/hover`,
`textDocument/definition`, and `textDocument/references`. A reference such as
`user.render` targets the source declaration in `impl Render for User`, not the
`user` binding. Trait methods are indexed under the trait name and concrete
implementation methods under their target type; struct fields are indexed
under their struct type. All returned source ranges are converted to UTF-16,
and source-less or generated symbols continue to return no fabricated
definition location. The complete request, identity, ordering, UTF-16
validation, and cancellation contract for references is documented in
[LSP-REFERENCES.md](LSP-REFERENCES.md).

`textDocument/documentHighlight` uses the same analyzer identity for a
cursor-local read/write view. It returns only occurrences in the requested
document, converts source spans to UTF-16, removes duplicate ranges, and
sorts them deterministically. Definitions and assignment targets are `write`
highlights; resolved uses are `read` highlights. Required fields, stale
versions, invalid UTF-16 positions, cancellation, and the publication
boundary are specified in [LSP-HIGHLIGHTS.md](LSP-HIGHLIGHTS.md).

`textDocument/signatureHelp` identifies the innermost unterminated call at the
cursor. Parentheses and commas inside nested calls are not counted as
arguments of the outer call, and quoted strings are ignored while locating the
call. For a source function, the response includes the checked signature,
one parameter label per declared parameter (including its declared type), and
the zero-based `activeParameter` selected by the commas before the cursor. A
cursor after the final argument keeps the active parameter clamped to the last
declared parameter. Both methods accept the optional
`textDocument.version`; an older version is rejected with
JSON-RPC `-32602` before semantic data is returned. Missing parameters,
missing positions, positions outside the document, and positions that split a
UTF-16 surrogate pair use the same `-32602` validation path. The request
checks cancellation before scanning, during the call scan, and before
publishing the complete response; cancellation returns `-32800` and never
publishes partial signature data or mutates the open-document overlay. The
single-signature response and parameter order are source-stable, and the
granular fixture `internal/osiris/testdata/lsp/lsp-signature-help.jsonl`
asserts valid nested-call boundaries, malformed/stale requests, and repeated
wire-level equality.

### Standard-library export navigation

For a resolvable import such as `import "std/text" as text`, the semantic
index includes the module's actual exported functions and constants. The
completion item kind and detail are derived from the runtime module value, so
the LSP does not maintain a second hand-written signature table. Completion,
hover, definition, and references use the same stable identity for a member
such as `text.trim`.

Standard-library definitions use virtual URIs of the form
`osiris://stdlib/<module>`, for example `osiris://stdlib/text`. These URIs are
intentionally source-less: clients may display the symbol and its signature,
but must not assume that a local filesystem document exists. Capability-gated
or otherwise unresolved modules are omitted from semantic results rather than
reported as speculative definitions. Requests still enforce UTF-16 positions,
document versions, cancellation, and the normal `-32602`/`-32800` protocol
errors.

### User-module navigation

For a relative import such as `import "./greetings.osr" as greetings`, the
language server resolves the file against the importing document and accepts
it only when the resolved path is inside a configured workspace root. The
module must be readable and parseable by the current workspace loader. Its
`export` statements define the only members exposed to the importer; private
top-level declarations are not advertised.

The exported `symbolDef` pointers are shared between the defining document and
every analyzed importer. Consequently, `greetings.Hello` has one identity for
completion, hover, go-to-definition, and references. References are collected
across the open-document overlay and indexed workspace documents, sorted by URI
and UTF-16 range. Rebuilding the workspace uses a local-analysis pass followed
by an import-linking pass so an updated module overlay cannot leave importers
pointing at an older definition generation.

User-module definitions use their real `file:` URI and source range. An
unresolved repository/module-path import, a file outside the configured roots,
a virtual standard-library export, or a module that cannot be parsed is not
claimed as source-backed navigation. The server also does not infer exports
from a package name or from an import whose local file has not been resolved.
Malformed requests return `-32602`, stale document versions return `-32602`,
and canceled requests return `-32800` before or during navigation. The framed
`internal/osiris/testdata/lsp/lsp-semantic-user-modules.jsonl` fixture covers
completion, definition, references, stale handling, malformed parameters, and
deterministic Content-Length framing; the focused test adds astral-Unicode
UTF-16 positions and cancellation coverage.

Repository imports use the runtime's verified `RepositoryProvider` registry
when a provider and a matching selector are available. The LSP passes the
request context through resolution, reuses the registry's lock/cache digest
boundary, parses the selected source without executing it, and indexes only
explicit exports. Repository definitions use deterministic virtual
`osiris://repository/<escaped-module-key>` URIs because they are not local
editable files; completion, hover, definition, and references share the
same source-backed identity. Provider errors, unavailable cached modules,
malformed source, and canceled resolution remain explicit non-results. The
`lsp-workspace-repository.jsonl` fixture records the framed request shape, and
the focused repository-provider test proves selector propagation and verified
lock-state creation.

### Generic type-parameter navigation

Generic declarations use the parser's `typeParam` nodes and the shared
checker scope. For a declaration such as
`func identity<T: Render>(value T) T => value`, the analyzer indexes `T` as a
source-backed `type parameter` definition. Its detail is `type parameter` or
`type parameter bounded by Render`, and completion reports LSP completion kind
25 (`TypeParameter`) when the parameter is in the analyzed scope.

The same symbol identity is used for occurrences in generic bounds, parameter
types, return types, type patterns, and explicit type arguments such as
`identity<T>(value)`. When the open-document source is available, each
occurrence is located by the original parser span and converted from UTF-8
byte columns to UTF-16 at the protocol boundary. Concrete type arguments such
as `u64` are not treated as generic references and do not receive speculative
definitions. Shared checker errors remain the source of truth for invalid
arity, inference, and bound violations.

`textDocument/hover`, `textDocument/definition`, `textDocument/references`,
and `textDocument/completion` therefore agree on generic parameter identity.
The deterministic `lsp-semantic-generics.jsonl` fixture covers framed navigation,
stale versions, malformed parameters, and the normal protocol lifecycle; the
focused lsp-semantic test additionally exercises UTF-16 positions and a canceled
framed request.

### Macro source navigation

Macro declarations are lexical definitions with LSP symbol kind `macro` and
detail `compile-time macro`. The semantic predeclaration pass indexes every
macro in the current statement sequence before analyzing bodies, so a macro
call may navigate to a declaration that appears later in the file or in an
enclosing block's declaration sequence. The call expression is analyzed as a
normal source reference; hover, go-to-definition, and references therefore
share the macro declaration identity rather than matching names textually.

Macro parameters are source-backed `param` symbols. The parser exposes their
names as part of the macro declaration, and the LSP recovers the exact
identifier spans from the original source header. This keeps a parameter
definition and each use in the macro body anchored to only that identifier,
including UTF-8 source text before the parameter; the final protocol range is
converted to UTF-16. Parameters are added to the same document symbol index
used by ordinary function parameters, while macro expansion remains owned by
the shared parser/macro expander and is never executed by the LSP.

`lsp-semantic-macros.jsonl` is a deterministic framed fixture covering macro-call
hover and definition, declaration references, an astral character before the
call position, stale document-version rejection, and malformed parameters.
The lsp-semantic test also checks exact parameter ranges and a canceled framed
hover request. Generated expansion locations are not fabricated: navigation
continues to point to the source macro declaration and source parameter.

### Structured product-pattern navigation

Named product patterns are analyzed as source-backed uses of their nominal
struct type. For a pattern such as
`Point { x: x, label: label }`, the analyzer resolves `Point` through the
shared type scope, resolves each field through the struct member index, and
recursively indexes the nested `x` and `label` bindings in the arm-local
scope. Field references therefore navigate to the original struct field
declarations, while binding references navigate to the pattern binding rather
than to an unrelated same-named local.

Unknown product types and fields produce diagnostics at the pattern span or
field span. Valid nested product patterns do not require execution or a host
capability. `textDocument/hover`, `textDocument/definition`, and
`textDocument/references` use the same analyzer identities and convert the
source-backed UTF-8 spans to UTF-16 at the protocol boundary. The focused
`TestLSPStructPatternNavigationAndNestedBindings` test covers field navigation,
nested binding identities, astral text before the pattern, and the exact
declaration range.

Enum patterns expose the same source-backed type identity. For
`Status.Ready`, the analyzer records a narrow reference over `Status` that
navigates to the enum declaration and a separate case reference over the full
qualified pattern that navigates to `Status.Ready`. This keeps cursor behavior
unambiguous: a request on the enum name resolves the enum, while a request on
the case resolves the case. `TestLSPEnumPatternTypeNavigationUsesExactSourceSpan`
checks the byte-column span before it is converted to UTF-16 for the protocol.

## Semantic tokens

`textDocument/semanticTokens/full` returns exactly one object with a `data`
array. The array uses the LSP five-integer encoding
`[deltaLine, deltaStart, length, tokenType, tokenModifiers]`; token modifiers
are currently always zero. Clients must use the legend advertised by
`initialize`:

| Index | Token type | Classification source |
| ---: | --- | --- |
| 0 | `namespace` | module and package definitions |
| 1 | `type` | type aliases and newtypes |
| 2 | `class` | enum cases |
| 3 | `enum` | enum definitions |
| 4 | `interface` | trait definitions |
| 5 | `struct` | struct definitions |
| 7 | `parameter` | function parameters |
| 8 | `variable` | local bindings and unresolved identifiers |
| 10 | `function` | function and builtin definitions |
| 11 | `method` | trait and implementation methods |
| 12 | `macro` | macro definitions |
| 13 | `keyword` | declaration, control-flow, boolean, and null keywords |
| 15 | `comment` | `#` and `//` comments outside string literals |
| 16 | `string` | quoted strings and raw field tags |
| 17 | `number` | numeric literals |
| 18 | `operator` | arithmetic, comparison, assignment, and logical operators |

Definition and resolved-reference classifications are matched by exact source
line and byte column before conversion to UTF-16. This prevents a same-named
definition elsewhere in the file from changing a token's class. String
escapes, astral Unicode, and comments are measured from the original source
spelling; a comment marker inside a quoted string is not classified as a
comment. Delimiters such as parentheses and commas are intentionally omitted
because they are punctuation rather than semantic operators.

The result is deterministic: tokens are sorted by `(line, UTF-16 start, length,
token type)`, capped at 10,000 entries, and returned with no implementation
fields beyond `data`. The request requires a non-empty open-document URI and
accepts an optional document version; missing or malformed parameters, an
unknown document, and stale versions return `-32602`. Generated token spans
must end on UTF-16 code-point boundaries, fit their source line, use a legend
index, and remain non-decreasing in source order. This rejects malformed
internal ranges before a delta array can be published, including a range that
would split an astral character's surrogate pair.

Cancellation is checked before lexing, while walking source lines and lexer
tokens, during delta encoding, and immediately before publication. A canceled
request returns `-32800` with no partial `data` result and does not mutate the
document overlay. Incomplete source produces the tokens available before the
lexer error while the normal diagnostics path reports the syntax error.
The exact wire contract, malformed and stale requests, astral-Unicode lengths,
and repeated-response equality are covered by
`lsp-workspace-semantic-tokens.jsonl`,
`TestLSPWorkspaceSemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism`,
`TestLSPSemanticTokenEncodingHonorsCancellationBeforePublication`, and
`TestLSPSemanticTokenProtocolFixtureHasExactValidationContract`.

## Source analysis coverage

Document analysis is performed by `analyzeStmtSeq`, `analyzeStmt`, and
`analyzeExpr` in `internal/osiris/lsp.go`. The analyzer is deliberately
source-oriented: it builds lexical definitions and references for navigation,
then lets the shared checker provide type and language diagnostics. It does
not execute code or resolve host capabilities.

Every parser statement has an explicit analyzer path:

| AST statement | LSP behavior |
| --- | --- |
| `packageStmt` | indexes the package definition |
| `importStmt` | indexes the module alias and its import reference |
| `enumStmt` | indexes the enum and every enum case, including forward case lookup |
| `structStmt` | indexes the struct type |
| `typeDeclStmt` | indexes the type-alias definition |
| `newtypeStmt` | indexes the newtype definition |
| `traitStmt` | indexes the trait and its method definitions |
| `implStmt` | analyzes trait and receiver type references, then each method |
| `funcStmt` | indexes the function, parameters, and body references |
| `letStmt` | analyzes the initializer and indexes each binding |
| `assignStmt` | resolves the assignment target, reports an undefined target at its name span, and analyzes the value |
| `emitStmt` | analyzes the emitted expression |
| `deferStmt` | analyzes the deferred expression or reports a missing body at the statement span |
| `macroStmt` | indexes the macro and analyzes its body in a parameter scope |
| `whileStmt` | analyzes the condition and body |
| `forStmt` | analyzes the iterator and body in a loop-item scope |
| `returnStmt` | analyzes its return expression when present |
| `breakStmt` and `continueStmt` | recognized as control-flow statements; the checker owns legality diagnostics |
| `exportStmt` | resolves every exported name and reports missing names at each name span |

Every parser expression has an explicit recursive analyzer path:

| AST expression | LSP behavior |
| --- | --- |
| `identExpr` | resolves a lexical definition or reports an undefined name at the identifier span |
| `memberExpr` | analyzes the receiver and resolves module and enum-case members |
| `callExpr` | analyzes the callee and every argument; generic type arguments remain metadata |
| `unaryExpr` and `binaryExpr` | recursively analyze operand expressions |
| `listExpr` and `tupleExpr` | recursively analyze every item |
| `structExpr` | resolves the struct type and analyzes every field value |
| `structUpdateExpr` | analyzes the base and every replacement value |
| `indexExpr` | analyzes the indexed object and index |
| `tryExpr` | analyzes the body and handler in a scope containing the caught error |
| `propagateExpr` | analyzes the propagated expression |
| `blockExpr` | analyzes statements in a child scope and its optional tail |
| `matchExpr` | analyzes the subject, patterns, guards, and arm bodies in per-arm scopes |
| `literalExpr` | recognized as a complete leaf expression |

Match patterns are also traversed individually. Enum patterns create a
source reference to the case; type patterns resolve their type when it is a
user declaration; list, `or`, `and`, and `not` patterns recurse into their
children; wildcard, binding, literal, and relational patterns are complete
leaf forms. Pattern-binding validation continues to report duplicate or
invalid bindings at `patternSpan(pattern)`.

The analyzer has a guarded default for statements, expressions, and patterns.
An AST extension that is not yet supported produces a severity-one diagnostic
whose range is the node's source span. New extension nodes should implement
the internal `lspAnalysisSpanned` contract so this fallback remains
source-accurate while the feature is being integrated. This prevents an AST
form from being silently ignored. Unsupported forms therefore remain visible
to the editor, but this guard is not a claim that every future language form
already has full semantic navigation.

## Workspace and resource bounds

Workspace indexing is deterministic and bounded to 512 `.osr` files, with a
4 MiB per-file limit and a 2,000-symbol result limit. Hidden directories,
`.git`, and generated `.osiris` state are skipped. Open-document overlays take
precedence over indexed workspace content. Semantic tokens are capped at
10,000 tokens. Workspace refresh and workspace-symbol queries honor context
cancellation before and during traversal.

Document and workspace symbols

`textDocument/documentSymbol` returns a JSON array of flat `DocumentSymbol`
objects for definitions owned by the requested open document. Each item has
`name`, numeric `kind`, `range`, and `selectionRange`; `detail` is included
only when the analyzer has a declaration detail. Ranges are converted from
Osiris source spans to LSP UTF-16 line/character coordinates, and the
selection range identifies the declaration span. Results are sorted by source
range and then name, so repeated requests have identical response order.
The request may include `textDocument.version`; a version other than the
open-document version is rejected with JSON-RPC `-32602` before analysis.
Cancellation returns JSON-RPC `-32800` and never returns a partial array.

Both symbol providers perform one final cancellation check after collection and
stable sorting, immediately before the response is published. This protects
the transaction boundary even when cancellation arrives after the last symbol
has been collected; the focused contract is recorded in
`docs/design/LSP-SYMBOL-REQUESTS.md`.

`workspace/symbol` returns a JSON array of `WorkspaceSymbol` objects with
`name`, numeric `kind`, optional `detail`, and a `location` containing the
source URI and UTF-16 range. The optional query is trimmed and matched
case-insensitively against symbol names. Results are sorted by symbol name and
URI and are capped at 2,000 items. Workspace indexing uses open-document
overlays and the bounded workspace roots described above. Cancellation is
checked before refresh, between documents, and between definitions; canceled
requests return `-32800` without a partial result.

## Workspace file watching

`workspace/didChangeWatchedFiles` is a notification with the exact parameter
shape `{"changes":[{"uri":"file:///...","type":1}]}`. Each change must
contain a valid file URI and an LSP file-change type: `1` for created, `2` for
changed, or `3` for deleted. The server validates the complete batch before
performing any work; malformed entries are logged for notifications and do not
partially refresh the index.

After a valid batch, the server performs one bounded `refreshWorkspace` pass.
Open documents remain authoritative overlays, newly created `.osr` files enter
workspace symbols and module navigation, changed files are reparsed, and
deleted files disappear from the disk-backed index. Symlink entries and files
outside configured roots remain excluded. Refresh cancellation leaves the
previous workspace index intact. Clients may send this notification after
registering their ordinary filesystem watcher; the server does not spawn a
background watcher or retain OS file descriptors.

When `textDocument/didClose` removes an editor overlay, the server runs the
same bounded disk-rooted restoration boundary for that URI. The file is parsed
and analyzed privately, then installed in the workspace index only after the
read, parse, analysis, and cancellation checks succeed. If the file was
deleted, moved outside the configured roots, or is no longer an `.osr` file,
its stale workspace entry is removed. This prevents closed documents from
continuing to expose pre-open content or partial analysis.

## Test and benchmark discovery

`osiris/testInventory` consumes the shared language-test discovery parser. It
returns stable schema-versioned test metadata, including skipped,
expected-failure, benchmark, fuzz, and source-location fields. Code lenses use
the same inventory and produce `osiris.test` or `osiris.benchmark` commands;
they do not execute test code inside the language server. Requests may target
either a colocated `name.test.osr`/`name_test.osr` file or its neighboring
production source; the latter resolves the sidecar using the same deterministic
rule as `osiris test`, so an editor and the CLI select the same test file.
When the sidecar is open, its current overlay takes precedence over disk. The
inventory reports the selected sidecar path, package, module, test names,
flags, and one-based source locations; code-lens commands use that same path
and metadata.

## Request and response contract

The server returns one JSON-RPC response for every request, including an error
response. Notifications never receive a response. The supported request
methods and their result shapes are:

| Method | Result shape | Source of truth |
| --- | --- | --- |
| `textDocument/hover` | `Hover` or `null` | analyzed definition/reference |
| `textDocument/definition` | `Location`, `Location[]`, or `null` | analyzed definition |
| `textDocument/typeDefinition` | `Location` or `null` | checked binding type |
| `textDocument/implementation` | `Location[]` | analyzed function/trait method definitions |
| `textDocument/references` | `Location[]` | resolved reference targets |
| `textDocument/documentHighlight` | `DocumentHighlight[]` | same-document resolved symbol occurrences |
| `textDocument/completion` | completion item array | lexical scope, typed members, and module exports |
| `textDocument/signatureHelp` | `SignatureHelp` or `null` | checked callable signature |
| `textDocument/documentSymbol` | document symbol array | source definitions |
| `workspace/symbol` | workspace symbol array | bounded workspace index |
| `workspace/didChangeWatchedFiles` | notification; no response | one bounded workspace refresh per valid batch |
| `textDocument/semanticTokens/full` | `{data: number[]}` | lexical tokens and definitions |
| `textDocument/inlayHint` | inlay hint array | checked binding types |
| `textDocument/foldingRange` | folding range array | source spans and comments |
| `textDocument/selectionRange` | one selection tree per position | UTF-16 source offsets |
| `textDocument/codeAction` | code action array | diagnostics and import organizer |
| `textDocument/formatting` | text edit array | `FormatSource` |
| `textDocument/diagnostic` | `{kind: "full", items: [...]}` | stored document diagnostics |
| `textDocument/codeLens` | code-lens array | shared language-test inventory |
| `textDocument/prepareRename` | range or `null` | resolved rename target |
| `textDocument/rename` | versioned `WorkspaceEdit` | resolved definitions/references |
| `callHierarchy/prepare` | call-hierarchy item array | function definitions |
| `callHierarchy/incomingCalls` | incoming-call array | resolved call references |
| `callHierarchy/outgoingCalls` | outgoing-call array | resolved call references |
| `typeHierarchy/prepare` | type-hierarchy item array | type definitions |
| `typeHierarchy/supertypes` | trait item array | `impl Trait for Type` declarations |
| `typeHierarchy/subtypes` | implementing type array | `impl Trait for Type` declarations |
| `osiris/testInventory` | schema-versioned inventory | shared test discovery |

### `textDocument/typeDefinition`

The request requires `textDocument.uri` and `position`; an optional
`textDocument.version` is checked against the current open-document overlay
before analysis. The position is zero-based UTF-16. Missing parameters,
unknown documents, stale versions, out-of-document positions, and positions
inside an astral-character surrogate pair are rejected with JSON-RPC `-32602`.

The server first resolves the analyzer identity under the cursor. A cursor on
a source-backed nominal type reference (`struct`, `enum`, `trait`, `type
alias`, or `newtype`) navigates directly to that declaration. For a source
identity that carries a declared type name, the server gathers matching
source-backed declarations across the open and workspace documents, then
selects the deterministic URI/source-span minimum. Generated, virtual,
unresolved, and invalid-span identities return `null`; no fabricated source
range is emitted.

`languageServer.typeDefinitionContext` checks the request context before
lookup, after reference resolution, during candidate traversal, after sorting,
and immediately before publishing the complete `Location`. Cancellation
returns `-32800` and never publishes a partial result. The legacy
`languageServer.typeDefinition` wrapper supplies a background context for
in-process callers; JSON-RPC dispatch always supplies the live request
context.

### `textDocument/implementation`

The request has the same URI, optional-version, and strict UTF-16 position
contract as `textDocument/typeDefinition`. It returns a `Location[]`, with an
empty array for an analyzed cursor that has no callable implementation. A
malformed, stale, unknown, out-of-document, or surrogate-splitting request
returns `-32602`.

`languageServer.implementationsContext` traverses the sorted open/workspace
document set and considers only valid source definitions classified as
`function` or `trait method`. Duplicate URI/source-span identities are
discarded. The complete result is sorted by URI, start line, start character,
end line, and end character before publication. Cancellation is checked before
lookup, after semantic resolution, during every document and definition
iteration, after sorting, and before return; it returns `-32800` with a nil
result rather than partially publishing locations. The compatibility
`languageServer.implementations` wrapper uses a background context, while
JSON-RPC dispatch uses the request context.

The normal and cancellation contracts are covered by
`lsp_type_navigation_cancellation_test.go`, including repeated-result
determinism, malformed and stale requests, UTF-16 boundaries, pre-cancelled
requests, mid-request cancellation, and context-aware dispatch.

Call hierarchy uses the definition identity carried by the prepared item, not
just its display name. `callHierarchy/prepare` accepts a text-document URI,
an LSP position, and an optional document version; both the URI and position
are required. It returns one item only when the position resolves to a
function, macro, or trait method. Missing parameters or a stale version return
`-32602`, and cancellation returns `-32800` before or during analysis.

`callHierarchy/incomingCalls` requires an `item` object with non-empty `name`
and `uri` fields, plus the ordinary LSP call-hierarchy item ranges. The item’s
`data` identity is honored when present, so two same-named definitions in
different files cannot be conflated. The response is an array of one record
per caller: `{from, fromRanges}` groups every matching call-site range for
that caller. Top-level references are not reported as callers. Every range is
converted to UTF-16 coordinates, ranges inside each group are sorted by
position, and caller records are sorted by URI, name, and first range. A
missing or empty item identity is malformed and returns JSON-RPC `-32602`; it
is never treated as a successful empty query. Cancellation returns `-32800`
without a partial array.

`callHierarchy/outgoingCalls` resolves the selected caller by the same
identity rule and scans only call references contained by that function,
macro, or trait method. It returns one `{to, fromRanges}` record per distinct
target definition, aggregates repeated call sites, emits UTF-16 ranges, and
uses the same deterministic ordering as incoming calls. References from a
different function in the same document are excluded. Malformed identity
returns `-32602`, and cancellation before or during the scan returns `-32800`.

The framed `lsp-call-hierarchy.jsonl` fixture covers prepare, incoming, and
outgoing results, a malformed item, exact identity data, and deterministic
Content-Length framing. The semantic call-hierarchy test additionally checks
an astral Unicode argument: the call-site range remains the exact UTF-16
range reported to the client.

`typeHierarchy/supertypes` and
`typeHierarchy/subtypes` apply the same required-item identity rule. Missing or
empty `name` or `uri` fields return `-32602` before the analyzer is queried;
valid but unresolved identities may return an empty result according to the
corresponding hierarchy contract.

Hierarchy traversal is context-aware and transactional. The server checks
cancellation before resolving the item, between indexed documents and
statements, after resolving each implementation target, and after deterministic
sorting. A canceled `prepare`, `supertypes`, or `subtypes` request returns
`-32800` with no partial hierarchy array. The compatibility helpers
`prepareTypeHierarchy`, `typeSupertypes`, and `typeSubtypes` use a background
context for direct internal callers, while the JSON-RPC path passes the
request context through `prepareTypeHierarchyContext`,
`typeSupertypesContext`, and `typeSubtypesContext`.

### Source-backed rename

`textDocument/prepareRename` accepts `textDocument.uri`, an optional document
`version`, and a UTF-16 `position`. The server returns the exact range of the
identifier at that position, not automatically the declaration range. A
position on a resolved local binding, function or method, struct field,
generic type parameter, macro declaration or parameter, imported module alias,
or a member imported from a source-backed user module returns a source range.
The request may target either an open document or a workspace-indexed `.osr`
file; open-document overlays take precedence. A position on an unresolved
name, a virtual standard-library export (`osiris://stdlib/...`), an invalid
UTF-16 boundary, or a symbol whose identity cannot be proven returns `null`
(invalid request data still returns `-32602`).

`textDocument/rename` accepts the same document/version/position fields plus
the required identifier `newName`. The result is a versioned
`WorkspaceEdit` whose edits are source-backed and identity-based: lexical
shadowing, same-spelled fields on unrelated types, and unresolved references
are not included. For an exported member of a local user module, a rename
started at either the importer reference or the defining module includes the
definition, explicit export reference, and all resolved importer references
across the indexed workspace. Each document's edits are UTF-16 ranges, sorted
from the end of the document toward the beginning for safe application,
deduplicated, and rejected if any source spans overlap. `documentChanges` are
ordered by URI and carry the document version captured when the edit was
built; workspace-indexed files use their indexed version until an open overlay
provides a newer version.

Rename requests reject stale versions and malformed identifiers with
`-32602`. Cancellation is checked before analysis, while scanning definitions
and references, and before the result is returned; canceled requests use
`-32800` and never return a partial edit. The analyzer identity boundary is
implemented by `documentAnalysis.findAt`, `sourceBackedRenameTarget`,
`languageServer.sourceBackedRenameOccurrence`, `sameRenameDefinition`,
`languageServer.prepareRenameContext`, and
`languageServer.renameWorkspaceEditContext`.

The methods above are advertised by `initialize` and have framed protocol
coverage in `internal/osiris/testdata/lsp/lsp-protocol.jsonl`, with
additional semantic and workspace fixtures in `lsp-semantic.jsonl` and
`lsp-workspace.jsonl`, `lsp-workspace-import-actions.jsonl`,
`lsp-workspace-test-inventory.jsonl`, `lsp-workspace-code-actions.jsonl`,
`lsp-workspace-semantic-tokens.jsonl`,
`lsp-workspace-symbols.jsonl`, `lsp-workspace-diagnostics.jsonl`,
`lsp-workspace-ide-ranges.jsonl`, and `lsp-workspace-rename.jsonl`; the focused
`TestLSPWorkspaceSourceBackedUserModuleRenameFromImporter` test verifies a
cross-document prepare/rename from an importer reference and from the indexed
definition. A method outside this table returns JSON-RPC
`-32601`; malformed parameters return `-32602`. An unsupported language or
AST form produces a source-ranged diagnostic rather than an empty successful
result.

### Inlay hints

`textDocument/inlayHint` accepts `textDocument` and a required `range`. The
optional document `version` is checked before analysis. The returned array is
sorted by UTF-16 position and then by label. Each item has the exact fields
`position`, `label`, `kind`, and `paddingLeft`; `label` is the checked Osiris
type string, `kind` is `1` (`Type`), and `paddingLeft` is `true`. Hints are
emitted after checked bindings and only when the binding span overlaps the
requested range. Missing or reversed ranges are `-32602` errors, and a
canceled request returns `-32800` without a partial result.

### Folding ranges

`textDocument/foldingRange` accepts `textDocument` and returns an array of
zero-based LSP line ranges. Multiline definitions produce `kind: "region"`
ranges with UTF-16 start and end characters. Consecutive `//` lines produce a
single `kind: "comment"` range; a one-line comment is not advertised as a
fold because it has no foldable extent. Results are sorted by start position,
end position, and kind. The document version is validated before the result
is computed; malformed or stale requests return `-32602`, and cancellation
returns `-32800`.

### Selection ranges

`textDocument/selectionRange` requires a `positions` array and returns one
selection tree for every input position, in the same order. Positions are
validated at UTF-16 code-point boundaries. An identifier position selects the
identifier, whose parent is the containing line and whose next parent is the
full document. A position on punctuation or whitespace gets a zero-width
point selection followed by the same line and document parents. Missing
positions, invalid UTF-16 offsets, and stale document versions return
`-32602`; cancellation returns `-32800`.

## Core navigation request transactions

The four core text-document requests have explicit context-aware entry points
and compatibility wrappers for direct internal callers:

| Protocol method | Context-aware symbol | Compatibility symbol | Result transaction |
| --- | --- | --- | --- |
| `textDocument/hover` | `languageServer.hoverContext` | `languageServer.hover` | A private `lspHover` is built and returned only after source lookup, markdown construction, and UTF-16 range conversion complete. |
| `textDocument/definition` | `languageServer.definitionContext` | `languageServer.definition` | A private `lspLocation` is built, including cross-document range conversion, before the final cancellation check. |
| `textDocument/completion` | `languageServer.completionContext` | `languageServer.completion` | Typed-member, standard-module, and lexical candidates are collected into private storage; cancellation never returns a partially collected array. |
| `textDocument/prepareRename` | `languageServer.prepareRenameContext` | `languageServer.prepareRename` | Only a source-backed occurrence is returned, and the range is published after the identity and UTF-16 position checks complete. |

`languageServer.Serve` passes its request context directly to the first three
context-aware symbols. `textDocument/prepareRename` is routed through
`languageServer.handleIDERequest`, which passes the same context to
`prepareRenameContext`; `textDocument/rename` uses the corresponding
`renameWorkspaceEditContext` transaction. The compatibility symbols use a
background context so existing in-process callers retain the original API
without bypassing the cancellation contract on the wire.

Each context-aware method checks cancellation before reading the document and
again at the result boundary. Completion additionally checks while traversing
module statements, module members, and lexical definitions. A canceled
request returns JSON-RPC `-32800` with a nil result. No response object, edit,
or completion slice assembled before cancellation is published, and none of
these request methods mutates the open-document overlay.

`internal/osiris/lsp_core_navigation_cancellation_test.go` verifies identical
normal results across repeated calls, late cancellation for all four methods,
and framed `Serve` dispatch for each protocol method. The existing
`internal/osiris/testdata/lsp/lsp-protocol.jsonl` fixture continues to provide
deterministic normal wire coverage for hover, definition, completion, and
prepare-rename responses.

## Versions, cancellation, and edits

Open and changed documents carry a monotonically increasing integer version.
The server rejects duplicate or older non-zero versions before replacing the
document or publishing diagnostics. Analysis is performed outside the server
lock and is discarded if its context is canceled. A canceled request uses the
JSON-RPC `-32800` code; a canceled server context is returned to the caller.

The standard LSP request structures do not normally include a document version,
so Osiris accepts an optional `textDocument.version` assertion on version-
sensitive requests. When present, a mismatch returns `-32602`. Rename and
organize-import code actions additionally return `WorkspaceEdit.documentChanges`
with one versioned document identifier per edited file. Edits are sorted by URI
and then from the end of each document toward the beginning, so applying them
does not invalidate an earlier range. The server never mutates the open text.

`textDocument/codeAction` requires a range whose UTF-16 endpoints are valid in
the current open document and whose end is not before its start. A missing,
reversed, or out-of-document range returns `-32602` before any action is
constructed. The request context is checked before validation, while scanning
diagnostics and imports, and after the edit is assembled; cancellation returns
`-32800` with no edits, so the server cannot expose a partially assembled
transaction.

The method returns a `quickfix` titled `Migrate deprecated API` when the
requested diagnostics contain a deprecation message. It also returns a
`source.organizeImports` action when the document has an out-of-order leading
import block. Import ordering is by module path and then alias. The action
replaces only the import statement spans, keeps unrelated source outside those
spans untouched, emits UTF-16 ranges in descending document order, and
includes the document's current version in `WorkspaceEdit.documentChanges`.
Imports after a non-import declaration are not moved because the AST cannot
prove that crossing intervening source is safe; `textDocument/formatting`
remains the explicit whole-document operation. Applying the action is a client
transaction: the server does not mutate its open overlay, and an optional
stale `textDocument.version` assertion returns `-32602` before any edit is
produced. The focused framed evidence is
`internal/osiris/testdata/lsp/lsp-workspace-code-actions.jsonl`; its canceled
wire request is executed with the same fixture prefix and must return
`-32800` without a result.

## Whole-document formatting

`textDocument/formatting` accepts `textDocument.uri` and an optional
`textDocument.version`. The server snapshots the open document, rejects a
stale version before formatting, and returns one full-document UTF-16 edit
when `FormatSource` changes the text; an already formatted document returns an
empty edit array. Formatting never mutates the server's open-document overlay.
The request context is checked before the formatter starts and after the
complete edit is assembled. Cancellation therefore returns `-32800` without
publishing a partial edit. The synchronous formatter is intentionally not
interrupted in the middle of its pure formatting pass; the post-pass check is
the transaction boundary. `TestLSPFormattingCancellationIsTransactional`
covers the canceled request, unchanged overlay, and exact protocol error.

## Workspace roots and test ownership

`initialize` accepts `rootUri`, `rootPath`, and all `workspaceFolders`. Paths are
absolute, deduplicated, and sorted before indexing. Each root is walked without
following symlink directories; hidden directories, `.git`, and `.osiris` are
skipped. The index is capped at 512 Osiris files and 4 MiB per file. Parse
failures remain indexed with diagnostics, while an interrupted refresh leaves
the previous index intact. Open-document overlays always replace indexed
copies.

`osiris/testInventory` accepts a test file, an ordinary source file, or a
configured workspace-directory URI.
For an ordinary source file it discovers the colocated `name.test.osr` or
`name_test.osr` file using the same discovery rule as the CLI. The response
contains the source file list, test locations and flags, the declared package,
and the nearest `osiris.mod` module path. These fields are metadata only; test
execution remains owned by the language-test runner. If workspace roots were
provided, a discovered sidecar must be lexically and symlink-resolved inside
one of those roots; a symlink pointing outside is rejected without reading the
target. Canceled inventory and code-lens requests return JSON-RPC `-32800`.
With no configured root, an explicitly opened document may use its neighboring
sidecar, preserving standalone editor behavior.

The same request also accepts a configured workspace-directory URI when the
URI is not an open document. In that form the server performs the CLI's default
recursive discovery below that directory: it includes both `.test.osr` and
`_test.osr` files, sorts absolute paths lexicographically, ignores hidden
directories plus `.git` and `.osiris`, excludes symlink entries, and stops at
the workspace file and per-file-size limits. The current open overlay wins for
each discovered test file. The response contains every discovered file and its
declarations, uses the nearest module manifest when one is available, and
leaves single-file package ownership empty when the root can contain multiple
packages. A workspace-directory URI must be inside a configured workspace
root; an unconfigured or outside directory is rejected without reading it.
The traversal checks cancellation before each filesystem entry and before each
parse, so a canceled request returns `-32800` without partial inventory.

The support matrix remains conservative: LSP cells stay `partial` until the
corresponding feature has a maintained fixture and documentation route. The
matrix test rejects orphaned evidence and the release gate is opt-in, so an
unsupported or untested feature cannot become `stable` through documentation
changes alone.
