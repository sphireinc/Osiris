# Task 138: Make the REPL one-to-one with language statement semantics

**Priority:** P1
**Dependencies:** 139
**Status:** Complete

## Objective

Make every valid top-level Osiris statement behave the same in `osiris repl`
as it does in a source file. Expression input may still receive the implicit
`emit` convenience, but declarations must never be rewritten as expressions.

## Exact scope and ownership

- Own `cmd/osiris/main.go:runRepl` and REPL-specific CLI tests.
- Recognize `const`, `let`, `var`, `func`, `struct`, `enum`, `type`, `newtype`,
  `trait`, `interface`, `impl`, `macro`, `package`, `export`, `import`,
  `defer`, and explicit `emit` as statement input.
- Preserve persistent session state, source locations, diagnostics, output
  ordering, and the expression convenience path.
- Add multiline input handling for compound declarations and blocks, with a
  deterministic incomplete-input prompt rather than partial execution.
- Do not duplicate parser or evaluator rules in the CLI; use `Parse` and
  `Session.ExecSource` as the only semantic path.

## Acceptance criteria

- `const e = 3` and `const limit = 2 + 3` work in the REPL.
- `var`, functions, structs, enums, traits, interfaces, impls, macros, and
  imports have the same behavior as equivalent source files.
- Expression input such as `list.reverse(x)` still behaves as `emit ...`.
- REPL tests compare results and diagnostics against `osiris run` fixtures.
- The formatter and LSP consume the same source forms without REPL-only syntax.
- Documentation explains statement input, expression input, multiline input,
  persistent bindings, and error recovery.

## Parallelism

This task edits only the CLI REPL path. It may run concurrently with formatter,
WASM, GPU, and LSP implementation tasks after parser task 139 is complete.

## Implementation evidence

- `cmd/osiris/main.go:runRepl` now delegates to `runReplWithInput`, which keeps
  one persistent `Session`, parser history, multiline input, and deterministic
  primary/continuation prompts.
- `cmd/osiris/main.go:replSourceForInputWithHistory` parses complete input first
  and applies implicit `emit` only to expressions; declarations and explicit
  statements use the same `Parse` and `Session.ExecSource` path as files.
- `cmd/osiris/main.go:replInputIncomplete` tracks strings, tags, comments,
  delimiters, and incomplete operators without executing partial input.
- `cmd/osiris/repl_parity_test.go` and `cmd/osiris/repl_test.go` cover bindings,
  functions, types, structs, enums, traits, interfaces, impls, macros, imports,
  expression input, multiline atomicity, persistence, history, and diagnostics.
- `docs/design/REPL.md` and `docs/src/cli-reference.json` document the complete
  input contract, prompts, persistence, recovery, and source locations.

Validation: `go test ./cmd/osiris -count=1`, `go test ./... -count=1`,
`go vet ./...`, and the Node 24 reference/navigation documentation check pass.
