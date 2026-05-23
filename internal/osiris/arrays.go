package osiris

import (
	"fmt"
	"math"
	"sort"
)

// mapValueUnary handles the map value unary step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func mapValueUnary(v Value, fn func(Value) (Value, error)) (Value, error) {
	switch v.kind {
	case kindList:
		out := make([]Value, len(v.list))
		for i := range v.list {
			mapped, err := mapValueUnary(v.list[i], fn)
			if err != nil {
				return nullValue(), err
			}
			out[i] = mapped
		}
		return listValue(out), nil
	case kindTensor:
		out := make([]Value, len(v.data))
		for i := range v.data {
			mapped, err := mapValueUnary(v.data[i], fn)
			if err != nil {
				return nullValue(), err
			}
			out[i] = mapped
		}
		return newTensorValue(v.shape, out), nil
	default:
		return fn(v)
	}
}

// numericUnaryValue handles the numeric unary value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func numericUnaryValue(args []Value, name string, fn func(float64) float64, rejectInvalid bool) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("%s expects 1 argument", name)
	}
	return mapValueUnary(args[0], func(v Value) (Value, error) {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		out := fn(n)
		if rejectInvalid && (math.IsNaN(out) || math.IsInf(out, 0)) {
			return nullValue(), fmt.Errorf("%s domain error", name)
		}
		return floatValueStyled(out, floatUnaryBits(v), floatUnaryBFloat(v)), nil
	})
}

// scalarUnaryMapped handles the scalar unary mapped step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func scalarUnaryMapped(args []Value, name string, fn func(float64) (float64, error)) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("%s expects 1 argument", name)
	}
	return mapValueUnary(args[0], func(v Value) (Value, error) {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		out, err := fn(n)
		if err != nil {
			return nullValue(), err
		}
		return floatValueStyled(out, floatUnaryBits(v), floatUnaryBFloat(v)), nil
	})
}

// signValue handles the sign value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func signValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "sign", func(n float64) (float64, error) {
		switch {
		case n > 0:
			return 1, nil
		case n < 0:
			return -1, nil
		default:
			return 0, nil
		}
	})
}

// sinhValue handles the sinh value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func sinhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "sinh", math.Sinh, false)
}

// coshValue handles the cosh value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func coshValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "cosh", math.Cosh, false)
}

// tanhValue handles the tanh value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tanhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "tanh", math.Tanh, false)
}

// asinhValue handles the asinh value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func asinhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "asinh", math.Asinh, false)
}

// acoshValue handles the acosh value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func acoshValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "acosh", math.Acosh, true)
}

// atanhValue handles the atanh value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func atanhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "atanh", math.Atanh, true)
}

// expm1Value handles the expm1 value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func expm1Value(args []Value) (Value, error) {
	return numericUnaryValue(args, "expm1", math.Expm1, false)
}

// log1pValue handles the log1p value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func log1pValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "log1p", math.Log1p, false)
}

// cbrtValue handles the cbrt value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func cbrtValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "cbrt", math.Cbrt, false)
}

// erfValue handles the erf value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func erfValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "erf", math.Erf, false)
}

// erfcValue handles the erfc value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func erfcValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "erfc", math.Erfc, false)
}

// gammaValue handles the gamma value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func gammaValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "gamma", math.Gamma, false)
}

// lgammaValue handles the lgamma value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func lgammaValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "lgamma", func(n float64) (float64, error) {
		lg, _ := math.Lgamma(n)
		return lg, nil
	})
}

// stepValue handles the step value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func stepValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "step", func(n float64) (float64, error) {
		if n > 0 {
			return 1, nil
		}
		return 0, nil
	})
}

// heavisideValue handles the heaviside value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func heavisideValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "heaviside", func(n float64) (float64, error) {
		switch {
		case n > 0:
			return 1, nil
		case n < 0:
			return 0, nil
		default:
			return 0.5, nil
		}
	})
}

// reluValue handles the relu value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func reluValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "relu", func(n float64) (float64, error) {
		if n < 0 {
			return 0, nil
		}
		return n, nil
	})
}

// sigmoidValue handles the sigmoid value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func sigmoidValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "sigmoid", func(n float64) (float64, error) {
		if n >= 0 {
			z := math.Exp(-n)
			return 1 / (1 + z), nil
		}
		z := math.Exp(n)
		return z / (1 + z), nil
	})
}

// softplusValue handles the softplus value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func softplusValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "softplus", func(n float64) (float64, error) {
		if n > 20 {
			// stable approximation
			return n, nil
		}
		return math.Log1p(math.Exp(n)), nil
	})
}

// makeTensorWithFill handles the make tensor with fill step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func makeTensorWithFill(shape []int, fill Value) (Value, error) {
	if len(shape) == 0 {
		return fill, nil
	}
	total := tensorShapeProduct(shape)
	if total < 0 {
		return nullValue(), fmt.Errorf("invalid tensor shape")
	}
	data := make([]Value, total)
	for i := range data {
		data[i] = fill
	}
	return tensorFromShapeAndData(shape, data)
}

// parseShapeArgs handles the parse shape args step in the surrounding package
// and returns []int, error.
//
// It returns []int and an error. Callers should only trust the value when the
// error is nil.
func parseShapeArgs(args []Value) ([]int, error) {
	if len(args) == 0 {
		return nil, fmt.Errorf("shape expects at least 1 dimension")
	}
	if len(args) == 1 {
		switch args[0].kind {
		case kindList, kindTensor:
			return tensorDims(args[0])
		default:
			n, err := args[0].Int()
			if err != nil {
				return nil, err
			}
			if n < 0 {
				return nil, fmt.Errorf("shape dimensions must be non-negative")
			}
			return []int{n}, nil
		}
	}
	dims := make([]int, len(args))
	for i := range args {
		n, err := args[i].Int()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, fmt.Errorf("shape dimensions must be non-negative")
		}
		dims[i] = n
	}
	return dims, nil
}

// zerosValue handles the zeros value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func zerosValue(args []Value) (Value, error) {
	shape, err := parseShapeArgs(args)
	if err != nil {
		return nullValue(), err
	}
	return makeTensorWithFill(shape, numberValue(0))
}

// onesValue handles the ones value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func onesValue(args []Value) (Value, error) {
	shape, err := parseShapeArgs(args)
	if err != nil {
		return nullValue(), err
	}
	return makeTensorWithFill(shape, numberValue(1))
}

// fullValue handles the full value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func fullValue(args []Value) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("full expects shape and fill value")
	}
	shape, err := parseShapeArgs(args[:len(args)-1])
	if err != nil {
		return nullValue(), err
	}
	return makeTensorWithFill(shape, args[len(args)-1])
}

// identityValue handles the identity value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func identityValue(args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nullValue(), fmt.Errorf("identity expects 1 or 2 arguments")
	}
	n, err := args[0].Int()
	if err != nil {
		return nullValue(), err
	}
	if n < 0 {
		return nullValue(), fmt.Errorf("identity size must be non-negative")
	}
	m := n
	if len(args) == 2 {
		m, err = args[1].Int()
		if err != nil {
			return nullValue(), err
		}
		if m < 0 {
			return nullValue(), fmt.Errorf("identity size must be non-negative")
		}
	}
	data := make([]Value, n*m)
	for i := range data {
		data[i] = numberValue(0)
	}
	lim := n
	if m < lim {
		lim = m
	}
	for i := 0; i < lim; i++ {
		data[i*m+i] = numberValue(1)
	}
	return tensorFromShapeAndData([]int{n, m}, data)
}

// eyeValue handles the eye value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func eyeValue(args []Value) (Value, error) {
	return identityValue(args)
}

// arangeValue handles the arange value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func arangeValue(args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 && len(args) != 3 {
		return nullValue(), fmt.Errorf("arange expects 1, 2, or 3 arguments")
	}
	start := 0.0
	stop := 0.0
	step := 1.0
	var err error
	switch len(args) {
	case 1:
		stop, err = args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
	case 2:
		start, err = args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		stop, err = args[1].AsNumber()
		if err != nil {
			return nullValue(), err
		}
	case 3:
		start, err = args[0].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		stop, err = args[1].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		step, err = args[2].AsNumber()
		if err != nil {
			return nullValue(), err
		}
	}
	if step == 0 {
		return nullValue(), fmt.Errorf("arange step must not be zero")
	}
	out := make([]Value, 0, 16)
	if step > 0 {
		for x := start; x < stop; x += step {
			out = append(out, numberValue(x))
			if len(out) > 1_000_000 {
				return nullValue(), fmt.Errorf("arange generated too many elements")
			}
		}
	} else {
		for x := start; x > stop; x += step {
			out = append(out, numberValue(x))
			if len(out) > 1_000_000 {
				return nullValue(), fmt.Errorf("arange generated too many elements")
			}
		}
	}
	return newTensorValue([]int{len(out)}, out), nil
}

// linspaceValue handles the linspace value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func linspaceValue(args []Value) (Value, error) {
	if len(args) != 2 && len(args) != 3 {
		return nullValue(), fmt.Errorf("linspace expects 2 or 3 arguments")
	}
	start, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	stop, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	count := 50
	if len(args) == 3 {
		count, err = args[2].Int()
		if err != nil {
			return nullValue(), err
		}
	}
	if count < 0 {
		return nullValue(), fmt.Errorf("linspace count must be non-negative")
	}
	if count == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	out := make([]Value, count)
	if count == 1 {
		out[0] = numberValue(start)
		return newTensorValue([]int{1}, out), nil
	}
	step := (stop - start) / float64(count-1)
	for i := 0; i < count; i++ {
		out[i] = numberValue(start + float64(i)*step)
	}
	return newTensorValue([]int{count}, out), nil
}

// logspaceValue handles the logspace value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func logspaceValue(args []Value) (Value, error) {
	if len(args) != 3 && len(args) != 4 {
		return nullValue(), fmt.Errorf("logspace expects 3 or 4 arguments")
	}
	start, err := args[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	stop, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	count, err := args[2].Int()
	if err != nil {
		return nullValue(), err
	}
	base := 10.0
	if len(args) == 4 {
		base, err = args[3].AsNumber()
		if err != nil {
			return nullValue(), err
		}
	}
	if count < 0 {
		return nullValue(), fmt.Errorf("logspace count must be non-negative")
	}
	if count == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	if count == 1 {
		out := []Value{numberValue(math.Pow(base, start))}
		return newTensorValue([]int{1}, out), nil
	}
	step := (stop - start) / float64(count-1)
	out := make([]Value, count)
	for i := 0; i < count; i++ {
		out[i] = numberValue(math.Pow(base, start+float64(i)*step))
	}
	return newTensorValue([]int{count}, out), nil
}

// flattenValues handles the flatten values step in the surrounding package and
// returns []Value.
//
// It returns []Value so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func flattenValues(v Value, out []Value) []Value {
	switch v.kind {
	case kindList:
		for _, item := range v.list {
			out = flattenValues(item, out)
		}
		return out
	case kindTensor:
		for _, item := range v.data {
			out = flattenValues(item, out)
		}
		return out
	default:
		return append(out, v)
	}
}

// numericSlice handles the numeric slice step in the surrounding package and
// returns []Value, error.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func numericSlice(args []Value) ([]Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}
	return flattenValues(args[0], nil), nil
}

// meanValue handles the mean value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func meanValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("mean expects non-empty data")
	}
	var total float64
	for _, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n
	}
	return numberValue(total / float64(len(values))), nil
}

// varValue handles the var value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func varValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("var expects non-empty data")
	}
	mean, err := meanValue(args)
	if err != nil {
		return nullValue(), err
	}
	m, _ := mean.AsNumber()
	var sum float64
	for _, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		d := n - m
		sum += d * d
	}
	return numberValue(sum / float64(len(values))), nil
}

// stdValue handles the std value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func stdValue(args []Value) (Value, error) {
	v, err := varValue(args)
	if err != nil {
		return nullValue(), err
	}
	n, err := v.AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return numberValue(math.Sqrt(n)), nil
}

// medianValue handles the median value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func medianValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("median expects non-empty data")
	}
	nums := make([]float64, len(values))
	for i, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		nums[i] = n
	}
	sort.Float64s(nums)
	mid := len(nums) / 2
	if len(nums)%2 == 1 {
		return numberValue(nums[mid]), nil
	}
	return numberValue((nums[mid-1] + nums[mid]) / 2), nil
}

// quantileValue handles the quantile value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func quantileValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("quantile expects data and q")
	}
	values := flattenValues(args[0], nil)
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("quantile expects non-empty data")
	}
	q, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	if q < 0 || q > 1 {
		return nullValue(), fmt.Errorf("quantile q must be in [0,1]")
	}
	nums := make([]float64, len(values))
	for i, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		nums[i] = n
	}
	sort.Float64s(nums)
	pos := q * float64(len(nums)-1)
	lo := int(math.Floor(pos))
	hi := int(math.Ceil(pos))
	if lo == hi {
		return numberValue(nums[lo]), nil
	}
	frac := pos - float64(lo)
	return numberValue(nums[lo]*(1-frac) + nums[hi]*frac), nil
}

// percentileValue handles the percentile value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func percentileValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("percentile expects data and p")
	}
	p, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return quantileValue([]Value{args[0], numberValue(p / 100)})
}

// argminValue handles the argmin value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func argminValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("argmin expects non-empty data")
	}
	best := 0
	bestVal, err := values[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	for i := 1; i < len(values); i++ {
		n, err := values[i].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if n < bestVal {
			bestVal = n
			best = i
		}
	}
	return uintValue(uint64(best), 64), nil
}

// argmaxValue handles the argmax value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func argmaxValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("argmax expects non-empty data")
	}
	best := 0
	bestVal, err := values[0].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	for i := 1; i < len(values); i++ {
		n, err := values[i].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if n > bestVal {
			bestVal = n
			best = i
		}
	}
	return uintValue(uint64(best), 64), nil
}

// anyValue handles the any value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func anyValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	for _, v := range values {
		if v.Truthy() {
			return boolValue(true), nil
		}
	}
	return boolValue(false), nil
}

// allValue handles the all value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func allValue(args []Value) (Value, error) {
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	for _, v := range values {
		if !v.Truthy() {
			return boolValue(false), nil
		}
	}
	return boolValue(true), nil
}

// cumsumValue handles the cumsum value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func cumsumValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("cumsum expects 1 argument")
	}
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(values))
	var total float64
	for i, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n
		out[i] = numberValue(total)
	}
	if args[0].kind == kindTensor {
		return newTensorValue(args[0].shape, out), nil
	}
	return listValue(out), nil
}

// cumprodValue handles the cumprod value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func cumprodValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("cumprod expects 1 argument")
	}
	values, err := numericSlice(args)
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(values))
	prod := 1.0
	for i, v := range values {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		prod *= n
		out[i] = numberValue(prod)
	}
	if args[0].kind == kindTensor {
		return newTensorValue(args[0].shape, out), nil
	}
	return listValue(out), nil
}
