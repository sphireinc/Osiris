package osiris

import (
	"fmt"
	"math/big"
)

type intervalValue struct {
	lo *big.Rat
	hi *big.Rat
}

func newIntervalValue(lo, hi *big.Rat) *intervalValue {
	if lo == nil {
		lo = new(big.Rat)
	}
	if hi == nil {
		hi = new(big.Rat)
	}
	if lo.Cmp(hi) > 0 {
		lo, hi = hi, lo
	}
	return &intervalValue{
		lo: new(big.Rat).Set(lo),
		hi: new(big.Rat).Set(hi),
	}
}

func intervalFromValue(v Value) (*intervalValue, error) {
	switch v.kind {
	case kindInterval:
		if v.iv == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		return newIntervalValue(v.iv.lo, v.iv.hi), nil
	case kindDecimal:
		if v.dec == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		return newIntervalValue(v.dec, v.dec), nil
	case kindFixed:
		if v.fx == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		r := fixedToRat(v.fx)
		return newIntervalValue(r, r), nil
	case kindNumber:
		r := new(big.Rat)
		r.SetFloat64(v.num)
		return newIntervalValue(r, r), nil
	case kindBigInt:
		if v.bi == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		r := new(big.Rat).SetInt(v.bi)
		return newIntervalValue(r, r), nil
	case kindBigFloat:
		if v.bf == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		r, _ := new(big.Float).Copy(v.bf).Rat(nil)
		return newIntervalValue(r, r), nil
	case kindInt:
		if v.i == nil {
			return newIntervalValue(new(big.Rat), new(big.Rat)), nil
		}
		r := new(big.Rat).SetInt(v.i)
		return newIntervalValue(r, r), nil
	case kindUint:
		if v.ub != nil {
			r := new(big.Rat).SetInt(new(big.Int).Set(v.ub))
			return newIntervalValue(r, r), nil
		}
		r := new(big.Rat).SetInt(new(big.Int).SetUint64(v.u))
		return newIntervalValue(r, r), nil
	case kindBool:
		if v.boo {
			return newIntervalValue(new(big.Rat).SetInt64(1), new(big.Rat).SetInt64(1)), nil
		}
		return newIntervalValue(new(big.Rat), new(big.Rat)), nil
	case kindQuantity:
		if v.qty == nil || len(v.qty.dims) != 0 {
			return nil, fmt.Errorf("expected scalar numeric value, got %s", v.kind)
		}
		return newIntervalValue(v.qty.mag, v.qty.mag), nil
	default:
		return nil, fmt.Errorf("expected numeric value, got %s", v.kind)
	}
}

func intervalValueFromArgs(args []Value) (Value, error) {
	switch len(args) {
	case 1:
		iv, err := intervalFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		return intervalValueOf(iv), nil
	case 2:
		lo, err := intervalBoundFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		hi, err := intervalBoundFromValue(args[1])
		if err != nil {
			return nullValue(), err
		}
		return intervalValueOf(newIntervalValue(lo, hi)), nil
	default:
		return nullValue(), fmt.Errorf("interval expects 1 or 2 arguments")
	}
}

func intervalBoundFromValue(v Value) (*big.Rat, error) {
	switch v.kind {
	case kindInterval:
		if v.iv == nil {
			return new(big.Rat), nil
		}
		if v.iv.lo.Cmp(v.iv.hi) != 0 {
			return nil, fmt.Errorf("expected scalar bound, got %s", v.kind)
		}
		return new(big.Rat).Set(v.iv.lo), nil
	case kindDecimal:
		if v.dec == nil {
			return new(big.Rat), nil
		}
		return new(big.Rat).Set(v.dec), nil
	case kindFixed:
		if v.fx == nil {
			return new(big.Rat), nil
		}
		return fixedToRat(v.fx), nil
	case kindNumber:
		r := new(big.Rat)
		r.SetFloat64(v.num)
		return r, nil
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
	default:
		return nil, fmt.Errorf("expected scalar numeric bound, got %s", v.kind)
	}
}

func intervalBinary(op tokenKind, left, right Value) (Value, error) {
	l, err := intervalFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	r, err := intervalFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		return intervalValueOf(newIntervalValue(new(big.Rat).Add(l.lo, r.lo), new(big.Rat).Add(l.hi, r.hi))), nil
	case tokenMinus:
		return intervalValueOf(newIntervalValue(new(big.Rat).Sub(l.lo, r.hi), new(big.Rat).Sub(l.hi, r.lo))), nil
	case tokenStar:
		products := []*big.Rat{
			new(big.Rat).Mul(l.lo, r.lo),
			new(big.Rat).Mul(l.lo, r.hi),
			new(big.Rat).Mul(l.hi, r.lo),
			new(big.Rat).Mul(l.hi, r.hi),
		}
		lo, hi := intervalMinMax(products)
		return intervalValueOf(newIntervalValue(lo, hi)), nil
	case tokenSlash:
		if r.lo.Sign() <= 0 && r.hi.Sign() >= 0 {
			return nullValue(), fmt.Errorf("division by interval containing zero")
		}
		recip := []*big.Rat{
			new(big.Rat).Quo(new(big.Rat).SetInt64(1), r.lo),
			new(big.Rat).Quo(new(big.Rat).SetInt64(1), r.hi),
		}
		loR, hiR := intervalMinMax(recip)
		return intervalBinary(tokenStar, intervalValueOf(l), intervalValueOf(newIntervalValue(loR, hiR)))
	case tokenEqualEqual:
		return boolValue(l.lo.Cmp(r.lo) == 0 && l.hi.Cmp(r.hi) == 0), nil
	case tokenBangEqual:
		return boolValue(!(l.lo.Cmp(r.lo) == 0 && l.hi.Cmp(r.hi) == 0)), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		cmp, err := intervalCompare(left, right)
		if err != nil {
			return nullValue(), err
		}
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
	return nullValue(), fmt.Errorf("unsupported interval operator %s", op)
}

func intervalMinMax(values []*big.Rat) (*big.Rat, *big.Rat) {
	if len(values) == 0 {
		return new(big.Rat), new(big.Rat)
	}
	lo := new(big.Rat).Set(values[0])
	hi := new(big.Rat).Set(values[0])
	for _, v := range values[1:] {
		if v.Cmp(lo) < 0 {
			lo = new(big.Rat).Set(v)
		}
		if v.Cmp(hi) > 0 {
			hi = new(big.Rat).Set(v)
		}
	}
	return lo, hi
}

func intervalCompare(left, right Value) (int, error) {
	l, err := intervalFromValue(left)
	if err != nil {
		return 0, err
	}
	r, err := intervalFromValue(right)
	if err != nil {
		return 0, err
	}
	if l.hi.Cmp(r.lo) < 0 {
		return -1, nil
	}
	if l.lo.Cmp(r.hi) > 0 {
		return 1, nil
	}
	if l.lo.Cmp(r.lo) == 0 && l.hi.Cmp(r.hi) == 0 {
		return 0, nil
	}
	return 0, fmt.Errorf("ordering is undefined for overlapping intervals")
}

func formatInterval(iv *intervalValue) string {
	if iv == nil {
		return "interval[0, 0]"
	}
	return "interval[" + iv.lo.RatString() + ", " + iv.hi.RatString() + "]"
}
