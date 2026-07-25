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
ERROR MODEL
---------------------------------------------------------------------------

Osiris uses structured error values.

  * recoverable failures are values, not hidden strings
  * errors carry a stable kind/code and a human-readable message
  * errors may wrap causes, source locations, stack frames, and metadata
  * errors are immutable records, so they can be copied and passed around
  * errors can be inspected with `errmeta(err)` and redacted with `redact(err)`
  * `wrap`, `cause`, `unwrap`, `withmeta`, and `withdebug` add context
  * `try { ... } catch err { ... }` handles recoverable failures explicitly
  * the postfix `?` operator propagates a failure out of the current function
  * `matches(err, kind)` and `is(err, kind)` check wrapped error kinds
  * errors serialize cleanly and stay comparable by structure
  * task boundaries and native boundaries normalize host errors into Osiris
    error records before they escape the runtime

The standard taxonomy intentionally stays small and memorable:

  * InvalidArgument
  * NotFound
  * AlreadyExists
  * PermissionDenied
  * Unauthenticated
  * Unavailable
  * Timeout
  * Cancelled
  * Conflict
  * ResourceExhausted
  * OutOfMemory
  * OutOfRange
  * Unsupported
  * NotImplemented
  * ParseError
  * TypeError
  * EncodingError
  * IOError
  * NetworkError
  * DatabaseError
  * InternalError
  * UnknownError

Osiris also supports standard result and option values:

  * `ok(value)` and `err(error)` for Result values
  * `some(value)` and `none()` for Option values
  * `unwrap(value)` to extract a payload
  * `expect(value, message)` to turn emptiness into a structured failure
  * `or_else(value, fallback)` to supply a default
  * `isok`, `iserr`, `issome`, and `isnone` for inspection

The postfix `?` operator is the short propagation form. It keeps the happy
path short by returning a recoverable failure to the current function
boundary immediately.

The exception model is intentionally small and explicit:

  * recoverable failures stay as structured error values
  * fatal faults use `panic`, `trap`, and `assert`
  * there are no language-level `finally` or `defer` blocks
  * there are no language-level destructors or finalizers
  * `try/catch` handles ordinary failures and does not catch fatal faults
  * structured errors keep their kind, message, cause, stack, and metadata
    as they cross function, task, and native boundaries

Use `panic`, `trap`, and `assert` for fatal states and invariants that should
not be recovered from by ordinary `try/catch` handling. Use Result and Option
for ordinary fallibility and ordinary absence.

Wrapping helpers like `wrap`, `cause`, `unwrap`, `withmeta`, `withdebug`,
`errmeta`, and `redact` preserve cause chains while letting you attach domain
context and control what is safe to expose.

---------------------------------------------------------------------------
WHAT CAN BE DONE WITH IT
---------------------------------------------------------------------------

With Osiris you can:

  * write scripts that turn inputs into stable outputs
  * work with files, text, JSON, CSV, lists, hashes, and bytes
  * use imports, modules, exports, macros, and pattern/switch matching
  * define recursive functions and use loops for controlled iteration
  * catch runtime failures with `try { ... } catch err { ... }`
  * fan out independent work through a multi-core task scheduler
  * run code through the interpreter or a bytecode VM
  * inspect source maps, traces, heap snapshots, dependency graphs, and live scopes
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
  4. Inspect it with `osiris trace`, `osiris debug`, `osiris heap`,
     `osiris map`, or `osiris deps`.
  5. Use `osiris lsp` if you want editor integration.
  6. Use `osiris ir` to inspect the lowered IR.
  7. Open the GitHub Pages React manual built from `newdocs/` for the
     long-form language reference, function index, and editorial guide.
  7. Use `osiris ssa` to inspect the SSA-verified IR and capture flow.
  8. Use `osiris cfg` to inspect the control-flow graph.
  9. Use `osiris dom` to inspect the dominator tree.
 10. Use `osiris loop` to inspect canonical loop shapes.
 11. Use `osiris alias` to inspect alias classes, fresh values, and escapes.
 12. Use `osiris infer` to inspect inferred kinds, shapes, and escapes.
 13. Use `osiris vm` when you want the alternate bytecode backend.
 14. Check the Backend support material in the React manual before choosing
     any non-interpreter backend; native C, WebAssembly, GPU, and LSP
     coverage are feature-specific.

There is also a REPL for quick experiments.
For quick cross-language timing, run `bench.sh` from the repo root. It builds
the Osiris binary, optimizes `examples/physics.osr` and `examples/tensor.osr`,
then times the major backends side by side.

The standard native build path is `osiris build`, which lowers Osiris through
the C backend and then compiles that output with `cc -O3 -march=native -flto
-DNDEBUG -lm`. The benchmark script also measures the other surfaces so you
can compare them directly:
  * interpreter
  * bytecode VM
  * standard native build
  * raw C emission
  * object-file emission
  * WebAssembly text and binary emission
  * GPU kernel source and kernel-plan emission
  * generated C compiled to native
  * handwritten C compiled to native

The tensor workload is there to stress the shape-aware fast path while the
physics workload gives you a scalar baseline.
The script writes all generated artifacts into a temporary scratch directory
and removes them on exit, so it does not leave benchmark files behind by
default.
For a smaller scientific language sample, run `go run ./cmd/osiris run
examples/named-values.osr`.

---------------------------------------------------------------------------
COMMANDS
---------------------------------------------------------------------------

  osiris run path/to/program.osr
  osiris vm path/to/program.osr
  osiris eval '1 + 2 * 3'
  osiris demo
  osiris fmt -w path/to/program.osr
  osiris ir path/to/program.osr
  osiris ssa path/to/program.osr
  osiris cfg path/to/program.osr
  osiris dom path/to/program.osr
  osiris loop path/to/program.osr
  osiris c [-o out.c] path/to/program.osr
  osiris obj -o out.o path/to/program.osr
  osiris build -o out path/to/program.osr
  osiris compile -o out path/to/program.osr
  osiris alias path/to/program.osr
  osiris pure path/to/program.osr
  osiris closure path/to/program.osr
  osiris infer path/to/program.osr
  osiris viz [-imports] path/to/program.osr
  osiris wasm [-binary] [-o out.wat|out.wasm] path/to/program.osr
  osiris lsp
  osiris map path/to/program.osr
  osiris deps [-json] path/to/program.osr
  osiris heap [-json] [-query expr] path/to/program.osr
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

  package name 1.2.3
  export a, b, c
  import "./local.osr" as local
  let name = expression
  macro name(arg) => expression
  emit expression
  emit left, right

Expressions:

  * arithmetic: + - * / %
  * comparisons: == != < <= > >=
  * boolean logic: && || !
  * bitwise ops: & | ^ ~ << >>
  * exact decimals: 0.1d + 0.2d, dec("1.25")
  * complex numbers: 5 + 3i, complex(5, 3), abs(3 + 4i)
  * signed integers: 42i16, i32(42), -5i64
  * unsigned integers: 42u, uint(42), 255u64, 1u128, 7u256
  * byte values: 255b, byte(255), pack([byte(65), byte(66)])
  * calls: foo(x, y)
  * member access: text.slug(...)
  * lists: [1, 2, 3]
  * blocks: { let x = 1; x + 2 }
  * error handling: try { risky() } catch err { fallback(err) }
  * pattern matching:

      emit match [1, 2] {
        [a, b] => a + b;
        _ => 0
      }

Numbers may be plain, hex, binary, or fixed-width values like 1u8, 2u64,
3u128, 1.5f32, 2.25f64, or 3i128.

Tensors are flat buffers with shape metadata:

  tensor([[1, 2], [3, 4]])
  matrix(2, 2, [1, 2, 3, 4])
  shape(value)
  flatten(value)
  transpose(matrix)
  dot(a, b)
  matmul(left, right)
  reshape(value, [rows, cols])
  view(tensor)
  clone(tensor)

Tensor element-wise arithmetic is lowered into parallel chunk work when the
operands are tensor-shaped. That is the SIMD-friendly path in this codebase.
Compatible shapes broadcast across axes, so `matrix(2, 1, [...]) + matrix(1,
2, [...])` is valid and runs as a single shaped operation. The compiler also
rejects obvious constant matrix-shape mismatches during assembly.

Strings are their own primitive and do not quietly become numbers.
Use explicit numeric literals or casts when you want numeric behavior.
Use `f32(...)` or `f64(...)` when you want an explicit float width instead of
the default `number` lane.

---------------------------------------------------------------------------
MODULES AND VERSIONS
---------------------------------------------------------------------------

File modules use a manifest-style header:

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

  u8, u16, u32, u64, u128, u256, u512, u1024
  band, bor, bxor, bnot, shl, shr, rotl, rotr
  hex, bin, bytes, pack, unpack, encode, decode

Matrix and tensor:

  tensor, matrix, shape, flatten, transpose, dot, matmul, reshape, view, clone

Math and vectors:

  sin, cos, tan, asin, acos, atan, exp, log, log10, log2, hypot, norm
  vector, vec, bigint, bigfloat

Units:

  Meter, Second, Kilogram, Ampere, Kelvin, Mole, Candela
  meter, second, kilogram, ampere, kelvin, mole, candela
  m, s, kg, A, K, mol, cd
  Newton, Joule, Watt, Pascal, Coulomb, Volt, Farad
  Ohm, Siemens, Weber, Tesla, Henry, Lumen, Lux
  Becquerel, Gray, Sievert, Katal, Liter, Minute, Hour, Day, Week
  kilo, mega, giga, tera, milli, micro, nano, pico
  Pi, Tau, E, SpeedOfLight, Planck, Boltzmann, Avogadro
  10 * Meter / (2 * Second)
  2 * kilo * Meter
  30 * minute
  2 * Pi

These are ordinary identifiers, so they behave like any other value.
You can store them, pass them into functions, and combine them with normal
arithmetic.

GPU target:

  osiris gpu [file]

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
  * willing to use functions and loops when the shape needs it
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
`emit` also accepts comma-separated expressions and concatenates them with `, `
for quick status lines like `emit "Solved:", solved`.

That is the point.
