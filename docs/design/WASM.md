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
  from generated memory and result globals; aggregate `aggregate-record-v1`
  artifacts receive codec records from generated linear memory; `payload-v1`
  remains the bounded compatibility path for dynamic forms that belong to
  follow-up lowering tasks.
- `NewDeterministicWASMProvider()` returns the bounded ABI provider used by
  tests and hosts that require reproducible execution. It is reached only
  after the generated binary has passed real engine compilation,
  instantiation, and `run` invocation.
- `WASMHostImport` describes one logical standard-library import in the
  artifact manifest. Its `Module` is the imported module name, its `Member`
  is the exact exported operation, its `Capability` is the exact profile
  grant required by that operation (or the empty string for a pure operation),
  and its `Encoding` must be `WASMHostImportEncoding`.
- `WASMHostImportEncoding` is `osiris-host-import-v1`. It identifies the
  bounded Osiris-value exchange used by the current logical import contract;
  it is not a raw native-pointer or ambient browser/Node ABI.

## ABI and ownership

The module exports `memory`, `result_ptr`, `result_len`, `result_kind`,
`result_value`, `result_f64`, `result_bits`, `instruction_count`,
`instruction_budget`, and
`run`. The custom
`osiris.abi.v1` section contains the source name, lowered function/block
metadata, instruction payload, result encoding declaration, and the sorted
`WASMABI.Imports` manifest. Results use
`osiris-json-v1`; the returned Go values are deep-cloned before leaving the
provider. The provider does not retain caller-owned `Program`, artifact, or
result buffers.

### Host-import manifest

`WASMABI.Imports` contains one record for each lowered module member reference.
The manifest is deterministic: records are unique and sorted by `Module`, then
`Member`. A record is emitted only after the lowering profile admits the
module/member pair. The capability is derived from the same admission table;
the compiler never accepts a caller-supplied capability string as authority.

For example, a source reference to `std/os.getenv` produces this exact logical
record:

```json
{
  "module": "std/os",
  "member": "getenv",
  "capability": "os.environment",
  "encoding": "osiris-host-import-v1"
}
```

`std/list.sum` produces the same record shape with an empty `capability`,
because it is deterministic and does not require a host grant. Module-only
imports do not create a member record until a member is referenced. This keeps
the manifest granular and prevents an unused module import from implying
ambient access.

`validateWASMProgramProfile` validates the manifest at artifact ingestion. It
rejects empty fields, unsupported encodings, duplicate or unsorted records,
modules outside the profile, members outside the profile, capability strings
that do not match the admission table, and manifests whose records do not
exactly match the lowered IR references. These checks run before the generated
module is instantiated and therefore apply equally to normal and concurrent
executions.

This bounded slice is an ABI declaration and validation layer. In ABI v1 the
logical host operation still resolves through the deterministic Osiris runtime
and bounded value codec; the generated module does not yet declare a raw WASM
engine import for every standard-library operation. Instruction-level host
imports remain open for
instruction-level host import calls, per-import result/error records,
cancellation handoff, and provider-owned resource cleanup.

`execution_mode` is `scalar-i64-v1` when the generated body owns scalar
constant, copy, mutable assignment, unary, binary, comparison, f64/f32
arithmetic, acyclic and cyclic branch/join control flow, and single-output
execution. Scalar locals are i64-backed. Unsigned values retain their declared
width through masks; signed i8, i16, and i32 values use native i64 sign
extension after each operation. f64 operands are reinterpreted around native
WASM f64 operations. When both operands are f32, lowering uses native f32
demotion, arithmetic/comparison, and promotion, preserving intermediate f32
rounding. A multi-block SSA function is represented by a bounded dispatcher
loop whose state local selects a lowered block and whose edge copies assign
block parameters. This supports ordinary `if`/join graphs and `while` loops
without falling back to the payload runner.

`execution_mode` is `aggregate-record-v1` for deterministic aggregate and
direct-function forms whose inputs are compile-time values. The module owns an
`OSMR` record in linear memory: magic `OSMR`, little-endian output count, then
`(length:u32, OSR1-codec-value)` entries. Lists, tuples, enums, tensors,
`Result`, `Option`, strings, and exact numeric values use the repository's
bounded `OSR1` codec. Straight-line direct functions with no captures and
constant arguments are specialized during lowering and their result is still
read from generated WASM memory. Dynamic calls, captures, and control-flow
aggregates remain outside this mode.

In `scalar-i64-v1`, `run() -> i32` returns zero on success, `result_kind` is
`1` for unsigned integer, `2` for signed integer, `3` for boolean, `4` for
f64, `5` for f32, or `6` for null. `result_value` contains integer/boolean
bits, and `result_f64` contains the floating-point result (rounded to f32 when
kind `5` is used). `result_bits` carries the integer width, and the same
kind/width/payload is written to the first two bytes and byte 8 of the owned
16-byte record at `result_ptr`; `result_len` is 16. `instruction_count` records
the number of lowered operations executed, while `instruction_budget` is
checked before each operation and decremented by the module. `run` returns
status `1` when the next operation would exceed the budget and status `0` on
success. Engine traps are converted into source-named diagnostics for integer
division by zero, signed overflow, unreachable, and linear-memory bounds
failures. wazero owns cancellation with close-on-context-done and returns the
originating context error.

Aggregate mode uses result kind `7`, width `0`, and a variable `result_len`.
The provider decodes only the bytes returned by module memory and enforces
output and memory limits before returning owned values. Dynamic closures, host
providers, and otherwise unsupported forms use `payload-v1` until their
dedicated lowering tasks replace that compatibility path; they are still
validated and bounded, but are not described as instruction-level WASM
execution.

`WASMCapabilityProfile` records the named profile, sorted module list, and
sorted capability list in the artifact. The default `deterministic-v1` profile
uses `NewTestRuntime`, whose filesystem, environment, identity, clock,
database, telemetry, and testing providers are in-memory or virtual. A WASM
execution therefore cannot read the developer's machine merely because source
imports `std/os`; the requested operation must also be present in the profile.

Unsupported IR fails before binary emission with a source-spanned diagnostic.
The scalar instruction-level ABI currently supports bounded i64-backed unsigned
and signed values with declared widths up to 64 bits, f64/f32 values,
booleans, null, constants, copies, mutable assignments, unary operations,
binary arithmetic, comparisons, cyclic and acyclic branch/join graphs, exact
instruction budgets, source-named traps, cancellation, and one scalar output.
The generated memory-record ABI covers deterministic aggregates, exact codec
values, and direct constant functions. The payload compatibility ABI covers
the broader dynamic typed-value, control-flow, closure, pattern, result/option,
and declared standard-library slices while dedicated lowering is completed.
The parity corpus covers text, math,
crypto, signal, list, time, regex, JSON, i18n, embedded DB, telemetry,
testing, and fake OS-file access. Network, ambient identity, unsafe, maps and
sets, product-type declarations, future reserved families, and unlisted
capabilities remain source-spanned compile-time boundaries; they are never
silently interpreted by another backend.

## Limits and cancellation

`MaxInstructions`, `MaxMemoryBytes`, `MaxOutputs`, and `MaxArtifactBytes` are
deterministic limits. The generated instruction step enforces the instruction
budget; wazero closes execution when the context is canceled. Malformed
headers, section ordering, duplicate ABI sections, invalid LEB128, truncated
payloads, and limit violations produce errors before execution continues.

Repeated executions of one artifact under the same inputs and limits have the
same JSON result, instruction count, and memory accounting.
