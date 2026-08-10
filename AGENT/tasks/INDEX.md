# Current task index

`tasks/index.tsv` is the canonical, machine-checked work queue. It is ordered
by task number, which follows the audit waves; `status` and `depends_on` decide
what is actionable. Do not infer current status from an old checkbox list or a
roadmap filename.

## Metadata convention

Every implementation task has exactly one row with these fields:

| Field | Meaning |
| --- | --- |
| `id` | Three-digit stable task identifier. |
| `path` | Current repository-relative task file. |
| `status` | `active`, `ready`, `blocked`, `complete`, `superseded`, or `historical`. |
| `owner` | `unassigned` until a person or agent takes the task; update it while active. |
| `depends_on` | Comma-separated task IDs, or `-` when none. A dependency is satisfied only when its status is `complete`, `superseded`, or `historical` with an explicit successor. |
| `evidence_or_next` | Completion evidence, successor, or the next gating condition. |

Only one implementation task should normally be `active`. A `ready` task has
all recorded dependencies satisfied and is the next selection candidate.

## Current selection

- **Active focus:** Native C and LSP stabilization. Task 160 semantic IDE
  parity is complete; task 162 is the next independently assignable Native C
  core/data slice, followed by tasks 161, 163, and 168. Task 154 is the
  Native C/LSP-only gate.
- **Ready:** task 162 is the independently assignable Native C priority slice.
- **Blocked:** task 154 remains blocked until the Native C/LSP closure tasks
  complete. The all-backend gate is task 310; WASM/GPU/environment work is
  deliberately blocked in the 300-series queue.
- **Deferred queue:** tasks 300–308 and 400–403 contain WASM, GPU, and
  alternate-platform work. They retain their implementation requirements and
  cannot promote support-matrix cells while deferred.
- **Complete records:** tasks 001–049, 100–126, 134, 138–153, and 160 have
  completion records; Native C/LSP closure and the remaining release gates
  remain open.
- **Complete (P2 native integration):** task 109 adds the exceptional/async
  IR, native value ABI, codecs, scheduler, and exact static-module slice
  required to compile the unchanged mission-control showcase into a
  standalone executable.
- **Documentation:** tasks 039 and 040 are complete. In this checkout `docs/`
  is the canonical public reference and owns API/reference data, examples,
  navigation, and the support matrix; `docs/design/` owns durable contracts.
- **Coordination:** completed task 100 preserves task 104's audited global
  catalog and exact reference indexes. Task 101's duration constants remain qualified
  under `std/time`; task 104's same-spelled globals are quantity values.
  Completed task 106 consumes task 105's ordered arrays and shared anchor
  model. Task 107 is independent of the reference-navigation sequence and is
  complete. Task 108 completed the sequential `emit` contract without
  reintroducing eager binding or per-iteration output into the arithmetic
  benchmarks. Task 109 completed the native integration target while retaining
  deferred task evaluation and the unchanged showcase fixture.

The TCP optimization records 126–133 are under
`tasks/completed/optimizations/`. They document the completed TCP lifecycle
batch. Their numeric prefixes are retained for provenance, but are not added
as separate index rows because the optimization record named 126 shares its
prefix with the canonical unsafe-boundary task 126.

## General-purpose maturity sequence

The tasks derived from `osiris-language-assessment.md` are organized as
reviewable increments rather than one open-ended language rewrite:

1. **Type and data model:** 110–114, using structs and Go-style field tags for
   product types.
2. **Execution and interoperability:** 115–117.
3. **Application standard library:** 118–122.
4. **User ecosystem and developer experience:** 123–125, with task 136
   extending task 123's test discovery and package workflow.
5. **Explicit low-level boundary:** 126 follows the safe FFI, ownership,
   cleanup, state, and backend foundations.
6. **REPL and backend parity:** 138–153 established the initial parity
  streams. Tasks 160–163 and 168 are the current Native C/LSP stabilization
  lane, with task 154 as its focused gate. Tasks 300–308 and 400–403 defer WASM, GPU, and
  environment-gated evidence; task 310 remains the final all-backend
  `stable` gate.

Task 110 now supplies the checker and typed callable foundation. Task 111 is
the next type/data-model increment. Tasks 115, 116, 123, and 124 remain
independent, subject to the file-ownership cautions in `README.md`. Task 124
uses repository module paths, Git tags, and GitHub as its first complete
provider rather than requiring a central Osiris package registry.

## Completed evidence

Tasks 001–049, 100–110 live under `tasks/completed/`. Their
completion records name commands, tests, and documentation evidence. In
particular, task 022’s public contract is `docs/src/support-matrix.json`,
validated by `internal/osiris/support_matrix_test.go`.

## Historical material

The large async, error, polling, and type phase files are preserved under
`tasks/archive/roadmaps/`. They are design source, not an implementation queue.
Their extracted actionable work is represented by tasks 006–018 and 024–049.
