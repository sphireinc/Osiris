# Deterministic state and mutation

A deliberately small state model keeps the language value-oriented. It remains
value-oriented: `const` and `let` bindings, function parameters, structs,
tuples, and collections are immutable values. A program may opt into
sequential local state with `var` and a name assignment.

```osiris
const limit = 3
var total = 0
for item in values {
    total = total + item
}
emit total
```

## Contract

- `const name = expression` is a compile-time/module-level constant. Its
  initializer is restricted to statically evaluable literals, constant names,
  pure unary/binary operations, lists, and tuples. It cannot depend on a
  runtime call, task, I/O, or mutable binding.
- `let` is an immutable runtime binding. Its initializer is evaluated once at
  declaration time and may depend on runtime values and function calls.
- `var name = expression` creates a mutable local cell.
- `name = expression` is the only assignment form in this release. The target
  must resolve to a mutable local; parameters, imports, `let` names, struct
  fields, and collection indexes are not assignable.
- The right-hand side is evaluated completely before the cell is replaced.
  If evaluation fails or is cancelled, the old value remains unchanged.
- A cell replacement is a value operation. It never mutates a previously
  observed struct, tuple, or collection, so aliases retain their old value.
  Builders and `with` expressions remain the supported way to produce updated
  aggregate values.
- Assignments are observable barriers in source order. Optimizers may not
  move, duplicate, or remove an assignment or an expression that can observe
  the assigned cell.
- A closure captures the mutable cell and therefore observes later assignments
  made by its defining activation. A task receives a value snapshot of its
  lexical scope; task-local assignments cannot modify the parent task. Shared
  mutation is outside this feature and must use an explicitly synchronized
  runtime capability.
- Loop iterations preserve assignment order. Completed assignments survive
  `continue` and `break`; a failing or cancelled assignment does not partially
  update state. Cleanup and unwinding observe the last successfully committed
  value.

## Backend and analysis requirements

The parser represents `const`, `let`, `var`, and assignment explicitly. Type
checking records constant/mutable status, validates constant initializers, and
rejects writes to immutable bindings before execution. The interpreter eagerly
installs both constant and `let` values, while mutable locals use cells. The VM
and SSA IR lower an assignment as a named state transition. Native lowering
uses the same SSA value replacement, retaining the assignment marker as an
optimization barrier in the generated artifact. No backend may evaluate a
runtime `let`, `var`, or assignment at compile time.

The alias, purity, inference, optimizer, macro, source-map, formatter, LSP,
and shape-validation passes must preserve these statement forms. In-place
aggregate mutation, borrowing, raw pointers, C pointer arithmetic, and
unsynchronized shared state are intentionally deferred.

## Cancellation and errors

Assignment is a two-phase operation: evaluate and validate the new value, then
commit the cell replacement. Context cancellation is checked before evaluation
and before commit. This makes cancellation and errors atomic with respect to a
single assignment and deterministic across the interpreter, VM, and native
paths.

## Migration

Existing programs continue to use immutable `let`. Code that previously relied
on shadowing should use a uniquely named `var` and explicit assignments; this
avoids changing the meaning of closures or loop bindings. Aggregate updates
continue to use `with`, `setat`, and other value-returning builders.
`var` is contextual rather than a globally reserved expression token, so the
existing statistical builtin `var(...)` remains callable. At declaration
position it has the mutable-local meaning; in expression position it retains
the builtin meaning.
