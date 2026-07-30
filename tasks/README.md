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

043, 044, 045, 046, 047, 048, 049

### Wave 4 - standard-library and constant expansion

100, 101, 102, 103, 104

### Wave 5 - runtime correctness and output semantics

107, then 108

### Wave 6 - reference ordering and navigation

105, then 106

## Parallelism cautions

- Do not parallelize tasks that simultaneously modify `cbackend_native.go`.
- Do not parallelize module tasks 033/036/037 without explicit ownership.
- Documentation tasks 039/040 can start after task 022 defines the support matrix.
- Release task 041 should wait for the toolchain and artifact-versioning tasks.
- TCP tuning task 049 must wait for task 011's cancellation/error contract and
  task 012's portable polling boundary; benchmark before claiming a gain.
- Task 100 must preserve task 104's exact named-constant and complete-surface
  audits. Tasks 101 and 104 have finalized qualified duration constants versus
  global measurement quantities.
- Task 106 depends on task 105. Its table of contents must reuse the canonical
  ordered entry arrays rather than creating an independent UI sort.
- Task 107 is independent of tasks 105 and 106 and takes priority as a
  confirmed runtime correctness defect. Do not make lazy bindings globally
  eager to fix its benchmark symptom.
- Task 108 depends on task 107 because both touch evaluator/block execution.
  It must use a separate small emission fixture and keep the arithmetic
  benchmarks checksum-only.

## Contents

- `INDEX.md` and `index.tsv`: authoritative status, dependency, owner, and
  next-step metadata for tasks 001–049 and 100–108.
- `completed/`: finished task records with validation evidence.
- repository-root tasks 105–108: canonical-reference ordering/navigation,
  the lazy loop-binding stack-overflow repair, and consistent synchronous
  `emit` semantics. Tasks 100–104 are under `completed/`.
- `archive/roadmaps/`: preserved historical phase breakdowns and design prose;
  they are not implementation tasks.
- `roadmaps/`: archived source text roadmaps.
