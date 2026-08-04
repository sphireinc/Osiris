# Task 139: Complete parser and source-surface parity

**Priority:** P1
**Dependencies:** -
**Status:** Complete

## Objective

Create a complete parser conformance contract for every language form exposed
by the backend support matrix, including the declaration forms currently
accepted by files but not uniformly exercised by tools.

## Exact scope and ownership

- Own `internal/osiris/lexer.go`, `internal/osiris/parser.go`, parser fixtures,
  and parser-focused conformance tests.
- Inventory tokens and productions for bindings (`const`, `let`, `var`),
  functions, typed/generic signatures, structs, tuples, aliases, newtypes,
  traits/interfaces/impls, iterators, modules, macros, patterns, async forms,
  errors/results/options, and future reserved names.
- Make valid syntax and invalid syntax diagnostics source-spanned and stable.
- Add parse/format/parse corpus fixtures for every matrix feature.
- Do not change runtime semantics, backend lowering, formatter implementation,
  or LSP protocol conversion except where a parser-owned source span is wrong.

## Acceptance criteria

- Every parser feature in the matrix has valid, invalid, boundary, and source
  location fixtures.
- Parser output is identical regardless of whether it is entered through file,
  REPL, formatter, LSP, native, WASM, or GPU entry points.
- No backend reparses or invents syntax rules.
- The parser conformance corpus is used by later backend tasks.

## Parallelism

This is the root task for the new parity wave. Complete it before assigning
tasks 138, 140, 141, 142, 149, or 152; those tasks consume its conformance
corpus and source-span contract.

## Implementation evidence

- `internal/osiris/lexer.go:nextToken`, `scanString`, and `scanTag` preserve
  source spelling and report positioned lexical errors; leading-dot numeric
  literals are accepted without changing ordinary tokenization.
- `internal/osiris/parser.go:parseWithDeclarationContext` provides shared
  declaration context for source fragments, while `parserSpanFromToken`,
  `parserSpanFromTokens`, and the statement/expression production helpers
  propagate stable source spans through nested syntax.
- `parser.expectTypeCloser` handles nested generic closers by splitting `>>`
  only while parsing type arguments or parameters; shift-right expressions keep
  their normal token semantics.
- `internal/osiris/task139_parser_conformance_test.go` covers valid language
  fixtures, invalid and boundary diagnostics, nested generics, source spans,
  and format/parse round trips.

Validation: the focused `TestTask139*` and parser/generic/numeric/diagnostic
suites, the complete `internal/osiris` suite, `go test ./...`, `go vet ./...`,
and `git diff --check` pass.
