# Traits, interfaces, methods, and iteration

Task 113 chooses nominal conformance. A `trait` and `interface` declaration
have the same syntax and semantics; `impl Trait for Type` is the only way to
declare conformance. A method declared with a receiver is callable on values of
that receiver type, but does not by itself create a trait implementation.

```osiris
trait Printable { func render() string; }
struct Point { value u64 }
impl Printable for Point { func render() string => "point" }

let point = Point { value: 1 }
let view Printable = point
emit view.render()
```

Methods use `func (receiver Type) name(...) ReturnType` at top level, or the
short `func name(...) ReturnType` form inside an `impl` block. The receiver is
bound as the first hidden call argument. Method lookup is deterministic by the
nominal receiver type and method name; fields win over methods with the same
name. Duplicate implementations, duplicate methods, missing methods, and
signature conflicts are source-spanned checker errors. There is no implicit
extension-method search or runtime monkey patching.

The interpreter and VM entry point share the same function value and receiver
binding representation. The IR records receiver methods as qualified function
names and retains the receiver type. Native C uses a tagged witness table for
primitive receiver types (`bool`, strings, numeric widths, and floating-point
widths): the table creates a closure whose first owned capture is the tagged
receiver, and the method wrapper prepends that capture to the call arguments.
Methods with lexical captures and product-type receivers remain explicit
compile-time errors until those layouts have a native ownership ABI.

`for item in value` consumes the shared lazy iterable contract. Lists and
tuples retain their existing order; strings yield Unicode scalar strings;
tensors yield their flat data order; maps yield insertion-ordered keys; sets
yield their stable insertion order; readable streams yield one line per
`next` until EOF; and tasks yield their awaited result once. No finite
collection is copied and a stream is not drained before the first body
iteration. A user struct may implement `iter()`, returning a cursor whose
`next()` returns `some(value)` or `none()`. Cancellation is checked before
every cursor step and before each blocking stream read. Unsupported values
report a structured runtime error instead of being treated as an arbitrary
list.

Generic method signatures use the existing generic type syntax and are checked
with the same function-signature rules. Trait/interface values are represented
at runtime by their concrete tagged value plus the static conformance boundary;
native dispatch uses that concrete tag and the witness table. Equality remains
the underlying value equality. Serialization encodes the concrete value, not a
process-local witness pointer, so decoded values do not retain a callable
method environment; resource handles and lazy cursors remain non-serializable.
Type tests and downcasts use the existing typed `match` patterns, which bind a
concrete value only after the checker/runtime verifies its nominal type. There
is no implicit downcast or unchecked cast.
