package osiris

import (
	"fmt"
	"math"
	"math/big"
	"math/cmplx"
)

func mathFailure(kind ErrorKind, format string, args ...any) error {
	return newOsirisError(kind, fmt.Sprintf(format, args...))
}

func mathArity(name string, args []Value, want int) error {
	if len(args) == want {
		return nil
	}
	return mathFailure(ErrorKindInvalidArgument, "%s expects %d arguments, got %d", name, want, len(args))
}

func mathRealArgument(name string, args []Value, index int) (float64, error) {
	if index >= len(args) {
		return 0, mathFailure(ErrorKindInvalidArgument, "%s is missing argument %d", name, index+1)
	}
	if args[index].kind != kindNumber {
		return 0, mathFailure(ErrorKindType, "%s argument %d expects a floating-point number, got %s", name, index+1, args[index].Kind())
	}
	return args[index].num, nil
}

func mathUnaryReal(args []Value, name string, fn func(float64) float64) (Value, error) {
	if err := mathArity(name, args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument(name, args, 0)
	if err != nil {
		return nullValue(), err
	}
	return floatValueStyled(fn(value), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

func atan2Value(args []Value) (Value, error) {
	if err := mathArity("atan2", args, 2); err != nil {
		return nullValue(), err
	}
	y, err := mathRealArgument("atan2", args, 0)
	if err != nil {
		return nullValue(), err
	}
	x, err := mathRealArgument("atan2", args, 1)
	if err != nil {
		return nullValue(), err
	}
	return floatResultValue(math.Atan2(y, x), args...), nil
}

func exp2Value(args []Value) (Value, error) {
	if err := mathArity("exp2", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("exp2", args, 0)
	if err != nil {
		return nullValue(), err
	}
	result := math.Exp2(value)
	if !math.IsInf(value, 0) && math.IsInf(result, 0) {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "exp2 overflow")
	}
	out := floatValueStyled(result, floatUnaryBits(args[0]), floatUnaryBFloat(args[0]))
	if !math.IsInf(value, 0) && !math.IsNaN(value) && math.IsInf(out.num, 0) {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "exp2 overflow")
	}
	return out, nil
}

func truncValue(args []Value) (Value, error) {
	return mathUnaryReal(args, "trunc", math.Trunc)
}

func fmodValue(args []Value) (Value, error) {
	if err := mathArity("fmod", args, 2); err != nil {
		return nullValue(), err
	}
	dividend, err := mathRealArgument("fmod", args, 0)
	if err != nil {
		return nullValue(), err
	}
	divisor, err := mathRealArgument("fmod", args, 1)
	if err != nil {
		return nullValue(), err
	}
	if divisor == 0 {
		return nullValue(), mathFailure(ErrorKindInvalidArgument, "fmod divisor must be non-zero")
	}
	return floatResultValue(math.Mod(dividend, divisor), args...), nil
}

func remainderValue(args []Value) (Value, error) {
	if err := mathArity("remainder", args, 2); err != nil {
		return nullValue(), err
	}
	dividend, err := mathRealArgument("remainder", args, 0)
	if err != nil {
		return nullValue(), err
	}
	divisor, err := mathRealArgument("remainder", args, 1)
	if err != nil {
		return nullValue(), err
	}
	if divisor == 0 {
		return nullValue(), mathFailure(ErrorKindInvalidArgument, "remainder divisor must be non-zero")
	}
	return floatResultValue(math.Remainder(dividend, divisor), args...), nil
}

func fmaValue(args []Value) (Value, error) {
	if err := mathArity("fma", args, 3); err != nil {
		return nullValue(), err
	}
	values := make([]float64, 3)
	for index := range values {
		value, err := mathRealArgument("fma", args, index)
		if err != nil {
			return nullValue(), err
		}
		values[index] = value
	}
	return floatResultValue(math.FMA(values[0], values[1], values[2]), args...), nil
}

func positiveDifferenceValue(args []Value) (Value, error) {
	if err := mathArity("positive_difference", args, 2); err != nil {
		return nullValue(), err
	}
	left, err := mathRealArgument("positive_difference", args, 0)
	if err != nil {
		return nullValue(), err
	}
	right, err := mathRealArgument("positive_difference", args, 1)
	if err != nil {
		return nullValue(), err
	}
	difference := left - right
	if math.IsNaN(difference) {
		return floatResultValue(difference, args...), nil
	}
	if difference > 0 {
		return floatResultValue(difference, args...), nil
	}
	return floatResultValue(0, args...), nil
}

func copySignValue(args []Value) (Value, error) {
	if err := mathArity("copysign", args, 2); err != nil {
		return nullValue(), err
	}
	magnitude, err := mathRealArgument("copysign", args, 0)
	if err != nil {
		return nullValue(), err
	}
	sign, err := mathRealArgument("copysign", args, 1)
	if err != nil {
		return nullValue(), err
	}
	return floatResultValue(math.Copysign(magnitude, sign), args...), nil
}

func nextAfterValue(args []Value) (Value, error) {
	if err := mathArity("nextafter", args, 2); err != nil {
		return nullValue(), err
	}
	from, err := mathRealArgument("nextafter", args, 0)
	if err != nil {
		return nullValue(), err
	}
	to, err := mathRealArgument("nextafter", args, 1)
	if err != nil {
		return nullValue(), err
	}
	bits := floatResultBitsMany(args...)
	if bits == 32 {
		return floatResultValue(float64(math.Nextafter32(float32(from), float32(to))), args...), nil
	}
	return floatResultValue(math.Nextafter(from, to), args...), nil
}

func frexpValue(args []Value) (Value, error) {
	if err := mathArity("frexp", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("frexp", args, 0)
	if err != nil {
		return nullValue(), err
	}
	fraction, exponent := math.Frexp(value)
	return listValue([]Value{
		floatValueStyled(fraction, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])),
		intValue(big.NewInt(int64(exponent)), 64),
	}), nil
}

func ldexpValue(args []Value) (Value, error) {
	if err := mathArity("ldexp", args, 2); err != nil {
		return nullValue(), err
	}
	fraction, err := mathRealArgument("ldexp", args, 0)
	if err != nil {
		return nullValue(), err
	}
	exponent, err := mathExactInteger(args[1], "ldexp exponent")
	if err != nil {
		return nullValue(), err
	}
	hostMaxInt := int64(^uint(0) >> 1)
	hostMinInt := -hostMaxInt - 1
	if !exponent.IsInt64() || exponent.Int64() < hostMinInt || exponent.Int64() > hostMaxInt {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "ldexp exponent is outside the host integer range")
	}
	result := math.Ldexp(fraction, int(exponent.Int64()))
	if fraction != 0 && !math.IsInf(fraction, 0) && !math.IsNaN(fraction) && math.IsInf(result, 0) {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "ldexp overflow")
	}
	out := floatValueStyled(result, floatUnaryBits(args[0]), floatUnaryBFloat(args[0]))
	if fraction != 0 && !math.IsInf(fraction, 0) && !math.IsNaN(fraction) && math.IsInf(out.num, 0) {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "ldexp overflow")
	}
	return out, nil
}

func modfValue(args []Value) (Value, error) {
	if err := mathArity("modf", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("modf", args, 0)
	if err != nil {
		return nullValue(), err
	}
	integer, fraction := math.Modf(value)
	return listValue([]Value{
		floatValueStyled(integer, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])),
		floatValueStyled(fraction, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])),
	}), nil
}

func isFiniteValue(args []Value) (Value, error) {
	if err := mathArity("isfinite", args, 1); err != nil {
		return nullValue(), err
	}
	switch args[0].kind {
	case kindNumber:
		return boolValue(!math.IsInf(args[0].num, 0) && !math.IsNaN(args[0].num)), nil
	case kindComplex:
		return boolValue(!complexHasInf(args[0].cx) && !complexHasNaN(args[0].cx)), nil
	default:
		return nullValue(), mathFailure(ErrorKindType, "isfinite expects a floating-point or complex number, got %s", args[0].Kind())
	}
}

func isInfValue(args []Value) (Value, error) {
	if err := mathArity("isinf", args, 1); err != nil {
		return nullValue(), err
	}
	switch args[0].kind {
	case kindNumber:
		return boolValue(math.IsInf(args[0].num, 0)), nil
	case kindComplex:
		return boolValue(complexHasInf(args[0].cx)), nil
	default:
		return nullValue(), mathFailure(ErrorKindType, "isinf expects a floating-point or complex number, got %s", args[0].Kind())
	}
}

func isNaNValue(args []Value) (Value, error) {
	if err := mathArity("isnan", args, 1); err != nil {
		return nullValue(), err
	}
	switch args[0].kind {
	case kindNumber:
		return boolValue(math.IsNaN(args[0].num)), nil
	case kindComplex:
		return boolValue(complexHasNaN(args[0].cx) && !complexHasInf(args[0].cx)), nil
	default:
		return nullValue(), mathFailure(ErrorKindType, "isnan expects a floating-point or complex number, got %s", args[0].Kind())
	}
}

func isNormalValue(args []Value) (Value, error) {
	if err := mathArity("isnormal", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("isnormal", args, 0)
	if err != nil {
		return nullValue(), err
	}
	absolute := math.Abs(value)
	if absolute == 0 || math.IsInf(absolute, 0) || math.IsNaN(absolute) {
		return boolValue(false), nil
	}
	minimum := math.Float64frombits(0x0010000000000000)
	if args[0].fbits > 0 && args[0].fbits <= 32 {
		minimum = float64(math.Float32frombits(0x00800000))
	}
	return boolValue(absolute >= minimum), nil
}

func signBitValue(args []Value) (Value, error) {
	if err := mathArity("signbit", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("signbit", args, 0)
	if err != nil {
		return nullValue(), err
	}
	return boolValue(math.Signbit(value)), nil
}

func degreesValue(args []Value) (Value, error) {
	return mathUnaryReal(args, "degrees", func(value float64) float64 {
		return value * (180 / math.Pi)
	})
}

func radiansValue(args []Value) (Value, error) {
	return mathUnaryReal(args, "radians", func(value float64) float64 {
		return value * (math.Pi / 180)
	})
}

func reducedPiArgument(value float64) float64 {
	return math.Remainder(value, 2)
}

func sinPiScalar(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.NaN()
	}
	reduced := reducedPiArgument(value)
	switch reduced {
	case 0:
		return math.Copysign(0, value)
	case 0.5:
		return 1
	case -0.5:
		return -1
	case 1:
		return 0
	case -1:
		return math.Copysign(0, -1)
	default:
		return math.Sin(math.Pi * reduced)
	}
}

func cosPiScalar(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return math.NaN()
	}
	reduced := reducedPiArgument(value)
	switch reduced {
	case 0:
		return 1
	case 0.5, -0.5:
		return 0
	case 1, -1:
		return -1
	default:
		return math.Cos(math.Pi * reduced)
	}
}

func sinPiValue(args []Value) (Value, error) {
	return mathUnaryReal(args, "sinpi", sinPiScalar)
}

func cosPiValue(args []Value) (Value, error) {
	return mathUnaryReal(args, "cospi", cosPiScalar)
}

func tanPiValue(args []Value) (Value, error) {
	if err := mathArity("tanpi", args, 1); err != nil {
		return nullValue(), err
	}
	value, err := mathRealArgument("tanpi", args, 0)
	if err != nil {
		return nullValue(), err
	}
	if !math.IsNaN(value) && !math.IsInf(value, 0) {
		reduced := reducedPiArgument(value)
		if reduced == 0.5 || reduced == -0.5 {
			return nullValue(), mathFailure(ErrorKindInvalidArgument, "tanpi is undefined at half-integers")
		}
		if reduced == 0.25 || reduced == -0.75 {
			return floatValueStyled(1, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
		}
		if reduced == -0.25 || reduced == 0.75 {
			return floatValueStyled(-1, floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
		}
	}
	return floatValueStyled(sinPiScalar(value)/cosPiScalar(value), floatUnaryBits(args[0]), floatUnaryBFloat(args[0])), nil
}

func mathExactInteger(value Value, name string) (*big.Int, error) {
	switch value.kind {
	case kindInt:
		if value.i == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Set(value.i), nil
	case kindUint:
		if value.ub != nil {
			return new(big.Int).Set(value.ub), nil
		}
		return new(big.Int).SetUint64(value.u), nil
	case kindBigInt:
		if value.bi == nil {
			return new(big.Int), nil
		}
		return new(big.Int).Set(value.bi), nil
	case kindNumber:
		if math.IsNaN(value.num) || math.IsInf(value.num, 0) || math.Trunc(value.num) != value.num {
			return nil, mathFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.String())
		}
		integer, _ := new(big.Float).SetFloat64(value.num).Int(nil)
		return integer, nil
	default:
		return nil, mathFailure(ErrorKindType, "%s must be an exact integer, got %s", name, value.Kind())
	}
}

func maxIntegerBits(values ...Value) uint16 {
	var width uint16
	for _, value := range values {
		switch value.kind {
		case kindUint:
			candidate := value.ubits
			if candidate == 0 {
				candidate = uint16(value.bits)
			}
			if candidate == 0 {
				candidate = 64
			}
			if candidate > width {
				width = candidate
			}
		case kindInt:
			if value.ibits > width {
				width = value.ibits
			}
		}
	}
	if width == 0 {
		width = 64
	}
	return width
}

func integerPairResult(value *big.Int, left, right Value) Value {
	switch {
	case left.kind == kindUint && right.kind == kindUint:
		return uintValueFromBigStyle(value, maxIntegerBits(left, right), unsignedStyleFromValues(left, right))
	case left.kind == kindInt && right.kind == kindInt:
		return intValue(value, maxIntegerBits(left, right))
	default:
		return bigIntValue(value)
	}
}

func nonnegativeIntegerResult(value *big.Int, left, right Value) Value {
	if left.kind == kindUint && right.kind == kindUint {
		width := maxIntegerBits(left, right)
		if value.BitLen() <= int(width) {
			return uintValueFromBigStyle(value, width, unsignedStyleFromValues(left, right))
		}
	}
	return bigIntValue(value)
}

func divmodValue(args []Value) (Value, error) {
	if err := mathArity("divmod", args, 2); err != nil {
		return nullValue(), err
	}
	dividend, err := mathExactInteger(args[0], "divmod dividend")
	if err != nil {
		return nullValue(), err
	}
	divisor, err := mathExactInteger(args[1], "divmod divisor")
	if err != nil {
		return nullValue(), err
	}
	if divisor.Sign() == 0 {
		return nullValue(), mathFailure(ErrorKindInvalidArgument, "divmod divisor must be non-zero")
	}
	quotient := new(big.Int)
	remainder := new(big.Int)
	quotient.QuoRem(dividend, divisor, remainder)
	return listValue([]Value{
		integerPairResult(quotient, args[0], args[1]),
		integerPairResult(remainder, args[0], args[1]),
	}), nil
}

func gcdValue(args []Value) (Value, error) {
	if err := mathArity("gcd", args, 2); err != nil {
		return nullValue(), err
	}
	left, err := mathExactInteger(args[0], "gcd argument 1")
	if err != nil {
		return nullValue(), err
	}
	right, err := mathExactInteger(args[1], "gcd argument 2")
	if err != nil {
		return nullValue(), err
	}
	result := new(big.Int).GCD(nil, nil, left, right)
	result.Abs(result)
	return nonnegativeIntegerResult(result, args[0], args[1]), nil
}

func lcmValue(args []Value) (Value, error) {
	if err := mathArity("lcm", args, 2); err != nil {
		return nullValue(), err
	}
	left, err := mathExactInteger(args[0], "lcm argument 1")
	if err != nil {
		return nullValue(), err
	}
	right, err := mathExactInteger(args[1], "lcm argument 2")
	if err != nil {
		return nullValue(), err
	}
	if left.Sign() == 0 || right.Sign() == 0 {
		return nonnegativeIntegerResult(new(big.Int), args[0], args[1]), nil
	}
	gcd := new(big.Int).GCD(nil, nil, left, right)
	result := new(big.Int).Quo(new(big.Int).Set(left), gcd)
	result.Mul(result, right)
	result.Abs(result)
	return nonnegativeIntegerResult(result, args[0], args[1]), nil
}

func mathBitInput(name string, args []Value) (*big.Int, uint16, unsignedStyle, error) {
	if err := mathArity(name, args, 1); err != nil {
		return nil, 0, unsignedStyle{}, err
	}
	if args[0].kind != kindUint {
		return nil, 0, unsignedStyle{}, mathFailure(ErrorKindType, "%s expects a fixed-width unsigned integer, got %s", name, args[0].Kind())
	}
	value, width, err := unsignedIntFromValue(args[0])
	if err != nil {
		return nil, 0, unsignedStyle{}, mathFailure(ErrorKindType, "%s expects a fixed-width unsigned integer", name)
	}
	return value, width, unsignedStyleFromValues(args[0]), nil
}

func countOneBits(value *big.Int, width uint16) uint64 {
	var count uint64
	for bit := uint16(0); bit < width; bit++ {
		count += uint64(value.Bit(int(bit)))
	}
	return count
}

func countOnesValue(args []Value) (Value, error) {
	value, width, _, err := mathBitInput("count_ones", args)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(countOneBits(value, width), 64), nil
}

func countZerosValue(args []Value) (Value, error) {
	value, width, _, err := mathBitInput("count_zeros", args)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(width)-countOneBits(value, width), 64), nil
}

func leadingZerosValue(args []Value) (Value, error) {
	value, width, _, err := mathBitInput("leading_zeros", args)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(int(width)-value.BitLen()), 64), nil
}

func trailingZerosValue(args []Value) (Value, error) {
	value, width, _, err := mathBitInput("trailing_zeros", args)
	if err != nil {
		return nullValue(), err
	}
	if value.Sign() == 0 {
		return uintValue(uint64(width), 64), nil
	}
	var count uint16
	for count < width && value.Bit(int(count)) == 0 {
		count++
	}
	return uintValue(uint64(count), 64), nil
}

func bitWidthValue(args []Value) (Value, error) {
	value, _, _, err := mathBitInput("bit_width", args)
	if err != nil {
		return nullValue(), err
	}
	return uintValue(uint64(value.BitLen()), 64), nil
}

func bitFloorValue(args []Value) (Value, error) {
	value, width, style, err := mathBitInput("bit_floor", args)
	if err != nil {
		return nullValue(), err
	}
	result := new(big.Int)
	if value.Sign() != 0 {
		result.Lsh(big.NewInt(1), uint(value.BitLen()-1))
	}
	return uintValueFromBigStyle(result, width, style), nil
}

func bitCeilValue(args []Value) (Value, error) {
	value, width, style, err := mathBitInput("bit_ceil", args)
	if err != nil {
		return nullValue(), err
	}
	result := big.NewInt(1)
	if value.Cmp(big.NewInt(1)) > 0 {
		result.Lsh(result, uint(value.BitLen()))
		if new(big.Int).And(new(big.Int).Set(value), new(big.Int).Sub(new(big.Int).Set(value), big.NewInt(1))).Sign() == 0 {
			result.Set(value)
		}
	}
	if result.BitLen() > int(width) {
		return nullValue(), mathFailure(ErrorKindOutOfRange, "bit_ceil result is not representable in u%d", width)
	}
	return uintValueFromBigStyle(result, width, style), nil
}

func hasSingleBitValue(args []Value) (Value, error) {
	value, _, _, err := mathBitInput("has_single_bit", args)
	if err != nil {
		return nullValue(), err
	}
	if value.Sign() == 0 {
		return boolValue(false), nil
	}
	lessOne := new(big.Int).Sub(new(big.Int).Set(value), big.NewInt(1))
	return boolValue(new(big.Int).And(new(big.Int).Set(value), lessOne).Sign() == 0), nil
}

func byteSwapValue(args []Value) (Value, error) {
	value, width, style, err := mathBitInput("byte_swap", args)
	if err != nil {
		return nullValue(), err
	}
	if width%8 != 0 {
		return nullValue(), mathFailure(ErrorKindUnsupported, "byte_swap requires a byte-aligned unsigned width")
	}
	bytes := make([]byte, int(width/8))
	value.FillBytes(bytes)
	for left, right := 0, len(bytes)-1; left < right; left, right = left+1, right-1 {
		bytes[left], bytes[right] = bytes[right], bytes[left]
	}
	return uintValueFromBigStyle(new(big.Int).SetBytes(bytes), width, style), nil
}

func phaseValue(args []Value) (Value, error) {
	if err := mathArity("phase", args, 1); err != nil {
		return nullValue(), err
	}
	if args[0].kind != kindComplex {
		return nullValue(), mathFailure(ErrorKindType, "phase expects a complex number, got %s", args[0].Kind())
	}
	return floatValue(cmplx.Phase(args[0].cx), args[0].fbits), nil
}

func complexHasInf(value complex128) bool {
	return math.IsInf(real(value), 0) || math.IsInf(imag(value), 0)
}

func complexHasNaN(value complex128) bool {
	return math.IsNaN(real(value)) || math.IsNaN(imag(value))
}

func complexUnaryValue(args []Value, name string, fn func(complex128) complex128) (Value, bool, error) {
	if len(args) != 1 || args[0].kind != kindComplex {
		return Value{}, false, nil
	}
	return complexValueStyled(fn(args[0].cx), args[0].fbits), true, nil
}

func numericComplexUnaryValue(args []Value, name string, realFn func(float64) float64, complexFn func(complex128) complex128) (Value, error) {
	if len(args) != 1 {
		return nullValue(), mathFailure(ErrorKindInvalidArgument, "%s expects 1 argument", name)
	}
	return mapValueUnary(args[0], func(value Value) (Value, error) {
		if value.kind == kindComplex {
			return complexValueStyled(complexFn(value.cx), value.fbits), nil
		}
		number, err := value.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return floatValueStyled(realFn(number), floatUnaryBits(value), floatUnaryBFloat(value)), nil
	})
}

func complexPowValue(args []Value) (Value, bool, error) {
	if len(args) != 2 || (args[0].kind != kindComplex && args[1].kind != kindComplex) {
		return Value{}, false, nil
	}
	left, err := complexOperand(args[0], "pow")
	if err != nil {
		return nullValue(), true, err
	}
	right, err := complexOperand(args[1], "pow")
	if err != nil {
		return nullValue(), true, err
	}
	return complexValueStyled(cmplx.Pow(left, right), complexResultBits(args...)), true, nil
}

func complexOperand(value Value, name string) (complex128, error) {
	switch value.kind {
	case kindComplex:
		return value.cx, nil
	case kindNumber:
		return complex(value.num, 0), nil
	default:
		return 0, mathFailure(ErrorKindType, "%s complex form expects floating-point or complex arguments, got %s", name, value.Kind())
	}
}

func complexLogValue(args []Value) (Value, bool, error) {
	if len(args) < 1 || len(args) > 2 {
		return Value{}, false, nil
	}
	hasComplex := false
	for _, arg := range args {
		hasComplex = hasComplex || arg.kind == kindComplex
	}
	if !hasComplex {
		return Value{}, false, nil
	}
	value, err := complexOperand(args[0], "log")
	if err != nil {
		return nullValue(), true, err
	}
	result := cmplx.Log(value)
	if len(args) == 2 {
		base, err := complexOperand(args[1], "log")
		if err != nil {
			return nullValue(), true, err
		}
		if base == 0 || base == 1 {
			return nullValue(), true, mathFailure(ErrorKindInvalidArgument, "log base must not be zero or one")
		}
		result /= cmplx.Log(base)
	}
	return complexValueStyled(result, complexResultBits(args...)), true, nil
}
