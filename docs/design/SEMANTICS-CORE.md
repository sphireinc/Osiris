# Core Semantics 1

**Status:** normative language contract. **Executable corpus:** `spec/core/`.
**Version report:** `osiris version` prints `semantics: Core Semantics 1`.

This document defines observable Osiris behavior independently of the
interpreter, VM, native C backend, or a future direct-native/WASM backend. A
backend either implements a rule or rejects its source before execution with a
stable unsupported-feature diagnostic. It may never interpret a program as a
fallback for a compiled artifact.

Examples use `=>` to mean the required result. Error names describe stable
categories; a diagnostic may add source location and cause information without
changing its category. Every feature change must cite the affected rule(s), add
or update `spec/core` fixtures, and update the public support matrix.

## 1. Evaluation order

1.1 Evaluation is source ordered: callee, then call arguments left-to-right;
binary left operand then right operand; collection members and fields
left-to-right; indexing evaluates container then index. `&&` and `||` first
evaluate their left operand and conditionally evaluate the right operand.

```osiris
func mark(x) { emit x; x }
emit [mark("a"), mark("b")] // prints a, then b
emit false && mark("never")  // => false; no "never"
```

Parallel physical execution is permitted only when it cannot change values,
errors, cleanup, output, allocation-visible identity, or source-observable
ordering. A scheduler must use source index as its tie breaker.

## 2. Bindings and mutation

2.1 `const` is a compile-time immutable value and is valid only where its
initializer can be evaluated at compile time. `let` eagerly evaluates its RHS,
then creates an immutable runtime binding. `var` eagerly evaluates its RHS,
then creates a mutable cell. A failing initializer creates no binding.

2.2 A name cannot be redeclared in the same lexical scope. An inner scope may
shadow an outer name; the initializer of the inner declaration resolves the
outer name because the new binding begins after its initializer.

```osiris
let n = 1
{ let n = n + 1; emit n } // => 2
emit n                     // => 1
var total = 1; total = 2   // => total is 2
```

2.3 Assignment requires an existing `var` cell and evaluates RHS before
committing the cell. Assignment to `const`, `let`, a missing name, or a
non-assignable target is a structured error.

## 3. Explicit laziness

3.1 Laziness is opt-in: `lazy { expression }` creates `Lazy<T>` without
evaluating `expression`; `force(value)` is the only operation that evaluates a
lazy value. It evaluates once, shares the terminal success, error, or
cancellation outcome with every force, and detects self-reentry as `LazyCycle`.

```osiris
var calls = 0
let x = lazy { calls = calls + 1; calls }
force(x) // => 1
force(x) // => 1; calls remains 1
```

No ordinary binding, argument, collection element, conversion, or print
implicitly forces `Lazy<T>`.

## 4. Scope and declaration visibility

4.1 Scopes are lexical. Blocks, functions, modules, match arms, and each loop
iteration create the documented nested scopes. Declarations are visible after
their initializer only. Functions are predeclared in their containing module or
block so mutual recursion works; non-function bindings are not hoisted.

```osiris
func even(n) => if(n == 0, true, odd(n - 1))
func odd(n) => if(n == 0, false, even(n - 1))
```

## 5. Calls and returns

5.1 A call evaluates its callee and arguments under Rule 1. User functions
have exact arity unless the signature explicitly declares a future variadic
form; missing or extra arguments are `ArityError`. Parameters are immutable
bindings to evaluated argument values. `return` exits the nearest function
once; a tail expression returns its value when no explicit return occurs.

```osiris
func sum(a, b) => a + b
sum(20, 22) // => 42
```

## 6. Closures

6.1 A function captures its lexical environment at creation. Immutable
bindings are captured as values; `var` bindings capture their mutable cell,
making a later outer assignment observable through the closure. Captures live
as long as any closure that needs them. Task capture is separately governed by
Rule 16.

```osiris
var n = 1
func read() => n
n = 2
read() // => 2
```

## 7. Types and dynamic boundaries

7.1 Osiris is statically checked. Public exports, function boundaries, and
ambiguous boundaries require explicit annotations; local expressions may be
inferred. `dynamic` is an explicit, checked interop/FFI escape hatch only. It
does not silently appear through an omitted annotation, and operations on it
perform a checked boundary conversion or return a structured type error.

```osiris
func distance(x: f64, y: f64) -> f64 => hypot(x, y)
```

## 8. Numeric literals and conversions

8.1 A bare integer is `u64`; a bare decimal is the language default floating
number type. Suffixes select an exact numeric family and width. Context may
narrow a bare integer only when it is representable. Cross-family conversions
are explicit; a lossy conversion must name its rounding/checking policy.

```osiris
let byte: u8 = 255
u8(256) // => OutOfRange
```

8.2 Arithmetic promotes only under the documented family/width rules. Fixed
width overflow behavior is part of the selected family contract and is tested
by the corpus; it must be identical in all supporting backends. Division by
zero is `DivisionByZero`.

## 9. Equality, identity, and ordering

9.1 `==` is total structural equality for comparable values. Exact numeric
values of different compatible families compare equal only when they denote the
same mathematical value. `NaN != NaN`; `+0 == -0`. Lists, tuples, structs,
maps, and sets compare recursively according to their published collection
semantics. Resources, tasks, and functions are identity-bearing handles and
are not structurally ordered.

9.2 Ordering is defined only for documented scalar families. Ordering a value
without an ordering relation is `OrderingError`; it must not borrow host
pointer ordering.

## 10. Hashes, maps, and sets

10.1 Map and set keys must be hashable, equality-compatible values. Equal keys
must produce equal hashes. Maps and sets have deterministic insertion-order
iteration. A duplicate map key replaces the value without moving its first
position; a duplicate set member has no effect.

```osiris
map_new("a", 1, "a", 2, "b", 3) // iteration: a, b; a maps to 2
```

## 11. Aggregate values

11.1 Lists, maps, sets, tuples, and structs are values. Constructors and
update APIs return logically independent aggregates and never mutate input
aggregates. Nested resource, task, function, and module handles retain their
explicit identity semantics. Aggregate iteration follows Rule 10 where
applicable and source order otherwise.

```osiris
let before = [1, 2]
let after = setat(before, 0, 9)
// before remains [1, 2]; after is [9, 2]
```

## 12. Control flow and loops

12.1 Conditions use the documented deterministic truthiness conversion: null,
false, numeric zero, and error values are false; other values are true. A
future bool-only condition rule is a language-version change, not a backend
choice. `if` evaluates one branch only.

12.2 `for` evaluates its iterable once before iterating and introduces a fresh
immutable iteration binding. `while` tests before each iteration. `break` and
`continue` target the innermost loop, execute cleanup under Rule 15, and check
cancellation at iteration boundaries.

## 13. Pattern matching

13.1 `match` evaluates its subject exactly once and tries arms in source order.
Only the selected arm body runs. Arm bindings are visible to its guard and body
only; a false guard proceeds to the next arm. Repeated bindings mean equality
constraints; `not` patterns bind nothing.

```osiris
match [2, 3] { [a, b] => a * b; _ => 0 } // => 6
```

13.2 Closed enum subjects are checked for exhaustiveness and unreachable arms.
No match is `MatchError` for dynamic/open subjects. Every backend must support
a pattern feature or reject it before execution.

## 14. Errors, Result, Option, and propagation

14.1 Language failures are structured records with stable kind, message,
cause, source span, and public metadata. Debug-only host details are redacted
from normal rendering. Host panics normalize to `InternalError` and never cross
the language or FFI boundary.

14.2 `Result` and `Option` are ordinary values. `?`/binding propagation returns
the original failure record rather than replacing it. `try/catch` receives that
record, optionally with documented context. Repeated rendering is deterministic.

```osiris
try { let? n = err(error(InvalidArgument, "bad")); n }
catch e { e.kind } // => invalid_argument
```

## 15. Defer and cleanup

15.1 `defer` registers only after its statement is reached. Deferred actions
run exactly once, in lexical LIFO order, on normal scope exit, return, break,
continue, error, cancellation, and task termination. Cancellation is delayed
while cleanup runs. Cleanup control flow becomes failure; a primary body error
retains ordered cleanup failures as suppressed causes.

```osiris
defer { emit "outer" }
{ defer { emit "inner" } }
// prints inner, then outer on scope exits
```

## 16. Tasks and capture

16.1 `spawn` snapshots lexical values and mutable cells at spawn time; tasks do
not share language `var` cells with the parent. A task has stable identity and
one terminal outcome; repeated `await` returns the same value or failure.
Cancellation is a cooperative request and is observable as a canonical
cancellation error when observed. Task-originated `emit` is rejected unless a
future ordered task-output mechanism explicitly permits it.

## 17. Parallel, race, and select

17.1 Parallel result collections preserve input order. If multiple failures are
available in one scheduler turn, the lowest lexical/source index wins and the
remaining failures are retained as ordered suppressed causes. `race` and
`select` choose the lowest arm index among simultaneously-ready alternatives,
then cancel non-winners and run their cleanup. Host timing may not select an
observable winner.

## 18. Modules and initialization

18.1 Module identity is canonical resolution plus content digest or resolved
package version. A successfully initialized canonical module runs once per
runtime; diamond imports share it. Cycles fail with a stable full import chain.
Exports become visible only after successful initialization. Resolution is
locked before execution; module initialization cannot perform unordered output.

```osiris
import "std/text" as text
text.slug("Hello, Weird World") // => "hello-weird-world"
```

## 19. Resources, FFI, and unsafe code

19.1 Resources are non-copyable identity handles. Each FFI declaration states
borrowed, owned, transferred, or retained ownership for parameters and returns;
the runtime enforces exactly-once release. Raw host pointers and undeclared
symbols never cross normal FFI. Unsafe code requires an explicit capability and
has no implicit conversion path. Use-after-close, double-close, and closing a
borrowed/active handle are structured errors.

## 20. Backend equivalence

20.1 For supported source, interpreter, VM, native, and WASM produce equivalent
value kinds/widths/shapes, ordered stdout, error fields/spans, cleanup,
module/task ordering, and collection order. A backend’s unsupported surface
must reject before artifact generation or execution. Floating behavior follows
Rule 24’s specified IEEE and formatting allowances. `spec/core` is the
backend-neutral enforcement point.

## 21–30. Required edge contracts

21. **Text:** strings are valid UTF-8; `len` and indexing use Unicode scalar
values, not bytes or grapheme clusters. Invalid external UTF-8 is a boundary
error.

22. **Bounds:** list, string, and tensor indexes are non-negative integers;
out-of-range access is `BoundsError`. Slices are half-open and may clamp only
when that API explicitly says so.

23. **Division:** quotient/remainder behavior for negative operands is
Euclidean or truncating only as a published family rule; zero is always
`DivisionByZero`. The corpus records each signed-family case.

24. **Floating point:** f32/f64 use IEEE 754 rounding and deterministic
shortest round-trip rendering. NaN/infinity and signed-zero behavior is Rule 9.
Numerical tolerance is allowed only by APIs that name it.

25. **Tensors and quantities:** tensors have declared rank/shape/dtype; ragged
construction, invalid broadcast, rank mismatch, bounds, and unit-incompatible
operations are structured errors. Updates obey Rule 11’s no-alias guarantee.

26. **Macros:** expansion is deterministic after parse and before type checking;
macro-generated binders are hygienic/fresh, argument evaluation remains exactly
once, and expansion-depth errors retain an expansion source chain.

27. **Generics and traits:** trait implementation selection is deterministic;
overlapping or ambiguous implementations are rejected. Specialization may not
change observable behavior.

28. **Blocking cancellation:** blocking filesystem/network/FFI operations check
cancellation before and after calls. Non-interruptible host calls document that
fact and still return canonical cancellation at the next safe boundary.

29. **Process exit:** normal main completion exits 0. An uncaught Osiris error
prints deterministic diagnostic output to stderr and exits nonzero; host panic
traces are never default user output.

30. **Time, randomness, and I/O:** deterministic mode injects/records clock
and PRNG state. Seeded runs repeat exactly. Directory/map/source artifact order
is sorted or otherwise published; external I/O preserves request order and
maps failures to structured errors.

## Normative example matrix

Each line below supplies two minimum, portable observations for its rule. They
are part of the contract, not an exhaustive tutorial. `error X` means a
structured error whose stable kind is `X`; backend obligations apply in
addition to Rule 20.

1. **Order:** `emit [mark("a"), mark("b")]` prints `a`, then `b`; `false && fail()` returns `false` without calling `fail`. Edge: a left-operand failure prevents the right operand. Every supporting backend SHALL preserve this trace.
2. **Bindings:** `let n = mark(1); emit n` calls `mark` before `emit`; `let n = fail(); emit n` is `error` before the emit and creates no `n`. Edge: `let n=1; let n=2` is `RedeclarationError`. Every backend SHALL create a value/cell, never a deferred ordinary binding.
3. **Lazy:** `let x = lazy { fail() }; emit 7` prints `7`; `force(x); force(x)` executes the body once. Edge: recursive force is `LazyCycle`. A backend without the feature SHALL reject `lazy` before execution.
4. **Scope:** `{ let n = 1 }; n` is `UnknownName`; `let n=1; { let n=n+1; emit n }; emit n` prints `2`, then `1`. Edge: only functions are predeclared for mutual recursion. Every backend SHALL use the same lexical visibility.
5. **Calls:** `func add(a,b)=>a+b; add(20,22)` returns `42`; `add(1)` is `ArityError`. Edge: callee evaluation precedes all arguments. Every backend SHALL bind evaluated arguments once to immutable parameters.
6. **Closures:** `var n=1; func f()=>n; n=2; f()` returns `2`; `let n=1; func f()=>n; f()` returns `1`. Edge: escaping a closure retains its captures. Every backend SHALL preserve value versus cell capture.
7. **Types:** `let count=3` infers the numeric literal type; `func f(x: u32)->u32=>x; f("x")` is a compile-time type error. Edge: unresolved inference is an error, not `dynamic`. Backends SHALL consume the same checked representation or reject first.
8. **Numerics:** `let n:u8=255` is valid; `let n:u8=256` is `OutOfRange`. Edge: a conversion that loses precision requires an explicit conversion operation. Every backend SHALL preserve kind, width, and overflow rule.
9. **Equality:** `0.0 == -0.0` is `true`; `nan() == nan()` is `false`. Edge: ordering a function/resource/task is `OrderingError`. Backends SHALL not use host pointer/order accidents.
10. **Collections:** `map_new("a",1,"a",2)` keeps one `a` with value `2`; `set_new(1,1)` has one member. Edge: a non-hashable key is `HashabilityError`. Every backend SHALL preserve insertion order and duplicate policy.
11. **Aggregates:** `let a=[1]; let b=setat(a,0,2)` leaves `a` as `[1]`; `b` is `[2]`. Edge: nested handle identity is not deep-copied. Every backend SHALL expose logically independent aggregate updates.
12. **Control:** `if(false, fail(), 7)` returns `7`; `for n in [1,2] { emit n }` prints `1`, then `2`. Edge: iterable evaluation happens once before the loop. Every backend SHALL run cleanup before a loop transfer escapes.
13. **Patterns:** `match [2,3] { [a,b]=>a*b; _=>0 }` returns `6`; `match 1 { 2=>2 }` is `MatchError`. Edge: a false guard continues arm search. Every backend SHALL evaluate the subject once and preserve arm order.
14. **Errors:** `try { err(error(InvalidArgument,"x"))? } catch e { e.kind }` returns `invalid_argument`; `ok(1)?` returns `1`. Edge: a host panic is `InternalError`, not an uncaught host trace. Every backend SHALL retain public error fields and source span.
15. **Cleanup:** `defer { emit "a" }; defer { emit "b" }` prints `b`, then `a`; `if(false) { defer { emit "x" } }` never prints `x`. Edge: cleanup failure is suppressed after the primary failure in LIFO order. Every backend SHALL execute each registered action once.
16. **Tasks:** `let t=spawn({ 42 }); await(t)` returns `42`; awaiting a failed task twice returns the same failure. Edge: a parent `var` is captured as a spawn-time snapshot. Every backend SHALL keep task output disallowed and cancellation canonical.
17. **Concurrency:** `parallel({1},{2})` returns results in source order; simultaneous `race` readiness picks the first lexical arm. Edge: same-turn child failures choose the lowest source index and retain ordered suppressed errors. Every backend SHALL not expose host scheduling timing.
18. **Modules:** a diamond import initializes a shared module once; `a -> b -> a` is a cycle error with the chain. Edge: a failed initialization never exposes partial exports. Every backend SHALL use canonical module identity and deterministic resolution.
19. **FFI/resources:** closing an owned handle twice is `ClosedHandleError`; a borrowed argument remains usable by its owner after a call. Edge: raw pointers and undeclared symbols are rejected. Every backend SHALL enforce the declared ownership transition.
20. **Equivalence:** a supported scalar program has identical rendered output in interpreter/VM/native; an unsupported native feature rejects before artifact execution. Edge: no generated artifact delegates a continuation to the interpreter. Every claiming backend SHALL run the Core corpus.
21. **Text:** `len("é")` is one scalar value; `at("é",0)` returns `"é"`. Edge: indexing the byte interior is impossible and negative/out-of-range is `BoundsError`. Every backend SHALL validate UTF-8 at boundaries.
22. **Bounds:** `at([1],0)` returns `1`; `at([1],1)` is `BoundsError`. Edge: a fractional/negative index is `IndexTypeError`. Every backend SHALL make the same before/after-boundary decision.
23. **Division:** `7 / 3` and `7 % 3` follow the published family quotient/remainder pair; `7 / 0` is `DivisionByZero`. Edge: `-7 / 3` and `-7 % 3` are tested as a pair, never inherited from host C/Go. Every backend SHALL match the corpus exactly.
24. **Floating:** shortest render/reparse of `1.5f64` preserves its value; `-0.0` retains its specified sign where observable. Edge: NaN and infinity classification follows IEEE 754. Every backend SHALL use named tolerance only in APIs that declare it.
25. **Tensors/quantities:** `shape(tensor([[1,2],[3,4]]))` returns `[2,2]`; a ragged tensor is `ShapeError`. Edge: incompatible units/broadcast are structured errors. A limited backend SHALL reject before execution.
26. **Macros:** `macro twice(x)=>x+x; twice(21)` returns `42`; recursive expansion past the limit is `MacroExpansionError`. Edge: generated binders are fresh and arguments are evaluated once. Every backend SHALL receive post-expansion source with origin spans.
27. **Traits:** one non-overlapping `impl` resolves deterministically; two applicable implementations are `AmbiguousImplementationError`. Edge: specialization cannot change a generic program’s output. Every backend SHALL share the resolved implementation decision.
28. **Cancellation:** a cancelled blocking accept/read returns `Cancelled`; an already-cancelled context prevents entering a blocking host call. Edge: noninterruptible calls report cancellation at their declared safe boundary. Every backend SHALL document the boundary and run cleanup.
29. **Exit:** `emit 1` from main exits `0`; an uncaught language error exits nonzero and writes its diagnostic to stderr. Edge: a host panic is normalized before process reporting. Every CLI/backend runner SHALL preserve stream and status separation.
30. **Determinism:** equal seed/counter random calls return equal values; deterministic directory enumeration returns published sorted order. Edge: clock/random/I/O variation requires an explicit nondeterministic capability. Every deterministic-mode backend SHALL reproduce the observable trace.

## Implementation deviation ledger

Core Semantics 1 is authoritative even where the current implementation does
not yet comply. This ledger is deliberately part of the specification: it
prevents status documents from being misread as alternate language semantics.
Each item is a later implementation task’s acceptance target, not an exception
that a backend may silently take.

| ID | Rules | Current evidence | Required correction | Owning later task(s) |
| --- | --- | --- | --- | --- |
| CS1-D01 | 1, 17 | `runtime.go` schedules ordinary binary operands and some builtin argument groups; `scheduler.go` reports first-arriving failure. | Preserve source order and source-index tie breaks for all observable effects/errors. | 01, 11; final Core gate: 18 |
| CS1-D02 | 2, 3 | **Resolved by Task 01 (2026-08-13):** ordinary `let` bindings now retain eager values; explicit `Lazy<T>` owns its distinct thunk and once-only state in interpreter, VM, and native C. The remaining parity audit is intentionally owned by Task 11. | Keep eager-binding and explicit-lazy coverage in the cross-backend parity gate. | 11 |
| CS1-D03 | 2, 4 | **Resolved by Task 11 (2026-08-13):** the frontend rejects a name already declared in the current lexical scope, while child scopes continue to shadow parents and initializers run before their own binding exists. | Keep same-scope redeclaration and nested-shadowing coverage in the cross-backend parity gate. | 18 |
| CS1-D04 | 7 | `typecheck.go` and public reference data preserve omitted annotations as implicit `dynamic`. | Local inference plus explicit-only dynamic/FFI boundaries. | 08, 09, 03, 07 |
| CS1-D05 | 8, 9, 24, 27 | Numeric-family promotion, equality, and trait decisions remain distributed across runtime/type strings. | Canonical structural type graph and published numeric/equality/coherence rules. | 08, 09, 03, 07 |
| CS1-D06 | 10, 11 | `collections.go` uses deterministic linear map/set scans; aggregate copy/alias limits are not uniformly proven. | Deterministic hash collections and explicit value-update/alias invariants. | 10, 17 |
| CS1-D07 | 13, 20, 25, 26 | Native/WASM support is feature-sliced; several pattern, tensor, macro, and error forms lack parity evidence. | One typed frontend/IR and stable reject-before-execution behavior for unsupported forms. | 02, 15, 05, 18 |
| CS1-D08 | 14, 15, 16, 17 | Cleanup/task behavior has unit coverage, but cross-backend error identity, cancellation, and deterministic failure ties are not one matrix. | Shared semantic probe suite with full error/cleanup/task parity. | 11, 12, 18 |
| CS1-D09 | 18, 19, 28–30 | Module/FFI/platform behavior is partly implemented in Go and generated native fragments with incomplete common ownership/cancellation proof. | Explicit compiler/runtime/platform boundary and canonical native runtime ABI. | 16, 05, 06 |
| CS1-D10 | 20 | Existing conformance corpus is selective and backend labels are not yet a Core-claim matrix. | Run the canonical `spec/core` corpus for every backend claiming Core 1 and generate status from evidence. | 18 |

The initial fixtures in `spec/core` cover behavior already supportable without
performing later architectural work. A fixture for a documented deviation is
added by its owning task when it can be required to pass. A `reject` backend
status is a testable promise to reject before execution; it is never a hidden
fallback.

### Task 14 completion boundary

Task 14 is complete when this contract is internally consistent, its
normative/source-data documentation is reconciled, currently supportable
fixtures execute through the public harness, and every known deviation is
listed above with a later owning task. Green implementation conformance to all
rules is deliberately the responsibility of those later tasks and the Task 18
Core 1 surface gate.
