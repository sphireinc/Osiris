package osiris

import (
	"context"
	"fmt"
)

func gpuKernelSupported(op GPUKernelOp) bool {
	switch op {
	case GPUKernelAdd, GPUKernelSub, GPUKernelMul, GPUKernelDiv, GPUKernelDot, GPUKernelMatMul,
		GPUKernelVector, GPUKernelZeros, GPUKernelOnes, GPUKernelFull, GPUKernelEye,
		GPUKernelTensor, GPUKernelMatrix,
		GPUKernelArange, GPUKernelLinspace, GPUKernelLogspace, GPUKernelRange,
		GPUKernelShape, GPUKernelFlatten, GPUKernelReshape, GPUKernelTranspose,
		GPUKernelSum, GPUKernelProduct, GPUKernelMean, GPUKernelStd, GPUKernelNorm,
		GPUKernelMin, GPUKernelMax, GPUKernelAbs, GPUKernelSqrt, GPUKernelPow,
		GPUKernelExp, GPUKernelLog, GPUKernelLog10, GPUKernelLog2, GPUKernelSin,
		GPUKernelCos, GPUKernelTan, GPUKernelAsin, GPUKernelAcos, GPUKernelAtan,
		GPUKernelFloor, GPUKernelCeil, GPUKernelRound, GPUKernelClamp, GPUKernelHypot,
		GPUKernelHann, GPUKernelConvolve:
		return true
	default:
		return false
	}
}

// runGPUReferenceKernel is a deterministic reference implementation of the
// versioned GPU operations. It calls pure value-model functions for the
// constructor, reduction, and signal contracts; it never evaluates source or
// invokes another backend.
func runGPUReferenceKernel(ctx context.Context, op GPUKernelOp, values []GPUValue, outputShape []int, limits GPULimits) (GPUValue, error) {
	if err := gpuContextError(ctx, "execute"); err != nil {
		return GPUValue{}, err
	}
	if op == GPUKernelAdd || op == GPUKernelSub || op == GPUKernelMul || op == GPUKernelDiv {
		return runGPUElementwise(ctx, op, values, outputShape, limits)
	}
	args := make([]Value, len(values))
	for i, value := range values {
		converted, err := value.ToValue()
		if err != nil {
			return GPUValue{}, err
		}
		args[i] = converted
	}

	var result Value
	var err error
	switch op {
	case GPUKernelDot:
		result, err = tensorDotValue(args)
	case GPUKernelMatMul:
		result, err = tensorMatMulValue(args)
	case GPUKernelVector:
		result, err = vectorValueFromArgs(args)
	case GPUKernelTensor:
		result, err = tensorValueFromArgs(args)
	case GPUKernelMatrix:
		result, err = matrixValueFromArgs(args)
	case GPUKernelZeros:
		result, err = zerosValue(args)
	case GPUKernelOnes:
		result, err = onesValue(args)
	case GPUKernelFull:
		result, err = fullValue(args)
	case GPUKernelEye:
		result, err = eyeValue(args)
	case GPUKernelArange:
		result, err = arangeValue(args)
	case GPUKernelLinspace:
		result, err = linspaceValue(args)
	case GPUKernelLogspace:
		result, err = logspaceValue(args)
	case GPUKernelRange:
		result, err = rangeValue(args)
	case GPUKernelShape:
		result, err = tensorShapeValue(args)
	case GPUKernelFlatten:
		result, err = tensorFlattenValue(args)
	case GPUKernelReshape:
		result, err = tensorReshapeValue(args)
	case GPUKernelTranspose:
		result, err = tensorTransposeValue(args)
	case GPUKernelSum:
		result, err = sumValue(args)
	case GPUKernelProduct:
		result, err = productValue(args)
	case GPUKernelMean:
		result, err = meanValue(args)
	case GPUKernelStd:
		result, err = stdValue(args)
	case GPUKernelNorm:
		result, err = normValue(args)
	case GPUKernelMin:
		result, err = minValue(args)
	case GPUKernelMax:
		result, err = maxValue(args)
	case GPUKernelPow, GPUKernelClamp, GPUKernelHypot:
		result, err = runGPUScalarMath(op, args)
	case GPUKernelHann:
		result, err = hannValue(args)
	case GPUKernelConvolve:
		result, err = convolveValue(args)
	case GPUKernelAbs, GPUKernelSqrt, GPUKernelExp, GPUKernelLog, GPUKernelLog10, GPUKernelLog2,
		GPUKernelSin, GPUKernelCos, GPUKernelTan, GPUKernelAsin, GPUKernelAcos, GPUKernelAtan,
		GPUKernelFloor, GPUKernelCeil, GPUKernelRound:
		return runGPUUnary(ctx, op, values, limits)
	default:
		return GPUValue{}, newGPUError(ErrGPUCapabilityUnavailable, "execute", "unsupported CPU operation "+string(op), nil)
	}
	if err != nil {
		return GPUValue{}, newGPUError(ErrGPUExecution, "execute", string(op), err)
	}
	return gpuValueFromReference(result, limits)
}

func runGPUElementwise(ctx context.Context, op GPUKernelOp, values []GPUValue, outputShape []int, limits GPULimits) (GPUValue, error) {
	if len(values) != 2 {
		return GPUValue{}, newGPUError(ErrGPUArtifactMalformed, "execute", "elementwise operation requires two inputs", nil)
	}
	if len(outputShape) == 0 {
		outputShape = nil
	}
	out := make([]float64, 1)
	if outputShape != nil {
		out = make([]float64, tensorShapeProduct(outputShape))
	}
	for i := range out {
		if err := gpuContextError(ctx, "execute"); err != nil {
			return GPUValue{}, err
		}
		left := values[0].Data[gpuBroadcastIndex(i, outputShape, values[0].Shape)]
		right := values[1].Data[gpuBroadcastIndex(i, outputShape, values[1].Shape)]
		switch op {
		case GPUKernelAdd:
			out[i] = left + right
		case GPUKernelSub:
			out[i] = left - right
		case GPUKernelMul:
			out[i] = left * right
		case GPUKernelDiv:
			if right == 0 {
				return GPUValue{}, newGPUError(ErrGPUExecution, "execute", "division by zero", nil)
			}
			out[i] = left / right
		}
	}
	return gpuValueFromData(out, outputShape, limits)
}

func runGPUUnary(ctx context.Context, op GPUKernelOp, values []GPUValue, limits GPULimits) (GPUValue, error) {
	if len(values) != 1 {
		return GPUValue{}, newGPUError(ErrGPUArtifactMalformed, "execute", "unary operation requires one input", nil)
	}
	out := make([]float64, len(values[0].Data))
	for i, value := range values[0].Data {
		if err := gpuContextError(ctx, "execute"); err != nil {
			return GPUValue{}, err
		}
		result, err := runGPUScalarMath(op, []Value{numberValue(value)})
		if err != nil {
			return GPUValue{}, newGPUError(ErrGPUExecution, "execute", string(op), err)
		}
		out[i], err = result.AsNumber()
		if err != nil {
			return GPUValue{}, newGPUError(ErrGPUExecution, "execute", string(op), err)
		}
	}
	return gpuValueFromData(out, values[0].Shape, limits)
}

func runGPUScalarMath(op GPUKernelOp, args []Value) (Value, error) {
	switch op {
	case GPUKernelAbs:
		return builtinAbs(args)
	case GPUKernelSqrt:
		return builtinSqrt(args)
	case GPUKernelPow:
		return builtinPow(args)
	case GPUKernelExp:
		return builtinExp(args)
	case GPUKernelLog:
		return builtinLog(args)
	case GPUKernelLog10:
		return builtinLog10(args)
	case GPUKernelLog2:
		return builtinLog2(args)
	case GPUKernelSin:
		return builtinSin(args)
	case GPUKernelCos:
		return builtinCos(args)
	case GPUKernelTan:
		return builtinTan(args)
	case GPUKernelAsin:
		return builtinAsin(args)
	case GPUKernelAcos:
		return builtinAcos(args)
	case GPUKernelAtan:
		return builtinAtan(args)
	case GPUKernelFloor:
		return builtinFloor(args)
	case GPUKernelCeil:
		return builtinCeil(args)
	case GPUKernelRound:
		return builtinRound(args)
	case GPUKernelClamp:
		return builtinClamp(args)
	case GPUKernelHypot:
		return builtinHypot(args)
	default:
		return nullValue(), fmt.Errorf("unsupported scalar GPU operation %s", op)
	}
}

func gpuValueFromReference(value Value, limits GPULimits) (GPUValue, error) {
	result, err := GPUValueFromValue(value)
	if err != nil {
		return GPUValue{}, newGPUError(ErrGPUCapabilityUnavailable, "execute", "reference result is outside the f64 ABI", err)
	}
	if err := ValidateGPUValue(result, limits); err != nil {
		return GPUValue{}, err
	}
	return result, nil
}

func gpuValueFromData(data []float64, shape []int, limits GPULimits) (GPUValue, error) {
	result, err := NewGPUValue(data, shape)
	if err != nil {
		return GPUValue{}, err
	}
	if err := ValidateGPUValue(result, limits); err != nil {
		return GPUValue{}, err
	}
	return result, nil
}
