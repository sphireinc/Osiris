package osiris

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestGPUReferenceProviderExecutesVersionedArtifact(t *testing.T) {
	prog, err := Parse("<gpu>", `emit matmul(matrix(2, 2, [1, 2, 3, 4]), matrix(2, 2, [5, 6, 7, 8]))`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileGPU(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGPUArtifact(artifact); err != nil {
		t.Fatal(err)
	}
	var matmul GPUKernel
	for _, kernel := range artifact.Kernels {
		if kernel.Op == GPUKernelMatMul {
			matmul = kernel
			break
		}
	}
	if matmul.Name == "" {
		t.Fatal("compiled artifact has no executable matmul kernel")
	}
	left, _ := NewGPUValue([]float64{1, 2, 3, 4}, []int{2, 2})
	right, _ := NewGPUValue([]float64{5, 6, 7, 8}, []int{2, 2})
	provider := NewDeterministicCPUProvider(artifact.Limits)
	got, err := ExecuteGPU(context.Background(), artifact, provider, GPUExecutionRequest{Kernel: matmul.Name, Inputs: []GPUValue{left, right}})
	if err != nil {
		t.Fatal(err)
	}
	want, _ := NewGPUValue([]float64{19, 22, 43, 50}, []int{2, 2})
	if err := CompareGPUValues(got, want, GPUComparisonTolerance{}); err != nil {
		t.Fatal(err)
	}
}

func TestGPUArtifactABIRoundTripAndMalformedRejection(t *testing.T) {
	prog, err := Parse("<gpu>", `emit [1, 2] + [3, 4]`)
	if err != nil {
		t.Fatal(err)
	}
	artifact, err := CompileGPU(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := EncodeGPUArtifactJSON(artifact)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodeGPUArtifactJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if decoded.ABI != defaultGPUABI() || decoded.Kernels[0].ABIVersion != GPUKernelABIVersion {
		t.Fatalf("decoded ABI = %#v, kernels = %#v", decoded.ABI, decoded.Kernels)
	}
	malformed := strings.Replace(string(raw), `"kernel_version": 1`, `"kernel_version": 99`, 1)
	if _, err := DecodeGPUArtifactJSON([]byte(malformed)); err == nil || !errors.Is(err, ErrGPUArtifactMalformed) {
		t.Fatalf("malformed ABI error = %v", err)
	}
}

func TestGPUProviderNegotiatesLimitsAndOwnership(t *testing.T) {
	provider := NewDeterministicCPUProvider(GPULimits{MaxElements: 2, MaxBufferBytes: 16, MaxRank: 2, MaxKernels: 4})
	if _, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{Required: []GPUCapability{"not-supported"}}); err == nil || !errors.Is(err, ErrGPUCapabilityUnavailable) {
		t.Fatalf("capability error = %v", err)
	}
	value, _ := NewGPUValue([]float64{1, 2, 3}, []int{3})
	device, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.Allocate(context.Background(), device, value); err == nil || !errors.Is(err, ErrGPUResourceLimit) {
		t.Fatalf("allocation limit error = %v", err)
	}
	foreign := newGPUBuffer("other-provider", value)
	kernel := newGPUKernel("osiris_elemwise_add", "elementwise")
	if _, _, err := provider.Launch(context.Background(), device, kernel, []*GPUBuffer{foreign, foreign}); err == nil || !errors.Is(err, ErrGPUOwnership) {
		t.Fatalf("ownership error = %v", err)
	}
	if err := foreign.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestGPUReferenceProviderCancellationAndRepeatability(t *testing.T) {
	artifact := GPUArtifact{
		SchemaVersion: gpuArtifactSchemaVersion,
		ABI:           defaultGPUABI(),
		Kernels:       []GPUKernel{newGPUKernel("osiris_elemwise_add", "elementwise")},
		Limits:        defaultGPULimits(),
	}
	left, _ := NewGPUValue([]float64{1, 2, 3}, []int{3})
	right, _ := NewGPUValue([]float64{4, 5, 6}, []int{3})
	provider := NewDeterministicCPUProvider(artifact.Limits)
	request := GPUExecutionRequest{Kernel: "osiris_elemwise_add", Inputs: []GPUValue{left, right}}
	first, err := ExecuteGPU(context.Background(), artifact, provider, request)
	if err != nil {
		t.Fatal(err)
	}
	second, err := ExecuteGPU(context.Background(), artifact, provider, request)
	if err != nil {
		t.Fatal(err)
	}
	if err := CompareGPUValues(first, second, GPUComparisonTolerance{}); err != nil {
		t.Fatal(err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ExecuteGPU(cancelled, artifact, provider, request); err == nil || !errors.Is(err, ErrGPUCancelled) {
		t.Fatalf("cancellation error = %v", err)
	}
}

func TestGPUParityProfileAndPureNumericKernelSurface(t *testing.T) {
	profile := DefaultGPUCapabilityProfile()
	if profile.Name != "deterministic-f64-tensor-v1" || profile.ValueABI == "" {
		t.Fatalf("profile = %#v", profile)
	}
	if profile.Unsupported["numeric/complex"] == "" || profile.Unsupported["stdlib/host-io"] == "" {
		t.Fatalf("profile does not state explicit host-only boundaries: %#v", profile.Unsupported)
	}

	good, err := Parse("<gpu-profile-good>", `emit sin([0, 1]) + [1, 2]`)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGPUProgram(good); err != nil {
		t.Fatal(err)
	}
	bad, err := Parse("<gpu-profile-bad>", `emit complex(1, 2)`)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateGPUProgram(bad); err == nil || !errors.Is(err, ErrGPUCapabilityUnavailable) {
		t.Fatalf("complex profile error = %v", err)
	}
}

func TestGPUReferenceProviderMatchesInterpreterAndVMForMappedSurface(t *testing.T) {
	prog, err := Parse("<gpu-differential>", `
import "std/math" as math
import "std/signal" as signal
emit sum([1, 2, 3, 4])
emit math.mean([1, 2, 3, 4])
emit [math.sin(0), math.sin(1)]
emit signal.hann(4)
emit signal.convolve([1, 2, 3], [1, 1])
`)
	if err != nil {
		t.Fatal(err)
	}
	rt := NewRuntime()
	interpreter, err := rt.Run(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	vm, err := rt.RunVM(context.Background(), prog)
	if err != nil {
		t.Fatal(err)
	}
	if len(interpreter) != len(vm) {
		t.Fatalf("interpreter outputs = %d, VM outputs = %d", len(interpreter), len(vm))
	}
	for i := range interpreter {
		if !interpreter[i].Equal(vm[i]) {
			t.Fatalf("runtime/VM output %d differ: %s vs %s", i, interpreter[i], vm[i])
		}
	}

	inputs := []struct {
		op     GPUKernelOp
		values []GPUValue
		want   Value
	}{
		{GPUKernelSum, []GPUValue{mustGPUValue(t, []float64{1, 2, 3, 4}, []int{4})}, interpreter[0]},
		{GPUKernelMean, []GPUValue{mustGPUValue(t, []float64{1, 2, 3, 4}, []int{4})}, interpreter[1]},
		{GPUKernelSin, []GPUValue{mustGPUValue(t, []float64{0, 1}, []int{2})}, interpreter[2]},
		{GPUKernelHann, []GPUValue{mustGPUValue(t, []float64{4}, nil)}, interpreter[3]},
		{GPUKernelConvolve, []GPUValue{mustGPUValue(t, []float64{1, 2, 3}, []int{3}), mustGPUValue(t, []float64{1, 1}, []int{2})}, interpreter[4]},
	}
	for i, test := range inputs {
		got := executeGPUOperationForTest(t, test.op, test.values)
		want, err := GPUValueFromValue(test.want)
		if err != nil {
			t.Fatal(err)
		}
		if err := CompareGPUValues(got, want, GPUComparisonTolerance{Absolute: 1e-14, Relative: 1e-14}); err != nil {
			t.Fatalf("operation %d (%s) differs from interpreter: %v", i, test.op, err)
		}
	}
}

func TestGPUReferenceProviderConstructorsRangesShapesAndSignalErrors(t *testing.T) {
	if got := executeGPUOperationForTest(t, GPUKernelArange, []GPUValue{mustGPUValue(t, []float64{5}, nil)}); !gpuFloatSlicesEqual(got.Data, []float64{0, 1, 2, 3, 4}) || !equalIntSlices(got.Shape, []int{5}) {
		t.Fatalf("arange = %#v", got)
	}
	if got := executeGPUOperationForTest(t, GPUKernelLinspace, []GPUValue{mustGPUValue(t, []float64{0}, nil), mustGPUValue(t, []float64{1}, nil), mustGPUValue(t, []float64{3}, nil)}); !gpuFloatSlicesEqual(got.Data, []float64{0, .5, 1}) {
		t.Fatalf("linspace = %#v", got)
	}
	if got := executeGPUOperationForTest(t, GPUKernelReshape, []GPUValue{mustGPUValue(t, []float64{1, 2, 3, 4}, []int{4}), mustGPUValue(t, []float64{2, 2}, []int{2})}); !equalIntSlices(got.Shape, []int{2, 2}) || !gpuFloatSlicesEqual(got.Data, []float64{1, 2, 3, 4}) {
		t.Fatalf("reshape = %#v", got)
	}
	if _, err := ExecuteGPU(context.Background(), testGPUArtifact(GPUKernelConvolve, 2), NewDeterministicCPUProvider(defaultGPULimits()), GPUExecutionRequest{Kernel: "test", Inputs: []GPUValue{mustGPUValue(t, []float64{1}, []int{1}), mustGPUValue(t, []float64{1}, []int{1})}}); err != nil {
		t.Fatal(err)
	}
	if _, err := ExecuteGPU(context.Background(), testGPUArtifact(GPUKernelHann, 1), NewDeterministicCPUProvider(defaultGPULimits()), GPUExecutionRequest{Kernel: "test", Inputs: []GPUValue{mustGPUValue(t, []float64{-1}, nil)}}); err == nil || !errors.Is(err, ErrGPUExecution) {
		t.Fatalf("hann domain error = %v", err)
	}
}

func executeGPUOperationForTest(t *testing.T, op GPUKernelOp, values []GPUValue) GPUValue {
	t.Helper()
	artifact := testGPUArtifact(op, len(values))
	got, err := ExecuteGPU(context.Background(), artifact, NewDeterministicCPUProvider(artifact.Limits), GPUExecutionRequest{Kernel: "test", Inputs: values})
	if err != nil {
		t.Fatalf("GPU %s: %v", op, err)
	}
	return got
}

func testGPUArtifact(op GPUKernelOp, arity int) GPUArtifact {
	return GPUArtifact{SchemaVersion: gpuArtifactSchemaVersion, ABI: defaultGPUABI(), Kernels: []GPUKernel{newGPUKernelForOp("test", string(op), op, arity)}, Limits: defaultGPULimits()}
}

func mustGPUValue(t *testing.T, data []float64, shape []int) GPUValue {
	t.Helper()
	value, err := NewGPUValue(data, shape)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func gpuFloatSlicesEqual(a, b []float64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
