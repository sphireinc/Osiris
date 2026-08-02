# Task 121 database access contract

## Scope and implementation boundary

This design defines the version-1 provider boundary and a deterministic
in-memory driver. The implementation is deliberately a driver API, not an ORM:
queries remain provider text, parameters remain positional `Value` values, and
the embedded fixture implements only a small documented SQL subset.

The implementation is in `internal/osiris/task121_database.go`,
`internal/osiris/task121_database_embedded.go`, and
`internal/osiris/task121_database_test.go`. It uses the existing Osiris
`Value`, struct tags, byte-list, decimal, and datetime representations. No Go
module dependency is required.

The normal language module dispatch is a closed switch in `internal/osiris/module.go`.
That file was explicitly excluded from this change, so this slice has no
automatic `import "std/database"` registration and no new database `Value` kind.
Hosts can use the provider API directly or register it in a
`DatabaseDriverRegistry`. Native lowering is consequently an explicit
unsupported boundary until a declared native driver dependency and a module
hook are approved.

## Public constants

Each constant has one stable spelling and no secret-bearing value.

### `DatabaseDriverAPIVersion`

Syntax: `DatabaseDriverAPIVersion`.

Value: unsigned integer `1`. A driver whose `APIVersion()` differs from this
value is rejected by `DatabaseDriverRegistry.Register`.

Errors: no runtime error; incompatible versions produce an unsupported-driver
error at registration.

Limits and ownership: immutable process metadata; callers do not own storage.
Cancellation: not applicable.

Example: `if driver.APIVersion() != DatabaseDriverAPIVersion { return err }`.

### `DatabaseEmbeddedProvider`

Syntax: `DatabaseEmbeddedProvider`.

Value: the provider identifier `"embedded"`. It is safe to put in logs and
query identities.

Errors, limits, ownership, and cancellation: none; this is immutable metadata.

Example: `registry.Open(ctx, DatabaseEmbeddedProvider, config)`.

### `DatabaseOpenCapability`

Syntax: `DatabaseOpenCapability`.

Value: `"database:open"`; opening requires this capability or a wildcard.

Errors: omission produces `permission_denied`.

Limits, ownership, and cancellation: authorization metadata only; the caller
owns its capability slice and the driver copies it.

Example: `Capabilities: []string{DatabaseOpenCapability, DatabaseEmbeddedCapability}`.

### `DatabaseEmbeddedCapability`

Syntax: `DatabaseEmbeddedCapability`.

Value: `"database:embedded"`; the embedded driver requires this capability or a
database wildcard in addition to `DatabaseOpenCapability`.

Errors, limits, ownership, and cancellation: the same authorization contract
as `DatabaseOpenCapability`.

Example: `config.Capabilities = []string{DatabaseOpenCapability, DatabaseEmbeddedCapability}`.

### Isolation constants

`DatabaseIsolationDefault`, `DatabaseIsolationReadCommitted`,
`DatabaseIsolationRepeatableRead`, and `DatabaseIsolationSerializable` have
syntax equal to their Go identifiers and values `"default"`,
`"read_committed"`, `"repeatable_read"`, and `"serializable"` respectively.

Each is immutable transaction metadata, has no direct error or cancellation
behavior, and is copied into transaction options. The embedded provider uses a
snapshot and detects stale commits; it accepts all four names for provider
compatibility. Example: `DatabaseTxOptions{Isolation:
DatabaseIsolationSerializable}`.

## Public configuration and data types

Every exported field below is copied or returned defensively where applicable.

### `DatabaseIsolationLevel`

Syntax: `DatabaseIsolationLevel("serializable")` or one of the four constants.

It is the named string passed in `DatabaseTxOptions.Isolation`. Unknown strings
are provider-defined; the embedded provider treats them as accepted metadata
and still supplies snapshot conflict detection. No resource is owned by the
type and cancellation is controlled by transaction operations.

Example: `options := DatabaseTxOptions{Isolation: DatabaseIsolationSerializable}`.

### `DatabaseLimits`

Syntax: `DatabaseLimits{...}` inside `DatabaseOpenConfig.Limits`.

`MaxOpenConnections int`: maximum simultaneous operation, row, or transaction
leases. Zero selects four. Values below one after defaulting are errors.

`MaxIdleConnections int`: reported compatibility limit; the fixture keeps no
physical idle connections. Zero is valid; a value above `MaxOpenConnections`
is an invalid-argument error.

`MaxPreparedStatements int`: maximum unclosed prepared statements. Zero selects
64. Exceeding it returns `resource_exhausted`; closing a statement releases its
count.

`MaxRows int`: maximum rows yielded by one result. Zero is unlimited. The row
that would exceed the limit is not yielded and iteration returns a structured
`resource_exhausted` error.

`MaxRowBytes int64`: approximate encoded size limit for each row. Zero is
unlimited. Oversize rows fail before being yielded.

`MaxQueryBytes int64`: UTF-8 query byte limit. Zero selects 1 MiB. Oversize
queries fail before execution or preparation.

`WaitTimeout time.Duration`: maximum wait for a pool lease. Zero delegates to
the caller context. A timeout returns a structured retryable timeout error.

All fields are input-only configuration; the driver copies the value. No field
owns a connection or cancels a context by itself.

Example: `Limits: DatabaseLimits{MaxOpenConnections: 2, MaxRows: 100}`.

### `DatabaseOpenConfig`

Syntax: `DatabaseOpenConfig{Capabilities: ..., Secrets: ..., Limits: ...}`.

`Capabilities []string`: exact grants checked by the provider. The driver
copies the slice; missing grants produce `permission_denied`.

`Secrets map[string]string`: provider secret values. The driver copies the map,
never includes values in messages or query hashes, and owns the retained copy
until connection close.

`Limits DatabaseLimits`: copied resource limits described above.

`OperationTimeout time.Duration`: optional per-operation deadline applied below
the caller context. A timeout returns `timeout`; cancellation retains the
caller context's `cancelled` meaning.

`OperationDelay time.Duration`: deterministic fixture delay for cancellation
tests. It is not a production query scheduler and is applied while a pool
lease is held.

Example: `config := DatabaseOpenConfig{Capabilities: []string{DatabaseOpenCapability,
DatabaseEmbeddedCapability}}`.

### `DatabaseTxOptions`

Syntax: `DatabaseTxOptions{Isolation: level, ReadOnly: true}`.

`Isolation DatabaseIsolationLevel`: requested isolation metadata. The embedded
driver implements snapshot isolation and stale-commit conflict detection.

`ReadOnly bool`: when true, embedded mutating statements fail with
`permission_denied`.

The options are copied at `Begin`; they own no external resources. Context
cancellation belongs to `Begin`, `Commit`, `Rollback`, and transaction
operations.

Example: `tx, err := conn.Begin(ctx, DatabaseTxOptions{ReadOnly: true})`.

### `DatabaseColumn`

Syntax: `DatabaseColumn{Name: "id", DatabaseType: "integer", Nullable: false}`.

`Name string` is the deterministic result name used for struct mapping.
`DatabaseType string` is the normalized provider type label.
`Nullable bool` reports source-schema nullability.

Columns are returned in result order, copied by `Columns`, and owned by the
rows object. They have no independent cancellation or limits.

Example: `for _, column := range rows.Columns() { _ = column.Name }`.

### `DatabaseResult`

Syntax: `DatabaseResult{RowsAffected: n}`.

`RowsAffected int64` is the count from one successful mutation. It is an
immutable result value with no owned resources, errors, limits, or cancellation.

Example: `result, err := conn.Exec(ctx, query, args); _ = result.RowsAffected`.

### `DatabasePoolStats`

Syntax: `DatabasePoolStats{...}` returned by `DatabaseConnection.Stats`.

`MaxOpen int` is the configured lease capacity. `InUse int` counts operation,
rows, and transaction leases. `Available int` is capacity minus in-use.
`Prepared int` counts unclosed statements. These are a point-in-time copy and
do not own or close resources; callers must not use them for synchronization.

Example: `if conn.Stats().Available == 0 { /* apply backpressure */ }`.

## Public interfaces and exact operation contracts

### `DatabaseExecutor`

Syntax: `DatabaseExecutor` as an interface value.

`Exec(context.Context, string, []Value) (DatabaseResult, error)` executes a
parameterized mutation. The query is provider text, arguments are positional,
and the result owns nothing. Errors include operation and a secret-free query
identity.

`Query(context.Context, string, []Value) (DatabaseRows, error)` executes a
parameterized read. On success the returned rows owns a pool lease until close
or exhaustion. Context cancellation, query limits, and malformed statements
are reported before or during iteration.

`Prepare(context.Context, string) (DatabaseStatement, error)` validates and
retains a statement. The caller owns the statement and must close it. The
statement limit applies until close; context cancellation and query limits
apply at preparation.

Example: `rows, err := executor.Query(ctx, "SELECT id FROM users WHERE id = ?", []Value{id})`.

### `DatabaseDriver`

Syntax: an implementation of `APIVersion() uint`, `Provider() string`, and
`Open(context.Context, DatabaseOpenConfig) (DatabaseConnection, error)`.

`APIVersion()` returns the exact interface version and has no error or
cancellation. `Provider()` returns a stable non-secret identifier and has no
error or cancellation. `Open` checks capabilities, observes the context,
copies config, and returns an owned connection; failed opens own nothing.

Example: `var driver DatabaseDriver = NewEmbeddedDriver()`.

### `DatabaseConnection`

Syntax: an owned `DatabaseExecutor` plus connection methods.

`Begin(context.Context, DatabaseTxOptions) (DatabaseTransaction, error)` leases
one pool slot and creates a snapshot. The transaction owns the slot until
commit, rollback, or close; cancellation aborts the begin.

`Stats() DatabasePoolStats` returns a defensive point-in-time copy and does not
block operation cancellation.

`Close() error` is idempotent, closes outstanding rows/statements/transactions,
and releases all owned pool slots. It does not expose secrets and does not
cancel caller contexts.

Example: `defer conn.Close(); tx, err := conn.Begin(ctx, DatabaseTxOptions{})`.

### `DatabaseTransaction`

Syntax: a `DatabaseExecutor` plus `Commit`, `Rollback`, and `Close`.

`Commit(context.Context) error` publishes a snapshot if the database revision
is unchanged. A stale snapshot returns retryable `conflict`; cancellation
leaves the transaction open so the caller can explicitly roll back.

`Rollback(context.Context) error` discards changes and releases the slot. It
observes cancellation; `Close` remains the unconditional cleanup fallback.

`Close() error` rolls back unfinished work and is idempotent. Ownership ends
after the first terminal operation.

Example: `defer tx.Close(); if err := tx.Commit(ctx); err != nil { return err }`.

### `DatabaseRows`

Syntax: the owned result returned by `Query`.

`Columns() []DatabaseColumn` returns copied metadata in result order.
`Next(context.Context) (bool, error)` advances one row, returns false at normal
end, and returns a structured error for cancellation or row/byte limits.
`Values() []Value` returns a defensive copy of the current row and is nil before
the first successful `Next`. `Err() error` returns the terminal iteration
error. `Close() error` releases the result lease exactly once and is idempotent.

Example: `for { ok, err := rows.Next(ctx); if err != nil { return err }; if !ok { break }; use(rows.Values()) }`.

### `DatabaseStatement`

Syntax: the owned result returned by `Prepare`.

`Exec(context.Context, []Value) (DatabaseResult, error)` and
`Query(context.Context, []Value) (DatabaseRows, error)` execute the retained
query with positional values. `QueryText() string` returns the original query
for tooling; it must not be printed as an error because it may contain user
data. `Close() error` releases the prepared-statement count and is idempotent.

Example: `stmt, err := conn.Prepare(ctx, query); defer stmt.Close(); _, err = stmt.Exec(ctx, args)`.

## Registry, driver, and mapping functions

### `NewDatabaseDriverRegistry`

Syntax: `NewDatabaseDriverRegistry() *DatabaseDriverRegistry`.

Parameters: none. Returns an empty concurrency-safe registry. Errors, limits,
ownership, and cancellation: none; the caller owns the returned registry and
registered drivers are references retained by it.

Example: `registry := NewDatabaseDriverRegistry()`.

### `DatabaseDriverRegistry.Register`

Syntax: `registry.Register(driver) error`.

Parameters: one non-nil version-1 driver. Returns nil on success; returns
invalid-argument, unsupported, or already-exists structured errors otherwise.
It does not open a connection, consume pool capacity, or observe a context.

Example: `err := registry.Register(NewEmbeddedDriver())`.

### `DatabaseDriverRegistry.Open`

Syntax: `registry.Open(ctx, provider, config) (DatabaseConnection, error)`.

Parameters: caller context, stable provider name, and explicit open config.
Returns an owned connection or a structured not-found/provider error. The
registry retains no config or secrets; provider cancellation and limits apply.

Example: `conn, err := registry.Open(ctx, DatabaseEmbeddedProvider, config)`.

### `DatabaseDriverRegistry.Providers`

Syntax: `registry.Providers() []string`.

Returns copied lexicographically sorted provider names. It has no error,
resource ownership, limits, or context cancellation and is safe concurrently
with registration/open.

Example: `for _, provider := range registry.Providers() { log(provider) }`.

### `NewEmbeddedDriver`

Syntax: `NewEmbeddedDriver() *EmbeddedDriver`.

Parameters: none. Returns an isolated deterministic driver with an empty schema.
The caller owns the driver; connections from it share its in-memory database.
There is no external service, filesystem, goroutine, or dependency. Cancellation
begins at each connection operation, not construction.

Example: `driver := NewEmbeddedDriver()`.

### `EmbeddedDriver.APIVersion`, `EmbeddedDriver.Provider`, and `EmbeddedDriver.Open`

Syntax: `driver.APIVersion() uint`, `driver.Provider() string`, and
`driver.Open(ctx, config) (DatabaseConnection, error)`.

The first two return the constant version and provider name with no errors or
resources. `Open` requires both explicit capabilities, copies secrets/config,
applies defaults and limits, observes operation timeout/cancellation, and
returns an owned connection. It never contacts an external service.

Example: `conn, err := NewEmbeddedDriver().Open(ctx, config); defer conn.Close()`.

### `DatabaseMapValuesToStruct`

Syntax: `DatabaseMapValuesToStruct(columns, values, target) (Value, error)`.

Parameters: result columns, same-length Osiris values, and a struct *type* Value
(not an instance). Returns a new immutable struct Value. `db` tag names take
precedence over `json`, then source field names; `-` excludes a field. Missing,
duplicate, or known-type-incompatible fields return structured errors. Inputs
and nested values are copied; no database resource is owned and no context is
accepted, so cancellation is the caller's responsibility before mapping.

Example: `mapped, err := DatabaseMapValuesToStruct(rows.Columns(), rows.Values(), userType)`.

### `DatabaseValueFromGo`

Syntax: `DatabaseValueFromGo(raw any) (Value, error)`.

Parameters: nil, bool, string, copied `[]byte`, `time.Time`, exact `big.Rat`,
`json.Number`, signed/unsigned integers, or float values. Returns an owned
Osiris Value or a type error for unsupported Go values. Byte slices are copied;
decimal and datetime machinery preserves exactness. It has no context or
database resource ownership.

Example: `value, err := DatabaseValueFromGo([]byte{1, 2, 3})`.

### `DatabaseValueToGo`

Syntax: `DatabaseValueToGo(value) (any, error)`.

Returns nil, bool, string, numeric scalar, copied `[]byte`, `time.Time`, or a
copied `*big.Rat` for supported Osiris values. Unsupported kinds and out-of-
range integers return structured errors. The returned byte slice or decimal is
owned by the caller; no database resource or cancellation is involved.

Example: `raw, err := DatabaseValueToGo(value); bytes := raw.([]byte)`.

## Embedded SQL and lifecycle semantics

Supported statements are:

* `CREATE TABLE name (column TYPE [NOT NULL], ...)`;
* parameter-only `INSERT INTO name (columns...) VALUES (?, ...)`;
* `UPDATE name SET column = ? WHERE column = ?`;
* `DELETE FROM name WHERE column = ?`; and
* `SELECT *` or a comma-separated column list, one `WHERE column = ?`, and
  optional `ORDER BY` on a selected column with `ASC` or `DESC`.

Malformed syntax, unknown tables/columns, unused parameters, nulls in `NOT
NULL` columns, and type mismatches are invalid-argument/type failures. Query
text is normalized and hashed with SHA-256; only a short identity is public.
Secrets and raw query text never enter structured error messages or ordinary
metadata. Provider details remain debug-only and are removed by redaction.

Rows, statements, and transactions are owned handles. `Close` is idempotent;
connection close force-closes every tracked handle. A transaction's `Close`
rolls back. A row result closes automatically at normal exhaustion or a limit
failure. This is the scope-bound cleanup fallback available without editing the
runtime's module/value registration: host code should use `defer rows.Close()`,
`defer stmt.Close()`, and `defer tx.Close()`.

## Native and interpreter/VM boundary

The host API is usable by Go embedding code and is covered by deterministic
tests. Automatic interpreter/VM parity is not claimed: the allowed-file set
for this implementation excludes `module.go`, the only current standard-module
dispatch point, and adding a database handle would require a new `Value` kind
or an existing resource representation that would be semantically incorrect.
Native compilation therefore has no database lowering in this slice. A future
integration must add a declared driver dependency, a capability-checked module
hook, a resource `Value` representation, and interpreter/VM/native differential
tests before advertising language-level database access.
