package osiris

import (
	"context"
	"errors"
	"math/big"
	"sync"
	"testing"
	"time"
)

func task121Open(t *testing.T, driver *EmbeddedDriver, limits DatabaseLimits, extras ...func(*DatabaseOpenConfig)) DatabaseConnection {
	t.Helper()
	config := DatabaseOpenConfig{
		Capabilities: []string{DatabaseOpenCapability, DatabaseEmbeddedCapability},
		Limits:       limits,
	}
	for _, extra := range extras {
		if extra != nil {
			extra(&config)
		}
	}
	connection, err := driver.Open(context.Background(), config)
	if err != nil {
		t.Fatalf("open embedded database: %v", err)
	}
	t.Cleanup(func() { _ = connection.Close() })
	return connection
}

func task121Exec(t *testing.T, connection DatabaseConnection, query string, args ...Value) DatabaseResult {
	t.Helper()
	result, err := connection.Exec(context.Background(), query, args)
	if err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
	return result
}

func task121UserTable(t *testing.T, connection DatabaseConnection) {
	t.Helper()
	task121Exec(t, connection, "CREATE TABLE users (id INTEGER NOT NULL, name TEXT, payload BLOB, score DECIMAL)")
}

func TestTask121EmbeddedDriverOpenExecuteQueryPreparedRowsAndTypedValues(t *testing.T) {
	driver := NewEmbeddedDriver()
	registry := NewDatabaseDriverRegistry()
	if err := registry.Register(driver); err != nil {
		t.Fatalf("register: %v", err)
	}
	if got := registry.Providers(); len(got) != 1 || got[0] != DatabaseEmbeddedProvider {
		t.Fatalf("providers = %v", got)
	}
	connection, err := registry.Open(context.Background(), DatabaseEmbeddedProvider, DatabaseOpenConfig{
		Capabilities: []string{DatabaseOpenCapability, DatabaseEmbeddedCapability},
	})
	if err != nil {
		t.Fatalf("registry open: %v", err)
	}
	defer connection.Close()
	task121UserTable(t, connection)
	statement, err := connection.Prepare(context.Background(), "INSERT INTO users (id, name, payload, score) VALUES (?, ?, ?, ?)")
	if err != nil {
		t.Fatalf("prepare: %v", err)
	}
	if _, err := statement.Exec(context.Background(), []Value{
		intValue(big.NewInt(2), 64), stringValue("Ada"), bytesListValue([]byte{1, 2, 3}), decimalValue(big.NewRat(7, 2)),
	}); err != nil {
		t.Fatalf("prepared exec: %v", err)
	}
	if got := statement.QueryText(); got == "" {
		t.Fatal("prepared statement lost query text")
	}
	if err := statement.Close(); err != nil {
		t.Fatalf("statement close: %v", err)
	}

	result := task121Exec(t, connection, "INSERT INTO users (id, name, payload, score) VALUES (?, ?, ?, ?)",
		intValue(big.NewInt(1), 64), stringValue("Grace"), bytesListValue([]byte{9}), decimalValue(big.NewRat(5, 2)))
	if result.RowsAffected != 1 {
		t.Fatalf("rows affected = %d", result.RowsAffected)
	}
	rows, err := connection.Query(context.Background(), "SELECT id, name, payload, score FROM users ORDER BY id", nil)
	if err != nil {
		t.Fatalf("query: %v", err)
	}
	defer rows.Close()
	columns := rows.Columns()
	if len(columns) != 4 || columns[0].Name != "id" || columns[2].DatabaseType != "blob" {
		t.Fatalf("columns = %+v", columns)
	}
	if ok, err := rows.Next(context.Background()); err != nil || !ok {
		t.Fatalf("first next = %v, %v", ok, err)
	}
	first := rows.Values()
	if got, _ := first[0].Int(); got != 1 || first[1].String() != `"Grace"` {
		t.Fatalf("first row = %v", first)
	}
	first[2].list[0] = uintValue(99, 8)
	if ok, err := rows.Next(context.Background()); err != nil || !ok {
		t.Fatalf("second next = %v, %v", ok, err)
	}
	second := rows.Values()
	data, err := bytesFromValue(second[2])
	if err != nil || len(data) != 3 || data[0] != 1 {
		t.Fatalf("second bytes = %v, %v", data, err)
	}
	if ok, err := rows.Next(context.Background()); err != nil || ok {
		t.Fatalf("end next = %v, %v", ok, err)
	}
}

func TestTask121TypedMappingAndGoValueRoundTrip(t *testing.T) {
	decimal := big.NewRat(11, 4)
	inputs := []any{nil, true, "text", []byte{1, 2}, time.Unix(10, 20).UTC(), decimal, int32(-4), uint16(9), float32(1.5)}
	for _, input := range inputs {
		value, err := DatabaseValueFromGo(input)
		if err != nil {
			t.Fatalf("from %T: %v", input, err)
		}
		if _, err := DatabaseValueToGo(value); err != nil {
			t.Fatalf("to %T: %v", input, err)
		}
	}
	typ := &structTypeValue{
		name: "User",
		fields: []structFieldValue{
			{name: "identifier", typeName: "i64", tagValues: map[string]string{"db": "user_id"}},
			{name: "displayName", typeName: "string", tagValues: map[string]string{"json": "display_name"}},
		},
		byName: map[string]int{"identifier": 0, "displayName": 1},
	}
	mapped, err := DatabaseMapValuesToStruct(
		[]DatabaseColumn{{Name: "user_id", DatabaseType: "integer"}, {Name: "display_name", DatabaseType: "text"}},
		[]Value{intValue(big.NewInt(8), 64), stringValue("Lin")},
		structTypeValueOf(typ),
	)
	if err != nil {
		t.Fatalf("map struct: %v", err)
	}
	if mapped.structValue == nil || mapped.structValue.fields[1].str != "Lin" {
		t.Fatalf("mapped struct = %s", mapped.String())
	}
}

func TestTask121TransactionsCommitRollbackCloseAndConflict(t *testing.T) {
	driver := NewEmbeddedDriver()
	connection := task121Open(t, driver, DatabaseLimits{})
	task121UserTable(t, connection)
	tx, err := connection.Begin(context.Background(), DatabaseTxOptions{Isolation: DatabaseIsolationSerializable})
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	if _, err := tx.Exec(context.Background(), "INSERT INTO users (id, name, payload, score) VALUES (?, ?, ?, ?)", []Value{intValue(big.NewInt(1), 64), stringValue("rollback"), nullValue(), nullValue()}); err != nil {
		t.Fatalf("tx insert: %v", err)
	}
	if err := tx.Rollback(context.Background()); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if err := tx.Close(); err != nil {
		t.Fatalf("rollback close: %v", err)
	}

	committed, err := connection.Begin(context.Background(), DatabaseTxOptions{})
	if err != nil {
		t.Fatalf("begin committed: %v", err)
	}
	if _, err := committed.Exec(context.Background(), "INSERT INTO users (id, name, payload, score) VALUES (?, ?, ?, ?)", []Value{intValue(big.NewInt(2), 64), stringValue("commit"), nullValue(), nullValue()}); err != nil {
		t.Fatalf("commit insert: %v", err)
	}
	if err := committed.Commit(context.Background()); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if connection.Stats().InUse != 0 {
		t.Fatalf("pool after commit = %+v", connection.Stats())
	}

	first, err := connection.Begin(context.Background(), DatabaseTxOptions{})
	if err != nil {
		t.Fatalf("first conflict begin: %v", err)
	}
	second, err := connection.Begin(context.Background(), DatabaseTxOptions{})
	if err != nil {
		t.Fatalf("second conflict begin: %v", err)
	}
	if _, err := first.Exec(context.Background(), "INSERT INTO users (id, name, payload, score) VALUES (?, ?, ?, ?)", []Value{intValue(big.NewInt(3), 64), stringValue("first"), nullValue(), nullValue()}); err != nil {
		t.Fatalf("first conflict insert: %v", err)
	}
	if err := first.Commit(context.Background()); err != nil {
		t.Fatalf("first conflict commit: %v", err)
	}
	if err := second.Commit(context.Background()); err == nil {
		t.Fatal("stale transaction committed")
	} else if record, ok := err.(*OsirisError); !ok || record.kind != ErrorKindConflict || record.retryable == nil || !*record.retryable {
		t.Fatalf("conflict error = %#v", err)
	}
	if err := second.Close(); err != nil {
		t.Fatalf("stale close: %v", err)
	}
}

func TestTask121CancellationPoolLimitsAndCleanup(t *testing.T) {
	driver := NewEmbeddedDriver()
	connection := task121Open(t, driver, DatabaseLimits{MaxOpenConnections: 1, MaxRows: 1, MaxPreparedStatements: 1}, func(config *DatabaseOpenConfig) {
		config.OperationDelay = 40 * time.Millisecond
	})
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if _, err := connection.Exec(ctx, "CREATE TABLE cancelled (id INTEGER)", nil); err == nil {
		t.Fatal("cancelled operation succeeded")
	} else if record, ok := err.(*OsirisError); !ok || (record.kind != ErrorKindTimeout && record.kind != ErrorKindCancelled) {
		t.Fatalf("cancel error = %#v", err)
	}
	configDriver := NewEmbeddedDriver()
	plain := task121Open(t, configDriver, DatabaseLimits{MaxOpenConnections: 1, MaxPreparedStatements: 1})
	task121Exec(t, plain, "CREATE TABLE pool (id INTEGER)")
	task121Exec(t, plain, "INSERT INTO pool (id) VALUES (?)", intValue(big.NewInt(1), 64))
	rows, err := plain.Query(context.Background(), "SELECT id FROM pool", nil)
	if err != nil {
		t.Fatalf("pool query: %v", err)
	}
	if got := plain.Stats(); got.InUse != 1 || got.Available != 0 {
		t.Fatalf("pool while rows open = %+v", got)
	}
	ctx, cancel = context.WithTimeout(context.Background(), 5*time.Millisecond)
	_, err = plain.Exec(ctx, "INSERT INTO pool (id) VALUES (?)", []Value{intValue(big.NewInt(2), 64)})
	cancel()
	if err == nil {
		t.Fatal("pool-exhausted operation succeeded")
	}
	if err := rows.Close(); err != nil {
		t.Fatalf("rows close: %v", err)
	}
	if got := plain.Stats(); got.InUse != 0 {
		t.Fatalf("pool after rows close = %+v", got)
	}
	statement, err := plain.Prepare(context.Background(), "INSERT INTO pool (id) VALUES (?)")
	if err != nil {
		t.Fatalf("prepare limit first: %v", err)
	}
	if _, err := plain.Prepare(context.Background(), "INSERT INTO pool (id) VALUES (?)"); err == nil {
		t.Fatal("second prepared statement exceeded no limit")
	}
	_ = statement.Close()
	if err := plain.Close(); err != nil {
		t.Fatalf("connection close: %v", err)
	}
	if err := plain.Close(); err != nil {
		t.Fatalf("idempotent connection close: %v", err)
	}
}

func TestTask121ConcurrencyAndRedactedStructuredErrors(t *testing.T) {
	connection := task121Open(t, NewEmbeddedDriver(), DatabaseLimits{MaxOpenConnections: 4})
	task121Exec(t, connection, "CREATE TABLE concurrent (id INTEGER, name TEXT)")
	const workers = 12
	var group sync.WaitGroup
	errs := make(chan error, workers)
	for i := 0; i < workers; i++ {
		group.Add(1)
		go func(id int) {
			defer group.Done()
			_, err := connection.Exec(context.Background(), "INSERT INTO concurrent (id, name) VALUES (?, ?)", []Value{intValue(big.NewInt(int64(id)), 64), stringValue("worker")})
			errs <- err
		}(i)
	}
	group.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent insert: %v", err)
		}
	}
	rows, err := connection.Query(context.Background(), "SELECT id FROM concurrent ORDER BY id", nil)
	if err != nil {
		t.Fatalf("concurrent query: %v", err)
	}
	count := 0
	for {
		ok, nextErr := rows.Next(context.Background())
		if nextErr != nil {
			t.Fatalf("concurrent next: %v", nextErr)
		}
		if !ok {
			break
		}
		count++
	}
	if count != workers {
		t.Fatalf("concurrent row count = %d", count)
	}

	secretQuery := "INSERT INTO no_such_table (password) VALUES (?)"
	_, err = connection.Exec(context.Background(), secretQuery, []Value{stringValue("super-secret")})
	if err == nil || errors.Is(err, nil) {
		t.Fatal("malformed query unexpectedly succeeded")
	}
	if text := err.Error(); containsTask121(text, "super-secret") || containsTask121(text, "password") {
		t.Fatalf("redacted error leaked query data: %s", text)
	}
	record, ok := err.(*OsirisError)
	if !ok || record.operation != "exec" || record.metadata["query_identity"].kind != kindString || len(record.debugMetadata) == 0 {
		t.Fatalf("structured database error = %#v", err)
	}
}

func containsTask121(text, fragment string) bool {
	for i := 0; i+len(fragment) <= len(text); i++ {
		if text[i:i+len(fragment)] == fragment {
			return true
		}
	}
	return false
}
