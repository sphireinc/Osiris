# Task 115: Close general-purpose backend parity gaps

**Priority:** P1
**Dependencies:** 025, 026, 028, 034, 109
**Status:** Ready

## Current implementation evidence

The VM now executes ordinary functions, closures, typed returns, recursion,
generic functions, trait/impl methods, `while`, `for`, `break`, `continue`,
modules, and task special forms as bytecode chunks; focused parity, conformance,
inventory, and full-repository tests pass. Native C continues to reject
unsupported host features with source-spanned diagnostics. The backend
inventory now enumerates every exported standard-library member and records
the exact native subset instead of making module-wide claims. Native build
flags are portable (no `-march=native`), and sanitizer cache keys include the
active sanitizer configuration. VM task and concurrent-expression snapshots
also clone captured VM function closures through a cycle-safe frame map, so
spawned work cannot mutate or race the parent runner's mutable frame maps.

This task is not complete yet: the machine-checked inventory is now present, and
the maintained macOS corpus has differential, conformance, ownership/error,
task, cancellation, and race-detector evidence, but portable Linux/macOS
execution and sanitizer evidence required by this task are not complete. UBSan
passes on the maintained macOS toolchain. The available Apple clang ASan
runtime aborts before generated `main` during process initialization with
`sanitizer_malloc_mac.inc:189`; Linux ASan evidence remains required before
completion. Do not move this record until those claims are closed.

## Objective

Make the public VM and native C compiler complete for the existing stable
general-purpose language surface, with no interpreter delegation or
compile-time evaluation fallback.

## Scope

- Derive the exact target set from `docs/src/support-matrix.json`, the
  conformance corpus, and runtime builtin/module catalogs.
- VM: remove remaining interpreter delegation for stable syntax, values,
  control flow, errors, modules, tasks, and I/O that the VM publicly claims.
- Native C: close remaining stable-core gaps that are not explicitly hosted,
  provider-backed, WASM-only, GPU-only, or separately unsupported by design.
- Keep WASM and GPU capability-scoped; do not falsely require scientific GPU
  kernels to implement host filesystem/process APIs.

## Implementation requirements

- Add a machine-checked backend capability inventory mapping every stable
  construct/value/builtin/module member to implementation and test evidence.
- Any intentionally unsupported surface needs a source-spanned compile-time
  diagnostic and accurate support-matrix entry.
- Never call the interpreter/VM during native compilation or embed output
  snapshots.
- Preserve value width/style/metadata, output order, errors, cancellation,
  module identity, and ownership.
- Add representative portable Linux/macOS native execution and sanitizer
  coverage.

## Tests and documentation

- Expand the differential harness and conformance corpus to all targeted
  stable features and failure paths.
- Add a test that fails when a public VM path delegates to the interpreter.
- Add generated-C forbidden-fallback assertions.
- Update support matrix and detailed reference notes from the checked
  inventory.

## Validation evidence

The following checks pass on the maintained macOS toolchain:

```text
GOCACHE=/tmp/osiris-go-cache CGO_ENABLED=1 go test ./... -count=1 -timeout=600s
GOCACHE=/tmp/osiris-go-cache go vet ./...
GOCACHE=/tmp/osiris-go-cache go build ./...
GOCACHE=/tmp/osiris-race-cache CGO_ENABLED=1 go test -race ./internal/osiris -run 'TestCBackendMissionControlShowcaseParity|TestCBackendRequiredNativeSurfaceParity|TestConformanceCorpus|TestDifferentialHarness|TestBackendCapabilityInventoryIsCompleteAndUnique|TestRunVMPublicPathHasNoInterpreterDelegation|TestNativeCompilerHasNoEvaluatorFallback|TestCBackendPortableCompilationFlags|TestCBackendSanitizerFlagsAreCacheKeyed' -count=1 -timeout=600s
OSIRIS_C_SANITIZERS=undefined GOCACHE=/tmp/osiris-ubsan-cache CGO_ENABLED=1 go test ./internal/osiris -run 'Test(CBackendMissionControlShowcaseParity|CBackendCodecCrossDirectionParity|CBackendDeferredTaskAndFailureParity|CBackendStructuredResultFailureParity|CBackendRequiredTextUnicodeParity|CBackendRequiredTimeBoundaryParity|CBackendRequiredModuleErrorParity|CBackendCryptoVectorsParity|CBackendRequiredNativeSurfaceParity|ConformanceCorpus|CBackendPortableCompilationFlags|CBackendSanitizerFlagsAreCacheKeyed)' -count=1 -timeout=600s
git diff --check
```

The corresponding `OSIRIS_C_SANITIZERS=address` run remains blocked by the
Apple runtime initialization failure described above; this is not counted as
native sanitizer coverage. A Linux ASan run is still required for closure.

## Acceptance criteria

- [ ] Every targeted stable feature has interpreter/VM/native evidence or an
      explicit scoped unsupported contract.
- [x] The VM contains no hidden interpreter delegation for its stable claims.
- [x] Native compilation contains no evaluator, subprocess, or snapshot
      fallback.
- [ ] Differential, conformance, sanitizer, docs, and full validation pass.

## Non-goals

- Do not claim complete WASM/GPU support for host-only APIs.
- Do not weaken stable semantics to match a narrower backend.
