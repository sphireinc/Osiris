# Task 151: Stabilize WASM libraries, I/O, and tooling parity

**Priority:** P1
**Dependencies:** 150,118,119,120,121,122,123,126,137
**Status:** Complete

## Objective

Complete the WASM standard-library and capability ABI so all public modules have
stable, tested behavior in a declared WASM profile.

## Exact scope and ownership

- Own WASM host-import adapters, module lowering, artifact manifests, and WASM
  integration tests.
- Cover `std/text`, `std/math`, `std/crypto`, `std/signal`, `std/list`,
  `std/time`, `std/regex`, `std/json`, `std/i18n`, `std/db`, `std/telemetry`,
  `std/testing`, `std/unsafe`, `std/net`, and `std/os` according to explicit
  capability profiles.
- Implement deterministic virtual filesystem/environment/process/clock/network
  providers or a stable WASM host ABI; no accidental browser/node dependency.
- Make test execution, artifact inspection, tracing, diagnostics, and source
  maps stable and reproducible.

## Matrix transitions

Promote WASM stdlib, I/O, networking, OS, testing, unsafe, tooling, and scope-
cleanup rows to `stable`.

## Parallelism

This is the final WASM task. It may run concurrently with native host and GPU
tasks, but not with another task editing WASM host-import contracts.

## Completion evidence

- `WASMCapabilityProfile`, `DefaultWASMCapabilityProfile`, and
  `CompileWASMWithProfile` carry sorted module and capability declarations in
  the artifact ABI. The default runner uses `NewTestRuntime` and deterministic
  virtual providers rather than ambient host state.
- The profile/runtime parity corpus covers text, math, crypto, signal, list,
  time, regex, JSON, i18n, embedded DB, telemetry, testing, and fake OS-file
  access. Network and ambient identity reject before emission unless an
  explicit profile allows them.
- `TestWASMDeterministicStdlibAndCapabilityProfile` exercises the supported
  providers; profile rejection, repeated-run, cancellation, and ownership
  tests are green.
- `docs/design/WASM.md` documents the profile, fake providers, capability
  declarations, rejection boundary, and deterministic testing contract.
