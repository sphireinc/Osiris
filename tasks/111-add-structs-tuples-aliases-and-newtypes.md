# Task 111: Add structs, tuples, aliases, and newtypes

**Priority:** P1
**Dependencies:** 018, 028, 036, 110
**Status:** Blocked

## Objective

Add general product types for ordinary application data: declared structs,
heterogeneous tuples, transparent aliases, and nominal newtypes.

## Required language contract

- Use `struct` as the public declaration term; do not introduce a separate
  `record` keyword for the same product-type concept.
- Define struct declaration, construction, field access, destructuring, and
  update syntax.
- Support Go-style raw field tags after a field type, including multiple
  namespace/value pairs and namespace-specific options:

  ```osiris
  struct Spacecraft {
    name string `json:"name" bson:"spacecraft_name"`
    health f64 `json:"health,omitempty" bson:"health"`
  }
  ```

- Define the tag grammar, escaping, duplicate namespace behavior, malformed
  tag diagnostics, unknown namespace preservation, reflection/tooling access,
  and whether tags participate in schema/cache identity. Tags are declaration
  metadata and must not silently change ordinary value equality.
- JSON must consume the `json` tag consistently. Preserve `bson` and other
  well-formed namespaces for their owning codec/tool without requiring those
  codecs in this task.
- Define ordered heterogeneous tuple types and indexing/destructuring.
- Distinguish transparent type aliases from nominal newtypes with explicit
  construction and conversion rules.
- Specify field visibility, declaration/module identity, equality,
  comparability, truthiness, canonical rendering, pattern matching, and
  deterministic field order.
- Define duplicate/missing/unknown field diagnostics and recursive-type rules.
- Structs and tuples are immutable values in this task; updates must return a
  new value and preserve alias safety.

## Implementation requirements

- Extend lexer/parser/formatter, typed semantic model, interpreter, VM,
  bytecode/cache versions, IR/SSA, native tagged ABI, codecs/JSON, inference,
  equality/hashing, errors, LSP, source maps, and documentation.
- Preserve nominal identity through modules, closures, tasks, lists, enums,
  results, and codec round trips.
- Use explicit owned/borrowed/copied contracts for native field storage.
- Stable serialization must use declaration order and reject schema/type
  mismatches rather than relying on Go map traversal.

## Tests and documentation

- Parse-format-parse and invalid-span suites.
- Cross-backend construction/access/destructuring/update/equality tests,
  including nesting and recursive declarations.
- Codec compatibility, malformed input, aliasing/cleanup, and sanitizer tests.
- Field-tag parser/formatter, malformed/duplicate tag, JSON rename/omit,
  unknown namespace round-trip, schema identity, and LSP tests.
- Module identity and visibility fixtures across files.
- Complete `newdocs/` language and technical-reference coverage with runnable
  examples.

## Acceptance criteria

- [ ] Structs, tuples, aliases, and newtypes have complete documented
      semantics and deterministic representation.
- [ ] Go-style field tags parse and format stably, survive semantic/artifact
      boundaries, and drive JSON without changing ordinary value equality.
- [ ] All promised execution paths preserve nominal identity and field types.
- [ ] Serialization, hashing, matching, errors, and native ownership are
      parity-tested.
- [ ] Visibility and cross-module diagnostics are source-spanned and stable.
- [ ] Full validation passes.

## Non-goals

- No mutable fields, inheritance, classes, generic struct parameters, or
  interface dispatch in this task.
