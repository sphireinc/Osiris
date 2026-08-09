# Task 160: Complete LSP semantic IDE parity

**Priority:** P1
**Dependencies:** 143
**Status:** Complete

## Objective

Complete semantic editor functionality for the full Osiris language surface:
completion, hover, signature help, definitions, references, document symbols,
workspace symbols, diagnostics, and semantic tokens.

## Exact scope

- Own LSP semantic indexes, source-span/UTF-16 conversion, completion,
  signature help, hover, definition/reference search, symbols, tokens, and
  diagnostics for functions, types, generics, traits, impl methods, modules,
  patterns, macros, and standard-library exports.
- Reuse compiler/parser/typechecker/module APIs; do not duplicate semantic
  rules inside the LSP.
- Add framed JSON-RPC fixtures for valid, malformed, incremental, and
  cancellation cases.

## Acceptance criteria

- Every advertised LSP method has protocol fixtures and exact response-shape
  documentation.
- Unsupported requests return explicit protocol errors; no AST form is silently
  ignored.
- UTF-16 positions, generated macro spans, module boundaries, and stale
  document versions are tested and race-safe.

## Current implementation evidence

- `languageServer.signatureHelp`, `lspActiveCall`, `findLSPFunction`, and
  `lspSignatureParameters` now provide structured parameter labels, types,
  active-parameter selection, nested-call handling, quoted-string handling,
  UTF-16 positioning, and stale-version rejection.
- `TestLanguageServerSignatureHelpProtocolUsesUTF16AndRejectsStaleVersion`
  provides framed JSON-RPC evidence for that slice, including a race-enabled
  focused run.
- `TestTask160LSPVersionsCancellationAndTypeHierarchy` and the deterministic
  `task160-semantic.jsonl` fixture cover cancellation, stale versions, save
  revalidation, and type hierarchy behavior.
- `documentAnalysis.members`, `addLSPMember`, and
  `resolveCheckedMemberReferences` now index struct fields, trait methods, and
  `impl` methods by nominal owner after shared type checking. The
  `languageServer.completion`, `hover`, `definition`, and `references`
  symbols use that index for checked receivers such as `user.render`, while
  dynamic receivers retain the existing conservative fallback behavior.
- `TestLanguageServerTypedMembersCompletionNavigationAndUTF16`,
  `TestLanguageServerTypedMemberProtocolUsesUTF16AndRejectsStaleVersion`, and
  `TestLanguageServerTypedMemberCompletionCancellation` cover field/method
  completion, source-backed method navigation, framed protocol handling,
  astral UTF-16 positions, stale versions, and canceled requests. The focused
  normal and race runs both pass.
- `indexLSPImportedModules` now indexes resolvable `std/*` exports with stable
  module-member identities and virtual `osiris://stdlib/<module>` definition
  URIs. `TestTask160StdlibExportsNavigateWithUTF16AndIdentity`,
  `TestTask160StdlibExportsFramedUTF16AndStale`, and
  `TestTask160StdlibExportsFramedCancellation` cover completion, hover,
  definition, references, UTF-16 positions, stale versions, and cancellation.

Generic function and trait type parameters are now indexed as source-backed
`type parameter` symbols by `lspTypeParameterDefinition`. The shared
`analyzeGenericFunctionSignature` path resolves occurrences in bounds,
parameter and result types, while `analyzeGenericCallTypeArguments` resolves
explicit in-scope type arguments. `functionDetail` includes generic names and
bounds, and completion exposes LSP kind 25. The
`TestTask160GenericTypeParametersNavigateWithUTF16AndSharedChecker` and
`TestTask160GenericTypeParametersFramedMalformedStaleAndCancellation` tests,
together with `task160-generics.jsonl`, cover source-backed hover, definition,
references, completion, UTF-16 positions, malformed parameters, stale
versions, and cancellation.

Macro declarations are now predeclared in `predeclareAnalysisStmt`, allowing
forward and nested macro calls to resolve through the normal lexical scope.
`analyzeStmt` reuses that declaration identity and indexes macro parameters as
source-backed `param` symbols. `lspMacroParameterSpans` recovers exact UTF-8
byte-column identifier spans from the original macro header, so hover,
definition, and references do not point at the whole macro declaration. The
`TestTask160MacroNavigationFramedUTF16MalformedStaleAndCancellation` test and
`testdata/lsp/task160-macros.jsonl` fixture cover macro-call navigation,
parameter definition ranges, astral UTF-16 positions, malformed parameters,
stale versions, deterministic framing, and cancellation.

The bounded user-module slice adds `lspResolvedModule`,
`resolveLSPImportedModules`, `lspExportedModuleMembers`, and
`indexLSPImportedUserModules`. A local file resolved by `resolveModulePath`
inside the configured workspace is analyzed for its explicit exports; those
definitions are borrowed by the importing analysis so completion, hover,
definition, and references share one pointer identity across documents.
`languageServer.references` scans the combined open/workspace document set and
sorts the result by URI and UTF-16 range. `relinkLSPDocumentsContext` rebuilds
local definitions before linking imports, preserving identity after an open
module overlay changes.
Source-backed completion items carry `lspCompletionData` in their LSP `data`
field with the defining URI and symbol name; virtual stdlib completion items do
not fabricate this payload.

`TestTask160UserModuleNavigationSharesIdentityAndUTF16`,
`TestTask160UserModuleNavigationRejectsStaleMalformedAndCancelled`, and
`TestTask160UserModuleNavigationFramedFixtureIsDeterministic` provide normal
and framed evidence. The slice intentionally does not claim virtual stdlib
sources, unresolved repository modules, imports outside configured roots, or
unparseable source files.

Source-backed rename now shares the same analyzer identity boundary. The
`languageServer.prepareRenameContext` and
`languageServer.renameWorkspaceEditContext` symbols resolve local bindings,
struct fields and methods, generic type parameters, macro symbols and
parameters, and imported module aliases when `documentAnalysis.findAt` proves
their identity. Preparation returns the occurrence range under the cursor;
rename emits versioned UTF-16 edits in deterministic descending source order,
rejects overlapping or stale edits, rejects virtual standard-library symbols,
and checks cancellation throughout the scan. The
`TestTask160161168SourceBackedRenameAcrossSemanticKinds` and
`TestTask160161168RenameMalformedCancellationAndSourceBoundary` tests cover
the semantic kinds, shadow exclusion, invalid positions, malformed identity
boundaries, stale versions, and `-32800` cancellation behavior. The framed
`task161-rename.jsonl` fixture covers exact prepare/rename ranges, stale
version errors, malformed parameters, and deterministic Content-Length
framing.

The semantic analyzer now has an explicit path for every parser statement,
expression, and pattern node. This includes named product patterns and their
recursive field bindings, plus enum pattern type identities that distinguish
the enum name from its qualified case. Product and enum navigation are covered
by `TestLSPStructPatternNavigationAndNestedBindings` and
`TestLSPEnumPatternTypeNavigationUsesExactSourceSpan`. Unsupported synthetic nodes
still produce source-spanned diagnostics rather than being silently ignored.
The complete advertised request set is exercised by
`lsp-protocol.jsonl` and the specialized workspace/semantic fixtures, while
`docs/design/LSP.md` and the generated reference entries document each request
shape, result shape, malformed-request error, stale-version rule, cancellation
behavior, and UTF-16 conversion boundary.

`callHierarchy/incomingCalls` now validates the required call-hierarchy item
identity at the protocol boundary. Empty or missing `item.name` or `item.uri`
returns JSON-RPC `-32602` instead of an ambiguous successful empty array.
`TestTask143ProtocolFixtureAdvertisesAndExercisesAllLSPMethods` covers the
malformed framed request alongside the valid call-hierarchy request, and the
normal plus race-focused protocol runs pass. The exact request, result, and
error contract is documented individually in `docs/design/LSP.md` and the
generated `callHierarchy/incomingCalls` reference entry. Outgoing call and
type-hierarchy item validation now applies the same required identity rule to
`callHierarchy/outgoingCalls`, `typeHierarchy/supertypes`, and
`typeHierarchy/subtypes`; the framed fixture covers all four malformed item
requests. Normal and race-focused protocol runs pass. The exact request,
result, and error contract is documented individually in `docs/design/LSP.md`
and the generated hierarchy reference surface.

The call-hierarchy implementation now closes a semantic identity and
cancellation gap. `prepareCallHierarchyContext` honors request cancellation
and returns only callable definitions; `callHierarchyDefinition` resolves the
prepared item by URI, name, source range, and its stable `data` identity.
`incomingCallsContext` groups references by their containing caller and
`outgoingCallsContext` scans only references contained by the selected caller,
so unrelated functions and top-level calls cannot leak into a result. Both
aggregate repeated ranges, sort callers/targets and UTF-16 ranges
deterministically, and check cancellation during document/reference scans.
`TestLSPCallHierarchyFiltersByDefinitionAndPreservesUTF16Ranges` and
`TestLSPCallHierarchyProtocolFixtureIsDeterministicAndExact`, using
`lsp-call-hierarchy.jsonl`, cover exact result shapes, astral-character UTF-16
call ranges, malformed `-32602` requests, canceled `-32800` requests, and
deterministic framing.

## Completion audit (2026-08-09)

The task acceptance gate is complete. Normal and race-enabled LSP suites pass:

```text
GOCACHE=/tmp/osiris-lsp-full-cache go test ./internal/osiris -run '^TestLSP' -count=1 -timeout=1200s
GOCACHE=/tmp/osiris-lsp-full-race-cache go test -race -vet=off ./internal/osiris -run '^TestLSP' -count=1 -timeout=1200s
```

The focused struct-pattern test additionally proves type/member navigation,
nested binding identities, astral-text UTF-16 conversion, and an
unknown-field diagnostic. No support-matrix cell is promoted here: aggregate
LSP closure remains owned by tasks 161 and 168, which must complete their own
workspace and matrix evidence before changing aggregate statuses.
