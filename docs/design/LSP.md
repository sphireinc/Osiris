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
- the Osiris test-inventory request `osiris/testInventory`.

The deterministic end-to-end protocol fixture at
`internal/osiris/testdata/lsp/task143-protocol.jsonl` sends every advertised
request and checks that each request receives a stable response. It also checks
initialize capabilities, diagnostic publication, malformed parameters, and an
unknown method without ending the session.

## Positions, changes, and diagnostics

Parser spans use the lexer’s UTF-8 byte columns. The server converts them to
UTF-16 only at the protocol boundary, including diagnostics, navigation
locations, semantic-token deltas, inlay hints, code lenses, and edits. An
incremental change is applied in the order received; a change without a range
replaces the complete document. A position that is outside the document or
splits an astral character’s surrogate pair is rejected. Invalid notification
changes are logged and leave the prior document unchanged. Malformed requests
receive JSON-RPC `-32602` and the server continues reading framed messages.

`textDocument/didSave` is a notification and therefore never receives a JSON-RPC
response. The server revalidates the current open-document overlay and publishes
fresh diagnostics. A client may include the optional saved `text`; when omitted,
the current overlay is revalidated. Revalidation keeps the document's existing
version. If a newer change or close wins while analysis is running, the saved
result is discarded instead of replacing newer state.

Malformed source is retained as the open document and produces a source
diagnostic. Generated or builtin definitions without a source span return no
location rather than a fabricated zero range.

## Completion and signature help

`textDocument/completion` returns deterministic, alphabetically sorted items.
At a module selector such as `text.` it enumerates the imported module's
exports; elsewhere it enumerates analyzed lexical definitions. Items use LSP
completion kinds for functions, types, variables, and modules and include the
definition detail where the analyzer has it. The request position is converted
from UTF-16 before the module selector is inspected, so an astral character
before the cursor does not shift the lookup boundary.

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
definition location.

`textDocument/signatureHelp` identifies the innermost unterminated call at the
cursor. Parentheses and commas inside nested calls are not counted as
arguments of the outer call, and quoted strings are ignored while locating the
call. For a source function, the response includes the checked signature,
one parameter label per declared parameter (including its declared type), and
the zero-based `activeParameter` selected by the commas before the cursor. A
cursor after the final argument keeps the active parameter clamped to the last
declared parameter. Both methods accept the optional
`textDocument.version`; a duplicate or older version is rejected with
JSON-RPC `-32602` before semantic data is returned.

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

The result is deterministic: tokens are sorted by `(line, UTF-16 start,
token type)`, capped at 10,000 entries, and returned with no implementation
fields beyond `data`. Requests accept an optional document version and reject
stale versions with `-32602`. Cancellation is checked before lexing and while
walking source lines and lexer tokens; a cancelled request returns `-32800`
without publishing a partial result. Incomplete source produces the tokens
available before the lexer error while the normal diagnostics path reports the
syntax error.

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
| `textDocument/completion` | completion item array | lexical scope, typed members, and module exports |
| `textDocument/signatureHelp` | `SignatureHelp` or `null` | checked callable signature |
| `textDocument/documentSymbol` | document symbol array | source definitions |
| `workspace/symbol` | workspace symbol array | bounded workspace index |
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

The methods above are advertised by `initialize` and have framed protocol
coverage in `internal/osiris/testdata/lsp/task143-protocol.jsonl`, with
additional semantic and workspace fixtures in `task160-semantic.jsonl` and
`task161-workspace.jsonl`, `task161-import-actions.jsonl`,
`task161-test-inventory.jsonl`, and `task161-semantic-tokens.jsonl`. A method outside this table returns JSON-RPC
`-32601`; malformed parameters return `-32602`. An unsupported language or
AST form produces a source-ranged diagnostic rather than an empty successful
result.

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

`textDocument/codeAction` returns a `quickfix` titled `Migrate deprecated API`
when the requested diagnostics contain a deprecation message. It also returns a
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
produced.

## Workspace roots and test ownership

`initialize` accepts `rootUri`, `rootPath`, and all `workspaceFolders`. Paths are
absolute, deduplicated, and sorted before indexing. Each root is walked without
following symlink directories; hidden directories, `.git`, and `.osiris` are
skipped. The index is capped at 512 Osiris files and 4 MiB per file. Parse
failures remain indexed with diagnostics, while an interrupted refresh leaves
the previous index intact. Open-document overlays always replace indexed
copies.

`osiris/testInventory` accepts either a test file or an ordinary source file.
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

The support matrix remains conservative: LSP cells stay `partial` until the
corresponding feature has a maintained fixture and documentation route. The
matrix test rejects orphaned evidence and the release gate is opt-in, so an
unsupported or untested feature cannot become `stable` through documentation
changes alone.
