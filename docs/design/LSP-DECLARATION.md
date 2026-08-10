# `textDocument/declaration`

Osiris implements the standard LSP declaration-navigation request for clients
that distinguish a declaration location from a definition location. Osiris'
source model uses one source-backed identity for both concepts, so a valid
declaration request returns the same location as `textDocument/definition`.
This keeps editor navigation consistent without inventing a second range for a
single binding, function, type, field, or method.

## Request

The request parameters are:

```json
{
  "textDocument": {
    "uri": "file:///workspace/greetings.osr",
    "version": 7
  },
  "position": {
    "line": 3,
    "character": 12
  }
}
```

`textDocument.uri` is required and must identify an open Osiris document. The
optional `textDocument.version` is an optimistic concurrency assertion; when
present, it must equal the current overlay version. `position` is required and
uses zero-based UTF-16 code units. A position outside the document or between
the two code units of an astral-character surrogate pair is invalid.

## Result

The result is one `Location` or `null`:

```json
{
  "uri": "file:///workspace/greetings.osr",
  "range": {
    "start": {"line": 0, "character": 4},
    "end": {"line": 0, "character": 10}
  }
}
```

The target URI and range come from the analyzer's source-backed `symbolDef`.
The server uses the defining document's current open overlay when converting
the source span to UTF-16. Generated, virtual, unresolved, and zero-span
identities return `null`; the server never fabricates a location.

## Errors and cancellation

Malformed parameters, unknown documents, stale versions, negative positions,
out-of-document positions, and surrogate-splitting positions return JSON-RPC
`-32602`. A request canceled through `$/cancelRequest` returns `-32800` and no
partial location. Cancellation is checked before lookup, before delegating to
the shared source-definition resolver, and at the resolver's publication
boundary. The request does not mutate the document overlay.

`languageServer.declarationContext` is the context-aware implementation;
`languageServer.declaration` is the background-context compatibility wrapper
for in-process callers. The framed protocol fixture
`internal/osiris/testdata/lsp/lsp-protocol.jsonl` covers the advertised
capability, a valid declaration lookup, and deterministic response framing.
`TestLSPDeclarationNavigationMatchesSourceDefinition`,
`TestLSPDeclarationNavigationValidatesRequests`, and
`TestLSPDeclarationNavigationCancellationIsTransactional` cover source ranges,
UTF-16 positions, stale and malformed requests, and cancellation.
