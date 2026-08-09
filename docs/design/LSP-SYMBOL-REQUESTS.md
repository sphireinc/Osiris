# LSP symbol request cancellation

The `textDocument/documentSymbol` and `workspace/symbol` handlers treat a
request as transactional. They check the request context before and during
symbol collection, sort the response into its stable order, and check the
context once more before returning. If cancellation arrives during sorting or
the final response boundary, the handlers return JSON-RPC `-32800` and no
partial symbol array.

The focused regression is
`TestLSPSymbolRequestsCancelAfterCollectionWithoutPartialResults` with
`testdata/lsp/lsp-workspace-symbol-cancellation.osr`. It drives both methods
through `handleIDERequest`, first measures the normal context-check boundary,
then cancels at that boundary to verify the final post-sort check.
