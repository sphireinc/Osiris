# Task 142: Make formatting one-to-one with the language

**Priority:** P1
**Dependencies:** 139
**Status:** Complete

## Objective

Guarantee that every valid language construct has a deterministic formatter
representation and that formatting never changes semantics or loses metadata.

## Exact scope and ownership

- Own `internal/osiris/format.go` and formatter/round-trip tests.
- Cover every declaration, type annotation, generic parameter/bound,
  trait/interface/impl method, iterator, pattern, macro, module, cleanup,
  task, error/result/option, reserved type, and standard-library expression.
- Preserve `const`/`let`/`var` distinctions, field tags, source-relevant spans,
  aliases, newtypes, backing values, and nested pattern bindings.
- Define stable whitespace, indentation, quoting, line wrapping, and malformed
  source behavior.
- Do not implement parser or backend semantics in the formatter.

## Acceptance criteria

- `format(format(source)) == format(source)` for the full corpus.
- `parse(format(parse(source)))` preserves the same semantic structure.
- Formatter output is identical for file, REPL, LSP, and CLI entry points.
- Every matrix feature has a checked formatting example.

## Parallelism

This task owns `format.go` and formatter tests and may run concurrently with
interpreter, VM, native, WASM, GPU, and REPL work.

## Completion evidence

- `internal/osiris/format.go` formats declarations, type parameters and
  bounds, receivers, traits/interfaces/impls, tags, enum backing values,
  macros, cleanup/task/error/result/option expressions, and nested patterns.
- `internal/osiris/formatter_roundtrip_test.go` covers parse/format/parse
  structure, idempotence, malformed input, source spans, and matrix forms.
- `docs/design/FORMATTER.md` documents the deterministic formatting contract.
- `GOCACHE=/tmp/osiris-go-cache go test ./internal/osiris -run '^TestFormatter' -count=1` passed.
