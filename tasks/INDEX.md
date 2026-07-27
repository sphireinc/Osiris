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

- **Active:** task 049 establishes a measured TCP baseline and only then tunes
  the completed task-011 TCP correctness and task-012 polling foundations.
  Task 014's host-error mapping, deterministic cancellation, safe native errno
  metadata, and recursive redaction contract are complete.
- **Documentation:** tasks 039 and 040 are complete. `newdocs/` is the
  canonical componentized React reference and owns public API/reference data,
  examples, and the support matrix. `docs/` retains frozen compatibility and
  durable design material only.
- **Native stream:** task 004 remains the historical umbrella for the
  in-progress C lowering work. Use `index.tsv` dependency/status fields rather
  than this summary when selecting later native tasks.

## Completed evidence

Tasks 019–022 live under `tasks/completed/`. Their completion records name
commands, tests, and documentation evidence. In particular, task 022’s public
contract is `newdocs/src/support-matrix.json`, validated by
`internal/osiris/support_matrix_test.go`.

## Historical material

The large async, error, polling, and type phase files are preserved under
`tasks/archive/roadmaps/`. They are design source, not an implementation queue.
Their extracted actionable work is represented by tasks 006–018 and 024–049.
