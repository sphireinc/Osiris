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

- **Active:** task 011 defines the minimal TCP client/server capability on the
  completed resolution and scheduler contracts. Tasks 012–014 remain ready
  behind it.
- **Documentation:** tasks 039 and 040 are complete. `newdocs/` is the
  canonical componentized React reference, while `docs/` retains design and
  compatibility material only.
- **Native stream:** task 004 remains the historical umbrella for the
  in-progress C lowering work. Use `index.tsv` dependency/status fields rather
  than this summary when selecting later native tasks.

## Completed evidence

Tasks 019–022 live under `tasks/completed/`. Their completion records name
commands, tests, and documentation evidence. In particular, task 022’s public
contract is `docs/support-matrix.json`, validated by
`internal/osiris/support_matrix_test.go`.

## Historical material

The large async, error, polling, and type phase files are preserved under
`tasks/archive/roadmaps/`. They are design source, not an implementation queue.
Their extracted actionable work is represented by tasks 006–018 and 024–048.
