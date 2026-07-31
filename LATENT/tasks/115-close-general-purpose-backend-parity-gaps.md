# Task 115: Close general-purpose backend parity gaps

**Priority:** P1
**Dependencies:** 025, 026, 028, 034, 109
**Status:** Ready

## Objective

Make the public VM and native C compiler complete for the existing stable
general-purpose language surface, with no interpreter delegation or
compile-time evaluation fallback.

## Scope

- Derive the exact target set from `newdocs/src/support-matrix.json`, the
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

## Acceptance criteria

- [ ] Every targeted stable feature has interpreter/VM/native evidence or an
      explicit scoped unsupported contract.
- [ ] The VM contains no hidden interpreter delegation for its stable claims.
- [ ] Native compilation contains no evaluator, subprocess, or snapshot
      fallback.
- [ ] Differential, conformance, sanitizer, docs, and full validation pass.

## Non-goals

- Do not claim complete WASM/GPU support for host-only APIs.
- Do not weaken stable semantics to match a narrower backend.
