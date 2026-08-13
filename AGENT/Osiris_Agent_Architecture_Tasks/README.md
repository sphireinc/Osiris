# Osiris Architecture Hardening Task Pack

This directory is an ordered implementation plan for hardening Osiris into a conventionally statically typed, general-purpose language with a small, precise semantic core and a native runtime capable of C-class performance.

These are **implementation instructions for Codex**, not brainstorming notes. Treat every MUST/SHALL statement as a requirement. Do not weaken requirements merely to preserve current implementation shortcuts. If an existing test contradicts a task's specified semantics, update the test as part of the task rather than preserving the old behavior.

## Non-negotiable project direction

1. Osiris SHALL become conventionally statically typed. `dynamic` must not remain the default escape hatch for ordinary source programs.
2. `let` SHALL be eager. Laziness SHALL be explicit in source syntax and type-visible.
3. The compiler SHALL have an explicit staged front end and one canonical typed intermediate representation.
4. The VM SHALL use compiler-assigned integer slots for lexical locals in hot paths.
5. Native Osiris SHALL use one reusable native runtime library rather than generating a separate hand-written-equivalent C runtime into every output program.
6. The canonical production runtime core SHALL be implemented as a portable C ABI/runtime library for native performance and broad platform interoperability. Go remains an excellent implementation language for compiler tooling, but runtime semantics must not be independently reimplemented in several backends.
7. Most standard-library algorithms and policy SHALL migrate into Osiris source; the runtime SHALL contain primitives that require privileged host/platform access or representation knowledge.
8. Maps and sets SHALL become real deterministic hash-based collections rather than linear scans.
9. Long-lived sessions SHALL have bounded/reclaimable evaluator/compiler caches and SHALL not retain dead lexical scope graphs indefinitely.
10. `internal/osiris` SHALL be decomposed into explicit compiler/runtime/backend/tooling boundaries.
11. Feature expansion SHALL stop until the Core 1 semantic and backend-parity gates defined in this pack are met.

## Recommended execution order

Run tasks in this order unless a task explicitly states otherwise:

1. `14_LANGUAGE_SEMANTIC_CONTRACT.md`
2. `01_EAGER_LET_AND_EXPLICIT_LAZY.md`
3. `11_LET_SPEC_IMPLEMENTATION_PARITY_GATE.md`
4. `02_EXPLICIT_STAGED_FRONTEND.md`
5. `08_CANONICAL_INTERNAL_TYPE_GRAPH.md`
6. `09_ELIMINATE_STRING_BASED_TYPE_DECISIONS.md`
7. `03_CONVENTIONAL_STRONG_STATIC_TYPING.md`
8. `07_TYPE_SYSTEM_HARDENING.md`
9. `15_CANONICAL_TYPED_IR.md`
10. `13_DECOMPOSE_INTERNAL_OSIRIS_PACKAGE.md`
11. `16_SPLIT_COMPILER_RUNTIME_PLATFORM.md`
12. `05_SINGLE_NATIVE_RUNTIME.md`
13. `06_STDLIB_IN_OSIRIS.md`
14. `12_SESSION_RETENTION_AND_CACHE_LIFETIME.md`
15. `10_REAL_DETERMINISTIC_HASH_MAPS_AND_SETS.md`
16. `04_VM_INTEGER_LOCAL_SLOTS.md`
17. `17_RUNTIME_DATA_STRUCTURES_AND_VM_PERFORMANCE_GATE.md`
18. `18_CORE1_SURFACE_FREEZE.md`

Several tasks can be developed in branches concurrently, but merge them in dependency order. In particular, do not complete the static-type-system migration before the canonical type graph exists, and do not make typed IR authoritative until front-end staging and canonical types are stable.

## Repository locations known from the architectural review

Important current files/subsystems include:

- `cmd/osiris/main.go` — CLI entry point.
- `internal/osiris/lexer.go` — lexer.
- `internal/osiris/parser.go` — parser/AST construction.
- `internal/osiris/typecheck.go`, `binding_type.go`, `infer.go`, `future_types.go` — current type machinery.
- `internal/osiris/session.go`, `runtime.go`, `evaluator_iterative.go`, `value.go` — interpreter/runtime.
- `internal/osiris/bytecode.go` — register VM/compiler.
- `internal/osiris/ir.go`, `ssa.go`, `cfg.go`, `dom.go`, `loop.go`, `alias.go`, `closure.go` — IR and compiler analyses.
- `internal/osiris/cbackend*.go` — native C backend and generated native runtime fragments.
- `internal/osiris/collections.go` — map/set implementation.
- `internal/osiris/task.go`, `scheduler.go` — task/concurrency machinery.
- `internal/osiris/wasm*.go`, `gpu*.go` — alternative backends.
- `internal/osiris/lsp*.go` — LSP/tooling.
- `docs/design/STATE-AND-MUTATION.md` and `docs/src/language-reference.json` — current `let` semantics documentation.
- `internal/osiris/lazy_loop_test.go`, conformance/parity tests — current lazy behavior and backend validation.

Do not assume these files remain in these locations after the package-decomposition tasks; update imports, tests, docs, generators, scripts, and tooling references as files move.

## Universal implementation rules

For every task:

- Preserve user-visible behavior unless this task explicitly changes it.
- Add tests before or alongside implementation changes.
- Prefer small explicit compiler passes over implicit side effects hidden inside parser/type-checker/evaluator methods.
- Do not add another fallback to the AST interpreter to make an incomplete backend pass tests.
- Do not silently treat unsupported constructs as `dynamic` or `any`.
- Do not encode semantic identity in formatted type strings.
- Do not add global mutable compiler state.
- Do not introduce a dependency cycle between syntax, types, IR, runtime, backends, standard library, and tools.
- Preserve useful source spans through every representation.
- All compiler diagnostics must remain source-located and deterministic.
- Run formatting, unit tests, conformance tests, differential/backend-parity tests, fuzz tests where relevant, and native sanitizer tests where available.
- When a task changes documented behavior, update both human-readable design docs and machine/data-driven documentation sources.
- Leave the repository in a buildable state at every merge commit. For large migrations, use adapters temporarily, but mark and remove them before declaring the task complete.

## Definition of done for this entire pack

The pack is complete only when:

- A normal Osiris source program is fully statically checked unless it explicitly opts into a narrowly defined dynamic/FFI boundary.
- `let` is eager and explicit lazy values work identically across supported backends.
- The front end has explicit parse → expand → resolve → type → typed-HIR/IR phases.
- Compiler-internal types have stable identity independent of strings.
- The VM uses slot-based lexical locals.
- Native executables link a reusable Osiris runtime library instead of receiving a many-thousand-line generated runtime implementation.
- Core stdlib algorithms are written in Osiris where practical.
- Maps/sets have expected O(1) lookup/insertion with deterministic observable iteration semantics.
- Long-running REPL/LSP/embedded sessions do not grow without bound because of stale AST/scope memoization.
- Compiler/runtime/platform/tooling package boundaries are enforced by imports.
- Core 1 has one semantic conformance suite executed by every backend that claims Core 1 support.
