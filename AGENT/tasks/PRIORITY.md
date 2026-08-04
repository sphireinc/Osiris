# Implementation priority

The canonical priority/status source is `tasks/index.tsv`; this page is a
human shortcut and must agree with it.

## Now

No task is active. Task 111 is the next type/data-model increment; tasks 115,
116, and 123 remain independently ready. Tasks 001–049, 100–110, 124, 125,
and 136 are complete.

## Ready — general-purpose foundations

1. 136 extends task 123 with Go-style colocated test discovery while retaining
   explicit file and directory arguments.
2. 111 adds immutable structs with Go-style field tags, tuples, aliases, and
   nominal newtypes on task 110's completed static checker.
3. 115 closes the remaining stable VM/native C parity gaps.
4. 116 adds deterministic scope-bound cleanup and unlocks FFI and
   resource-owning libraries.
5. 123 adds an Osiris-native testing workflow.
6. 124 adds Go-style repository modules, GitHub-backed resolution, workspaces,
   separate package discovery, and supply-chain auditing without requiring a
   central registry.

## Blocked — follow-on work

- 112–114 depend on task 111 and the remaining type/data-model sequence.
- 117 depends on deterministic cleanup before owning foreign resources.
- 118–122 depend on the relevant cleanup, FFI, data-model, or OS foundations.
- 125 depends on the completed type system and language-native test model so
  its editor contracts are not built around temporary semantics.
- 126 depends on the safe buffer/handle, ownership, cleanup, FFI, mutation, and
  backend contracts. It is the explicit capability-gated unsafe boundary, not
  an ambient pointer facility.

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
