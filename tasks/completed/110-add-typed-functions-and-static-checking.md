# Task 110: Add typed functions and whole-program static checking

**Priority:** P1
**Dependencies:** 006, 013, 018, 025, 038, 109
**Status:** Complete

## Objective

Establish the first coherent static type-checking layer by adding typed
function parameters, explicit return types, function types, and compile-time
checking before execution or artifact generation.

## Required language contract

- Define syntax for parameter and return annotations using canonical Osiris
  type names. Include named functions, closures, callbacks, and recursive
  functions.
- Define function types such as a callable accepting `u32` and returning
  `string`; do not infer callability from runtime success.
- Specify assignability, numeric-width compatibility, `Result`/`Option`,
  enum, tensor-shape, task, and `null` rules.
- Decide what is inferred versus what requires an annotation. Diagnostics must
  be deterministic and source-spanned.
- Reject invalid calls and inconsistent returns during checking, before the
  interpreter, VM, native C, WASM, or GPU backend begins execution/emission.
- Preserve existing unannotated programs through documented inference or a
  migration diagnostic; do not silently reinterpret existing syntax.

## Implementation requirements

- Add a typed semantic representation shared by inference, LSP, IR lowering,
  and all execution paths. Do not build a second checker inside one backend.
- Handle recursion and mutually recursive declarations without execution.
- Check every reachable return and fallthrough path.
- Include function types in `Value`/type metadata without erasing closure
  captures or callable identity.
- Keep parse-format-parse stability and update formatter, inference output,
  source maps, completion/signature help foundations, and native diagnostics.

## Tests and documentation

- Parser/formatter tests for valid and invalid signatures.
- Table-driven checker tests for calls, returns, recursion, closures,
  callbacks, shadowing, generics-ready type variables being rejected for now,
  and precise spans.
- Interpreter/VM/native parity for supported typed programs and compile-time
  rejection tests proving no backend executes an ill-typed program.
- LSP fixtures for diagnostics and signature information.
- Canonical `newdocs/` chapters and technical-reference syntax entries with
  checked examples and migration notes.

## Acceptance criteria

- [x] Typed parameters, returns, and function values have a documented grammar
      and semantic contract.
- [x] A single deterministic checker rejects invalid programs before execution
      or artifact generation.
- [x] Interpreter, VM, and supported native paths preserve type identity and
      behavior for focused fixtures.
- [x] Formatter, inference, LSP, diagnostics, and documentation agree on the
      same types and source spans.
- [x] Focused tests, full normal/race/vet suites, docs checks/build, task-index
      check, and `git diff --check` pass.

## Non-goals

- Do not add structs, generics, interfaces, implicit numeric casts, or mutable
  variables in this task.
- Do not implement checker behavior by evaluating user code.

## Completion record

Completed 2026-07-31.

### Language and semantic decisions

- Function annotations use the existing Go-style, colon-free spelling:
  `func apply(value u32, callback func(u32) string) string`.
- Omitted parameter and result annotations remain dynamic for source
  compatibility. Exact function types are invariant; the broad `function`
  kind continues to accept any callable.
- Annotations never cast. Exact numeric widths are required, except that an
  unsuffixed non-negative integer literal may inherit its expected width at a
  typed binding, argument, return, or control-flow outcome.
- `result`, `option`, `task`, `tensor`, and `enum` are first-stage runtime-kind
  constraints. Generic payload/result types, nominal user-defined
  annotations, and parameterized shapes remain assigned to tasks 111 and 112.
- `null` is exact and does not make another type nullable.

The complete grammar, assignability matrix, inference rules, return/fallthrough
rules, dynamic-boundary behavior, and backend contract are in
`docs/design/STATIC-TYPES.md`.

### Implementation

- `internal/osiris/typecheck.go` provides the single deterministic
  `Runtime.CheckProgram` semantic pass and its typed `CheckedProgram` view.
  Function groups are predeclared before bodies, including nested declarations,
  so direct and mutual recursion require no execution.
- Parser, formatter, macro cloning/expansion, source optimization, inference,
  LSP, IR, interpreter, and VM paths preserve annotations and exact callable
  metadata. Function `Value` objects retain signature, closure, and identity.
- Interpreter sessions check the source and a module-stack-aware expanded
  program before the first statement. The expansion path retains incremental
  session macros/enums and preserves import-cycle diagnostics.
- IR carries parameter/result types and emits `asserttype` instructions at
  typed entries and results. Native C retains exact closure signatures, emits
  matching guards, and source-locates unsupported C-ABI runtime kinds rather
  than deferring them to execution.
- WASM snapshot generation, GPU planning, inference, and LSP analysis reject
  known type errors through the same checker before producing their result.
- `newdocs/` language, technical, legacy-rendered, and support-matrix content;
  `LANG.txt`; `README.txt`; changelog; and migration notes describe the same
  optional typed surface.

### Evidence

- `TestTypedFunctionSignaturesParseFormatParse` covers valid signatures,
  canonical formatting, parse-format-parse stability, and optimizer
  preservation.
- `TestTypedFunctionCheckerRejectsKnownMismatchesBeforeExecution`,
  `TestTypedFunctionContextFitsEveryDefaultIntegerOutcome`, and
  `TestTypedFunctionDiagnosticHasArgumentSpan` cover calls, callbacks,
  returns, fallthrough, numeric widths, `Result`/`Option`/`null`, rejected
  generic placeholders, branch outcomes, and exact source spans.
- `TestTypedFunctionRecursionCallbacksAndBackendParity`,
  `TestTypedFunctionMutualRecursionIsCheckedWithoutExecution`, and
  `TestTypedFunctionNestedClosureAndShadowing` cover interpreter/VM/native
  parity, recursion, mutual recursion, closures, captures, callbacks, and
  lexical shadowing.
- `TestTypedFunctionIllTypedProgramIsRejectedByEveryArtifactPath`,
  `TestTypedFunctionCheckerRunsAfterMacroExpansionBeforeExecution`,
  `TestTypedFunctionNativeRejectsUnsupportedRuntimeKindsBeforeEmission`, and
  `TestTypedFunctionDynamicBoundaryKeepsRuntimeGuards` prove pre-execution
  rejection, expanded checking, explicit native boundaries, generated-C exact
  signature assertions, and compatibility guards.
- `TestTypedFunctionMetadataFlowsThroughValueInferenceIRAndLSP` and
  `TestTypedFunctionLSPHoverAndMutualRecursionFixture` cover `Value`, inference,
  typed IR/source maps, LSP diagnostics, exact hover metadata, and mutual
  recursion resolution.

Validation completed with Go 1.26.1, Node 24.12.0, npm 11.6.2, and the system C
compiler:

```text
GOCACHE=/tmp/osiris-go-cache go test ./...
GOCACHE=/tmp/osiris-go-cache go test -race ./...
GOCACHE=/tmp/osiris-go-cache go vet ./...
GOCACHE=/tmp/osiris-go-cache GOMODCACHE=/tmp/osiris-go-mod-cache \
  GOTOOLCHAIN=local ./build.sh
./scripts/check-task-index.sh
cd newdocs
nvm use 24
npm run check-reference
PATH="/opt/homebrew/bin:$PATH" npm run check-examples
PATH="/opt/homebrew/bin:$PATH" npm run build
git diff --check
```

The native cancellation parity fixture uses a cooperatively running task so
the tested task is provably pending when cancelled; it no longer races a
trivial task that may legitimately complete before the cancellation request.
