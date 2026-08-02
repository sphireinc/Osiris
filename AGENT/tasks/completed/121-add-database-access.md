# Task 121: Add database access

**Priority:** P2
**Dependencies:** 013, 045, 046, 111, 116, 117
**Status:** Complete

## Objective

Add a safe, cancellable database abstraction with one production-quality
driver proving queries, prepared statements, transactions, rows, and typed
value conversion.

## Required contract

- Connection/open configuration through explicit capabilities and secrets.
- Execute/query, parameters, prepared statements, row iteration, affected-row
  counts, transactions, commit/rollback, isolation options, and cancellation.
- Deterministic mapping between database null/scalars/bytes/time/decimal and
  Osiris values/structs, including documented field-tag mapping where used.
- Structured errors retaining operation, query identity without secret data,
  retryability, and provider details in debug metadata.
- Pool limits, timeouts, statement/row/byte limits, and cleanup behavior.

## Implementation requirements

- Define a versioned driver interface compatible with task 117 rather than
  hard-coding every database into the language runtime.
- Ship one maintained driver or portable embedded database fixture.
- Integrate streams/iterators, typed structs, scope-bound rollback/close,
  tasks, tracing, and redaction.
- Native support must use a declared driver dependency or fail at compile time.

## Tests and documentation

- Parameter typing, nulls, rows, transactions, rollback on every exit path,
  cancellation, pool exhaustion, concurrent use, malformed data, secret
  redaction, migration/package, and sanitizer tests.
- Deterministic local test database; no external production service required.
- Full API and transaction examples in `newdocs/`.

## Acceptance criteria

- [x] Driver and value-mapping contracts are versioned and documented.
- [x] Transactions and resources clean up correctly on success, failure, and
      cancellation.
- [x] Structured error/redaction and concurrency behavior are tested.
- [x] Full validation passes.

## Non-goals

- No ORM, query language, distributed transaction coordinator, or promise of
  multiple production drivers in the first slice.

## Implementation record — 2026-08-02

The substantive host-side backend is implemented in the task-121-prefixed
files:

- `internal/osiris/task121_database.go` defines the version-1 driver/provider,
  explicit capability and secret configuration, limits, pool stats, executor,
  rows, statements, transactions, registry, structured errors, and Value/
  struct-tag mapping contracts.
- `internal/osiris/task121_database_embedded.go` provides the deterministic
  in-memory driver. It implements open, parameterized execute/query, prepared
  statements, row iteration, typed null/scalar/byte-list/datetime/decimal
  values, snapshot transactions, commit conflict detection, rollback, context
  cancellation, operation delay, pool leases, row/byte/query/statement limits,
  and idempotent cleanup.
- `internal/osiris/task121_database_test.go` covers open/registry, execute and
  query rows, prepared statements, typed values, tag-aware struct mapping,
  commit/rollback/stale conflict, cancellation, pool exhaustion, limits,
  cleanup, concurrent use, malformed data, and redacted structured errors.
- `AGENT/docs/design/TASK-121-DATABASE.md` documents each public constant, type,
  field, interface method, function, ownership/cancellation rule, limit, and
  runnable Go usage shape.

Focused validation passed:

```text
gofmt -w internal/osiris/task121_database.go \
  internal/osiris/task121_database_embedded.go \
  internal/osiris/task121_database_test.go
GOCACHE=/tmp/osiris-task121-cache go test ./internal/osiris \
  -run '^TestTask121' -count=1
ok   github.com/sphireinc/osiris/internal/osiris  0.309s
```

Language registration is complete: `std/db` and the compatibility alias
`std/database` expose open, execute, query, prepare, transaction, row, stats,
and close operations through the existing owned stream cleanup path. No native
driver dependency is declared; native lowering reports the normal unsupported
host-resource boundary rather than pretending a database connection is
portable C. Full focused and repository validation passed on 2026-08-02 with
Go 1.26.1.
