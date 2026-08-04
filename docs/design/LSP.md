# Osiris LSP contract

The Osiris language server is a stdio JSON-RPC 2.0 endpoint. Requests use
`Content-Length` framing, and all document positions and ranges are zero-based
UTF-16 code-unit coordinates. The parser and checker remain the source of
language truth; the LSP layer converts their source spans and checked types to
protocol values.

## Advertised surface

The initialize response advertises only the methods implemented by the server:

- incremental open/change/close synchronization;
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

Malformed source is retained as the open document and produces a source
diagnostic. Generated or builtin definitions without a source span return no
location rather than a fabricated zero range.

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
they do not execute test code inside the language server.
