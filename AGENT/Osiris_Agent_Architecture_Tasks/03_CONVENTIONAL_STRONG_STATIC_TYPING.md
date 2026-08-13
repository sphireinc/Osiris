# Task 03 — Make Osiris Conventionally and Strongly Statically Typed

## Objective

Change Osiris from a dynamically compatible / progressively typed language into a conventionally statically typed language with a strong type system.

For normal Osiris source code, every expression, binding, call, field access, operator, pattern, return path, collection element, generic instantiation, and control-flow merge MUST have a statically known type before execution or backend lowering succeeds.

`dynamic` MUST NOT be the implicit default for missing information.

## Language-level policy

Adopt these rules:

1. Local type inference is allowed and encouraged:

```osiris
let count = 3
```

infers an integer type according to numeric-literal rules.

2. Function signatures SHALL be statically typed. Decide whether parameter/result annotations are always required or may be inferred for private/local functions. Preferred initial rule for a stable systems language: require parameter and public/exported return types; allow obvious return inference only where already ergonomic and unambiguous.
3. A failed inference is a compile-time error, not a fallback to `dynamic`.
4. Operators are statically resolved. `a + b` must have a valid operator rule/trait implementation for the static operand types.
5. Function calls are statically arity/type checked.
6. Struct field and enum payload access is statically checked.
7. Match/pattern binding types are statically derived and exhaustiveness should be checked where feasible.
8. Collection literals infer a single valid element/key/value type or require an annotation. Heterogeneous collections require an explicit sum/union/boxed type; do not silently turn them into `dynamic`.
9. `Option<T>`, `Result<T,E>`, `Lazy<T>`, tensors, quantities, numeric families and user-defined generics remain structural compiler types, not runtime guesses.
10. Implicit numeric conversions must be deliberately specified and conservative. Lossy/narrowing conversions require explicit casts.
11. No implicit truthiness across arbitrary values unless deliberately part of the language. Prefer `bool` conditions.
12. Undefined identifiers and unresolved members are compile-time errors.
13. Compile-time generic/trait resolution failures are errors before execution.

## Dynamic escape hatch

If `dynamic` remains at all, narrow it to explicit interoperability boundaries:

```osiris
let payload: dynamic = ffi_or_untyped_data()
```

or similar explicit syntax.

Requirements:

- `dynamic` must be written or introduced by a documented foreign/dynamic API.
- Operations on `dynamic` must require explicit checking/casting or invoke a clearly defined dynamic operation.
- `dynamic` must not silently infect surrounding inference.
- Public APIs using `dynamic` should receive compiler/lint warnings unless explicitly annotated as intended.
- Backend optimizers may conservatively treat explicit dynamic regions as opaque.

Do not rename the current fallback to `any` and declare the problem solved.

## Type inference

Implement inference as constraint solving over canonical compiler types, not by evaluating code or inspecting runtime `Value`s.

At minimum support:

- literal inference;
- local binding inference;
- generic call inference;
- expected-type propagation from assignment/return/call argument context;
- branch/control-flow join typing;
- closure parameter/capture typing according to annotation rules;
- collection literal element inference;
- `Option`/`Result` propagation.

Keep inference intentionally local. Do not attempt whole-program Hindley-Milner inference if it compromises diagnostics or implementation clarity.

## Strong typing requirements

"Strong" means static types are not casually bypassed at runtime.

- No arbitrary reinterpretation of one value representation as another.
- FFI conversions are explicit and ABI checked.
- Unsafe memory APIs use explicit unsafe/capability boundaries.
- Newtypes remain distinct unless explicitly converted.
- Type aliases are aliases, not accidental new runtime identities.
- Generic instantiations preserve their parameter identities.
- Runtime type tags may remain for GC/runtime dispatch, but they must not substitute for compile-time checking.

## Migration work

Search for every place that creates, accepts, returns, or compares `dynamicStaticType`, `"dynamic"`, unknown/fallback types, or type strings.

Classify each occurrence:

A. legitimate explicit dynamic boundary;
B. temporary incomplete-inference state;
C. compiler bug/shortcut;
D. error recovery only.

Remove category C. Ensure B cannot reach backend lowering. Error-recovery sentinel types must be distinct from user-visible `dynamic`.

Introduce a compiler-internal `ErrorType`/invalid type ID so the checker can continue diagnostics without pretending an erroneous expression is dynamically valid.

## Runtime simplification

Once types are authoritative, remove runtime checks that only exist to compensate for missing compile-time checking where safe to do so. Keep defensive checks at trust boundaries, FFI, unsafe operations, deserialization, and corrupted/host input.

Do not prematurely remove runtime checks needed for structured language errors such as division by zero, bounds checks, failed checked casts, etc.

## Backend requirements

Interpreter, VM, native, and supported WASM paths must receive typed input. A backend must not re-infer ordinary source types independently.

Native lowering should exploit static types to specialize operations and avoid generic tagged-value dispatch where representation can be unboxed.

The VM may initially keep tagged `Value`s, but bytecode instructions should increasingly encode typed operations such as integer add versus floating add where justified.

## Diagnostics

Produce direct static errors such as:

```text
cannot pass String to parameter `count: u32`
expected: u32
found:    String
```

Include source spans and, where useful, declaration locations.

Avoid diagnostics like "unsupported operation" when the real issue is a specific type mismatch.

## Required tests

Add negative compile tests for:

- wrong argument types;
- wrong return types;
- mixed incompatible collection literals;
- invalid field/member access;
- invalid arithmetic;
- narrowing conversion without cast;
- unresolved generic parameters;
- unsatisfied trait bounds;
- branch type mismatches;
- invalid mutation type;
- invalid pattern payload type;
- accidental dynamic fallback.

Add positive tests for inference and generic/type propagation.

Add a repository guard test that compiles representative programs and asserts no unresolved/non-error sentinel type reaches typed HIR/IR.

## Acceptance criteria

- Ordinary source programs cannot execute if static type checking failed.
- Missing type information never silently becomes `dynamic`.
- Explicit dynamic boundaries are rare, documented and type-visible.
- All supported backends consume the same typed semantic representation.
- Static typing is strong enough that runtime `Value` kind errors caused by ordinary source type mismatch are treated as compiler bugs, not expected user behavior.
