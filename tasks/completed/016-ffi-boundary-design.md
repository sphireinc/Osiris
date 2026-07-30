# Deferred FFI boundary design

Source: CHECKLIST.md, IMPORTING-C-CODE.txt, and IMPORTING-GO-CODE.txt.

FFI is explicitly deferred. Reconcile the C and Go interop notes with actual
ABI, ownership, error, versioning, and sandbox rules before adding public syntax.

Acceptance: an approved design document only; no public FFI syntax or dynamic Go
package loading is introduced.

## Completion record — 2026-07-28

Task 047 completed this design-only requirement. `docs/design/FFI-BOUNDARY.md`
defines a versioned, capability-gated, digest-pinned C ABI proposal with
explicit scalar and UTF-8 mappings, ownership/failure rules, and exclusions
for aggregates, callbacks, Go package loading, re-entrancy, and arbitrary
dynamic-library discovery. No public FFI syntax or dynamic Go loading was
introduced. Future implementation requires a separate approved feature task.
