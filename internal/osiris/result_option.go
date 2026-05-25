package osiris

import (
	"fmt"
	"strings"
)

// resultValue stores the standard Result type as an immutable pair of a
// success payload and an optional structured error record.
type resultValue struct {
	ok  bool
	val Value
	err *OsirisError
}

// optionValue stores the standard Option type as an immutable present/absent
// flag plus the wrapped payload when it exists.
type optionValue struct {
	present bool
	val     Value
}

// resultOkValue constructs a successful Result value.
func resultOkValue(v Value) Value {
	return Value{kind: kindResult, res: &resultValue{ok: true, val: v}}
}

// resultErrValue constructs a failed Result value from a structured error.
func resultErrValue(err *OsirisError) Value {
	if err == nil {
		err = newOsirisError(ErrorKindUnknown, "")
	}
	return Value{kind: kindResult, res: &resultValue{ok: false, err: err.clone()}}
}

// optionSomeValue constructs a present Option value.
func optionSomeValue(v Value) Value {
	return Value{kind: kindOption, opt: &optionValue{present: true, val: v}}
}

// optionNoneValue constructs an absent Option value.
func optionNoneValue() Value { return Value{kind: kindOption, opt: &optionValue{present: false}} }

// resultRecord returns a deep copy of the structured result record, if any.
func (v Value) resultRecord() *resultValue {
	if v.kind != kindResult || v.res == nil {
		return nil
	}
	out := *v.res
	out.val = cloneValue(out.val)
	if out.err != nil {
		out.err = out.err.clone()
	}
	return &out
}

// optionRecord returns a deep copy of the structured option record, if any.
func (v Value) optionRecord() *optionValue {
	if v.kind != kindOption || v.opt == nil {
		return nil
	}
	out := *v.opt
	out.val = cloneValue(out.val)
	return &out
}

// cloneValue copies the shallow structure of a Value while preserving its
// logical content. It is used to keep option/result records immutable.
func cloneValue(v Value) Value {
	switch v.kind {
	case kindResult:
		if rec := v.resultRecord(); rec != nil {
			return Value{kind: kindResult, res: rec}
		}
	case kindOption:
		if rec := v.optionRecord(); rec != nil {
			return Value{kind: kindOption, opt: rec}
		}
	}
	return v
}

// unwrapResult returns the success payload or an error when the result is a
// failure. The returned error is the structured Osiris error record so callers
// can continue normal propagation behavior.
func unwrapResult(v Value) (Value, error) {
	if v.kind != kindResult || v.res == nil {
		return nullValue(), fmt.Errorf("expected result, got %s", v.Kind())
	}
	if v.res.ok {
		return v.res.val, nil
	}
	if v.res.err == nil {
		return nullValue(), fmt.Errorf("result is missing error payload")
	}
	return nullValue(), v.res.err.clone()
}

// unwrapOption returns the present payload or a structured not-found error
// when the option is empty.
func unwrapOption(v Value) (Value, error) {
	if v.kind != kindOption || v.opt == nil {
		return nullValue(), fmt.Errorf("expected option, got %s", v.Kind())
	}
	if v.opt.present {
		return v.opt.val, nil
	}
	return nullValue(), newOsirisError(ErrorKindNotFound, "option is empty")
}

// okBuiltin constructs a successful Result value.
func okBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("ok expects 1 argument")
	}
	return resultOkValue(args[0]), nil
}

// errBuiltin constructs a failed Result value from an error value or message.
func errBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("err expects 1 argument")
	}
	rec, err := args[0].AsError()
	if err == nil && rec != nil {
		return resultErrValue(rec), nil
	}
	msg, msgErr := args[0].AsString()
	if msgErr != nil {
		return nullValue(), msgErr
	}
	return resultErrValue(newOsirisError(ErrorKindUnknown, msg)), nil
}

// someBuiltin constructs a present Option value.
func someBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("some expects 1 argument")
	}
	return optionSomeValue(args[0]), nil
}

// noneBuiltin constructs an empty Option value.
func noneBuiltin(args []Value) (Value, error) {
	if len(args) != 0 {
		return nullValue(), fmt.Errorf("none expects no arguments")
	}
	return optionNoneValue(), nil
}

// isOkBuiltin reports whether the supplied value is a successful Result.
func isOkBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("isok expects 1 argument")
	}
	return boolValue(args[0].kind == kindResult && args[0].res != nil && args[0].res.ok), nil
}

// isErrBuiltin reports whether the supplied value is a failed Result or an
// error value.
func isErrBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("iserr expects 1 argument")
	}
	if args[0].kind == kindError {
		return boolValue(true), nil
	}
	return boolValue(args[0].kind == kindResult && args[0].res != nil && !args[0].res.ok), nil
}

// isSomeBuiltin reports whether the supplied value is a present Option.
func isSomeBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("issome expects 1 argument")
	}
	return boolValue(args[0].kind == kindOption && args[0].opt != nil && args[0].opt.present), nil
}

// isNoneBuiltin reports whether the supplied value is an empty Option.
func isNoneBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("isnone expects 1 argument")
	}
	return boolValue(args[0].kind == kindOption && (args[0].opt == nil || !args[0].opt.present)), nil
}

// unwrapResultBuiltin extracts the payload from a Result or Option value.
func unwrapResultBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("unwrap expects 1 argument")
	}
	switch args[0].kind {
	case kindResult:
		return unwrapResult(args[0])
	case kindOption:
		return unwrapOption(args[0])
	case kindError:
		if rec := args[0].errorRecord(); rec != nil {
			if rec.cause != nil {
				return errorValueFromRecord(rec.cause), nil
			}
			return errorValueFromRecord(rec), nil
		}
		return nullValue(), fmt.Errorf("expected error, got %s", args[0].Kind())
	default:
		return nullValue(), fmt.Errorf("unwrap expects a result, option, or error")
	}
}

// expectBuiltin extracts the payload from a Result or Option, or returns a
// structured error with the supplied message when the value is empty or a
// failure.
func expectBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("expect expects 2 arguments")
	}
	msg, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	switch args[0].kind {
	case kindResult:
		if args[0].res != nil && args[0].res.ok {
			return args[0].res.val, nil
		}
		return nullValue(), newOsirisError(ErrorKindInvalidArgument, msg)
	case kindOption:
		if args[0].opt != nil && args[0].opt.present {
			return args[0].opt.val, nil
		}
		return nullValue(), newOsirisError(ErrorKindNotFound, msg)
	default:
		return nullValue(), fmt.Errorf("expect expects a result or option")
	}
}

// orElseBuiltin returns the value inside a Result or Option, or the provided
// default value when the container is empty or failed.
func orElseBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("or_else expects 2 arguments")
	}
	switch args[0].kind {
	case kindResult:
		if args[0].res != nil && args[0].res.ok {
			return args[0].res.val, nil
		}
		return args[1], nil
	case kindOption:
		if args[0].opt != nil && args[0].opt.present {
			return args[0].opt.val, nil
		}
		return args[1], nil
	default:
		return nullValue(), fmt.Errorf("or_else expects a result or option")
	}
}

// panicBuiltin constructs an unrecoverable fatal structured error.
func panicBuiltin(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("panic expects 1 argument")
	}
	msg, err := args[0].AsString()
	if err != nil {
		return nullValue(), err
	}
	rec := newOsirisError(ErrorKindInternal, msg)
	rec.severity = ErrorSeverityFatal
	return errorValueFromRecord(rec), rec
}

// trapBuiltin is an alias for panicBuiltin so source code can use whichever
// word reads better in context.
func trapBuiltin(args []Value) (Value, error) { return panicBuiltin(args) }

// assertBuiltin raises a fatal structured error when the condition is false.
func assertBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("assert expects 2 arguments")
	}
	if args[0].Truthy() {
		return boolValue(true), nil
	}
	msg, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	rec := newOsirisError(ErrorKindInternal, msg)
	rec.severity = ErrorSeverityFatal
	return errorValueFromRecord(rec), rec
}

// matchesErrorBuiltin checks whether an error has the requested kind.
func matchesErrorBuiltin(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("matches expects 2 arguments")
	}
	rec, err := args[0].AsError()
	if err != nil {
		return nullValue(), err
	}
	kind, err := args[1].AsString()
	if err != nil {
		return nullValue(), err
	}
	if rec == nil {
		return boolValue(false), nil
	}
	return boolValue(strings.EqualFold(string(rec.kind), kind) || strings.EqualFold(rec.code, kind)), nil
}

// isErrorBuiltin is an alias for matchesErrorBuiltin.
func isErrorBuiltin(args []Value) (Value, error) { return matchesErrorBuiltin(args) }
