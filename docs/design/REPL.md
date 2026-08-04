# REPL input contract

The REPL is an interactive front end to the same parser, checker, and
interpreter session used by source files. It is not a second language and does
not have declaration rules of its own.

## Starting and ending a session

Start a session with:

```sh
osiris repl
```

The prompt is `osiris> `. Input is read from standard input one logical
submission at a time. `quit`, `exit`, and `:q` end the session. End of file
also ends the session. Empty lines and a line containing only `;` do not
execute a program and simply request another prompt.

Bindings, declarations, imports, and macros remain available until the
session ends. A later submission therefore sees values installed by earlier
submissions:

```text
osiris> let name = "Osiris"
osiris> emit name
"Osiris"
```

## Statement input and expression convenience

Every complete language statement is submitted exactly as it would appear in
a `.osr` source file. This includes:

- `const`, `let`, and `var` bindings;
- `func`, `struct`, `enum`, `type`, and `newtype` declarations;
- `trait`, `interface`, and `impl` declarations;
- `macro`, `package`, `export`, and `import` declarations;
- `defer`, `emit`, `return`, loop, match, and other supported statements.

The REPL must send those forms through the shared parser and session execution
path. It must never prepend `emit` to a declaration. In particular,
`const limit = 2 + 3` is a declaration, not an expression named `const`.

For exploratory work, a complete input that is an expression may omit `emit`.
The REPL evaluates it as though `emit` had been written:

```text
osiris> 2 + 3
3u64
osiris> emit 4
4u64
```

This convenience applies only when the input is classified as an expression;
it does not change the syntax or semantics of the language.

## Constants and mutable state

`const` is statically evaluated before execution and is immutable. Its
initializer can use literals, earlier constants, and pure operators, but not
runtime calls or mutable state. `let` is an immutable runtime binding and may
depend on calls or other runtime values. `var` creates a mutable local cell;
assignment commits only after its right-hand side succeeds.

```text
osiris> const limit = 2 + 3
osiris> let runtime = limit + 1
osiris> var current = runtime
osiris> current = current + limit
osiris> emit limit, runtime, current
"5u64, 6u64, 11u64"
```

The same sequence in a source file must produce the same values and
diagnostics. REPL persistence changes only the lifetime of the session, not
the meaning of a declaration.

## Multiline and incomplete input

Compound declarations and blocks may span multiple input lines. The REPL
collects lines until the parser recognizes one complete submission. Nothing
from an incomplete declaration is executed or installed:

```text
osiris> func add(value u64) u64 {
....> return value + 1u64
....> }
osiris> emit add(4u64)
5u64
```

The continuation prompt is the deterministic `....> ` prompt, distinct from
`osiris> `. A blank line while collecting a compound form remains part of the
current submission; it must not cause a partial declaration to execute. If
input ends while a submission is incomplete, the REPL returns an
`<repl>: incomplete input at end of input` diagnostic and does not execute or
install that submission. A complete but malformed submission reports its
parser diagnostic and keeps the existing session state unchanged.

## Output ordering, diagnostics, and recovery

Explicit `emit` statements and expression convenience outputs are written in
source order. A successful declaration normally produces no value. A runtime
failure or cancellation does not erase outputs already committed by earlier
statements in the same session.

Diagnostics use the same source-spanned errors as source-file execution. The
REPL reports a failed submission and returns to its prompt, allowing a later
independent submission to continue. A failed binding initializer does not
install the binding, and a failed `var` assignment leaves the previous cell
value unchanged.

## Source-file parity

For a complete input transcript `S`, the observable contract is:

```text
REPL(S) == run(S)
```

where equality covers emitted values, declaration effects visible to later
inputs, diagnostic category/message/source location, and output ordering. The
only REPL-specific behavior is the prompt protocol and the optional implicit
`emit` for a standalone expression. The canonical CLI tests in
`cmd/osiris/repl_test.go` cover declaration input, expression convenience,
mutable state, multiline blocks, deterministic continuation prompts, and
incomplete-input handling. They call the same `runReplWithInput` framing path
used by `runRepl`; semantic execution still goes through `Parse` and
`Session.ExecSource`.
