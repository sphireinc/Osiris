package osiris

import (
	"context"
	"fmt"
	"math/big"
)

type task121ConnectionHandle struct{ connection DatabaseConnection }

func (h *task121ConnectionHandle) Close() error {
	if h == nil || h.connection == nil {
		return nil
	}
	return h.connection.Close()
}

type task121TransactionHandle struct{ transaction DatabaseTransaction }

func (h *task121TransactionHandle) Close() error {
	if h == nil || h.transaction == nil {
		return nil
	}
	return h.transaction.Close()
}

type task121RowsHandle struct{ rows DatabaseRows }

func (h *task121RowsHandle) Close() error {
	if h == nil || h.rows == nil {
		return nil
	}
	return h.rows.Close()
}

type task121StatementHandle struct{ statement DatabaseStatement }

func (h *task121StatementHandle) Close() error {
	if h == nil || h.statement == nil {
		return nil
	}
	return h.statement.Close()
}

func task121Resource(value Value) (any, error) {
	stream, err := value.AsStream()
	if err != nil {
		return nil, fmt.Errorf("database handle: %w", err)
	}
	if stream.closer == nil {
		return nil, fmt.Errorf("database handle is closed")
	}
	switch handle := stream.closer.(type) {
	case *task121ConnectionHandle:
		return handle.connection, nil
	case *task121TransactionHandle:
		return handle.transaction, nil
	case *task121RowsHandle:
		return handle.rows, nil
	case *task121StatementHandle:
		return handle.statement, nil
	default:
		return nil, fmt.Errorf("value is not a database handle")
	}
}

func task121Connection(value Value) (DatabaseConnection, error) {
	resource, err := task121Resource(value)
	if err != nil {
		return nil, err
	}
	connection, ok := resource.(DatabaseConnection)
	if !ok {
		return nil, fmt.Errorf("expected database connection")
	}
	return connection, nil
}
func task121Executor(value Value) (DatabaseExecutor, error) {
	resource, err := task121Resource(value)
	if err != nil {
		return nil, err
	}
	executor, ok := resource.(DatabaseExecutor)
	if !ok {
		return nil, fmt.Errorf("expected database connection or transaction")
	}
	return executor, nil
}
func task121Transaction(value Value) (DatabaseTransaction, error) {
	resource, err := task121Resource(value)
	if err != nil {
		return nil, err
	}
	transaction, ok := resource.(DatabaseTransaction)
	if !ok {
		return nil, fmt.Errorf("expected database transaction")
	}
	return transaction, nil
}
func task121Rows(value Value) (DatabaseRows, error) {
	resource, err := task121Resource(value)
	if err != nil {
		return nil, err
	}
	rows, ok := resource.(DatabaseRows)
	if !ok {
		return nil, fmt.Errorf("expected database rows")
	}
	return rows, nil
}
func task121Statement(value Value) (DatabaseStatement, error) {
	resource, err := task121Resource(value)
	if err != nil {
		return nil, err
	}
	statement, ok := resource.(DatabaseStatement)
	if !ok {
		return nil, fmt.Errorf("expected database statement")
	}
	return statement, nil
}

func task121Handle(id, mode string, closer interface{ Close() error }) Value {
	return streamValueOf(&streamValue{id: id, mode: mode, closer: closer})
}

func task121Args(value Value) ([]Value, error) {
	if value.kind == kindNull {
		return nil, nil
	}
	args, err := value.AsList()
	if err != nil {
		return nil, fmt.Errorf("database arguments: %w", err)
	}
	return args, nil
}

func task121RowsValue(rows DatabaseRows) Value {
	return task121Handle("database-rows", "database-rows", &task121RowsHandle{rows: rows})
}

func task121DatabaseModule() map[string]Value {
	driver := NewEmbeddedDriver()
	return map[string]Value{
		"api_version": uintValue(uint64(DatabaseDriverAPIVersion), 32),
		"provider":    stringValue(DatabaseEmbeddedProvider),
		"open": contextFunctionValueOf("std/db.open", func(ctx context.Context, args []Value) (Value, error) {
			provider := DatabaseEmbeddedProvider
			if len(args) > 1 {
				return nullValue(), fmt.Errorf("db.open expects zero or one provider")
			}
			if len(args) == 1 {
				var err error
				provider, err = taskStringArg(args[0], "database provider")
				if err != nil {
					return nullValue(), err
				}
			}
			if provider != DatabaseEmbeddedProvider {
				return nullValue(), fmt.Errorf("unsupported database provider %q", provider)
			}
			connection, err := driver.Open(ctx, DatabaseOpenConfig{Capabilities: []string{DatabaseOpenCapability, DatabaseEmbeddedCapability}})
			if err != nil {
				return nullValue(), err
			}
			return task121Handle("database-connection", "database-connection", &task121ConnectionHandle{connection: connection}), nil
		}),
		"exec": contextFunctionValueOf("std/db.exec", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 && len(args) != 3 {
				return nullValue(), fmt.Errorf("db.exec expects handle, query, and optional arguments")
			}
			executor, err := task121Executor(args[0])
			if err != nil {
				return nullValue(), err
			}
			query, err := taskStringArg(args[1], "database query")
			if err != nil {
				return nullValue(), err
			}
			var parameters []Value
			if len(args) == 3 {
				parameters, err = task121Args(args[2])
				if err != nil {
					return nullValue(), err
				}
			}
			result, err := executor.Exec(ctx, query, parameters)
			if err != nil {
				return nullValue(), err
			}
			return mapValueOf([]mapEntry{{key: stringValue("rows_affected"), value: intValue(big.NewInt(result.RowsAffected), 64)}}), nil
		}),
		"query": contextFunctionValueOf("std/db.query", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 && len(args) != 3 {
				return nullValue(), fmt.Errorf("db.query expects handle, query, and optional arguments")
			}
			executor, err := task121Executor(args[0])
			if err != nil {
				return nullValue(), err
			}
			query, err := taskStringArg(args[1], "database query")
			if err != nil {
				return nullValue(), err
			}
			var parameters []Value
			if len(args) == 3 {
				parameters, err = task121Args(args[2])
				if err != nil {
					return nullValue(), err
				}
			}
			rows, err := executor.Query(ctx, query, parameters)
			if err != nil {
				return nullValue(), err
			}
			return task121RowsValue(rows), nil
		}),
		"prepare": contextFunctionValueOf("std/db.prepare", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 2 {
				return nullValue(), fmt.Errorf("db.prepare expects handle and query")
			}
			executor, err := task121Executor(args[0])
			if err != nil {
				return nullValue(), err
			}
			query, err := taskStringArg(args[1], "database query")
			if err != nil {
				return nullValue(), err
			}
			statement, err := executor.Prepare(ctx, query)
			if err != nil {
				return nullValue(), err
			}
			return task121Handle("database-statement", "database-statement", &task121StatementHandle{statement: statement}), nil
		}),
		"statement_exec": contextFunctionValueOf("std/db.statement_exec", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 && len(args) != 2 {
				return nullValue(), fmt.Errorf("db.statement_exec expects statement and optional arguments")
			}
			statement, err := task121Statement(args[0])
			if err != nil {
				return nullValue(), err
			}
			var parameters []Value
			if len(args) == 2 {
				parameters, err = task121Args(args[1])
				if err != nil {
					return nullValue(), err
				}
			}
			result, err := statement.Exec(ctx, parameters)
			if err != nil {
				return nullValue(), err
			}
			return intValue(big.NewInt(result.RowsAffected), 64), nil
		}),
		"statement_query": contextFunctionValueOf("std/db.statement_query", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 && len(args) != 2 {
				return nullValue(), fmt.Errorf("db.statement_query expects statement and optional arguments")
			}
			statement, err := task121Statement(args[0])
			if err != nil {
				return nullValue(), err
			}
			var parameters []Value
			if len(args) == 2 {
				parameters, err = task121Args(args[1])
				if err != nil {
					return nullValue(), err
				}
			}
			rows, err := statement.Query(ctx, parameters)
			if err != nil {
				return nullValue(), err
			}
			return task121RowsValue(rows), nil
		}),
		"begin": contextFunctionValueOf("std/db.begin", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.begin expects connection")
			}
			connection, err := task121Connection(args[0])
			if err != nil {
				return nullValue(), err
			}
			transaction, err := connection.Begin(ctx, DatabaseTxOptions{})
			if err != nil {
				return nullValue(), err
			}
			return task121Handle("database-transaction", "database-transaction", &task121TransactionHandle{transaction: transaction}), nil
		}),
		"commit": contextFunctionValueOf("std/db.commit", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.commit expects transaction")
			}
			transaction, err := task121Transaction(args[0])
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), transaction.Commit(ctx)
		}),
		"rollback": contextFunctionValueOf("std/db.rollback", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.rollback expects transaction")
			}
			transaction, err := task121Transaction(args[0])
			if err != nil {
				return nullValue(), err
			}
			return boolValue(true), transaction.Rollback(ctx)
		}),
		"next": contextFunctionValueOf("std/db.next", func(ctx context.Context, args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.next expects rows")
			}
			rows, err := task121Rows(args[0])
			if err != nil {
				return nullValue(), err
			}
			ok, err := rows.Next(ctx)
			return boolValue(ok), err
		}),
		"values": functionValueOf("std/db.values", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.values expects rows")
			}
			rows, err := task121Rows(args[0])
			if err != nil {
				return nullValue(), err
			}
			return listValue(rows.Values()), nil
		}),
		"columns": functionValueOf("std/db.columns", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.columns expects rows")
			}
			rows, err := task121Rows(args[0])
			if err != nil {
				return nullValue(), err
			}
			columns := rows.Columns()
			out := make([]Value, len(columns))
			for i, column := range columns {
				out[i] = mapValueOf([]mapEntry{{key: stringValue("name"), value: stringValue(column.Name)}, {key: stringValue("type"), value: stringValue(column.DatabaseType)}, {key: stringValue("nullable"), value: boolValue(column.Nullable)}})
			}
			return listValue(out), nil
		}),
		"row_error": functionValueOf("std/db.row_error", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.row_error expects rows")
			}
			rows, err := task121Rows(args[0])
			if err != nil {
				return nullValue(), err
			}
			if rowErr := rows.Err(); rowErr != nil {
				return errorValue(rowErr.Error()), nil
			}
			return nullValue(), nil
		}),
		"stats": functionValueOf("std/db.stats", func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("db.stats expects connection")
			}
			connection, err := task121Connection(args[0])
			if err != nil {
				return nullValue(), err
			}
			stats := connection.Stats()
			return mapValueOf([]mapEntry{{key: stringValue("max_open"), value: intValue(big.NewInt(int64(stats.MaxOpen)), 64)}, {key: stringValue("in_use"), value: intValue(big.NewInt(int64(stats.InUse)), 64)}, {key: stringValue("available"), value: intValue(big.NewInt(int64(stats.Available)), 64)}, {key: stringValue("prepared"), value: intValue(big.NewInt(int64(stats.Prepared)), 64)}}), nil
		}),
		"close": functionValueOf("std/db.close", closeBuiltinValue),
	}
}
