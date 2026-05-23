package osiris

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

type modIntValue struct {
	mod *big.Int
	val *big.Int
}

// newModIntValue performs the new mod int value step in the surrounding package and returns *modIntValue, error.. It returns *modIntValue and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func newModIntValue(modulus, value *big.Int) (*modIntValue, error) {
	if modulus == nil {
		return nil, fmt.Errorf("modint requires a modulus")
	}
	if modulus.Sign() <= 0 {
		return nil, fmt.Errorf("modulus must be positive")
	}
	if value == nil {
		value = new(big.Int)
	}
	mod := new(big.Int).Set(modulus)
	val := new(big.Int).Mod(new(big.Int).Set(value), mod)
	if val.Sign() < 0 {
		val.Add(val, mod)
	}
	return &modIntValue{mod: mod, val: val}, nil
}

// modIntValueOf performs the mod int value of step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func modIntValueOf(modulus, value *big.Int) (Value, error) {
	mv, err := newModIntValue(modulus, value)
	if err != nil {
		return nullValue(), err
	}
	return Value{kind: kindModInt, mi: mv}, nil
}

// modIntValueFromArgs performs the mod int value from args step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func modIntValueFromArgs(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("modint expects 2 arguments")
	}
	modulus, err := parseExactIntArg(args[0], "modint modulus")
	if err != nil {
		return nullValue(), err
	}
	value, err := parseExactIntArg(args[1], "modint value")
	if err != nil {
		return nullValue(), err
	}
	return modIntValueOf(modulus, value)
}

// parseExactIntArg performs the parse exact int arg step in the surrounding package and returns *big.Int, error.. It returns *big.Int and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func parseExactIntArg(v Value, label string) (*big.Int, error) {
	if v.kind == kindString {
		if i, ok := new(big.Int).SetString(strings.TrimSpace(v.str), 0); ok {
			return i, nil
		}
		return nil, fmt.Errorf("invalid %s", label)
	}
	i, err := bigIntFromValue(v)
	if err != nil {
		return nil, err
	}
	return i, nil
}

// modIntFromValue performs the mod int from value step in the surrounding package and returns *modIntValue, error.. It returns *modIntValue and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func modIntFromValue(v Value) (*modIntValue, error) {
	switch v.kind {
	case kindModInt:
		if v.mi == nil {
			return newModIntValue(big.NewInt(1), nil)
		}
		return newModIntValue(v.mi.mod, v.mi.val)
	case kindBigInt:
		return newModIntValue(big.NewInt(1), v.bi)
	case kindBigFloat:
		if v.bf == nil {
			return newModIntValue(big.NewInt(1), nil)
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact {
			return nil, fmt.Errorf("expected integer, got %s", formatBigFloat(v.bf))
		}
		return newModIntValue(big.NewInt(1), i)
	case kindInt:
		return newModIntValue(big.NewInt(1), v.i)
	case kindUint:
		if v.ub != nil {
			return newModIntValue(big.NewInt(1), v.ub)
		}
		return newModIntValue(big.NewInt(1), new(big.Int).SetUint64(v.u))
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		return newModIntValue(big.NewInt(1), v.dec.Num())
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return nil, err
		}
		return newModIntValue(big.NewInt(1), i)
	case kindNumber:
		if strings.Contains(formatNumber(v.num), ".") {
			return nil, fmt.Errorf("expected integer, got %s", formatNumber(v.num))
		}
		if v.num != float64(int64(v.num)) {
			return nil, fmt.Errorf("expected integer, got %s", formatNumber(v.num))
		}
		return newModIntValue(big.NewInt(1), big.NewInt(int64(v.num)))
	case kindBool:
		if v.boo {
			return newModIntValue(big.NewInt(1), big.NewInt(1))
		}
		return newModIntValue(big.NewInt(1), new(big.Int))
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatQuantity(v.qty))
		}
		return newModIntValue(big.NewInt(1), v.qty.mag.Num())
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return newModIntValue(big.NewInt(1), v.iv.lo.Num())
	default:
		return nil, fmt.Errorf("expected integer, got %s", v.kind)
	}
}

// modIntResidueFromValue performs the mod int residue from value step in the surrounding package and returns *big.Int, error.. It returns *big.Int and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func modIntResidueFromValue(v Value, mod *big.Int) (*big.Int, error) {
	switch v.kind {
	case kindModInt:
		if v.mi == nil {
			return new(big.Int), nil
		}
		if mod != nil && v.mi.mod.Cmp(mod) != 0 {
			return nil, fmt.Errorf("modint modulus mismatch: %s vs %s", v.mi.mod.String(), mod.String())
		}
		return new(big.Int).Set(v.mi.val), nil
	case kindBigInt:
		if v.bi == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Mod(new(big.Int).Set(v.bi), mod), nil
	case kindBigFloat:
		if v.bf == nil {
			return new(big.Int), nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact {
			return nil, fmt.Errorf("expected integer, got %s", formatBigFloat(v.bf))
		}
		return new(big.Int).Mod(i, mod), nil
	case kindInt:
		if v.i == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Mod(new(big.Int).Set(v.i), mod), nil
	case kindUint:
		if v.ub != nil {
			return new(big.Int).Mod(new(big.Int).Set(v.ub), mod), nil
		}
		return new(big.Int).Mod(new(big.Int).SetUint64(v.u), mod), nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		return new(big.Int).Mod(new(big.Int).Set(v.dec.Num()), mod), nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return nil, err
		}
		if i.Sign() < 0 {
			return nil, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Mod(i, mod), nil
	case kindNumber:
		if math.Trunc(v.num) != v.num {
			return nil, fmt.Errorf("expected integer, got %s", formatNumber(v.num))
		}
		return new(big.Int).Mod(big.NewInt(int64(v.num)), mod), nil
	case kindBool:
		if v.boo {
			return new(big.Int).Mod(big.NewInt(1), mod), nil
		}
		return new(big.Int), nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatQuantity(v.qty))
		}
		return new(big.Int).Mod(new(big.Int).Set(v.qty.mag.Num()), mod), nil
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Mod(new(big.Int).Set(v.iv.lo.Num()), mod), nil
	default:
		return nil, fmt.Errorf("expected integer, got %s", v.kind)
	}
}

// formatModInt performs the format mod int step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatModInt(v *modIntValue) string {
	if v == nil {
		return "0 mod 1"
	}
	return v.val.String() + " mod " + v.mod.String()
}

// modIntBinary performs the mod int binary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func modIntBinary(op tokenKind, left, right Value) (Value, error) {
	var mod *big.Int
	if left.kind == kindModInt && left.mi != nil {
		mod = left.mi.mod
	}
	if right.kind == kindModInt && right.mi != nil {
		if mod != nil && mod.Cmp(right.mi.mod) != 0 {
			return nullValue(), fmt.Errorf("modint modulus mismatch: %s vs %s", mod.String(), right.mi.mod.String())
		}
		mod = right.mi.mod
	}
	if mod == nil {
		return nullValue(), fmt.Errorf("modint requires at least one modular operand")
	}
	lr, err := modIntResidueFromValue(left, mod)
	if err != nil {
		return nullValue(), err
	}
	rr, err := modIntResidueFromValue(right, mod)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		return modIntValueOf(mod, new(big.Int).Add(lr, rr))
	case tokenMinus:
		return modIntValueOf(mod, new(big.Int).Sub(lr, rr))
	case tokenStar:
		return modIntValueOf(mod, new(big.Int).Mul(lr, rr))
	case tokenSlash:
		inv := new(big.Int).ModInverse(rr, mod)
		if inv == nil {
			return nullValue(), fmt.Errorf("modular inverse does not exist")
		}
		return modIntValueOf(mod, new(big.Int).Mul(lr, inv))
	case tokenPercent:
		return nullValue(), fmt.Errorf("modint modulo is not supported")
	case tokenEqualEqual:
		return boolValue(lr.Cmp(rr) == 0), nil
	case tokenBangEqual:
		return boolValue(lr.Cmp(rr) != 0), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		cmp := lr.Cmp(rr)
		switch op {
		case tokenLess:
			return boolValue(cmp < 0), nil
		case tokenLessEqual:
			return boolValue(cmp <= 0), nil
		case tokenGreater:
			return boolValue(cmp > 0), nil
		case tokenGreaterEqual:
			return boolValue(cmp >= 0), nil
		}
	}
	return nullValue(), fmt.Errorf("unsupported modint operator %s", op)
}
