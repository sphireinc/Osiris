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
  Osiris ABI payload.
- `NewDeterministicWASMProvider()` returns the bounded ABI provider used by
  tests and hosts that require reproducible execution. It is reached only
  after the generated binary has passed real engine compilation,
  instantiation, and `run` invocation.

## ABI and ownership

The module exports `memory`, `result_ptr`, `result_len`, and `run`. The custom
`osiris.abi.v1` section contains the source name, lowered function/block
metadata, instruction payload, and result encoding declaration. Results use
`osiris-json-v1`; the returned Go values are deep-cloned before leaving the
provider. The provider does not retain caller-owned `Program`, artifact, or
result buffers.

`WASMCapabilityProfile` records the named profile, sorted module list, and
sorted capability list in the artifact. The default `deterministic-v1` profile
uses `NewTestRuntime`, whose filesystem, environment, identity, clock,
database, telemetry, and testing providers are in-memory or virtual. A WASM
execution therefore cannot read the developer's machine merely because source
imports `std/os`; the requested operation must also be present in the profile.

Unsupported IR fails before binary emission with a source-spanned diagnostic.
The current ABI supports typed values, exact represented numeric families,
control flow and mutation, direct calls and captured closures, deterministic
list/tuple/tensor operations, enum and list-pattern matching, results/options,
and the declared standard-library slices. The parity corpus covers text, math,
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
