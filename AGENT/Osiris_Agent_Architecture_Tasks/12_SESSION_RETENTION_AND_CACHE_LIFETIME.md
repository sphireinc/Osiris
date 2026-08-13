# Task 12 — Bound Long-Lived Session Retention and Evaluator Cache Lifetimes

## Objective

Prevent REPL, LSP, embedded and daemon-like Osiris sessions from retaining dead AST nodes, lexical scopes, completed promises, fingerprints or runtime values indefinitely.

This is a memory-lifetime hardening task. Do not call existing retention a proven leak without measurement, but design the session so dead program state is reclaimable by construction.

## Current risk areas

Inspect `internal/osiris/session.go`, `evaluator_iterative.go`, evaluator structures and any caches containing:

```go
cache   map[string]Value
pending map[string]*promise
fpMemo  map[fpKey]string
fpBusy  map[fpKey]struct{}
```

with keys such as expression pointers + scope pointers.

Any cache key retaining `Expr` or `*scope` can keep an entire old syntax/scope graph reachable.

The eager-`let` task should reduce this pressure, but this task must independently address remaining caches.

## Define lifetimes explicitly

Classify every cache/state map as one of:

1. expression-evaluation lifetime;
2. statement/program execution lifetime;
3. module-compilation lifetime;
4. session lifetime;
5. process/toolchain lifetime.

Default to the shortest correct lifetime.

Document why any cache needs session/process lifetime.

## Remove pointer-graph retention from keys

Do not use arbitrary AST/scope object pointers as lifetime-unbounded cache keys.

Where memoization is needed, use stable IDs scoped to a compilation/execution epoch:

```go
type ExprID uint32
type ScopeID uint32
type Epoch uint64
```

and clear the relevant arena/cache when the epoch ends.

Better: eliminate memoization entirely where eager semantics no longer need it.

## Session epochs

Introduce an execution-generation concept for long-lived sessions if helpful.

At the end of a top-level submission/program:

- clear transient evaluator pending state;
- drop fingerprints that cannot be reused safely;
- release old parse/type IR arenas not referenced by live declarations;
- retain only state genuinely required for REPL definitions/modules.

If a REPL must preserve definitions, store normalized compiled declarations/values rather than keeping every historical whole-program AST alive.

## Module/cache management

Compiler/module caches may be useful. Give them:

- explicit keys (content hash/module ID/compiler version/options);
- bounded capacity or invalidation policy;
- observable metrics for debugging;
- no hidden strong references to obsolete session scopes.

Use LRU/size limits only where reuse is genuinely beneficial. "Clear everything always" is acceptable for temporary evaluator caches.

## Pending promise cleanup

Every pending operation must leave the pending map on:

- success;
- language failure;
- cancellation;
- panic/internal error recovery path.

Use `defer`/structured cleanup in Go to enforce removal.

Tests should intentionally cancel operations and assert no pending entries remain.

## Instrumentation

Add debug-only/internal metrics such as:

```text
active scopes
retained compilation arenas
fingerprint entries
memoized values
pending promises
module cache entries
```

Expose through existing heap/debug tooling if appropriate.

Do not make these public language semantics.

## Stress tests

Build tests/benchmarks that execute thousands of REPL submissions/program fragments and measure heap after forced Go GC in the test harness.

Patterns:

- repeated `let` declarations;
- large temporary literals;
- nested scopes;
- closures created/dropped;
- failed evaluations;
- canceled tasks;
- module reloads;
- LSP document open/change/close if relevant.

The test should demonstrate bounded or workload-proportional retained state, not monotonic retention of every historical AST.

Avoid flaky exact byte thresholds. Assert stable object/cache counts and use generous memory-growth bounds.

## Acceptance criteria

- Every evaluator/compiler cache has documented lifetime/invalidation.
- Ordinary session execution cannot retain every prior AST/scope merely through memoization keys.
- `pending` state is cleaned on all exits.
- REPL stress tests show bounded historical-state retention.
- Heap/debug instrumentation can identify retained compiler/runtime structures.
- Eager `let` implementation no longer requires lifetime-wide expression/scope memoization.
