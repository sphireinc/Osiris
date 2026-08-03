# Osiris project map

- Purpose: Go-native implementation of a deterministic language for readable data work, numerical programs, structured errors, and inspectable native compilation artifacts.
- Architecture: one Go module; `cmd/osiris` is the CLI boundary and `internal/osiris` contains the language/toolchain. The central flow is source → lexer/parser/AST → formatter and shared analyses (type check/inference/CFG/SSA/optimization) → tree-walking runtime, bytecode VM, or IR/native C backend. WASM/GPU emitters and LSP/tooling are capability-specific consumers.
- Important entry points:
  - CLI: `cmd/osiris/main.go:main`, `execute`, and `run`; subcommands dispatch to `runProgram`, `runProgramVM`, `runIR`, `runInfer`, `runCBackend`, `runWASM`, `runGPU`, and `runLSP`.
  - Syntax: `internal/osiris.Parse`, `FormatSource`.
  - Semantic analysis: `Runtime.CheckProgram`, `Runtime.InferProgram`.
  - Lowering: `Runtime.BuildIR`; optimization/CFG/SSA passes consume the IR.
  - Execution: `Runtime.Run`/evaluation, `Session.ExecSource`/module execution, and bytecode compiler/VM paths.
  - Native: `CompileC`/cached variants and `emitCIR`.
- Architectural constraint: supported interpreter, VM, and native behavior must remain deterministic and semantically aligned; native compilation rejects unsupported IR and never evaluates source as a fallback.
- Detailed repository policy, source map, workflow, and invariants live in `AGENT/AGENTS.md`; do not reproduce that document here.
- Related orientation: `mem:tech_stack`, `mem:suggested_commands`, `mem:conventions`, and `mem:task_completion`.