# Native runtime ABI and scalar specialization

Source: OPTCHK.md, TYPECHK.txt, and docs/design/NATIVE-COMPILER.md.

Define a small C runtime ABI before expanding code generation. The initial
contract is documented in docs/design/NATIVE-ABI.md. It must cover
ownership, strings/bytes, errors, functions, output, and explicit dynamic
fallback boundaries. Define native representations and calling conventions for
bool, fixed-width integers, f32, and f64.

Keep generic boxed Value only at dynamic boundaries. C-level performance means
hot scalar and kernel loops do not dispatch through interpreter values.

Acceptance: documented ABI, generated C runtime helpers, scalar passing/return
and overflow tests, and parity tests for conversion into/out of fallback values.

Progress: tagged scalar values, width-preserving f32/f64 casts, UTF-8 string
length and concatenation with process-lifetime arena ownership, immutable list
literals/indexing/iteration, direct user-function ABI, checked unboxed scalar
arithmetic, straight-line scalar helper inlining (including locals), and
branching scalar helper inlining, and operator-specific C entry points are
implemented and covered by executable parity tests. This task remains open for
tensors, host calls, and a unified native
recoverable-error/process-error contract.
