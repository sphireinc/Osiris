# Core Semantics 1 conformance corpus

This directory is the executable companion to
`docs/design/SEMANTICS-CORE.md`. Each `.osr` program has one adjacent
`.golden.json` file. A fixture describes one observable rule; it must not rely
on Go implementation details, timing, or a backend fallback.

Metadata is deliberately explicit:

```json
{
  "version": "Core Semantics 1",
  "rule": "13.1",
  "category": "patterns",
  "output": "6u64\\n",
  "backends": {
    "interpreter": "required",
    "vm": "required",
    "native": "required",
    "wasm": "skip"
  },
  "reason": "WASM implementation is not available yet."
}
```

Exactly one of `output` and `error_contains` is required. Backend statuses are
`required`, `reject`, and `skip`. `reject` means a backend must reject before
execution; `skip` is temporary and requires a reason. Interpreter and VM are
always required. Run the corpus with:

```sh
go test ./internal/osiris -run '^TestCoreSemanticsConformance$' -count=1
```

Adding or changing a language feature requires a matching rule citation,
fixture(s), and support-matrix update in the same change.
