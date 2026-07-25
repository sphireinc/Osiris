package osiris

import (
	"fmt"
	"math"
)

// hannValue returns the symmetric Hann window of the requested length.
func hannValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("hann expects 1 argument")
	}
	number, err := args[0].AsNumber()
	if err != nil || number < 0 || math.Trunc(number) != number {
		return nullValue(), fmt.Errorf("hann length must be a non-negative integer")
	}
	n := int(number)
	data := make([]Value, n)
	if n == 1 {
		data[0] = numberValue(1)
		return newTensorValue([]int{1}, data), nil
	}
	for i := range data {
		data[i] = numberValue(0.5 - 0.5*math.Cos(2*math.Pi*float64(i)/float64(n-1)))
	}
	return newTensorValue([]int{n}, data), nil
}

// convolveValue performs deterministic full one-dimensional convolution.
func convolveValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("convolve expects 2 arguments")
	}
	leftShape, left, err := inferTensor(args[0])
	if err != nil || len(leftShape) != 1 {
		return nullValue(), fmt.Errorf("convolve expects rank-one signal and kernel")
	}
	rightShape, right, err := inferTensor(args[1])
	if err != nil || len(rightShape) != 1 {
		return nullValue(), fmt.Errorf("convolve expects rank-one signal and kernel")
	}
	if len(left) == 0 || len(right) == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	data := make([]Value, len(left)+len(right)-1)
	for i := range data {
		var total float64
		for j := 0; j < len(left); j++ {
			k := i - j
			if k < 0 || k >= len(right) {
				continue
			}
			lv, err := left[j].AsNumber()
			if err != nil {
				return nullValue(), fmt.Errorf("convolve expects numeric values")
			}
			rv, err := right[k].AsNumber()
			if err != nil {
				return nullValue(), fmt.Errorf("convolve expects numeric values")
			}
			total += lv * rv
		}
		data[i] = numberValue(total)
	}
	return newTensorValue([]int{len(data)}, data), nil
}
