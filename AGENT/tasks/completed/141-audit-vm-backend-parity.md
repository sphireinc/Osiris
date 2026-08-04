# Task 141: Complete VM semantic parity

**Priority:** P1
**Dependencies:** 139,140
**Status:** Complete

## Objective

Make the VM an independent implementation of the complete language contract,
with no interpreter delegation or construct-specific semantic drift.

## Exact scope and ownership

- Own `internal/osiris/bytecode.go`, VM compiler/runner tests, and bytecode
  version fixtures.
- Cover all parser/interpreter fixtures: declarations, typed and generic
  calls, methods and trait witnesses, iterators, patterns, modules/macros,
  Result/Option/error control flow, tasks, cancellation, cleanup, streams, and
  stdlib calls.
- Verify register lifetime, closure capture, source spans, output ordering,
  cleanup edges, cancellation, and resource ownership.
- Add a differential harness that rejects VM fallback to interpreter execution.
- Do not alter parser, interpreter, native C, WASM, GPU, formatter, or LSP
  source except for shared fixture metadata.

## Acceptance criteria

- Interpreter and VM produce identical outputs, errors, cleanup effects, and
  cancellation outcomes for the complete conformance corpus.
- Bytecode cache/version behavior is updated only when required and tested.
- VM unsupported behavior is eliminated or is a stable, source-spanned
  diagnostic explicitly listed in the final matrix.
- Race-sensitive VM tests pass.

## Parallelism

This task owns VM files. It becomes independently assignable after tasks 139
and 140 complete and may then run concurrently with formatter, LSP, WASM, and
GPU work.

## Implementation evidence

- `internal/osiris/bytecode.go:vmRunner` carries a VM-local source-module cache
  and mutex through nested calls, closures, loops, parallel branches, timeout
  branches, and spawned tasks; VM imports no longer reuse interpreter-owned
  module values.
- `(*vmRunner).loadModuleVM`, `closeResource`, `run`, and `runVMCallable` keep
  source-module compilation, cancellation commits, built-in close handling,
  and user-defined `Closable.close` dispatch on the VM path.
- `(*Runtime).RunVM` initializes the isolated VM cache and synchronization state
  for every VM execution.
- `internal/osiris/vm_parity_test.go` and `internal/osiris/testdata/vm-parity/`
  cover language-contract differential execution, generic/trait/pattern/
  iterator behavior, source-spanned errors, cleanup, cancellation, source
  modules, concurrent imports, child-task module state, and no-interpreter
  fallback.

Validation: VM parity and no-fallback suites, VM race coverage, the complete
`internal/osiris` suite, `go test ./...`, and `go vet ./...` pass.
