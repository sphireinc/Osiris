# Generics and typed collections

Task 112 adds one shared generic contract to the parser, checker, interpreter,
VM, IR builder, and native boundary.

## Surface syntax

Generic functions declare type parameters after the function name. A bound is
introduced with `:` and is checked at every call site.

```osiris
func identity<T>(value T) T => value
func numeric<T: number>(value T) T => value

emit identity(7)
emit identity<string>("text")
```

Type arguments are inferred from argument positions when possible. Explicit
arguments must have the declared arity. An argument that is only dynamic does
not silently choose a type parameter; the checker reports an inference error.
Conflicting deductions and failed bounds are also errors.

Parameterized collection forms are parsed recursively:

```text
list<T> map<K, V> set<T> option<T> result<T, E> task<T> stream<T>
```

Function types can contain these forms. Parameterized types are invariant:
`list<u64>` is not assignable to `list<number>` and container casts are never
inserted implicitly. Dynamic positions remain an explicit compatibility
boundary.

## Runtime collections

`map_new`, `map_set`, `map_get`, `map_keys`, and `map_values` provide ordered
map values. `set_new`, `set_add`, `set_has`, and `set_values` provide ordered
sets. Updates are persistent: replacing a map key keeps its original position,
while a new key is appended. Duplicate keys replace the earlier map value;
duplicate set values are ignored. Keys and set values must be scalar/hashable
values, and iteration, rendering, and codec serialization retain insertion
order.

The codec uses dedicated map and set tags and validates key eligibility and
duplicate handling while decoding. Element types are recovered from concrete
runtime values at reflection and codec boundaries; empty containers retain the
checker’s typed boundary through dynamic element compatibility.

## Native strategy

Osiris uses a hybrid strategy. The checker performs deterministic substitution
and bounds validation once for all execution modes. Interpreter and VM calls
execute the shared function body with concrete runtime values. Native lowering
uses the existing tagged `OValue` ABI for generic calls; scalar-only paths can
still enter the existing native specialization, while generic type assertions
are compile-time obligations and are not redundantly emitted inside a generic
body. This keeps native artifacts safe without backend-local template logic.
