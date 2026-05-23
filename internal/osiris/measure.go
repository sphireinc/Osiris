package osiris

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strings"
)

type quantityValue struct {
	mag  *big.Rat
	dims map[string]int
}

// newQuantityValue performs the new quantity value step in the surrounding package and returns *quantityValue.. It returns *quantityValue so callers can continue with the next stage of the pipeline without re-deriving the same information.
func newQuantityValue(mag *big.Rat, dims map[string]int) *quantityValue {
	if mag == nil {
		mag = new(big.Rat)
	}
	return &quantityValue{mag: new(big.Rat).Set(mag), dims: cloneDims(dims)}
}

// cloneDims performs the clone dims step in the surrounding package and returns map[string]int.. It returns map[string]int so callers can continue with the next stage of the pipeline without re-deriving the same information.
func cloneDims(dims map[string]int) map[string]int {
	if len(dims) == 0 {
		return nil
	}
	out := make(map[string]int, len(dims))
	for k, v := range dims {
		if v != 0 {
			out[k] = v
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// dimsEqual performs the dims equal step in the surrounding package and returns bool.. It returns bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func dimsEqual(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// combineDims performs the combine dims step in the surrounding package and returns map[string]int.. It returns map[string]int so callers can continue with the next stage of the pipeline without re-deriving the same information.
func combineDims(left, right map[string]int, sign int) map[string]int {
	out := cloneDims(left)
	if out == nil && len(right) == 0 {
		return nil
	}
	if out == nil {
		out = make(map[string]int, len(right))
	}
	for k, v := range right {
		out[k] += sign * v
		if out[k] == 0 {
			delete(out, k)
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// formatRat performs the format rat step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatRat(r *big.Rat) string {
	if r == nil {
		return "0"
	}
	if r.IsInt() {
		return r.Num().String()
	}
	return r.RatString()
}

// formatBigFloat performs the format big float step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatBigFloat(f *big.Float) string {
	if f == nil {
		return "0bf"
	}
	return f.Text('g', -1) + "bf"
}

// formatDims performs the format dims step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatDims(dims map[string]int) string {
	if len(dims) == 0 {
		return ""
	}
	num := make([]string, 0, len(dims))
	den := make([]string, 0, len(dims))
	for name, exp := range dims {
		if exp == 0 {
			continue
		}
		part := name
		abs := exp
		if abs < 0 {
			abs = -abs
		}
		if abs != 1 {
			part += "^" + fmt.Sprintf("%d", abs)
		}
		if exp > 0 {
			num = append(num, part)
		} else {
			den = append(den, part)
		}
	}
	sort.Strings(num)
	sort.Strings(den)
	out := strings.Join(num, "_Times_")
	if len(den) > 0 {
		if out == "" {
			out = "1"
		}
		out += "_Per_" + strings.Join(den, "_Times_")
	}
	return out
}

// formatQuantity performs the format quantity step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func formatQuantity(q *quantityValue) string {
	if q == nil || q.mag == nil {
		return "0"
	}
	dims := formatDims(q.dims)
	if dims == "" {
		return formatRat(q.mag)
	}
	if q.mag.Cmp(big.NewRat(1, 1)) == 0 {
		return dims
	}
	if q.mag.Cmp(big.NewRat(-1, 1)) == 0 {
		return "-" + dims
	}
	return formatRat(q.mag) + "*" + dims
}

// unitValue performs the unit value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func unitValue(name string) Value {
	return Value{kind: kindQuantity, qty: newQuantityValue(big.NewRat(1, 1), map[string]int{name: 1})}
}

// quantityValueOf performs the quantity value of step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func quantityValueOf(mag *big.Rat, dims map[string]int) Value {
	return Value{kind: kindQuantity, qty: newQuantityValue(mag, dims)}
}

// bigIntValue performs the big int value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func bigIntValue(v *big.Int) Value {
	if v == nil {
		v = new(big.Int)
	}
	return Value{kind: kindBigInt, bi: new(big.Int).Set(v)}
}

// bigFloatValue performs the big float value step in the surrounding package and returns Value.. It returns Value so callers can continue with the next stage of the pipeline without re-deriving the same information.
func bigFloatValue(v *big.Float) Value {
	if v == nil {
		v = new(big.Float).SetPrec(256)
	}
	return Value{kind: kindBigFloat, bf: new(big.Float).Copy(v)}
}

// isNumericish performs the is numericish step in the surrounding package and returns bool.. It returns bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func isNumericish(v Value) bool {
	switch v.kind {
	case kindNumber, kindDecimal, kindFixed, kindInterval, kindModInt, kindComplex, kindUint, kindBool, kindBigInt, kindBigFloat, kindInt, kindQuantity:
		return true
	default:
		return false
	}
}

// quantityFromValue performs the quantity from value step in the surrounding package and returns *quantityValue, error.. It returns *quantityValue and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func quantityFromValue(v Value) (*quantityValue, error) {
	switch v.kind {
	case kindQuantity:
		if v.qty == nil {
			return newQuantityValue(new(big.Rat), nil), nil
		}
		return newQuantityValue(v.qty.mag, v.qty.dims), nil
	case kindBigInt:
		return newQuantityValue(new(big.Rat).SetInt(v.bi), nil), nil
	case kindBigFloat:
		r, _ := v.bf.Rat(nil)
		return newQuantityValue(r, nil), nil
	case kindInt:
		return newQuantityValue(new(big.Rat).SetInt(v.i), nil), nil
	case kindDecimal:
		if v.dec == nil {
			return newQuantityValue(new(big.Rat), nil), nil
		}
		return newQuantityValue(v.dec, nil), nil
	case kindFixed:
		if v.fx == nil {
			return newQuantityValue(new(big.Rat), nil), nil
		}
		return newQuantityValue(fixedToRat(v.fx), nil), nil
	case kindModInt:
		if v.mi == nil {
			return newQuantityValue(new(big.Rat), nil), nil
		}
		return newQuantityValue(new(big.Rat).SetInt(v.mi.val), nil), nil
	case kindNumber:
		r := new(big.Rat)
		r.SetFloat64(v.num)
		return newQuantityValue(r, nil), nil
	case kindUint:
		if v.ub != nil {
			return newQuantityValue(new(big.Rat).SetInt(new(big.Int).Set(v.ub)), nil), nil
		}
		return newQuantityValue(new(big.Rat).SetInt(new(big.Int).SetUint64(v.u)), nil), nil
	case kindBool:
		if v.boo {
			return newQuantityValue(new(big.Rat).SetInt64(1), nil), nil
		}
		return newQuantityValue(new(big.Rat), nil), nil
	default:
		return nil, fmt.Errorf("expected numeric value, got %s", v.kind)
	}
}

// bigFloatFromValue performs the big float from value step in the surrounding package and returns *big.Float, error.. It returns *big.Float and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func bigFloatFromValue(v Value) (*big.Float, error) {
	switch v.kind {
	case kindBigFloat:
		if v.bf == nil {
			return new(big.Float).SetPrec(256), nil
		}
		return new(big.Float).Copy(v.bf), nil
	case kindInt:
		if v.i == nil {
			return new(big.Float).SetPrec(256), nil
		}
		return new(big.Float).SetInt(v.i), nil
	case kindBigInt:
		if v.bi == nil {
			return new(big.Float).SetPrec(256), nil
		}
		return new(big.Float).SetInt(v.bi), nil
	case kindQuantity:
		if v.qty == nil {
			return new(big.Float).SetPrec(256), nil
		}
		if len(v.qty.dims) != 0 {
			return nil, fmt.Errorf("expected dimensionless value, got %s", formatQuantity(v.qty))
		}
		r := new(big.Float).SetPrec(256)
		r.SetRat(v.qty.mag)
		return r, nil
	case kindDecimal:
		if v.dec == nil {
			return new(big.Float).SetPrec(256), nil
		}
		r := new(big.Float).SetPrec(256)
		r.SetRat(v.dec)
		return r, nil
	case kindFixed:
		if v.fx == nil {
			return new(big.Float).SetPrec(256), nil
		}
		r := new(big.Float).SetPrec(256)
		r.SetRat(fixedToRat(v.fx))
		return r, nil
	case kindModInt:
		if v.mi == nil {
			return new(big.Float).SetPrec(256), nil
		}
		return new(big.Float).SetInt(v.mi.val), nil
	case kindInterval:
		if v.iv == nil {
			return new(big.Float).SetPrec(256), nil
		}
		if v.iv.lo.Cmp(v.iv.hi) != 0 {
			return nil, fmt.Errorf("expected scalar numeric value, got %s", v.kind)
		}
		r := new(big.Float).SetPrec(256)
		r.SetRat(v.iv.lo)
		return r, nil
	case kindNumber:
		return new(big.Float).SetPrec(256).SetFloat64(v.num), nil
	case kindUint:
		if v.ub != nil {
			return new(big.Float).SetPrec(256).SetInt(v.ub), nil
		}
		return new(big.Float).SetPrec(256).SetUint64(v.u), nil
	case kindBool:
		if v.boo {
			return new(big.Float).SetPrec(256).SetInt64(1), nil
		}
		return new(big.Float).SetPrec(256), nil
	default:
		return nil, fmt.Errorf("expected numeric value, got %s", v.kind)
	}
}

// bigIntFromValue performs the big int from value step in the surrounding package and returns *big.Int, error.. It returns *big.Int and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func bigIntFromValue(v Value) (*big.Int, error) {
	switch v.kind {
	case kindBigInt:
		if v.bi == nil {
			return big.NewInt(0), nil
		}
		return new(big.Int).Set(v.bi), nil
	case kindBigFloat:
		if v.bf == nil {
			return big.NewInt(0), nil
		}
		i, acc := new(big.Float).Copy(v.bf).Int(nil)
		if acc != big.Exact {
			return nil, fmt.Errorf("expected integer, got %s", formatBigFloat(v.bf))
		}
		return i, nil
	case kindInt:
		if v.i == nil {
			return big.NewInt(0), nil
		}
		return new(big.Int).Set(v.i), nil
	case kindQuantity:
		if v.qty == nil {
			return big.NewInt(0), nil
		}
		if len(v.qty.dims) != 0 {
			return nil, fmt.Errorf("expected dimensionless integer, got %s", formatQuantity(v.qty))
		}
		if !v.qty.mag.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatQuantity(v.qty))
		}
		return new(big.Int).Set(v.qty.mag.Num()), nil
	case kindDecimal:
		if v.dec == nil || !v.dec.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", formatDecimal(v.dec))
		}
		return new(big.Int).Set(v.dec.Num()), nil
	case kindFixed:
		i, _, _, err := fixedIntFromValue(v)
		if err != nil {
			return nil, err
		}
		return i, nil
	case kindModInt:
		if v.mi == nil {
			return big.NewInt(0), nil
		}
		return new(big.Int).Set(v.mi.val), nil
	case kindInterval:
		if v.iv == nil {
			return big.NewInt(0), nil
		}
		if v.iv.lo.Cmp(v.iv.hi) != 0 || !v.iv.lo.IsInt() {
			return nil, fmt.Errorf("expected integer, got %s", v.kind)
		}
		return new(big.Int).Set(v.iv.lo.Num()), nil
	case kindNumber:
		if math.Trunc(v.num) != v.num {
			return nil, fmt.Errorf("expected integer, got %s", formatNumber(v.num))
		}
		return big.NewInt(int64(v.num)), nil
	case kindUint:
		if v.ub != nil {
			return new(big.Int).Set(v.ub), nil
		}
		return new(big.Int).SetUint64(v.u), nil
	case kindBool:
		if v.boo {
			return big.NewInt(1), nil
		}
		return big.NewInt(0), nil
	default:
		return nil, fmt.Errorf("expected integer, got %s", v.kind)
	}
}

// quantityBinary performs the quantity binary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func quantityBinary(op tokenKind, left, right Value) (Value, error) {
	lq, err := quantityFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	rq, err := quantityFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	switch op {
	case tokenPlus:
		if !dimsEqual(lq.dims, rq.dims) {
			return nullValue(), fmt.Errorf("cannot add %s and %s", formatQuantity(lq), formatQuantity(rq))
		}
		return quantityValueOf(new(big.Rat).Add(lq.mag, rq.mag), lq.dims), nil
	case tokenMinus:
		if !dimsEqual(lq.dims, rq.dims) {
			return nullValue(), fmt.Errorf("cannot subtract %s and %s", formatQuantity(lq), formatQuantity(rq))
		}
		return quantityValueOf(new(big.Rat).Sub(lq.mag, rq.mag), lq.dims), nil
	case tokenStar:
		return quantityValueOf(new(big.Rat).Mul(lq.mag, rq.mag), combineDims(lq.dims, rq.dims, 1)), nil
	case tokenSlash:
		if rq.mag.Sign() == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		return quantityValueOf(new(big.Rat).Quo(lq.mag, rq.mag), combineDims(lq.dims, rq.dims, -1)), nil
	case tokenPercent:
		if len(lq.dims) != 0 || len(rq.dims) != 0 {
			return nullValue(), fmt.Errorf("modulo is only defined for dimensionless values")
		}
		if rq.mag.Sign() == 0 {
			return nullValue(), fmt.Errorf("modulo by zero")
		}
		q := new(big.Rat).Quo(lq.mag, rq.mag)
		if !q.IsInt() {
			return nullValue(), fmt.Errorf("modulo is only defined for integer values")
		}
		return quantityValueOf(new(big.Rat).Sub(lq.mag, new(big.Rat).Mul(rq.mag, q)), nil), nil
	case tokenEqualEqual:
		return boolValue(dimsEqual(lq.dims, rq.dims) && lq.mag.Cmp(rq.mag) == 0), nil
	case tokenBangEqual:
		return boolValue(!(dimsEqual(lq.dims, rq.dims) && lq.mag.Cmp(rq.mag) == 0)), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		if !dimsEqual(lq.dims, rq.dims) {
			return nullValue(), fmt.Errorf("ordering is only defined for values with the same dimensions")
		}
		cmp := lq.mag.Cmp(rq.mag)
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
	return nullValue(), fmt.Errorf("unsupported quantity operator %s", op)
}

// bigBinary performs the big binary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func bigBinary(op tokenKind, left, right Value) (Value, error) {
	li, lerr := bigIntFromValue(left)
	ri, rerr := bigIntFromValue(right)
	if lerr == nil && rerr == nil {
		if left.kind == kindInt && right.kind == kindInt {
			_, lw, _ := signedIntFromValue(left)
			_, rw, _ := signedIntFromValue(right)
			width := lw
			if rw > width {
				width = rw
			}
			if width == 0 {
				width = 64
			}
			switch op {
			case tokenPlus:
				return intValue(new(big.Int).Add(li, ri), width), nil
			case tokenMinus:
				return intValue(new(big.Int).Sub(li, ri), width), nil
			case tokenStar:
				return intValue(new(big.Int).Mul(li, ri), width), nil
			case tokenSlash:
				return exactSignedDivisionValue(left, right)
			case tokenPercent:
				if ri.Sign() == 0 {
					return nullValue(), fmt.Errorf("modulo by zero")
				}
				return intValue(new(big.Int).Rem(li, ri), width), nil
			case tokenEqualEqual:
				return boolValue(li.Cmp(ri) == 0), nil
			case tokenBangEqual:
				return boolValue(li.Cmp(ri) != 0), nil
			case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
				cmp := li.Cmp(ri)
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
		}
		switch op {
		case tokenPlus:
			return bigIntValue(new(big.Int).Add(li, ri)), nil
		case tokenMinus:
			return bigIntValue(new(big.Int).Sub(li, ri)), nil
		case tokenStar:
			return bigIntValue(new(big.Int).Mul(li, ri)), nil
		case tokenSlash:
			if ri.Sign() == 0 {
				return nullValue(), fmt.Errorf("division by zero")
			}
			q, r := new(big.Int).QuoRem(li, ri, new(big.Int))
			if r.Sign() == 0 {
				if left.kind == kindInt || right.kind == kindInt {
					return exactSignedDivisionValue(left, right)
				}
				return bigIntValue(q), nil
			}
			return decimalValue(new(big.Rat).SetFrac(li, ri)), nil
		case tokenPercent:
			if ri.Sign() == 0 {
				return nullValue(), fmt.Errorf("modulo by zero")
			}
			if left.kind == kindInt || right.kind == kindInt {
				_, lw, _ := signedIntFromValue(left)
				_, rw, _ := signedIntFromValue(right)
				width := lw
				if rw > width {
					width = rw
				}
				if width == 0 {
					width = 64
				}
				return intValue(new(big.Int).Rem(li, ri), width), nil
			}
			return bigIntValue(new(big.Int).Mod(li, ri)), nil
		case tokenEqualEqual:
			return boolValue(li.Cmp(ri) == 0), nil
		case tokenBangEqual:
			return boolValue(li.Cmp(ri) != 0), nil
		case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
			cmp := li.Cmp(ri)
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
	}
	lf, err := bigFloatFromValue(left)
	if err != nil {
		return nullValue(), err
	}
	rf, err := bigFloatFromValue(right)
	if err != nil {
		return nullValue(), err
	}
	res := new(big.Float).SetPrec(256)
	switch op {
	case tokenPlus:
		res.Add(lf, rf)
	case tokenMinus:
		res.Sub(lf, rf)
	case tokenStar:
		res.Mul(lf, rf)
	case tokenSlash:
		if rf.Sign() == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		res.Quo(lf, rf)
	case tokenEqualEqual:
		return boolValue(lf.Cmp(rf) == 0), nil
	case tokenBangEqual:
		return boolValue(lf.Cmp(rf) != 0), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		cmp := lf.Cmp(rf)
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
	return bigFloatValue(res), nil
}
