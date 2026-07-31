<p align="center">
  <img src="assets/osiris_logo_textonly.png" alt="Osiris logo" width="430">
</p>

<h1 align="center">Osiris</h1>

<p align="center">A deterministic language for readable data work, numerical programs, and native compilation.</p>

<p align="center">
  <a href="https://sphireinc.github.io/Osiris/">Read the documentation</a>
  ·
  <a href="https://github.com/sphireinc/Osiris">View the source</a>
</p>

Osiris is a Go-native programming language and toolchain. Its runtime and
compiler are designed around explicit data flow, structured errors, predictable
results, and artifacts that can be inspected before they run.

The project is still evolving and is being prepared for a public FOSS release.
The hosted reference is the best place to see the language that is currently
implemented and the backend support available today:

**[Open the Osiris public documentation](https://sphireinc.github.io/Osiris/)**

## What is here

- A lexer, parser, formatter, interpreter, and bytecode VM.
- Deterministic modules, imports, structured errors, tasks, and tracing tools.
- Numerical, text, byte, tensor, unit-aware, and standard-library values.
- An ahead-of-time native C backend that rejects unsupported programs clearly.
- WebAssembly and GPU artifact emitters, plus formatter and LSP tooling.
- Executable examples, benchmarks, and conformance tests.

Support is feature-specific across the interpreter, VM, native C, WebAssembly,
GPU, formatter, and LSP. Check the [backend support matrix](https://sphireinc.github.io/Osiris/#support)
before choosing a target.

## A small example

```osiris
let greeting = "hello, Osiris"
let length = len(greeting)
emit greeting, length
```

The [language reference](https://sphireinc.github.io/Osiris/#language) covers
the syntax, values, control flow, errors, modules, and standard library. The
[technical reference](https://sphireinc.github.io/Osiris/#technical) records
the detailed grammar and implementation contracts.

## Try it from a checkout

Go 1.26 or newer is required by the current module. From the repository root:

```sh
go run ./cmd/osiris run examples/named-values.osr
go run ./cmd/osiris vm examples/named-values.osr
go run ./cmd/osiris fmt examples/named-values.osr
go run ./cmd/osiris build -o program examples/named-values.osr
```

The command-line reference is available under [Tooling](https://sphireinc.github.io/Osiris/#tooling).
For a local documentation build:

```sh
cd docs
npm ci
npm run check-reference
npm run check-examples
npm run build
```

## Learn more

- [Public language documentation](https://sphireinc.github.io/Osiris/)
- [Syntax and semantics](https://sphireinc.github.io/Osiris/#language)
- [Types and constants](https://sphireinc.github.io/Osiris/#types)
- [Standard library](https://sphireinc.github.io/Osiris/#text)
- [Native and other backend support](https://sphireinc.github.io/Osiris/#support)
- [Technical reference](https://sphireinc.github.io/Osiris/#technical)

## Contributing

Issues, examples, documentation improvements, tests, and focused compiler work
are welcome. Start with the hosted documentation, run the focused checks for
the area you change, and include a clear description of behavior and test
coverage in your pull request.

The project is preparing its public contribution and licensing policy. Until
that policy is published, please open an issue to discuss substantial changes
before investing in a large pull request.

<p align="center">
  <img src="assets/logo_black_bg.png" alt="Osiris logo on black" width="120">
</p>
