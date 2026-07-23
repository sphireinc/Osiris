# Exhaustive remaining text-roadmap items

These files preserve every remaining unchecked item from the original root
`.txt` roadmaps. They are archival source material. The active work is split
into 46 semi-grouped task files directly under the parent `tasks/` directory.

| Roadmap | Open items | Focused task files |
| --- | ---: | --- |
| `ASYNC-PLAN-CHK.txt` | 55 | `async-phase-*.md` |
| `ERROR-CHK.txt` | 202 | `errors-phase-*.md` |
| `POLLING-CHK.txt` | 177 | `polling-phase-*.md` |
| `TYPECHK.txt` | 41 | `types-section-*.md` |

The native compiler is the first cross-cutting priority. Any roadmap item that
requires native C emission, an ABI, hot-path performance, async lowering, FFI,
or platform runtime support must follow the C-lowering and runtime-ABI tasks
before it is marked complete.
