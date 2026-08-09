# LSP document highlights

Osiris implements the standard `textDocument/documentHighlight` request for
open Osiris documents. The provider is source-backed: it follows the shared
`symbolDef` identity produced by the LSP analyzer instead of matching text by
name. Only occurrences in the requested document are returned.

## Request

```json
{
  "textDocument": {
    "uri": "file:///workspace/main.osr",
    "version": 7
  },
  "position": {
    "line": 3,
    "character": 12
  }
}
```

`textDocument` and `position` are required objects. `textDocument.uri` must be
non-empty and identify an open document. `textDocument.version` is optional;
when present it must equal the current overlay version. `position.line` and
`position.character` are required non-negative integers in zero-based LSP
coordinates.

The position is interpreted as UTF-16 code units. A position outside the
document, on a nonexistent line, or between the two UTF-16 code units of an
astral Unicode scalar is rejected with JSON-RPC `-32602`. Malformed JSON,
missing required objects or fields, unknown documents, and stale versions are
also rejected with `-32602`.

## Result

The result is an array of standard `DocumentHighlight` objects:

```json
[
  {
    "range": {
      "start": {"line": 1, "character": 4},
      "end": {"line": 1, "character": 10}
    },
    "kind": "write"
  },
  {
    "range": {
      "start": {"line": 3, "character": 12},
      "end": {"line": 3, "character": 18}
    },
    "kind": "read"
  }
]
```

`range` is always UTF-16 and is converted from the analyzer's UTF-8
byte-column source spans using the current document text. `kind` is one of
the LSP kinds `write`, `read`, or `text` (Osiris currently emits `write` and
`read`). A definition or mutable assignment target is `write`; a resolved
identifier, member, export, generic type reference, or pattern reference is
`read`. Duplicate ranges are removed. Results are sorted by start line,
start character, end line, and end character.

If the cursor does not resolve to an analyzed identity, or the identity has no
occurrence in the requested document, the successful result is an empty
array. Source-less generated and virtual definitions are never represented by
a fabricated range, although source references to a virtual identity may
still be highlighted in the open document.

## Cancellation and version publication

`languageServer.documentHighlightsContext` checks the request context before
lookup, while scanning references, after sorting, and immediately before
returning the private result. Cancellation returns JSON-RPC `-32800` and no
partial array. The method checks the requested version before analysis and
again against the live document before publication; a concurrent edit cannot
publish highlights computed from an older overlay.

The JSON-RPC dispatch is implemented by
`languageServer.handleIDERequest` in `internal/osiris/lsp_ide.go`. The
capability response advertises `documentHighlightProvider: true`. The result
shape is represented by `lspDocumentHighlight`, and strict required-field
decoding is represented by `lspDocumentHighlightParams`.

## Evidence

`TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests`
uses `internal/osiris/testdata/lsp/lsp-document-highlight.jsonl` to verify
capability advertisement, UTF-16 conversion after an astral scalar, read and
write kinds, deterministic wire output, malformed positions, stale versions,
surrogate-splitting positions, and out-of-document positions.

`TestLSPDocumentHighlightsAreDeterministicAndConcurrent` verifies repeated
concurrent results, and
`TestLSPDocumentHighlightsCancellationAndStaleVersionAreTransactional`
verifies early and late cancellation plus direct stale-version rejection.
