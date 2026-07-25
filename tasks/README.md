# Osiris Codex task pack

This pack is the current, audit-driven work queue. The root `AGENTS.md` is the
repository policy. Select work from `INDEX.md` and `index.tsv`, not from old
checkboxes or roadmap filenames. Assign one task at a time unless file
boundaries and dependencies make parallel work safe.

## Suggested execution order

### Wave 0 - baseline and reproducibility

019, 020, 021, 022, 023, 024 (019–022 complete; 023 active; 024 next)

### Wave 1 - correctness and native ABI

025, 026, 028, 032, 027, 029, 030, 031, 033, 034

### Wave 2 - product tooling and distribution

035, 036, 037, 038, 039, 040, 041, 042

### Wave 3 - scale, security, and future language work

043, 044, 045, 046, 047, 048

## Parallelism cautions

- Do not parallelize tasks that simultaneously modify `cbackend_native.go`.
- Do not parallelize module tasks 033/036/037 without explicit ownership.
- Documentation tasks 039/040 can start after task 022 defines the support matrix.
- Release task 041 should wait for the toolchain and artifact-versioning tasks.

## Contents

- `INDEX.md` and `index.tsv`: authoritative status, dependency, owner, and
  next-step metadata for tasks 001–048.
- `completed/`: finished task records with validation evidence.
- `tasks/`: current audit task files (019–048).
- `archive/roadmaps/`: preserved historical phase breakdowns and design prose;
  they are not implementation tasks.
- `roadmaps/`: archived source text roadmaps.
