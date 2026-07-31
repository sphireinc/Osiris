# Task 121: Add database access

**Priority:** P2
**Dependencies:** 013, 045, 046, 111, 116, 117
**Status:** Blocked

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

- [ ] Driver and value-mapping contracts are versioned and documented.
- [ ] Transactions and resources clean up correctly on success, failure, and
      cancellation.
- [ ] Structured error/redaction and concurrency behavior are tested.
- [ ] Full validation passes.

## Non-goals

- No ORM, query language, distributed transaction coordinator, or promise of
  multiple production drivers in the first slice.
