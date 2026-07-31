# Refactoring and rewrite tools

Source: CHECKLIST.md.

Build on parser, formatter, source maps, and LSP. Start with rename plus a
dry-run source rewrite API; do not claim general refactoring support first.

Acceptance: rename respects lexical scope and imports, produces formatted output,
and has fixture and LSP integration tests.

## Active increment: lexical dry-run rename

- `languageServer.RenameDocument` resolves the definition/reference graph from
  the existing semantic analysis and returns formatted source without mutating
  the open document.
- Scope and import tests cover shadowed parameters and module aliases. The
  server advertises and handles `textDocument/rename`, returning one atomic
  formatted WorkspaceEdit for an open document.
- Builtins, unresolved names, invalid identifiers, and cross-document targets
  fail explicitly. General multi-file/project-wide rename remains out of scope
  for this first task increment.
