package osiris

import (
	"fmt"
	"math/big"
)

// isBindingTypeName admits the canonical scalar names and the non-numeric
// runtime kinds that have an unambiguous Value representation. A declaration
// is a checked constraint; it never performs an implicit conversion.
func isBindingTypeName(name string) bool {
	switch name {
	case "number", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "string", "bool", "null", "list", "tensor", "enum", "function", "duration", "datetime", "location":
		return true
	default:
		return false
	}
}

func checkBindingType(name string, value Value, allowDefaultInteger bool) (Value, error) {
	if allowDefaultInteger && value.kind == kindUint && value.ub == nil && value.bits == 64 {
		switch name {
		case "number":
			return numberValue(float64(value.u)), nil
		case "u8":
			if value.u <= 0xff {
				return uintValue(value.u, 8), nil
			}
		case "u16":
			if value.u <= 0xffff {
				return uintValue(value.u, 16), nil
			}
		case "u32":
			if value.u <= 0xffffffff {
				return uintValue(value.u, 32), nil
			}
		case "u64":
			return value, nil
		case "i8", "i16", "i32", "i64":
			bits := map[string]uint16{"i8": 8, "i16": 16, "i32": 32, "i64": 64}[name]
			limit := uint64(1) << (bits - 1)
			if bits == 64 {
				limit = uint64(1) << 63
			}
			if value.u < limit {
				return intValue(new(big.Int).SetUint64(value.u), bits), nil
			}
		}
	}
	if bindingTypeMatches(name, value) {
		return value, nil
	}
	return nullValue(), fmt.Errorf("binding type %s requires %s, got %s", name, name, value.Kind())
}

func bindingTypeMatches(name string, value Value) bool {
	switch name {
	case "number":
		return value.kind == kindNumber
	case "u8", "u16", "u32", "u64":
		if value.kind != kindUint || value.ub != nil || value.packed {
			return false
		}
		return value.bits == map[string]uint8{"u8": 8, "u16": 16, "u32": 32, "u64": 64}[name]
	case "i8", "i16", "i32", "i64":
		return value.kind == kindInt && value.ibits == map[string]uint16{"i8": 8, "i16": 16, "i32": 32, "i64": 64}[name]
	case "f32":
		return value.kind == kindNumber && !value.bfloat && value.fbits == 32
	case "f64":
		return value.kind == kindNumber && !value.bfloat && value.fbits == 64
	case "string":
		return value.kind == kindString
	case "bool":
		return value.kind == kindBool
	case "null":
		return value.kind == kindNull
	case "list":
		return value.kind == kindList
	case "tensor":
		return value.kind == kindTensor
	case "enum":
		return value.kind == kindEnum
	case "function":
		return value.kind == kindFunction
	case "duration":
		return value.kind == kindDuration
	case "datetime":
		return value.kind == kindDateTime
	case "location":
		return value.kind == kindTimeLocation
	default:
		return false
	}
}
