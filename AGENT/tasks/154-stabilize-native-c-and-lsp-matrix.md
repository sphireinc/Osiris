# Task 154: Stabilize the Native C and LSP support-matrix columns

**Priority:** P0
**Dependencies:** 160,161,162,163,168
**Status:** Blocked until the Native C and LSP closure tasks complete

## Objective

Bring the Native C and LSP columns of the Backend Support Matrix from
`partial` to `stable`. This is the active parity gate for the current release
priority. WASM, GPU, alternate-platform, and all-backend release work is
tracked separately by tasks 300–308, 310, and 400 and is not required to begin
this gate.

## Exact scope

- Reconcile every Native C row against generated-C execution, interpreter/VM
  differential fixtures, ownership and cancellation checks, and granular
  documentation.
- Reconcile every LSP row against protocol fixtures, UTF-16/source-span
  checks, cancellation/race evidence, workspace/refactoring behavior, and
  documented response shapes.
- Add a Native C/LSP-only matrix audit so these columns cannot be promoted by
  prose or by evidence belonging to another backend.
- Keep WASM and GPU statuses unchanged. Task 310 owns the eventual all-backend
  release decision after tasks 300–308 and 400 are complete.

## Acceptance criteria

- Every Native C and LSP cell in `docs/src/support-matrix.json` is exactly
  `stable` and has executable evidence and a granular documentation route.
- Native C evidence includes normal, race/sanitizer-equivalent, ownership,
  cancellation, generated-executable, and documented unsupported-path checks.
- LSP evidence includes every advertised method, malformed and cancelled
  requests, UTF-16 positions, stale document versions, multi-file workspace
  edits, and deterministic protocol fixtures.
- No Native C or LSP row invokes an interpreter, VM, planner, or reference
  provider as a hidden fallback.
- Tasks 160, 161, 162, 163, and 168 have complete records under
  `AGENT/tasks/completed/` before this task is marked complete.

## Current gate evidence

The focused Native C/LSP parity suites pass, including the task-160 semantic
and task-161 workspace fixtures, task-162 tuple/data/product-pattern
differential coverage, and task-163 text/signal/time/crypto stdlib boundary
coverage. The support-matrix checker also passes its schema and documentation
checks.

The release gate remains blocked: the matrix currently contains 30 `partial`
Native C cells and 23 `partial` LSP cells. The bounded slices above do not
prove all rows, sanitizer/cross-platform execution, or the full ownership and
cancellation contracts required by this task, so no matrix promotion or
completion record is justified yet.

The repository now exposes a priority-lane gate independent of the deferred
backends: `OSIRIS_REQUIRE_NATIVE_LSP_STABLE=1 ./scripts/check-support-matrix.sh`
runs `TestSupportMatrixNativeAndLSPReleaseGate` and checks only the Native C and
LSP columns. The all-backend `OSIRIS_REQUIRE_STABLE_MATRIX=1` gate remains
separate so WASM/GPU evidence cannot mask or block the Native C/LSP closure
audit.

The latest evidence extends the gate without changing its status: typed-member
and generic/macro LSP navigation, cancellable colocated-test inventory/code
lenses, deterministic semantic tokens, pull diagnostics, inlay hints, folding
and selection ranges, Native C tuple destructuring, named products,
aliases/newtypes, product patterns, portable SHA-512-family crypto, signal
transforms, and pure text operations now have focused normal/race or
sanitizer-backed fixtures and granular documentation. The remaining rows still
require complete semantic/workspace/refactoring coverage and the
standard-library, host-resource, wider numeric, and cross-platform Native C
surfaces listed in the dependent tasks.

The local evidence gate was strengthened on 2026-08-09. Generated-C
sanitizer builds now retain the production `-O3 -flto` boundary, and portable
`osolve` uses explicit fused multiply-add elimination updates. The complete
Native C suite passed on this macOS host under normal, race, LLVM ASan, and
LLVM UBSan execution. A bounded `std/crypto.mix` member also passed normal,
race, and LLVM ASan differential tests. LSP declaration navigation passed
normal and race validation, cancellation, UTF-16, and framed protocol tests.
These additions improve evidence quality but do not promote aggregate cells:
the release gate now reports 53 non-stable cells, 30 Native C and 23 LSP,
because each column still has rows whose complete semantics, ownership,
workspace, or cross-platform evidence is not closed.

The Native C `std/list` cell is now independently closed. The generated
dispatcher executes all 25 exported list members, including overlapping
`windows(values, width, step)` after separating window validation from slice
validation. `TestNativeLibraryListPureParity` and
`TestNativeLibraryListCompleteOwnershipAndCapabilityContract` pass across
interpreter, VM, generated C, and native execution with normal, race, and LLVM
ASan evidence. This is one module-level cell only; the Native C/LSP release
gate remains open for the other 27 rows in each column.

The Native C `std/crypto` cell is now independently closed as well. The
generated runtime executes every exported constant and function, including
local SHA-1 UUID v5 and bounded deterministic HMAC expansion. The complete
normal, race, LLVM ASan, and LLVM UBSan suites pass through interpreter, VM,
generated C, and native execution, with granular public reference entries and
no capability fallback. The aggregate Native C/LSP release gate remains open
for the other rows.

The latest bounded Native C evidence extends task 162 without changing this
release gate. Qualified `std/list.range`, `std/list.sum`, and
`std/list.product` now select owned direct helpers
`ostd_list_range`, `ostd_list_sum`, and `ostd_list_product`, with differential
normal and race coverage for identities, numeric validation, arity errors,
ownership, and the absence of dispatcher or capability fallback. Native
Result/Option combinators (`map`, `map_error`, and `flat_map`) and `isok`/
`iserr` likewise have direct tagged-ABI lowering with callback short-circuit
and structured-error propagation through `try`; `wrap(cause, kind, message)`
has direct `owrap` lowering with independently tracked cause ownership.
Focused generated-executable parity, malformed-input, ownership, and
no-fallback tests cover these slices. The remaining structured-error metadata
and cause mutation, dynamic callback shapes, async failure paths, and broader
array/range/reduction/data rows remain open, so the aggregate Native C cells
stay unchanged.

The latest LSP request-boundary evidence is also recorded without promotion.
`typeHierarchy/prepare` now requires and validates `position`,
`textDocument/formatting` requires a non-empty `textDocument.uri`, and the
navigation validation fixtures cover malformed, negative, out-of-document,
and UTF-16-surrogate-splitting positions with deterministic `-32602` errors;
cancellation remains `-32800` without partial publication. These are strict
method-boundary slices only. Broader advertised semantic, workspace,
refactoring, repository-module, test-discovery, and client-conflict behavior
still prevents the LSP column from reaching stable.

The Native C `std/signal` and `std/text` cells are now independently closed.
`std/signal` has direct value lowering for its version, bounds, and `pi`
constants plus generated-C/executable differential coverage for all sixteen
operations, including resampling; its focused normal, race, LLVM ASan, and
LLVM UBSan suites pass. `std/text` now has an exhaustive generated-C fixture
for every exported member, including trim aliases, predicates, slug, `cat`,
and Unicode chunks, with normal, race, LLVM ASan, and LLVM UBSan evidence. The
support matrix promotes only these two Native C cells to stable; the aggregate
Native C/LSP gate remains open for the other rows.

The Native C `std/time.zone` UTC-only slice is also now admitted and covered by
normal, race, and LLVM ASan differential tests. It does not promote the
`std/time` cell: the location-bearing datetime ABI, deterministic timezone
data, parser, and injected clock remain required before that module can be
stable.

The LSP `typed-functions-and-static-checking` cell is now independently closed.
The shared checker supplies typed parameter/result, generic type-parameter,
typed-member, trait-method, and callable-signature identities to hover,
definition, references, completion, and signature help. Maintained semantic and
framed fixtures cover UTF-16 positions, malformed and stale requests,
deterministic concurrent responses, and cancellation without partial results.
The aggregate gate remains open because the remaining Native C and
construct-specific LSP cells still require their own complete evidence.

The LSP `deterministic-state-and-mutation` cell is now independently closed for
scalar bindings. The analyzer exposes distinct `const`, `let`, and `var`
contracts, retains assignment references, and emits immutable-target
diagnostics. Direct and framed protocol tests pass under normal and race
execution; aggregate Native C and remaining LSP rows are unchanged.

The LSP `patterns-and-matching` cell is now independently closed for enum and
named-product/struct navigation. Maintained analyzer and framed tests cover
source-backed type/case/field definitions, nested bindings, exact UTF-16
ranges, stale and malformed requests, and unknown-pattern diagnostics. The
Native C pattern row remains separate and unchanged.

The LSP `modules-packages-and-macros` cell is now independently closed for
explicit source-backed, standard-library, and verified repository exports plus
hygienic macro navigation. Maintained user-module, repository-provider, and
macro fixtures cover shared identities, UTF-16 positions, stale/malformed
requests, cancellation, workspace boundaries, and deterministic framing. The
Native C module row remains partial and unchanged.

The LSP `std-text` cell is now independently closed. Runtime-derived export
inventory coverage proves every `std/text` member appears in completion, while
the existing semantic/framed fixtures prove representative navigation,
Unicode/UTF-16, stale/malformed requests, cancellation, and deterministic
responses. Native C text remains stable independently; no other backend status
is changed here.

The Native C `linear-algebra` cell is now independently closed for the bounded
configured numerical-provider ABI. Generated executable parity, complex
residuals, provider absence/ABI mismatch diagnostics, and owned tensor/list
result checks pass under normal and race execution. Production provider
availability remains an explicit deployment requirement; this does not claim a
bundled solver or alternate-platform evidence.

The LSP standard-library export cells for `std/math`, `std/crypto`,
`std/signal`, `std/list`, and `std/time` are now independently closed. A
runtime-derived inventory fixture covers every exported member with deterministic
framed completion, while the existing semantic fixtures cover representative
virtual identity, UTF-16, stale/malformed, and cancellation behavior. The
remaining Native C/LSP rows are unchanged.

The LSP `types-and-numeric-families` cell is now independently closed. Exact
source-backed alias/newtype definitions, struct-field annotations, typed
function parameter/return references, and numeric-literal cursor handling are
covered by deterministic direct and framed fixtures with UTF-16, malformed,
stale-version, and cancellation checks. Only this LSP cell is promoted; the
Native C numeric-family column remains partial.

The Native C `native-result-option-error-codec` row is now independently
closed for the constructor and OSR1 codec boundary. Generated executable
parity, malformed-record rejection, exact arity and wrong-kind failures,
payload-box ownership, direct helper selection, and no-capability/no-dispatch
checks pass under normal, race, and LLVM AddressSanitizer execution. The
broader `errors-results-and-options` row remains partial because control-flow
helpers such as `try/catch`, `unwrap`, `expect`, and `collect` are not included
in this bounded row.

The LSP `tooling-and-analysis` cell is now independently closed. The complete
advertised-method protocol fixture and focused semantic/workspace fixtures
cover initialize capability negotiation, deterministic framed responses,
UTF-16 positions, stale versions, malformed requests, cancellation, semantic
tokens, workspace symbols, code actions, formatting, diagnostics, hierarchy,
and colocated test tooling. Only that LSP cell is promoted; the remaining
construct-specific LSP cells remain partial.

The LSP column now also has a stable granular row for synchronous control flow
and lexical bindings. `analyzeStmt` and `analyzeExpr` preserve source-backed
identity through function parameters, `const`/`let`/`var` declarations,
assignments, loop scopes, `defer`, returns, calls, and block control flow.
`TestLSPCoreSynchronousControlFlowTracksLexicalTargets` and
`TestLSPCoreSynchronousControlFlowProtocolIsDeterministicAndValidatesRequests`
prove exact source spans, deterministic UTF-16 definition output, malformed and
stale request errors, and cancellation. This closes only the named
synchronous slice; asynchronous tasks, streams, and host I/O remain partial in
the broader LSP row.

Native C also has a stable granular `std-math-remainder` row for the direct
`fmod`, `remainder`, and `fma` scalar emitters. The maintained differential,
ownership, structured-error, arity, and shape tests cover matching-width
default/f32/f64 floating values without dispatch or host capability. The
aggregate `std-math-scalar` row remains partial for its unproved numeric
families and is not changed by this bounded promotion.

The LSP column also has a stable granular `lsp-document-highlights` row.
`languageServer.documentHighlightsContext` uses shared analyzer identities to
produce deterministic UTF-16 `read` and `write` highlights, with maintained
normal/race and framed fixtures for malformed, stale, surrogate-splitting, and
canceled requests. This method-level promotion leaves the broader tooling and
core language rows unchanged.

Native C has a stable granular `std-time-utc-projection` row covering
`time.utc(datetime)` and the UTC-only `time.zone(datetime)` projection. The
maintained generated-executable, ownership, wrong-kind/arity, and capability
tests prove value-preserving UTC conversion and newly owned `["UTC", 0i64]`
results without clock or timezone-database access. The aggregate `std-time`
row remains partial for IANA/fixed locations, `now`, parsing, timers, and
alternate-platform evidence.

The LSP column now has stable granular inventory rows for `std/net` and
`std/os`. The runtime-derived export test enumerates every live member without
performing host I/O, while shared framed fixtures cover deterministic
completion, virtual identity, UTF-16 positions, stale/malformed validation,
and cancellation. These rows do not alter the partial Native C network or OS
execution claims.

Native C now has a stable granular `native-posix-filesystem-and-roots` row for
the locally executable macOS/POSIX boundary. File resources, metadata and
mutation, root FS bridging, ownership/timestamp operations, capability gates,
cleanup, and symlink-safe no-escape resolution have generated-executable
evidence. Windows HANDLE adapters, Linux differential evidence, and the
remaining aggregate std/os APIs stay partial.

Native C also has stable granular POSIX rows for process resources and standard
resources. Process spawn/wait/signal/kill, environment overlays, process-group
timeouts and reaping, standard streams/raw handles, signal subscriptions, and
bounded watch snapshots pass focused generated-executable normal/race tests.
The aggregate OS row remains partial for Windows HANDLE adapters, Linux
evidence, and unlisted host semantics.

The promoted Native C granular rows were also rerun together under the locally
installed LLVM AddressSanitizer toolchain: remainder math, UTC/time-zone
projection, OSR1 codecs, synchronous cleanup, environment/identity,
POSIX filesystem/roots, process lifecycle, standard resources, signals, and
watches all passed. This is macOS evidence only and does not replace the
deferred Linux/Windows or network-host evidence.

The LSP column now has a stable granular `lsp-declaration-navigation` row.
`declarationContext` shares source-backed identities with definition
navigation, and its dedicated framed fixture plus direct tests cover exact
UTF-16 ranges, deterministic output, malformed/stale/out-of-document and
surrogate-splitting positions, and cancellation. The broader LSP aggregate
remains partial for the still-open semantic and refactoring surfaces.

The LSP column now has a stable granular `lsp-implementation-navigation` row.
`implementationsContext` selects concrete `method` definitions for trait and
implementation-method identities, preserves deterministic source ordering, and
rejects stale/malformed/canceled requests without partial publication. The
dedicated framed fixture and type-navigation tests provide UTF-16 and race
evidence; the broad LSP aggregate remains partial for other semantic and
refactoring gaps.

Native C also has a stable granular `native-osr1-codecs` row covering the
maintained scalar and structured Error/Result/Option OSR1 tree. Cross-direction
parity, canonical tags, malformed framing and recursive-bound rejection,
tracked decoded ownership, exact constructor contracts, and no host-capability
or evaluator fallback are covered by normal, race, and sanitizer-ready tests.
The aggregate data-helper row remains partial for unimplemented codec tags and
data-pipeline helpers.

Native C now has a stable granular `native-os-environment-and-identity` row
for the POSIX/macOS boundary. Generated-executable parity covers capability-
gated environment snapshots/mutation, missing values, path separators,
hostname, process IDs, and page size; the aggregate OS row remains partial for
filesystem, process, watch, Windows/Linux, and cross-platform evidence.

The Native C `native-synchronous-cleanup` row is now stable for the bounded
lexical scope surface. IR cleanup edges, normal and return exits, loop
`break`/`continue`, captured values, and optimizer-preserved thunk ownership
are covered by maintained generated-C tests. Async cancellation, task
termination, and complete cross-platform resource-close semantics remain
partial in the aggregate cleanup row.

## Explicitly deferred

WASM lowering/import execution, GPU hardware/device execution, alternate-OS
evidence, and the final all-backend matrix transition remain in tasks 300–310.
This task must not weaken their requirements or change their matrix cells.

## Pause checkpoint (2026-08-09)

Implementation is paused with the release gate intentionally open. The current
machine-readable matrix has 48 features: 18 stable and 30 partial Native C
cells, and 25 stable and 23 partial LSP cells. Tasks 161, 162, 163, and 168
remain active prerequisites; none has a complete acceptance record under
`AGENT/tasks/completed/`. Environment-gated evidence is split into tasks
401 (Linux generated-C/sanitizers), 402 (Windows native resources), and 403
(real GPU device execution), coordinated by task 400. Resume by closing the
remaining local semantic, ownership, workspace, and host-provider gaps before
re-running the Native C/LSP release gate.

The checkpoint also includes Native C structured-error `wrap`/`cause` and
`std/time` comparison slices, plus LSP test-code-lens UTF-16 conversion and
strict navigation-position validation. These bounded additions do not change
the aggregate counts or release-gate status. The inventory test now reflects
the already implemented native tensor-shape builtins `tile`, `gather`,
`scatter`, and `pad`; `clone` and `view` remain unsupported.
