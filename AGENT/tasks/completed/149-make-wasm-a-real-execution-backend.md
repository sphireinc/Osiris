# Task 149: Replace WASM snapshots with a real execution backend

**Priority:** P1
**Dependencies:** 139,140,141,036
**Status:** Complete; scalar instruction-level execution is active in the
`scalar-i64-v1` module mode, including cyclic and acyclic SSA branch/join
graphs, mutable assignments, null bookkeeping values, edge parameter copies,
and generated instruction-budget enforcement. Aggregate, closure, and
host-provider forms continue through the bounded `payload-v1` compatibility
mode until tasks 156-157 provide their dedicated lowering and import ABIs.

## Objective

Turn `CompileWASM` from interpreter-evaluated JSON/WAT snapshots into a real,
bounded WebAssembly artifact backend with a stable execution ABI.

## Exact scope and ownership

- Own `internal/osiris/wasm.go`, new WASM lowering/runtime files, WASM artifact
  tests, and WASM CLI integration.
- Define a versioned module ABI for values, memory ownership, result/error
  transport, cancellation, limits, imports, and deterministic execution.
- Lower typed scalar/control-flow IR to valid WASM instructions and execute the
  generated artifact in a maintained runtime or a deterministic test runner.
- Reject unsupported IR before emission; never execute source through the
  interpreter while labeling the result WASM.
- Add binary validation, malformed artifact, resource limit, and repeated-run
  tests.

## Matrix transitions

Establish the stable WASM foundation required by all language and library rows.

## Parallelism

This task owns WASM files and may run concurrently with native, GPU, formatter,
and LSP tasks. Later WASM tasks depend on its ABI.

## Completion evidence

- `CompileWASM` emits a valid versioned binary with an `osiris.wasm.v1` custom
  section containing lowered IR, ABI metadata, limits, ownership, and
  cancellation policy; scalar artifacts execute their arithmetic, cyclic and
  acyclic branch/join control flow, generated instruction accounting, and
  result transport inside wazero, while the deterministic provider remains the
  bounded compatibility runner for forms outside the scalar lowering scope.
- `wasm_provider.go`, `wasm_runtime.go`, and `wasm_test.go` cover binary
  validation, malformed sections, bounded instructions/memory/outputs,
  repeated deterministic execution, source-spanned rejection, and
  cancellation.
- `docs/design/WASM.md` documents the ABI, profile, limits, ownership,
  cancellation, imports, and unsupported forms.
