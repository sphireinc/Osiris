# Osiris implementation conventions

- Treat `Value` as the language boundary: preserve kind, numeric width/style, shape, ownership, and metadata across evaluation, codecs, tasks, FFI, and backends.
- Keep semantics shared: syntax/value changes normally flow through lexer, parser, formatter, interpreter, VM, IR/native gates, tests, and canonical docs; do not add backend-local language rules.
- Prefer explicit source-spanned diagnostics and parse/format/parse stability tests.
- Preserve deterministic ordering in traversal, diagnostics, generated artifacts, dependency output, and analysis reports.
- Keep optimizations conservative around aliasing, effects, overflow, shape, and module identity; prove semantic equivalence.
- Native ownership must be explicit (owned/borrowed/retained), and unsupported native forms must fail at compile time.
- The detailed invariant list and review discipline are authoritative in `AGENT/AGENTS.md`; this memory records only the implementation patterns most useful for rediscovery.