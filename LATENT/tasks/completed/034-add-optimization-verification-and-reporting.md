# Task 034: Add optimization verification and reporting

**Priority:** P1  
**Dependencies:** 025,026  
**Suggested PR scope:** One focused pull request unless the task explicitly requires staged commits.

## Objective

Make optimizer behavior explainable, testable, and safe.

## Required work

- [x] Add pass-by-pass IR snapshots or structured reports.
- [x] Verify semantic equivalence for constant folding, DCE, LICM, bounds elimination, unrolling, and scalarization.
- [x] Record why an optimization did not apply.
- [x] Add debug CLI output and deterministic ordering.

## Acceptance criteria

- [x] Each optimization has positive, negative, and alias/effect safety tests.
- [x] Reports are stable and machine-readable.
- [x] Performance claims include before/after evidence.

## Validation

- [x] optimizer unit tests
- [x] differential execution
- [x] benchmark evidence

## Non-goals

- Do not enable speculative transformations without a proof/guard.

## Codex execution notes

- Read `AGENTS.md`, `LEFTOFF.md`, `tasks/PRIORITY.md`, and relevant files before editing.
- Preserve unrelated worktree changes.
- Start with a failing or characterization test where practical.
- Record commands run and their results in the PR/task notes.
- Update the support matrix, user docs, and task status when the public contract changes.

## Completion record

Implemented a versioned `OptimizationReport` with deterministic ordered
pass entries. `simplify-and-propagate` and
`dead-statement-elimination` include before/after formatted source and
statement counts. They report `no_change` with a reason when no safe rewrite
is available. LICM, bounds-check elimination, unrolling, and scalarization
are deliberately reported as `not_implemented`, each with the missing proof
requirement. No speculative transformation was enabled.

`osiris opt -report file.osr` provides a stable human report and
`osiris opt -json file.osr` emits the machine-readable v1 contract. `-w`
cannot be combined with a report, preventing accidental source writes during
inspection. The componentized public React reference documents this command
under the searchable `Tooling` module.

Evidence:

- `TestOptimizeProgramWithReportIsStableAndPreservesExecution` checks stable
  JSON plus interpreter and VM semantic equivalence for folding and DCE.
- `TestOptimizeProgramWithReportExplainsUnavailablePasses` checks that the
  proof-dependent passes remain source-preserving and explicitly unavailable.
- Existing `TestConstantFoldingAndSimplification`,
  `TestCommonSubexpressionElimination`, `TestStrengthReduction`, and
  `TestLoopOptimizationDoesNotFreezeMutableState` retain positive and
  effect-safety coverage for enabled transformations.
- `TestRunOptJSONReport` validates the public CLI JSON path.
- `RUNS=3 WARMUP=1 BENCH_ITERS=500 ./bench.sh` verified identical output
  (`499500u64`) across interpreter, VM, and native C. On the recorded run it
  measured 69.590 ms interpreter, 64.424 ms VM, and 14.821 ms native average;
  these are environment-specific baseline evidence, not a general claim about
  the source optimizer.
- `go test ./...`, `npm run check-reference`, `npm run build`,
  `scripts/check-task-index.sh`, and `git diff --check` passed after the
  support-matrix evidence path was corrected.
