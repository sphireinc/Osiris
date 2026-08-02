package osiris

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	databaseDefaultMaxOpen       = 4
	databaseDefaultMaxStatements = 64
	databaseDefaultMaxQueryBytes = 1 << 20
)

var (
	databaseCreateRE = regexp.MustCompile(`(?is)^CREATE\s+TABLE\s+([A-Za-z_][A-Za-z0-9_]*)\s*\((.*)\)$`)
	databaseInsertRE = regexp.MustCompile(`(?is)^INSERT\s+INTO\s+([A-Za-z_][A-Za-z0-9_]*)\s*\(([^)]*)\)\s*VALUES\s*\(([^)]*)\)$`)
	databaseUpdateRE = regexp.MustCompile(`(?is)^UPDATE\s+([A-Za-z_][A-Za-z0-9_]*)\s+SET\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\?\s+WHERE\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\?$`)
	databaseDeleteRE = regexp.MustCompile(`(?is)^DELETE\s+FROM\s+([A-Za-z_][A-Za-z0-9_]*)\s+WHERE\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\?$`)
	databaseSelectRE = regexp.MustCompile(`(?is)^SELECT\s+(.+?)\s+FROM\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+WHERE\s+([A-Za-z_][A-Za-z0-9_]*)\s*=\s*\?)?(?:\s+ORDER\s+BY\s+([A-Za-z_][A-Za-z0-9_]*)(?:\s+(ASC|DESC))?)?$`)
)

// EmbeddedDriver is a deterministic, process-local SQL fixture. It supports
// CREATE TABLE, parameter-only INSERT/UPDATE/DELETE, and SELECT with one
// equality predicate and optional ordering; it is deliberately not an ORM.
type EmbeddedDriver struct {
	state *embeddedDatabaseState
}

// NewEmbeddedDriver constructs an isolated embedded provider with no tables.
// Each driver owns its data; connections opened from the same driver share it.
func NewEmbeddedDriver() *EmbeddedDriver {
	return &EmbeddedDriver{state: &embeddedDatabaseState{tables: make(map[string]*embeddedTable)}}
}

// APIVersion implements DatabaseDriver and returns DatabaseDriverAPIVersion.
func (d *EmbeddedDriver) APIVersion() uint { return DatabaseDriverAPIVersion }

// Provider implements DatabaseDriver and returns DatabaseEmbeddedProvider.
func (d *EmbeddedDriver) Provider() string { return DatabaseEmbeddedProvider }

// Open implements DatabaseDriver. It requires database:open and
// database:embedded capabilities, copies all configuration, and returns an
// independently closable pool-backed connection.
func (d *EmbeddedDriver) Open(ctx context.Context, config DatabaseOpenConfig) (DatabaseConnection, error) {
	if d == nil || d.state == nil {
		return nil, databaseError(ErrorKindInternal, "open", "", false, DatabaseEmbeddedProvider, "embedded driver is not initialized", nil)
	}
	ctx, cancel := databaseContext(ctx, config.OperationTimeout)
	defer cancel()
	if err := databaseContextError(ctx); err != nil {
		return nil, databaseError(databaseErrorKind(err), "open", "", true, DatabaseEmbeddedProvider, "database open was cancelled", nil)
	}
	if !databaseHasCapability(config.Capabilities, DatabaseOpenCapability) || !databaseHasCapability(config.Capabilities, DatabaseEmbeddedCapability) {
		return nil, databaseError(ErrorKindPermissionDenied, "open", "", false, DatabaseEmbeddedProvider, "database capabilities do not authorize the embedded provider", nil)
	}
	limits, err := databaseNormalizeLimits(config.Limits)
	if err != nil {
		return nil, err
	}
	return newEmbeddedConnection(d.state, config, limits), nil
}

func databaseHasCapability(capabilities []string, want string) bool {
	for _, capability := range capabilities {
		if capability == want || capability == "database:*" || capability == "*" {
			return true
		}
	}
	return false
}

func databaseNormalizeLimits(limits DatabaseLimits) (DatabaseLimits, error) {
	if limits.MaxOpenConnections == 0 {
		limits.MaxOpenConnections = databaseDefaultMaxOpen
	}
	if limits.MaxPreparedStatements == 0 {
		limits.MaxPreparedStatements = databaseDefaultMaxStatements
	}
	if limits.MaxQueryBytes == 0 {
		limits.MaxQueryBytes = databaseDefaultMaxQueryBytes
	}
	if limits.MaxOpenConnections < 1 || limits.MaxPreparedStatements < 1 || limits.MaxQueryBytes < 1 || limits.MaxRows < 0 || limits.MaxRowBytes < 0 || limits.MaxIdleConnections < 0 || limits.WaitTimeout < 0 {
		return DatabaseLimits{}, databaseError(ErrorKindInvalidArgument, "open", "", false, DatabaseEmbeddedProvider, "database limits must be non-negative and MaxOpenConnections must be positive", nil)
	}
	if limits.MaxIdleConnections > limits.MaxOpenConnections {
		return DatabaseLimits{}, databaseError(ErrorKindInvalidArgument, "open", "", false, DatabaseEmbeddedProvider, "MaxIdleConnections cannot exceed MaxOpenConnections", nil)
	}
	return limits, nil
}

type embeddedDatabaseState struct {
	mu       sync.RWMutex
	tables   map[string]*embeddedTable
	revision uint64
}

type embeddedTable struct {
	name    string
	columns []embeddedColumn
	rows    [][]Value
}

type embeddedColumn struct {
	name     string
	typ      string
	nullable bool
}

type databasePool struct {
	mu     sync.Mutex
	slots  chan struct{}
	max    int
	inUse  int
	closed bool
}

func newDatabasePool(max int) *databasePool {
	return &databasePool{slots: make(chan struct{}, max), max: max}
}

func (p *databasePool) acquire(ctx context.Context, waitTimeout time.Duration) error {
	if ctx == nil {
		ctx = context.Background()
	}
	waitCtx := ctx
	var cancel context.CancelFunc
	if waitTimeout > 0 {
		waitCtx, cancel = context.WithTimeout(ctx, waitTimeout)
		defer cancel()
	}
	select {
	case p.slots <- struct{}{}:
		p.mu.Lock()
		if p.closed {
			p.mu.Unlock()
			<-p.slots
			return errors.New("database pool is closed")
		}
		p.inUse++
		p.mu.Unlock()
		return nil
	case <-waitCtx.Done():
		return waitCtx.Err()
	}
}

func (p *databasePool) release() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.inUse > 0 {
		p.inUse--
	}
	p.mu.Unlock()
	select {
	case <-p.slots:
	default:
	}
}

func (p *databasePool) close() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
}

func (p *databasePool) stats(prepared int) DatabasePoolStats {
	p.mu.Lock()
	inUse, max := p.inUse, p.max
	p.mu.Unlock()
	return DatabasePoolStats{MaxOpen: max, InUse: inUse, Available: max - inUse, Prepared: prepared}
}

type embeddedConnection struct {
	state  *embeddedDatabaseState
	config DatabaseOpenConfig
	limits DatabaseLimits
	pool   *databasePool

	mu        sync.Mutex
	closed    bool
	prepared  int
	resources map[databaseResource]struct{}
}

type databaseResource interface{ forceClose() }

func newEmbeddedConnection(state *embeddedDatabaseState, config DatabaseOpenConfig, limits DatabaseLimits) *embeddedConnection {
	return &embeddedConnection{
		state: state, config: databaseCopyConfig(config), limits: limits,
		pool: newDatabasePool(limits.MaxOpenConnections), resources: make(map[databaseResource]struct{}),
	}
}

func databaseCopyConfig(config DatabaseOpenConfig) DatabaseOpenConfig {
	copyConfig := config
	copyConfig.Capabilities = append([]string(nil), config.Capabilities...)
	copyConfig.Secrets = make(map[string]string, len(config.Secrets))
	for key, value := range config.Secrets {
		copyConfig.Secrets[key] = value
	}
	return copyConfig
}

func (c *embeddedConnection) ensureOpen(operation, query string) error {
	if c == nil {
		return databaseError(ErrorKindInvalidArgument, operation, query, false, DatabaseEmbeddedProvider, "database connection is nil", nil)
	}
	c.mu.Lock()
	closed := c.closed
	c.mu.Unlock()
	if closed {
		return databaseError(ErrorKindUnavailable, operation, query, true, DatabaseEmbeddedProvider, "database connection is closed", nil)
	}
	return nil
}

func (c *embeddedConnection) beginLease(ctx context.Context, operation, query string) (context.Context, func(), error) {
	if err := c.ensureOpen(operation, query); err != nil {
		return nil, nil, err
	}
	ctx, cancel := databaseContext(ctx, c.config.OperationTimeout)
	if err := c.pool.acquire(ctx, c.limits.WaitTimeout); err != nil {
		cancel()
		kind := databaseErrorKind(err)
		return nil, nil, databaseError(kind, operation, query, kind == ErrorKindTimeout || kind == ErrorKindCancelled, DatabaseEmbeddedProvider, "database pool lease was not available", err)
	}
	release := func() {
		c.pool.release()
		cancel()
	}
	if c.config.OperationDelay > 0 {
		timer := time.NewTimer(c.config.OperationDelay)
		select {
		case <-timer.C:
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			release()
			kind := databaseErrorKind(ctx.Err())
			return nil, nil, databaseError(kind, operation, query, true, DatabaseEmbeddedProvider, "database operation was cancelled", ctx.Err())
		}
	}
	return ctx, release, nil
}

func (c *embeddedConnection) register(resource databaseResource) {
	c.mu.Lock()
	if !c.closed {
		c.resources[resource] = struct{}{}
	}
	c.mu.Unlock()
}

func (c *embeddedConnection) unregister(resource databaseResource) {
	c.mu.Lock()
	delete(c.resources, resource)
	c.mu.Unlock()
}

// Exec implements DatabaseExecutor for the embedded connection.
func (c *embeddedConnection) Exec(ctx context.Context, query string, args []Value) (DatabaseResult, error) {
	ctx, release, err := c.beginLease(ctx, "exec", query)
	if err != nil {
		return DatabaseResult{}, err
	}
	defer release()
	if err := databaseQueryLimit(c.limits, query); err != nil {
		return DatabaseResult{}, err
	}
	if err := databaseContextError(ctx); err != nil {
		return DatabaseResult{}, databaseError(databaseErrorKind(err), "exec", query, true, DatabaseEmbeddedProvider, "database execution was cancelled", nil)
	}
	c.state.mu.Lock()
	result, rawErr := embeddedExec(c.state.tables, query, args, false)
	if rawErr == nil {
		c.state.revision++
	}
	c.state.mu.Unlock()
	if rawErr != nil {
		return DatabaseResult{}, databaseEmbeddedError("exec", query, rawErr)
	}
	return result, nil
}

// Query implements DatabaseExecutor for the embedded connection. The returned
// rows owns the pool lease until Close or natural exhaustion.
func (c *embeddedConnection) Query(ctx context.Context, query string, args []Value) (DatabaseRows, error) {
	ctx, release, err := c.beginLease(ctx, "query", query)
	if err != nil {
		return nil, err
	}
	if err := databaseQueryLimit(c.limits, query); err != nil {
		release()
		return nil, err
	}
	c.state.mu.RLock()
	columns, values, rawErr := embeddedQuery(c.state.tables, query, args)
	c.state.mu.RUnlock()
	if rawErr != nil {
		release()
		return nil, databaseEmbeddedError("query", query, rawErr)
	}
	rows := &embeddedRows{connection: c, columns: columns, rows: values, release: release, maxRows: c.limits.MaxRows, maxBytes: c.limits.MaxRowBytes}
	c.register(rows)
	return rows, nil
}

// Prepare implements DatabaseExecutor and enforces the configured statement
// count before retaining the query.
func (c *embeddedConnection) Prepare(ctx context.Context, query string) (DatabaseStatement, error) {
	ctx, cancel := databaseContext(ctx, c.config.OperationTimeout)
	defer cancel()
	if err := c.ensureOpen("prepare", query); err != nil {
		return nil, err
	}
	if err := databaseQueryLimit(c.limits, query); err != nil {
		return nil, err
	}
	if err := databaseContextError(ctx); err != nil {
		return nil, databaseError(databaseErrorKind(err), "prepare", query, true, DatabaseEmbeddedProvider, "statement preparation was cancelled", nil)
	}
	if !databaseStatementShapeSupported(query) {
		return nil, databaseEmbeddedError("prepare", query, errors.New("unsupported embedded statement shape"))
	}
	c.mu.Lock()
	if c.prepared >= c.limits.MaxPreparedStatements {
		c.mu.Unlock()
		return nil, databaseError(ErrorKindResourceExhausted, "prepare", query, true, DatabaseEmbeddedProvider, "prepared statement limit exceeded", nil)
	}
	c.prepared++
	statement := &embeddedStatement{connection: c, query: query}
	c.resources[statement] = struct{}{}
	c.mu.Unlock()
	return statement, nil
}

// Begin implements DatabaseConnection and creates a snapshot transaction.
func (c *embeddedConnection) Begin(ctx context.Context, options DatabaseTxOptions) (DatabaseTransaction, error) {
	ctx, release, err := c.beginLease(ctx, "begin", "")
	if err != nil {
		return nil, err
	}
	if options.Isolation == "" {
		options.Isolation = DatabaseIsolationDefault
	}
	c.state.mu.RLock()
	tables, revision := cloneDatabaseTables(c.state.tables), c.state.revision
	c.state.mu.RUnlock()
	tx := &embeddedTransaction{connection: c, state: c.state, tables: tables, baseRevision: revision, options: options, release: release}
	c.register(tx)
	return tx, nil
}

// Stats implements DatabaseConnection.
func (c *embeddedConnection) Stats() DatabasePoolStats {
	if c == nil {
		return DatabasePoolStats{}
	}
	c.mu.Lock()
	prepared := c.prepared
	c.mu.Unlock()
	return c.pool.stats(prepared)
}

// Close implements DatabaseConnection and closes all tracked resources.
func (c *embeddedConnection) Close() error {
	if c == nil {
		return nil
	}
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil
	}
	c.closed = true
	resources := make([]databaseResource, 0, len(c.resources))
	for resource := range c.resources {
		resources = append(resources, resource)
	}
	c.mu.Unlock()
	c.pool.close()
	for _, resource := range resources {
		resource.forceClose()
	}
	return nil
}

type embeddedStatement struct {
	connection  *embeddedConnection
	transaction *embeddedTransaction
	query       string
	mu          sync.Mutex
	closed      bool
}

// Exec implements DatabaseExecutor for a prepared statement.
func (s *embeddedStatement) Exec(ctx context.Context, args []Value) (DatabaseResult, error) {
	if err := s.ensureOpen(); err != nil {
		return DatabaseResult{}, err
	}
	if s.transaction != nil {
		return s.transaction.Exec(ctx, s.query, args)
	}
	return s.connection.Exec(ctx, s.query, args)
}

// Query implements DatabaseExecutor for a prepared statement.
func (s *embeddedStatement) Query(ctx context.Context, args []Value) (DatabaseRows, error) {
	if err := s.ensureOpen(); err != nil {
		return nil, err
	}
	if s.transaction != nil {
		return s.transaction.Query(ctx, s.query, args)
	}
	return s.connection.Query(ctx, s.query, args)
}

// QueryText implements DatabaseStatement.
func (s *embeddedStatement) QueryText() string { return s.query }

// Close implements DatabaseStatement and releases the statement count.
func (s *embeddedStatement) Close() error {
	if s == nil || s.connection == nil {
		return nil
	}
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()
	s.connection.mu.Lock()
	if s.connection.prepared > 0 {
		s.connection.prepared--
	}
	delete(s.connection.resources, s)
	s.connection.mu.Unlock()
	return nil
}

func (s *embeddedStatement) forceClose() { _ = s.Close() }

func (s *embeddedStatement) ensureOpen() error {
	if s == nil || s.connection == nil {
		return databaseError(ErrorKindInvalidArgument, "statement", s.query, false, DatabaseEmbeddedProvider, "statement is nil", nil)
	}
	s.mu.Lock()
	closed := s.closed
	s.mu.Unlock()
	if closed {
		return databaseError(ErrorKindUnavailable, "statement", s.query, true, DatabaseEmbeddedProvider, "statement is closed", nil)
	}
	return s.connection.ensureOpen("statement", s.query)
}

type embeddedRows struct {
	connection *embeddedConnection
	columns    []DatabaseColumn
	rows       [][]Value
	release    func()
	maxRows    int
	maxBytes   int64

	mu      sync.Mutex
	index   int
	current []Value
	err     error
	closed  bool
}

// Columns implements DatabaseRows and returns copied metadata.
func (r *embeddedRows) Columns() []DatabaseColumn {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	columns := append([]DatabaseColumn(nil), r.columns...)
	r.mu.Unlock()
	return columns
}

// Next implements DatabaseRows. Limits are checked before yielding the row
// that would exceed them, so callers never receive an unbounded result.
func (r *embeddedRows) Next(ctx context.Context) (bool, error) {
	if r == nil {
		return false, databaseError(ErrorKindInvalidArgument, "next", "", false, DatabaseEmbeddedProvider, "rows is nil", nil)
	}
	if err := databaseContextError(ctx); err != nil {
		r.mu.Lock()
		r.err = databaseError(databaseErrorKind(err), "next", "", true, DatabaseEmbeddedProvider, "row iteration was cancelled", nil)
		r.closeLocked()
		err = r.err
		r.mu.Unlock()
		return false, err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.closed {
		if r.err != nil {
			return false, r.err
		}
		return false, nil
	}
	if r.index >= len(r.rows) {
		r.closeLocked()
		return false, nil
	}
	if r.maxRows > 0 && r.index >= r.maxRows {
		r.err = databaseError(ErrorKindResourceExhausted, "next", "", true, DatabaseEmbeddedProvider, "row limit exceeded", nil)
		r.closeLocked()
		return false, r.err
	}
	row := r.rows[r.index]
	if r.maxBytes > 0 && databaseRowBytes(row) > r.maxBytes {
		r.err = databaseError(ErrorKindResourceExhausted, "next", "", false, DatabaseEmbeddedProvider, "row byte limit exceeded", nil)
		r.closeLocked()
		return false, r.err
	}
	r.current = cloneDatabaseRow(row)
	r.index++
	return true, nil
}

// Values implements DatabaseRows and returns a defensive copy of the current
// row. It returns nil before the first successful Next call.
func (r *embeddedRows) Values() []Value {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	values := cloneDatabaseRow(r.current)
	r.mu.Unlock()
	return values
}

// Err implements DatabaseRows.
func (r *embeddedRows) Err() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	err := r.err
	r.mu.Unlock()
	return err
}

// Close implements DatabaseRows and releases its pool lease exactly once.
func (r *embeddedRows) Close() error {
	if r == nil {
		return nil
	}
	r.mu.Lock()
	r.closeLocked()
	r.mu.Unlock()
	return nil
}

func (r *embeddedRows) closeLocked() {
	if r.closed {
		return
	}
	r.closed = true
	if r.release != nil {
		r.release()
		r.release = nil
	}
	if r.connection != nil {
		r.connection.unregister(r)
	}
}

func (r *embeddedRows) forceClose() { _ = r.Close() }

type embeddedTransaction struct {
	connection   *embeddedConnection
	state        *embeddedDatabaseState
	tables       map[string]*embeddedTable
	baseRevision uint64
	options      DatabaseTxOptions
	release      func()
	mu           sync.Mutex
	closed       bool
}

// Exec implements DatabaseExecutor for a snapshot transaction.
func (t *embeddedTransaction) Exec(ctx context.Context, query string, args []Value) (DatabaseResult, error) {
	if err := t.ensureOpen("exec", query); err != nil {
		return DatabaseResult{}, err
	}
	ctx, cancel := databaseContext(ctx, t.connection.config.OperationTimeout)
	defer cancel()
	if err := databaseContextError(ctx); err != nil {
		return DatabaseResult{}, databaseError(databaseErrorKind(err), "exec", query, true, DatabaseEmbeddedProvider, "transaction execution was cancelled", nil)
	}
	if t.options.ReadOnly && embeddedQueryMutates(query) {
		return DatabaseResult{}, databaseError(ErrorKindPermissionDenied, "exec", query, false, DatabaseEmbeddedProvider, "read-only transaction cannot mutate", nil)
	}
	if err := databaseQueryLimit(t.connection.limits, query); err != nil {
		return DatabaseResult{}, err
	}
	t.mu.Lock()
	result, rawErr := embeddedExec(t.tables, query, args, true)
	t.mu.Unlock()
	if rawErr != nil {
		return DatabaseResult{}, databaseEmbeddedError("exec", query, rawErr)
	}
	return result, nil
}

// Query implements DatabaseExecutor for a snapshot transaction.
func (t *embeddedTransaction) Query(ctx context.Context, query string, args []Value) (DatabaseRows, error) {
	if err := t.ensureOpen("query", query); err != nil {
		return nil, err
	}
	ctx, cancel := databaseContext(ctx, t.connection.config.OperationTimeout)
	defer cancel()
	if err := databaseContextError(ctx); err != nil {
		return nil, databaseError(databaseErrorKind(err), "query", query, true, DatabaseEmbeddedProvider, "transaction query was cancelled", nil)
	}
	if err := databaseQueryLimit(t.connection.limits, query); err != nil {
		return nil, err
	}
	t.mu.Lock()
	columns, values, rawErr := embeddedQuery(t.tables, query, args)
	t.mu.Unlock()
	if rawErr != nil {
		return nil, databaseEmbeddedError("query", query, rawErr)
	}
	rows := &embeddedRows{connection: t.connection, columns: columns, rows: values, maxRows: t.connection.limits.MaxRows, maxBytes: t.connection.limits.MaxRowBytes}
	t.connection.register(rows)
	return rows, nil
}

// Prepare implements DatabaseExecutor for a transaction by retaining a
// statement that executes against this transaction's snapshot.
func (t *embeddedTransaction) Prepare(ctx context.Context, query string) (DatabaseStatement, error) {
	if err := t.ensureOpen("prepare", query); err != nil {
		return nil, err
	}
	statement, err := t.connection.Prepare(ctx, query)
	if err != nil {
		return nil, err
	}
	prepared := statement.(*embeddedStatement)
	prepared.transaction = t
	return prepared, nil
}

// Commit implements DatabaseTransaction using optimistic snapshot conflict
// detection. A concurrent committed writer causes a retryable conflict.
func (t *embeddedTransaction) Commit(ctx context.Context) error {
	if err := t.ensureOpen("commit", ""); err != nil {
		return err
	}
	ctx, cancel := databaseContext(ctx, t.connection.config.OperationTimeout)
	defer cancel()
	if err := databaseContextError(ctx); err != nil {
		return databaseError(databaseErrorKind(err), "commit", "", true, DatabaseEmbeddedProvider, "transaction commit was cancelled", nil)
	}
	t.state.mu.Lock()
	if t.state.revision != t.baseRevision {
		t.state.mu.Unlock()
		return databaseError(ErrorKindConflict, "commit", "", true, DatabaseEmbeddedProvider, "transaction snapshot is stale", nil)
	}
	t.state.tables = cloneDatabaseTables(t.tables)
	t.state.revision++
	t.state.mu.Unlock()
	t.finish()
	return nil
}

// Rollback implements DatabaseTransaction and discards the private snapshot.
func (t *embeddedTransaction) Rollback(ctx context.Context) error {
	if t == nil {
		return nil
	}
	if err := databaseContextError(ctx); err != nil {
		return databaseError(databaseErrorKind(err), "rollback", "", true, DatabaseEmbeddedProvider, "transaction rollback was cancelled", nil)
	}
	t.finish()
	return nil
}

// Close implements DatabaseTransaction and rolls back unfinished work.
func (t *embeddedTransaction) Close() error {
	if t == nil {
		return nil
	}
	t.finish()
	return nil
}

func (t *embeddedTransaction) finish() {
	t.mu.Lock()
	if t.closed {
		t.mu.Unlock()
		return
	}
	t.closed = true
	release := t.release
	t.release = nil
	t.mu.Unlock()
	if release != nil {
		release()
	}
	if t.connection != nil {
		t.connection.unregister(t)
	}
}

func (t *embeddedTransaction) forceClose() { _ = t.Close() }

func (t *embeddedTransaction) ensureOpen(operation, query string) error {
	if t == nil || t.connection == nil {
		return databaseError(ErrorKindInvalidArgument, operation, query, false, DatabaseEmbeddedProvider, "transaction is nil", nil)
	}
	t.mu.Lock()
	closed := t.closed
	t.mu.Unlock()
	if closed {
		return databaseError(ErrorKindUnavailable, operation, query, true, DatabaseEmbeddedProvider, "transaction is closed", nil)
	}
	return t.connection.ensureOpen(operation, query)
}

func databaseQueryLimit(limits DatabaseLimits, query string) error {
	if limits.MaxQueryBytes > 0 && int64(len([]byte(query))) > limits.MaxQueryBytes {
		return databaseError(ErrorKindResourceExhausted, "query", query, false, DatabaseEmbeddedProvider, "query byte limit exceeded", nil)
	}
	return nil
}

func databaseStatementShapeSupported(query string) bool {
	query = databaseNormalizeQuery(query)
	return databaseCreateRE.MatchString(query) || databaseInsertRE.MatchString(query) || databaseUpdateRE.MatchString(query) || databaseDeleteRE.MatchString(query) || databaseSelectRE.MatchString(query)
}

func databaseNormalizeQuery(query string) string {
	query = strings.TrimSpace(query)
	query = strings.TrimSuffix(query, ";")
	return strings.Join(strings.Fields(query), " ")
}

func databaseEmbeddedError(operation, query string, raw error) error {
	if raw == nil {
		return nil
	}
	if osiris, ok := raw.(*OsirisError); ok {
		return osiris
	}
	return databaseError(ErrorKindInvalidArgument, operation, query, false, DatabaseEmbeddedProvider, redactErrorText(raw.Error()), nil)
}

func embeddedExec(tables map[string]*embeddedTable, query string, args []Value, inTransaction bool) (DatabaseResult, error) {
	query = databaseNormalizeQuery(query)
	if matches := databaseCreateRE.FindStringSubmatch(query); matches != nil {
		return embeddedCreate(tables, matches[1], matches[2])
	}
	if matches := databaseInsertRE.FindStringSubmatch(query); matches != nil {
		return embeddedInsert(tables, matches[1], matches[2], matches[3], args)
	}
	if matches := databaseUpdateRE.FindStringSubmatch(query); matches != nil {
		return embeddedUpdate(tables, matches[1], matches[2], matches[3], args)
	}
	if matches := databaseDeleteRE.FindStringSubmatch(query); matches != nil {
		return embeddedDelete(tables, matches[1], matches[2], args)
	}
	if embeddedQueryMutates(query) && inTransaction {
		return DatabaseResult{}, errors.New("unsupported mutating statement")
	}
	return DatabaseResult{}, errors.New("unsupported embedded execute statement")
}

func embeddedCreate(tables map[string]*embeddedTable, name, definition string) (DatabaseResult, error) {
	if _, exists := tables[name]; exists {
		return DatabaseResult{}, fmt.Errorf("table %s already exists", name)
	}
	parts, err := databaseSplitCSV(definition)
	if err != nil || len(parts) == 0 {
		return DatabaseResult{}, errors.New("malformed CREATE TABLE definition")
	}
	table := &embeddedTable{name: name, columns: make([]embeddedColumn, 0, len(parts))}
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		fields := strings.Fields(part)
		if len(fields) < 2 {
			return DatabaseResult{}, fmt.Errorf("malformed column definition %q", part)
		}
		column := embeddedColumn{name: fields[0], typ: strings.ToLower(fields[1]), nullable: true}
		if _, exists := seen[column.name]; exists {
			return DatabaseResult{}, fmt.Errorf("duplicate column %s", column.name)
		}
		seen[column.name] = struct{}{}
		for _, field := range fields[2:] {
			if strings.EqualFold(field, "NOT") {
				column.nullable = false
			}
		}
		table.columns = append(table.columns, column)
	}
	tables[name] = table
	return DatabaseResult{}, nil
}

func embeddedInsert(tables map[string]*embeddedTable, name, columnText, placeholderText string, args []Value) (DatabaseResult, error) {
	table, ok := tables[name]
	if !ok {
		return DatabaseResult{}, fmt.Errorf("table %s does not exist", name)
	}
	columns, err := databaseIdentifiers(columnText)
	if err != nil || len(columns) != len(args) || !databaseAllPlaceholders(placeholderText, len(args)) {
		return DatabaseResult{}, errors.New("INSERT requires one ? parameter for each listed column")
	}
	row := make([]Value, len(table.columns))
	for i := range row {
		row[i] = nullValue()
	}
	for i, name := range columns {
		index := embeddedColumnIndex(table, name)
		if index < 0 {
			return DatabaseResult{}, fmt.Errorf("unknown column %s", name)
		}
		if args[i].kind == kindNull && !table.columns[index].nullable {
			return DatabaseResult{}, fmt.Errorf("column %s is not nullable", name)
		}
		if args[i].kind != kindNull && !databaseValueMatchesType(args[i], table.columns[index].typ) {
			return DatabaseResult{}, fmt.Errorf("value for %s does not match %s", name, table.columns[index].typ)
		}
		row[index] = cloneDatabaseValue(args[i])
	}
	table.rows = append(table.rows, row)
	return DatabaseResult{RowsAffected: 1}, nil
}

func embeddedUpdate(tables map[string]*embeddedTable, name, setColumn, whereColumn string, args []Value) (DatabaseResult, error) {
	table, ok := tables[name]
	if !ok {
		return DatabaseResult{}, fmt.Errorf("table %s does not exist", name)
	}
	if len(args) != 2 {
		return DatabaseResult{}, errors.New("UPDATE requires two parameters")
	}
	setIndex, whereIndex := embeddedColumnIndex(table, setColumn), embeddedColumnIndex(table, whereColumn)
	if setIndex < 0 || whereIndex < 0 {
		return DatabaseResult{}, errors.New("UPDATE references an unknown column")
	}
	if args[0].kind != kindNull && !databaseValueMatchesType(args[0], table.columns[setIndex].typ) {
		return DatabaseResult{}, fmt.Errorf("value for %s does not match %s", setColumn, table.columns[setIndex].typ)
	}
	var affected int64
	for i := range table.rows {
		if table.rows[i][whereIndex].Equal(args[1]) {
			table.rows[i][setIndex] = cloneDatabaseValue(args[0])
			affected++
		}
	}
	return DatabaseResult{RowsAffected: affected}, nil
}

func embeddedDelete(tables map[string]*embeddedTable, name, whereColumn string, args []Value) (DatabaseResult, error) {
	table, ok := tables[name]
	if !ok {
		return DatabaseResult{}, fmt.Errorf("table %s does not exist", name)
	}
	if len(args) != 1 {
		return DatabaseResult{}, errors.New("DELETE requires one parameter")
	}
	whereIndex := embeddedColumnIndex(table, whereColumn)
	if whereIndex < 0 {
		return DatabaseResult{}, errors.New("DELETE references an unknown column")
	}
	kept := table.rows[:0]
	var affected int64
	for _, row := range table.rows {
		if row[whereIndex].Equal(args[0]) {
			affected++
			continue
		}
		kept = append(kept, row)
	}
	table.rows = kept
	return DatabaseResult{RowsAffected: affected}, nil
}

func embeddedQuery(tables map[string]*embeddedTable, query string, args []Value) ([]DatabaseColumn, [][]Value, error) {
	query = databaseNormalizeQuery(query)
	matches := databaseSelectRE.FindStringSubmatch(query)
	if matches == nil {
		return nil, nil, errors.New("unsupported embedded SELECT statement")
	}
	selectedText, tableName, whereColumn, orderColumn, orderDirection := matches[1], matches[2], matches[3], matches[4], matches[5]
	table, ok := tables[tableName]
	if !ok {
		return nil, nil, fmt.Errorf("table %s does not exist", tableName)
	}
	var selected []int
	if strings.TrimSpace(selectedText) == "*" {
		selected = make([]int, len(table.columns))
		for i := range selected {
			selected[i] = i
		}
	} else {
		identifiers, err := databaseIdentifiers(selectedText)
		if err != nil {
			return nil, nil, err
		}
		for _, identifier := range identifiers {
			index := embeddedColumnIndex(table, identifier)
			if index < 0 {
				return nil, nil, fmt.Errorf("unknown selected column %s", identifier)
			}
			selected = append(selected, index)
		}
	}
	if whereColumn != "" && len(args) != 1 {
		return nil, nil, errors.New("SELECT WHERE requires one parameter")
	}
	if whereColumn == "" && len(args) != 0 {
		return nil, nil, errors.New("SELECT has unused parameters")
	}
	whereIndex := -1
	if whereColumn != "" {
		whereIndex = embeddedColumnIndex(table, whereColumn)
		if whereIndex < 0 {
			return nil, nil, fmt.Errorf("unknown WHERE column %s", whereColumn)
		}
	}
	rows := make([][]Value, 0, len(table.rows))
	for _, row := range table.rows {
		if whereIndex >= 0 && !row[whereIndex].Equal(args[0]) {
			continue
		}
		selectedRow := make([]Value, len(selected))
		for i, index := range selected {
			selectedRow[i] = cloneDatabaseValue(row[index])
		}
		rows = append(rows, selectedRow)
	}
	if orderColumn != "" {
		orderIndex := -1
		for i, index := range selected {
			if table.columns[index].name == orderColumn {
				orderIndex = i
				break
			}
		}
		if orderIndex < 0 {
			return nil, nil, fmt.Errorf("ORDER BY column %s is not selected", orderColumn)
		}
		sort.SliceStable(rows, func(i, j int) bool {
			cmp, err := rows[i][orderIndex].Compare(rows[j][orderIndex])
			if err != nil {
				return false
			}
			if strings.EqualFold(orderDirection, "DESC") {
				return cmp > 0
			}
			return cmp < 0
		})
	}
	columns := make([]DatabaseColumn, len(selected))
	for i, index := range selected {
		column := table.columns[index]
		columns[i] = DatabaseColumn{Name: column.name, DatabaseType: column.typ, Nullable: column.nullable}
	}
	return columns, rows, nil
}

func embeddedQueryMutates(query string) bool {
	fields := strings.Fields(databaseNormalizeQuery(query))
	if len(fields) == 0 {
		return false
	}
	switch strings.ToUpper(fields[0]) {
	case "CREATE", "INSERT", "UPDATE", "DELETE", "DROP", "ALTER":
		return true
	default:
		return false
	}
}

func embeddedColumnIndex(table *embeddedTable, name string) int {
	for i, column := range table.columns {
		if column.name == name {
			return i
		}
	}
	return -1
}

func databaseValueMatchesType(value Value, typ string) bool {
	typ = strings.ToLower(typ)
	switch {
	case strings.Contains(typ, "bool"):
		return value.kind == kindBool
	case strings.Contains(typ, "char"), strings.Contains(typ, "text"), strings.Contains(typ, "string"):
		return value.kind == kindString
	case strings.Contains(typ, "blob"), strings.Contains(typ, "byte"):
		_, err := bytesFromValue(value)
		return err == nil
	case strings.Contains(typ, "real"), strings.Contains(typ, "float"), strings.Contains(typ, "double"):
		return value.kind == kindNumber || value.kind == kindDecimal || value.kind == kindInt || value.kind == kindUint
	case strings.Contains(typ, "decimal"), strings.Contains(typ, "numeric"):
		return value.kind == kindDecimal || value.kind == kindInt || value.kind == kindUint || value.kind == kindNumber
	case strings.Contains(typ, "time"), strings.Contains(typ, "date"):
		return value.kind == kindDateTime || value.kind == kindString
	case strings.Contains(typ, "int"):
		return value.kind == kindInt || value.kind == kindUint || value.kind == kindNumber
	default:
		return true
	}
}

func databaseAllPlaceholders(text string, count int) bool {
	parts := strings.Split(text, ",")
	if len(parts) != count {
		return false
	}
	for _, part := range parts {
		if strings.TrimSpace(part) != "?" {
			return false
		}
	}
	return true
}

func databaseIdentifiers(text string) ([]string, error) {
	parts := strings.Split(text, ",")
	identifiers := make([]string, 0, len(parts))
	seen := make(map[string]struct{}, len(parts))
	for _, part := range parts {
		identifier := strings.TrimSpace(part)
		if !regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`).MatchString(identifier) {
			return nil, fmt.Errorf("invalid identifier %q", identifier)
		}
		if _, exists := seen[identifier]; exists {
			return nil, fmt.Errorf("duplicate identifier %s", identifier)
		}
		seen[identifier] = struct{}{}
		identifiers = append(identifiers, identifier)
	}
	return identifiers, nil
}

func databaseSplitCSV(text string) ([]string, error) {
	var parts []string
	start, depth := 0, 0
	for i, char := range text {
		switch char {
		case '(':
			depth++
		case ')':
			depth--
		case ',':
			if depth == 0 {
				parts = append(parts, strings.TrimSpace(text[start:i]))
				start = i + 1
			}
		}
		if depth < 0 {
			return nil, errors.New("unbalanced parentheses")
		}
	}
	if depth != 0 {
		return nil, errors.New("unbalanced parentheses")
	}
	parts = append(parts, strings.TrimSpace(text[start:]))
	return parts, nil
}

func cloneDatabaseTables(source map[string]*embeddedTable) map[string]*embeddedTable {
	out := make(map[string]*embeddedTable, len(source))
	for name, table := range source {
		copyTable := &embeddedTable{name: table.name, columns: append([]embeddedColumn(nil), table.columns...), rows: make([][]Value, len(table.rows))}
		for i, row := range table.rows {
			copyTable.rows[i] = cloneDatabaseRow(row)
		}
		out[name] = copyTable
	}
	return out
}

func cloneDatabaseRow(row []Value) []Value {
	if row == nil {
		return nil
	}
	out := make([]Value, len(row))
	for i, value := range row {
		out[i] = cloneDatabaseValue(value)
	}
	return out
}

func databaseRowBytes(row []Value) int64 {
	var total int64
	for _, value := range row {
		switch value.kind {
		case kindString:
			total += int64(len(value.str))
		case kindList:
			if data, err := bytesFromValue(value); err == nil {
				total += int64(len(data))
			} else {
				total += int64(len(value.list)) * 8
			}
		default:
			total += int64(len(value.String()))
		}
	}
	return total
}

var _ DatabaseDriver = (*EmbeddedDriver)(nil)
var _ DatabaseConnection = (*embeddedConnection)(nil)
var _ DatabaseTransaction = (*embeddedTransaction)(nil)
var _ DatabaseStatement = (*embeddedStatement)(nil)
var _ DatabaseRows = (*embeddedRows)(nil)
