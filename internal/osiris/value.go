package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"
)

type valueKind uint8

const (
	kindNull valueKind = iota
	kindBool
	kindNumber
	kindUint
	kindString
	kindList
	kindFunction
	kindModule
)

type callable = Builtin

type moduleLookup func(ctx context.Context, name string) (Value, error)

type functionValue struct {
	name string
	fn   callable
}

type moduleValue struct {
	name    string
	version string
	lookup  moduleLookup
}

type Value struct {
	kind valueKind
	num  float64
	u    uint64
	bits uint8
	boo  bool
	str  string
	list []Value
	fn   *functionValue
	mod  *moduleValue
}

func nullValue() Value            { return Value{kind: kindNull} }
func boolValue(v bool) Value      { return Value{kind: kindBool, boo: v} }
func numberValue(v float64) Value { return Value{kind: kindNumber, num: v} }
func uintValue(v uint64, bits uint8) Value {
	if bits == 0 || bits > 64 {
		bits = 64
	}
	return Value{kind: kindUint, u: v & maskForWidth(bits), bits: bits}
}
func stringValue(v string) Value { return Value{kind: kindString, str: v} }
func listValue(v []Value) Value  { return Value{kind: kindList, list: v} }
func functionValueOf(name string, fn callable) Value {
	return Value{kind: kindFunction, fn: &functionValue{name: name, fn: fn}}
}
func moduleValueOf(name, version string, lookup moduleLookup) Value {
	return Value{kind: kindModule, mod: &moduleValue{name: name, version: version, lookup: lookup}}
}

func (v Value) Kind() string { return v.kind.String() }

func (k valueKind) String() string {
	switch k {
	case kindNull:
		return "null"
	case kindBool:
		return "bool"
	case kindNumber:
		return "number"
	case kindUint:
		return "uint"
	case kindString:
		return "string"
	case kindList:
		return "list"
	case kindFunction:
		return "function"
	case kindModule:
		return "module"
	default:
		return "<?>"
	}
}
func (v Value) Truthy() bool {
	switch v.kind {
	case kindNull:
		return false
	case kindBool:
		return v.boo
	case kindNumber:
		return v.num != 0
	case kindUint:
		return v.u != 0
	case kindString:
		return v.str != ""
	case kindList:
		return len(v.list) != 0
	case kindFunction, kindModule:
		return true
	default:
		return false
	}
}
func (v Value) AsNumber() (float64, error) {
	switch v.kind {
	case kindNumber:
		return v.num, nil
	case kindUint:
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

func (v Value) AsString() (string, error) {
	switch v.kind {
	case kindString:
		return v.str, nil
	case kindNull:
		return "", nil
	default:
		return "", fmt.Errorf("expected string, got %s", v.kind)
	}
}

func (v Value) AsUint() (uint64, uint8, error) {
	switch v.kind {
	case kindUint:
		return v.u, v.bits, nil
	case kindNumber:
		if math.Trunc(v.num) != v.num || v.num < 0 {
			return 0, 0, fmt.Errorf("expected unsigned integer, got %s", formatNumber(v.num))
		}
		return uint64(v.num), 64, nil
	case kindBool:
		if v.boo {
			return 1, 64, nil
		}
		return 0, 64, nil
	default:
		return 0, 0, fmt.Errorf("expected unsigned integer, got %s", v.kind)
	}
}

func (v Value) AsList() ([]Value, error) {
	if v.kind != kindList {
		return nil, fmt.Errorf("expected list, got %s", v.kind)
	}
	return v.list, nil
}

func (v Value) AsFunction() (callable, error) {
	if v.kind != kindFunction || v.fn == nil {
		return nil, fmt.Errorf("expected function, got %s", v.kind)
	}
	return v.fn.fn, nil
}

func (v Value) AsModule() (*moduleValue, error) {
	if v.kind != kindModule || v.mod == nil {
		return nil, fmt.Errorf("expected module, got %s", v.kind)
	}
	return v.mod, nil
}

func (v Value) Int() (int, error) {
	switch v.kind {
	case kindUint:
		if v.u > uint64(^uint(0)>>1) {
			return 0, fmt.Errorf("integer overflow for host int: %s", formatUint(v.u, v.bits))
		}
		return int(v.u), nil
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

func (v Value) Equal(other Value) bool {
	switch v.kind {
	case kindNull:
		return other.kind == kindNull
	case kindBool:
		return other.kind == kindBool && v.boo == other.boo
	case kindNumber:
		return other.kind == kindNumber && v.num == other.num
	case kindUint:
		return other.kind == kindUint && v.u == other.u && v.bits == other.bits
	case kindString:
		return other.kind == kindString && v.str == other.str
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
	case kindFunction:
		return other.kind == kindFunction && v.fn != nil && other.fn != nil && v.fn.name == other.fn.name
	case kindModule:
		return other.kind == kindModule && v.mod != nil && other.mod != nil && v.mod.name == other.mod.name && v.mod.version == other.mod.version
	default:
		return false
	}
}

func (v Value) Compare(other Value) (int, error) {
	if v.kind != other.kind {
		return 0, fmt.Errorf("cannot compare %s and %s", v.kind, other.kind)
	}
	switch v.kind {
	case kindNumber:
		switch {
		case v.num < other.num:
			return -1, nil
		case v.num > other.num:
			return 1, nil
		default:
			return 0, nil
		}
	case kindString:
		return strings.Compare(v.str, other.str), nil
	case kindUint:
		switch {
		case v.u < other.u:
			return -1, nil
		case v.u > other.u:
			return 1, nil
		default:
			return 0, nil
		}
	default:
		return 0, fmt.Errorf("ordering is only defined for numbers, unsigned integers, and strings")
	}
}

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
		return formatNumber(v.num)
	case kindUint:
		return formatUint(v.u, v.bits)
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

func (v Value) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.toInterface())
}

func (v Value) toInterface() any {
	switch v.kind {
	case kindNull:
		return nil
	case kindBool:
		return v.boo
	case kindNumber:
		return v.num
	case kindUint:
		return v.u
	case kindString:
		return v.str
	case kindList:
		items := make([]any, len(v.list))
		for i := range v.list {
			items[i] = v.list[i].toInterface()
		}
		return items
	case kindFunction, kindModule:
		return v.String()
	default:
		return nil
	}
}

func formatNumber(n float64) string {
	if math.IsNaN(n) || math.IsInf(n, 0) {
		return strconv.FormatFloat(n, 'g', -1, 64)
	}
	if math.Trunc(n) == n {
		return strconv.FormatInt(int64(n), 10)
	}
	return strconv.FormatFloat(n, 'g', -1, 64)
}

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
		return formatNumber(v.num)
	case kindUint:
		return formatUint(v.u, v.bits)
	case kindString:
		return v.str
	case kindList:
		parts := make([]string, len(v.list))
		for i := range v.list {
			parts[i] = textify(v.list[i])
		}
		return strings.Join(parts, " ")
	case kindFunction, kindModule:
		return v.String()
	default:
		return ""
	}
}
