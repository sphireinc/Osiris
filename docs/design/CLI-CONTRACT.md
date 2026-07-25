# Osiris CLI contract

## Scope and stability

This document defines the v1 process contract for the `osiris` executable.
Command names, documented aliases, flags, exit categories, and JSON diagnostic
keys are stable within v1. New commands and additive JSON fields are allowed;
removing or changing an existing field requires a new schema version and
release notes.

## Streams

- Successful command results go to standard output.
- Diagnostics go to standard error.
- `--diagnostics=json` changes only failure rendering; it never changes a
  successful command payload and is accepted only before the command name.
- The CLI emits no terminal-control or colour sequences. Redirected and
  interactive output therefore have identical text/JSON content.

## Exit codes

| Code | Category | Meaning |
| ---: | --- | --- |
| 0 | success | The command completed. A bare `osiris` prints usage and succeeds. |
| 64 | usage | Unknown command, invalid flag, missing required argument, or incompatible flags. |
| 65 | compile | Parsing, analysis, formatting, or native-lowering failure. |
| 66 | input | A requested source or input path could not be read. |
| 70 | runtime | The interpreter, VM, debugger, trace, REPL, or evaluated program failed while executing. |
| 71 | internal | A failure that does not fit a public command category. |

## JSON diagnostic envelope

```json
{
  "schema_version": 1,
  "command": "run",
  "category": "compile",
  "exit_code": 65,
  "message": "error[...]"
}
```

`schema_version` is required. `command` may be omitted before command parsing
has succeeded. Consumers must ignore unknown additive fields and branch on the
schema version before treating changed fields as compatible.

## Command inventory

| Command | Stable flags / alias | Result |
| --- | --- | --- |
| `run`, `vm` | `-f file` | Execute source through the interpreter or VM. |
| `eval` | — | Evaluate one expression. |
| `demo`, `repl`, `help` | `-h`, `--help` aliases for help | Built-in demo, interactive REPL, or command help. |
| `fmt` | `-w` | Format source or rewrite a file. |
| `opt` | `-w`, `-report`, `-json` | Emit optimized source or optimizer report. Its `-json` is a successful-result schema, not a diagnostic envelope. |
| `ir`, `ssa`, `cfg`, `dom`, `loop` | — | Compiler analysis renderings. |
| `infer`, `alias`, `pure`, `closure`, `viz`, `map` | `viz -imports` | Analysis and debugging renderings. |
| `c`, `obj`, `build` | `-o`, `-cache-dir`, `-linalg-provider`, repeatable `-linalg-provider-flag` | C source, object, or executable lowering. |
| `compile` | Same as `build` | Stable alias for `build`. |
| `wasm` | `-binary`, `-o` | WebAssembly text or binary artifact. |
| `gpu` | `-json`, `-o` | GPU kernel source or plan. |
| `deps`, `heap`, `trace`, `debug` | Their documented `-json`, query, or breakpoint flags | Introspection and debugging data. |
| `lsp` | — | Language Server Protocol over standard I/O. |

Use `osiris help <command>` for the command synopsis. Command-owned JSON
outputs (such as `opt -json`, `gpu -json`, or `deps -json`) retain their own
versioning contracts and are never wrapped in a diagnostic envelope on
success.
