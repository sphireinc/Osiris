# Task 111: Add structs, tuples, aliases, and newtypes

**Priority:** P1
**Dependencies:** 018, 028, 036, 110
**Status:** Complete

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
  bytecode/cache versions, codecs/JSON, inference, equality, LSP, source maps,
  and documentation. The native C IR boundary rejects product-type lowering
  with a source-spanned diagnostic until a separately scoped tagged-ABI task
  provides owned field storage; it never erases nominal identity or evaluates
  the source as a compilation fallback.
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
- Complete `docs/` language and technical-reference coverage with runnable
  examples.

## Acceptance criteria

- [x] Structs, tuples, aliases, and newtypes have complete documented
      semantics and deterministic representation.
- [x] Go-style field tags parse and format stably, survive semantic/artifact
      boundaries, and drive JSON without changing ordinary value equality.
- [x] The interpreter and VM preserve nominal identity and field types; native
      C reports the documented product-type compile-time boundary.
- [x] Serialization, matching, equality, codec metadata, and native rejection
      are covered by focused tests.
- [x] Declaration and field diagnostics are source-spanned; fields are public
      within the current module model, with cross-module visibility enforcement
      deferred until module-private members are specified.
- [x] Focused Go tests, the complete `internal/osiris` package, documentation
      reference/navigation checks, runnable documentation examples, and the
      Vite production build pass.

## Implementation evidence

- `internal/osiris/structs_test.go` covers interpreter/VM parity, tuple
  destructuring/indexing, aliases, newtypes, immutable `with` updates, JSON
  tags, malformed tags, codec round trips, and the explicit native boundary.
- `docs/src/language-reference.json` adds runnable entries for `struct`,
  tuples, `type`, `newtype`, and `with`; `docs/src/technical-reference.json`
  documents grammar, identity, tags, and backend support.
- Validation: `go test ./internal/osiris -count=1`; `npm run check-reference`;
  `npm run check-examples` with Go 1.26.1; and `npm run build` under Node 24.

## Non-goals

- No mutable fields, inheritance, classes, generic struct parameters, or
  interface dispatch in this task.
