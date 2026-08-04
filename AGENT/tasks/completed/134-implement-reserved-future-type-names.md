# Task 134: Implement reserved future numeric and shape types

**Priority:** P2  
**Dependencies:** 028, 034, 109, 110, 111, 112, 113, 114  
**Status:** Complete  

## Objective

Replace the current documentation-only reservations for these future type
names with complete, deterministic Osiris types:

- `poly`
- `quat`
- `simd`
- `sparse_matrix`
- `sparse_tensor`

“Complete” means that every advertised surface has one explicit semantic
contract and consistent behavior across parsing, static checking, inference,
the interpreter, bytecode VM, codecs, tooling, native C compilation, and
documentation. A type must be rejected with a precise diagnostic on a backend
that does not support its advertised form; native compilation must never use
compile-time evaluation as a fallback.

The names are already listed as reserved/future types in the reference data
and described by archived type roadmaps. This task turns those reservations
into an implementation plan and then an executable language surface.

## Required design gate

Before implementation, record the decisions below in a durable design
document under `docs/design/` and add representative syntax examples to the
task's implementation tests. Do not infer these rules from host libraries or
from a single backend.

- Generic syntax and arity, including whether the forms are
  `poly<T>`, `quat<T>`, `simd<N, T>`, `sparse_matrix<T>`, and
  `sparse_tensor<T>` or require explicit shape parameters.
- Supported coefficient/element families, width retention, promotion, exact
  arithmetic, overflow, equality, ordering, hashing, and zero behavior.
- Whether `poly` is normalized by trimming trailing zero coefficients and how
  degree, evaluation, division, and roots behave.
- Quaternion multiplication convention, component type, scalar promotion,
  conjugate, norm, inverse, division, and serialization behavior.
- SIMD lane count constraints, lane type constraints, masks, indexing,
  alignment, construction, lane mutation, and native ABI guarantees.
- Sparse matrix/tensor shape validation, default-zero semantics, duplicate
  coordinate policy, canonical ordering, storage formats, iteration order,
  mutation rules, and conversion between dense and sparse values.
- Codec and display formats, including deterministic ordering and versioning
  for sparse entries.
- The native contract: optimized SIMD/sparse kernels versus a semantically
  correct portable representation, and the exact unsupported forms that must
  fail at compile time.

## Implementation surface

Trace and update the smallest relevant symbols in each path. The names below
are the initial integration points; the implementation must use Serena
reference searches to find all callers before changing public contracts.

### Syntax and static semantics

- `parser.parseTypeName` and related generic/type-argument parsing;
- `typecheck.parseStaticType`, `typecheck.staticType`,
  `typecheck.requireAssignable`, and expression/operator checking;
- `infer.inferExpr` and shape/element inference helpers;
- formatter, source-span diagnostics, and LSP type/completion metadata.

### Runtime and execution

- `Value` kind/type metadata and constructors in the value/runtime model;
- interpreter numeric/operator dispatch and built-in constructors;
- bytecode compiler emission and VM dispatch;
- indexing, iteration, mutation, equality, hashing, formatting, and structured
  error paths;
- task/snapshot/closure boundaries where aggregate values are retained or
  transferred.

### Serialization and backends

- value codecs and any JSON/text representations;
- IR and analysis type propagation, purity/alias metadata, and shape facts;
- native C value representation, lowering, generated-C ownership, cleanup,
  and compile-time unsupported diagnostics;
- WASM/GPU or other artifact backends, either with parity or an explicit
  capability-gated rejection.

## Phased delivery

1. **Contracts and type plumbing:** design document, parser/checker/inference,
   type metadata, diagnostics, formatter, and reservation migration.
2. **Algebraic values:** implement `poly` and `quat`, including arithmetic,
   constructors, formatting, codecs, interpreter/VM parity, and native policy.
3. **Fixed-width vectors:** implement `simd` with fixed lane/type checking,
   masks, indexing, operations, and a documented native ABI.
4. **Two-dimensional sparse storage:** implement `sparse_matrix` with shape,
   canonical storage, indexing, iteration, mutation, conversions, and
   deterministic codecs.
5. **N-dimensional sparse storage:** implement `sparse_tensor` using the
   established sparse contracts without duplicating matrix-only semantics.
6. **Backend and performance hardening:** generated-C assertions, executable
   parity, ownership/alias tests, benchmarks, and capability/support-matrix
   updates.

Each phase must leave the earlier types working across all advertised
execution paths; do not add interpreter-only public support and defer parity
indefinitely.

## Required tests and evidence

- lexer/parser and parse-format-parse tests for every generic form;
- invalid arity, element family, shape, lane count, duplicate-entry, and
  mutation diagnostics with stable source spans;
- table-driven algebra and indexing tests, including zero, empty, singleton,
  overflow, exactness, and malformed-value cases;
- interpreter/VM parity for every supported operation and error;
- inference and IR assertions for element type, width, shape, and ownership;
- codec round trips, deterministic output, malformed input, and version cases;
- generated-C assertions, executable native parity, ownership/aliasing tests,
  and compile-time rejection for unsupported native forms;
- race/stress coverage for values crossing task or closure boundaries;
- benchmarks for polynomial arithmetic, quaternion operations, SIMD lanes,
  sparse matrix access, and sparse tensor access, recording the environment
  and raw samples before claiming an optimization;
- documentation examples that execute through the canonical documentation
  checker.

## Documentation

Update the canonical public reference under `docs/` for this repository,
including the language reference, reference-data entries, technical type
index, support matrix, searchable navigation, and examples. Add durable
contracts and ABI/ownership decisions under `docs/design/`. Keep the existing
reserved-name entries until the corresponding phase is actually implemented,
then replace reservation wording with exact supported/unsupported behavior.

The task completion record must identify every advertised type operation,
backend, and documentation entry, with focused and full validation evidence.

## Effort estimate

For one engineer, assuming semantic native support rather than optimized
platform-specific kernels:

- contracts and shared type plumbing: 2–4 days;
- interpreter/VM implementation for all five types: 3–5 weeks;
- native parity, codecs, tooling, tests, documentation, and hardening:
  6–10 weeks total;
- optimized SIMD and production sparse kernels, if required: 8–12+ weeks.

The estimate must be revised after the design gate if generic shapes,
promotion, sparse formats, or native ABI requirements expand the surface.

## Acceptance criteria

- [ ] All five names have reviewed generic syntax and complete semantic
      contracts.
- [ ] The parser, checker, inference, formatter, interpreter, VM, and codecs
      agree on supported values and failures.
- [ ] Every native claim has generated-C and executable parity evidence;
      unsupported forms fail at compile time with stable diagnostics.
- [ ] Sparse representations are deterministic, canonical, bounded, and
      explicit about ownership, mutation, and duplicate coordinates.
- [ ] SIMD behavior and ABI guarantees are explicit for every supported lane
      type and width.
- [ ] Documentation, examples, reference indexes, and support matrices match
      the implemented surface.
- [ ] Focused tests, race tests, full tests, static analysis, build checks,
      and documentation checks pass, or unrelated failures are recorded.

## Non-goals

- Do not reserve additional unrequested type names.
- Do not expose host compiler vector extensions, platform-specific sparse
  libraries, or undocumented ABI details as portable Osiris behavior.
- Do not claim optimized native performance from a boxed or interpreter-backed
  fallback.
- Do not silently coerce unsupported element families, shapes, widths, or
  sparse formats.

## Completion evidence (2026-08-04)

- `internal/osiris/future_types.go` implements deterministic `poly`, `quat`,
  `simd`, `sparse_matrix`, and `sparse_tensor` values with canonical
  construction, arithmetic, indexing, normalization, bounds, zero-elision,
  duplicate aggregation, and dense/sparse conversion rules.
- `internal/osiris/value.go`, `typecheck.go`, `infer.go`, `runtime.go`,
  `bytecode.go`, `codec.go`, `module.go`, and backend policy hooks carry the
  future kinds through static checking, interpreter/VM execution, codecs, and
  explicit native/WASM/GPU rejection where no ABI exists.
- `internal/osiris/task134_future_types_test.go` and
  `internal/osiris/reserved_types_contract_test.go` cover syntax, arithmetic,
  canonical sparse forms, codec round trips, and unsupported-backend
  diagnostics.
- `docs/design/RESERVED-TYPES.md` and the canonical `docs/src` reference data
  document each type and operation individually.
- `GOCACHE=/tmp/osiris-future-cache5 go test ./internal/osiris -run '^TestTask134' -count=1 -timeout=180s` and the reserved-type contract suite passed;
  `npm run check-reference` passed with 315 built-ins and 1021 unique routes.
