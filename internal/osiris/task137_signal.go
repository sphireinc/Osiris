package osiris

import (
	"fmt"
	"math"
)

const (
	task137SignalVersion             = "1.1.0"
	task137SignalMaxSamples          = 1 << 16
	task137SignalMaxTransformSamples = 1 << 12
)

// task137SignalModule is the integration surface for the expanded
// std/signal module. All transforms are bounded, synchronous, and deterministic.
func task137SignalModule() map[string]Value {
	return map[string]Value{
		"version":               stringValue(task137SignalVersion),
		"max_samples":           uintValue(task137SignalMaxSamples, 64),
		"max_transform_samples": uintValue(task137SignalMaxTransformSamples, 64),
		"pi":                    numberValue(math.Pi),
		"rectangular":           functionValueOf("std/signal.rectangular", task137SignalRectangular),
		"hamming":               functionValueOf("std/signal.hamming", task137SignalHamming),
		"blackman":              functionValueOf("std/signal.blackman", task137SignalBlackman),
		"bartlett":              functionValueOf("std/signal.bartlett", task137SignalBartlett),
		"correlate":             functionValueOf("std/signal.correlate", task137SignalCorrelate),
		"filter_fir":            functionValueOf("std/signal.filter_fir", task137SignalFIR),
		"resample":              functionValueOf("std/signal.resample", task137SignalResample),
		"normalize":             functionValueOf("std/signal.normalize", task137SignalNormalize),
		"mean":                  functionValueOf("std/signal.mean", task137SignalMean),
		"variance":              functionValueOf("std/signal.variance", task137SignalVariance),
		"energy":                functionValueOf("std/signal.energy", task137SignalEnergy),
		"rms":                   functionValueOf("std/signal.rms", task137SignalRMS),
		"fft":                   functionValueOf("std/signal.fft", task137SignalFFT),
		"ifft":                  functionValueOf("std/signal.ifft", task137SignalIFFT),
	}
}

func task137SignalData(value Value, operation string) ([]Value, error) {
	_, data, err := inferTensor(value)
	if err != nil {
		return nil, fmt.Errorf("signal.%s: expected a one-dimensional numeric signal", operation)
	}
	if len(data) > task137SignalMaxSamples {
		return nil, fmt.Errorf("signal.%s exceeds the %d-sample limit", operation, task137SignalMaxSamples)
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

func task137SignalLength(value Value, operation string, max int) (int, error) {
	number, err := value.AsNumber()
	if err != nil || number < 0 || number != float64(int(number)) || number > float64(max) {
		return 0, fmt.Errorf("signal.%s length must be an integer from 0 through %d", operation, max)
	}
	return int(number), nil
}

func task137SignalWindow(args []Value, name string, formula func(i, n int) float64) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.%s expects length", name)
	}
	n, err := task137SignalLength(args[0], name, task137SignalMaxSamples)
	if err != nil {
		return nullValue(), err
	}
	data := make([]Value, n)
	for i := range data {
		data[i] = numberValue(formula(i, n))
	}
	return newTensorValue([]int{n}, data), nil
}

func task137SignalRectangular(args []Value) (Value, error) {
	return task137SignalWindow(args, "rectangular", func(_, _ int) float64 { return 1 })
}

func task137SignalHamming(args []Value) (Value, error) {
	return task137SignalWindow(args, "hamming", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		return 0.54 - 0.46*math.Cos(2*math.Pi*float64(i)/float64(n-1))
	})
}

func task137SignalBlackman(args []Value) (Value, error) {
	return task137SignalWindow(args, "blackman", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		x := 2 * math.Pi * float64(i) / float64(n-1)
		return 0.42 - 0.5*math.Cos(x) + 0.08*math.Cos(2*x)
	})
}

func task137SignalBartlett(args []Value) (Value, error) {
	return task137SignalWindow(args, "bartlett", func(i, n int) float64 {
		if n <= 1 {
			return 1
		}
		return 2 / float64(n-1) * (float64(n-1)/2 - math.Abs(float64(i)-float64(n-1)/2))
	})
}

func task137SignalRealValues(data []Value) ([]float64, error) {
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

func task137SignalCorrelate(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.correlate expects signal and kernel")
	}
	left, err := task137SignalData(args[0], "correlate")
	if err != nil {
		return nullValue(), err
	}
	right, err := task137SignalData(args[1], "correlate")
	if err != nil {
		return nullValue(), err
	}
	if len(left) == 0 || len(right) == 0 {
		return newTensorValue([]int{0}, nil), nil
	}
	leftNumbers, _ := task137SignalRealValues(left)
	rightNumbers, _ := task137SignalRealValues(right)
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

func task137SignalFIR(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.filter_fir expects signal and coefficients")
	}
	signal, err := task137SignalData(args[0], "filter_fir")
	if err != nil {
		return nullValue(), err
	}
	coefficients, err := task137SignalData(args[1], "filter_fir")
	if err != nil {
		return nullValue(), err
	}
	values, _ := task137SignalRealValues(signal)
	coeffs, _ := task137SignalRealValues(coefficients)
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

func task137SignalResample(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.resample expects signal and output length")
	}
	signal, err := task137SignalData(args[0], "resample")
	if err != nil {
		return nullValue(), err
	}
	length, err := task137SignalLength(args[1], "resample", task137SignalMaxSamples)
	if err != nil {
		return nullValue(), err
	}
	values, _ := task137SignalRealValues(signal)
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

func task137SignalNormalize(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("signal.normalize expects signal and mode")
	}
	signal, err := task137SignalData(args[0], "normalize")
	if err != nil {
		return nullValue(), err
	}
	mode, err := args[1].AsString()
	if err != nil {
		return nullValue(), fmt.Errorf("signal.normalize mode must be a string")
	}
	values, _ := task137SignalRealValues(signal)
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
		mean := task137SignalMeanNumbers(values)
		deviation := math.Sqrt(task137SignalVarianceNumbers(values, mean))
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

func task137SignalMeanNumbers(values []float64) float64 {
	total := 0.0
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func task137SignalVarianceNumbers(values []float64, mean float64) float64 {
	total := 0.0
	for _, value := range values {
		delta := value - mean
		total += delta * delta
	}
	return total / float64(len(values))
}

func task137SignalMean(args []Value) (Value, error) {
	values, err := task137SignalDataArg(args, "mean")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.mean rejects an empty signal")
	}
	numbers, _ := task137SignalRealValues(values)
	return numberValue(task137SignalMeanNumbers(numbers)), nil
}

func task137SignalVariance(args []Value) (Value, error) {
	values, err := task137SignalDataArg(args, "variance")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.variance rejects an empty signal")
	}
	numbers, _ := task137SignalRealValues(values)
	mean := task137SignalMeanNumbers(numbers)
	return numberValue(task137SignalVarianceNumbers(numbers, mean)), nil
}

func task137SignalEnergy(args []Value) (Value, error) {
	values, err := task137SignalDataArg(args, "energy")
	if err != nil {
		return nullValue(), err
	}
	numbers, _ := task137SignalRealValues(values)
	total := 0.0
	for _, value := range numbers {
		total += value * value
	}
	return numberValue(total), nil
}

func task137SignalRMS(args []Value) (Value, error) {
	values, err := task137SignalDataArg(args, "rms")
	if err != nil {
		return nullValue(), err
	}
	if len(values) == 0 {
		return nullValue(), fmt.Errorf("signal.rms rejects an empty signal")
	}
	energy, err := task137SignalEnergy(args)
	if err != nil {
		return nullValue(), err
	}
	return numberValue(math.Sqrt(energy.num / float64(len(values)))), nil
}

func task137SignalDataArg(args []Value, operation string) ([]Value, error) {
	if len(args) != 1 {
		return nil, fmt.Errorf("signal.%s expects one signal", operation)
	}
	return task137SignalData(args[0], operation)
}

func task137SignalComplexData(value Value, operation string) ([]complex128, error) {
	_, data, err := inferTensor(value)
	if err != nil || len(data) > task137SignalMaxTransformSamples {
		return nil, fmt.Errorf("signal.%s expects at most %d one-dimensional samples", operation, task137SignalMaxTransformSamples)
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

func task137SignalFFT(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.fft expects one signal")
	}
	return task137SignalTransform(args[0], false)
}

func task137SignalIFFT(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("signal.ifft expects one spectrum")
	}
	return task137SignalTransform(args[0], true)
}

func task137SignalTransform(value Value, inverse bool) (Value, error) {
	input, err := task137SignalComplexData(value, "transform")
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
