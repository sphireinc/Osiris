OSIRIS
======

Osiris is a Go-native sigil engine.

It is a tiny deterministic language for forging outputs from text, numbers,
bytes, and structured values.

Its name nods to Osiris, the ancient Egyptian god of the underworld, death,
and resurrection as an homage because the lang is about breaking input down,
rebuilding it deterministically, and bringing useful state back.

It is useful as a language playground, a deterministic scripting tool, and a
base for editor and runtime experiments.

---------------------------------------------------------------------------
WHAT IT IS FOR
---------------------------------------------------------------------------

Osiris is useful when you want a language that is:

  * small enough to read in one sitting
  * strong on determinism
  * good at text and data transforms
  * good at byte and bit work
  * explicit about modules and versions
  * easy to inspect, trace, and debug

It is not trying to be a general purpose application framework.
It is trying to be a tight language surface with serious tooling around it.

---------------------------------------------------------------------------
WHAT CAN BE DONE WITH IT
---------------------------------------------------------------------------

With Osiris you can:

  * write scripts that turn inputs into stable outputs
  * work with files, text, JSON, CSV, lists, hashes, and bytes
  * use imports, modules, exports, macros, and pattern matching
  * run code through an interpreter, a bytecode VM, or WebAssembly
  * inspect source maps, traces, dependency graphs, and live scopes
  * use the LSP server for hover and go-to-definition
  * build reproducible transforms and language experiments in Go

---------------------------------------------------------------------------
WHAT SHOULD BE DONE WITH IT
---------------------------------------------------------------------------

The best uses of Osiris are:

  * deterministic build steps
  * artifact generation
  * content transforms
  * file cleanup jobs
  * byte-level utilities
  * language and tooling research

The language is strongest when the program is written as a visible pipeline:

  input -> named transforms -> output

If a program becomes hard to trace, it is usually written in the wrong shape.

---------------------------------------------------------------------------
HOW IT IS USED
---------------------------------------------------------------------------

The usual flow is:

  1. Write a `.osr` file.
  2. Run it with `osiris run`.
  3. Normalize it with `osiris fmt`.
  4. Inspect it with `osiris trace`, `osiris debug`, `osiris map`, or
     `osiris deps`.
  5. Use `osiris lsp` if you want editor integration.
  6. Use `osiris vm` or `osiris wasm` when you want alternate backends.

There is also a REPL for quick experiments.

---------------------------------------------------------------------------
COMMANDS
---------------------------------------------------------------------------

  osiris run path/to/program.osr
  osiris vm path/to/program.osr
  osiris eval '1 + 2 * 3'
  osiris demo
  osiris fmt -w path/to/program.osr
  osiris viz [-imports] path/to/program.osr
  osiris wasm [-binary] [-o out.wat|out.wasm] path/to/program.osr
  osiris lsp
  osiris map path/to/program.osr
  osiris deps [-json] path/to/program.osr
  osiris trace [-json] path/to/program.osr
  osiris debug [-json] [-break line[:col]] path/to/program.osr
  osiris repl

---------------------------------------------------------------------------
PROGRAM SHAPE
---------------------------------------------------------------------------

The usual order in a file is:

  package
  export
  import
  macro
  let
  emit

That is not a hard rule, but it reads well.

Example:

  package sigil 1.0.0
  export visible

  import "std/text" as text

  let seed = "osiris::demo"
  let visible = text.slug(seed)
  emit visible

If a file is meant to be imported, give it a package name and explicit
exports.

---------------------------------------------------------------------------
LANGUAGE NOTES
---------------------------------------------------------------------------

Statements:

  package name
  package name 1.2.3
  export a, b, c
  import "./local.osr" as local
  let name = expression
  macro name(arg) => expression
  emit expression

Expressions:

  * arithmetic: + - * / %
  * comparisons: == != < <= > >=
  * boolean logic: && || !
  * bitwise ops: & | ^ ~ << >>
  * calls: foo(x, y)
  * member access: text.slug(...)
  * lists: [1, 2, 3]
  * blocks: { let x = 1; x + 2 }
  * pattern matching:

      emit match [1, 2] {
        [a, b] => a + b;
        _ => 0
      }

Numbers may be plain, hex, binary, or fixed-width values like 1u8 or 2u64.

---------------------------------------------------------------------------
MODULES AND VERSIONS
---------------------------------------------------------------------------

File modules use a manifest-style header:

  package name
  package name 2.1.0
  export visible

Imports may include selectors:

  import "std/text@1" as text
  import "./local.osr@2.1.0" as local

Modules are cached through a project-local `.osiris.lock` and a content-
addressed cache under `.osiris/cache/modules`.

If a source file disappears, Osiris can still replay the locked version from
cache.

---------------------------------------------------------------------------
BUILTINS
---------------------------------------------------------------------------

Text and data:

  hash, mix, cat, chunks, join, split, upper, lower, trim, replace, repeat
  len, take, range, sum, product, json, format, slug, noise

Bytes and bits:

  u8, u16, u32, u64
  band, bor, bxor, bnot, shl, shr, rotl, rotr
  hex, bin, bytes, pack, unpack, encode, decode

Pipelines:

  read, readbytes, write, lines, csv, jsonparse

---------------------------------------------------------------------------
WRITING STYLE
---------------------------------------------------------------------------

Good Osiris code is:

  * named
  * deterministic
  * easy to trace
  * small on globals
  * explicit about modules
  * clear about byte work
  * direct about outputs

Try to write programs like this:

  let basis = hash(seed)
  let salt = mix(basis, "scarab", "moon")
  let sigil = join("-", chunks(mix(basis, salt), 4))
  emit sigil

Avoid huge hidden expressions when a few named steps would be clearer.

---------------------------------------------------------------------------
EXAMPLES
---------------------------------------------------------------------------

  package sigil 1.0.0
  export visible

  import "std/text" as text

  let seed = "osiris::demo"
  let basis = hash(seed)
  let salt = mix(basis, "scarab", "moon")
  let visible = text.slug(join("-", chunks(mix(basis, salt), 4)))
  emit visible

  emit { let x = 2; let y = 3; x + y }

  emit match [1, 2] {
    [a, b] => a + b;
    _ => 0
  }

---------------------------------------------------------------------------
FINAL NOTE
---------------------------------------------------------------------------

Osiris is meant to stay small, sharp, and inspectable.

Write it so it still makes sense after you have not looked at it for a while.
Keep the data flow obvious.
Keep the modules tight.
Keep the outputs deterministic.

That is the point.
