package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"strings"
	"sync"
	"time"
)

// DatabaseDriverAPIVersion is the stable version of the database provider
// contract. A driver must report this exact value before it can be registered.
const DatabaseDriverAPIVersion uint = 1

// DatabaseEmbeddedProvider is the provider name of the deterministic embedded
// driver shipped with this task. It is also the capability suffix required to
// open that driver.
const DatabaseEmbeddedProvider = "embedded"

// DatabaseOpenCapability authorizes opening a database connection. A caller
// may grant this capability together with a provider-specific capability.
const DatabaseOpenCapability = "database:open"

// DatabaseEmbeddedCapability authorizes the embedded provider. It is required
// in addition to DatabaseOpenCapability for EmbeddedDriver.Open.
const DatabaseEmbeddedCapability = "database:embedded"

// DatabaseIsolationLevel controls the visibility guarantees requested by a
// transaction. The embedded driver accepts all values for API compatibility,
// but implements snapshot isolation with conflict detection at commit.
type DatabaseIsolationLevel string

const (
	// DatabaseIsolationDefault asks the provider to choose its default level.
	DatabaseIsolationDefault DatabaseIsolationLevel = "default"
	// DatabaseIsolationReadCommitted requests read-committed behavior.
	DatabaseIsolationReadCommitted DatabaseIsolationLevel = "read_committed"
	// DatabaseIsolationRepeatableRead requests repeatable-read behavior.
	DatabaseIsolationRepeatableRead DatabaseIsolationLevel = "repeatable_read"
	// DatabaseIsolationSerializable requests serializable behavior.
	DatabaseIsolationSerializable DatabaseIsolationLevel = "serializable"
)

// DatabaseLimits bounds resources owned by one opened connection. Zero means
// that the provider default applies, except for byte and row limits where zero
// means unlimited.
type DatabaseLimits struct {
	// MaxOpenConnections is the number of simultaneously leased operation or
	// transaction slots. The embedded provider defaults to four.
	MaxOpenConnections int
	// MaxIdleConnections is retained for provider compatibility. The embedded
	// provider reports it but does not retain physical idle connections.
	MaxIdleConnections int
	// MaxPreparedStatements bounds prepared statements that have not been
	// closed. The embedded provider defaults to 64.
	MaxPreparedStatements int
	// MaxRows bounds rows yielded by one result. Zero means unlimited.
	MaxRows int
	// MaxRowBytes bounds the approximate encoded size of one yielded row. Zero
	// means unlimited.
	MaxRowBytes int64
	// MaxQueryBytes bounds the UTF-8 query text accepted by the provider. Zero
	// means the embedded default of 1 MiB.
	MaxQueryBytes int64
	// WaitTimeout bounds waiting for a pool slot. Zero delegates cancellation
	// and deadlines entirely to the operation context.
	WaitTimeout time.Duration
}

// DatabaseOpenConfig carries explicit authorization, secret references, pool
// limits, and operation timing into a driver. Drivers must copy it before
// retaining it; callers may reuse their slices and map after Open returns.
type DatabaseOpenConfig struct {
	// Capabilities lists exact capability strings granted to the provider.
	Capabilities []string
	// Secrets contains provider-specific secret values. Providers must never
	// put these values in errors, query identities, or ordinary metadata.
	Secrets map[string]string
	// Limits contains connection pool, statement, row, byte, and wait limits.
	Limits DatabaseLimits
	// OperationTimeout is applied to each open, execute, query, and prepare
	// operation when it is non-zero, while preserving the caller's deadline.
	OperationTimeout time.Duration
	// OperationDelay is a deterministic fixture delay used to test cancellation
	// and timeout behavior. Production drivers may reject it.
	OperationDelay time.Duration
}

// DatabaseTxOptions configures one transaction. Close on a transaction is
// equivalent to rollback when neither Commit nor Rollback has completed.
type DatabaseTxOptions struct {
	// Isolation selects the requested isolation level.
	Isolation DatabaseIsolationLevel
	// ReadOnly rejects mutating statements when true.
	ReadOnly bool
}

// DatabaseColumn describes one result column in declaration order.
type DatabaseColumn struct {
	// Name is the deterministic provider column name.
	Name string
	// DatabaseType is the provider's normalized type label.
	DatabaseType string
	// Nullable reports whether the source schema permits null.
	Nullable bool
}

// DatabaseResult reports the effect of one successful mutation.
type DatabaseResult struct {
	// RowsAffected is the number of rows inserted, updated, or deleted.
	RowsAffected int64
}

// DatabasePoolStats reports a point-in-time view of one connection's pool.
type DatabasePoolStats struct {
	// MaxOpen is the configured number of simultaneous leases.
	MaxOpen int
	// InUse is the number of currently leased operation, row, or transaction
	// slots.
	InUse int
	// Available is MaxOpen minus InUse.
	Available int
	// Prepared is the number of open prepared statements.
	Prepared int
}

// DatabaseExecutor is the common operation surface of connections and
// transactions. Every operation observes context cancellation and configured
// query and timeout limits.
type DatabaseExecutor interface {
	// Exec executes a parameterized mutation and returns its affected-row count.
	Exec(context.Context, string, []Value) (DatabaseResult, error)
	// Query executes a parameterized read and returns owned, closable rows.
	Query(context.Context, string, []Value) (DatabaseRows, error)
	// Prepare validates and retains a parameterized statement until Close.
	Prepare(context.Context, string) (DatabaseStatement, error)
}

// DatabaseDriver is the versioned provider boundary. Implementations must be
// safe for concurrent Open calls and must not retain caller-owned config data.
type DatabaseDriver interface {
	// APIVersion returns the provider ABI version.
	APIVersion() uint
	// Provider returns a stable, non-secret provider identifier.
	Provider() string
	// Open authorizes and opens one independently closable connection.
	Open(context.Context, DatabaseOpenConfig) (DatabaseConnection, error)
}

// DatabaseConnection is an owned pool-backed database connection. Close is
// idempotent and closes outstanding rows, statements, and active transactions.
type DatabaseConnection interface {
	DatabaseExecutor
	// Begin starts a snapshot transaction and leases one pool slot.
	Begin(context.Context, DatabaseTxOptions) (DatabaseTransaction, error)
	// Stats returns pool and prepared-statement counts.
	Stats() DatabasePoolStats
	// Close releases all connection-owned resources; repeated calls are safe.
	Close() error
}

// DatabaseTransaction is a connection-scoped transaction. Commit releases its
// slot only after a successful conflict check; Rollback is idempotent.
type DatabaseTransaction interface {
	DatabaseExecutor
	// Commit atomically publishes the transaction snapshot or returns a
	// structured conflict error.
	Commit(context.Context) error
	// Rollback discards all transaction changes and releases its pool slot.
	Rollback(context.Context) error
	// Close rolls back an unfinished transaction and is idempotent.
	Close() error
}

// DatabaseRows is an owned result iterator. Values is valid only for the most
// recently successful Next call and returns a copy of that row.
type DatabaseRows interface {
	// Columns returns immutable column metadata in result order.
	Columns() []DatabaseColumn
	// Next advances to one row, observing cancellation and row/byte limits.
	Next(context.Context) (bool, error)
	// Values returns the current row as Osiris Values, or nil before Next.
	Values() []Value
	// Err returns the terminal iteration error, if any.
	Err() error
	// Close releases the result's pool lease; repeated calls are safe.
	Close() error
}

// DatabaseStatement is a retained parameterized statement. It owns no pool
// slot between calls and must be closed to release its statement-limit count.
type DatabaseStatement interface {
	// Exec executes the retained mutation with positional parameters.
	Exec(context.Context, []Value) (DatabaseResult, error)
	// Query executes the retained read with positional parameters.
	Query(context.Context, []Value) (DatabaseRows, error)
	// QueryText returns the original query text for diagnostics and tooling.
	QueryText() string
	// Close releases the statement; repeated calls are safe.
	Close() error
}

// DatabaseDriverRegistry is a concurrency-safe explicit provider registry. It
// is intentionally separate from the language module registry so embedders can
// choose providers and capabilities without ambient global state.
type DatabaseDriverRegistry struct {
	mu      sync.RWMutex
	drivers map[string]DatabaseDriver
}

// NewDatabaseDriverRegistry constructs an empty provider registry.
func NewDatabaseDriverRegistry() *DatabaseDriverRegistry {
	return &DatabaseDriverRegistry{drivers: make(map[string]DatabaseDriver)}
}

// Register adds one version-1 driver under its stable provider name. Duplicate
// names and incompatible versions are rejected without changing the registry.
func (r *DatabaseDriverRegistry) Register(driver DatabaseDriver) error {
	if r == nil || driver == nil {
		return databaseError(ErrorKindInvalidArgument, "register", "", false, "", "database registry is nil or driver is nil", nil)
	}
	if driver.APIVersion() != DatabaseDriverAPIVersion {
		return databaseError(ErrorKindUnsupported, "register", "", false, driver.Provider(), fmt.Sprintf("unsupported database driver API version %d", driver.APIVersion()), nil)
	}
	provider := driver.Provider()
	if provider == "" || strings.ContainsAny(provider, " \t\r\n") {
		return databaseError(ErrorKindInvalidArgument, "register", "", false, provider, "database provider name is invalid", nil)
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.drivers[provider]; exists {
		return databaseError(ErrorKindAlreadyExists, "register", "", false, provider, "database provider is already registered", nil)
	}
	r.drivers[provider] = driver
	return nil
}

// Open finds a registered provider and opens it with the caller's context and
// explicit capabilities. The registry does not retain secrets or config.
func (r *DatabaseDriverRegistry) Open(ctx context.Context, provider string, config DatabaseOpenConfig) (DatabaseConnection, error) {
	if r == nil {
		return nil, databaseError(ErrorKindInvalidArgument, "open", "", false, provider, "database registry is nil", nil)
	}
	r.mu.RLock()
	driver := r.drivers[provider]
	r.mu.RUnlock()
	if driver == nil {
		return nil, databaseError(ErrorKindNotFound, "open", "", false, provider, "database provider is not registered", nil)
	}
	return driver.Open(ctx, config)
}

// Providers returns registered provider names in lexicographic order.
func (r *DatabaseDriverRegistry) Providers() []string {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	providers := make([]string, 0, len(r.drivers))
	for provider := range r.drivers {
		providers = append(providers, provider)
	}
	r.mu.RUnlock()
	sort.Strings(providers)
	return providers
}

// DatabaseMapValuesToStruct maps result values into an existing Osiris struct
// type. A field's `db` tag name wins, then its `json` tag name, then its source
// field name; `-` excludes a field. Mapping preserves nulls and validates known
// Osiris binding types without introducing an ORM or reflection schema.
func DatabaseMapValuesToStruct(columns []DatabaseColumn, values []Value, target Value) (Value, error) {
	if target.kind != kindStruct || target.structType == nil || target.structValue != nil {
		return nullValue(), databaseError(ErrorKindType, "map_struct", "", false, DatabaseEmbeddedProvider, "target must be an Osiris struct type", nil)
	}
	if len(columns) != len(values) {
		return nullValue(), databaseError(ErrorKindInvalidArgument, "map_struct", "", false, DatabaseEmbeddedProvider, "column and value counts differ", nil)
	}
	byColumn := make(map[string]Value, len(columns))
	for i, column := range columns {
		if _, exists := byColumn[column.Name]; exists {
			return nullValue(), databaseError(ErrorKindInvalidArgument, "map_struct", "", false, DatabaseEmbeddedProvider, fmt.Sprintf("duplicate result column %q", column.Name), nil)
		}
		byColumn[column.Name] = values[i]
	}
	fields := make([]Value, len(target.structType.fields))
	for i, field := range target.structType.fields {
		name := databaseFieldColumn(field)
		if name == "-" {
			fields[i] = nullValue()
			continue
		}
		value, ok := byColumn[name]
		if !ok {
			return nullValue(), databaseError(ErrorKindNotFound, "map_struct", "", false, DatabaseEmbeddedProvider, fmt.Sprintf("result is missing struct field %q", field.name), nil)
		}
		if value.kind != kindNull && (isBindingTypeName(field.typeName) || strings.HasPrefix(field.typeName, "func(")) {
			if _, err := checkBindingType(field.typeName, value, false); err != nil {
				return nullValue(), databaseError(ErrorKindType, "map_struct", "", false, DatabaseEmbeddedProvider, fmt.Sprintf("field %s: %v", field.name, err), err)
			}
		}
		fields[i] = cloneDatabaseValue(value)
	}
	return structInstanceValueOf(target.structType, fields), nil
}

func databaseFieldColumn(field structFieldValue) string {
	for _, namespace := range []string{"db", "json"} {
		if tag, ok := field.tagValues[namespace]; ok {
			name := strings.Split(tag, ",")[0]
			if name != "" {
				return name
			}
		}
	}
	return field.name
}

// DatabaseValueFromGo converts the portable scalar set accepted by drivers to
// an owned Osiris Value. []byte becomes the existing uint8-list byte-buffer
// representation; time.Time and exact decimals retain their native machinery.
func DatabaseValueFromGo(raw any) (Value, error) {
	switch value := raw.(type) {
	case nil:
		return nullValue(), nil
	case Value:
		return cloneDatabaseValue(value), nil
	case bool:
		return boolValue(value), nil
	case string:
		return stringValue(value), nil
	case []byte:
		return bytesListValue(append([]byte(nil), value...)), nil
	case time.Time:
		return dateTimeValueFromTime(value, parsedTimeLocation(value, nil))
	case *big.Rat:
		if value == nil {
			return nullValue(), nil
		}
		return decimalValue(value), nil
	case big.Rat:
		return decimalValue(&value), nil
	case json.Number:
		rat, ok := new(big.Rat).SetString(string(value))
		if !ok {
			return nullValue(), databaseError(ErrorKindType, "value_from_go", "", false, DatabaseEmbeddedProvider, "invalid decimal number", nil)
		}
		return decimalValue(rat), nil
	case int:
		return intValue(big.NewInt(int64(value)), 64), nil
	case int8:
		return intValue(big.NewInt(int64(value)), 8), nil
	case int16:
		return intValue(big.NewInt(int64(value)), 16), nil
	case int32:
		return intValue(big.NewInt(int64(value)), 32), nil
	case int64:
		return intValue(big.NewInt(value), 64), nil
	case uint:
		return uintValue(uint64(value), 64), nil
	case uint8:
		return uintValue(uint64(value), 8), nil
	case uint16:
		return uintValue(uint64(value), 16), nil
	case uint32:
		return uintValue(uint64(value), 32), nil
	case uint64:
		return uintValue(value, 64), nil
	case float32:
		return floatValue(float64(value), 32), nil
	case float64:
		return floatValue(value, 64), nil
	default:
		return nullValue(), databaseError(ErrorKindType, "value_from_go", "", false, DatabaseEmbeddedProvider, fmt.Sprintf("unsupported database value type %T", raw), nil)
	}
}

// DatabaseValueToGo converts an Osiris database value back to a portable Go
// scalar. A uint8 list is copied as []byte, protecting database-owned buffers.
func DatabaseValueToGo(value Value) (any, error) {
	switch value.kind {
	case kindNull:
		return nil, nil
	case kindBool:
		return value.boo, nil
	case kindString:
		return value.str, nil
	case kindNumber:
		return value.num, nil
	case kindInt:
		if value.i == nil || !value.i.IsInt64() {
			return nil, databaseError(ErrorKindOutOfRange, "value_to_go", "", false, DatabaseEmbeddedProvider, "integer does not fit int64", nil)
		}
		return value.i.Int64(), nil
	case kindUint:
		if value.ub != nil {
			if !value.ub.IsUint64() {
				return nil, databaseError(ErrorKindOutOfRange, "value_to_go", "", false, DatabaseEmbeddedProvider, "unsigned integer does not fit uint64", nil)
			}
			return value.ub.Uint64(), nil
		}
		return value.u, nil
	case kindDecimal:
		if value.dec == nil {
			return nil, databaseError(ErrorKindType, "value_to_go", "", false, DatabaseEmbeddedProvider, "decimal has no value", nil)
		}
		return new(big.Rat).Set(value.dec), nil
	case kindDateTime:
		if value.datetime == nil {
			return nil, databaseError(ErrorKindType, "value_to_go", "", false, DatabaseEmbeddedProvider, "datetime has no value", nil)
		}
		return value.datetime.asTime(), nil
	case kindList:
		data, err := bytesFromValue(value)
		if err != nil {
			return nil, databaseError(ErrorKindType, "value_to_go", "", false, DatabaseEmbeddedProvider, "list is not a byte buffer", err)
		}
		return append([]byte(nil), data...), nil
	default:
		return nil, databaseError(ErrorKindType, "value_to_go", "", false, DatabaseEmbeddedProvider, fmt.Sprintf("unsupported Osiris database value kind %s", value.Kind()), nil)
	}
}

func cloneDatabaseValue(value Value) Value {
	switch value.kind {
	case kindList:
		items := make([]Value, len(value.list))
		for i, item := range value.list {
			items[i] = cloneDatabaseValue(item)
		}
		return listValue(items)
	case kindTuple:
		items := make([]Value, len(value.tuple))
		for i, item := range value.tuple {
			items[i] = cloneDatabaseValue(item)
		}
		return tupleValue(items)
	case kindStruct:
		if value.structType == nil {
			return value
		}
		if value.structValue == nil {
			return structTypeValueOf(value.structType)
		}
		fields := make([]Value, len(value.structValue.fields))
		for i, item := range value.structValue.fields {
			fields[i] = cloneDatabaseValue(item)
		}
		return structInstanceValueOf(value.structType, fields)
	case kindDecimal:
		return decimalValue(value.dec)
	case kindDateTime:
		return dateTimeValueOf(value.datetime)
	default:
		return value
	}
}

func databaseQueryIdentity(query string) string {
	sum := sha256.Sum256([]byte(strings.Join(strings.Fields(query), " ")))
	return hex.EncodeToString(sum[:])[:16]
}

func databaseError(kind ErrorKind, operation, query string, retryable bool, provider, message string, cause error) error {
	if kind == "" {
		kind = ErrorKindDatabase
	}
	message = redactErrorText(message)
	record := newOsirisError(kind, message)
	record = record.withMetadata("operation", stringValue(operation))
	record = record.withMetadata("query_identity", stringValue(databaseQueryIdentity(query)))
	record = record.withMetadata("retryable", boolValue(retryable))
	if provider != "" {
		record = record.withMetadata("resource", stringValue(provider))
		record = record.withDebugMetadata("provider", stringValue(provider))
	}
	if query != "" {
		record = record.withDebugMetadata("query_identity", stringValue(databaseQueryIdentity(query)))
	}
	if cause != nil {
		if nested, ok := cause.(*OsirisError); ok {
			record = record.withCause(nested)
		}
	}
	return record
}

func databaseContext(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

func databaseContextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return nil
}

func databaseErrorKind(err error) ErrorKind {
	switch {
	case errors.Is(err, context.Canceled):
		return ErrorKindCancelled
	case errors.Is(err, context.DeadlineExceeded):
		return ErrorKindTimeout
	default:
		return ErrorKindDatabase
	}
}
