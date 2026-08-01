# Task 115: Close general-purpose backend parity gaps

**Priority:** P1
**Dependencies:** 025, 026, 028, 034, 109
**Status:** Complete

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
Native floating-point contraction is explicit (`-ffp-contract=fast`) so the
mission-control production fingerprint is stable across the validated macOS
and Linux/arm64 toolchains. Sanitizer builds retain practical O1
instrumentation without LTO. VM concurrent-emission validation now rejects
statically visible effects before scheduling, and TCP deadline errors normalize
to context cancellation semantics across operating systems.

The targeted stable surface is now covered by the machine-checked inventory,
differential/conformance corpus, native fallback assertions, portable
macOS/Linux execution, race detection, UBSan, and ASan. The macOS ASan run
uses the Homebrew LLVM compiler/runtime because the older Apple clang runtime
aborted before generated `main`; that toolchain limitation is no longer a
blocking validation gap.

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

The following checks pass on the maintained macOS toolchain and in the local
Linux/arm64 Colima container:

```text
CC=/opt/homebrew/opt/llvm/bin/clang GOCACHE=/tmp/osiris-final-mac-cache CGO_ENABLED=1 go test ./... -count=1 -timeout=600s
GOCACHE=/tmp/osiris-go-cache go vet ./...
GOCACHE=/tmp/osiris-go-cache go build ./...
GOCACHE=/tmp/osiris-race-cache CGO_ENABLED=1 go test -race ./internal/osiris -run 'TestCBackendMissionControlShowcaseParity|TestCBackendRequiredNativeSurfaceParity|TestConformanceCorpus|TestDifferentialHarness|TestBackendCapabilityInventoryIsCompleteAndUnique|TestRunVMPublicPathHasNoInterpreterDelegation|TestNativeCompilerHasNoEvaluatorFallback|TestCBackendPortableCompilationFlags|TestCBackendSanitizerFlagsAreCacheKeyed' -count=1 -timeout=600s
OSIRIS_C_SANITIZERS=undefined GOCACHE=/tmp/osiris-ubsan-cache CGO_ENABLED=1 go test ./internal/osiris -run 'Test(CBackendMissionControlShowcaseParity|CBackendCodecCrossDirectionParity|CBackendDeferredTaskAndFailureParity|CBackendStructuredResultFailureParity|CBackendRequiredTextUnicodeParity|CBackendRequiredTimeBoundaryParity|CBackendRequiredModuleErrorParity|CBackendCryptoVectorsParity|CBackendRequiredNativeSurfaceParity|ConformanceCorpus|CBackendPortableCompilationFlags|CBackendSanitizerFlagsAreCacheKeyed)' -count=1 -timeout=600s
CC=/opt/homebrew/opt/llvm/bin/clang OSIRIS_C_SANITIZERS=address GOCACHE=/tmp/osiris-final-asan-mac-cache CGO_ENABLED=1 go test ./internal/osiris -run 'Test(CBackendMissionControlShowcaseParity|CBackendCodecCrossDirectionParity|CBackendDeferredTaskAndFailureParity|CBackendStructuredResultFailureParity|CBackendRequiredTextUnicodeParity|CBackendRequiredTimeBoundaryParity|CBackendRequiredModuleErrorParity|CBackendCryptoVectorsParity|CBackendRequiredNativeSurfaceParity|ConformanceCorpus|CBackendPortableCompilationFlags|CBackendSanitizerFlagsAreCacheKeyed)' -count=1 -timeout=600s
docker run --rm --platform linux/arm64 -v /Users/JuanSanchez/SphireProjects/Osiris:/src:ro -w / -e GOCACHE=/tmp/osiris-linux-full-cache-3 -e CGO_ENABLED=1 golang:1.26-bookworm sh -c 'cp -a /src /work && cd /work && go test ./... -count=1 -timeout=600s'
docker run --rm --platform linux/arm64 -v /Users/JuanSanchez/SphireProjects/Osiris:/src:ro -w /src -e GOCACHE=/tmp/osiris-linux-asan-cache -e CGO_ENABLED=1 -e OSIRIS_C_SANITIZERS=address golang:1.26-bookworm go test ./internal/osiris -run 'Test(CBackendMissionControlShowcaseParity|CBackendCodecCrossDirectionParity|CBackendDeferredTaskAndFailureParity|CBackendStructuredResultFailureParity|CBackendRequiredTextUnicodeParity|CBackendRequiredTimeBoundaryParity|CBackendRequiredModuleErrorParity|CBackendCryptoVectorsParity|CBackendRequiredNativeSurfaceParity|ConformanceCorpus|CBackendPortableCompilationFlags|CBackendSanitizerFlagsAreCacheKeyed)' -count=1 -timeout=600s
git diff --check
```

Normal native builds enforce the characterized mission-control SHA-256 on both
validated platforms. Instrumented builds enforce successful execution,
repeated deterministic output, and sanitizer cleanliness; they do not enforce
the production SHA-256 because O1 instrumentation can change the final
rounding digit of platform floating-point calculations.

## Acceptance criteria

- [x] Every targeted stable feature has interpreter/VM/native evidence or an
      explicit scoped unsupported contract.
- [x] The VM contains no hidden interpreter delegation for its stable claims.
- [x] Native compilation contains no evaluator, subprocess, or snapshot
      fallback.
- [x] Differential, conformance, sanitizer, docs, and full validation pass.

## Non-goals

- Do not claim complete WASM/GPU support for host-only APIs.
- Do not weaken stable semantics to match a narrower backend.
