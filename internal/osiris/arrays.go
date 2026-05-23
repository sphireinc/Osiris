package osiris

import (
	"fmt"
	"math"
	"sort"
)

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

func sinhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "sinh", math.Sinh, false)
}

func coshValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "cosh", math.Cosh, false)
}

func tanhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "tanh", math.Tanh, false)
}

func asinhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "asinh", math.Asinh, false)
}

func acoshValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "acosh", math.Acosh, true)
}

func atanhValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "atanh", math.Atanh, true)
}

func expm1Value(args []Value) (Value, error) {
	return numericUnaryValue(args, "expm1", math.Expm1, false)
}

func log1pValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "log1p", math.Log1p, false)
}

func cbrtValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "cbrt", math.Cbrt, false)
}

func erfValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "erf", math.Erf, false)
}

func erfcValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "erfc", math.Erfc, false)
}

func gammaValue(args []Value) (Value, error) {
	return numericUnaryValue(args, "gamma", math.Gamma, false)
}

func lgammaValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "lgamma", func(n float64) (float64, error) {
		lg, _ := math.Lgamma(n)
		return lg, nil
	})
}

func stepValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "step", func(n float64) (float64, error) {
		if n > 0 {
			return 1, nil
		}
		return 0, nil
	})
}

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

func reluValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "relu", func(n float64) (float64, error) {
		if n < 0 {
			return 0, nil
		}
		return n, nil
	})
}

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

func softplusValue(args []Value) (Value, error) {
	return scalarUnaryMapped(args, "softplus", func(n float64) (float64, error) {
		if n > 20 {
			// stable approximation
			return n, nil
		}
		return math.Log1p(math.Exp(n)), nil
	})
}

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

func zerosValue(args []Value) (Value, error) {
	shape, err := parseShapeArgs(args)
	if err != nil {
		return nullValue(), err
	}
	return makeTensorWithFill(shape, numberValue(0))
}

func onesValue(args []Value) (Value, error) {
	shape, err := parseShapeArgs(args)
	if err != nil {
		return nullValue(), err
	}
	return makeTensorWithFill(shape, numberValue(1))
}

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

func eyeValue(args []Value) (Value, error) {
	return identityValue(args)
}

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

func numericSlice(args []Value) ([]Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("expected 1 argument")
	}
	return flattenValues(args[0], nil), nil
}

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
