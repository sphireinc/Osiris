# Native compiler direction

## Decision

Osiris will become a general ahead-of-time compiler. Its first production
backend will lower Osiris AST/IR into portable C and invoke the system C
compiler. The existing snapshot backend is transitional only.

The target pipeline is:

```text
.osr source -> parser -> typed IR/SSA -> optimization -> C emission
           -> system C compiler -> native executable
```

The executable must execute program logic at runtime. It must not be a C
program that merely prints values computed during compilation.

## Why C first

C is the best first native backend for this codebase: IR/SSA/CFG already exist,
the generated code can be inspected, and mature system compilers provide
register allocation, instruction selection, vectorization, linking, and
platform support. Good generated C with unboxed hot paths can reach C-level
performance for the supported subset.

A direct machine-code backend should not be the first milestone. It would
duplicate mature compiler work and make debugging, portability, ABI work, and
performance validation harder. LLVM or direct code generation can be evaluated
later if C emission proves to be the bottleneck.

## Implementation stages

1. **Executable subset.** Lower constants, bindings, arithmetic, branches,
   loops, functions, and emit from IR to C. Reject unsupported features with
   clear diagnostics; never fall back to compile-time evaluation.
2. **Runtime ABI.** Define ownership, error, string, collection, function, and
   task runtime interfaces. Use unboxed bool, integer, and f32/f64 values in
   hot code; retain boxed values only at explicit dynamic boundaries.
3. **Semantic coverage.** Add calls, closures, modules, lists, strings,
   structured errors, I/O, and task lowering in separately tested increments.
4. **Scientific kernels.** Specialize contiguous tensor operations, matrix
   multiplication, and SIMD/vector-friendly loops. Keep generic paths correct.
5. **Validation.** Require interpreter/VM/native parity tests, generated-C
   snapshots, codegen fuzzing, and benchmark gates before claiming speedups.

## Reproducible executable cache

`CompileCExecutableCached` provides an explicit content-addressed executable
cache for build integrations. Its key includes a cache-format version, the
generated C SHA-256, numerical-provider SHA-256, resolved C compiler path and
version output, exact compiler/linker flags, and native OS/architecture target.
Each entry has a versioned JSON manifest beside the executable. Reuse validates
that manifest exactly; a missing, corrupt, old, or mismatched one is an error,
never an unchecked cache hit. The cache directory is supplied by the caller so
cache lifetime and location do not affect ordinary builds.

The CLI exposes this through `osiris build -cache-dir <directory> -o <output>
<source>`. Without `-cache-dir`, builds compile directly as before.

## Static module identity

Native C does not represent dynamic module values. The lowering analysis treats
an import path as compile-time metadata with a two-point lattice: a value is
either a proven path or unknown. Imports introduce a proven path; copies retain
it; a CFG block parameter retains it only when every incoming edge has the
same proven path. Any missing or conflicting predecessor yields unknown.

The same rule applies to lexical captures. The compiler propagates the proven
identity from each `makefunc` capture site into the nested function and accepts
it only when all construction sites agree. This permits a captured `std/math`
alias to lower directly to native helpers, without turning module values into a
runtime ABI feature.

A member access on an unknown module value is a compile-time diagnostic
(`dynamic module member "name" cannot be lowered`). A known but unsupported
module is also rejected explicitly. Dynamic imports, escaping module values,
and conflicting control-flow paths must never be lowered as `null` values or
indirect native calls.

## Non-goals

- Do not preserve the snapshot generator as the meaning of `osiris build`.
- Do not introduce a second language or require users to write C.
- Do not claim native coverage for features that still require the host runtime.
- Do not optimize broadly before a correct, measurable executable subset exists.
