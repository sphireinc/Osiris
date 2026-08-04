package osiris

import (
	"fmt"
	"math"
	"math/big"
	"sort"
	"strconv"
	"strings"
)

// futureValue is the common boxed representation for the five formerly
// reserved numeric/shape families. The representation is intentionally
// portable: element values retain their Value metadata, sparse coordinates are
// canonicalized, and no host SIMD or sparse-library ABI is implied.
type futureValue struct {
	name   string
	params []string
	data   []Value
	shape  []int
	coords [][]int
	format string
}

func futureIntSlicesEqual(left, right []int) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func newFutureValue(name string, params []string, data []Value, shape []int, coords [][]int, format string) Value {
	return Value{kind: kindFuture, future: &futureValue{
		name: name, params: append([]string(nil), params...), data: append([]Value(nil), data...),
		shape: append([]int(nil), shape...), coords: cloneFutureCoords(coords), format: format,
	}}
}

func cloneFutureCoords(coords [][]int) [][]int {
	out := make([][]int, len(coords))
	for i := range coords {
		out[i] = append([]int(nil), coords[i]...)
	}
	return out
}

func futureTypeString(v *futureValue) string {
	if v == nil {
		return "future"
	}
	if len(v.params) == 0 {
		return v.name
	}
	return v.name + "<" + strings.Join(v.params, ", ") + ">"
}

func futureTypeParts(name string, params []string) (string, []string, error) {
	if name == "" {
		return "", nil, fmt.Errorf("future type name cannot be empty")
	}
	canonical := append([]string(nil), params...)
	switch name {
	case "poly", "quat", "sparse_matrix", "sparse_tensor":
		if len(canonical) != 1 {
			return "", nil, fmt.Errorf("%s requires exactly one element type", name)
		}
	case "simd":
		if len(canonical) != 2 {
			return "", nil, fmt.Errorf("simd requires a lane count and element type")
		}
		lanes, err := strconv.Atoi(canonical[0])
		if err != nil || lanes < 2 || lanes > 16 || lanes&(lanes-1) != 0 {
			return "", nil, fmt.Errorf("simd lane count must be a power of two from 2 through 16")
		}
	default:
		return "", nil, fmt.Errorf("unknown future type %q", name)
	}
	for i, param := range canonical {
		if name == "simd" && i == 0 {
			continue
		}
		if name == "simd" && i == len(canonical)-1 && param == "bool" {
			continue
		}
		if !futureScalarType(param) {
			return "", nil, fmt.Errorf("%s does not support element type %q; use number, f32, or f64", name, param)
		}
	}
	return name, canonical, nil
}

func futureConstructorName(name string) string {
	if dot := strings.LastIndexByte(name, '.'); dot >= 0 {
		return name[dot+1:]
	}
	return name
}

func futureConstructorCall(name string, typeArgs []string, args []Value) (Value, error) {
	base := futureConstructorName(name)
	if base == "simd" && len(typeArgs) == 2 && typeArgs[1] == "bool" {
		return futureSIMDMask(args)
	}
	var value Value
	var err error
	switch base {
	case "poly":
		value, err = futurePolyValue(args)
	case "quat":
		value, err = futureQuatValue(args)
	case "simd":
		value, err = futureSIMDValue(args)
	case "sparse_matrix", "sparse_tensor":
		value, err = futureSparseValue(base, args)
	default:
		return nullValue(), fmt.Errorf("%s is not a reserved type constructor", name)
	}
	if err != nil || len(typeArgs) == 0 {
		return value, err
	}
	_, params, typeErr := futureTypeParts(base, typeArgs)
	if typeErr != nil {
		return nullValue(), typeErr
	}
	if value.kind != kindFuture || value.future == nil {
		return nullValue(), fmt.Errorf("%s did not produce a future value", name)
	}
	if base == "simd" && len(value.future.data) != atoiOrZero(params[0]) {
		return nullValue(), fmt.Errorf("simd lane count %s does not match %d values", params[0], len(value.future.data))
	}
	element := params[len(params)-1]
	if base == "simd" && element == "bool" {
		return futureSIMDMask(args)
	}
	data := make([]Value, len(value.future.data))
	for i, item := range value.future.data {
		data[i], err = futureScalar(item, element)
		if err != nil {
			return nullValue(), err
		}
	}
	return newFutureValue(base, params, data, value.future.shape, value.future.coords, value.future.format), nil
}

func atoiOrZero(text string) int {
	value, _ := strconv.Atoi(text)
	return value
}

func isFutureConstructorName(name string) bool {
	switch futureConstructorName(name) {
	case "poly", "quat", "simd", "sparse_matrix", "sparse_tensor":
		return true
	default:
		return false
	}
}

func futureCallStaticType(name string, typeArgs []string, args []Expr, actual []staticType) (staticType, error) {
	name = futureConstructorName(name)
	if !isFutureConstructorName(name) {
		return staticType{}, fmt.Errorf("%s is not a reserved type constructor", name)
	}
	validArity := func(ok bool) error {
		if !ok {
			return fmt.Errorf("%s received an invalid number of arguments", name)
		}
		return nil
	}
	switch name {
	case "poly", "sparse_matrix", "sparse_tensor":
		if err := validArity(len(args) == 1 || (name != "poly" && (len(args) == 2 || len(args) == 3))); err != nil {
			return staticType{}, err
		}
	case "quat":
		if err := validArity(len(args) == 1 || len(args) == 4); err != nil {
			return staticType{}, err
		}
	case "simd":
		if err := validArity(len(args) == 1 || len(args) == 2); err != nil {
			return staticType{}, err
		}
	}
	if len(typeArgs) > 0 {
		params := make([]string, len(typeArgs))
		copy(params, typeArgs)
		_, params, err := futureTypeParts(name, params)
		if err != nil {
			return staticType{}, err
		}
		out := make([]staticType, len(params))
		for i, param := range params {
			out[i] = staticType{name: param}
		}
		return staticType{name: name, params: out}, nil
	}
	// A constructor without explicit parameters remains a valid dynamic
	// boundary. Runtime construction infers the element family from values;
	// static literal types such as u64 are not enough to distinguish an
	// untyped number from an explicitly widened future element.
	return staticType{name: name}, nil
}

func validateFutureStaticType(name string, params []staticType) error {
	if name != "poly" && name != "quat" && name != "simd" && name != "sparse_matrix" && name != "sparse_tensor" {
		return nil
	}
	parts := make([]string, len(params))
	for i, param := range params {
		parts[i] = param.String()
	}
	_, _, err := futureTypeParts(name, parts)
	return err
}

func futureScalarType(name string) bool {
	return name == "number" || name == "f32" || name == "f64"
}

func futureElementType(values []Value) (string, error) {
	if len(values) == 0 {
		return "number", nil
	}
	result := ""
	for _, value := range values {
		var current string
		switch {
		case value.kind == kindNumber && value.bfloat:
			return "", fmt.Errorf("future types do not support bfloat elements")
		case value.kind == kindNumber && value.fbits == 32:
			current = "f32"
		case value.kind == kindNumber && value.fbits == 64:
			current = "f64"
		case value.kind == kindNumber:
			current = "number"
		case value.kind == kindUint || value.kind == kindInt || value.kind == kindDecimal:
			current = "number"
		default:
			return "", fmt.Errorf("future types require numeric elements, got %s", value.Kind())
		}
		if result == "" {
			result = current
		} else if result != current {
			if result == "number" || current == "number" {
				result = "number"
			} else {
				return "", fmt.Errorf("future type elements must retain one width")
			}
		}
	}
	return result, nil
}

func futureScalar(value Value, element string) (Value, error) {
	if value.bfloat || (value.kind != kindNumber && value.kind != kindUint && value.kind != kindInt && value.kind != kindDecimal) {
		return nullValue(), fmt.Errorf("future types require numeric elements")
	}
	n, err := value.AsNumber()
	if err != nil {
		return nullValue(), err
	}
	switch element {
	case "number":
		return numberValue(n), nil
	case "f32":
		return floatValue(n, 32), nil
	case "f64":
		return floatValue(n, 64), nil
	default:
		return nullValue(), fmt.Errorf("unsupported future element type %q", element)
	}
}

func futureScalarBinary(op tokenKind, left, right Value, element string) (Value, error) {
	l, err := left.AsNumber()
	if err != nil {
		return nullValue(), err
	}
	r, err := right.AsNumber()
	if err != nil {
		return nullValue(), err
	}
	var result float64
	switch op {
	case tokenPlus:
		result = l + r
	case tokenMinus:
		result = l - r
	case tokenStar:
		result = l * r
	case tokenSlash:
		if r == 0 {
			return nullValue(), fmt.Errorf("future numeric division by zero")
		}
		result = l / r
	default:
		return nullValue(), fmt.Errorf("operator %s is not defined for future values", op)
	}
	return futureScalar(numberValue(result), element)
}

func futureValuesFromArgs(args []Value) ([]Value, error) {
	if len(args) == 1 && (args[0].kind == kindList || args[0].kind == kindTuple) {
		if args[0].kind == kindList {
			return append([]Value(nil), args[0].list...), nil
		}
		return append([]Value(nil), args[0].tuple...), nil
	}
	if len(args) == 1 && args[0].kind == kindTensor {
		return append([]Value(nil), args[0].data...), nil
	}
	return append([]Value(nil), args...), nil
}

func futurePolyValue(args []Value) (Value, error) {
	values, err := futureValuesFromArgs(args)
	if err != nil {
		return nullValue(), err
	}
	element, err := futureElementType(values)
	if err != nil {
		return nullValue(), err
	}
	coeffs := make([]Value, len(values))
	for i, value := range values {
		coeffs[i], err = futureScalar(value, element)
		if err != nil {
			return nullValue(), err
		}
	}
	coeffs = futureTrimCoefficients(coeffs)
	return newFutureValue("poly", []string{element}, coeffs, nil, nil, ""), nil
}

func futureTrimCoefficients(values []Value) []Value {
	if len(values) == 0 {
		return nil
	}
	out := append([]Value(nil), values...)
	for len(out) > 1 && futureScalarIsZero(out[len(out)-1]) {
		out = out[:len(out)-1]
	}
	if len(out) == 1 && futureScalarIsZero(out[0]) {
		return nil
	}
	return out
}

func futureScalarIsZero(value Value) bool {
	n, err := value.AsNumber()
	return err == nil && n == 0
}

func futureQuatValue(args []Value) (Value, error) {
	values, err := futureValuesFromArgs(args)
	if err != nil {
		return nullValue(), err
	}
	if len(values) != 4 {
		return nullValue(), fmt.Errorf("quat expects exactly four components")
	}
	element, err := futureElementType(values)
	if err != nil {
		return nullValue(), err
	}
	components := make([]Value, 4)
	for i, value := range values {
		components[i], err = futureScalar(value, element)
		if err != nil {
			return nullValue(), err
		}
	}
	return newFutureValue("quat", []string{element}, components, []int{4}, nil, ""), nil
}

func futureSIMDValue(args []Value) (Value, error) {
	if len(args) == 0 || len(args) > 2 {
		return nullValue(), fmt.Errorf("simd expects lane values or lane count and values")
	}
	lanes := 0
	var values []Value
	if len(args) == 2 {
		var err error
		lanes, err = args[0].Int()
		if err != nil {
			return nullValue(), fmt.Errorf("simd lane count must be an integer")
		}
		values, err = futureValuesFromArgs([]Value{args[1]})
		if err != nil {
			return nullValue(), err
		}
	} else {
		values, _ = futureValuesFromArgs(args)
		lanes = len(values)
	}
	if lanes != len(values) {
		return nullValue(), fmt.Errorf("simd lane count %d does not match %d values", lanes, len(values))
	}
	element, err := futureElementType(values)
	if err != nil {
		return nullValue(), err
	}
	_, params, err := futureTypeParts("simd", []string{strconv.Itoa(lanes), element})
	if err != nil {
		return nullValue(), err
	}
	lanesOut := make([]Value, lanes)
	for i, value := range values {
		lanesOut[i], err = futureScalar(value, element)
		if err != nil {
			return nullValue(), err
		}
	}
	return newFutureValue("simd", params, lanesOut, []int{lanes}, nil, ""), nil
}

func futureSIMDMask(args []Value) (Value, error) {
	if len(args) == 0 || len(args) > 2 {
		return nullValue(), fmt.Errorf("simd mask expects lane values or lane count and values")
	}
	lanes := 0
	var values []Value
	if len(args) == 2 {
		var err error
		lanes, err = args[0].Int()
		if err != nil {
			return nullValue(), fmt.Errorf("simd mask lane count must be an integer")
		}
		values, err = futureValuesFromArgs([]Value{args[1]})
		if err != nil {
			return nullValue(), err
		}
	} else {
		values, _ = futureValuesFromArgs(args)
		lanes = len(values)
	}
	if lanes != len(values) {
		return nullValue(), fmt.Errorf("simd mask lane count %d does not match %d values", lanes, len(values))
	}
	if _, _, err := futureTypeParts("simd", []string{strconv.Itoa(lanes), "bool"}); err != nil {
		return nullValue(), err
	}
	for _, value := range values {
		if value.kind != kindBool {
			return nullValue(), fmt.Errorf("simd masks require boolean lanes")
		}
	}
	return newFutureValue("simd", []string{strconv.Itoa(lanes), "bool"}, values, []int{lanes}, nil, ""), nil
}

func futureSparseValue(name string, args []Value) (Value, error) {
	if len(args) < 2 || len(args) > 4 {
		return nullValue(), fmt.Errorf("%s expects shape, entries, and optional format", name)
	}
	entriesAt := 1
	formatAt := 2
	shapeValue := args[0]
	if name == "sparse_matrix" && shapeValue.kind != kindList {
		if len(args) < 3 {
			return nullValue(), fmt.Errorf("sparse_matrix expects rows, columns, and entries")
		}
		shapeValue = listValue([]Value{args[0], args[1]})
		entriesAt = 2
		formatAt = 3
	}
	if shapeValue.kind != kindList {
		return nullValue(), fmt.Errorf("%s shape must be a list of dimensions", name)
	}
	shape := make([]int, len(shapeValue.list))
	for i, item := range shapeValue.list {
		dim, err := item.Int()
		if err != nil || dim < 0 {
			return nullValue(), fmt.Errorf("%s shape dimensions must be non-negative integers", name)
		}
		shape[i] = dim
	}
	if name == "sparse_matrix" && len(shape) != 2 {
		return nullValue(), fmt.Errorf("sparse_matrix requires a two-dimensional shape")
	}
	if name == "sparse_tensor" && len(shape) == 0 {
		return nullValue(), fmt.Errorf("sparse_tensor requires at least one dimension")
	}
	format := "coo"
	if name == "sparse_matrix" {
		format = "csr"
	}
	if len(args) > formatAt {
		if args[formatAt].kind != kindString {
			return nullValue(), fmt.Errorf("sparse format must be a string")
		}
		format = args[formatAt].str
	}
	if name == "sparse_matrix" && format != "csr" && format != "csc" {
		return nullValue(), fmt.Errorf("sparse_matrix format must be csr or csc")
	}
	if name == "sparse_tensor" && format != "coo" {
		return nullValue(), fmt.Errorf("sparse_tensor format must be coo")
	}
	if args[entriesAt].kind != kindList {
		return nullValue(), fmt.Errorf("%s entries must be a list", name)
	}
	coords := make([][]int, 0, len(args[entriesAt].list))
	values := make([]Value, 0, len(args[entriesAt].list))
	element := ""
	for _, entry := range args[entriesAt].list {
		if entry.kind != kindList {
			return nullValue(), fmt.Errorf("%s entries must contain coordinates followed by one value", name)
		}
		coordinateValues := entry.list
		var sparseValue Value
		if len(entry.list) == 2 && entry.list[0].kind == kindList && len(entry.list[0].list) == len(shape) {
			coordinateValues = entry.list[0].list
			sparseValue = entry.list[1]
		} else if len(entry.list) == len(shape)+1 {
			sparseValue = entry.list[len(shape)]
		} else {
			return nullValue(), fmt.Errorf("%s entries must contain coordinates followed by one value", name)
		}
		coord := make([]int, len(shape))
		for i := range shape {
			index, err := coordinateValues[i].Int()
			if err != nil || index < 0 || index >= shape[i] {
				return nullValue(), fmt.Errorf("%s entry coordinate is out of range", name)
			}
			coord[i] = index
		}
		if element == "" {
			inferred, inferErr := futureElementType([]Value{sparseValue})
			if inferErr != nil {
				return nullValue(), inferErr
			}
			element = inferred
		}
		value, err := futureScalar(sparseValue, element)
		if err != nil {
			return nullValue(), err
		}
		found := -1
		for i := range coords {
			if equalIntSlices(coords[i], coord) {
				found = i
				break
			}
		}
		if found >= 0 {
			value, err = futureScalarBinary(tokenPlus, values[found], value, element)
			if err != nil {
				return nullValue(), err
			}
			values[found] = value
		} else {
			coords = append(coords, coord)
			values = append(values, value)
		}
	}
	if element == "" {
		element = "number"
	}
	order := make([]int, len(coords))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return futureCoordLess(coords[order[i]], coords[order[j]]) })
	sortedCoords := make([][]int, 0, len(order))
	sortedValues := make([]Value, 0, len(order))
	for _, index := range order {
		if futureScalarIsZero(values[index]) {
			continue
		}
		sortedCoords = append(sortedCoords, coords[index])
		sortedValues = append(sortedValues, values[index])
	}
	return newFutureValue(name, []string{element}, sortedValues, shape, sortedCoords, format), nil
}

func futureCoordLess(left, right []int) bool {
	for i := range left {
		if left[i] != right[i] {
			return left[i] < right[i]
		}
	}
	return false
}

func formatFutureCoord(coord []int) string {
	items := make([]string, len(coord))
	for i, value := range coord {
		items[i] = strconv.Itoa(value)
	}
	return "[" + strings.Join(items, ", ") + "]"
}

func futureBinary(op tokenKind, left, right Value) (Value, bool, error) {
	if left.kind != kindFuture && right.kind != kindFuture {
		return nullValue(), false, nil
	}
	if left.kind != kindFuture || right.kind != kindFuture || left.future == nil || right.future == nil {
		return nullValue(), true, fmt.Errorf("future values require matching future values")
	}
	if left.future.name != right.future.name || !futureStringSlicesEqual(left.future.params, right.future.params) {
		return nullValue(), true, fmt.Errorf("future values require the same family and parameters")
	}
	if op == tokenEqualEqual || op == tokenBangEqual {
		equal := left.Equal(right)
		if op == tokenBangEqual {
			equal = !equal
		}
		return boolValue(equal), true, nil
	}
	lv, rv := left.future, right.future
	switch lv.name {
	case "poly":
		if op != tokenPlus && op != tokenMinus && op != tokenStar {
			return nullValue(), true, fmt.Errorf("operator %s is not defined for polynomials", op)
		}
		return futurePolyBinary(op, lv, rv), true, nil
	case "quat":
		return futureQuatBinary(op, lv, rv)
	case "simd":
		if op != tokenPlus && op != tokenMinus && op != tokenStar && op != tokenSlash {
			return nullValue(), true, fmt.Errorf("operator %s is not defined for simd values", op)
		}
		if len(lv.data) != len(rv.data) {
			return nullValue(), true, fmt.Errorf("simd lane counts do not match")
		}
		out := make([]Value, len(lv.data))
		for i := range out {
			var err error
			out[i], err = futureScalarBinary(op, lv.data[i], rv.data[i], lv.params[1])
			if err != nil {
				return nullValue(), true, err
			}
		}
		return newFutureValue("simd", lv.params, out, lv.shape, nil, ""), true, nil
	case "sparse_matrix", "sparse_tensor":
		if op != tokenPlus && op != tokenMinus {
			return nullValue(), true, fmt.Errorf("operator %s is not defined for sparse values", op)
		}
		value, err := futureSparseBinary(op, lv, rv)
		return value, true, err
	default:
		return nullValue(), true, fmt.Errorf("unknown future value family %q", lv.name)
	}
}

func futureStringSlicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for i := range left {
		if left[i] != right[i] {
			return false
		}
	}
	return true
}

func futurePolyBinary(op tokenKind, left, right *futureValue) Value {
	if op == tokenStar {
		out := make([]Value, len(left.data)+len(right.data)-1)
		for i := range out {
			out[i] = numberValue(0)
		}
		for i, lv := range left.data {
			for j, rv := range right.data {
				part, _ := futureScalarBinary(tokenStar, lv, rv, left.params[0])
				out[i+j], _ = futureScalarBinary(tokenPlus, out[i+j], part, left.params[0])
			}
		}
		return newFutureValue("poly", left.params, futureTrimCoefficients(out), nil, nil, "")
	}
	n := len(left.data)
	if len(right.data) > n {
		n = len(right.data)
	}
	out := make([]Value, n)
	for i := range out {
		lv, rv := numberValue(0), numberValue(0)
		if i < len(left.data) {
			lv = left.data[i]
		}
		if i < len(right.data) {
			rv = right.data[i]
		}
		out[i], _ = futureScalarBinary(op, lv, rv, left.params[0])
	}
	return newFutureValue("poly", left.params, futureTrimCoefficients(out), nil, nil, "")
}

func futureQuatBinary(op tokenKind, left, right *futureValue) (Value, bool, error) {
	if op == tokenSlash {
		inverse, err := futureQuatInverse([]Value{newFutureValue("quat", right.params, right.data, right.shape, right.coords, right.format)})
		if err != nil {
			return nullValue(), true, err
		}
		return futureQuatBinary(tokenStar, left, inverse.future)
	}
	if op != tokenPlus && op != tokenMinus && op != tokenStar {
		return nullValue(), true, fmt.Errorf("operator %s is not defined for quaternions", op)
	}
	out := make([]Value, 4)
	if op == tokenStar {
		for i := 0; i < 4; i++ {
			out[i] = numberValue(0)
		}
		terms := [4][4]int{{0, -1, -2, -3}, {1, 0, 3, -2}, {2, -3, 0, 1}, {3, 2, -1, 0}}
		for i := range terms {
			for j, sign := range terms[i] {
				part, err := futureScalarBinary(tokenStar, left.data[j], right.data[absInt(sign)], left.params[0])
				if err != nil {
					return nullValue(), true, err
				}
				if sign < 0 {
					part, err = applyUnary(tokenMinus, part)
					if err != nil {
						return nullValue(), true, err
					}
				}
				out[i], err = futureScalarBinary(tokenPlus, out[i], part, left.params[0])
				if err != nil {
					return nullValue(), true, err
				}
			}
		}
	} else {
		for i := range out {
			var err error
			out[i], err = futureScalarBinary(op, left.data[i], right.data[i], left.params[0])
			if err != nil {
				return nullValue(), true, err
			}
		}
	}
	return newFutureValue("quat", left.params, out, []int{4}, nil, ""), true, nil
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func futureSparseBinary(op tokenKind, left, right *futureValue) (Value, error) {
	if !equalIntSlices(left.shape, right.shape) || left.format != right.format {
		return nullValue(), fmt.Errorf("sparse values require matching shape and format")
	}
	coords := make([][]int, 0, len(left.coords)+len(right.coords))
	values := make([]Value, 0, cap(coords))
	add := func(coord []int, value Value) error {
		for i := range coords {
			if equalIntSlices(coords[i], coord) {
				result, err := futureScalarBinary(op, values[i], value, left.params[0])
				if err != nil {
					return err
				}
				values[i] = result
				return nil
			}
		}
		coords = append(coords, append([]int(nil), coord...))
		values = append(values, value)
		return nil
	}
	for i := range left.coords {
		if err := add(left.coords[i], left.data[i]); err != nil {
			return nullValue(), err
		}
	}
	for i := range right.coords {
		value := right.data[i]
		if op == tokenMinus {
			var err error
			value, err = applyUnary(tokenMinus, value)
			if err != nil {
				return nullValue(), err
			}
		}
		if err := add(right.coords[i], value); err != nil {
			return nullValue(), err
		}
	}
	order := make([]int, len(coords))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return futureCoordLess(coords[order[i]], coords[order[j]]) })
	sortedCoords, sortedValues := make([][]int, 0, len(order)), make([]Value, 0, len(order))
	for _, index := range order {
		if !futureScalarIsZero(values[index]) {
			sortedCoords = append(sortedCoords, coords[index])
			sortedValues = append(sortedValues, values[index])
		}
	}
	return newFutureValue(left.name, left.params, sortedValues, left.shape, sortedCoords, left.format), nil
}

func futureIndex(v Value, index Value) (Value, error) {
	if v.kind != kindFuture || v.future == nil {
		return nullValue(), fmt.Errorf("future index requires a future value")
	}
	if v.future.name == "sparse_matrix" || v.future.name == "sparse_tensor" {
		if index.kind != kindList {
			return nullValue(), fmt.Errorf("sparse index requires a coordinate list")
		}
		coord := make([]int, len(index.list))
		if len(coord) != len(v.future.shape) {
			return nullValue(), fmt.Errorf("sparse index requires %d coordinates", len(v.future.shape))
		}
		for i, item := range index.list {
			value, err := item.Int()
			if err != nil {
				return nullValue(), err
			}
			coord[i] = value
			if value < 0 || value >= v.future.shape[i] {
				return nullValue(), fmt.Errorf("sparse coordinate out of range")
			}
		}
		for i := range v.future.coords {
			if equalIntSlices(v.future.coords[i], coord) {
				return v.future.data[i], nil
			}
		}
		return futureScalar(numberValue(0), v.future.params[0])
	}
	idx, err := index.Int()
	if err != nil || idx < 0 || idx >= len(v.future.data) {
		return nullValue(), fmt.Errorf("future index out of range")
	}
	return v.future.data[idx], nil
}

func futureSetIndex(v Value, index, replacement Value) (Value, error) {
	if v.kind != kindFuture || v.future == nil {
		return nullValue(), fmt.Errorf("future setat requires a future value")
	}
	var value Value
	var err error
	if v.future.params[len(v.future.params)-1] == "bool" {
		if replacement.kind != kindBool {
			return nullValue(), fmt.Errorf("simd mask replacement must be boolean")
		}
		value = replacement
	} else {
		value, err = futureScalar(replacement, v.future.params[len(v.future.params)-1])
		if err != nil {
			return nullValue(), err
		}
	}
	next := newFutureValue(v.future.name, v.future.params, v.future.data, v.future.shape, v.future.coords, v.future.format)
	if next.future.name == "sparse_matrix" || next.future.name == "sparse_tensor" {
		if index.kind != kindList {
			return nullValue(), fmt.Errorf("sparse setat requires a coordinate list")
		}
		coord := make([]int, len(index.list))
		for i, item := range index.list {
			coord[i], err = item.Int()
			if err != nil || coord[i] < 0 || i >= len(next.future.shape) || coord[i] >= next.future.shape[i] {
				return nullValue(), fmt.Errorf("sparse coordinate out of range")
			}
		}
		for i := range next.future.coords {
			if equalIntSlices(next.future.coords[i], coord) {
				if futureScalarIsZero(value) {
					next.future.coords = append(next.future.coords[:i], next.future.coords[i+1:]...)
					next.future.data = append(next.future.data[:i], next.future.data[i+1:]...)
				} else {
					next.future.data[i] = value
				}
				return next, nil
			}
		}
		if !futureScalarIsZero(value) {
			next.future.coords = append(next.future.coords, coord)
			next.future.data = append(next.future.data, value)
			order := make([]int, len(next.future.coords))
			for i := range order {
				order[i] = i
			}
			sort.Slice(order, func(i, j int) bool {
				return futureCoordLess(next.future.coords[order[i]], next.future.coords[order[j]])
			})
			sortedCoords, sortedData := make([][]int, len(order)), make([]Value, len(order))
			for i, at := range order {
				sortedCoords[i], sortedData[i] = next.future.coords[at], next.future.data[at]
			}
			next.future.coords, next.future.data = sortedCoords, sortedData
		}
		return next, nil
	}
	idx, err := index.Int()
	if err != nil || idx < 0 || idx >= len(next.future.data) {
		return nullValue(), fmt.Errorf("future setat index out of range")
	}
	next.future.data[idx] = value
	if next.future.name == "poly" {
		next.future.data = futureTrimCoefficients(next.future.data)
	}
	return next, nil
}

func futureUnary(op tokenKind, value Value) (Value, error) {
	if value.kind != kindFuture || value.future == nil {
		return nullValue(), fmt.Errorf("future unary operator requires a future value")
	}
	if op != tokenMinus {
		return nullValue(), fmt.Errorf("operator %s is not defined for future values", op)
	}
	if value.future.params[len(value.future.params)-1] == "bool" {
		return nullValue(), fmt.Errorf("simd masks do not support numeric unary operators")
	}
	out := make([]Value, len(value.future.data))
	for i, item := range value.future.data {
		var err error
		out[i], err = applyUnary(tokenMinus, item)
		if err != nil {
			return nullValue(), err
		}
	}
	return newFutureValue(value.future.name, value.future.params, out, value.future.shape, value.future.coords, value.future.format), nil
}

func futureLength(value Value) (Value, error) {
	if value.kind != kindFuture || value.future == nil {
		return nullValue(), fmt.Errorf("len expects a future value")
	}
	return uintValue(uint64(len(value.future.data)), 64), nil
}

func futureShape(value Value) (Value, error) {
	if value.kind != kindFuture || value.future == nil {
		return nullValue(), fmt.Errorf("shape expects a future value")
	}
	shape := value.future.shape
	if value.future.name == "poly" {
		shape = []int{len(value.future.data)}
	}
	if value.future.name == "quat" {
		shape = []int{4}
	}
	out := make([]Value, len(shape))
	for i, dim := range shape {
		out[i] = uintValue(uint64(dim), 64)
	}
	return listValue(out), nil
}

func futureConjugate(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || args[0].future.name != "quat" {
		return nullValue(), fmt.Errorf("quat_conjugate expects one quaternion")
	}
	data := append([]Value(nil), args[0].future.data...)
	data[1], _ = applyUnary(tokenMinus, data[1])
	data[2], _ = applyUnary(tokenMinus, data[2])
	data[3], _ = applyUnary(tokenMinus, data[3])
	return newFutureValue("quat", args[0].future.params, data, []int{4}, nil, ""), nil
}

func futureQuatNorm(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || args[0].future.name != "quat" {
		return nullValue(), fmt.Errorf("quat_norm expects one quaternion")
	}
	total := 0.0
	for _, item := range args[0].future.data {
		n, err := item.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n * n
	}
	return futureScalar(numberValue(math.Sqrt(total)), args[0].future.params[0])
}

func futureQuatInverse(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || args[0].future.name != "quat" {
		return nullValue(), fmt.Errorf("quat_inverse expects one quaternion")
	}
	total := 0.0
	for _, item := range args[0].future.data {
		n, err := item.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		total += n * n
	}
	if total == 0 {
		return nullValue(), fmt.Errorf("cannot invert a zero quaternion")
	}
	conjugate, err := futureConjugate(args)
	if err != nil {
		return nullValue(), err
	}
	data := make([]Value, len(conjugate.future.data))
	denom, err := futureScalar(numberValue(total), args[0].future.params[0])
	if err != nil {
		return nullValue(), err
	}
	for i, item := range conjugate.future.data {
		data[i], err = futureScalarBinary(tokenSlash, item, denom, args[0].future.params[0])
		if err != nil {
			return nullValue(), err
		}
	}
	return newFutureValue("quat", args[0].future.params, data, []int{4}, nil, ""), nil
}

func futureSIMDSelect(args []Value) (Value, error) {
	if len(args) != 3 || args[0].kind != kindFuture || args[1].kind != kindFuture || args[2].kind != kindFuture || args[0].future == nil || args[1].future == nil || args[2].future == nil {
		return nullValue(), fmt.Errorf("simd_select expects a mask and two simd values")
	}
	mask, left, right := args[0].future, args[1].future, args[2].future
	if mask.name != "simd" || len(mask.params) != 2 || mask.params[1] != "bool" || left.name != "simd" || right.name != "simd" || !futureStringSlicesEqual(left.params, right.params) || len(mask.data) != len(left.data) || len(left.data) != len(right.data) {
		return nullValue(), fmt.Errorf("simd_select requires a matching boolean mask and simd values")
	}
	return newFutureValue("simd", left.params, func() []Value {
		out := make([]Value, len(left.data))
		for i := range out {
			if mask.data[i].Truthy() {
				out[i] = left.data[i]
			} else {
				out[i] = right.data[i]
			}
		}
		return out
	}(), left.shape, nil, ""), nil
}

func futureNNZ(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || (args[0].future.name != "sparse_matrix" && args[0].future.name != "sparse_tensor") {
		return nullValue(), fmt.Errorf("nnz expects one sparse matrix or tensor")
	}
	return uintValue(uint64(len(args[0].future.data)), 64), nil
}

func futureToDense(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || (args[0].future.name != "sparse_matrix" && args[0].future.name != "sparse_tensor") {
		return nullValue(), fmt.Errorf("to_dense expects one sparse matrix or tensor")
	}
	v := args[0].future
	total := tensorShapeProduct(v.shape)
	zero, err := futureScalar(numberValue(0), v.params[0])
	if err != nil {
		return nullValue(), err
	}
	data := make([]Value, total)
	for i := range data {
		data[i] = zero
	}
	strides := tensorShapeStrides(v.shape)
	for i, coord := range v.coords {
		flat := 0
		for j, value := range coord {
			flat += value * strides[j]
		}
		data[flat] = v.data[i]
	}
	return newTensorValue(v.shape, data), nil
}

func futureToSparse(args []Value) (Value, error) {
	if len(args) < 1 || len(args) > 2 {
		return nullValue(), fmt.Errorf("to_sparse expects a tensor and optional format")
	}
	if args[0].kind != kindTensor {
		return nullValue(), fmt.Errorf("to_sparse expects a dense tensor")
	}
	tensor := args[0]
	name, format := "sparse_tensor", "coo"
	if len(tensor.shape) == 2 {
		name, format = "sparse_matrix", "csr"
	}
	if len(args) == 2 {
		if args[1].kind != kindString {
			return nullValue(), fmt.Errorf("sparse format must be a string")
		}
		format = args[1].str
	}
	element, err := futureElementType(tensor.data)
	if err != nil {
		return nullValue(), err
	}
	entries := make([]Value, 0)
	strides := tensorShapeStrides(tensor.shape)
	for flat, item := range tensor.data {
		if futureScalarIsZero(item) {
			continue
		}
		coord := tensorFlatToCoords(flat, tensor.shape, strides)
		row := make([]Value, 0, len(coord)+1)
		for _, value := range coord {
			row = append(row, uintValue(uint64(value), 64))
		}
		converted, err := futureScalar(item, element)
		if err != nil {
			return nullValue(), err
		}
		row = append(row, converted)
		entries = append(entries, listValue(row))
	}
	shape := make([]Value, len(tensor.shape))
	for i, dim := range tensor.shape {
		shape[i] = uintValue(uint64(dim), 64)
	}
	return futureSparseValue(name, []Value{listValue(shape), listValue(entries), stringValue(format)})
}

func futurePolyDegree(args []Value) (Value, error) {
	if len(args) != 1 || args[0].kind != kindFuture || args[0].future == nil || args[0].future.name != "poly" {
		return nullValue(), fmt.Errorf("poly_degree expects one polynomial")
	}
	return intValue(new(big.Int).SetInt64(int64(len(args[0].future.data)-1)), 64), nil
}

func futurePolyEval(args []Value) (Value, error) {
	if len(args) != 2 || args[0].kind != kindFuture || args[0].future == nil || args[0].future.name != "poly" {
		return nullValue(), fmt.Errorf("poly_eval expects a polynomial and a number")
	}
	x, err := futureScalar(args[1], args[0].future.params[0])
	if err != nil {
		return nullValue(), err
	}
	result := numberValue(0)
	for i := len(args[0].future.data) - 1; i >= 0; i-- {
		if i != len(args[0].future.data)-1 {
			result, err = futureScalarBinary(tokenStar, result, x, args[0].future.params[0])
			if err != nil {
				return nullValue(), err
			}
		}
		result, err = futureScalarBinary(tokenPlus, result, args[0].future.data[i], args[0].future.params[0])
		if err != nil {
			return nullValue(), err
		}
	}
	return result, nil
}

func rejectTask134FutureBackend(prog *Program, backend string) error {
	if prog == nil {
		return nil
	}
	for _, stmt := range prog.Statements {
		if name, span, ok := futureStmtUse(stmt); ok {
			line, col := span.start.line, span.start.col
			if line == 0 {
				line, col = 1, 1
			}
			return fmt.Errorf("%s:%d:%d: %s backend: unsupported reserved type %q; use interpreter or VM", prog.SourceName, line, col, backend, name)
		}
	}
	return nil
}

func futureTypeUse(text string) string {
	base := strings.TrimSpace(text)
	if open := strings.IndexByte(base, '<'); open >= 0 {
		base = strings.TrimSpace(base[:open])
	}
	switch base {
	case "poly", "quat", "simd", "sparse_matrix", "sparse_tensor":
		return base
	default:
		return ""
	}
}

func futureCallUse(name string) string {
	switch name {
	case "poly", "poly_degree", "poly_eval", "poly_divmod", "quat", "quat_conjugate", "quat_norm", "quat_inverse", "simd", "simd_mask", "simd_select", "sparse_matrix", "sparse_tensor", "to_dense", "to_sparse", "nnz":
		if strings.HasPrefix(name, "poly") {
			return "poly"
		}
		if strings.HasPrefix(name, "quat") {
			return "quat"
		}
		if strings.HasPrefix(name, "simd") {
			return "simd"
		}
		if strings.HasPrefix(name, "sparse_matrix") || name == "to_dense" || name == "to_sparse" || name == "nnz" {
			return "sparse_matrix"
		}
		return "sparse_tensor"
	default:
		return ""
	}
}

func futureStmtUse(stmt Stmt) (string, sourceSpan, bool) {
	switch n := stmt.(type) {
	case letStmt:
		if name := futureTypeUse(n.Type); name != "" {
			return name, n.Span, true
		}
		return futureExprUse(n.Expr)
	case assignStmt:
		return futureExprUse(n.Expr)
	case emitStmt:
		return futureExprUse(n.Expr)
	case deferStmt:
		return futureExprUse(n.Body)
	case funcStmt:
		for _, typ := range append(append([]string{n.ReceiverType, n.ReturnType}, n.ParamTypes...), "") {
			if name := futureTypeUse(typ); name != "" {
				return name, n.Span, true
			}
		}
		return futureExprUse(n.Body)
	case returnStmt:
		return futureExprUse(n.Expr)
	case whileStmt:
		if name, span, ok := futureExprUse(n.Cond); ok {
			return name, span, true
		}
		return futureExprUse(n.Body)
	case forStmt:
		if name, span, ok := futureExprUse(n.Iter); ok {
			return name, span, true
		}
		return futureExprUse(n.Body)
	case macroStmt:
		return futureExprUse(n.Body)
	case traitStmt:
		for _, method := range n.Methods {
			if name, span, ok := futureStmtUse(method); ok {
				return name, span, true
			}
		}
	case implStmt:
		if name := futureTypeUse(n.Target); name != "" {
			return name, n.Span, true
		}
		for _, method := range n.Methods {
			if name, span, ok := futureStmtUse(method); ok {
				return name, span, true
			}
		}
	case structStmt:
		for _, field := range n.Fields {
			if name := futureTypeUse(field.Type); name != "" {
				return name, field.Span, true
			}
		}
	}
	return "", sourceSpan{}, false
}

func futureExprUse(expr Expr) (string, sourceSpan, bool) {
	switch n := expr.(type) {
	case nil:
		return "", sourceSpan{}, false
	case *callExpr:
		if ident, ok := n.callee.(*identExpr); ok {
			if name := futureCallUse(ident.name); name != "" {
				return name, n.Span, true
			}
		}
		if member, ok := n.callee.(*memberExpr); ok {
			if name := futureCallUse(member.name); name != "" {
				return name, n.Span, true
			}
		}
		if name, span, ok := futureExprUse(n.callee); ok {
			return name, span, true
		}
		for _, arg := range n.args {
			if name, span, ok := futureExprUse(arg); ok {
				return name, span, true
			}
		}
	case *binaryExpr:
		if name, span, ok := futureExprUse(n.left); ok {
			return name, span, true
		}
		return futureExprUse(n.right)
	case *unaryExpr:
		return futureExprUse(n.expr)
	case *memberExpr:
		return futureExprUse(n.object)
	case *indexExpr:
		if name, span, ok := futureExprUse(n.Object); ok {
			return name, span, true
		}
		return futureExprUse(n.Index)
	case *listExpr:
		for _, item := range n.items {
			if name, span, ok := futureExprUse(item); ok {
				return name, span, true
			}
		}
	case *tupleExpr:
		for _, item := range n.items {
			if name, span, ok := futureExprUse(item); ok {
				return name, span, true
			}
		}
	case *blockExpr:
		for _, stmt := range n.stmts {
			if name, span, ok := futureStmtUse(stmt); ok {
				return name, span, true
			}
		}
		return futureExprUse(n.tail)
	case *tryExpr:
		if name, span, ok := futureExprUse(n.body); ok {
			return name, span, true
		}
		return futureExprUse(n.handler)
	case *propagateExpr:
		return futureExprUse(n.expr)
	case *structExpr:
		if name := futureTypeUse(n.TypeName); name != "" {
			return name, n.Span, true
		}
		for _, field := range n.Fields {
			if name, span, ok := futureExprUse(field.Expr); ok {
				return name, span, true
			}
		}
	case *structUpdateExpr:
		if name, span, ok := futureExprUse(n.Base); ok {
			return name, span, true
		}
		for _, field := range n.Fields {
			if name, span, ok := futureExprUse(field.Expr); ok {
				return name, span, true
			}
		}
	case *matchExpr:
		if name, span, ok := futureExprUse(n.subject); ok {
			return name, span, true
		}
		for _, arm := range n.arms {
			if name, span, ok := futureExprUse(arm.guard); ok {
				return name, span, true
			}
			if name, span, ok := futureExprUse(arm.body); ok {
				return name, span, true
			}
		}
	}
	return "", sourceSpan{}, false
}
