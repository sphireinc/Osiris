package osiris

import (
	"math"
	"testing"
)

func extendedSignalCall(t *testing.T, name string, args ...Value) Value {
	t.Helper()
	member, ok := extendedSignalModule()[name]
	if !ok || member.kind != kindFunction || member.fn == nil || member.fn.builtin == nil {
		t.Fatalf("signal member %q is not a builtin function", name)
	}
	value, err := member.fn.builtin(args)
	if err != nil {
		t.Fatalf("signal.%s failed: %v", name, err)
	}
	return value
}

func extendedSignalNumbers(t *testing.T, value Value) []float64 {
	t.Helper()
	_, data, err := inferTensor(value)
	if err != nil {
		t.Fatal(err)
	}
	result := make([]float64, len(data))
	for i, item := range data {
		var err error
		result[i], err = item.AsNumber()
		if err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestExtendedSignalModuleSurfaceAndWindows(t *testing.T) {
	members := extendedSignalModule()
	for _, name := range []string{
		"version", "max_samples", "max_transform_samples", "pi", "rectangular", "hamming",
		"blackman", "bartlett", "correlate", "filter_fir", "resample", "normalize", "mean",
		"variance", "energy", "rms", "fft", "ifft",
	} {
		if _, ok := members[name]; !ok {
			t.Fatalf("signal module is missing %q", name)
		}
	}
	for _, name := range []string{"rectangular", "hamming", "blackman", "bartlett"} {
		values := extendedSignalNumbers(t, extendedSignalCall(t, name, numberValue(5)))
		if len(values) != 5 || math.Abs(values[0]-values[4]) > 1e-12 {
			t.Fatalf("signal.%s is not symmetric: %v", name, values)
		}
	}
	if got := extendedSignalNumbers(t, extendedSignalCall(t, "rectangular", numberValue(3))); got[0] != 1 || got[1] != 1 || got[2] != 1 {
		t.Fatalf("rectangular window = %v", got)
	}
}

func TestExtendedSignalFilteringResamplingAndNormalization(t *testing.T) {
	signal := listValue([]Value{numberValue(1), numberValue(2), numberValue(3)})
	kernel := listValue([]Value{numberValue(1), numberValue(1)})
	if got := extendedSignalNumbers(t, extendedSignalCall(t, "filter_fir", signal, kernel)); !equalFloatSlice(got, []float64{1, 3, 5}, 1e-12) {
		t.Fatalf("FIR filter = %v", got)
	}
	if got := extendedSignalNumbers(t, extendedSignalCall(t, "correlate", signal, kernel)); !equalFloatSlice(got, []float64{1, 3, 5, 3}, 1e-12) {
		t.Fatalf("correlation = %v", got)
	}
	if got := extendedSignalNumbers(t, extendedSignalCall(t, "resample", signal, numberValue(5))); !equalFloatSlice(got, []float64{1, 1.5, 2, 2.5, 3}, 1e-12) {
		t.Fatalf("resample = %v", got)
	}
	if got := extendedSignalNumbers(t, extendedSignalCall(t, "normalize", signal, stringValue("minmax"))); !equalFloatSlice(got, []float64{0, 0.5, 1}, 1e-12) {
		t.Fatalf("minmax normalize = %v", got)
	}
	if got := extendedSignalCall(t, "normalize", signal, stringValue("peak")); got.kind != kindTensor {
		t.Fatalf("peak normalize kind = %s", got.kind)
	}
}

func TestExtendedSignalStatisticsAndTransforms(t *testing.T) {
	signal := listValue([]Value{numberValue(1), numberValue(2), numberValue(3), numberValue(4)})
	if got := extendedSignalCall(t, "mean", signal); math.Abs(got.num-2.5) > 1e-12 {
		t.Fatalf("mean = %v", got.num)
	}
	if got := extendedSignalCall(t, "variance", signal); math.Abs(got.num-1.25) > 1e-12 {
		t.Fatalf("variance = %v", got.num)
	}
	if got := extendedSignalCall(t, "energy", signal); math.Abs(got.num-30) > 1e-12 {
		t.Fatalf("energy = %v", got.num)
	}
	if got := extendedSignalCall(t, "rms", signal); math.Abs(got.num-math.Sqrt(7.5)) > 1e-12 {
		t.Fatalf("RMS = %v", got.num)
	}
	spectrum := extendedSignalCall(t, "fft", listValue([]Value{numberValue(1), numberValue(0), numberValue(0), numberValue(0)}))
	_, spectrumData, _ := inferTensor(spectrum)
	if len(spectrumData) != 4 || math.Abs(real(spectrumData[0].cx)-1) > 1e-12 || math.Abs(imag(spectrumData[1].cx)) > 1e-12 {
		t.Fatalf("FFT impulse spectrum = %v", spectrumData)
	}
	recovered := extendedSignalCall(t, "ifft", spectrum)
	_, recoveredData, _ := inferTensor(recovered)
	for i, item := range recoveredData {
		if math.Abs(real(item.cx)-[]float64{1, 0, 0, 0}[i]) > 1e-12 || math.Abs(imag(item.cx)) > 1e-12 {
			t.Fatalf("IFFT sample %d = %v", i, item)
		}
	}
}

func TestExtendedSignalLimitsAndErrors(t *testing.T) {
	if _, err := extendedSignalModule()["hamming"].fn.builtin([]Value{numberValue(extendedSignalMaxSamples + 1)}); err == nil {
		t.Fatal("window accepted more than the maximum sample count")
	}
	if _, err := extendedSignalModule()["fft"].fn.builtin([]Value{listValue([]Value{stringValue("not numeric")})}); err == nil {
		t.Fatal("FFT accepted a non-numeric signal")
	}
	if _, err := extendedSignalModule()["normalize"].fn.builtin([]Value{listValue([]Value{numberValue(1)}), stringValue("unknown")}); err == nil {
		t.Fatal("normalize accepted an unknown mode")
	}
	if _, err := extendedSignalModule()["mean"].fn.builtin([]Value{listValue(nil)}); err == nil {
		t.Fatal("mean accepted an empty signal")
	}
}

func equalFloatSlice(left, right []float64, epsilon float64) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if math.Abs(left[i]-right[i]) > epsilon {
			return false
		}
	}
	return true
}
