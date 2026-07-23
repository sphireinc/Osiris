# Refactoring and rewrite tools

Source: CHECKLIST.md.

Build on parser, formatter, source maps, and LSP. Start with rename plus a
dry-run source rewrite API; do not claim general refactoring support first.

Acceptance: rename respects lexical scope and imports, produces formatted output,
and has fixture and LSP integration tests.
