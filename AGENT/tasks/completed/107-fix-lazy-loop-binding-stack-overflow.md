# Task 107: Eliminate lazy loop-binding stack overflow

**Priority:** P1  
**Dependencies:** 004, 025, 043, 044  
**Status:** Complete

## Objective

Make large iterative Osiris loops safe when a lazy binding shadows a value from
the preceding iteration. The interpreter and the public `vm` command must
evaluate deep deferred dependency graphs without growing the Go call stack in
proportion to the iteration count.

Preserve the documented language rule that lexical bindings are lazy. The fix
must not turn every loop-body `let` into eager evaluation or otherwise make an
unused failure, task, or effect observable.

## Confirmed defect

The original `benchmarks/native-float-arithmetic.osr` and
`benchmarks/native-narrow-integers.osr` programs are valid iterative workloads.
They are not recursively calling an Osiris function.

On 2026-07-30, both original programs reproduced the failure after replacing
their `1000000` loop bound with `200000`:

```text
runtime: goroutine stack exceeds 1000000000-byte limit
fatal error: stack overflow
...
(*evaluator).fingerprint ... runtime.go:342
(*evaluator).fingerprint ... runtime.go:356
...226627 frames elided...
```

Reproduction environment:

- macOS Darwin 25.5.0 on arm64;
- Go 1.26.1;
- Apple clang 15.0.0.

The narrow-integer program's native executable completed and emitted
`2820030816u32`. The original float program's native executable completed and
emitted `166666f64`. Native IR correctly represents the accumulators as SSA
block parameters; no native evaluator fallback is involved.

## Root cause

Interpreter bindings store an expression together with the lexical scope in
which it was declared. Repeated loop shadowing such as:

```osiris
let total = total + i
```

builds a deferred chain in which each `total` refers to the preceding
iteration's `total`. Because the accumulator is not demanded inside the loop,
the final `emit total` encounters a dependency graph whose depth is
proportional to the iteration count.

`evaluator.fingerprint` follows identifiers into their binding expressions
recursively. The reproduced trace alternates between `identExpr` and
`binaryExpr` fingerprint frames until the Go runtime's one-gigabyte stack limit
is reached. Fixing only fingerprint construction is insufficient if ordinary
evaluation subsequently walks the same deferred expression graph recursively.

`Runtime.RunVM` currently detects control flow and deliberately delegates the
whole program to `Session.ExecProgram`. The `vm` command therefore exposes the
same interpreter defect; it does not currently execute these loops as VM
bytecode.

## Scope

### Required

- Bound host-stack use while fingerprinting and evaluating deep lazy binding
  dependency graphs.
- Preserve deterministic fingerprints, memoization, cycle handling, error
  propagation, source locations, cancellation, and concurrent evaluation
  behavior.
- Preserve lexical shadowing and closure capture. A closure created during an
  earlier iteration must continue to observe the bindings it originally
  captured.
- Exercise the public interpreter and `Runtime.RunVM` paths. The VM test and
  completion record must explicitly state that control flow still uses the
  interpreter fallback unless this architecture changes independently.
- Keep the existing native SSA loop representation and generated-C behavior.
- Restore `benchmarks/native-float-arithmetic.osr` to a checksum-only timing
  workload by removing the temporary per-iteration debugging emissions. Do
  this as an explicit task change, not as an unrelated worktree cleanup.

### Semantics that must not change

- An unused lazy binding is not evaluated.
- An unused binding that would fail does not produce an error.
- Declaring an unused `spawn`, I/O, or other effectful expression does not
  perform that effect.
- Fixed-width arithmetic retains its wrapping/checked behavior and visible
  width.
- Floating arithmetic and shortest-roundtrip output retain interpreter/native
  parity.
- `break`, `continue`, nested loops, functions, and closures retain their
  current scope behavior.

## Implementation guidance

Choose the smallest coherent representation or evaluator change that fixes
both recursive phases. Suitable designs may use an explicit work stack,
trampolining, memoized thunks, or another bounded-stack graph walk. Do not
paper over the crash by increasing Go's stack limit, reducing benchmark loop
bounds, periodically forcing arbitrary bindings, or catching a fatal runtime
overflow.

Avoid global eager evaluation. It would contradict `LANG.txt` and could expose
previously unused failures or effects. Be conservative when compacting scope
or dependency state: old scopes may remain reachable through closures,
function values, pending evaluations, or fingerprint cache keys.

If the implementation introduces a new evaluator state machine or thunk type:

- define ownership and synchronization explicitly;
- ensure cancellation is checked during long graph walks;
- do not hold evaluator mutexes while executing user expressions or waiting on
  another promise;
- retain stable cycle detection and avoid turning cycles into deadlocks;
- document why cached values and errors are safe to reuse.

Implementing bytecode control-flow execution is not required by this task.
That would be a separate architectural change. This task must nevertheless
prove that the public `vm` command no longer overflows while it uses the
documented fallback.

## Required tests

Add focused tests before implementation where practical.

1. A subprocess regression must evaluate a large lazy loop-carried accumulator
   under a deliberately bounded Go stack and exit normally. Use a subprocess
   because a Go fatal stack overflow cannot be recovered inside the test
   process.
2. Exercise at least:
   - a default integer accumulator;
   - a wrapping `u32` accumulator;
   - an `f64` accumulator with an intermediate binding;
   - two independent loop-carried bindings;
   - a nested-loop or closure-capture case that retains older scopes.
3. Run equivalent public interpreter and `Runtime.RunVM` assertions. State in
   the test name or comment when VM control flow is using the interpreter
   fallback.
4. Prove unused lazy expressions remain unevaluated:
   - an unused expression that would return an error;
   - an unused task/effect expression with an observable counter or fixture;
   - an unused self-reference/cycle consistent with the existing cycle
     contract.
5. Prove demanded errors retain their structured kind, message, source span,
   and deterministic behavior after the bounded-stack change.
6. Add a cancellation test for a long deferred dependency walk without using
   an unbounded sleep.
7. Keep existing evaluator memoization, task, loop, closure, interpreter/VM
   parity, native generated-C, and race tests passing.

Do not place a million-iteration case in the ordinary unit-test path if it
makes the suite unreasonably slow. Use a smaller bounded-stack subprocess test
for regression coverage and reserve the large workloads for the recorded
benchmark/parity validation.

## Benchmark and generated-artifact evidence

After focused tests pass:

1. Inspect `osiris ir` for both canonical benchmark programs and record that
   `i` and the accumulator remain SSA block parameters.
2. Inspect representative generated C and prove compilation does not execute
   the program to manufacture output.
3. Run both workloads through `bench.sh` with `RUNS=1`, `WARMUP=0`, and a
   deliberately large recorded `BENCH_ITERS` that previously reproduced the
   overflow. Confirm interpreter/VM/native checksum parity.
4. Record OS, architecture, Go version, C compiler/version, workload revision,
   loop count, elapsed samples, and peak-memory observations available from the
   host. These runs are correctness and scale evidence, not a performance
   improvement claim.
5. Retain the ordinary multi-sample performance process from
   `docs/development/BENCHMARKS.md`; do not replace its historical baselines
   with the one-run correctness evidence.

## Acceptance criteria

- [x] Deep lazy loop-carried dependency graphs use bounded Go call-stack space
      during both fingerprinting and value evaluation.
- [x] The original integer and float benchmark programs complete through
      interpreter, public VM, and native paths at the recorded large loop
      count with identical visible values.
- [x] Lazy unused-error and unused-effect behavior is unchanged and covered by
      focused tests.
- [x] Shadowing, closure capture, nested control flow, cancellation, cycle
      detection, errors, and numeric width/style retain their contracts.
- [x] Native IR still uses SSA loop parameters and generated C contains no
      compile-time evaluator output snapshot.
- [x] The float benchmark is again a checksum-only workload accepted by
      `bench.sh`.
- [x] Focused tests pass normally and under `-race`.
- [x] `go test ./...`, `go test -race ./...`, `go vet ./...`,
      `git diff --check`, task-index validation, and the recorded benchmark
      commands pass.
- [x] This task contains the final design decision, raw reproduction/parity
      evidence, and any deliberately deferred memory or VM architecture work
      before it moves to `tasks/completed/`.

## Non-goals

- Do not make all Osiris bindings eager.
- Do not weaken or remove fingerprint determinism, memoization, cancellation,
  structured errors, or cycle detection to obtain a passing benchmark.
- Do not merely lower the benchmark iteration count or raise the host stack
  limit.
- Do not implement general VM bytecode control flow unless separately scoped
  and justified.
- Do not redesign native SSA lowering when its current output is already
  correct.

## Completion record — 2026-07-30

### Design decision

Deep identifier/unary/binary fingerprinting now uses an explicit continuation
stack in `evaluator_iterative.go`. Each frame owns only its expression, immutable lexical
scope, stage, and completed child results. The walk produces the same
`hashParts` structure as the former recursive path and records dependency
depth beside the existing fingerprint memo. Other expression families keep
the established recursive implementation and may appear as opaque leaves;
there is no eager binding rewrite.

Value evaluation switches to a matching explicit continuation stack only when
the recorded simple-expression depth reaches 128. Shallow programs retain
their existing scheduler and error-shaping path. The deep walker preserves
short-circuiting, unary and numeric behavior, identifier lookup, constants,
builtins, memoized values, and lexical scopes. A demand-local active-key set
detects demanded cycles deterministically. Cancellation is checked every 256
continuation steps.

Continuation frames and cycle sets are owned by one demand and never shared.
The evaluator's existing `fpMu` and `mu` protect only fingerprint/depth and
value-cache map accesses; neither mutex is held while evaluating an expression,
executing a builtin, or waiting for a promise. Successful immutable `Value`
results remain safe to reuse under their deterministic fingerprint; errors
are not cached. Concurrent readers continue to rendezvous through the existing
promise and focused `-race` coverage proves eight simultaneous demands.

This preserves the language's lazy rule: loop construction still records
expressions and prior scopes, and no unused binding is forced. Focused evidence
covers an unused divide-by-zero, unused `spawn` with an empty task trace,
unused missing name, and unused recursive function cycle. Demanded structured
errors retain kind, message, source, and call span. Integer, wrapping `u32`,
`f64` with an intermediate binding, two independent accumulators, nested
loops, preceding-iteration closure capture, cancellation, cycle detection,
and memoization pass through both public paths. `Runtime.RunVM` still
deliberately delegates control-flow programs to `Session.ExecProgram`; this
task does not claim new VM loop bytecode.

The bounded-stack subprocess first failed against the old implementation in
both modes with:

```text
runtime: goroutine stack exceeds 524288-byte limit
fatal error: stack overflow
(*evaluator).fingerprint ... runtime.go:299
(*evaluator).fingerprint ... runtime.go:342
(*evaluator).fingerprint ... runtime.go:356
```

After the repair, both modes complete the 6,000-iteration fixture under the
same 512 KiB maximum Go stack and emit `17997000u64`.

### Native and scale evidence

Environment:

```text
Darwin 25.5.0 arm64
go version go1.26.1 darwin/arm64
Apple clang version 15.0.0 (clang-1500.0.40.1)
```

Canonical workload revisions:

```text
fa5c494c909838ccd54812853fa7a29ecab890610ca36f467ac7d579b3cb6c0f  benchmarks/native-narrow-integers.osr
ca6d329fcb0e5c99a463cd94f5a2e62edc258458e431e86e6db1ee13c37bb8e4  benchmarks/native-float-arithmetic.osr
```

`osiris ir` at `BENCH_ITERS=200000` retained `i` and `total` as block
parameters on the narrow loop header and retained `i`, `ratio`, and `total` on
the float loop header. Task-specific generated-C assertions and manual
inspection prove both artifacts contain `block_main_while_header`,
`block_main_while_body`, a runtime backedge, arithmetic, and runtime printing;
neither contains the computed checksum as a captured literal.

Raw one-run correctness/scale samples:

```text
RUNS=1 WARMUP=0 BENCH_ITERS=200000 \
  WORKLOAD=benchmarks/native-narrow-integers.osr ./bench.sh
checksum: 2820030816u32
interpreter 3744.428 ms; vm fallback 3943.471 ms; native 13.285 ms
native compile 762.186 ms

RUNS=1 WARMUP=0 BENCH_ITERS=200000 \
  WORKLOAD=benchmarks/native-float-arithmetic.osr ./bench.sh
checksum: 166666f64
interpreter 6126.475 ms; vm fallback 6397.136 ms; native 17.457 ms
native compile 751.524 ms
```

macOS `/usr/bin/time -l` peak-memory observations for the same generated
200,000-iteration sources:

```text
narrow: interpreter 1,783,447,552 B; vm fallback 1,781,104,640 B;
        native 1,392,640 B maximum RSS
float:  interpreter 2,821,701,632 B; vm fallback 3,130,212,352 B;
        native 1,392,640 B maximum RSS
```

These are correctness and scale observations, not a speed or allocation
improvement claim. Immutable per-iteration scopes remain reachable by lazy
bindings and captured closures, so large hosted runs still consume substantial
memory. Scope/thunk compaction requires a separate proof of closure,
effect/error, and cache-key safety and is deliberately deferred. General VM
control-flow bytecode also remains separate. The ordinary multi-sample
benchmark policy and historical baselines are unchanged.

The temporary per-iteration float emissions were removed, restoring the
canonical checksum-only workload. `check-benchmark-workloads.sh` passes.

### Validation

```text
go test ./internal/osiris -run '^TestLazy' -count=1
ok github.com/sphireinc/osiris/internal/osiris

go test -race ./internal/osiris -run '^TestLazy' -count=1
ok github.com/sphireinc/osiris/internal/osiris

go test ./...
ok github.com/sphireinc/osiris/cmd/osiris
ok github.com/sphireinc/osiris/internal/osiris

go test -race ./...
ok github.com/sphireinc/osiris/cmd/osiris
ok github.com/sphireinc/osiris/internal/osiris

go vet ./...
(no output)

./scripts/check-benchmark-workloads.sh
Benchmark workload interpreter/VM parity passed.

git diff --check
(no output)

./scripts/check-task-index.sh
Task index checks passed.
```
