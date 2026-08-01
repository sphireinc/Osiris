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
names and retains the receiver type. Native C currently rejects receiver and
interface dispatch at the artifact boundary until the vtable/witness ABI is
implemented; it never evaluates these forms during compilation.

`for item in value` consumes the shared iterable contract. Lists and tuples
retain their existing order; strings yield Unicode scalar strings; tensors yield
their flat data order; maps yield insertion-ordered keys; sets yield their
stable insertion order; and readable streams yield lines until EOF. The stream
boundary observes cancellation before each read. Unsupported values report a
structured runtime error instead of being treated as an arbitrary list.

Generic method signatures use the existing generic type syntax and are checked
with the same function-signature rules. Trait values are ordinary runtime
values with a static trait boundary; equality remains the underlying value
equality, and no implicit downcast is introduced by this task.
