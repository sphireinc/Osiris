package osiris

import (
	"fmt"
)

// normalizeTensorShape handles the normalize tensor shape step in the
// surrounding package and returns []int.
//
// It returns []int so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func normalizeTensorShape(shape []int, data []Value) []int {
	if len(shape) == 0 {
		return []int{1}
	}
	return append([]int(nil), shape...)
}

// broadcastValuesToShape handles the broadcast values to shape step in the
// surrounding package and returns []Value, error.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func broadcastValuesToShape(shape []int, data []Value, target []int) ([]Value, error) {
	if equalIntSlices(shape, target) {
		return append([]Value(nil), data...), nil
	}
	if len(target) == 0 {
		if len(data) != 1 {
			return nil, fmt.Errorf("broadcast shape mismatch")
		}
		return append([]Value(nil), data...), nil
	}
	outShape, err := tensorBroadcastShape(shape, target)
	if err != nil {
		return nil, err
	}
	if !equalIntSlices(outShape, target) {
		return nil, fmt.Errorf("broadcast shape mismatch")
	}
	out := make([]Value, tensorShapeProduct(target))
	strides := tensorShapeStrides(shape)
	outStrides := tensorShapeStrides(target)
	for i := range out {
		coords := tensorFlatToCoords(i, target, outStrides)
		idx := tensorBroadcastIndexFromCoords(coords, shape, strides)
		out[i] = data[idx]
	}
	return out, nil
}

// tensorSqueezeValue handles the tensor squeeze value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorSqueezeValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("squeeze expects 1 argument")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	outShape := make([]int, 0, len(shape))
	for _, dim := range shape {
		if dim != 1 {
			outShape = append(outShape, dim)
		}
	}
	return tensorFromShapeAndData(outShape, data)
}

// tensorExpandDimsValue handles the tensor expand dims value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorExpandDimsValue(args []Value) (Value, error) {
	if len(args) != 1 && len(args) != 2 {
		return nullValue(), fmt.Errorf("expand_dims expects 1 or 2 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	axis := 0
	if len(args) == 2 {
		axis, err = args[1].Int()
		if err != nil {
			return nullValue(), err
		}
	}
	if axis < 0 {
		axis += len(shape) + 1
	}
	if axis < 0 || axis > len(shape) {
		return nullValue(), fmt.Errorf("expand_dims axis out of range")
	}
	outShape := make([]int, 0, len(shape)+1)
	outShape = append(outShape, shape[:axis]...)
	outShape = append(outShape, 1)
	outShape = append(outShape, shape[axis:]...)
	return tensorFromShapeAndData(outShape, data)
}

// tensorConcatValue handles the tensor concat value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorConcatValue(args []Value) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("concat expects at least 2 arguments")
	}
	shapes := make([][]int, len(args))
	dataSets := make([][]Value, len(args))
	for i := range args {
		shape, data, err := inferTensor(args[i])
		if err != nil {
			return nullValue(), err
		}
		shapes[i] = normalizeTensorShape(shape, data)
		dataSets[i] = data
	}
	tail := append([]int(nil), shapes[0][1:]...)
	total := 0
	for i, shape := range shapes {
		if len(shape) == 0 {
			return nullValue(), fmt.Errorf("concat expects arrays, got scalar")
		}
		if !equalIntSlices(shape[1:], tail) {
			return nullValue(), fmt.Errorf("concat shape mismatch")
		}
		total += shape[0]
		if i == 0 && len(tail) == 0 {
			tail = nil
		}
	}
	outShape := append([]int{total}, tail...)
	out := make([]Value, 0, tensorShapeProduct(outShape))
	for _, data := range dataSets {
		out = append(out, data...)
	}
	return tensorFromShapeAndData(outShape, out)
}

// tensorStackValue handles the tensor stack value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorStackValue(args []Value) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("stack expects at least 2 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	shapes := make([][]int, len(args))
	dataSets := make([][]Value, len(args))
	shapes[0] = append([]int(nil), shape...)
	dataSets[0] = data
	for i := 1; i < len(args); i++ {
		curShape, curData, err := inferTensor(args[i])
		if err != nil {
			return nullValue(), err
		}
		if !equalIntSlices(curShape, shape) {
			return nullValue(), fmt.Errorf("stack shape mismatch")
		}
		shapes[i] = curShape
		dataSets[i] = curData
	}
	outShape := append([]int{len(args)}, shape...)
	out := make([]Value, 0, tensorShapeProduct(outShape))
	for _, data := range dataSets {
		out = append(out, data...)
	}
	return tensorFromShapeAndData(outShape, out)
}

// tensorRepeatValue handles the tensor repeat value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorRepeatValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("repeat expects 2 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	count, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if count < 0 {
		return nullValue(), fmt.Errorf("repeat count must be non-negative")
	}
	if len(shape) == 0 {
		out := make([]Value, 0, count)
		for i := 0; i < count; i++ {
			out = append(out, data[0])
		}
		return tensorFromShapeAndData([]int{count}, out)
	}
	outShape := append([]int{shape[0] * count}, shape[1:]...)
	out := make([]Value, 0, tensorShapeProduct(outShape))
	for i := 0; i < count; i++ {
		out = append(out, data...)
	}
	return tensorFromShapeAndData(outShape, out)
}

// tensorTileValue handles the tensor tile value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorTileValue(args []Value) (Value, error) {
	return tensorRepeatValue(args)
}

// tensorBroadcastToValue handles the tensor broadcast to value step in the
// surrounding package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorBroadcastToValue(args []Value) (Value, error) {
	if len(args) < 2 {
		return nullValue(), fmt.Errorf("broadcast_to expects a value and target shape")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	target, err := parseShapeArgs(args[1:])
	if err != nil {
		return nullValue(), err
	}
	out, err := broadcastValuesToShape(shape, data, target)
	if err != nil {
		return nullValue(), err
	}
	return tensorFromShapeAndData(target, out)
}

// tensorWhereValue handles the tensor where value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorWhereValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("where expects 3 arguments")
	}
	cshape, cdata, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	tshape, tdata, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	fshape, fdata, err := inferTensor(args[2])
	if err != nil {
		return nullValue(), err
	}
	outShape, err := tensorBroadcastShape(cshape, tshape)
	if err != nil {
		return nullValue(), err
	}
	outShape, err = tensorBroadcastShape(outShape, fshape)
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, tensorShapeProduct(outShape))
	cstrides := tensorShapeStrides(cshape)
	tstrides := tensorShapeStrides(tshape)
	fstrides := tensorShapeStrides(fshape)
	outStrides := tensorShapeStrides(outShape)
	for i := range out {
		coords := tensorFlatToCoords(i, outShape, outStrides)
		ci := tensorBroadcastIndexFromCoords(coords, cshape, cstrides)
		ti := tensorBroadcastIndexFromCoords(coords, tshape, tstrides)
		fi := tensorBroadcastIndexFromCoords(coords, fshape, fstrides)
		if cdata[ci].Truthy() {
			out[i] = tdata[ti]
		} else {
			out[i] = fdata[fi]
		}
	}
	return tensorFromShapeAndData(outShape, out)
}

// tensorClipValue handles the tensor clip value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorClipValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("clip expects 3 arguments")
	}
	lo, err := args[1].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	hi, err := args[2].AsNumber()
	if err != nil {
		return nullValue(), err
	}
	return mapValueUnary(args[0], func(v Value) (Value, error) {
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		switch {
		case n < lo:
			n = lo
		case n > hi:
			n = hi
		}
		return floatValueStyled(n, floatUnaryBits(v), floatUnaryBFloat(v)), nil
	})
}

// tensorSplitValue handles the tensor split value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorSplitValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("split expects 2 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	chunk, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	if chunk <= 0 {
		return nullValue(), fmt.Errorf("split chunk must be positive")
	}
	if len(shape) == 0 {
		return listValue([]Value{args[0]}), nil
	}
	tailShape := append([]int(nil), shape[1:]...)
	tailSize := tensorShapeProduct(tailShape)
	if tailSize == 0 {
		tailSize = 1
	}
	var out []Value
	for start := 0; start < shape[0]; start += chunk {
		end := start + chunk
		if end > shape[0] {
			end = shape[0]
		}
		sliceShape := append([]int{end - start}, tailShape...)
		sliceData := append([]Value(nil), data[start*tailSize:end*tailSize]...)
		slice, err := tensorFromShapeAndData(sliceShape, sliceData)
		if err != nil {
			return nullValue(), err
		}
		out = append(out, slice)
	}
	return listValue(out), nil
}

// tensorGatherValue handles the tensor gather value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorGatherValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("gather expects 2 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	idxShape, idxData, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	flat := append([]Value(nil), data...)
	out := make([]Value, len(idxData))
	for i, idxVal := range idxData {
		idx, err := idxVal.Int()
		if err != nil {
			return nullValue(), err
		}
		if idx < 0 || idx >= len(flat) {
			return nullValue(), fmt.Errorf("gather index out of range")
		}
		out[i] = flat[idx]
	}
	_ = shape
	return tensorFromShapeAndData(idxShape, out)
}

// tensorScatterValue handles the tensor scatter value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorScatterValue(args []Value) (Value, error) {
	if len(args) != 3 {
		return nullValue(), fmt.Errorf("scatter expects 3 arguments")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	_, idxData, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	_, valData, err := inferTensor(args[2])
	if err != nil {
		return nullValue(), err
	}
	out := append([]Value(nil), data...)
	if len(valData) != 1 && len(valData) != len(idxData) {
		return nullValue(), fmt.Errorf("scatter values must be scalar or match indices")
	}
	for i, idxVal := range idxData {
		idx, err := idxVal.Int()
		if err != nil {
			return nullValue(), err
		}
		if idx < 0 || idx >= len(out) {
			return nullValue(), fmt.Errorf("scatter index out of range")
		}
		if len(valData) == 1 {
			out[idx] = valData[0]
		} else {
			out[idx] = valData[i]
		}
	}
	return tensorFromShapeAndData(shape, out)
}

// tensorPadValue handles the tensor pad value step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func tensorPadValue(args []Value) (Value, error) {
	if len(args) < 3 || len(args) > 4 {
		return nullValue(), fmt.Errorf("pad expects value, left, right, and optional fill")
	}
	shape, data, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	left, err := args[1].Int()
	if err != nil {
		return nullValue(), err
	}
	right, err := args[2].Int()
	if err != nil {
		return nullValue(), err
	}
	if left < 0 || right < 0 {
		return nullValue(), fmt.Errorf("pad widths must be non-negative")
	}
	fill := numberValue(0)
	if len(args) == 4 {
		fill = args[3]
	}
	if len(shape) == 0 {
		shape = []int{1}
	}
	tailShape := append([]int(nil), shape[1:]...)
	tailSize := tensorShapeProduct(tailShape)
	if tailSize == 0 {
		tailSize = 1
	}
	if len(shape) == 1 {
		tailSize = 1
	}
	outShape := append([]int{left + shape[0] + right}, tailShape...)
	out := make([]Value, 0, tensorShapeProduct(outShape))
	for i := 0; i < left*tailSize; i++ {
		out = append(out, fill)
	}
	out = append(out, data...)
	for i := 0; i < right*tailSize; i++ {
		out = append(out, fill)
	}
	return tensorFromShapeAndData(outShape, out)
}
