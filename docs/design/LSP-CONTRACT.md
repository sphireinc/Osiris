# Osiris LSP contract

`osiris lsp` speaks JSON-RPC 2.0 over stdio using the Language Server Protocol
`Content-Length` framing. It is intentionally a small, dependable editor
surface rather than a claim of complete IDE support.

## Advertised capabilities

Initialization advertises only these capabilities:

| Capability | Contract |
| --- | --- |
| `textDocumentSync` | Open/close plus incremental (`change: 2`) updates. |
| `hoverProvider` | Hover for lexical definitions and resolved references. |
| `definitionProvider` | Go to definition for source-backed definitions. |

Completion, rename, references, formatting, code actions, semantic tokens,
workspace symbols, and diagnostics pull are not advertised. Adding any of
them requires a protocol fixture and source-location coverage first.

## Positions, ranges, and document changes

All protocol positions use zero-based UTF-16 code units, as LSP requires.
The lexer/source map currently stores columns as one-based UTF-8 byte offsets;
the LSP boundary converts between the two representations. This makes astral
Unicode characters work in editor requests, hover/definition ranges, and
analysis diagnostics without changing parser locations.

`textDocument/didChange` applies changes in array order against the current
document. A change with no range replaces the whole document. An invalid range
(including one that splits a UTF-16 surrogate pair) is ignored, logged to the
LSP log writer, and leaves the prior document state intact. A malformed or
incomplete source document remains open and receives a diagnostic; later valid
changes recover the same document state.

## Source-backed locations

Hover and definition use parser spans plus lexical analysis. Definitions for
builtins or other generated symbols have no source span and return `null`
rather than a misleading zero location. When a definition names another open
document, its range is converted using that target document's text.

## Cancellation and lifecycle

The server observes cancellation between protocol messages and returns the
context error. `shutdown` responds normally; `exit` stops the server. Clients
should cancel or close the input stream to unblock a server waiting for the
next framed message.

## Verification

`internal/osiris/lsp_test.go` covers framed initialization, honest capability
advertisement, Unicode positions and diagnostics, incremental edits, invalid
edit preservation, cancellation, malformed partial source recovery, module
and macro analysis, and generated-span behavior.
