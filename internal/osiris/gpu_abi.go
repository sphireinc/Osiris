package osiris

import (
	"errors"
	"fmt"
	"math"
)

// GPU ABI versions are independent so a provider can reject a value or kernel
// ABI it does not understand without guessing from the artifact schema.
const (
	GPUKernelABIVersion   = 1
	GPUValueABIVersion    = 1
	GPUArtifactABIVersion = 1

	gpuMaxArtifactBytes = 8 << 20
	gpuMaxSourceBytes   = 4 << 20
)

type GPUKernelOp string

const (
	GPUKernelAdd       GPUKernelOp = "elementwise.add"
	GPUKernelSub       GPUKernelOp = "elementwise.sub"
	GPUKernelMul       GPUKernelOp = "elementwise.mul"
	GPUKernelDiv       GPUKernelOp = "elementwise.div"
	GPUKernelDot       GPUKernelOp = "dot"
	GPUKernelMatMul    GPUKernelOp = "matmul"
	GPUKernelVector    GPUKernelOp = "constructor.vector"
	GPUKernelTensor    GPUKernelOp = "constructor.tensor"
	GPUKernelMatrix    GPUKernelOp = "constructor.matrix"
	GPUKernelZeros     GPUKernelOp = "constructor.zeros"
	GPUKernelOnes      GPUKernelOp = "constructor.ones"
	GPUKernelFull      GPUKernelOp = "constructor.full"
	GPUKernelEye       GPUKernelOp = "constructor.eye"
	GPUKernelArange    GPUKernelOp = "constructor.arange"
	GPUKernelLinspace  GPUKernelOp = "constructor.linspace"
	GPUKernelLogspace  GPUKernelOp = "constructor.logspace"
	GPUKernelRange     GPUKernelOp = "range"
	GPUKernelShape     GPUKernelOp = "shape"
	GPUKernelFlatten   GPUKernelOp = "shape.flatten"
	GPUKernelReshape   GPUKernelOp = "shape.reshape"
	GPUKernelTranspose GPUKernelOp = "shape.transpose"
	GPUKernelSum       GPUKernelOp = "reduction.sum"
	GPUKernelProduct   GPUKernelOp = "reduction.product"
	GPUKernelMean      GPUKernelOp = "reduction.mean"
	GPUKernelStd       GPUKernelOp = "reduction.std"
	GPUKernelNorm      GPUKernelOp = "reduction.norm"
	GPUKernelMin       GPUKernelOp = "reduction.min"
	GPUKernelMax       GPUKernelOp = "reduction.max"
	GPUKernelAbs       GPUKernelOp = "math.abs"
	GPUKernelSqrt      GPUKernelOp = "math.sqrt"
	GPUKernelPow       GPUKernelOp = "math.pow"
	GPUKernelExp       GPUKernelOp = "math.exp"
	GPUKernelLog       GPUKernelOp = "math.log"
	GPUKernelLog10     GPUKernelOp = "math.log10"
	GPUKernelLog2      GPUKernelOp = "math.log2"
	GPUKernelSin       GPUKernelOp = "math.sin"
	GPUKernelCos       GPUKernelOp = "math.cos"
	GPUKernelTan       GPUKernelOp = "math.tan"
	GPUKernelAsin      GPUKernelOp = "math.asin"
	GPUKernelAcos      GPUKernelOp = "math.acos"
	GPUKernelAtan      GPUKernelOp = "math.atan"
	GPUKernelFloor     GPUKernelOp = "math.floor"
	GPUKernelCeil      GPUKernelOp = "math.ceil"
	GPUKernelRound     GPUKernelOp = "math.round"
	GPUKernelClamp     GPUKernelOp = "math.clamp"
	GPUKernelHypot     GPUKernelOp = "math.hypot"
	GPUKernelHann      GPUKernelOp = "signal.hann"
	GPUKernelConvolve  GPUKernelOp = "signal.convolve"
)

type GPUValueType string

const GPUValueFloat64 GPUValueType = "f64"

type GPUOwnership string

const (
	GPUOwned    GPUOwnership = "owned"
	GPUBorrowed GPUOwnership = "borrowed"
)

type GPUCapability string

const (
	GPUCapabilityNative        GPUCapability = "native-device"
	GPUCapabilityFloat32       GPUCapability = "float32"
	GPUCapabilityFloat64       GPUCapability = "float64"
	GPUCapabilityTensor        GPUCapability = "tensor"
	GPUCapabilityDeterministic GPUCapability = "deterministic"
	GPUCapabilityConstructors  GPUCapability = "constructors"
	GPUCapabilityRanges        GPUCapability = "ranges"
	GPUCapabilityReductions    GPUCapability = "reductions"
	GPUCapabilityMath          GPUCapability = "math"
	GPUCapabilitySignal        GPUCapability = "signal"
)

type GPUABI struct {
	KernelVersion int `json:"kernel_version"`
	ValueVersion  int `json:"value_version"`
	Version       int `json:"version"`
}

type GPUValueSpec struct {
	ABI       int          `json:"abi_version"`
	Type      GPUValueType `json:"type"`
	Ownership GPUOwnership `json:"ownership"`
}

// GPUValue is the host-side representation of the version-1 value ABI. The
// scalar shape is nil; tensors use row-major dimensions and a flat payload.
// Constructors and provider reads copy Data, so the provider owns its buffers
// independently of caller-owned memory.
type GPUValue struct {
	ABI   int          `json:"abi_version"`
	Type  GPUValueType `json:"type"`
	Shape []int        `json:"shape,omitempty"`
	Data  []float64    `json:"data"`
}

type GPULimits struct {
	MaxElements    int `json:"max_elements"`
	MaxBufferBytes int `json:"max_buffer_bytes"`
	MaxRank        int `json:"max_rank"`
	MaxKernels     int `json:"max_kernels"`
}

// GPUComparisonTolerance is the comparison contract for host reference
// checks. A result passes when both absolute and relative bounds pass.
type GPUComparisonTolerance struct {
	Absolute float64 `json:"absolute"`
	Relative float64 `json:"relative"`
}

var (
	ErrGPUArtifactMalformed     = errors.New("malformed GPU artifact")
	ErrGPUCapabilityUnavailable = errors.New("GPU capability unavailable")
	ErrGPUResourceLimit         = errors.New("GPU resource limit exceeded")
	ErrGPUOwnership             = errors.New("GPU buffer ownership violation")
	ErrGPUExecution             = errors.New("GPU execution failed")
	ErrGPUCancelled             = errors.New("GPU execution cancelled")
)

type GPUError struct {
	Kind   error
	Stage  string
	Detail string
	Cause  error
}

func (e *GPUError) Error() string {
	if e == nil {
		return "<nil>"
	}
	if e.Cause == nil {
		return fmt.Sprintf("GPU %s: %s: %v", e.Stage, e.Kind, e.Detail)
	}
	return fmt.Sprintf("GPU %s: %s: %s: %v", e.Stage, e.Kind, e.Detail, e.Cause)
}

func (e *GPUError) Unwrap() error { return e.Cause }

func (e *GPUError) Is(target error) bool { return target == e.Kind }

func newGPUError(kind error, stage, detail string, cause error) error {
	return &GPUError{Kind: kind, Stage: stage, Detail: detail, Cause: cause}
}

func defaultGPUABI() GPUABI {
	return GPUABI{KernelVersion: GPUKernelABIVersion, ValueVersion: GPUValueABIVersion, Version: GPUArtifactABIVersion}
}

func defaultGPULimits() GPULimits {
	return GPULimits{MaxElements: 1 << 20, MaxBufferBytes: 64 << 20, MaxRank: 8, MaxKernels: 64}
}

func (l GPULimits) normalized() GPULimits {
	d := defaultGPULimits()
	if l.MaxElements > 0 {
		d.MaxElements = l.MaxElements
	}
	if l.MaxBufferBytes > 0 {
		d.MaxBufferBytes = l.MaxBufferBytes
	}
	if l.MaxRank > 0 {
		d.MaxRank = l.MaxRank
	}
	if l.MaxKernels > 0 {
		d.MaxKernels = l.MaxKernels
	}
	return d
}

func stricterGPULimits(a, b GPULimits) GPULimits {
	a = a.normalized()
	b = b.normalized()
	if b.MaxElements < a.MaxElements {
		a.MaxElements = b.MaxElements
	}
	if b.MaxBufferBytes < a.MaxBufferBytes {
		a.MaxBufferBytes = b.MaxBufferBytes
	}
	if b.MaxRank < a.MaxRank {
		a.MaxRank = b.MaxRank
	}
	if b.MaxKernels < a.MaxKernels {
		a.MaxKernels = b.MaxKernels
	}
	return a
}

func newGPUKernel(name, kind string) GPUKernel {
	op := gpuKernelOpForName(name)
	return newGPUKernelForOp(name, kind, op, gpuKernelDefaultArity(op))
}

func newGPUKernelForOp(name, kind string, op GPUKernelOp, arity int) GPUKernel {
	inputs := make([]GPUValueSpec, arity)
	for i := range inputs {
		inputs[i] = GPUValueSpec{ABI: GPUValueABIVersion, Type: GPUValueFloat64, Ownership: GPUBorrowed}
	}
	return GPUKernel{
		Name:       name,
		Kind:       kind,
		Op:         op,
		ABIVersion: GPUKernelABIVersion,
		Inputs:     inputs,
		Output:     GPUValueSpec{ABI: GPUValueABIVersion, Type: GPUValueFloat64, Ownership: GPUOwned},
	}
}

func gpuKernelOpForName(name string) GPUKernelOp {
	for op, names := range map[GPUKernelOp][]string{
		GPUKernelAdd: {"osiris_elemwise_add"}, GPUKernelSub: {"osiris_elemwise_sub"},
		GPUKernelMul: {"osiris_elemwise_mul"}, GPUKernelDiv: {"osiris_elemwise_div"},
		GPUKernelDot: {"osiris_dot"}, GPUKernelMatMul: {"osiris_matmul"},
		GPUKernelVector: {"osiris_vector"}, GPUKernelZeros: {"osiris_zeros"},
		GPUKernelTensor: {"osiris_tensor"}, GPUKernelMatrix: {"osiris_matrix"},
		GPUKernelOnes: {"osiris_ones"}, GPUKernelFull: {"osiris_full"},
		GPUKernelEye: {"osiris_eye", "osiris_identity"}, GPUKernelArange: {"osiris_arange"},
		GPUKernelLinspace: {"osiris_linspace"}, GPUKernelLogspace: {"osiris_logspace"},
		GPUKernelRange: {"osiris_range"}, GPUKernelShape: {"osiris_shape"},
		GPUKernelFlatten: {"osiris_flatten"}, GPUKernelReshape: {"osiris_reshape"},
		GPUKernelTranspose: {"osiris_transpose"}, GPUKernelSum: {"osiris_sum"},
		GPUKernelProduct: {"osiris_product"}, GPUKernelMean: {"osiris_mean"},
		GPUKernelStd: {"osiris_std"}, GPUKernelNorm: {"osiris_norm"},
		GPUKernelMin: {"osiris_min"}, GPUKernelMax: {"osiris_max"},
		GPUKernelAbs: {"osiris_abs"}, GPUKernelSqrt: {"osiris_sqrt"},
		GPUKernelPow: {"osiris_pow"}, GPUKernelExp: {"osiris_exp"},
		GPUKernelLog: {"osiris_log"}, GPUKernelLog10: {"osiris_log10"},
		GPUKernelLog2: {"osiris_log2"}, GPUKernelSin: {"osiris_sin"},
		GPUKernelCos: {"osiris_cos"}, GPUKernelTan: {"osiris_tan"},
		GPUKernelAsin: {"osiris_asin"}, GPUKernelAcos: {"osiris_acos"},
		GPUKernelAtan: {"osiris_atan"}, GPUKernelFloor: {"osiris_floor"},
		GPUKernelCeil: {"osiris_ceil"}, GPUKernelRound: {"osiris_round"},
		GPUKernelClamp: {"osiris_clamp"}, GPUKernelHypot: {"osiris_hypot"},
		GPUKernelHann: {"osiris_hann"}, GPUKernelConvolve: {"osiris_convolve"},
	} {
		for _, candidate := range names {
			if candidate == name {
				return op
			}
		}
	}
	return ""
}

func gpuKernelDefaultArity(op GPUKernelOp) int {
	switch op {
	case GPUKernelAdd, GPUKernelSub, GPUKernelMul, GPUKernelDiv, GPUKernelDot, GPUKernelMatMul, GPUKernelPow, GPUKernelHypot, GPUKernelConvolve:
		return 2
	case GPUKernelClamp:
		return 3
	case GPUKernelFull:
		return 2
	case GPUKernelEye:
		return 1
	case GPUKernelArange, GPUKernelRange, GPUKernelHann:
		return 1
	case GPUKernelLinspace:
		return 2
	case GPUKernelLogspace:
		return 3
	case GPUKernelShape, GPUKernelFlatten, GPUKernelTranspose, GPUKernelSum, GPUKernelProduct, GPUKernelMean, GPUKernelStd, GPUKernelNorm, GPUKernelMin, GPUKernelMax, GPUKernelAbs, GPUKernelSqrt, GPUKernelExp, GPUKernelLog, GPUKernelLog10, GPUKernelLog2, GPUKernelSin, GPUKernelCos, GPUKernelTan, GPUKernelAsin, GPUKernelAcos, GPUKernelAtan, GPUKernelFloor, GPUKernelCeil, GPUKernelRound:
		return 1
	case GPUKernelVector, GPUKernelZeros, GPUKernelOnes:
		return 1
	case GPUKernelReshape:
		return 2
	default:
		return 2
	}
}

func gpuKernelArityValid(op GPUKernelOp, n int) bool {
	switch op {
	case GPUKernelVector, GPUKernelZeros, GPUKernelOnes:
		return n >= 0
	case GPUKernelTensor, GPUKernelMatrix:
		return n >= 1
	case GPUKernelArange:
		return n >= 1 && n <= 3
	case GPUKernelLinspace:
		return n == 2 || n == 3
	case GPUKernelLogspace:
		return n == 3 || n == 4
	case GPUKernelEye:
		return n == 1 || n == 2
	case GPUKernelFull:
		return n >= 2
	case GPUKernelHypot:
		return n >= 2
	case GPUKernelConvolve:
		return n == 2
	default:
		return n == gpuKernelDefaultArity(op)
	}
}

func ValidateGPUArtifact(artifact GPUArtifact) error {
	if artifact.SchemaVersion != gpuArtifactSchemaVersion {
		return newGPUError(ErrGPUArtifactMalformed, "validate", fmt.Sprintf("unsupported schema version %d", artifact.SchemaVersion), nil)
	}
	if artifact.ABI != defaultGPUABI() {
		return newGPUError(ErrGPUArtifactMalformed, "validate", "unsupported kernel/value ABI version", nil)
	}
	if len(artifact.Source) > gpuMaxSourceBytes {
		return newGPUError(ErrGPUResourceLimit, "validate", "kernel source exceeds byte limit", nil)
	}
	limits := artifact.Limits
	if limits.MaxElements < 0 || limits.MaxBufferBytes < 0 || limits.MaxRank < 0 || limits.MaxKernels < 0 {
		return newGPUError(ErrGPUArtifactMalformed, "validate", "negative resource limit", nil)
	}
	limits = limits.normalized()
	if len(artifact.Kernels) == 0 || len(artifact.Kernels) > limits.MaxKernels {
		return newGPUError(ErrGPUResourceLimit, "validate", "kernel count is outside the configured bound", nil)
	}
	seen := make(map[string]struct{}, len(artifact.Kernels))
	for _, kernel := range artifact.Kernels {
		if kernel.Name == "" || kernel.Op == "" {
			return newGPUError(ErrGPUArtifactMalformed, "validate", "kernel name and operation are required", nil)
		}
		if _, ok := seen[kernel.Name]; ok {
			return newGPUError(ErrGPUArtifactMalformed, "validate", "duplicate kernel "+kernel.Name, nil)
		}
		seen[kernel.Name] = struct{}{}
		if kernel.ABIVersion != GPUKernelABIVersion {
			return newGPUError(ErrGPUArtifactMalformed, "validate", "unsupported kernel ABI for "+kernel.Name, nil)
		}
		if !gpuKernelSupported(kernel.Op) {
			return newGPUError(ErrGPUCapabilityUnavailable, "validate", "unsupported kernel operation "+string(kernel.Op), nil)
		}
		if !gpuKernelArityValid(kernel.Op, len(kernel.Inputs)) {
			return newGPUError(ErrGPUArtifactMalformed, "validate", "kernel "+kernel.Name+" has an invalid input count", nil)
		}
		for _, spec := range kernel.Inputs {
			if spec.ABI != GPUValueABIVersion || spec.Type != GPUValueFloat64 {
				return newGPUError(ErrGPUArtifactMalformed, "validate", "kernel "+kernel.Name+" has an unsupported value ABI", nil)
			}
			if spec.Ownership != GPUBorrowed {
				return newGPUError(ErrGPUOwnership, "validate", "kernel "+kernel.Name+" inputs must be borrowed", nil)
			}
		}
		if kernel.Output.ABI != GPUValueABIVersion || kernel.Output.Type != GPUValueFloat64 {
			return newGPUError(ErrGPUArtifactMalformed, "validate", "kernel "+kernel.Name+" has an unsupported output ABI", nil)
		}
		if kernel.Output.Ownership != GPUOwned {
			return newGPUError(ErrGPUOwnership, "validate", "kernel output must be owned", nil)
		}
	}
	return nil
}

func ValidateGPUValue(value GPUValue, limits GPULimits) error {
	limits = limits.normalized()
	if value.ABI != GPUValueABIVersion || value.Type != GPUValueFloat64 {
		return newGPUError(ErrGPUArtifactMalformed, "value", "unsupported value ABI", nil)
	}
	if len(value.Shape) > limits.MaxRank {
		return newGPUError(ErrGPUResourceLimit, "value", "tensor rank exceeds limit", nil)
	}
	if len(value.Shape) == 0 {
		if len(value.Data) != 1 {
			return newGPUError(ErrGPUArtifactMalformed, "value", "scalar must contain one element", nil)
		}
	} else {
		total, err := gpuShapeElements(value.Shape)
		if err != nil {
			return err
		}
		if total != len(value.Data) {
			return newGPUError(ErrGPUArtifactMalformed, "value", "shape does not match payload length", nil)
		}
	}
	if len(value.Data) > limits.MaxElements || len(value.Data) > limits.MaxBufferBytes/8 {
		return newGPUError(ErrGPUResourceLimit, "value", "payload exceeds element or byte limit", nil)
	}
	return nil
}

func gpuShapeElements(shape []int) (int, error) {
	total := 1
	for _, dim := range shape {
		if dim < 0 {
			return 0, newGPUError(ErrGPUArtifactMalformed, "value", "negative tensor dimension", nil)
		}
		if dim != 0 && total > int(^uint(0)>>1)/dim {
			return 0, newGPUError(ErrGPUResourceLimit, "value", "tensor element count overflows host size", nil)
		}
		total *= dim
	}
	return total, nil
}

func NewGPUValue(data []float64, shape []int) (GPUValue, error) {
	value := GPUValue{ABI: GPUValueABIVersion, Type: GPUValueFloat64, Shape: append([]int(nil), shape...), Data: append([]float64(nil), data...)}
	if err := ValidateGPUValue(value, defaultGPULimits()); err != nil {
		return GPUValue{}, err
	}
	return value, nil
}

func GPUValueFromValue(value Value) (GPUValue, error) {
	shape, data, err := inferTensor(value)
	if err != nil {
		return GPUValue{}, newGPUError(ErrGPUCapabilityUnavailable, "value", "value is not representable by the f64 ABI", err)
	}
	out := make([]float64, len(data))
	for i, item := range data {
		out[i], err = item.AsNumber()
		if err != nil || math.IsNaN(out[i]) || math.IsInf(out[i], 0) {
			return GPUValue{}, newGPUError(ErrGPUCapabilityUnavailable, "value", "non-finite or non-numeric value is not supported", err)
		}
	}
	if len(shape) == 0 {
		shape = nil
	}
	return NewGPUValue(out, shape)
}

func (value GPUValue) ToValue() (Value, error) {
	if err := ValidateGPUValue(value, defaultGPULimits()); err != nil {
		return nullValue(), err
	}
	data := make([]Value, len(value.Data))
	for i, item := range value.Data {
		data[i] = numberValue(item)
	}
	if len(value.Shape) == 0 {
		return data[0], nil
	}
	return tensorFromShapeAndData(value.Shape, data)
}

func CompareGPUValues(got, want GPUValue, tolerance GPUComparisonTolerance) error {
	if tolerance.Absolute < 0 || tolerance.Relative < 0 || math.IsNaN(tolerance.Absolute) || math.IsNaN(tolerance.Relative) {
		return fmt.Errorf("invalid GPU comparison tolerance")
	}
	if err := ValidateGPUValue(got, defaultGPULimits()); err != nil {
		return err
	}
	if err := ValidateGPUValue(want, defaultGPULimits()); err != nil {
		return err
	}
	if !equalIntSlices(got.Shape, want.Shape) || len(got.Data) != len(want.Data) {
		return fmt.Errorf("GPU values have different shapes or lengths")
	}
	for i := range got.Data {
		delta := math.Abs(got.Data[i] - want.Data[i])
		bound := tolerance.Absolute + tolerance.Relative*math.Abs(want.Data[i])
		if delta > bound {
			return fmt.Errorf("GPU value[%d] differs by %.17g, tolerance %.17g", i, delta, bound)
		}
	}
	return nil
}
