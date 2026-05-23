package osiris

import (
	"fmt"
	"math"
	"math/big"
	"strings"
)

type fixedValue struct {
	width uint16
	frac  uint16
	raw   *big.Int
}

// newFixedValue performs the new fixed value step in the surrounding package and returns *fixedValue, error.. It returns *fixedValue and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func newFixedValue(width, frac uint16, raw *big.Int) (*fixedValue, error) {
	if width == 0 {
		width = 64
	}
	if frac > width {
		return nil, fmt.Errorf("fixed fractional bits cannot exceed total width")
	}
	if raw == nil {
		raw = new(big.Int)
	}
	return &fixedValue{
		width: width,
		frac:  frac,
		raw:   wrapSignedWidth(raw, width),
	}, nil
}

// fixedValueOf performs the fixed value of step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func fixedValueOf(width, frac uint16, raw *big.Int) (Value, error) {
	fx, err := newFixedValue(width, frac, raw)
	if err != nil {
		return nullValue(), err
	}
	return Value{kind: kindFixed, fx: fx}, nil
}

// fixedWidthOf performs the fixed width of step in the surrounding package and returns uint16.. It returns uint16 so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedWidthOf(v Value) uint16 {
	if v.kind != kindFixed || v.fx == nil || v.fx.width == 0 {
		return 64
	}
	return v.fx.width
}

// fixedFracOf performs the fixed frac of step in the surrounding package and returns uint16.. It returns uint16 so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedFracOf(v Value) uint16 {
	if v.kind != kindFixed || v.fx == nil {
		return 0
	}
	return v.fx.frac
}

// fixedScale performs the fixed scale step in the surrounding package and returns *big.Int.. It returns *big.Int so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedScale(frac uint16) *big.Int {
	if frac == 0 {
		return big.NewInt(1)
	}
	return new(big.Int).Lsh(big.NewInt(1), uint(frac))
}

// fixedToRat performs the fixed to rat step in the surrounding package and returns *big.Rat.. It returns *big.Rat so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedToRat(v *fixedValue) *big.Rat {
	if v == nil {
		return new(big.Rat)
	}
	if v.frac == 0 {
		return new(big.Rat).SetInt(v.raw)
	}
	return new(big.Rat).SetFrac(new(big.Int).Set(v.raw), fixedScale(v.frac))
}

// fixedString performs the fixed string step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedString(v *fixedValue) string {
	if v == nil {
		return "fixed[64,0](0)"
	}
	return fmt.Sprintf("fixed[%d,%d](%s)", v.width, v.frac, formatRat(fixedToRat(v)))
}

// fixedRatFromValue performs the fixed rat from value step in the surrounding package and returns *big.Rat, error.. It returns *big.Rat and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func fixedRatFromValue(v Value) (*big.Rat, error) {
	switch v.kind {
	case kindFixed:
		if v.fx == nil {
			return new(big.Rat), nil
		}
		return fixedToRat(v.fx), nil
	case kindDecimal:
		if v.dec == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).Set(v.dec), nil
	case kindModInt:
		if v.mi == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).SetInt(v.mi.val), nil
	case kindInterval:
		if v.iv == nil {
			return new(big.Rat), nil
		}
		if v.iv.lo.Cmp(v.iv.hi) != 0 {
			return nil, fmt.Errorf("expected scalar fixed-point value, got %s", v.kind)
		}
		return new(big.Rat).Set(v.iv.lo), nil
	case kindBigInt:
		if v.bi == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).SetInt(v.bi), nil
	case kindBigFloat:
		if v.bf == nil {
			return new(big.Rat), nil
		}
		r, _ := new(big.Float).Copy(v.bf).Rat(nil)
		return r, nil
	case kindInt:
		if v.i == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).SetInt(v.i), nil
	case kindUint:
		if v.ub != nil {
			return new(big.Rat).SetInt(new(big.Int).Set(v.ub)), nil
		}
		return new(big.Rat).SetInt(new(big.Int).SetUint64(v.u)), nil
	case kindBool:
		if v.boo {
			return new(big.Rat).SetInt64(1), nil
		}
		return new(big.Rat), nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 {
			return nil, fmt.Errorf("expected scalar numeric value, got %s", v.kind)
		}
		return new(big.Rat).Set(v.qty.mag), nil
	case kindNumber:
		if math.IsNaN(v.num) || math.IsInf(v.num, 0) {
			return nil, fmt.Errorf("cannot convert %s to fixed-point", formatNumber(v.num))
		}
		r := new(big.Rat)
		r.SetFloat64(v.num)
		return r, nil
	case kindString:
		r, err := parseDecimalText(strings.TrimSpace(v.str))
		if err != nil {
			return nil, err
		}
		return r, nil
	default:
		return nil, fmt.Errorf("expected numeric value, got %s", v.kind)
	}
}

// fixedRawFromRat performs the fixed raw from rat step in the surrounding package and returns *big.Int.. It returns *big.Int so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedRawFromRat(r *big.Rat, frac uint16) *big.Int {
	if r == nil {
		return new(big.Int)
	}
	scaled := new(big.Rat).Mul(r, new(big.Rat).SetInt(fixedScale(frac)))
	if scaled.IsInt() {
		return new(big.Int).Set(scaled.Num())
	}
	num := new(big.Int).Set(scaled.Num())
	den := new(big.Int).Set(scaled.Denom())
	q, rem := new(big.Int).QuoRem(num, den, new(big.Int))
	if rem.Sign() == 0 {
		return q
	}
	absRem := new(big.Int).Abs(rem)
	twiceRem := new(big.Int).Lsh(absRem, 1)
	if twiceRem.Cmp(den) >= 0 {
		if num.Sign() >= 0 {
			q.Add(q, big.NewInt(1))
		} else {
			q.Sub(q, big.NewInt(1))
		}
	}
	return q
}

// fixedIntFromValue performs the fixed int from value step in the surrounding package and returns *big.Int, uint16, uint16, error.. It returns *big.Int, uint16, uint16, error so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fixedIntFromValue(v Value) (*big.Int, uint16, uint16, error) {
	switch v.kind {
	case kindFixed:
		if v.fx == nil {
			return big.NewInt(0), 64, 0, nil
		}
		scale := fixedScale(v.fx.frac)
		q, r := new(big.Int).QuoRem(new(big.Int).Set(v.fx.raw), scale, new(big.Int))
		if r.Sign() != 0 {
			return nil, 0, 0, fmt.Errorf("expected integer, got %s", fixedString(v.fx))
		}
		return q, v.fx.width, v.fx.frac, nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return nil, 0, 0, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		return new(big.Int).Set(v.dec.Num()), 0, 0, nil
	case kindModInt:
		if v.mi == nil {
			return big.NewInt(0), 0, 0, nil
		}
		return new(big.Int).Set(v.mi.val), 0, 0, nil
	case kindInterval:
		if v.iv == nil || v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() {
			return nil, 0, 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.iv.lo.Num()), 0, 0, nil
	case kindBigInt:
		if v.bi == nil {
			return big.NewInt(0), 0, 0, nil
		}
		return new(big.Int).Set(v.bi), 0, 0, nil
	case kindBigFloat:
		if v.bf == nil {
			return big.NewInt(0), 0, 0, nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact {
			return nil, 0, 0, fmt.Errorf("expected integer, got %s", formatBigFloat(v.bf))
		}
		return i, 0, 0, nil
	case kindInt:
		if v.i == nil {
			return big.NewInt(0), 0, 0, nil
		}
		return new(big.Int).Set(v.i), v.ibits, 0, nil
	case kindUint:
		if v.ub != nil {
			return new(big.Int).Set(v.ub), v.ubitsToUint16(), 0, nil
		}
		return new(big.Int).SetUint64(v.u), uint16(v.bits), 0, nil
	case kindBool:
		if v.boo {
			return big.NewInt(1), 0, 0, nil
		}
		return big.NewInt(0), 0, 0, nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 || !v.qty.mag.IsInt() {
			return nil, 0, 0, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.qty.mag.Num()), 0, 0, nil
	default:
		return nil, 0, 0, fmt.Errorf("expected integer, got %s", v.kind)
	}
}

// fixedValueFromArgs performs the fixed value from args step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func fixedValueFromArgs(args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nullValue(), fmt.Errorf("fixed expects 2 or 3 arguments")
	}
	widthI, _, _, err := fixedIntFromValue(args[0])
	if err != nil {
		return nullValue(), fmt.Errorf("fixed width: %w", err)
	}
	fracI, _, _, err := fixedIntFromValue(args[1])
	if err != nil {
		return nullValue(), fmt.Errorf("fixed fractional bits: %w", err)
	}
	if widthI.Sign() < 0 || fracI.Sign() < 0 {
		return nullValue(), fmt.Errorf("fixed width and fractional bits must be non-negative")
	}
	if !widthI.IsUint64() || !fracI.IsUint64() {
		return nullValue(), fmt.Errorf("fixed width or fractional bits too large")
	}
	width := uint16(widthI.Uint64())
	frac := uint16(fracI.Uint64())
	if width == 0 {
		width = 64
	}
	if frac > width {
		return nullValue(), fmt.Errorf("fixed fractional bits cannot exceed total width")
	}
	if len(args) == 2 {
		return fixedValueOf(width, frac, new(big.Int))
	}
	rat, err := fixedRatFromValue(args[2])
	if err != nil {
		return nullValue(), err
	}
	return fixedValueOf(width, frac, fixedRawFromRat(rat, frac))
}

// fixedBinary performs the fixed binary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func fixedBinary(op tokenKind, left, right Value) (Value, error) {
	if op == tokenPlus && (left.kind == kindString || right.kind == kindString) {
		return stringValue(textify(left) + textify(right)), nil
	}
	lr, err := fixedRatFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	rr, err := fixedRatFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	width := fixedWidthOf(left)
	if w := fixedWidthOf(right); w > width {
		width = w
	}
	frac := fixedFracOf(left)
	if f := fixedFracOf(right); f > frac {
		frac = f
	}
	if width == 0 {
		width = 64
	}
	switch op {
	case tokenPlus:
		out, err := fixedValueOf(width, frac, fixedRawFromRat(new(big.Rat).Add(lr, rr), frac))
		return out, err
	case tokenMinus:
		out, err := fixedValueOf(width, frac, fixedRawFromRat(new(big.Rat).Sub(lr, rr), frac))
		return out, err
	case tokenStar:
		out, err := fixedValueOf(width, frac, fixedRawFromRat(new(big.Rat).Mul(lr, rr), frac))
		return out, err
	case tokenSlash:
		if rr.Sign() == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		out, err := fixedValueOf(width, frac, fixedRawFromRat(new(big.Rat).Quo(lr, rr), frac))
		return out, err
	case tokenPercent:
		if rr.Sign() == 0 {
			return nullValue(), fmt.Errorf("modulo by zero")
		}
		num := new(big.Int).Mul(new(big.Int).Set(lr.Num()), new(big.Int).Set(rr.Denom()))
		den := new(big.Int).Mul(new(big.Int).Set(lr.Denom()), new(big.Int).Set(rr.Num()))
		if den.Sign() == 0 {
			return nullValue(), fmt.Errorf("modulo by zero")
		}
		q := new(big.Int).Quo(num, den)
		remainder := new(big.Rat).Sub(lr, new(big.Rat).Mul(rr, new(big.Rat).SetInt(q)))
		out, err := fixedValueOf(width, frac, fixedRawFromRat(remainder, frac))
		return out, err
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
	return nullValue(), fmt.Errorf("unsupported fixed operator %s", op)
}
