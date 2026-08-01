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

### Wave 7 - broad native integration

109 is complete. It compiles the unchanged mission-control showcase after
adding parity-safe exceptional and deferred-task IR, missing C value families,
codecs, and the exact static-module members it uses.

### Wave 8 - static types and application data

Tasks 110–114 are complete: typed functions, product types, generics,
behavioral abstractions, iteration, and deterministic local state now have
their implementation, parity tests, and documentation evidence.

### Wave 9 - execution, resources, and interoperability

115 and 116 are ready. Task 117 follows task 116 and implements the
capability-gated FFI design from completed task 047.

### Wave 10 - application standard library

118, 119, 120, 121, and 122 follow the required cleanup, FFI, struct, and OS
foundations. Keep each module capability-gated, bounded, deterministic, and
portable.

### Wave 11 - project ecosystem and IDE

123 and 124 are ready. Task 124 follows a repository-oriented Go-style model,
with GitHub as the first complete provider and discovery separate from build
resolution. Task 125 follows the type-system and testing work so completion,
refactoring, hints, and test discovery use stable contracts.

### Wave 12 - explicit low-level access

126 follows tasks 114–117. It must reuse safe bounded buffers, typed views,
opaque handles, ownership, cleanup, and capability-gated FFI before admitting
any explicitly unsafe operation.

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
- Tasks 110–114 form a semantic sequence and should not be implemented in
  parallel.
- Tasks 115–117 all touch native compilation or the C ABI. Assign explicit
  file ownership if more than one is active.
- Tasks 118–122 can split by module only after their shared resource, FFI, and
  value-model dependencies are complete.
- Task 125 must consume the compiler APIs established by tasks 110–113 and the
  discovery model from task 123; it must not duplicate them inside the LSP.
- Task 126 may not weaken ordinary value safety to simplify FFI. Unsafe access
  remains a separately imported, capability-gated, auditable boundary.

## Contents

- `INDEX.md` and `index.tsv`: authoritative status, dependency, owner, and
  next-step metadata for tasks 001–049 and 100–126.
- `completed/`: finished task records with validation evidence.
- Tasks 100–110 are under `completed/`.
- Tasks 111–126 are the scoped general-purpose maturity backlog derived from
  `osiris-language-assessment.md`.
- `archive/roadmaps/`: preserved historical phase breakdowns and design prose;
  they are not implementation tasks.
- `roadmaps/`: archived source text roadmaps.
