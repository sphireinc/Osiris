# Deferred FFI boundary design

Source: CHECKLIST.md, IMPORTING-C-CODE.txt, and IMPORTING-GO-CODE.txt.

FFI is explicitly deferred. Reconcile the C and Go interop notes with actual
ABI, ownership, error, versioning, and sandbox rules before adding public syntax.

Acceptance: an approved design document only; no public FFI syntax or dynamic Go
package loading is introduced.
