package osiris

import (
	"context"
	"fmt"
	"strings"
)

func newTensorValue(shape []int, data []Value) Value {
	shapeCopy := append([]int(nil), shape...)
	dataCopy := append([]Value(nil), data...)
	return Value{kind: kindTensor, shape: shapeCopy, data: dataCopy}
}

func tensorShapeMatches(shape []int, n int) bool {
	if len(shape) == 0 {
		return n == 1
	}
	total := 1
	for _, dim := range shape {
		if dim < 0 {
			return false
		}
		total *= dim
	}
	return total == n
}

func tensorShapeProduct(shape []int) int {
	if len(shape) == 0 {
		return 1
	}
	total := 1
	for _, dim := range shape {
		total *= dim
	}
	return total
}

func tensorShapeString(shape []int) string {
	if len(shape) == 0 {
		return "[]"
	}
	parts := make([]string, len(shape))
	for i, dim := range shape {
		parts[i] = fmt.Sprintf("%d", dim)
	}
	return "[" + strings.Join(parts, "x") + "]"
}

func formatTensor(shape []int, data []Value) string {
	var b strings.Builder
	b.WriteString("tensor")
	b.WriteString(tensorShapeString(shape))
	b.WriteByte('(')
	b.WriteString(listValue(data).String())
	b.WriteByte(')')
	return b.String()
}

func inferTensor(v Value) ([]int, []Value, error) {
	switch v.kind {
	case kindTensor:
		return append([]int(nil), v.shape...), append([]Value(nil), v.data...), nil
	case kindList:
		if len(v.list) == 0 {
			return []int{0}, nil, nil
		}
		allScalar := true
		for _, item := range v.list {
			if item.kind == kindList || item.kind == kindTensor {
				allScalar = false
				break
			}
		}
		if allScalar {
			return []int{len(v.list)}, append([]Value(nil), v.list...), nil
		}
		var (
			subshape []int
			data     []Value
		)
		for i, item := range v.list {
			shape, flat, err := inferTensor(item)
			if err != nil {
				return nil, nil, err
			}
			if i == 0 {
				subshape = shape
			} else if !equalIntSlices(subshape, shape) {
				return nil, nil, fmt.Errorf("tensor rows must share the same shape")
			}
			data = append(data, flat...)
		}
		shape := append([]int{len(v.list)}, subshape...)
		return shape, data, nil
	default:
		return []int{}, []Value{v}, nil
	}
}

func equalIntSlices(a, b []int) bool {
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

func tensorBroadcastShape(left, right []int) ([]int, error) {
	width := len(left)
	if len(right) > width {
		width = len(right)
	}
	out := make([]int, width)
	for i := 0; i < width; i++ {
		li := 1
		ri := 1
		if idx := len(left) - 1 - i; idx >= 0 {
			li = left[idx]
		}
		if idx := len(right) - 1 - i; idx >= 0 {
			ri = right[idx]
		}
		if li != ri && li != 1 && ri != 1 {
			return nil, fmt.Errorf("tensor shape %v does not match %v", left, right)
		}
		if li > ri {
			out[width-1-i] = li
		} else {
			out[width-1-i] = ri
		}
	}
	return out, nil
}

func tensorShapeStrides(shape []int) []int {
	if len(shape) == 0 {
		return nil
	}
	strides := make([]int, len(shape))
	stride := 1
	for i := len(shape) - 1; i >= 0; i-- {
		strides[i] = stride
		stride *= shape[i]
	}
	return strides
}

func tensorFlatToCoords(flat int, shape, strides []int) []int {
	if len(shape) == 0 {
		return nil
	}
	coords := make([]int, len(shape))
	rem := flat
	for i, stride := range strides {
		if stride == 0 {
			continue
		}
		coords[i] = rem / stride
		rem %= stride
	}
	return coords
}

func tensorBroadcastIndexFromCoords(coords, shape, strides []int) int {
	if len(shape) == 0 {
		return 0
	}
	offset := len(coords) - len(shape)
	idx := 0
	for i, dim := range shape {
		if dim == 1 {
			continue
		}
		idx += coords[offset+i] * strides[i]
	}
	return idx
}

func tensorIncrementCoords(coords, shape []int) {
	for i := len(coords) - 1; i >= 0; i-- {
		coords[i]++
		if coords[i] < shape[i] {
			return
		}
		coords[i] = 0
	}
}

func tensorFromShapeAndData(shape []int, data []Value) (Value, error) {
	if !tensorShapeMatches(shape, len(data)) {
		return nullValue(), fmt.Errorf("tensor shape %v does not match %d elements", shape, len(data))
	}
	return newTensorValue(shape, data), nil
}

func tensorValueFromArgs(args []Value) (Value, error) {
	switch len(args) {
	case 1:
		shape, data, err := inferTensor(args[0])
		if err != nil {
			return nullValue(), err
		}
		return tensorFromShapeAndData(shape, data)
	case 2:
		dims, err := tensorDims(args[0])
		if err != nil {
			return nullValue(), err
		}
		_, data, err := inferTensor(args[1])
		if err != nil {
			return nullValue(), err
		}
		if !tensorShapeMatches(dims, len(data)) {
			return nullValue(), fmt.Errorf("tensor shape %v does not match %d elements", dims, len(data))
		}
		return tensorFromShapeAndData(dims, data)
	default:
		return nullValue(), fmt.Errorf("tensor expects 1 or 2 arguments")
	}
}

func matrixValueFromArgs(args []Value) (Value, error) {
	switch len(args) {
	case 1, 2:
		return tensorValueFromArgs(args)
	case 3:
		rows, err := args[0].Int()
		if err != nil {
			return nullValue(), err
		}
		cols, err := args[1].Int()
		if err != nil {
			return nullValue(), err
		}
		if rows < 0 || cols < 0 {
			return nullValue(), fmt.Errorf("matrix dimensions must be non-negative")
		}
		shape := []int{rows, cols}
		_, data, err := inferTensor(args[2])
		if err != nil {
			return nullValue(), err
		}
		return tensorFromShapeAndData(shape, data)
	default:
		return nullValue(), fmt.Errorf("matrix expects 1, 2, or 3 arguments")
	}
}

func tensorDims(v Value) ([]int, error) {
	items, err := v.AsList()
	if err != nil {
		return nil, fmt.Errorf("expected tensor shape list, got %s", v.Kind())
	}
	dims := make([]int, len(items))
	for i, item := range items {
		n, err := item.Int()
		if err != nil {
			return nil, err
		}
		if n < 0 {
			return nil, fmt.Errorf("tensor dimensions must be non-negative")
		}
		dims[i] = n
	}
	return dims, nil
}

func tensorShapeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("shape expects 1 argument")
	}
	shape, _, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(shape))
	for i, dim := range shape {
		out[i] = uintValue(uint64(dim), 64)
	}
	return listValue(out), nil
}

func tensorFlattenValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("flatten expects 1 argument")
	}
	_, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	return listValue(append([]Value(nil), data...)), nil
}

func tensorTransposeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("transpose expects 1 argument")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 {
		return nullValue(), fmt.Errorf("transpose expects a 2D matrix")
	}
	rows, cols := shape[0], shape[1]
	out := make([]Value, len(data))
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			out[c*rows+r] = data[r*cols+c]
		}
	}
	return tensorFromShapeAndData([]int{cols, rows}, out)
}

func tensorDotValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("dot expects 2 arguments")
	}
	_, left, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	_, right, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	if len(left) != len(right) {
		return nullValue(), fmt.Errorf("dot expects tensors with the same length")
	}
	var total float64
	for i := range left {
		ln, err := left[i].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right[i].AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += ln * rn
	}
	return floatResultValue(total, append(append([]Value(nil), left...), right...)...), nil
}

func tensorMatMulValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("matmul expects 2 arguments")
	}
	leftShape, leftData, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	rightShape, rightData, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	if len(leftShape) != 2 || len(rightShape) != 2 {
		return nullValue(), fmt.Errorf("matmul expects 2D matrices")
	}
	if leftShape[1] != rightShape[0] {
		return nullValue(), fmt.Errorf("matmul shape mismatch")
	}
	rows, mid, cols := leftShape[0], leftShape[1], rightShape[1]
	out := make([]Value, rows*cols)
	for r := 0; r < rows; r++ {
		for c := 0; c < cols; c++ {
			var total float64
			for k := 0; k < mid; k++ {
				ln, err := leftData[r*mid+k].AsNumber()
				if err != nil {
					return nullValue(), err
				}
				rn, err := rightData[k*cols+c].AsNumber()
				if err != nil {
					return nullValue(), err
				}
				total += ln * rn
			}
			out[r*cols+c] = floatResultValue(total, append(append([]Value(nil), leftData...), rightData...)...)
		}
	}
	return tensorFromShapeAndData([]int{rows, cols}, out)
}

func tensorReshapeValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("reshape expects 2 arguments")
	}
	shape, err := tensorDims(args[1])
	if err != nil {
		return nullValue(), err
	}
	_, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	return tensorFromShapeAndData(shape, data)
}

func tensorCloneValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("clone expects 1 argument")
	}
	if args[0].kind != kindTensor {
		return nullValue(), fmt.Errorf("clone expects a tensor")
	}
	return newTensorValue(args[0].shape, args[0].data), nil
}

func tensorViewValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("view expects 1 argument")
	}
	if args[0].kind != kindTensor {
		return nullValue(), fmt.Errorf("view expects a tensor")
	}
	return Value{kind: kindTensor, shape: args[0].shape, data: args[0].data}, nil
}

func tensorBinaryValue(ctx context.Context, sched deterministicScheduler, op tokenKind, left, right Value) (Value, error) {
	if op == tokenEqualEqual {
		return boolValue(left.Equal(right)), nil
	}
	if op == tokenBangEqual {
		return boolValue(!left.Equal(right)), nil
	}
	if op != tokenPlus && op != tokenMinus && op != tokenStar && op != tokenSlash {
		return nullValue(), fmt.Errorf("unsupported tensor operator %s", op)
	}

	lshape, ldata, err := inferTensor(left)
	if err != nil {
		if left.kind != kindTensor {
			lshape = nil
			ldata = []Value{left}
		} else {
			return nullValue(), err
		}
	}
	rshape, rdata, err := inferTensor(right)
	if err != nil {
		if right.kind != kindTensor {
			rshape = nil
			rdata = []Value{right}
		} else {
			return nullValue(), err
		}
	}

	switch {
	case left.kind == kindTensor && right.kind == kindTensor:
		// General NumPy-style broadcasting.
		if _, err := tensorBroadcastShape(lshape, rshape); err != nil {
			return nullValue(), err
		}
	case left.kind == kindTensor && right.kind != kindTensor:
		if len(ldata) == 0 {
			return nullValue(), fmt.Errorf("tensor is empty")
		}
	case right.kind == kindTensor && left.kind != kindTensor:
		if len(rdata) == 0 {
			return nullValue(), fmt.Errorf("tensor is empty")
		}
	default:
		return nullValue(), fmt.Errorf("tensor binary operator requires at least one tensor")
	}

	outShape, err := tensorBroadcastShape(lshape, rshape)
	if err != nil {
		return nullValue(), err
	}
	outLen := tensorShapeProduct(outShape)
	if outLen == 0 {
		return newTensorValue(outShape, nil), nil
	}

	out := make([]Value, outLen)
	lstrides := tensorShapeStrides(lshape)
	rstrides := tensorShapeStrides(rshape)
	outStrides := tensorShapeStrides(outShape)
	chunk := 256
	if outLen < chunk {
		chunk = outLen
	}
	tasks := make([]evalTask, 0, (outLen+chunk-1)/chunk)
	for start := 0; start < outLen; start += chunk {
		end := start + chunk
		if end > outLen {
			end = outLen
		}
		start, end := start, end
		tasks = append(tasks, evalTask{
			key: fmt.Sprintf("%08d", start),
			run: func() (Value, error) {
				coords := tensorFlatToCoords(start, outShape, outStrides)
				for i := start; i < end; i++ {
					li := tensorBroadcastIndexFromCoords(coords, lshape, lstrides)
					ri := tensorBroadcastIndexFromCoords(coords, rshape, rstrides)
					ln, err := ldata[li].AsNumber()
					if err != nil {
						return nullValue(), err
					}
					rn, err := rdata[ri].AsNumber()
					if err != nil {
						return nullValue(), err
					}
					switch op {
					case tokenPlus:
						out[i] = floatResultValue(ln+rn, ldata[li], rdata[ri])
					case tokenMinus:
						out[i] = floatResultValue(ln-rn, ldata[li], rdata[ri])
					case tokenStar:
						out[i] = floatResultValue(ln*rn, ldata[li], rdata[ri])
					case tokenSlash:
						if rn == 0 {
							return nullValue(), fmt.Errorf("division by zero")
						}
						out[i] = floatResultValue(ln/rn, ldata[li], rdata[ri])
					}
					tensorIncrementCoords(coords, outShape)
				}
				return nullValue(), nil
			},
		})
	}
	if _, err := sched.run(ctx, tasks); err != nil {
		return nullValue(), err
	}
	return newTensorValue(outShape, out), nil
}
