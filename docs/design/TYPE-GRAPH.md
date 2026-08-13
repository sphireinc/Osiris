# Canonical compiler type graph

## Contract

Each Osiris compilation owns a `TypeContext`.  A `TypeID` is valid only inside
the context that allocated it; it is a compact identity for one canonical type
node, not a serialized type name.  Contexts have no process-global mutable
state, so independent compiler, test, and LSP compilations cannot share or
contaminate type arenas.

Type equality inside the checker is identity equality.  Structural constructors
intern their kind and canonical child IDs; two independently discovered
`list<Lazy<u32>>` types therefore have one `TypeID` in a context.  Nominal
nodes include a declaration identity, so declarations with an identical
spelling or representation can remain distinct.

`TypeNode.Display` is presentation metadata.  It is excluded from the interner
key.  The only formatter is `TypeContext.Format`; changing a diagnostic or
source-friendly label must never affect type checking, inference, or lowering.

Generic substitution works by replacing `TypeID`s recursively and interning
the resulting graph.  It must not format a type, replace text, then parse the
result back into a type.

## Current migration boundary

The parser still produces legacy `staticType` values while reading source
annotations.  `TypeContext.InternStatic` is the one-way adapter at that source
boundary.  Checked bindings, checked function signatures, checked expressions,
IR function signatures, typed product fields, and IR type assertions
immediately acquire a canonical `TypeID`; checker equality, joins, generic
argument conflict checks, trait method signature comparison, and native
assertion support validation use the context's canonical identities.

VM type assertions and compiled VM function signature metadata carry the same
IDs. WASM's bounded static evaluator validates IR assertions through the graph.
The LSP formats checked binding IDs for hover/detail and inlay-hint display.
At a runtime value boundary, a compiled assertion structurally walks the
canonical node; it does not format then parse a compiler type.

Generic parameters are canonical nodes scoped by their declaring signature's
stable compilation-local declaration ID. Consequently two unrelated
declarations that both spell a parameter `T` remain distinct. The IR and VM
reuse the checked function node rather than independently interning generic
source spelling during lowering.

Interpreter and VM function values retain the checked type context plus their
signature, parameter, and result IDs. Their runtime argument/result guards use
those IDs directly. Legacy text is retained only to render source-compatible
diagnostics and to support values constructed by older compatibility helpers.

The remaining migration work is deliberately tracked by Task 08: declaration
symbols for generic parameters and nominal declarations must replace
name-based source adapters; generic function IDs must be threaded unchanged
from their checked declaration into all IR/VM signatures; IR annotations and
all backend type assertion payloads must carry `TypeID`s; and the runtime-only
value/type compatibility adapter must remain a value-boundary check rather
than become compiler type semantics.

## Invariants

- `TypeID(0)` is invalid; all valid IDs belong to exactly one `TypeContext`.
- Formatting is pure and cannot allocate or alter canonical identity.
- Canonical nodes own defensive copies of child-ID slices.
- Alias resolution occurs before canonicalization, so aliases of the same
  target share identity while newtypes and nominal declarations do not.
- Checked programs expose canonical alias targets and nominal declaration IDs
  for tooling and lowering. Enums, structs, newtypes, and traits all use the
  declaration-scoped nominal identity rule.
- Checker struct-field tables retain a source adapter for diagnostics but store
  canonical field IDs and compare those IDs for non-dynamic field values.
- Trait and implementation method tables retain canonical signature IDs;
  receiver-method lowering preserves those IDs and only adapts a synthesized
  receiver parameter at the source-to-IR boundary.
- A backend must receive semantic type information from the typed program or
  IR; it must never parse a formatter-produced type string to decide behavior.
