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

func TestNativeGPUProviderDiscoveryAndExplicitUnavailableBehavior(t *testing.T) {
	provider, err := NewNativeGPUProvider(defaultGPULimits())
	if err != nil {
		if !errors.Is(err, ErrGPUCapabilityUnavailable) {
			t.Fatalf("native provider discovery error = %v, want capability-unavailable error", err)
		}
		return
	}
	defer provider.Close()
	devices, err := provider.Devices(context.Background())
	if err != nil {
		t.Fatalf("native devices: %v", err)
	}
	if len(devices) != 1 || devices[0].Kind != "metal" || !gpuHasCapability(devices[0].Capabilities, GPUCapabilityNative) {
		t.Fatalf("native devices = %#v", devices)
	}
	if gpuHasCapability(devices[0].Capabilities, GPUCapabilityDeterministic) {
		t.Fatal("native device must not claim deterministic reference semantics")
	}
}

func TestNativeGPUProviderExecutesAndOwnsMetalBuffers(t *testing.T) {
	provider, err := NewNativeGPUProvider(defaultGPULimits())
	if err != nil {
		if errors.Is(err, ErrGPUCapabilityUnavailable) {
			t.Skipf("native GPU unavailable: %v", err)
		}
		t.Fatalf("native provider: %v", err)
	}
	defer provider.Close()
	artifact := testGPUArtifact(GPUKernelAdd, 2)
	left := mustGPUValue(t, []float64{1, 2, 3}, []int{3})
	right := mustGPUValue(t, []float64{4, 5, 6}, []int{3})
	got, err := ExecuteGPU(context.Background(), artifact, provider, GPUExecutionRequest{Kernel: "test", Inputs: []GPUValue{left, right}})
	if err != nil {
		t.Fatalf("native add: %v", err)
	}
	if err := CompareGPUValues(got, mustGPUValue(t, []float64{5, 7, 9}, []int{3}), GPUComparisonTolerance{Absolute: 1e-5, Relative: 1e-5}); err != nil {
		t.Fatal(err)
	}

	device, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{})
	if err != nil {
		t.Fatalf("select native device: %v", err)
	}
	owned, err := provider.Allocate(context.Background(), device, left)
	if err != nil {
		t.Fatalf("allocate native buffer: %v", err)
	}
	if err := owned.Release(); err != nil {
		t.Fatalf("release native buffer: %v", err)
	}
	if _, err := owned.Read(context.Background()); !errors.Is(err, ErrGPUOwnership) {
		t.Fatalf("read released native buffer = %v, want ownership error", err)
	}
}

func TestExecuteGPUUsesNativeProviderWithoutReferenceFallback(t *testing.T) {
	artifact := testGPUArtifact(GPUKernelMul, 2)
	request := GPUExecutionRequest{Kernel: "test", Inputs: []GPUValue{
		mustGPUValue(t, []float64{2, 3}, []int{2}),
		mustGPUValue(t, []float64{4, 5}, []int{2}),
	}}
	got, err := ExecuteGPU(context.Background(), artifact, nil, request)
	if err != nil {
		if !errors.Is(err, ErrGPUCapabilityUnavailable) {
			t.Fatalf("default native provider error = %v, want capability-unavailable when device is absent", err)
		}
		return
	}
	if err := CompareGPUValues(got, mustGPUValue(t, []float64{8, 15}, []int{2}), GPUComparisonTolerance{Absolute: 1e-5, Relative: 1e-5}); err != nil {
		t.Fatal(err)
	}
}

func TestNativeGPUProviderLifecycleWithDeterministicAdapter(t *testing.T) {
	backend := &deterministicNativeGPUBackend{buffers: make(map[uintptr][]float64)}
	provider := &NativeGPUProvider{
		limits:  defaultGPULimits(),
		backend: backend,
		device:  GPUDevice{ID: "fake-native-0", Kind: "fake-native", Capabilities: []GPUCapability{GPUCapabilityNative, GPUCapabilityFloat32}, MaxElements: 64, MaxBufferBytes: 512, MaxRank: 4},
	}
	device, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{})
	if err != nil {
		t.Fatal(err)
	}
	left := mustGPUValue(t, []float64{1, 2}, []int{2})
	right := mustGPUValue(t, []float64{3, 4}, []int{2})
	leftBuffer, err := provider.Allocate(context.Background(), device, left)
	if err != nil {
		t.Fatal(err)
	}
	rightBuffer, err := provider.Allocate(context.Background(), device, right)
	if err != nil {
		t.Fatal(err)
	}
	foreign := newGPUBuffer("foreign", left)
	kernel := newGPUKernelForOp("test", "test", GPUKernelAdd, 2)
	if _, _, err := provider.Launch(context.Background(), device, kernel, []*GPUBuffer{foreign, rightBuffer}); !errors.Is(err, ErrGPUOwnership) {
		t.Fatalf("foreign native launch = %v, want ownership error", err)
	}
	output, event, err := provider.Launch(context.Background(), device, kernel, []*GPUBuffer{leftBuffer, rightBuffer})
	if err != nil {
		t.Fatal(err)
	}
	if err := event.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	got, err := output.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if err := CompareGPUValues(got, mustGPUValue(t, []float64{4, 6}, []int{2}), GPUComparisonTolerance{}); err != nil {
		t.Fatal(err)
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := provider.Allocate(cancelled, device, left); !errors.Is(err, ErrGPUCancelled) {
		t.Fatalf("cancelled allocation = %v, want cancellation", err)
	}
	if err := provider.Close(); err != nil {
		t.Fatal(err)
	}
	if backend.closed {
		t.Fatal("provider closed device while owned buffers remained")
	}
	_ = leftBuffer.Release()
	_ = rightBuffer.Release()
	_ = output.Release()
	if !backend.closed {
		t.Fatal("provider did not close device after final buffer release")
	}
	if err := provider.Close(); err != nil {
		t.Fatal("repeated provider close: ", err)
	}
	if backend.closeCalls != 1 {
		t.Fatalf("fake device close calls = %d, want exactly one", backend.closeCalls)
	}
	if backend.releaseBuffers != 3 {
		t.Fatalf("fake device released buffers = %d, want three", backend.releaseBuffers)
	}
}

func TestNativeGPUProviderFakeCapabilityAndABIBoundaries(t *testing.T) {
	backend := &deterministicNativeGPUBackend{buffers: make(map[uintptr][]float64)}
	provider := &NativeGPUProvider{
		limits:  defaultGPULimits(),
		backend: backend,
		device: GPUDevice{
			ID: "fake-native-abi-0", Kind: "fake-native",
			Capabilities: []GPUCapability{GPUCapabilityNative, GPUCapabilityFloat32, GPUCapabilityMath},
			MaxElements:  8, MaxBufferBytes: 64, MaxRank: 2,
		},
	}
	defer provider.Close()

	device, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{Required: []GPUCapability{GPUCapabilityNative, GPUCapabilityMath}})
	if err != nil {
		t.Fatalf("select supported fake capabilities: %v", err)
	}
	if _, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{Required: []GPUCapability{GPUCapabilityReductions}}); err == nil || !errors.Is(err, ErrGPUCapabilityUnavailable) {
		t.Fatalf("select unsupported fake capability = %v, want capability-unavailable", err)
	}
	if _, err := provider.SelectDevice(context.Background(), GPUDeviceRequest{MaxElements: 9}); err == nil || !errors.Is(err, ErrGPUResourceLimit) {
		t.Fatalf("select over-limit fake device = %v, want resource-limit", err)
	}

	base := testGPUArtifact(GPUKernelAdd, 2)
	badInputOwnership := base
	badInputOwnership.Kernels = append([]GPUKernel(nil), base.Kernels...)
	badInputOwnership.Kernels[0].Inputs = append([]GPUValueSpec(nil), base.Kernels[0].Inputs...)
	badInputOwnership.Kernels[0].Inputs[0].Ownership = GPUOwned
	if err := ValidateGPUArtifact(badInputOwnership); err == nil || !errors.Is(err, ErrGPUOwnership) {
		t.Fatalf("owned fake kernel input = %v, want ownership error", err)
	}

	badOutputOwnership := base
	badOutputOwnership.Kernels = append([]GPUKernel(nil), base.Kernels...)
	badOutputOwnership.Kernels[0].Output.Ownership = GPUBorrowed
	if err := ValidateGPUArtifact(badOutputOwnership); err == nil || !errors.Is(err, ErrGPUOwnership) {
		t.Fatalf("borrowed fake kernel output = %v, want ownership error", err)
	}

	badInputABI := base
	badInputABI.Kernels = append([]GPUKernel(nil), base.Kernels...)
	badInputABI.Kernels[0].Inputs = append([]GPUValueSpec(nil), base.Kernels[0].Inputs...)
	badInputABI.Kernels[0].Inputs[0].ABI++
	if err := ValidateGPUArtifact(badInputABI); err == nil || !errors.Is(err, ErrGPUArtifactMalformed) {
		t.Fatalf("unsupported fake kernel input ABI = %v, want malformed error", err)
	}

	if _, _, err := provider.Launch(context.Background(), device, newGPUKernelForOp("unsupported", "test", GPUKernelSum, 1), nil); err == nil || !errors.Is(err, ErrGPUCapabilityUnavailable) {
		t.Fatalf("unsupported fake kernel launch = %v, want capability-unavailable", err)
	}

	reference := NewDeterministicCPUProvider(defaultGPULimits())
	devices, err := reference.Devices(context.Background())
	if err != nil {
		t.Fatalf("reference devices: %v", err)
	}
	if len(devices) != 1 || devices[0].Kind != "cpu-reference" || devices[0].ID != "cpu-reference-0" {
		t.Fatalf("reference device identity = %#v", devices)
	}
	if gpuHasCapability(devices[0].Capabilities, GPUCapabilityNative) || !gpuHasCapability(devices[0].Capabilities, GPUCapabilityDeterministic) {
		t.Fatalf("reference device capabilities = %#v", devices[0].Capabilities)
	}
}

type deterministicNativeGPUBackend struct {
	buffers        map[uintptr][]float64
	next           uintptr
	closed         bool
	closeCalls     int
	releaseBuffers int
}

func (b *deterministicNativeGPUBackend) close() error {
	b.closed = true
	b.closeCalls++
	return nil
}

func (b *deterministicNativeGPUBackend) allocate(data []float64) (uintptr, error) {
	b.next++
	b.buffers[b.next] = append([]float64(nil), data...)
	return b.next, nil
}

func (b *deterministicNativeGPUBackend) launch(op GPUKernelOp, inputs []nativeGPUInput, outputShape []int, outputCount int) (uintptr, uintptr, error) {
	b.next++
	result := make([]float64, outputCount)
	for i := range result {
		result[i] = b.buffers[inputs[0].handle][i%inputs[0].count] + b.buffers[inputs[1].handle][i%inputs[1].count]
	}
	b.buffers[b.next] = result
	return b.next, b.next + 1000, nil
}

func (b *deterministicNativeGPUBackend) wait(uintptr) error { return nil }

func (b *deterministicNativeGPUBackend) read(handle uintptr, shape []int, count int) (GPUValue, error) {
	return NewGPUValue(append([]float64(nil), b.buffers[handle]...), shape)
}

func (b *deterministicNativeGPUBackend) releaseBuffer(handle uintptr) {
	delete(b.buffers, handle)
	b.releaseBuffers++
}

func (b *deterministicNativeGPUBackend) releaseEvent(uintptr) {}

func (b *deterministicNativeGPUBackend) supports(op GPUKernelOp) bool { return op == GPUKernelAdd }

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
