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
	kindBigInt
	kindBigFloat
	kindInt
	kindQuantity
	kindUint
	kindString
	kindList
	kindTensor
	kindFunction
	kindModule
)

type callable = Builtin

type moduleLookup func(ctx context.Context, name string) (Value, error)

type functionValue struct {
	name    string
	builtin callable
	params  []string
	body    Expr
	closure *scope
	session *Session
}

type moduleValue struct {
	name    string
	version string
	lookup  moduleLookup
	members map[string]Value
}

type Value struct {
	kind   valueKind
	num    float64
	fbits  uint16
	bfloat bool
	dec    *big.Rat
	fx     *fixedValue
	iv     *intervalValue
	mi     *modIntValue
	cx     complex128
	bi     *big.Int
	bf     *big.Float
	i      *big.Int
	ibits  uint16
	qty    *quantityValue
	u      uint64
	bits   uint8
	ub     *big.Int
	ubits  uint16
	packed bool
	native bool
	boo    bool
	str    string
	list   []Value
	shape  []int
	data   []Value
	errMsg string
	fn     *functionValue
	mod    *moduleValue
}

// nullValue performs the null value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func nullValue() Value { return Value{kind: kindNull} }

// boolValue performs the bool value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func boolValue(v bool) Value { return Value{kind: kindBool, boo: v} }

// numberValue performs the number value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func numberValue(v float64) Value { return floatValue(v, 0) }

// floatValue performs the float value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func floatValue(v float64, bits uint16) Value {
	if bits == 32 {
		v = float64(float32(v))
	}
	return Value{kind: kindNumber, num: v, fbits: bits}
}

// bfloatValue performs the bfloat value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func bfloatValue(v float64, bits uint16) Value {
	if bits == 32 {
		v = float64(float32(v))
	}
	return Value{kind: kindNumber, num: v, fbits: bits, bfloat: true}
}

// decimalValue performs the decimal value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func decimalValue(v *big.Rat) Value {
	if v == nil {
		v = new(big.Rat)
	}
	return Value{kind: kindDecimal, dec: new(big.Rat).Set(v)}
}

// fixedValueOfValue performs the fixed value of value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
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

// intervalValueOf performs the interval value of step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func intervalValueOf(v *intervalValue) Value {
	if v == nil {
		v = newIntervalValue(new(big.Rat), new(big.Rat))
	}
	return Value{kind: kindInterval, iv: newIntervalValue(v.lo, v.hi)}
}

// modIntValueOfValue performs the mod int value of value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func modIntValueOfValue(v *modIntValue) Value {
	if v == nil {
		v = &modIntValue{mod: big.NewInt(1), val: new(big.Int)}
	}
	return Value{kind: kindModInt, mi: &modIntValue{mod: new(big.Int).Set(v.mod), val: new(big.Int).Set(v.val)}}
}

// complexValue performs the complex value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func complexValue(v complex128) Value { return Value{kind: kindComplex, cx: v} }

// errorValue performs the error value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func errorValue(msg string) Value { return Value{kind: kindError, errMsg: msg} }

// bigintValue performs the bigint value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func bigintValue(v *big.Int) Value { return bigIntValue(v) }

// bigfloatValue performs the bigfloat value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func bigfloatValue(v *big.Float) Value { return bigFloatValue(v) }

// intValue performs the int value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func intValue(v *big.Int, bits uint16) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindInt, i: wrapSignedWidth(v, bits), ibits: bits}
}

// measureValue performs the measure value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func measureValue(v *big.Rat, dims map[string]int) Value {
	return quantityValueOf(v, dims)
}

// uintValue performs the uint value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func uintValue(v uint64, bits uint8) Value {
	if bits == 0 || bits > 64 {
		bits = 64
	}
	return Value{kind: kindUint, u: v & maskForWidth(bits), bits: bits}
}

// packedUintValue performs the packed uint value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func packedUintValue(v uint64, bits uint8, native bool) Value {
	if bits == 0 || bits > 64 {
		bits = 64
	}
	return Value{kind: kindUint, u: v & maskForWidth(bits), bits: bits, packed: true, native: native}
}

// wideUintValue performs the wide uint value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func wideUintValue(v *big.Int, bits uint16) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindUint, ub: wrapUnsignedWidth(v, bits), ubits: bits}
}

// packedWideUintValue performs the packed wide uint value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func packedWideUintValue(v *big.Int, bits uint16, native bool) Value {
	if bits == 0 {
		bits = 64
	}
	return Value{kind: kindUint, ub: wrapUnsignedWidth(v, bits), ubits: bits, packed: true, native: native}
}

// stringValue performs the string value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func stringValue(v string) Value { return Value{kind: kindString, str: v} }

// listValue performs the list value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func listValue(v []Value) Value { return Value{kind: kindList, list: v} }

// tensorValue performs the tensor value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func tensorValue(shape []int, data []Value) Value {
	return newTensorValue(shape, data)
}

// functionValueOf performs the function value of step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func functionValueOf(name string, fn callable) Value {
	return Value{kind: kindFunction, fn: &functionValue{name: name, builtin: fn}}
}

// userFunctionValue performs the user function value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func userFunctionValue(name string, params []string, body Expr, closure *scope, sess *Session) Value {
	return Value{kind: kindFunction, fn: &functionValue{
		name:    name,
		params:  append([]string(nil), params...),
		body:    body,
		closure: closure,
		session: sess,
	}}
}

// moduleValueOf performs the module value of step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func moduleValueOf(name, version string, lookup moduleLookup, members map[string]Value) Value {
	return Value{kind: kindModule, mod: &moduleValue{name: name, version: version, lookup: lookup, members: members}}
}

// Kind handles on Value the kind. It keeps the Value subsystem self-contained by handling the kind-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
	return v.kind.String()
}

// String handles on valueKind the string. It keeps the valueKind subsystem self-contained by handling the string-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
	case kindTensor:
		return "tensor"
	case kindFunction:
		return "function"
	case kindModule:
		return "module"
	default:
		return "<?>"
	}
}

// Truthy handles on Value the truthy. It keeps the Value subsystem self-contained by handling the truthy-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
	case kindError:
		return false
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
	case kindTensor:
		return len(v.data) != 0
	case kindFunction, kindModule:
		return true
	default:
		return false
	}
}

// AsNumber handles on Value the as number. It keeps the Value subsystem self-contained by handling the as number-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// AsString handles on Value the as string. It keeps the Value subsystem self-contained by handling the as string-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (v Value) AsString() (string, error) {
	switch v.kind {
	case kindString:
		return v.str, nil
	case kindError:
		return "", errors.New(v.errMsg)
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

// AsUint handles on Value the as uint. It keeps the Value subsystem self-contained by handling the as uint-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// AsList handles on Value the as list. It keeps the Value subsystem self-contained by handling the as list-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (v Value) AsList() ([]Value, error) {
	if v.kind != kindList && v.kind != kindTensor {
		return nil, fmt.Errorf("expected list, got %s", v.kind)
	}
	if v.kind == kindTensor {
		return v.data, nil
	}
	return v.list, nil
}

// AsFunction handles on Value the as function. It keeps the Value subsystem self-contained by handling the as function-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (v Value) AsFunction() (callable, error) {
	if v.kind != kindFunction || v.fn == nil || v.fn.builtin == nil {
		return nil, fmt.Errorf("expected function, got %s", v.kind)
	}
	return v.fn.builtin, nil
}

// AsModule handles on Value the as module. It keeps the Value subsystem self-contained by handling the as module-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (v Value) AsModule() (*moduleValue, error) {
	if v.kind != kindModule || v.mod == nil {
		return nil, fmt.Errorf("expected module, got %s", v.kind)
	}
	return v.mod, nil
}

// Int handles on Value the int. It keeps the Value subsystem self-contained by handling the int-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// Equal handles on Value the equal. It keeps the Value subsystem self-contained by handling the equal-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
		return other.kind == kindError && v.errMsg == other.errMsg
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
	case kindFunction:
		return other.kind == kindFunction && v.fn != nil && other.fn != nil && v.fn.name == other.fn.name
	case kindModule:
		return other.kind == kindModule && v.mod != nil && other.mod != nil && v.mod.name == other.mod.name && v.mod.version == other.mod.version
	default:
		return false
	}
}

// Compare handles on Value the compare. It keeps the Value subsystem self-contained by handling the compare-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
	case kindError:
		return 0, fmt.Errorf("ordering is not defined for errors")
	case kindTensor:
		return 0, fmt.Errorf("ordering is not defined for tensors")
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

// String handles on Value the string. It keeps the Value subsystem self-contained by handling the string-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
		return formatComplex(v.cx)
	case kindError:
		return "error(" + strconv.Quote(v.errMsg) + ")"
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
	case kindTensor:
		return formatTensor(v.shape, v.data)
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
	default:
		return "null"
	}
}

// MarshalJSON handles on Value the marshal json. It keeps the Value subsystem self-contained by handling the marshal json-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (v Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.toInterface())
}

// toInterface handles on Value the to interface. It keeps the Value subsystem self-contained by handling the to interface-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
	case kindError:
		return map[string]any{"error": v.errMsg}
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
	case kindTensor:
		data := make([]any, len(v.data))
		for i := range v.data {
			data[i] = v.data[i].toInterface()
		}
		shape := make([]int, len(v.shape))
		copy(shape, v.shape)
		return map[string]any{"shape": shape, "data": data}
	case kindFunction, kindModule:
		return v.String()
	default:
		return nil
	}
}

// formatNumber performs the format number step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatNumber(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	if math.Trunc(n) == n {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

// formatFloat performs the format float step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatFloat(n float64, bits uint16) string {
	if bits == 0 {
		return formatNumber(n)
	}
	return formatNumber(n) + "f" + strconv.FormatUint(uint64(bits), 10)
}

// formatBFloat performs the format b float step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatBFloat(n float64, bits uint16) string {
	if bits == 0 {
		return formatNumber(n)
	}
	return formatNumber(n) + "bf" + strconv.FormatUint(uint64(bits), 10)
}

// formatFloatValue performs the format float value step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatFloatValue(v Value) string {
	if v.bfloat {
		return formatBFloat(v.num, v.fbits)
	}
	return formatFloat(v.num, v.fbits)
}

// formatComplex performs the format complex step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatComplex(c complex128) string {
	re := real(c)
	im := imag(c)
	switch {
	case im == 0:
		return formatNumber(re) + "+0i"
	case re == 0:
		return formatNumber(im) + "i"
	case im < 0:
		return formatNumber(re) + formatNumber(im) + "i"
	default:
		return formatNumber(re) + "+" + formatNumber(im) + "i"
	}
}

// textify performs the textify step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
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
	case kindError:
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
	case kindFunction, kindModule:
		return v.String()
	default:
		return ""
	}
}
