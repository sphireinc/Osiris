# AGENTS.md

## Project summary

Osiris is a deterministic language implementation written in Go. The repository
contains the lexer/parser, formatter, tree-walking interpreter, bytecode VM,
module system, structured errors, scientific/numeric types, analysis and
optimization passes, a native C backend, WebAssembly/GPU artifact emitters,
LSP support, task/scheduler primitives, CLI, examples, benchmarks, and two
public-documentation implementations.

The primary production direction is an ahead-of-time compiler that lowers
Osiris IR to portable C and invokes a system C compiler. The native backend
must reject unsupported programs at compile time; it must never evaluate a
program during compilation to manufacture output.

## Current repository state

- The worktree may contain intentional uncommitted native-backend and
  documentation work. Inspect `git status --short` before editing and preserve
  unrelated changes.
- `LEFTOFF.md` is the current native-lowering handoff and should be read before
  native compiler work.
- `tasks/PRIORITY.md` describes the historical backlog. New work should follow
  the scoped task file supplied by the user rather than attempting an entire
  roadmap at once.
- `go.mod` currently requests Go 1.26. Verify that the selected toolchain is
  actually available in the target environment before changing code or CI.
- `newdocs/` is the canonical Vite/React public language reference and the
  GitHub Pages publishing source. `docs/` is a preserved legacy static manual
  and compatibility-redirect source; do not add public-reference content there
  or maintain two competing sources of truth.

## Repository map

- `cmd/osiris/`: CLI commands, flags, output, and exit behavior.
- `internal/osiris/lexer.go`, `parser.go`, `format.go`: syntax pipeline.
- `internal/osiris/runtime.go`, `session.go`, `value.go`: interpreter and value model.
- `internal/osiris/bytecode.go`: bytecode compiler and VM.
- `internal/osiris/module.go`, `deps.go`, `registry.go`, `semver.go`: modules and dependencies.
- `internal/osiris/ir.go`, `ssa.go`, `cfg.go`, `dom.go`, `loop.go`,
  `infer.go`, `alias.go`, `purity.go`, `closure.go`, `optimize.go`: compiler
  analyses and optimization.
- `internal/osiris/cbackend.go`, `cbackend_native.go`: native compilation and C ABI/lowering.
- `internal/osiris/wasm.go`, `gpu.go`: artifact backends.
- `internal/osiris/error_model.go`, `result_option.go`: fallibility model.
- `internal/osiris/task.go`, `scheduler.go`, `stream.go`: concurrency primitives.
- `internal/osiris/lsp.go`, `sourcemap.go`, `trace.go`, `viz.go`: developer tooling.
- `internal/osiris/*_test.go`: tests; `runtime_test.go` and
  `language_suite_test.go` carry broad behavioral coverage.
- `docs/design/`: durable architecture and ABI contracts.
- `docs/`: preserved legacy static manual and compatibility assets.
- `newdocs/`: canonical componentized public documentation application.
- `examples/`, `benchmarks/`: executable examples and benchmark workloads.

## Required workflow

1. Read the assigned task file completely, then inspect `git status --short`,
   `LEFTOFF.md`, and the relevant design documents.
2. Identify the smallest coherent behavior change. Do not opportunistically
   redesign adjacent subsystems.
3. Add or update a focused failing test before implementation whenever
   practical.
4. Implement across every required execution path. Syntax or value-model
   changes usually require lexer/parser/formatter/interpreter/VM/native/docs
   work; do not stop after one path passes.
5. Run the focused tests first, then the full validation commands available in
   the current toolchain.
6. Inspect generated IR/C/WASM or CLI output when the task changes a compiler
   or user-facing contract.
7. Finish with `gofmt`, `git diff --check`, and `git status --short`.
8. Update the assigned task file with evidence, decisions, and any deliberately
   deferred work. Do not mark acceptance criteria complete without proof.

## Commands

Run from the repository root:

```sh
go test ./...
go test ./internal/osiris -run 'TestName$' -count=1
go test -race ./...
go vet ./...
go run ./cmd/osiris help
go run ./cmd/osiris run examples/named-values.osr
go run ./cmd/osiris vm examples/named-values.osr
go run ./cmd/osiris ir examples/named-values.osr
go run ./cmd/osiris infer examples/named-values.osr
go run ./cmd/osiris c examples/named-values.osr
./build.sh
```

Use `bench.sh` only for deliberate performance work. Record OS, architecture,
Go version, C compiler/version, workload, iteration count, and raw samples for
performance claims.

## Language and compiler invariants

1. Preserve determinism: map traversal, diagnostics, generated artifacts,
   dependency output, and analysis reports must have stable ordering.
2. Preserve interpreter/VM/native parity for supported programs, including
   values, output order, errors, numeric width/style, and source locations.
3. `Value` is the language boundary. Never erase kind, width, style, shape,
   ownership, or metadata through arithmetic, codecs, errors, tasks, or FFI.
4. Syntax changes must include source-span diagnostics and parse-format-parse
   stability tests.
5. Optimizations require semantic-equivalence tests and must be conservative
   when aliasing, effects, overflow, shape, or module identity is uncertain.
6. Native lowering must fail clearly for unsupported IR. Never reintroduce
   compile-time execution as a fallback.
7. C ABI ownership must be explicit. Every returned aggregate is either owned,
   borrowed, or retained by a documented contract; test aliasing and cleanup.
8. Integer power, overflow, decimal, fixed-width, bigint, complex, tensor, and
   unit-aware semantics must match the interpreter exactly before lowering to
   a host operation.
9. Structured errors retain kind, message, cause, stack, source, metadata, and
   redaction behavior across VM, task, native, and serialization boundaries.
10. Cancellation is cooperative and context-based. Scheduler changes require
    deterministic-mode coverage plus race/stress tests.
11. Platform polling and OS integration belong behind build-tagged files and a
    portable interface.
12. Do not hand-edit `.osiris.lock` or `.osiris/cache/`; they are generated
    module artifacts.

## Testing expectations

- Parser/formatter: lexer tests, invalid syntax spans, parse-format-parse.
- Runtime values: focused unit tests plus interpreter/VM parity.
- Native backend: generated-C assertions, compile diagnostics, executable
  parity, ownership/aliasing cases, and sanitizer runs where available.
- Optimizer: before/after IR assertions and semantic equivalence.
- Scheduler/task work: deterministic tests, cancellation tests, `-race`, and
  bounded stress tests without sleeps where synchronization can be explicit.
- LSP: protocol fixtures with stable locations/ranges and malformed-input tests.
- CLI: exit code, stdout/stderr separation, machine-readable mode, and help text.
- Docs: examples must execute or be checked by an automated documentation test.

## Documentation and generated files

- `README.txt` and `LANG.txt` are intentional fixed-width plain-text documents.
- Durable design decisions belong in `docs/design/`; implementation handoffs
  belong in `LEFTOFF.md`; scoped work belongs in `tasks/`.
- Add public language-reference content, examples, and API contracts to
  `newdocs/`; retain `docs/` only for legacy compatibility and durable
  non-public design material.
- Do not commit `newdocs/node_modules/`, local caches, benchmark scratch files,
  platform binaries, or generated site output unless the repository explicitly
  adopts those artifacts.
- Update help text, reference docs, and examples in the same change as a
  user-visible language or CLI contract.

## Scope and review discipline

- One task file should normally produce one reviewable pull request.
- Do not combine formatting sweeps, renames, generated files, and semantic
  changes unless the task specifically requires them.
- Preserve compatibility unless a task explicitly authorizes a breaking change
  and includes migration notes.
- Never claim a feature is supported based only on interpreter behavior when
  the public contract promises VM or native support.
