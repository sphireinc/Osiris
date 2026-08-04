package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"math/big"
	"strconv"
	"strings"
)

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindDecimal
	kindFixed
	kindInterval
	kindModInt
	kindComplex
	kindError
	kindResult
	kindOption
	kindBigInt
	kindBigFloat
	kindInt
	kindQuantity
	kindUint
	kindString
	kindList
	kindMap
	kindSet
	kindTensor
	kindStream
	kindTask
	kindFunction
	kindModule
	kindEnum
	kindStruct
	kindTuple
	kindDual
	kindFuture
	kindTCPListener
	kindUDPSocket
	kindFFILibrary
	kindFFIHandle
	kindDuration
	kindDateTime
	kindTimeLocation
	kindOSProcess
)

type callable = Builtin
type contextCallable func(context.Context, []Value) (Value, error)

type moduleLookup func(ctx context.Context, name string) (Value, error)

type functionValue struct {
	name           string
	signature      string
	typeParams     []typeParam
	paramTypes     []string
	returnType     string
	builtin        callable
	contextBuiltin contextCallable
	params         []string
	body           Expr
	closure        *scope
	group          *scope
	predeclared    bool
	session        *Session
	vmBody         *vmChunk
	vmClosure      *vmFrame
	receiver       string
	receiverType   string
	boundReceiver  *Value
}

// FunctionType returns the canonical static signature retained by a user
// function value. Unannotated positions are rendered as dynamic; builtins and
// zero-parameter functions without a return annotation return "function".
func (v Value) FunctionType() string {
	if v.kind != kindFunction || v.fn == nil || v.fn.signature == "" {
		return "function"
	}
	return v.fn.signature
}

// TypeParameters returns the declared generic parameter names for a function
// value. It is intentionally metadata-only; inference and substitution remain
// checker responsibilities.
func (v Value) TypeParameters() []string {
	if v.kind != kindFunction || v.fn == nil || len(v.fn.typeParams) == 0 {
		return nil
	}
	out := make([]string, len(v.fn.typeParams))
	for i, param := range v.fn.typeParams {
		out[i] = param.Name
	}
	return out
}

type moduleValue struct {
	name      string
	version   string
	lookup    moduleLookup
	members   map[string]Value
	enumCases map[string]enumCase
}

type structFieldValue struct {
	name      string
	typeName  string
	tag       string
	tagValues map[string]string
}

type structTypeValue struct {
	name    string
	fields  []structFieldValue
	byName  map[string]int
	newtype bool
	target  string
}

type structInstanceValue struct {
	typ    *structTypeValue
	fields []Value
}

// enumValue is the nominal runtime identity of one declared enum case.  The
// enum declaration itself is exposed as a module-like namespace so cases use
// ordinary member access (for example, Status.Ready).
type enumValue struct {
	typeName string
	caseName string
	payload  []Value
}

type dualValue struct {
	primal  float64
	tangent float64
}

type mapEntry struct {
	key   Value
	value Value
}

type mapValue struct {
	entries []mapEntry
}

type setValue struct {
	items []Value
}

type Value struct {
	kind        valueKind
	num         float64
	fbits       uint16
	bfloat      bool
	dec         *big.Rat
	fx          *fixedValue
	iv          *intervalValue
	mi          *modIntValue
	cx          complex128
	bi          *big.Int
	bf          *big.Float
	i           *big.Int
	ibits       uint16
	qty         *quantityValue
	u           uint64
	bits        uint8
	ub          *big.Int
	ubits       uint16
	packed      bool
	native      bool
	boo         bool
	str         string
	list        []Value
	mapValue    *mapValue
	setValue    *setValue
	shape       []int
	data        []Value
	stream      *streamValue
	task        *taskValue
	err         *OsirisError
	errMsg      string
	res         *resultValue
	opt         *optionValue
	fn          *functionValue
	mod         *moduleValue
	enum        *enumValue
	structType  *structTypeValue
	structValue *structInstanceValue
	tuple       []Value
	dual        *dualValue
	future      *futureValue
	listener    *tcpListener
	udp         *udpSocket
	ffi         *FFILibrary
	ffiHandle   *FFIHandle
	duration    int64
	datetime    *dateTimeValue
	location    *timeLocationValue
	process     *osProcess
}

// nullValue handles the null value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func nullValue() Value { return Value{kind: kindNull} }

// boolValue handles the bool value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func boolValue(v bool) Value { return Value{kind: kindBool, boo: v} }

// numberValue handles the number value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func numberValue(v float64) Value { return floatValue(v, 0) }

// floatValue handles the float value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func floatValue(v float64, bits uint16) Value {
	if bits == 32 {
		v = float64(float32(v))
	}
	return Value{kind: kindNumber, num: v, fbits: bits}
}

// bfloatValue handles the bfloat value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func bfloatValue(v float64, bits uint16) Value {
	if bits == 32 {
		v = float64(float32(v))
	}
	return Value{kind: kindNumber, num: v, fbits: bits, bfloat: true}
}

// decimalValue handles the decimal value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func decimalValue(v *big.Rat) Value {
	if v == nil {
		v = new(big.Rat)
	}
	return Value{kind: kindDecimal, dec: new(big.Rat).Set(v)}
}

// fixedValueOfValue handles the fixed value of value step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func fixedValueOfValue(v *fixedValue) Value {
	if v == nil {
		v = &fixedValue{width: 64}
	}
	raw := new(big.Int)
	if v.raw != nil {
		raw.Set(v.raw)
	}
	return Value{kind: kindFixed, fx: &fixedValue{width: v.width, frac: v.frac, raw: raw}}
}

// intervalValueOf handles the interval value of step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func intervalValueOf(v *intervalValue) Value {
	if v == nil {
		v = newIntervalValue(new(big.Rat), new(big.Rat))
	}
	return Value{kind: kindInterval, iv: newIntervalValue(v.lo, v.hi)}
}

// modIntValueOfValue handles the mod int value of value step in the
// surrounding package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func modIntValueOfValue(v *modIntValue) Value {
	if v == nil {
		v = &modIntValue{mod: big.NewInt(1), val: new(big.Int)}
	}
	return Value{kind: kindModInt, mi: &modIntValue{mod: new(big.Int).Set(v.mod), val: new(big.Int).Set(v.val)}}
}

// complexValue handles the complex value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func complexValue(v complex128) Value { return complexValueStyled(v, 0) }

// complexValueStyled creates a complex value with an optional component
// precision.  A zero width is the historical/default complex family; the
// explicit f32 and f64 families retain their component width just as scalar
// floats do.
func complexValueStyled(v complex128, bits uint16) Value {
	if bits == 32 {
		v = complex(float64(float32(real(v))), float64(float32(imag(v))))
	}
	return Value{kind: kindComplex, cx: v, fbits: bits}
}

func dualValueOf(primal, tangent float64) Value {
	return Value{kind: kindDual, dual: &dualValue{primal: primal, tangent: tangent}}
}

func tcpListenerValueOf(listener *tcpListener) Value {
	return Value{kind: kindTCPListener, listener: listener}
}

func udpSocketValueOf(socket *udpSocket) Value {
	return Value{kind: kindUDPSocket, udp: socket}
}

func osProcessValueOf(process *osProcess) Value {
	return Value{kind: kindOSProcess, process: process}
}

func ffiLibraryValueOf(library *FFILibrary) Value {
	return Value{kind: kindFFILibrary, ffi: library}
}

func ffiHandleValueOf(handle *FFIHandle) Value {
	return Value{kind: kindFFIHandle, ffiHandle: handle}
}

func durationValue(nanoseconds int64) Value {
	return Value{kind: kindDuration, duration: nanoseconds}
}

func dateTimeValueOf(value *dateTimeValue) Value {
	return Value{kind: kindDateTime, datetime: value.clone()}
}

func timeLocationValueOf(value *timeLocationValue) Value {
	return Value{kind: kindTimeLocation, location: value.clone()}
}

// errorValue handles the error value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func errorValue(msg string) Value {
	rec := newOsirisError(ErrorKindUnknown, msg)
	return Value{kind: kindError, err: rec, errMsg: msg}
}

// bigintValue handles the bigint value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func bigintValue(v *big.Int) Value { return bigIntValue(v) }

// bigfloatValue handles the bigfloat value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func bigfloatValue(v *big.Float) Value { return bigFloatValue(v) }

// intValue handles the int value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func intValue(v *big.Int, bits uint16) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindInt, i: wrapSignedWidth(v, bits), ibits: bits}
}

// measureValue handles the measure value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func measureValue(v *big.Rat, dims map[string]int) Value {
	return quantityValueOf(v, dims)
}

// uintValue handles the uint value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func uintValue(v uint64, bits uint8) Value {
	if bits == 0 || bits > 64 {
		bits = 64
	}
	return Value{kind: kindUint, u: v & maskForWidth(bits), bits: bits}
}

// packedUintValue handles the packed uint value step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func packedUintValue(v uint64, bits uint8, native bool) Value {
	if bits == 0 || bits > 64 {
		bits = 64
	}
	return Value{kind: kindUint, u: v & maskForWidth(bits), bits: bits, packed: true, native: native}
}

// wideUintValue handles the wide uint value step in the surrounding package
// and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func wideUintValue(v *big.Int, bits uint16) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindUint, ub: wrapUnsignedWidth(v, bits), ubits: bits}
}

// packedWideUintValue handles the packed wide uint value step in the
// surrounding package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func packedWideUintValue(v *big.Int, bits uint16, native bool) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindUint, ub: wrapUnsignedWidth(v, bits), ubits: bits, packed: true, native: native}
}

// stringValue handles the string value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func stringValue(v string) Value { return Value{kind: kindString, str: v} }

// listValue handles the list value step in the surrounding package and returns
// Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func listValue(v []Value) Value { return Value{kind: kindList, list: v} }

func mapValueOf(entries []mapEntry) Value {
	return Value{kind: kindMap, mapValue: &mapValue{entries: append([]mapEntry(nil), entries...)}}
}

func setValueOf(items []Value) Value {
	return Value{kind: kindSet, setValue: &setValue{items: append([]Value(nil), items...)}}
}

func tupleValue(v []Value) Value {
	return Value{kind: kindTuple, tuple: append([]Value(nil), v...)}
}

func structTypeValueOf(typ *structTypeValue) Value {
	return Value{kind: kindStruct, structType: typ}
}

func structInstanceValueOf(typ *structTypeValue, fields []Value) Value {
	return Value{kind: kindStruct, structType: typ, structValue: &structInstanceValue{typ: typ, fields: append([]Value(nil), fields...)}}
}

// tensorValue handles the tensor value step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func tensorValue(shape []int, data []Value) Value {
	return newTensorValue(shape, data)
}

// streamValueOf handles the stream value of step in the surrounding package
// and returns Value.
//
// It returns Value so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func streamValueOf(v *streamValue) Value { return Value{kind: kindStream, stream: v} }

// taskValueOf handles the task value of step in the surrounding package and
// returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func taskValueOf(v *taskValue) Value { return Value{kind: kindTask, task: v} }

// functionValueOf handles the function value of step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func functionValueOf(name string, fn callable) Value {
	return Value{kind: kindFunction, fn: &functionValue{name: name, builtin: fn}}
}

// contextFunctionValueOf constructs a builtin whose blocking work must observe
// the evaluator/session context. Interpreter and VM callers both pass their
// current context at the invocation boundary.
func contextFunctionValueOf(name string, fn contextCallable) Value {
	return Value{kind: kindFunction, fn: &functionValue{name: name, contextBuiltin: fn}}
}

// userFunctionValue handles the user function value step in the surrounding
// package and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func userFunctionValue(name string, params []string, body Expr, closure *scope, sess *Session) Value {
	return typedUserFunctionValue(name, params, nil, "", body, closure, sess)
}

func typedUserFunctionValue(name string, params, paramTypes []string, returnType string, body Expr, closure *scope, sess *Session) Value {
	return typedGenericUserFunctionValue(name, nil, params, paramTypes, returnType, body, closure, sess)
}

func typedGenericUserFunctionValue(name string, typeParams []typeParam, params, paramTypes []string, returnType string, body Expr, closure *scope, sess *Session) Value {
	signature := ""
	if len(paramTypes) > 0 || returnType != "" {
		parts := make([]staticType, len(params))
		for i := range parts {
			parts[i] = dynamicStaticType
			if i < len(paramTypes) && paramTypes[i] != "" {
				if parsed, err := parseStaticType(paramTypes[i]); err == nil {
					parts[i] = parsed
				}
			}
		}
		result := dynamicStaticType
		if returnType != "" {
			if parsed, err := parseStaticType(returnType); err == nil {
				result = parsed
			}
		}
		signature = staticType{name: "func", params: parts, result: &result}.String()
	}
	return Value{kind: kindFunction, fn: &functionValue{
		name:       name,
		signature:  signature,
		typeParams: append([]typeParam(nil), typeParams...),
		paramTypes: append([]string(nil), paramTypes...),
		returnType: returnType,
		params:     append([]string(nil), params...),
		body:       body,
		closure:    closure,
		session:    sess,
	}}
}

func bindMethodReceiver(method, receiver Value) Value {
	if method.kind != kindFunction || method.fn == nil || method.fn.receiver == "" {
		return method
	}
	copyFn := *method.fn
	copyFn.params = append([]string{copyFn.receiver}, copyFn.params...)
	copyFn.paramTypes = append([]string{""}, copyFn.paramTypes...)
	copyFn.boundReceiver = &receiver
	return Value{kind: kindFunction, fn: &copyFn}
}

func checkFunctionArguments(fn *functionValue, args []Value) ([]Value, error) {
	if fn == nil {
		return nil, fmt.Errorf("missing function metadata")
	}
	if len(args) != len(fn.params) {
		return nil, fmt.Errorf("function %s expects %d arguments", fn.name, len(fn.params))
	}
	checked := append([]Value(nil), args...)
	for i, typeName := range fn.paramTypes {
		if typeName == "" || i >= len(checked) {
			continue
		}
		if fn.hasGenericType(typeName) {
			continue
		}
		value, err := checkBindingType(typeName, checked[i], false)
		if err != nil {
			actual := staticTypeFromValue(checked[i]).String()
			return nil, fmt.Errorf("function %s argument %d requires %s, got %s", fn.name, i+1, typeName, actual)
		}
		checked[i] = value
	}
	return checked, nil
}

func checkFunctionResult(fn *functionValue, value Value) (Value, error) {
	if fn == nil || fn.returnType == "" {
		return value, nil
	}
	if fn.hasGenericType(fn.returnType) {
		return value, nil
	}
	checked, err := checkBindingType(fn.returnType, value, false)
	if err != nil {
		return nullValue(), fmt.Errorf("function %s returns %s, requires %s", fn.name, staticTypeFromValue(value).String(), fn.returnType)
	}
	return checked, nil
}

func (fn *functionValue) hasGenericType(text string) bool {
	if fn == nil || len(fn.typeParams) == 0 {
		return false
	}
	typ, err := parseStaticType(text)
	if err != nil {
		return false
	}
	var contains func(staticType) bool
	contains = func(t staticType) bool {
		for _, param := range fn.typeParams {
			if t.name == param.Name && len(t.params) == 0 && t.result == nil {
				return true
			}
		}
		for _, param := range t.params {
			if contains(param) {
				return true
			}
		}
		return t.result != nil && contains(*t.result)
	}
	return contains(typ)
}

// moduleValueOf handles the module value of step in the surrounding package
// and returns Value.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func moduleValueOf(name, version string, lookup moduleLookup, members map[string]Value) Value {
	return Value{kind: kindModule, mod: &moduleValue{name: name, version: version, lookup: lookup, members: members}}
}

func enumCaseValue(typeName, caseName string, payload ...Value) Value {
	return Value{kind: kindEnum, enum: &enumValue{typeName: typeName, caseName: caseName, payload: append([]Value(nil), payload...)}}
}

func enumNamespaceValue(name string, cases []enumCase) Value {
	members := make(map[string]Value, len(cases))
	caseDefs := make(map[string]enumCase, len(cases))
	for _, item := range cases {
		item := item
		caseDefs[item.Name] = item
		if item.Arity() == 0 {
			members[item.Name] = enumCaseValue(name, item.Name)
			continue
		}
		members[item.Name] = functionValueOf(name+"."+item.Name, func(args []Value) (Value, error) {
			if len(args) != item.Arity() {
				return nullValue(), fmt.Errorf("enum case %s.%s expects %d arguments", name, item.Name, item.Arity())
			}
			return enumCaseValue(name, item.Name, args...), nil
		})
	}
	value := moduleValueOf(name, "", func(_ context.Context, member string) (Value, error) {
		value, ok := members[member]
		if !ok {
			return nullValue(), fmt.Errorf("enum %s has no case %q", name, member)
		}
		return value, nil
	}, members)
	value.mod.enumCases = caseDefs
	return value
}

// Kind operates on Value values and keeps the value-specific behavior attached
// to the receiver so callers can use the method as the natural entry point for
// that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (v Value) Kind() string {
	if v.kind == kindNumber && v.bfloat {
		if v.fbits != 0 {
			return "bf" + strconv.FormatUint(uint64(v.fbits), 10)
		}
		return "bf64"
	}
	if v.kind == kindNumber && v.fbits != 0 {
		return "f" + strconv.FormatUint(uint64(v.fbits), 10)
	}
	if v.kind == kindComplex && v.fbits != 0 {
		return "complex<f" + strconv.FormatUint(uint64(v.fbits), 10) + ">"
	}
	if v.kind == kindDual {
		return "dual"
	}
	if v.kind == kindFuture {
		return futureTypeString(v.future)
	}
	if v.kind == kindTCPListener {
		return "tcp_listener"
	}
	if v.kind == kindUDPSocket {
		return "udp_socket"
	}
	if v.kind == kindFFILibrary {
		return "ffi_library"
	}
	if v.kind == kindFFIHandle {
		if v.ffiHandle == nil {
			return "ffi_handle"
		}
		return "handle:" + strings.TrimPrefix(v.ffiHandle.typeID, "handle:")
	}
	if v.kind == kindOSProcess {
		return "os_process"
	}
	if v.kind == kindFixed && v.fx != nil {
		return fmt.Sprintf("fixed[%d,%d]", v.fx.width, v.fx.frac)
	}
	if v.kind == kindUint && v.packed {
		if v.native {
			return "ufnum"
		}
		if v.ub != nil {
			return "uf" + strconv.FormatUint(uint64(v.ubitsToUint16()), 10)
		}
		return "uf" + strconv.FormatUint(uint64(v.bits), 10)
	}
	if v.kind == kindStruct && v.structType != nil {
		return v.structType.name
	}
	return v.kind.String()
}

// String operates on valueKind values and keeps the valuekind-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (k valueKind) String() string {
	switch k {
	case kindNull:
		return "null"
	case kindBool:
		return "bool"
	case kindNumber:
		return "number"
	case kindDecimal:
		return "decimal"
	case kindFixed:
		return "fixed"
	case kindInterval:
		return "interval"
	case kindModInt:
		return "modint"
	case kindComplex:
		return "complex"
	case kindError:
		return "error"
	case kindResult:
		return "result"
	case kindOption:
		return "option"
	case kindBigInt:
		return "bigint"
	case kindBigFloat:
		return "bigfloat"
	case kindInt:
		return "int"
	case kindQuantity:
		return "quantity"
	case kindUint:
		return "uint"
	case kindString:
		return "string"
	case kindList:
		return "list"
	case kindMap:
		return "map"
	case kindSet:
		return "set"
	case kindTensor:
		return "tensor"
	case kindStream:
		return "stream"
	case kindTask:
		return "task"
	case kindFunction:
		return "function"
	case kindModule:
		return "module"
	case kindEnum:
		return "enum"
	case kindStruct:
		return "struct"
	case kindTuple:
		return "tuple"
	case kindDual:
		return "dual"
	case kindFuture:
		return "future"
	case kindTCPListener:
		return "tcp_listener"
	case kindUDPSocket:
		return "udp_socket"
	case kindFFILibrary:
		return "ffi_library"
	case kindFFIHandle:
		return "ffi_handle"
	case kindDuration:
		return "duration"
	case kindDateTime:
		return "datetime"
	case kindTimeLocation:
		return "location"
	case kindOSProcess:
		return "os_process"
	default:
		return "<?>"
	}
}

// Truthy operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (v Value) Truthy() bool {
	switch v.kind {
	case kindNull:
		return false
	case kindBool:
		return v.boo
	case kindNumber:
		return v.num != 0
	case kindDecimal:
		return v.dec != nil && v.dec.Sign() != 0
	case kindFixed:
		return v.fx != nil && v.fx.raw.Sign() != 0
	case kindInterval:
		return v.iv != nil && (v.iv.lo.Sign() != 0 || v.iv.hi.Sign() != 0)
	case kindModInt:
		return v.mi != nil && v.mi.val.Sign() != 0
	case kindComplex:
		return real(v.cx) != 0 || imag(v.cx) != 0
	case kindDual:
		return v.dual != nil && (v.dual.primal != 0 || v.dual.tangent != 0)
	case kindTCPListener:
		return v.listener != nil && v.listener.listener != nil
	case kindUDPSocket:
		return v.udp != nil && v.udp.isOpen()
	case kindFFILibrary:
		return v.ffi != nil
	case kindFFIHandle:
		return v.ffiHandle != nil && !v.ffiHandle.IsClosed()
	case kindOSProcess:
		return v.process != nil && !v.process.isClosed()
	case kindDuration:
		return v.duration != 0
	case kindDateTime, kindTimeLocation:
		return true
	case kindError:
		return false
	case kindResult:
		return v.res != nil && v.res.ok && v.res.val.Truthy()
	case kindOption:
		return v.opt != nil && v.opt.present
	case kindBigInt:
		return v.bi != nil && v.bi.Sign() != 0
	case kindBigFloat:
		return v.bf != nil && v.bf.Sign() != 0
	case kindInt:
		return v.i != nil && v.i.Sign() != 0
	case kindQuantity:
		return v.qty != nil && v.qty.mag.Sign() != 0
	case kindUint:
		if v.ub != nil {
			return v.ub.Sign() != 0
		}
		return v.u != 0
	case kindString:
		return v.str != ""
	case kindList:
		return len(v.list) != 0
	case kindMap:
		return v.mapValue != nil && len(v.mapValue.entries) != 0
	case kindSet:
		return v.setValue != nil && len(v.setValue.items) != 0
	case kindTensor:
		return len(v.data) != 0
	case kindStream:
		return v.stream != nil
	case kindTask:
		return v.task != nil
	case kindFunction, kindModule:
		return true
	case kindStruct:
		return true
	case kindTuple:
		return len(v.tuple) != 0
	case kindFuture:
		return v.future != nil && len(v.future.data) != 0
	default:
		return false
	}
}

// AsNumber operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns float64 and an error. Callers should only trust the value when
// the error is nil.
func (v Value) AsNumber() (float64, error) {
	switch v.kind {
	case kindNumber:
		return v.num, nil
	case kindDecimal:
		if v.dec == nil {
			return 0, nil
		}
		f, _ := v.dec.Float64()
		return f, nil
	case kindFixed:
		if v.fx == nil {
			return 0, nil
		}
		f, _ := fixedToRat(v.fx).Float64()
		return f, nil
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 {
			return 0, fmt.Errorf("expected number, got %s", v.kind)
		}
		f, _ := v.iv.lo.Float64()
		return f, nil
	case kindModInt:
		if v.mi == nil {
			return 0, nil
		}
		f, _ := new(big.Float).SetInt(v.mi.val).Float64()
		return f, nil
	case kindComplex:
		if imag(v.cx) != 0 {
			return 0, fmt.Errorf("expected number, got complex")
		}
		return real(v.cx), nil
	case kindDual:
		if v.dual == nil {
			return 0, nil
		}
		if v.dual.tangent != 0 {
			return 0, fmt.Errorf("expected number, got dual with non-zero tangent")
		}
		return v.dual.primal, nil
	case kindBigInt:
		if v.bi == nil {
			return 0, nil
		}
		f, _ := new(big.Float).SetInt(v.bi).Float64()
		return f, nil
	case kindBigFloat:
		if v.bf == nil {
			return 0, nil
		}
		f, _ := v.bf.Float64()
		return f, nil
	case kindInt:
		if v.i == nil {
			return 0, nil
		}
		f, _ := new(big.Float).SetInt(v.i).Float64()
		return f, nil
	case kindQuantity:
		if v.qty == nil {
			return 0, nil
		}
		if len(v.qty.dims) != 0 {
			return 0, fmt.Errorf("expected number, got %s", formatQuantity(v.qty))
		}
		f, _ := v.qty.mag.Float64()
		return f, nil
	case kindUint:
		if v.ub != nil {
			if v.ubitsToUint16() > 64 {
				f, _ := new(big.Float).SetInt(v.ub).Float64()
				return f, nil
			}
			f, _ := new(big.Float).SetInt(v.ub).Float64()
			return f, nil
		}
		return float64(v.u), nil
	case kindBool:
		if v.boo {
			return 1, nil
		}
		return 0, nil
	default:
		return 0, fmt.Errorf("expected number, got %s", v.kind)
	}
}

// AsString operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (v Value) AsString() (string, error) {
	switch v.kind {
	case kindString:
		return v.str, nil
	case kindError:
		if rec := v.errorRecord(); rec != nil {
			return rec.displayMessage(), nil
		}
		return "", errors.New(v.errMsg)
	case kindResult, kindOption:
		return v.String(), nil
	case kindNull:
		return "", nil
	case kindBigInt:
		if v.bi == nil {
			return "", nil
		}
		return "", fmt.Errorf("expected string, got %s", v.kind)
	case kindBigFloat:
		return "", fmt.Errorf("expected string, got %s", v.kind)
	case kindInt:
		return "", fmt.Errorf("expected string, got %s", v.kind)
	case kindQuantity:
		return "", fmt.Errorf("expected string, got %s", v.kind)
	default:
		return "", fmt.Errorf("expected string, got %s", v.kind)
	}
}

// AsUint operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns uint64, uint8, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func (v Value) AsUint() (uint64, uint8, error) {
	switch v.kind {
	case kindUint:
		if v.ub != nil {
			if !v.ub.IsUint64() {
				return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
			}
			return v.ub.Uint64(), 64, nil
		}
		return v.u, v.bits, nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatDecimal(v.dec))
		}
		if !v.dec.Num().IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatDecimal(v.dec))
		}
		return v.dec.Num().Uint64(), 64, nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return 0, 0, err
		}
		if !i.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return i.Uint64(), 64, nil
	case kindComplex:
		if imag(v.cx) != 0 || math.Trunc(real(v.cx)) != real(v.cx) || real(v.cx) < 0 {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return uint64(real(v.cx)), 64, nil
	case kindBigInt:
		if v.bi == nil || v.bi.Sign() < 0 || !v.bi.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.bi.Uint64(), 64, nil
	case kindBigFloat:
		if v.bf == nil {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact || !i.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return i.Uint64(), 64, nil
	case kindInt:
		if v.i == nil || v.i.Sign() < 0 || !v.i.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.i.Uint64(), 64, nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() || !v.qty.mag.Num().IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.qty.mag.Num().Uint64(), 64, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num || v.num < 0 {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatNumber(v.num))
		}
		return uint64(v.num), 64, nil
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() || !v.iv.lo.Num().IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.iv.lo.Num().Uint64(), 64, nil
	case kindModInt:
		if v.mi == nil || !v.mi.val.IsUint64() {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
		}
		return v.mi.val.Uint64(), 64, nil
	case kindBool:
		if v.boo {
			return 1, 64, nil
		}
		return 0, 64, nil
	default:
		return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
	}
}

// AsList operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (v Value) AsList() ([]Value, error) {
	if v.kind != kindList && v.kind != kindTensor {
		return nil, fmt.Errorf("expected list, got %s", v.kind)
	}
	if v.kind == kindTensor {
		return v.data, nil
	}
	return v.list, nil
}

// AsFunction operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns callable and an error. Callers should only trust the value when
// the error is nil.
func (v Value) AsFunction() (callable, error) {
	if v.kind != kindFunction || v.fn == nil || v.fn.builtin == nil {
		return nil, fmt.Errorf("expected function, got %s", v.kind)
	}
	return v.fn.builtin, nil
}

// AsModule operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *moduleValue and an error. Callers should only trust the value
// when the error is nil.
func (v Value) AsModule() (*moduleValue, error) {
	if v.kind != kindModule || v.mod == nil {
		return nil, fmt.Errorf("expected module, got %s", v.kind)
	}
	return v.mod, nil
}

// AsTask operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *taskValue and an error. Callers should only trust the value when
// the error is nil.
func (v Value) AsTask() (*taskValue, error) {
	if v.kind != kindTask || v.task == nil {
		return nil, fmt.Errorf("expected task, got %s", v.kind)
	}
	return v.task, nil
}

// AsError operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *OsirisError and an error. Callers should only trust the value
// when the error is nil.
func (v Value) AsError() (*OsirisError, error) {
	if v.kind != kindError {
		return nil, fmt.Errorf("expected error, got %s", v.Kind())
	}
	rec := v.errorRecord()
	if rec == nil {
		return nil, fmt.Errorf("expected error, got %s", v.Kind())
	}
	return rec, nil
}

// AsStream operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *streamValue and an error. Callers should only trust the value
// when the error is nil.
func (v Value) AsStream() (*streamValue, error) {
	if v.kind != kindStream || v.stream == nil {
		return nil, fmt.Errorf("expected stream, got %s", v.kind)
	}
	return v.stream, nil
}

func (v Value) AsTCPListener() (*tcpListener, error) {
	if v.kind != kindTCPListener || v.listener == nil {
		return nil, fmt.Errorf("expected tcp listener, got %s", v.kind)
	}
	return v.listener, nil
}

// Int operates on Value values and keeps the value-specific behavior attached
// to the receiver so callers can use the method as the natural entry point for
// that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (v Value) Int() (int, error) {
	switch v.kind {
	case kindUint:
		if v.ub != nil {
			if !v.ub.IsInt64() {
				return 0, fmt.Errorf("integer overflow for host int: %s", formatUintValue(v))
			}
			n := v.ub.Int64()
			if n > int64(^uint(0)>>1) {
				return 0, fmt.Errorf("integer overflow for host int: %s", formatUintValue(v))
			}
			return int(n), nil
		}
		if v.u > uint64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", formatUint(v.u, v.bits))
		}
		return int(v.u), nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return 0, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		if !v.dec.Num().IsInt64() {
			return 0, fmt.Errorf("integer overflow for host int: %s", formatDecimal(v.dec))
		}
		n := v.dec.Num().Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", formatDecimal(v.dec))
		}
		return int(n), nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return 0, err
		}
		if !i.IsInt64() {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		n := i.Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindComplex:
		if imag(v.cx) != 0 || math.Trunc(real(v.cx)) != real(v.cx) {
			return 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		if real(v.cx) > float64(int(^uint(0)>>1)) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(real(v.cx)), nil
	case kindBigInt:
		if v.bi == nil {
			return 0, nil
		}
		if !v.bi.IsInt64() {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		n := v.bi.Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindBigFloat:
		if v.bf == nil {
			return 0, nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact || !i.IsInt64() {
			return 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		n := i.Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindInt:
		if v.i == nil || !v.i.IsInt64() {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		n := v.i.Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() || !v.qty.mag.Num().IsInt64() {
			return 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		n := v.qty.mag.Num().Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() || !v.iv.lo.Num().IsInt64() {
			return 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		n := v.iv.lo.Num().Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	case kindModInt:
		if v.mi == nil || !v.mi.val.IsInt64() {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		n := v.mi.val.Int64()
		if n > int64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", v.kind)
		}
		return int(n), nil
	default:
		n, err := v.AsNumber()
		if err != nil {
			return 0, err
		}
		if math.Trunc(n) != n {
			return 0, fmt.Errorf("expected integer, got %s", formatNumber(n))
		}
		return int(n), nil
	}
}

// Equal operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (v Value) Equal(other Value) bool {
	switch v.kind {
	case kindNull:
		return other.kind == kindNull
	case kindBool:
		return other.kind == kindBool && v.boo == other.boo
	case kindNumber:
		switch other.kind {
		case kindNumber:
			return v.num == other.num
		case kindUint:
			if other.ub != nil {
				lf, _ := new(big.Float).SetFloat64(v.num).Float64()
				ov, err := bigFloatFromValue(other)
				if err != nil {
					return false
				}
				return new(big.Float).SetFloat64(lf).Cmp(ov) == 0
			}
			return v.num == float64(other.u)
		case kindDecimal:
			r, err := decimalFromValue(v)
			if err != nil {
				return false
			}
			if other.dec == nil {
				return r.Sign() == 0
			}
			return r.Cmp(other.dec) == 0
		case kindInterval:
			iv, err := intervalFromValue(v)
			if err != nil {
				return false
			}
			if other.iv == nil {
				return iv.lo.Sign() == 0 && iv.hi.Sign() == 0
			}
			return iv.lo.Cmp(other.iv.lo) == 0 && iv.hi.Cmp(other.iv.hi) == 0
		default:
			return false
		}
	case kindDecimal:
		switch other.kind {
		case kindDecimal:
			if v.dec == nil || other.dec == nil {
				return v.dec == nil && other.dec == nil
			}
			return v.dec.Cmp(other.dec) == 0
		case kindNumber, kindUint, kindBool:
			r, err := decimalFromValue(other)
			if err != nil {
				return false
			}
			if v.dec == nil {
				return r.Sign() == 0
			}
			return v.dec.Cmp(r) == 0
		case kindInterval:
			if v.dec == nil || other.iv == nil {
				return v.dec == nil && other.iv == nil
			}
			return other.iv.lo.Cmp(v.dec) == 0 && other.iv.hi.Cmp(v.dec) == 0
		default:
			return false
		}
	case kindFixed:
		switch other.kind {
		case kindFixed, kindNumber, kindUint, kindBool, kindDecimal, kindInterval, kindBigInt, kindBigFloat, kindInt, kindQuantity, kindModInt:
			lr, err := fixedRatFromValue(v)
			if err != nil {
				return false
			}
			rr, err := fixedRatFromValue(other)
			if err != nil {
				return false
			}
			return lr.Cmp(rr) == 0
		default:
			return false
		}
	case kindInterval:
		switch other.kind {
		case kindInterval:
			if v.iv == nil || other.iv == nil {
				return v.iv == nil && other.iv == nil
			}
			return v.iv.lo.Cmp(other.iv.lo) == 0 && v.iv.hi.Cmp(other.iv.hi) == 0
		case kindNumber, kindUint, kindDecimal, kindBool, kindInt, kindBigInt, kindBigFloat, kindQuantity:
			ov, err := intervalFromValue(other)
			if err != nil {
				return false
			}
			if v.iv == nil {
				return ov.lo.Sign() == 0 && ov.hi.Sign() == 0
			}
			return v.iv.lo.Cmp(ov.lo) == 0 && v.iv.hi.Cmp(ov.hi) == 0
		default:
			return false
		}
	case kindModInt:
		switch other.kind {
		case kindModInt:
			if v.mi == nil || other.mi == nil {
				return v.mi == nil && other.mi == nil
			}
			return v.mi.mod.Cmp(other.mi.mod) == 0 && v.mi.val.Cmp(other.mi.val) == 0
		default:
			return false
		}
	case kindComplex:
		switch other.kind {
		case kindComplex:
			return v.cx == other.cx
		case kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindInt:
			if imag(v.cx) != 0 {
				return false
			}
			r, err := decimalFromValue(other)
			if err != nil {
				return false
			}
			or, _ := r.Float64()
			return real(v.cx) == or
		default:
			return false
		}
	case kindDual:
		return other.kind == kindDual && v.dual != nil && other.dual != nil && v.dual.primal == other.dual.primal && v.dual.tangent == other.dual.tangent
	case kindFuture:
		if other.kind != kindFuture || v.future == nil || other.future == nil || futureTypeString(v.future) != futureTypeString(other.future) || !futureIntSlicesEqual(v.future.shape, other.future.shape) || !equalStringSlices([]string{v.future.format}, []string{other.future.format}) || len(v.future.data) != len(other.future.data) || len(v.future.coords) != len(other.future.coords) {
			return false
		}
		for i := range v.future.data {
			if !v.future.data[i].Equal(other.future.data[i]) {
				return false
			}
		}
		for i := range v.future.coords {
			if !futureIntSlicesEqual(v.future.coords[i], other.future.coords[i]) {
				return false
			}
		}
		return true
	case kindDuration:
		return other.kind == kindDuration && v.duration == other.duration
	case kindDateTime:
		return other.kind == kindDateTime && compareDateTimeValues(v.datetime, other.datetime) == 0
	case kindTimeLocation:
		return other.kind == kindTimeLocation && equalTimeLocations(v.location, other.location)
	case kindUint:
		switch other.kind {
		case kindUint:
			eq, err := equalUnsignedValues(v, other)
			if err != nil {
				return false
			}
			return eq
		case kindNumber:
			if v.ub != nil {
				ov, err := bigFloatFromValue(other)
				if err != nil {
					return false
				}
				return new(big.Float).SetInt(v.ub).Cmp(ov) == 0
			}
			return float64(v.u) == other.num
		case kindDecimal:
			r, err := decimalFromValue(v)
			if err != nil {
				return false
			}
			if other.dec == nil {
				return r.Sign() == 0
			}
			return r.Cmp(other.dec) == 0
		case kindInterval:
			iv, err := intervalFromValue(v)
			if err != nil {
				return false
			}
			if other.iv == nil {
				return iv.lo.Sign() == 0 && iv.hi.Sign() == 0
			}
			return iv.lo.Cmp(other.iv.lo) == 0 && iv.hi.Cmp(other.iv.hi) == 0
		default:
			return false
		}
	case kindString:
		return other.kind == kindString && v.str == other.str
	case kindError:
		if other.kind != kindError {
			return false
		}
		vr := v.errorRecord()
		or := other.errorRecord()
		if vr == nil || or == nil {
			return vr == nil && or == nil
		}
		return vr.Equal(or)
	case kindResult:
		if other.kind != kindResult {
			return false
		}
		vr := v.resultRecord()
		or := other.resultRecord()
		if vr == nil || or == nil {
			return vr == nil && or == nil
		}
		if vr.ok != or.ok {
			return false
		}
		if vr.ok {
			return vr.val.Equal(or.val)
		}
		if vr.err == nil || or.err == nil {
			return vr.err == nil && or.err == nil
		}
		return vr.err.Equal(or.err)
	case kindOption:
		if other.kind != kindOption {
			return false
		}
		vr := v.optionRecord()
		or := other.optionRecord()
		if vr == nil || or == nil {
			return vr == nil && or == nil
		}
		if vr.present != or.present {
			return false
		}
		if !vr.present {
			return true
		}
		return vr.val.Equal(or.val)
	case kindBigInt:
		switch other.kind {
		case kindBigInt:
			return v.bi.Cmp(other.bi) == 0
		case kindBigFloat, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity, kindInt:
			ov, err := bigFloatFromValue(other)
			if err != nil {
				return false
			}
			return new(big.Float).SetInt(v.bi).Cmp(ov) == 0
		default:
			return false
		}
	case kindBigFloat:
		switch other.kind {
		case kindBigFloat:
			return v.bf.Cmp(other.bf) == 0
		case kindBigInt, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity, kindInt:
			ov, err := bigFloatFromValue(other)
			if err != nil {
				return false
			}
			return v.bf.Cmp(ov) == 0
		default:
			return false
		}
	case kindInt:
		switch other.kind {
		case kindInt:
			if v.i == nil || other.i == nil {
				return v.i == nil && other.i == nil
			}
			return v.i.Cmp(other.i) == 0
		case kindBigInt, kindBigFloat, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity:
			ov, err := bigIntFromValue(other)
			if err == nil {
				if v.i == nil {
					return ov.Sign() == 0
				}
				return v.i.Cmp(ov) == 0
			}
			of, err := bigFloatFromValue(other)
			if err != nil {
				return false
			}
			return new(big.Float).SetInt(v.i).Cmp(of) == 0
		default:
			return false
		}
	case kindQuantity:
		switch other.kind {
		case kindQuantity:
			return dimsEqual(v.qty.dims, other.qty.dims) && v.qty.mag.Cmp(other.qty.mag) == 0
		case kindBigInt, kindBigFloat, kindNumber, kindUint, kindDecimal, kindBool:
			ov, err := quantityFromValue(other)
			if err != nil {
				return false
			}
			return dimsEqual(v.qty.dims, ov.dims) && v.qty.mag.Cmp(ov.mag) == 0
		default:
			return false
		}
	case kindList:
		if other.kind != kindList || len(v.list) != len(other.list) {
			return false
		}
		for i := range v.list {
			if !v.list[i].Equal(other.list[i]) {
				return false
			}
		}
		return true
	case kindMap:
		if other.kind != kindMap || v.mapValue == nil || other.mapValue == nil || len(v.mapValue.entries) != len(other.mapValue.entries) {
			return false
		}
		for i, entry := range v.mapValue.entries {
			if !entry.key.Equal(other.mapValue.entries[i].key) || !entry.value.Equal(other.mapValue.entries[i].value) {
				return false
			}
		}
		return true
	case kindSet:
		if other.kind != kindSet || v.setValue == nil || other.setValue == nil || len(v.setValue.items) != len(other.setValue.items) {
			return false
		}
		for i, item := range v.setValue.items {
			if !item.Equal(other.setValue.items[i]) {
				return false
			}
		}
		return true
	case kindTuple:
		if other.kind != kindTuple || len(v.tuple) != len(other.tuple) {
			return false
		}
		for i := range v.tuple {
			if !v.tuple[i].Equal(other.tuple[i]) {
				return false
			}
		}
		return true
	case kindStruct:
		if other.kind != kindStruct || v.structType == nil || other.structType == nil || v.structType.name != other.structType.name || v.structValue == nil || other.structValue == nil || len(v.structValue.fields) != len(other.structValue.fields) {
			return false
		}
		for i := range v.structValue.fields {
			if !v.structValue.fields[i].Equal(other.structValue.fields[i]) {
				return false
			}
		}
		return true
	case kindTensor:
		if other.kind != kindTensor || len(v.shape) != len(other.shape) || len(v.data) != len(other.data) {
			return false
		}
		for i := range v.shape {
			if v.shape[i] != other.shape[i] {
				return false
			}
		}
		for i := range v.data {
			if !v.data[i].Equal(other.data[i]) {
				return false
			}
		}
		return true
	case kindStream:
		return other.kind == kindStream && v.stream != nil && other.stream != nil && v.stream == other.stream
	case kindFFILibrary:
		return other.kind == kindFFILibrary && v.ffi != nil && v.ffi == other.ffi
	case kindFFIHandle:
		return other.kind == kindFFIHandle && v.ffiHandle != nil && v.ffiHandle == other.ffiHandle
	case kindOSProcess:
		return other.kind == kindOSProcess && v.process != nil && v.process == other.process
	case kindTask:
		return other.kind == kindTask && v.task != nil && other.task != nil && v.task == other.task
	case kindFunction:
		return other.kind == kindFunction && v.fn != nil && other.fn != nil && v.fn.name == other.fn.name
	case kindModule:
		return other.kind == kindModule && v.mod != nil && other.mod != nil && v.mod.name == other.mod.name && v.mod.version == other.mod.version
	case kindEnum:
		if other.kind != kindEnum || v.enum == nil || other.enum == nil || v.enum.typeName != other.enum.typeName || v.enum.caseName != other.enum.caseName || len(v.enum.payload) != len(other.enum.payload) {
			return false
		}
		for i := range v.enum.payload {
			if !v.enum.payload[i].Equal(other.enum.payload[i]) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

// Compare operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (v Value) Compare(other Value) (int, error) {
	switch v.kind {
	case kindNumber:
		switch other.kind {
		case kindNumber:
			switch {
			case v.num < other.num:
				return -1, nil
			case v.num > other.num:
				return 1, nil
			default:
				return 0, nil
			}
		case kindUint:
			if other.ub != nil {
				ov, err := bigFloatFromValue(other)
				if err != nil {
					return 0, err
				}
				nf := new(big.Float).SetFloat64(v.num)
				return nf.Cmp(ov), nil
			}
			ou := float64(other.u)
			switch {
			case v.num < ou:
				return -1, nil
			case v.num > ou:
				return 1, nil
			default:
				return 0, nil
			}
		case kindDecimal:
			r, err := decimalFromValue(v)
			if err != nil {
				return 0, err
			}
			if other.dec == nil {
				return r.Cmp(new(big.Rat).SetInt64(0)), nil
			}
			return r.Cmp(other.dec), nil
		case kindInterval:
			iv, err := intervalFromValue(v)
			if err != nil {
				return 0, err
			}
			return intervalCompare(intervalValueOf(iv), other)
		default:
			return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, decimals, and strings")
		}
	case kindDecimal:
		switch other.kind {
		case kindDecimal:
			if v.dec == nil || other.dec == nil {
				switch {
				case v.dec == nil && other.dec == nil:
					return 0, nil
				case v.dec == nil:
					return -1, nil
				default:
					return 1, nil
				}
			}
			return v.dec.Cmp(other.dec), nil
		case kindNumber, kindUint, kindBool:
			r, err := decimalFromValue(other)
			if err != nil {
				return 0, err
			}
			if v.dec == nil {
				return new(big.Rat).SetInt64(0).Cmp(r), nil
			}
			return v.dec.Cmp(r), nil
		case kindInterval:
			if v.dec == nil {
				if other.iv == nil {
					return 0, nil
				}
				return new(big.Rat).SetInt64(0).Cmp(other.iv.lo), nil
			}
			return intervalCompare(intervalValueOf(newIntervalValue(v.dec, v.dec)), other)
		default:
			return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, decimals, and strings")
		}
	case kindFixed:
		switch other.kind {
		case kindFixed, kindNumber, kindUint, kindBool, kindDecimal, kindBigInt, kindBigFloat, kindInt, kindInterval, kindQuantity, kindModInt:
			lr, err := fixedRatFromValue(v)
			if err != nil {
				return 0, err
			}
			rr, err := fixedRatFromValue(other)
			if err != nil {
				return 0, err
			}
			return lr.Cmp(rr), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for numeric values")
		}
	case kindInterval:
		switch other.kind {
		case kindInterval:
			if v.iv == nil || other.iv == nil {
				switch {
				case v.iv == nil && other.iv == nil:
					return 0, nil
				case v.iv == nil:
					return -1, nil
				default:
					return 1, nil
				}
			}
			return intervalCompare(v, other)
		case kindNumber, kindUint, kindDecimal, kindBool, kindBigInt, kindBigFloat, kindInt, kindQuantity:
			return intervalCompare(v, other)
		default:
			return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, decimals, and strings")
		}
	case kindModInt:
		switch other.kind {
		case kindModInt:
			if v.mi == nil || other.mi == nil {
				switch {
				case v.mi == nil && other.mi == nil:
					return 0, nil
				case v.mi == nil:
					return -1, nil
				default:
					return 1, nil
				}
			}
			if v.mi.mod.Cmp(other.mi.mod) != 0 {
				return 0, fmt.Errorf("ordering is only defined for modints with the same modulus")
			}
			return v.mi.val.Cmp(other.mi.val), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for modints")
		}
	case kindComplex:
		return 0, fmt.Errorf("ordering is not defined for complex numbers")
	case kindFuture:
		return 0, fmt.Errorf("ordering is not defined for future numeric and shape values")
	case kindError:
		return 0, fmt.Errorf("ordering is not defined for errors")
	case kindTensor:
		return 0, fmt.Errorf("ordering is not defined for tensors")
	case kindStream:
		return 0, fmt.Errorf("ordering is not defined for streams")
	case kindTask:
		return 0, fmt.Errorf("ordering is not defined for task handles")
	case kindBigInt:
		switch other.kind {
		case kindBigInt:
			return v.bi.Cmp(other.bi), nil
		case kindBigFloat, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity, kindInt:
			ov, err := bigFloatFromValue(other)
			if err != nil {
				return 0, err
			}
			return new(big.Float).SetInt(v.bi).Cmp(ov), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for numeric values")
		}
	case kindBigFloat:
		switch other.kind {
		case kindBigFloat:
			return v.bf.Cmp(other.bf), nil
		case kindBigInt, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity, kindInt:
			ov, err := bigFloatFromValue(other)
			if err != nil {
				return 0, err
			}
			return v.bf.Cmp(ov), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for numeric values")
		}
	case kindInt:
		switch other.kind {
		case kindInt:
			if v.i == nil || other.i == nil {
				switch {
				case v.i == nil && other.i == nil:
					return 0, nil
				case v.i == nil:
					return -1, nil
				default:
					return 1, nil
				}
			}
			return v.i.Cmp(other.i), nil
		case kindBigInt, kindBigFloat, kindNumber, kindUint, kindDecimal, kindInterval, kindBool, kindQuantity:
			ov, err := bigIntFromValue(other)
			if err == nil {
				if v.i == nil {
					return -ov.Sign(), nil
				}
				return v.i.Cmp(ov), nil
			}
			of, err := bigFloatFromValue(other)
			if err != nil {
				return 0, err
			}
			return new(big.Float).SetInt(v.i).Cmp(of), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for numeric values")
		}
	case kindQuantity:
		switch other.kind {
		case kindQuantity:
			if !dimsEqual(v.qty.dims, other.qty.dims) {
				return 0, fmt.Errorf("ordering is only defined for values with the same dimensions")
			}
			return v.qty.mag.Cmp(other.qty.mag), nil
		case kindBigInt, kindBigFloat, kindNumber, kindUint, kindDecimal, kindInterval, kindBool:
			ov, err := quantityFromValue(other)
			if err != nil {
				return 0, err
			}
			if !dimsEqual(v.qty.dims, ov.dims) {
				return 0, fmt.Errorf("ordering is only defined for values with the same dimensions")
			}
			return v.qty.mag.Cmp(ov.mag), nil
		default:
			return 0, fmt.Errorf("ordering is only defined for numeric values")
		}
	case kindString:
		return strings.Compare(v.str, other.str), nil
	case kindDuration:
		if other.kind != kindDuration {
			return 0, fmt.Errorf("duration ordering requires another duration")
		}
		return compareInt64(v.duration, other.duration), nil
	case kindDateTime:
		if other.kind != kindDateTime {
			return 0, fmt.Errorf("datetime ordering requires another datetime")
		}
		return compareDateTimeValues(v.datetime, other.datetime), nil
	case kindUint:
		switch other.kind {
		case kindUint:
			cmp, err := compareUnsignedValues(v, other)
			if err != nil {
				return 0, err
			}
			return cmp, nil
		case kindNumber:
			if v.ub != nil {
				ov, err := bigFloatFromValue(other)
				if err != nil {
					return 0, err
				}
				return new(big.Float).SetInt(v.ub).Cmp(ov), nil
			}
			ov := other.num
			switch {
			case float64(v.u) < ov:
				return -1, nil
			case float64(v.u) > ov:
				return 1, nil
			default:
				return 0, nil
			}
		case kindDecimal:
			r, err := decimalFromValue(v)
			if err != nil {
				return 0, err
			}
			if other.dec == nil {
				return r.Cmp(new(big.Rat).SetInt64(0)), nil
			}
			return r.Cmp(other.dec), nil
		case kindInterval:
			iv, err := intervalFromValue(v)
			if err != nil {
				return 0, err
			}
			return intervalCompare(intervalValueOf(iv), other)
		default:
			return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, decimals, and strings")
		}
	default:
		return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, decimals, and strings")
	}
}

// String operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (v Value) String() string {
	switch v.kind {
	case kindNull:
		return "null"
	case kindBool:
		if v.boo {
			return "true"
		}
		return "false"
	case kindNumber:
		return formatFloatValue(v)
	case kindDecimal:
		return formatDecimal(v.dec)
	case kindFixed:
		return fixedString(v.fx)
	case kindInterval:
		return formatInterval(v.iv)
	case kindModInt:
		return formatModInt(v.mi)
	case kindComplex:
		return formatComplexStyled(v.cx, v.fbits)
	case kindDual:
		if v.dual == nil {
			return "dual(0, 0)"
		}
		return "dual(" + formatNumber(v.dual.primal) + ", " + formatNumber(v.dual.tangent) + ")"
	case kindFuture:
		if v.future == nil {
			return "future<?>"
		}
		if v.future.name == "poly" && len(v.future.data) == 0 {
			return futureTypeString(v.future) + "[]"
		}
		if v.future.name == "sparse_matrix" || v.future.name == "sparse_tensor" {
			entries := make([]string, len(v.future.data))
			for i := range v.future.data {
				entries[i] = fmt.Sprintf("%s: %s", formatFutureCoord(v.future.coords[i]), v.future.data[i].String())
			}
			return futureTypeString(v.future) + "(" + strings.Join(entries, ", ") + ")"
		}
		items := make([]string, len(v.future.data))
		for i := range v.future.data {
			items[i] = v.future.data[i].String()
		}
		return futureTypeString(v.future) + "(" + strings.Join(items, ", ") + ")"
	case kindTCPListener:
		if v.listener == nil {
			return "<tcp-listener>"
		}
		return "<tcp-listener " + v.listener.address() + ">"
	case kindUDPSocket:
		if v.udp == nil {
			return "<udp-socket>"
		}
		return "<udp-socket " + v.udp.address() + ">"
	case kindFFILibrary:
		return "<ffi-library>"
	case kindFFIHandle:
		if v.ffiHandle == nil {
			return "<ffi-handle>"
		}
		return "<ffi-handle " + strings.TrimPrefix(v.ffiHandle.typeID, "handle:") + ">"
	case kindOSProcess:
		if v.process == nil {
			return "<os-process>"
		}
		return "<os-process " + strconv.Itoa(v.process.pid()) + ">"
	case kindDuration:
		return `duration(` + strconv.Quote(formatDuration(v.duration)) + `)`
	case kindDateTime:
		return formatDateTimeValue(v.datetime)
	case kindTimeLocation:
		return formatTimeLocation(v.location)
	case kindError:
		if rec := v.errorRecord(); rec != nil {
			return rec.String()
		}
		return "error(" + strconv.Quote(v.errMsg) + ")"
	case kindResult:
		if rec := v.resultRecord(); rec != nil {
			if rec.ok {
				return "ok(" + rec.val.String() + ")"
			}
			if rec.err != nil {
				return "err(" + rec.err.String() + ")"
			}
		}
		return "result<?>"
	case kindOption:
		if rec := v.optionRecord(); rec != nil {
			if rec.present {
				return "some(" + rec.val.String() + ")"
			}
		}
		return "none"
	case kindBigInt:
		if v.bi == nil {
			return "0n"
		}
		return v.bi.String() + "n"
	case kindBigFloat:
		return formatBigFloat(v.bf)
	case kindInt:
		return formatInt(v.i, v.ibits)
	case kindQuantity:
		return formatQuantity(v.qty)
	case kindUint:
		return formatUintValue(v)
	case kindString:
		return strconv.Quote(v.str)
	case kindList:
		var b bytes.Buffer
		b.WriteByte('[')
		for i, item := range v.list {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(item.String())
		}
		b.WriteByte(']')
		return b.String()
	case kindMap:
		var b bytes.Buffer
		b.WriteString("map{")
		if v.mapValue != nil {
			for i, entry := range v.mapValue.entries {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(entry.key.String())
				b.WriteString(": ")
				b.WriteString(entry.value.String())
			}
		}
		b.WriteByte('}')
		return b.String()
	case kindSet:
		var b bytes.Buffer
		b.WriteString("set{")
		if v.setValue != nil {
			for i, item := range v.setValue.items {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(item.String())
			}
		}
		b.WriteByte('}')
		return b.String()
	case kindTuple:
		var b bytes.Buffer
		b.WriteByte('(')
		for i, item := range v.tuple {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(item.String())
		}
		if len(v.tuple) == 1 {
			b.WriteByte(',')
		}
		b.WriteByte(')')
		return b.String()
	case kindStruct:
		if v.structType == nil {
			return "<struct>"
		}
		if v.structValue == nil {
			return v.structType.name
		}
		if v.structType.newtype && len(v.structValue.fields) == 1 {
			return v.structType.name + "(" + v.structValue.fields[0].String() + ")"
		}
		parts := make([]string, len(v.structType.fields))
		for i, field := range v.structType.fields {
			parts[i] = field.name + ": " + v.structValue.fields[i].String()
		}
		return v.structType.name + " { " + strings.Join(parts, ", ") + " }"
	case kindTensor:
		return formatTensor(v.shape, v.data)
	case kindStream:
		if v.stream == nil {
			return "<stream>"
		}
		return v.stream.String()
	case kindTask:
		if v.task == nil {
			return "<task>"
		}
		return v.task.String()
	case kindFunction:
		if v.fn == nil {
			return "<function>"
		}
		return "<function " + v.fn.name + ">"
	case kindModule:
		if v.mod == nil {
			return "<module>"
		}
		if v.mod.version != "" {
			return "<module " + v.mod.name + "@" + v.mod.version + ">"
		}
		return "<module " + v.mod.name + ">"
	case kindEnum:
		if v.enum == nil {
			return "<enum>"
		}
		name := v.enum.typeName + "." + v.enum.caseName
		if len(v.enum.payload) == 0 {
			return name
		}
		parts := make([]string, len(v.enum.payload))
		for i, item := range v.enum.payload {
			parts[i] = item.String()
		}
		return name + "(" + strings.Join(parts, ", ") + ")"
	default:
		return "null"
	}
}

// MarshalJSON operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []byte and an error. Callers should only trust the value when the
// error is nil.
func (v Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.toInterface())
}

// toInterface operates on Value values and keeps the value-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns any so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (v Value) toInterface() any {
	switch v.kind {
	case kindNull:
		return nil
	case kindBool:
		return v.boo
	case kindNumber:
		if v.fbits != 0 {
			if v.bfloat {
				return map[string]any{"bfloat": formatFloatValue(v), "bits": v.fbits}
			}
			return map[string]any{"float": formatFloatValue(v), "bits": v.fbits}
		}
		return v.num
	case kindDecimal:
		return formatDecimal(v.dec)
	case kindFixed:
		if v.fx == nil {
			return map[string]any{"fixed": map[string]any{"width": 64, "frac": 0, "raw": "0", "value": "0"}}
		}
		return map[string]any{"fixed": map[string]any{
			"width": v.fx.width,
			"frac":  v.fx.frac,
			"raw":   v.fx.raw.String(),
			"value": fixedToRat(v.fx).RatString(),
		}}
	case kindInterval:
		if v.iv == nil {
			return map[string]any{"interval": map[string]any{"lo": "0", "hi": "0"}}
		}
		return map[string]any{"interval": map[string]any{"lo": v.iv.lo.RatString(), "hi": v.iv.hi.RatString()}}
	case kindModInt:
		if v.mi == nil {
			return map[string]any{"modint": map[string]any{"mod": "1", "value": "0"}}
		}
		return map[string]any{"modint": map[string]any{"mod": v.mi.mod.String(), "value": v.mi.val.String()}}
	case kindComplex:
		return []any{real(v.cx), imag(v.cx)}
	case kindDual:
		if v.dual == nil {
			return map[string]any{"dual": []float64{0, 0}}
		}
		return map[string]any{"dual": []float64{v.dual.primal, v.dual.tangent}}
	case kindFuture:
		if v.future == nil {
			return map[string]any{"future": nil}
		}
		data := make([]any, len(v.future.data))
		for i := range v.future.data {
			data[i] = v.future.data[i].toInterface()
		}
		out := map[string]any{"type": futureTypeString(v.future), "data": data}
		if v.future.name == "sparse_matrix" || v.future.name == "sparse_tensor" {
			coords := make([]any, len(v.future.coords))
			for i := range v.future.coords {
				coords[i] = append([]int(nil), v.future.coords[i]...)
			}
			out["shape"] = append([]int(nil), v.future.shape...)
			out["coords"] = coords
			out["format"] = v.future.format
		}
		return map[string]any{"future": out}
	case kindDuration:
		return map[string]any{"duration": map[string]any{"nanoseconds": strconv.FormatInt(v.duration, 10)}}
	case kindDateTime:
		return dateTimeInterface(v.datetime)
	case kindTimeLocation:
		return timeLocationInterface(v.location)
	case kindTCPListener:
		if v.listener == nil {
			return nil
		}
		return map[string]any{"tcp_listener": v.listener.address()}
	case kindUDPSocket:
		if v.udp == nil {
			return nil
		}
		return map[string]any{"udp_socket": v.udp.address()}
	case kindFFILibrary:
		return map[string]any{"ffi_library": true}
	case kindFFIHandle:
		if v.ffiHandle == nil {
			return nil
		}
		return map[string]any{"ffi_handle": strings.TrimPrefix(v.ffiHandle.typeID, "handle:"), "closed": v.ffiHandle.IsClosed()}
	case kindOSProcess:
		if v.process == nil {
			return nil
		}
		return map[string]any{"os_process": v.process.pid(), "closed": v.process.isClosed()}
	case kindError:
		if rec := v.errorRecord(); rec != nil {
			return rec.publicInterface()
		}
		return map[string]any{"error": v.errMsg}
	case kindResult:
		if rec := v.resultRecord(); rec != nil {
			if rec.ok {
				return map[string]any{"result": map[string]any{"ok": true, "value": rec.val.toInterface()}}
			}
			if rec.err != nil {
				return map[string]any{"result": map[string]any{"ok": false, "error": rec.err.publicInterface()}}
			}
		}
		return map[string]any{"result": nil}
	case kindOption:
		if rec := v.optionRecord(); rec != nil {
			if rec.present {
				return map[string]any{"option": map[string]any{"present": true, "value": rec.val.toInterface()}}
			}
			return map[string]any{"option": map[string]any{"present": false}}
		}
		return map[string]any{"option": nil}
	case kindEnum:
		if v.enum == nil {
			return map[string]any{"enum": nil}
		}
		payload := make([]any, len(v.enum.payload))
		for i, item := range v.enum.payload {
			payload[i] = item.toInterface()
		}
		return map[string]any{"enum": map[string]any{"type": v.enum.typeName, "case": v.enum.caseName, "payload": payload}}
	case kindBigInt:
		if v.bi == nil {
			return map[string]any{"bigint": "0"}
		}
		return map[string]any{"bigint": v.bi.String()}
	case kindBigFloat:
		if v.bf == nil {
			return map[string]any{"bigfloat": "0"}
		}
		r, _ := v.bf.Rat(nil)
		return map[string]any{"bigfloat": r.RatString()}
	case kindInt:
		if v.i == nil {
			return map[string]any{"int": "0", "bits": v.ibits}
		}
		return map[string]any{"int": v.i.String(), "bits": v.ibits}
	case kindQuantity:
		if v.qty == nil {
			return map[string]any{"quantity": nil}
		}
		dims := make(map[string]int, len(v.qty.dims))
		for k, val := range v.qty.dims {
			dims[k] = val
		}
		return map[string]any{"quantity": map[string]any{"mag": v.qty.mag.RatString(), "dims": dims}}
	case kindUint:
		if v.packed {
			if v.native {
				return map[string]any{"ufnum": v.u, "bits": 64}
			}
			if v.ub != nil {
				return map[string]any{"uf": v.ub.String(), "bits": v.ubitsToUint16()}
			}
			return map[string]any{"uf": v.u, "bits": v.bits}
		}
		if v.ub != nil {
			return map[string]any{"uint": v.ub.String(), "bits": v.ubitsToUint16()}
		}
		return v.u
	case kindString:
		return v.str
	case kindList:
		items := make([]any, len(v.list))
		for i := range v.list {
			items[i] = v.list[i].toInterface()
		}
		return items
	case kindMap:
		entries := make([]any, 0)
		if v.mapValue != nil {
			entries = make([]any, len(v.mapValue.entries))
			for i, entry := range v.mapValue.entries {
				entries[i] = []any{entry.key.toInterface(), entry.value.toInterface()}
			}
		}
		return entries
	case kindSet:
		items := make([]any, 0)
		if v.setValue != nil {
			items = make([]any, len(v.setValue.items))
			for i := range v.setValue.items {
				items[i] = v.setValue.items[i].toInterface()
			}
		}
		return items
	case kindTuple:
		items := make([]any, len(v.tuple))
		for i := range v.tuple {
			items[i] = v.tuple[i].toInterface()
		}
		return items
	case kindStruct:
		if v.structType == nil || v.structValue == nil {
			return nil
		}
		items := make(map[string]any, len(v.structType.fields))
		for i, field := range v.structType.fields {
			key := field.name
			if tag, ok := field.tagValues["json"]; ok {
				parts := strings.Split(tag, ",")
				if len(parts) > 0 && parts[0] != "-" && parts[0] != "" {
					key = parts[0]
				}
				if len(parts) > 0 && parts[0] == "-" {
					continue
				}
				omitEmpty := false
				for _, option := range parts[1:] {
					omitEmpty = omitEmpty || option == "omitempty"
				}
				if omitEmpty && !v.structValue.fields[i].Truthy() {
					continue
				}
			}
			items[key] = v.structValue.fields[i].toInterface()
		}
		return items
	case kindTensor:
		data := make([]any, len(v.data))
		for i := range v.data {
			data[i] = v.data[i].toInterface()
		}
		shape := make([]int, len(v.shape))
		copy(shape, v.shape)
		return map[string]any{"shape": shape, "data": data}
	case kindStream:
		if v.stream == nil {
			return map[string]any{"stream": nil}
		}
		return map[string]any{"stream": v.stream.toInterface()}
	case kindTask:
		if v.task == nil {
			return map[string]any{"task": nil}
		}
		id, status, res, err := v.task.snapshot()
		out := map[string]any{"id": id, "status": status}
		if status == "done" {
			if err != nil {
				out["error"] = err.Error()
			} else {
				out["result"] = res.toInterface()
			}
		}
		return map[string]any{"task": out}
	case kindFunction, kindModule:
		return v.String()
	default:
		return nil
	}
}

// formatNumber handles the format number step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatNumber(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	if math.Trunc(n) == n {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// formatFloat handles the format float step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatFloat(n float64, bits uint16) string {
	if bits == 0 {
		return formatNumber(n)
	}
	return formatNumber(n) + "f" + strconv.FormatUint(uint64(bits), 10)
}

// formatBFloat handles the format bfloat step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatBFloat(n float64, bits uint16) string {
	if bits == 0 {
		return formatNumber(n)
	}
	return formatNumber(n) + "bf" + strconv.FormatUint(uint64(bits), 10)
}

// formatFloatValue handles the format float value step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatFloatValue(v Value) string {
	if v.bfloat {
		return formatBFloat(v.num, v.fbits)
	}
	return formatFloat(v.num, v.fbits)
}

// formatComplex handles the format complex step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatComplex(c complex128) string {
	return formatComplexStyled(c, 0)
}

// formatComplexStyled renders a complex value while retaining an explicitly
// selected component precision in the observable text form.
func formatComplexStyled(c complex128, bits uint16) string {
	re := real(c)
	im := imag(c)
	format := formatNumber
	if bits != 0 {
		format = func(value float64) string { return formatFloat(value, bits) }
	}
	switch {
	case im == 0:
		if bits == 0 {
			return format(re) + "+0i"
		}
		return format(re) + "+" + format(0) + "i"
	case re == 0:
		return format(im) + "i"
	case im < 0:
		return format(re) + format(im) + "i"
	default:
		return format(re) + "+" + format(im) + "i"
	}
}

// textify handles the textify step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func textify(v Value) string {
	switch v.kind {
	case kindNull:
		return ""
	case kindBool:
		if v.boo {
			return "true"
		}
		return "false"
	case kindNumber:
		return formatFloatValue(v)
	case kindDecimal:
		return formatDecimal(v.dec)
	case kindFixed:
		return fixedString(v.fx)
	case kindInterval:
		return formatInterval(v.iv)
	case kindModInt:
		return formatModInt(v.mi)
	case kindComplex:
		return formatComplex(v.cx)
	case kindDual:
		return v.String()
	case kindDuration:
		return formatDuration(v.duration)
	case kindDateTime:
		return formatDateTimeText(v.datetime)
	case kindTimeLocation:
		return formatTimeLocation(v.location)
	case kindError:
		if rec := v.errorRecord(); rec != nil {
			return rec.displayMessage()
		}
		return v.errMsg
	case kindBigInt:
		if v.bi == nil {
			return "0"
		}
		return v.bi.String()
	case kindBigFloat:
		if v.bf == nil {
			return "0"
		}
		return v.bf.Text('g', -1)
	case kindInt:
		return formatInt(v.i, v.ibits)
	case kindQuantity:
		return formatQuantity(v.qty)
	case kindUint:
		return formatUintValue(v)
	case kindString:
		return v.str
	case kindList:
		parts := make([]string, len(v.list))
		for i := range v.list {
			parts[i] = textify(v.list[i])
		}
		return strings.Join(parts, " ")
	case kindTensor:
		return v.String()
	case kindFuture:
		return v.String()
	case kindFunction, kindModule:
		return v.String()
	default:
		return ""
	}
}
