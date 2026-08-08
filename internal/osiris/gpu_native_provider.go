package osiris

import (
	"context"
	"sync"
)

// NativeGPUProvider is a device-backed provider. Its platform adapter owns
// the device and buffer handles; the Go runtime owns the public lifecycle and
// never exposes those handles to language values.
type NativeGPUProvider struct {
	mu           sync.RWMutex
	limits       GPULimits
	device       GPUDevice
	backend      nativeGPUBackend
	closed       bool
	deviceClosed bool
	refs         int
}

type nativeGPUInput struct {
	handle uintptr
	count  int
	shape  []int
}

type nativeGPUBackend interface {
	close() error
	allocate([]float64) (uintptr, error)
	launch(GPUKernelOp, []nativeGPUInput, []int, int) (uintptr, uintptr, error)
	wait(uintptr) error
	read(uintptr, []int, int) (GPUValue, error)
	releaseBuffer(uintptr)
	releaseEvent(uintptr)
	supports(GPUKernelOp) bool
}

// NewNativeGPUProvider discovers the host's real GPU API. It never creates a
// CPU reference device. Hosts without a supported adapter return
// ErrGPUCapabilityUnavailable so callers can make an explicit policy choice.
func NewNativeGPUProvider(limits GPULimits) (*NativeGPUProvider, error) {
	limits = limits.normalized()
	backend, device, err := newNativeGPUBackend(limits)
	if err != nil {
		return nil, err
	}
	return &NativeGPUProvider{limits: limits, device: device, backend: backend}, nil
}

// NewMetalProvider is the explicit name for the currently supported native
// adapter. On non-macOS hosts it returns ErrGPUCapabilityUnavailable.
func NewMetalProvider(limits GPULimits) (*NativeGPUProvider, error) {
	return NewNativeGPUProvider(limits)
}

func (p *NativeGPUProvider) Devices(ctx context.Context) ([]GPUDevice, error) {
	if err := gpuContextError(ctx, "devices"); err != nil {
		return nil, err
	}
	if err := p.checkOpen("devices"); err != nil {
		return nil, err
	}
	return []GPUDevice{cloneGPUDevice(p.device)}, nil
}

func (p *NativeGPUProvider) SelectDevice(ctx context.Context, request GPUDeviceRequest) (GPUDevice, error) {
	if err := gpuContextError(ctx, "select"); err != nil {
		return GPUDevice{}, err
	}
	if err := p.checkOpen("select"); err != nil {
		return GPUDevice{}, err
	}
	if request.ID != "" && request.ID != p.device.ID {
		return GPUDevice{}, newGPUError(ErrGPUCapabilityUnavailable, "select", "requested device is unavailable: "+request.ID, nil)
	}
	for _, required := range request.Required {
		if !gpuHasCapability(p.device.Capabilities, required) {
			return GPUDevice{}, newGPUError(ErrGPUCapabilityUnavailable, "select", "device lacks capability "+string(required), nil)
		}
	}
	if request.MaxElements > 0 && request.MaxElements > p.device.MaxElements {
		return GPUDevice{}, newGPUError(ErrGPUResourceLimit, "select", "requested element limit exceeds device limit", nil)
	}
	if request.MaxBufferBytes > 0 && request.MaxBufferBytes > p.device.MaxBufferBytes {
		return GPUDevice{}, newGPUError(ErrGPUResourceLimit, "select", "requested buffer limit exceeds device limit", nil)
	}
	if request.MaxRank > 0 && request.MaxRank > p.device.MaxRank {
		return GPUDevice{}, newGPUError(ErrGPUResourceLimit, "select", "requested rank limit exceeds device limit", nil)
	}
	return cloneGPUDevice(p.device), nil
}

func (p *NativeGPUProvider) Allocate(ctx context.Context, device GPUDevice, value GPUValue) (*GPUBuffer, error) {
	if err := gpuContextError(ctx, "allocate"); err != nil {
		return nil, err
	}
	if err := p.checkDevice(device, "allocate"); err != nil {
		return nil, err
	}
	if err := ValidateGPUValue(value, p.limits); err != nil {
		return nil, err
	}
	if len(value.Data) == 0 {
		return nil, newGPUError(ErrGPUCapabilityUnavailable, "allocate", "native provider does not allocate zero-element buffers", nil)
	}
	if !p.retainBuffer() {
		return nil, newGPUError(ErrGPUOwnership, "allocate", "native GPU provider closed during allocation", nil)
	}
	handle, err := p.backend.allocate(value.Data)
	if err != nil {
		p.dropBufferRef()
		return nil, newGPUError(ErrGPUExecution, "allocate", "device buffer allocation failed", err)
	}
	read := func(readCtx context.Context) (GPUValue, error) {
		if err := gpuContextError(readCtx, "read"); err != nil {
			return GPUValue{}, err
		}
		return p.backend.read(handle, value.Shape, len(value.Data))
	}
	return newNativeGPUBuffer(p.device.ID, value, handle, read, func() { p.releaseBuffer(handle) }), nil
}

func (p *NativeGPUProvider) Launch(ctx context.Context, device GPUDevice, kernel GPUKernel, inputs []*GPUBuffer) (*GPUBuffer, *GPUEvent, error) {
	if err := gpuContextError(ctx, "launch"); err != nil {
		return nil, nil, err
	}
	if err := p.checkDevice(device, "launch"); err != nil {
		return nil, nil, err
	}
	if err := validateCPUKernel(kernel); err != nil {
		return nil, nil, err
	}
	if !p.backend.supports(kernel.Op) {
		return nil, nil, newGPUError(ErrGPUCapabilityUnavailable, "launch", "native provider does not implement "+string(kernel.Op), nil)
	}
	if !gpuKernelArityValid(kernel.Op, len(inputs)) {
		return nil, nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native kernel input count is invalid", nil)
	}
	nativeInputs := make([]nativeGPUInput, len(inputs))
	values := make([]GPUValue, len(inputs))
	for i, input := range inputs {
		if err := input.Synchronize(ctx); err != nil {
			return nil, nil, err
		}
		handle, err := input.nativeFor(p.device.ID)
		if err != nil {
			return nil, nil, err
		}
		value, err := input.launchValue()
		if err != nil {
			return nil, nil, err
		}
		nativeInputs[i] = nativeGPUInput{handle: handle, count: len(value.Data), shape: append([]int(nil), value.Shape...)}
		values[i] = value
	}
	if len(values) == 0 {
		return nil, nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native kernel requires at least one input", nil)
	}
	outputShape, err := nativeGPUOutputShape(kernel.Op, values)
	if err != nil {
		return nil, nil, err
	}
	outputElements, err := gpuShapeElements(outputShape)
	if err != nil {
		return nil, nil, err
	}
	if len(outputShape) == 0 {
		outputElements = 1
	}
	if outputElements > p.limits.MaxElements || outputElements > p.limits.MaxBufferBytes/8 {
		return nil, nil, newGPUError(ErrGPUResourceLimit, "launch", "output exceeds provider resource limit", nil)
	}
	output, err := NewGPUValue(make([]float64, outputElements), outputShape)
	if err != nil {
		return nil, nil, err
	}
	if !p.retainBuffer() {
		return nil, nil, newGPUError(ErrGPUOwnership, "launch", "native GPU provider closed during launch", nil)
	}
	outputHandle, nativeEvent, err := p.backend.launch(kernel.Op, nativeInputs, outputShape, outputElements)
	if err != nil {
		p.dropBufferRef()
		return nil, nil, newGPUError(ErrGPUExecution, "launch", "device kernel launch failed", err)
	}
	event := newGPUEvent()
	go func() {
		waitErr := p.backend.wait(nativeEvent)
		p.backend.releaseEvent(nativeEvent)
		if waitErr != nil {
			waitErr = newGPUError(ErrGPUExecution, "synchronize", "device command failed", waitErr)
		}
		event.complete(waitErr)
	}()
	read := func(readCtx context.Context) (GPUValue, error) {
		if err := gpuContextError(readCtx, "read"); err != nil {
			return GPUValue{}, err
		}
		return p.backend.read(outputHandle, output.Shape, len(output.Data))
	}
	buffer := newNativeGPUBuffer(p.device.ID, output, outputHandle, read, func() { p.releaseBuffer(outputHandle) })
	buffer.setEvent(event)
	return buffer, event, nil
}

func (p *NativeGPUProvider) Close() error {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return nil
	}
	p.closed = true
	shouldClose := p.refs == 0
	if shouldClose {
		p.deviceClosed = true
	}
	p.mu.Unlock()
	if shouldClose {
		return p.backend.close()
	}
	return nil
}

func (p *NativeGPUProvider) checkOpen(stage string) error {
	if p == nil || p.backend == nil {
		return newGPUError(ErrGPUCapabilityUnavailable, stage, "native GPU provider is unavailable", nil)
	}
	p.mu.RLock()
	closed := p.closed
	p.mu.RUnlock()
	if closed {
		return newGPUError(ErrGPUOwnership, stage, "native GPU provider is closed", nil)
	}
	return nil
}

func (p *NativeGPUProvider) retainBuffer() bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.closed || p.deviceClosed {
		return false
	}
	p.refs++
	return true
}

func (p *NativeGPUProvider) releaseBuffer(handle uintptr) {
	p.backend.releaseBuffer(handle)
	p.dropBufferRef()
}

func (p *NativeGPUProvider) dropBufferRef() {
	p.mu.Lock()
	if p.refs > 0 {
		p.refs--
	}
	shouldClose := p.closed && !p.deviceClosed && p.refs == 0
	if shouldClose {
		p.deviceClosed = true
	}
	p.mu.Unlock()
	if shouldClose {
		_ = p.backend.close()
	}
}

func (p *NativeGPUProvider) checkDevice(device GPUDevice, stage string) error {
	if err := p.checkOpen(stage); err != nil {
		return err
	}
	if device.ID != p.device.ID {
		return newGPUError(ErrGPUCapabilityUnavailable, stage, "device does not belong to provider", nil)
	}
	return nil
}

func (b *GPUBuffer) launchValue() (GPUValue, error) {
	if b == nil {
		return GPUValue{}, newGPUError(ErrGPUOwnership, "launch", "nil input buffer", nil)
	}
	b.mu.RLock()
	defer b.mu.RUnlock()
	if b.released {
		return GPUValue{}, newGPUError(ErrGPUOwnership, "launch", "input buffer was released", nil)
	}
	return cloneGPUValue(b.value), nil
}

var _ GPUProvider = (*NativeGPUProvider)(nil)

func nativeGPUOutputShape(op GPUKernelOp, values []GPUValue) ([]int, error) {
	if len(values) == 0 {
		return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native kernel requires at least one input", nil)
	}
	switch op {
	case GPUKernelConvolve:
		if len(values) != 2 || len(values[0].Shape) != 1 || len(values[1].Shape) != 1 {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native convolution requires rank-one inputs", nil)
		}
		return []int{len(values[0].Data) + len(values[1].Data) - 1}, nil
	case GPUKernelHann:
		if len(values) != 1 || len(values[0].Data) != 1 {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native hann requires one scalar length", nil)
		}
		n := int(values[0].Data[0])
		if float64(n) != values[0].Data[0] || n < 2 {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "native hann length must be an integer at least two", nil)
		}
		return []int{n}, nil
	case GPUKernelDot, GPUKernelSum, GPUKernelProduct, GPUKernelMean, GPUKernelStd, GPUKernelNorm, GPUKernelMin, GPUKernelMax:
		return nil, nil
	default:
		left := values[0].Shape
		var right []int
		if len(values) > 1 {
			right = values[1].Shape
		}
		return gpuOutputShape(op, left, right)
	}
}
