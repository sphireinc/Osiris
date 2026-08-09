# LSP references

`textDocument/references` returns every source location that shares the
analyzer identity selected at the request position. The method is advertised
by `languageServer.capabilities` as `referencesProvider: true` and is dispatched
through `languageServer.handleIDERequest`, so the request context remains
available for the entire lookup.

## Request

The request parameters are:

```json
{
  "textDocument": {
    "uri": "file:///workspace/main.osr",
    "version": 7
  },
  "position": {
    "line": 4,
    "character": 12
  }
}
```

`textDocument.uri` is required. `textDocument.version` is optional; when it is
present it must equal the open document version. `position` is required and is
zero-based UTF-16. A position that is outside the document or between the two
UTF-16 code units of an astral character is invalid.

Malformed parameters, a missing position, an invalid UTF-16 position, or a
stale version return JSON-RPC `-32602`. The server does not turn an invalid
cursor into a successful empty result because that would hide client
coordinate bugs.

## Result and identity

The result is a JSON array of LSP `Location` objects. The analyzer first
resolves the definition or reference under the cursor. It then returns the
source-backed definition location, when one exists, and every analyzed
reference whose `target` is the same symbol identity. Virtual standard-library
definitions are not emitted as locations; references to them still resolve to
the same analyzer identity and are returned.

Locations are sorted by URI, start line, start character, end line, and end
character. The order is therefore stable across repeated requests and does not
depend on Go map iteration or workspace traversal order. The returned slice is
owned by the request and is never reused by a later request.

## Cancellation and publication boundary

`languageServer.referencesContext` checks the request context before document
lookup, after cursor resolution, before each document, before each candidate
reference, and after sorting. It builds the result in a private slice and
returns JSON-RPC `-32800` with no partial locations if cancellation is observed
at any checkpoint. The open document and workspace index are read-only during
the operation.

The ordinary `languageServer.references` method remains a compatibility
wrapper using `context.Background`; protocol requests use the context-aware
method. This keeps direct library callers source-compatible while making the
wire method transactional with respect to cancellation.

## Evidence

`internal/osiris/lsp_references_cancellation_test.go` verifies deterministic
framed responses, exact definition/reference locations, missing and invalid
positions, stale versions, cancellation before scanning, and cancellation
during scanning. The request sequence is recorded in
`internal/osiris/testdata/lsp/lsp-references.jsonl`.
