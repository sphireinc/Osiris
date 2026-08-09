# Formatter contract

The formatter is a syntax-preserving projection of the current parser AST. It
does not type-check, evaluate, expand macros, or choose a backend. The public
entry point is `FormatSource(sourceName, source)` in
`internal/osiris/format.go`; `FormatProgram` is the shared renderer used after
parsing. File, REPL, LSP, and CLI callers must obtain the same text from these
entry points.

## Round-trip invariants

For every source accepted by `Parse`:

1. `FormatSource` succeeds and returns a deterministic representation.
2. Formatting the returned representation produces byte-identical text.
3. Parsing the returned representation preserves the semantic AST structure.
4. Declaration and value metadata remains present: `const`/`let`/`var` and
   `let?` distinctions, aliases, newtypes, enum backing values, struct tags,
   generic bounds, and nested pattern bindings are not discarded.
5. The newly parsed representation still receives source spans for diagnostics
   and tooling. Spans describe the formatted source positions; they are not
   expected to equal the original positions after whitespace normalization.

The semantic comparison intentionally ignores source positions and the parser's
temporary `defaultInteger` fitting flag. It retains every declaration, type
spelling, expression, pattern, literal value/style, tag, alias, and backing
value. This is exercised by
`TestFormatterLanguageCorpus` and
`TestFormatterMetadataSpansAreRecordedBeforeRoundTrip` in
`internal/osiris/formatter_roundtrip_test.go`.

## Canonical rendering rules

- Statements are separated by one newline, and a non-empty program ends with
  one newline. Semicolons and comments are normalized away.
- Nested blocks and match arms use one-line braces with spaces and semicolon
  separators. Expression-bodied functions use `=>`; block-bodied functions
  retain their block.
- Binary operators are surrounded by one space and parentheses are inserted
  according to the parser's precedence. The parser's current unary binding
  rule is retained, so `-1 + 2 * 3` canonicalizes as
  `-(1u64 + 2u64 * 3u64)`.
- Calls, generic call arguments, members, indexes, struct literals, `with`
  updates, lists, tuples, `try`/`catch`, propagation, and `match`/`switch`
  use stable comma/space spelling. `emit a, b` is rendered as one emit with
  comma-separated expressions.
- Numeric values use the canonical `Value.String()` spelling. This preserves
  numeric family and width/style, but may normalize a source base such as
  `0x2a` to `42u64`.
- Strings and imports use escaped double-quoted text. Struct field tags retain
  their raw backtick contents, including unknown tag namespaces.
- `trait`, `interface`, and `impl` are rendered through their AST forms. The
  parser deliberately gives `trait` and `interface` the same semantic trait
  node, so the canonical declaration spelling is `trait`.

## Conformance matrix

The formatter corpus is table-driven. The following rows name the accepted
parser surface and the checked fixture that owns it:

| Matrix row | Coverage |
| --- | --- |
| Module metadata | `package` with version, `export`, `import`, and `ffi` |
| Bindings and mutation | `const`, `let`, typed/generic `let`, `let?`, tuple destructuring, `var`, assignment |
| Declarations | enums with payloads and string/integer backing, tagged structs, aliases, newtypes |
| Behavior | generic `trait`, contextual `interface`, `impl`, methods, receiver and signature metadata |
| Control and cleanup | expression/block functions, `return` with and without a value, `while`, `for`, `break`, `continue`, `defer` |
| Expressions | literals, calls, generic calls, blocks, lists, tuples, struct literals, members, indexes, `with`, unary/binary operators, `try`/`catch`, propagation, `match`, `switch` |
| Patterns | wildcard, binding, enum payload, named-product/struct fields, list, literal, relational, type-binding, conjunction, negation, and grouped alternatives with nested bindings |
| Types and values | every current built-in parser type name, function types, nested generic type arguments, width/base/decimal/complex numeric forms |
| Library surface | representative `std/text`, `std/math`, option/result, and tensor expressions as ordinary parsed calls |
| Source robustness | comments, semicolons, irregular whitespace, malformed expression/lexer/tag/enum/match input, and diagnostic locations |

Each row checks both `format(format(source)) == format(source)` and
`parse(format(parse(source)))` through the shared test helper. New parser
constructs must add a named row and a checked output fragment before being
considered formatter-complete.

## Malformed input

`FormatSource` returns the parser or lexer diagnostic and never returns a
partially formatted value. The malformed corpus covers missing expressions,
unexpected characters, unterminated strings, malformed struct tags, missing
backing values, and invalid guarded defaults. Diagnostics remain source-located
by the parser/lexer contract; formatter code must not replace them with an
`<unknown>` rendering or silently recover.
