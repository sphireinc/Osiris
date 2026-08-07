# WASM execution contract

Osiris WebAssembly artifacts are executable, versioned modules. `CompileWASM`
lowers validated IR and never runs the interpreter to manufacture a result.
`ExecuteWASM` first compiles, instantiates, and invokes the generated module
with the standards-compliant `wazero` engine. It then decodes the Osiris ABI
payload through the deterministic provider and returns owned
`WASMExecution` results. Engine validation is therefore part of execution;
the custom ABI payload is not accepted as a substitute for a valid executable
WASM module.

## Public symbols

- `CompileWASM(ctx, runtime, program)` uses
  `DefaultWASMExecutionLimits`.
- `CompileWASMWithLimits(ctx, runtime, program, limits)` applies explicit
  instruction, output, memory, and artifact bounds.
- `ExecuteWASM(ctx, artifact, limits)` validates the binary and executes its
  generated WASM body. Scalar `scalar-i64-v1` artifacts receive their result
  from exported WASM result globals; `payload-v1` artifacts use the bounded
  ABI runner for aggregate/library forms that belong to follow-up lowering
  tasks.
- `NewDeterministicWASMProvider()` returns the bounded ABI provider used by
  tests and hosts that require reproducible execution. It is reached only
  after the generated binary has passed real engine compilation,
  instantiation, and `run` invocation.

## ABI and ownership

The module exports `memory`, `result_ptr`, `result_len`, `result_kind`,
`result_value`, `result_f64`, `instruction_count`, `instruction_budget`, and
`run`. The custom
`osiris.abi.v1` section contains the source name, lowered function/block
metadata, instruction payload, and result encoding declaration. Results use
`osiris-json-v1`; the returned Go values are deep-cloned before leaving the
provider. The provider does not retain caller-owned `Program`, artifact, or
result buffers.

`execution_mode` is `scalar-i64-v1` when the generated body owns scalar
constant, copy, mutable assignment, unary, binary, comparison, f64 arithmetic,
acyclic and cyclic branch/join control flow, and single-output execution. The
scalar locals are i64-backed; f64 operands are reinterpreted around native WASM
f64 operations. A multi-block SSA function is represented by a bounded
dispatcher loop whose state local selects a lowered block and whose edge copies
assign block parameters. This supports ordinary `if`/join graphs and `while`
loops without falling back to the payload runner.

In `scalar-i64-v1`, `run() -> i32` returns zero on success, `result_kind` is
`1` for unsigned integer, `2` for signed integer, `3` for boolean, `4` for
f64, `5` for f32, or `6` for null. `result_value` contains integer/boolean
bits, and `result_f64` contains the floating-point result (rounded to f32 when
kind `5` is used). `instruction_count` records the number of lowered
operations executed, while `instruction_budget` is decremented by the module
and causes `run` to return status `1` when exhausted; status `0` means success.
The result and accounting globals are mutable module state and are read only
after `run` returns. Aggregates, closures, host providers, and otherwise
unsupported forms use `payload-v1` until their dedicated lowering tasks
replace that compatibility path; they are still validated and bounded, but
are not described as instruction-level WASM execution.

`WASMCapabilityProfile` records the named profile, sorted module list, and
sorted capability list in the artifact. The default `deterministic-v1` profile
uses `NewTestRuntime`, whose filesystem, environment, identity, clock,
database, telemetry, and testing providers are in-memory or virtual. A WASM
execution therefore cannot read the developer's machine merely because source
imports `std/os`; the requested operation must also be present in the profile.

Unsupported IR fails before binary emission with a source-spanned diagnostic.
The scalar instruction-level ABI currently supports bounded i64 unsigned and
signed values, f64/f32-width values, booleans, null, constants, copies,
mutable assignments, unary operations, binary arithmetic, comparisons, cyclic
and acyclic branch/join graphs, generated instruction budgets, and one scalar
output. The payload compatibility ABI covers the
broader typed-value, control-flow, closure, aggregate, pattern, result/option,
and declared standard-library slices while dedicated instruction/import
lowering is completed. The parity corpus covers text, math,
crypto, signal, list, time, regex, JSON, i18n, embedded DB, telemetry,
testing, and fake OS-file access. Network, ambient identity, unsafe, maps and
sets, product-type declarations, future reserved families, and unlisted
capabilities remain source-spanned compile-time boundaries; they are never
silently interpreted by another backend.

## Limits and cancellation

`MaxInstructions`, `MaxMemoryBytes`, `MaxOutputs`, and `MaxArtifactBytes` are
deterministic limits. Every instruction step checks the context, and malformed
headers, section ordering, duplicate ABI sections, invalid LEB128, truncated
payloads, and limit violations produce errors before execution continues.

Repeated executions of one artifact under the same inputs and limits have the
same JSON result, instruction count, and memory accounting.
