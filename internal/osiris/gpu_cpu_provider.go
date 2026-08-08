package osiris

import (
	"context"
)

// CPUReferenceProvider is deliberately single-device and single-ordered. It
// supplies a deterministic execution reference for hosts without GPU drivers
// and is also the conformance oracle for later hardware providers.
type CPUReferenceProvider struct {
	limits GPULimits
	device GPUDevice
}

func NewDeterministicCPUProvider(limits GPULimits) *CPUReferenceProvider {
	limits = limits.normalized()
	return &CPUReferenceProvider{
		limits: limits,
		device: GPUDevice{
			ID:             "cpu-reference-0",
			Name:           "Osiris deterministic CPU reference",
			Kind:           "cpu-reference",
			Capabilities:   []GPUCapability{GPUCapabilityFloat64, GPUCapabilityTensor, GPUCapabilityDeterministic},
			MaxElements:    limits.MaxElements,
			MaxBufferBytes: limits.MaxBufferBytes,
			MaxRank:        limits.MaxRank,
		},
	}
}

func NewCPUReferenceProvider(limits GPULimits) *CPUReferenceProvider {
	return NewDeterministicCPUProvider(limits)
}

func (p *CPUReferenceProvider) Devices(ctx context.Context) ([]GPUDevice, error) {
	if err := gpuContextError(ctx, "devices"); err != nil {
		return nil, err
	}
	return []GPUDevice{cloneGPUDevice(p.device)}, nil
}

func (p *CPUReferenceProvider) SelectDevice(ctx context.Context, request GPUDeviceRequest) (GPUDevice, error) {
	if err := gpuContextError(ctx, "select"); err != nil {
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

func (p *CPUReferenceProvider) Allocate(ctx context.Context, device GPUDevice, value GPUValue) (*GPUBuffer, error) {
	if err := gpuContextError(ctx, "allocate"); err != nil {
		return nil, err
	}
	if device.ID != p.device.ID {
		return nil, newGPUError(ErrGPUCapabilityUnavailable, "allocate", "device does not belong to provider", nil)
	}
	if err := ValidateGPUValue(value, p.limits); err != nil {
		return nil, err
	}
	return newGPUBuffer(p.device.ID, value), nil
}

func (p *CPUReferenceProvider) Launch(ctx context.Context, device GPUDevice, kernel GPUKernel, inputs []*GPUBuffer) (*GPUBuffer, *GPUEvent, error) {
	if err := gpuContextError(ctx, "launch"); err != nil {
		return nil, nil, err
	}
	if device.ID != p.device.ID {
		return nil, nil, newGPUError(ErrGPUCapabilityUnavailable, "launch", "device does not belong to provider", nil)
	}
	if err := validateCPUKernel(kernel); err != nil {
		return nil, nil, err
	}
	if !gpuKernelArityValid(kernel.Op, len(inputs)) {
		return nil, nil, newGPUError(ErrGPUArtifactMalformed, "launch", "CPU reference kernel input count is invalid", nil)
	}
	values := make([]GPUValue, len(inputs))
	for i, input := range inputs {
		if input == nil || input.owner != p.device.ID {
			return nil, nil, newGPUError(ErrGPUOwnership, "launch", "input buffer is not owned by this provider", nil)
		}
		value, err := input.Read(ctx)
		if err != nil {
			return nil, nil, err
		}
		if err := ValidateGPUValue(value, p.limits); err != nil {
			return nil, nil, err
		}
		values[i] = value
	}
	outputShape := values[0].Shape
	if len(values) > 1 {
		var err error
		outputShape, err = gpuOutputShape(kernel.Op, values[0].Shape, values[1].Shape)
		if err != nil {
			return nil, nil, err
		}
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
	buffer := newGPUBuffer(p.device.ID, output)
	event := newGPUEvent()
	buffer.setEvent(event)
	go func() {
		result, runErr := runCPUKernel(ctx, kernel.Op, values, outputShape, p.limits)
		if runErr == nil {
			buffer.replace(result)
		}
		event.complete(runErr)
	}()
	return buffer, event, nil
}

func validateCPUKernel(kernel GPUKernel) error {
	if kernel.ABIVersion != GPUKernelABIVersion || !gpuKernelArityValid(kernel.Op, len(kernel.Inputs)) || kernel.Output.ABI != GPUValueABIVersion || kernel.Output.Type != GPUValueFloat64 || kernel.Output.Ownership != GPUOwned {
		return newGPUError(ErrGPUArtifactMalformed, "launch", "kernel ABI is malformed", nil)
	}
	for _, input := range kernel.Inputs {
		if input.ABI != GPUValueABIVersion || input.Type != GPUValueFloat64 || input.Ownership != GPUBorrowed {
			return newGPUError(ErrGPUOwnership, "launch", "kernel inputs must be borrowed f64 values", nil)
		}
	}
	if !gpuKernelSupported(kernel.Op) {
		return newGPUError(ErrGPUCapabilityUnavailable, "launch", "CPU reference provider does not implement "+string(kernel.Op), nil)
	}
	return nil
}

func runCPUKernel(ctx context.Context, op GPUKernelOp, values []GPUValue, outputShape []int, limits GPULimits) (GPUValue, error) {
	return runGPUReferenceKernel(ctx, op, values, outputShape, limits)
}

func gpuOutputShape(op GPUKernelOp, left, right []int) ([]int, error) {
	switch op {
	case GPUKernelAdd, GPUKernelSub, GPUKernelMul, GPUKernelDiv, GPUKernelPow, GPUKernelHypot:
		return gpuBroadcastShape(left, right)
	case GPUKernelDot:
		if len(left) > 0 && len(right) > 0 && (len(left) != 1 || len(right) != 1) {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "dot requires vectors", nil)
		}
		return nil, nil
	case GPUKernelMatMul:
		if len(left) != 2 || len(right) != 2 || left[1] != right[0] {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "matmul shapes are incompatible", nil)
		}
		return []int{left[0], right[1]}, nil
	case GPUKernelAbs, GPUKernelSqrt, GPUKernelExp, GPUKernelLog, GPUKernelLog10, GPUKernelLog2,
		GPUKernelSin, GPUKernelCos, GPUKernelTan, GPUKernelAsin, GPUKernelAcos, GPUKernelAtan,
		GPUKernelFloor, GPUKernelCeil, GPUKernelRound, GPUKernelTranspose:
		return append([]int(nil), left...), nil
	case GPUKernelFlatten, GPUKernelRange:
		if op == GPUKernelFlatten {
			return []int{tensorShapeProduct(left)}, nil
		}
		return nil, nil
	case GPUKernelShape:
		return []int{len(left)}, nil
	case GPUKernelSum, GPUKernelProduct, GPUKernelMean, GPUKernelStd, GPUKernelNorm, GPUKernelMin, GPUKernelMax,
		GPUKernelClamp, GPUKernelHann, GPUKernelConvolve,
		GPUKernelVector, GPUKernelZeros, GPUKernelOnes, GPUKernelFull, GPUKernelEye, GPUKernelArange,
		GPUKernelLinspace, GPUKernelLogspace, GPUKernelReshape:
		return nil, nil
	default:
		return nil, newGPUError(ErrGPUCapabilityUnavailable, "launch", "unknown kernel operation "+string(op), nil)
	}
}

func gpuBroadcastShape(left, right []int) ([]int, error) {
	width := len(left)
	if len(right) > width {
		width = len(right)
	}
	if width == 0 {
		return nil, nil
	}
	out := make([]int, width)
	for i := 0; i < width; i++ {
		li, ri := 1, 1
		if idx := len(left) - 1 - i; idx >= 0 {
			li = left[idx]
		}
		if idx := len(right) - 1 - i; idx >= 0 {
			ri = right[idx]
		}
		if li != ri && li != 1 && ri != 1 {
			return nil, newGPUError(ErrGPUArtifactMalformed, "launch", "elementwise shapes are incompatible", nil)
		}
		if li > ri {
			out[width-1-i] = li
		} else {
			out[width-1-i] = ri
		}
	}
	return out, nil
}

func gpuBroadcastIndex(flat int, outputShape, inputShape []int) int {
	if len(inputShape) == 0 {
		return 0
	}
	inputStrides := tensorShapeStrides(inputShape)
	outputStrides := tensorShapeStrides(outputShape)
	coords := tensorFlatToCoords(flat, outputShape, outputStrides)
	return tensorBroadcastIndexFromCoords(coords, inputShape, inputStrides)
}

func gpuContextError(ctx context.Context, stage string) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return newGPUError(ErrGPUCancelled, stage, "context cancelled", err)
	}
	return nil
}

func gpuHasCapability(capabilities []GPUCapability, required GPUCapability) bool {
	for _, capability := range capabilities {
		if capability == required {
			return true
		}
	}
	return false
}

func cloneGPUDevice(device GPUDevice) GPUDevice {
	device.Capabilities = append([]GPUCapability(nil), device.Capabilities...)
	return device
}
