# Implementation priority

The canonical priority/status source is `tasks/index.tsv`; this page is a
human shortcut and must agree with it.

## Now — stabilize Native C and LSP

1. Task 160: complete semantic LSP parity.
2. Task 162: close Native C core, numeric, and data rows.
3. Task 161: complete workspace, refactoring, and colocated-test tooling after
   task 160.
4. Task 163: close Native C standard-library and host rows after task 162.
5. Task 168: close the LSP support-matrix column after tasks 160 and 161.
6. Task 154: promote only the Native C and LSP matrix columns after tasks
   160–163 and 168 have complete evidence.

Native C and LSP work must preserve the existing support-matrix states until
the focused gate has executable evidence. The global all-backend release gate
is intentionally separate.

## Deferred — WASM, GPU, and environment-gated evidence

Tasks 300–308 and 400 are deliberately lower priority while Native C and LSP are
stabilized:

- 300–302: WASM scalar, aggregate/closure, and host-import execution.
- 303–304: GPU device provider and device execution.
- 305–306: WASM matrix closure.
- 307–308: GPU matrix closure.
- 400: Windows/Linux/real-device evidence unavailable on the current machine.

## Final release gate

Task 310 closes every support-matrix cell only after task 154, tasks 300–308,
and task 400 are complete. Deferred tasks retain their full acceptance criteria;
they are not considered stable merely because the Native C and LSP columns
reach stable.

## Completed — native integration

1. 109 added the IR, C ABI, portable task runtime, exact values, codecs, and
   required static-module members needed to compile the unchanged
   mission-control showcase into a standalone executable.

## Completed — scoped surface expansion

Task 102 completed the bounded file, directory, and seekable-stream I/O
surface with shared runtime, native-gate, lifecycle, and documentation
evidence.

Completed task 100 preserved task 104's exact named-constant and
dual-reference audits.

## Completed foundations

Tasks 001–049 and 100–110 are complete and remain indexed for
provenance and acceptance evidence. Use `tasks/index.tsv` as the canonical
status/dependency source.
