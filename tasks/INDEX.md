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

- **Active:** none. Tasks 001–049 and 100–104 are complete.
- **Ready:** task 105 defines and enforces canonical reference-entry ordering.
- **Blocked:** task 106 adds `#page/entry` deep links and per-page tables of
  contents after task 105 establishes the order those links display.
- **Documentation:** tasks 039 and 040 are complete. `newdocs/` is the
  canonical componentized React reference and owns public API/reference data,
  examples, and the support matrix. `docs/` retains frozen compatibility and
  durable design material only.
- **Coordination:** completed task 100 preserves task 104's audited global
  catalog and exact reference indexes. Task 101's duration constants remain qualified
  under `std/time`; task 104's same-spelled globals are quantity values.
  Task 106 must consume task 105's ordered arrays rather than defining another
  comparator in the navigation component.

## Completed evidence

Tasks 001–049 and 100–104 live under `tasks/completed/`. Their
completion records name commands, tests, and documentation evidence. In
particular, task 022’s public contract is `newdocs/src/support-matrix.json`,
validated by `internal/osiris/support_matrix_test.go`.

## Historical material

The large async, error, polling, and type phase files are preserved under
`tasks/archive/roadmaps/`. They are design source, not an implementation queue.
Their extracted actionable work is represented by tasks 006–018 and 024–049.
