# Task 108: Make `emit` consistent in sequential executable blocks

**Priority:** P1  
**Dependencies:** 004, 018, 025, 035, 040, 045, 107  
**Status:** Blocked on task 107  

## Objective

Make `emit` a predictable executable statement instead of a construct whose
validity changes between parsing, interpretation, macro expansion, and native
compilation.

Allow `emit` at any execution point in a synchronous Osiris program: top-level
statement sequences, loop bodies, function bodies, ordinary block
expressions, selected conditional and pattern arms, and `try`/`catch` blocks.
Preserve deterministic ordering and interpreter/VM/native parity. Keep module
initialization and concurrent task execution behind explicit, documented
boundaries.

This task follows task 107 because both modify block evaluation, evaluator
state, and the float benchmark worktree. Finish and validate the lazy-binding
stack repair first.

## Confirmed current state

The apparent end-of-file restriction is not a parser rule:

- `parseBlockExpr` accepts `tokenEmit` among block statements.
- Top-level programs may contain multiple `emit` statements in any position.
- `Session.evalBlockExpr` executes `emit` inside control-flow bodies.
- `TestEmitInsideLoopBody` passes in the tree-walking interpreter.
- `Runtime.RunVM` accepts that test only because programs containing control
  flow currently fall back to `Session.ExecProgram`.
- `irBuilder.lowerStmt` already has an `emitStmt` lowering that produces an
  explicit IR `emit` instruction.

The inconsistency is introduced by later paths:

- whole-program macro expansion rejects `emit` in `seqBlock`, preventing
  `BuildIR` and native C from reaching the existing IR lowering;
- macro template block expansion omits `emit`;
- the generic expression-block evaluator still accepts only `let` and
  `import`, so functions and other expression contexts do not share the loop
  behavior;
- documentation describes `emit` as an output statement and includes a
  conditional example, but does not define its valid scopes or concurrency
  behavior consistently.

Focused evidence recorded on 2026-07-30:

```text
go test ./internal/osiris -run 'TestEmitInsideLoopBody$' -count=1
ok github.com/sphireinc/osiris/internal/osiris

osiris build benchmarks/native-float-arithmetic.osr
osiris: emit is not allowed in block expressions
```

## Language contract

### Sequential execution

`emit expression` is a non-terminating executable statement. At the point the
statement executes, Osiris evaluates its expression exactly once and appends
one value to the program's ordered output sequence. Execution then continues
with the following statement.

These locations support `emit`:

- at the top level of an executable script, before, between, or after other
  executable statements;
- inside `while` and `for` bodies;
- inside synchronously called function and closure bodies;
- inside an ordinary block expression;
- inside the selected branch of call-shaped `if`;
- inside the selected `match` or `switch` arm;
- inside the executed `try` body or `catch` handler;
- inside a synchronously expanded macro block when the resulting syntax is
  otherwise valid in that location.

An `emit` statement does not become the block's tail value and does not replace
`return`. A block without a tail expression retains its existing `null` value.
`break`, `continue`, and `return` stop later statements normally but do not
erase output already emitted.

`emit left, right` retains its existing contract: evaluate its component
expressions in source order and append one concatenated output value using
`, ` separators. It does not append two independently addressable values.

### Ordering and failure

Outputs are committed in execution order. Earlier successful emissions remain
observable if a later expression returns an error, the program is cancelled,
or native execution terminates with a runtime diagnostic.

Bring the interpreter and CLI into that contract deliberately:

- `Runtime.Run`, `Runtime.RunVM`, and session execution may return both a
  non-empty committed output prefix and an error;
- `osiris run` and `osiris vm` write that committed prefix to standard output
  before rendering the diagnostic on standard error;
- exit categories and JSON diagnostic envelopes remain governed by
  `docs/design/CLI-CONTRACT.md`;
- a successful program's output is unchanged.

Document this as a user-visible clarification and add a migration note because
failed interpreter/VM runs previously discarded their buffered output at the
public API/CLI boundary. Do not make native output transactional merely to
preserve that accidental difference.

Treat `emit` as an observable effect and sequencing barrier. Optimizers must
not delete, duplicate, hoist, sink, reorder, or speculate an emission. They
must also preserve the order of fallible/effectful work relative to an
emission when reordering would change the committed output prefix.

### Modules

Package initialization remains unable to emit. Importing a module must not
silently write output:

```osiris
package sample 1.0.0
emit "not allowed"
```

must retain a clear source-spanned diagnostic.

An exported function or closure defined by a module may emit when a caller
invokes it synchronously. Its output joins the caller's output sequence at the
call site. Test local-module interpreter/VM/native behavior explicitly.

### Concurrent execution

This task does not invent ordering for output produced concurrently. Until a
separate task defines task-local output capture and deterministic merge rules,
`emit` is prohibited while executing:

- `spawn` task bodies;
- `parallel` branches;
- `race` and `select` candidates;
- asynchronous callbacks or indirect functions reached from those contexts.

Reject direct cases as early as the compiler can prove them. Retain a runtime
guard for indirect function/closure calls so dynamic dispatch cannot mutate a
shared output slice or race. Use one stable structured diagnostic explaining
that the task must return a value and the caller may emit it after `await`.

This restriction is based on execution context, not merely syntax. Calling an
emitting function from a concurrent task is prohibited even if the task body
contains no literal `emit` token.

## Implementation requirements

1. Reconcile parser, formatter, macro expansion, session block execution,
   generic expression-block evaluation, bytecode/public VM behavior, IR, and
   native C around the contract above.
2. Use one shared block-statement execution path or a clearly factored common
   helper where possible. Do not retain two diverging allowlists for loop
   blocks and expression blocks.
3. Preserve statement and expression source spans during macro expansion.
   Diagnostics must point to the offending `emit` or concurrent call site.
4. Ensure nested emitted values use the same `Value` rendering, width, style,
   error, enum, collection, and ownership rules as top-level emissions.
5. Carry committed output prefixes through interpreter and VM error returns
   without losing the structured error.
6. Keep stdout and stderr separate in the CLI. Machine-readable diagnostics
   must remain valid JSON on stderr and must not absorb ordinary emitted
   values.
7. Make emitting functions visibly impure to purity analysis and prevent
   unsafe constant folding, CSE, LICM, unrolling, or dead-code elimination.
8. Confirm IR has explicit `emit` instructions inside the correct CFG blocks.
   Native compilation must never execute the source program to manufacture an
   output snapshot.
9. Audit tagged and unboxed native C paths. An emission inside a scalar loop
   must render the same value as the interpreter without forcing unrelated
   loop state into an unsafe representation.
10. Keep unsupported WASM/GPU contexts as explicit compile-time rejections
    unless parity already exists. Parser acceptance alone must not expand
    their documented support claims.
11. Update `docs/design/CLI-CONTRACT.md`, `LANG.txt`, the canonical `newdocs/`
    language pages, support statements, checked examples, and migration notes.
    Correct any example that uses nonexistent statement-shaped
    `if condition { ... } else { ... }` syntax; Osiris currently uses the
    call-shaped `if(condition, then, else)` form.

## Required tests

### Syntax and formatting

- Parse and format multiple top-level emissions separated by declarations.
- Parse-format-parse an `emit` in each supported block context.
- Preserve source spans through a macro that expands to a synchronous block
  containing `emit`.
- Reject module-initialization and concurrent-task cases at the most precise
  available span.

### Sequential semantics

- Prove exact output order for nested `while` and `for` loops with zero, one,
  and multiple iterations.
- Prove an emitting function or closure can emit, return a value, and be
  followed by another caller emission in the expected order.
- Cover selected and unselected call-shaped `if` branches, `match`/`switch`
  arms, and `try`/`catch` paths.
- Cover `break`, `continue`, early `return`, nested blocks, closure capture,
  and a macro-expanded block.
- Prove each expression is evaluated exactly once and comma-form `emit`
  remains one output value.
- Prove an emitting function imported from a local module joins the caller's
  output sequence, while module initialization cannot emit.

### Failure and CLI behavior

- Emit a value and then trigger a structured error. Assert the interpreter and
  public VM API return the same committed prefix plus equivalent errors.
- Add CLI fixtures asserting stdout, stderr, and exit code for runtime failure,
  cancellation, and JSON diagnostics after an earlier emission.
- Add native executable parity for output followed by a runtime error; the
  committed stdout prefix and diagnostic/exit behavior must match the
  documented process contract.

### Concurrency

- Reject a direct `emit` inside each prohibited concurrent form.
- Reject an indirect call to an emitting function or closure from a task.
- Prove returning a task value and emitting it after `await` remains valid.
- Run focused task/output tests under `-race` and prove no concurrent writes to
  session output storage.

### Compiler and native evidence

- Add IR assertions placing `emit` inside loop/function/branch CFG blocks.
- Add generated-C assertions that emissions remain in the corresponding block
  and are not hoisted, duplicated, or replaced by compile-time output.
- Add executable interpreter/VM/native parity for a small loop that emits each
  iteration and a final checksum.
- Cover fixed-width integers, f32/f64 rendering, strings, lists, and enums in
  nested emissions where native support already claims those values.
- Confirm the optimizer's effect report identifies emitting functions and
  prevents unsafe loop transformations.

## Benchmark boundary

Do not add per-iteration output to
`benchmarks/native-float-arithmetic.osr` or
`benchmarks/native-narrow-integers.osr`. Those workloads intentionally emit
one checksum so they measure arithmetic rather than output allocation,
rendering, shell capture, or terminal throughput.

Use a separate small correctness fixture for loop emissions. If output
performance is measured, create a dedicated workload and record output count,
byte count, sink, buffering mode, OS, architecture, Go version, C compiler,
and raw samples. Do not compare it with the arithmetic benchmark baseline.

## Acceptance criteria

- [ ] `emit` works in every specified synchronous executable block with one
      documented ordering and continuation rule.
- [ ] Top-level `emit` remains valid anywhere and is demonstrably not an
      end-of-file terminator.
- [ ] Interpreter, public VM, IR, and native C agree on successful nested
      output sequences.
- [ ] Earlier emissions remain visible after later errors or cancellation,
      with stdout/stderr/exit behavior covered at the CLI boundary.
- [ ] Module initialization remains silent, while synchronously called module
      functions can emit into the caller's sequence.
- [ ] Concurrent task execution rejects direct and indirect emissions without
      races or nondeterministic partial output.
- [ ] Optimizers treat emission as an effect barrier and preserve exact count
      and order.
- [ ] Generated IR/C contains runtime emissions in the correct CFG blocks and
      no compile-time evaluator snapshot.
- [ ] Canonical documentation defines supported scopes, ordering, failure,
      module, and concurrency behavior with checked examples.
- [ ] The arithmetic benchmarks remain checksum-only.
- [ ] Focused normal/race tests, `go test ./...`, `go test -race ./...`,
      `go vet ./...`, documentation checks/build under Node 24,
      `git diff --check`, and task-index validation pass.
- [ ] The completion record includes the final output-commit design, backend
      evidence, CLI fixtures, generated artifacts inspected, and any
      deliberately deferred concurrent-output work.

## Non-goals

- Do not add a second printing keyword or silently redefine `emit` as logging.
- Do not allow top-level package initialization to produce import-time output.
- Do not define concurrent output merge ordering in this task.
- Do not turn the arithmetic benchmarks into output benchmarks.
- Do not implement general VM control-flow bytecode solely for this feature;
  characterize the existing fallback honestly.
- Do not expand WASM/GPU support without their own parity and artifact tests.

