package osiris

import (
	"fmt"
	"math"
)

const (
	extendedSignalVersion             = "1.1.0"
	extendedSignalMaxSamples          = 1 << 16
	extendedSignalMaxTransformSamples = 1 << 12
)

// extendedSignalModule is the integration surface for the expanded
// std/signal module. All transforms are bounded, synchronous, and deterministic.
func extendedSignalModule() map[string]Value {
	return map[string]Value{
		"version":               stringValue(extendedSignalVersion),
		"max_samples":           uintValue(extendedSignalMaxSamples, 64),
		"max_transform_samples": uintValue(extendedSignalMaxTransformSamples, 64),
		"pi":                    numberValue(math.Pi),
		"rectangular":           functionValueOf("std/signal.rectangular", extendedSignalRectangular),
		"hamming":               functionValueOf("std/signal.hamming", extendedSignalHamming),
		"blackman":              functionValueOf("std/signal.blackman", extendedSignalBlackman),
		"bartlett":              functionValueOf("std/signal.bartlett", extendedSignalBartlett),
		"correlate":             functionValueOf("std/signal.correlate", extendedSignalCorrelate),
		"filter_fir":            functionValueOf("std/signal.filter_fir", extendedSignalFIR),
		"resample":              functionValueOf("std/signal.resample", extendedSignalResample),
		"normalize":             functionValueOf("std/signal.normalize", extendedSignalNormalize),
		"mean":                  functionValueOf("std/signal.mean", extendedSignalMean),
		"variance":              functionValueOf("std/signal.variance", extendedSignalVariance),
		"energy":                functionValueOf("std/signal.energy", extendedSignalEnergy),
		"rms":                   functionValueOf("std/signal.rms", extendedSignalRMS),
		"fft":                   functionValueOf("std/signal.fft", extendedSignalFFT),
		"ifft":                  functionValueOf("std/signal.ifft", extendedSignalIFFT),
	}
}

func extendedSignalData(value Value, operation string) ([]Value, error) {
	_, data, err := inferTensor(value)
	if err != nil {
		return nil, fmt.Errorf("signal.%s: expected a one-dimensional numeric signal", operation)
	}
	if len(data) > extendedSignalMaxSamples {
		return nil, fmt.Errorf("signal.%s exceeds the %d-sample limit", operation, extendedSignalMaxSamples)
	}
	for _, item := range data {
		if item.kind == kindComplex {
			return nil, fmt.Errorf("signal.%s expects real numeric samples", operation)
		}
		if _, err := item.AsNumber(); err != nil {
			return nil, fmt.Errorf("signal.%s expects numeric samples", operation)
		}
	}
	return data, nil
}

func extendedSignalLength(value Value, operation string, max int) (int, error) {
	number, err := value.AsNumber()
	if err != nil || number < 0 || number != float64(int(number)) || number > float64(max) {
		return 0, fmt.Errorf("signal.%s length must be an integer from 0 through %d", operation, max)
	}
	return int(number), nil
}

func extendedSignalWindow(args []Value, name string, formula func(i, n int) float64) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.%s expects length", name)
	}
	n, err := extendedSignalLength(args[0], name, extendedSignalMaxSamples)
	if err != nil {
		return nullValue(), err
	}
	data := make([]Value, n)
	for i := range data {
		data[i] = numberValue(formula(i, n))
	}
	return newTensorValue([]int{n}, data), nil
}

func extendedSignalRectangular(args []Value) (Value, error) {
	return extendedSignalWindow(args, "rectangular", func(_, _ int) float64 { return 1 })
}

func extendedSignalHamming(args []Value) (Value, error) {
	return extendedSignalWindow(args, "hamming", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		return 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	})
}

func extendedSignalBlackman(args []Value) (Value, error) {
	return extendedSignalWindow(args, "blackman", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		x := 2 * math.Pi * float64(i) / float64(n-1)
		return 0.42 - 0.5*math.Cos(x) + 0.08*math.Cos(2*x)
	})
}

func extendedSignalBartlett(args []Value) (Value, error) {
	return extendedSignalWindow(args, "bartlett", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		return 2 / float64(n-1) * (float64(n-1)/2 - math.Abs(float64(i)-float64(n-1)/2))
	})
}

func extendedSignalRealValues(data []Value) ([]float64, error) {
	result := make([]float64, len(data))
	for i, item := range data {
		value, err := item.AsNumber()
		if err != nil {
			return nil, err
		}
		result[i] = value
	}
	return result, nil
}

func extendedSignalCorrelate(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.correlate expects signal and kernel")
	}
	left, err := extendedSignalData(args[0], "correlate")
	if err != nil {
		return nullValue(), err
	}
	right, err := extendedSignalData(args[1], "correlate")
	if err != nil {
		return nullValue(), err
	}
	if len(left) == 0 || len(right) == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	leftNumbers, _ := extendedSignalRealValues(left)
	rightNumbers, _ := extendedSignalRealValues(right)
	data := make([]Value, len(left)+len(right)-1)
	for i := range data {
		total := 0.0
		for j := range leftNumbers {
			k := i - j
			if k >= 0 && k < len(rightNumbers) {
				total += leftNumbers[j] * rightNumbers[len(rightNumbers)-1-k]
			}
		}
		data[i] = numberValue(total)
	}
	return newTensorValue([]int{len(data)}, data), nil
}

func extendedSignalFIR(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.filter_fir expects signal and coefficients")
	}
	signal, err := extendedSignalData(args[0], "filter_fir")
	if err != nil {
		return nullValue(), err
	}
	coefficients, err := extendedSignalData(args[1], "filter_fir")
	if err != nil {
		return nullValue(), err
	}
	values, _ := extendedSignalRealValues(signal)
	coeffs, _ := extendedSignalRealValues(coefficients)
	output := make([]Value, len(values))
	for i := range values {
		total := 0.0
		for j := range coeffs {
			if i >= j {
				total += values[i-j] * coeffs[j]
			}
		}
		output[i] = numberValue(total)
	}
	return newTensorValue([]int{len(output)}, output), nil
}

func extendedSignalResample(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.resample expects signal and output length")
	}
	signal, err := extendedSignalData(args[0], "resample")
	if err != nil {
		return nullValue(), err
	}
	length, err := extendedSignalLength(args[1], "resample", extendedSignalMaxSamples)
	if err != nil {
		return nullValue(), err
	}
	values, _ := extendedSignalRealValues(signal)
	output := make([]Value, length)
	if length == 0 {
		return newTensorValue([]int{0}, output), nil
	}
	if len(values) == 0 {
		for i := range output {
			output[i] = numberValue(0)
		}
		return newTensorValue([]int{length}, output), nil
	}
	if len(values) == 1 {
		for i := range output {
			output[i] = numberValue(values[0])
		}
		return newTensorValue([]int{length}, output), nil
	}
	for i := range output {
		position := float64(i) * float64(len(values)-1) / float64(length-1)
		if length == 1 {
			position = 0
		}
		left := int(math.Floor(position))
		right := left + 1
		if right >= len(values) {
			right = len(values) - 1
		}
		fraction := position - float64(left)
		output[i] = numberValue(values[left] + fraction*(values[right]-values[left]))
	}
	return newTensorValue([]int{length}, output), nil
}

func extendedSignalNormalize(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.normalize expects signal and mode")
	}
	signal, err := extendedSignalData(args[0], "normalize")
	if err != nil {
		return nullValue(), err
	}
	mode, err := args[1].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("signal.normalize mode must be a string")
	}
	values, _ := extendedSignalRealValues(signal)
	if len(values) == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	output := make([]Value, len(values))
	switch mode {
	case "peak":
		peak := 0.0
		for _, value := range values {
			peak = math.Max(peak, math.Abs(value))
		}
		if peak == 0 {
			peak = 1
		}
		for i, value := range values {
			output[i] = numberValue(value / peak)
		}
	case "minmax":
		minimum, maximum := values[0], values[0]
		for _, value := range values[1:] {
			minimum, maximum = math.Min(minimum, value), math.Max(maximum, value)
		}
		span := maximum - minimum
		for i, value := range values {
			if span == 0 {
				output[i] = numberValue(0)
			} else {
				output[i] = numberValue((value - minimum) / span)
			}
		}
	case "zscore":
		mean := extendedSignalMeanNumbers(values)
		deviation := math.Sqrt(extendedSignalVarianceNumbers(values, mean))
		for i, value := range values {
			if deviation == 0 {
				output[i] = numberValue(0)
			} else {
				output[i] = numberValue((value - mean) / deviation)
			}
		}
	default:
		return nullValue(), fmt.Errorf("signal.normalize mode must be peak, minmax, or zscore")
	}
	return newTensorValue([]int{len(output)}, output), nil
}

func extendedSignalMeanNumbers(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func extendedSignalVarianceNumbers(values []float64, mean float64) float64 {
	total := 0.0
	for _, value := range values {
		delta := value - mean
		total += delta * delta
	}
	return total / float64(len(values))
}

func extendedSignalMean(args []Value) (Value, error) {
	values, err := extendedSignalDataArg(args, "mean")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.mean rejects an empty signal")
	}
	numbers, _ := extendedSignalRealValues(values)
	return numberValue(extendedSignalMeanNumbers(numbers)), nil
}

func extendedSignalVariance(args []Value) (Value, error) {
	values, err := extendedSignalDataArg(args, "variance")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.variance rejects an empty signal")
	}
	numbers, _ := extendedSignalRealValues(values)
	mean := extendedSignalMeanNumbers(numbers)
	return numberValue(extendedSignalVarianceNumbers(numbers, mean)), nil
}

func extendedSignalEnergy(args []Value) (Value, error) {
	values, err := extendedSignalDataArg(args, "energy")
	if err != nil {
		return nullValue(), err
	}
	numbers, _ := extendedSignalRealValues(values)
	total := 0.0
	for _, value := range numbers {
		total += value * value
	}
	return numberValue(total), nil
}

func extendedSignalRMS(args []Value) (Value, error) {
	values, err := extendedSignalDataArg(args, "rms")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.rms rejects an empty signal")
	}
	energy, err := extendedSignalEnergy(args)
	if err != nil {
		return nullValue(), err
	}
	return numberValue(math.Sqrt(energy.num / float64(len(values)))), nil
}

func extendedSignalDataArg(args []Value, operation string) ([]Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("signal.%s expects one signal", operation)
	}
	return extendedSignalData(args[0], operation)
}

func extendedSignalComplexData(value Value, operation string) ([]complex128, error) {
	_, data, err := inferTensor(value)
	if err != nil || len(data) > extendedSignalMaxTransformSamples {
		return nil, fmt.Errorf("signal.%s expects at most %d one-dimensional samples", operation, extendedSignalMaxTransformSamples)
	}
	output := make([]complex128, len(data))
	for i, item := range data {
		if item.kind == kindComplex {
			output[i] = item.cx
			continue
		}
		realValue, err := item.AsNumber()
		if err != nil {
			return nil, fmt.Errorf("signal.%s expects numeric samples", operation)
		}
		output[i] = complex(realValue, 0)
	}
	return output, nil
}

func extendedSignalFFT(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.fft expects one signal")
	}
	return extendedSignalTransform(args[0], false)
}

func extendedSignalIFFT(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.ifft expects one spectrum")
	}
	return extendedSignalTransform(args[0], true)
}

func extendedSignalTransform(value Value, inverse bool) (Value, error) {
	input, err := extendedSignalComplexData(value, "transform")
	if err != nil {
		return nullValue(), err
	}
	output := make([]Value, len(input))
	if len(input) == 0 {
		return newTensorValue([]int{0}, output), nil
	}
	direction := -1.0
	if inverse {
		direction = 1
	}
	for k := range input {
		total := complex(0, 0)
		for n, sample := range input {
			angle := direction * 2 * math.Pi * float64(k*n) / float64(len(input))
			total += sample * complex(math.Cos(angle), math.Sin(angle))
		}
		if inverse {
			total /= complex(float64(len(input)), 0)
		}
		output[k] = complexValue(total)
	}
	return newTensorValue([]int{len(output)}, output), nil
}
