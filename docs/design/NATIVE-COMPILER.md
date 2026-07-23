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
cache for build integrations. Its key includes the generated C SHA-256,
resolved C compiler path and version output, the exact compiler flags, and the
artifact target. Each entry has a JSON manifest beside the executable. The
cache directory is supplied by the caller so cache lifetime and location do not
affect ordinary builds.

The CLI exposes this through `osiris build -cache-dir <directory> -o <output>
<source>`. Without `-cache-dir`, builds compile directly as before.

## Non-goals

- Do not preserve the snapshot generator as the meaning of `osiris build`.
- Do not introduce a second language or require users to write C.
- Do not claim native coverage for features that still require the host runtime.
- Do not optimize broadly before a correct, measurable executable subset exists.
