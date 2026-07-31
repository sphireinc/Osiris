# Task 043: Create benchmark JSON and regression tracking

**Priority:** P2  
**Dependencies:** 024,025,034  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Turn performance work into reproducible data rather than anecdotal timing.

## Required work

- [x] Add machine-readable output to `bench.sh` or a Go benchmark driver.
- [x] Separate parse, compile, startup, and execution costs.
- [x] Record environment metadata and raw samples.
- [x] Define non-flaky alert thresholds for scheduled CI, not PR blocking.

## Acceptance criteria

- [x] Results are comparable across commits with caveats recorded.
- [x] Correctness parity is checked before timing.
- [x] Benchmark workloads cover scalar, text, modules, tensors, and tasks.

## Validation

- [x] benchmark parser tests
- [x] scheduled CI dry run

## Non-goals

- Do not compare unlike workloads or include compile time in native execution numbers.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

`bench.sh` emits schema-versioned JSON when `BENCH_JSON` is set. It records
raw samples, averages, workload/iteration/warmup metadata, host and Go
metadata, parity status, caveats, and separate native-build versus execution
costs. A smoke run parsed the JSON and confirmed two raw samples per path.

The scheduled/manual benchmark workflow uploads the scalar native baseline as
an artifact and verifies text, module, tensor, and task fixtures with the
reusable interpreter/VM parity script. `BENCHMARKS.md` defines comparable-run
caveats and a deliberately non-blocking 15%/15-sample repeat-confirmed alert
policy. Full Go tests, task-index checks, benchmark smoke, workload parity,
and diff checks passed during this task.
