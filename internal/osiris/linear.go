package osiris

import (
	"fmt"
	"math"
	"math/cmplx"
	"sort"
)

// valueSliceToFloat64s handles the value slice to float64s step in the
// surrounding package and returns []float64, error.
//
// It returns []float64 and an error. Callers should only trust the value when
// the error is nil.
func valueSliceToFloat64s(values []Value) ([]float64, error) {
	out := make([]float64, len(values))
	for i := range values {
		n, err := values[i].AsNumber()
		if err != nil {
			return nil, err
		}
		out[i] = n
	}
	return out, nil
}

// matrixValues handles the matrix values step in the surrounding package and
// returns []int, [][]float64, error.
//
// It returns []int, [][]float64, error so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func matrixValues(v Value) ([]int, [][]float64, error) {
	shape, data, err := inferTensor(v)
	if err != nil {
		return nil, nil, err
	}
	if len(shape) == 1 {
		row, err := valueSliceToFloat64s(data)
		if err != nil {
			return nil, nil, err
		}
		return []int{1, len(row)}, [][]float64{row}, nil
	}
	if len(shape) != 2 {
		return nil, nil, fmt.Errorf("expected a matrix")
	}
	rows, cols := shape[0], shape[1]
	if len(data) != rows*cols {
		return nil, nil, fmt.Errorf("expected a matrix")
	}
	mat := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		row := make([]float64, cols)
		for c := 0; c < cols; c++ {
			n, err := data[r*cols+c].AsNumber()
			if err != nil {
				return nil, nil, err
			}
			row[c] = n
		}
		mat[r] = row
	}
	return shape, mat, nil
}

// complexMatrixValues converts a rank-two tensor into a dense complex matrix.
// Real numeric elements are promoted with a zero imaginary component.
func complexMatrixValues(v Value) ([]int, [][]complex128, error) {
	shape, data, err := inferTensor(v)
	if err != nil {
		return nil, nil, err
	}
	if len(shape) != 2 || len(data) != shape[0]*shape[1] {
		return nil, nil, fmt.Errorf("expected a matrix")
	}
	mat := make([][]complex128, shape[0])
	for row := range mat {
		mat[row] = make([]complex128, shape[1])
		for col := range mat[row] {
			cell := data[row*shape[1]+col]
			if cell.kind == kindComplex {
				mat[row][col] = cell.cx
				continue
			}
			n, err := cell.AsNumber()
			if err != nil {
				return nil, nil, err
			}
			mat[row][col] = complex(n, 0)
		}
	}
	return shape, mat, nil
}

func matrixContainsComplex(v Value) (bool, error) {
	_, data, err := inferTensor(v)
	if err != nil {
		return false, err
	}
	for _, cell := range data {
		if cell.kind == kindComplex {
			return true, nil
		}
	}
	return false, nil
}

// vectorValues handles the vector values step in the surrounding package and
// returns []float64, error.
//
// It returns []float64 and an error. Callers should only trust the value when
// the error is nil.
func vectorValues(v Value) ([]float64, error) {
	_, data, err := inferTensor(v)
	if err != nil {
		return nil, err
	}
	return valueSliceToFloat64s(data)
}

// floatMatrixValue handles the float matrix value step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func floatMatrixValue(mat [][]float64) (Value, error) {
	rows := len(mat)
	cols := 0
	if rows > 0 {
		cols = len(mat[0])
	}
	data := make([]Value, 0, rows*cols)
	for _, row := range mat {
		if len(row) != cols {
			return nullValue(), fmt.Errorf("matrix rows must have equal length")
		}
		for _, n := range row {
			data = append(data, numberValue(n))
		}
	}
	return tensorFromShapeAndData([]int{rows, cols}, data)
}

// outerValue handles the outer value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func outerValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("outer expects 2 arguments")
	}
	_, left, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	_, right, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, 0, len(left)*len(right))
	complexResult := false
	for _, value := range append(append([]Value(nil), left...), right...) {
		complexResult = complexResult || value.kind == kindComplex
	}
	for _, ln := range left {
		for _, rn := range right {
			leftNumber, err := complexFromValue(ln)
			if err != nil {
				return nullValue(), err
			}
			rightNumber, err := complexFromValue(rn)
			if err != nil {
				return nullValue(), err
			}
			product := leftNumber * rightNumber
			if complexResult {
				out = append(out, complexValue(product))
			} else {
				out = append(out, numberValue(real(product)))
			}
		}
	}
	return tensorFromShapeAndData([]int{len(left), len(right)}, out)
}

// traceValue handles the trace value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func traceValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("trace expects 1 argument")
	}
	containsComplex, err := matrixContainsComplex(args[0])
	if err != nil {
		return nullValue(), err
	}
	if containsComplex {
		shape, mat, err := complexMatrixValues(args[0])
		if err != nil {
			return nullValue(), err
		}
		if len(shape) != 2 {
			return nullValue(), fmt.Errorf("trace expects a matrix")
		}
		limit := shape[0]
		if shape[1] < limit {
			limit = shape[1]
		}
		total := complex(0, 0)
		for i := 0; i < limit; i++ {
			total += mat[i][i]
		}
		return complexValue(total), nil
	}
	shape, mat, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 {
		return nullValue(), fmt.Errorf("trace expects a matrix")
	}
	rows, cols := shape[0], shape[1]
	lim := rows
	if cols < lim {
		lim = cols
	}
	total := 0.0
	for i := 0; i < lim; i++ {
		total += mat[i][i]
	}
	return numberValue(total), nil
}

// detValue handles the det value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func detValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("det expects 1 argument")
	}
	containsComplex, err := matrixContainsComplex(args[0])
	if err != nil {
		return nullValue(), err
	}
	if containsComplex {
		shape, mat, err := complexMatrixValues(args[0])
		if err != nil {
			return nullValue(), err
		}
		if len(shape) != 2 || shape[0] != shape[1] {
			return nullValue(), fmt.Errorf("det expects a square matrix")
		}
		n := shape[0]
		if n == 0 {
			return complexValue(1), nil
		}
		a := make([][]complex128, n)
		for i := range mat {
			a[i] = append([]complex128(nil), mat[i]...)
		}
		det := complex(1, 0)
		for i := 0; i < n; i++ {
			pivot := i
			maxAbs := cmplx.Abs(a[i][i])
			for row := i + 1; row < n; row++ {
				if current := cmplx.Abs(a[row][i]); current > maxAbs {
					maxAbs = current
					pivot = row
				}
			}
			if maxAbs == 0 {
				return complexValue(0), nil
			}
			if pivot != i {
				a[i], a[pivot] = a[pivot], a[i]
				det = -det
			}
			piv := a[i][i]
			det *= piv
			for row := i + 1; row < n; row++ {
				factor := a[row][i] / piv
				for col := i; col < n; col++ {
					a[row][col] -= factor * a[i][col]
				}
			}
		}
		return complexValue(det), nil
	}
	shape, mat, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("det expects a square matrix")
	}
	n := shape[0]
	if n == 0 {
		return numberValue(1), nil
	}
	a := make([][]float64, n)
	for i := range mat {
		a[i] = append([]float64(nil), mat[i]...)
	}
	sign := 1.0
	det := 1.0
	for i := 0; i < n; i++ {
		pivot := i
		maxAbs := math.Abs(a[i][i])
		for r := i + 1; r < n; r++ {
			if cur := math.Abs(a[r][i]); cur > maxAbs {
				maxAbs = cur
				pivot = r
			}
		}
		if maxAbs == 0 {
			return numberValue(0), nil
		}
		if pivot != i {
			a[i], a[pivot] = a[pivot], a[i]
			sign = -sign
		}
		piv := a[i][i]
		det *= piv
		for r := i + 1; r < n; r++ {
			factor := a[r][i] / piv
			for c := i; c < n; c++ {
				a[r][c] -= factor * a[i][c]
			}
		}
	}
	return numberValue(sign * det), nil
}

// solveValue handles the solve value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func solveValue(args []Value) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("solve expects 2 arguments")
	}
	complexMatrix, err := matrixContainsComplex(args[0])
	if err != nil {
		return nullValue(), err
	}
	complexRHS, err := matrixContainsComplex(args[1])
	if err != nil {
		return nullValue(), err
	}
	if complexMatrix || complexRHS {
		return complexSolveValue(args)
	}
	ashape, a, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(ashape) != 2 || ashape[0] != ashape[1] {
		return nullValue(), fmt.Errorf("solve expects a square matrix")
	}
	rhsShape, rhsData, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	n := len(a)
	var rhsMat [][]float64
	switch len(rhsShape) {
	case 0, 1:
		if len(rhsData) != n {
			return nullValue(), fmt.Errorf("solve dimension mismatch")
		}
		vec, err := valueSliceToFloat64s(rhsData)
		if err != nil {
			return nullValue(), err
		}
		rhsMat = make([][]float64, n)
		for i := range rhsMat {
			rhsMat[i] = []float64{vec[i]}
		}
	case 2:
		if rhsShape[0] != n {
			return nullValue(), fmt.Errorf("solve dimension mismatch")
		}
		rhsMat = make([][]float64, n)
		cols := rhsShape[1]
		for i := 0; i < n; i++ {
			rhsMat[i] = make([]float64, cols)
			for j := 0; j < cols; j++ {
				nv, err := rhsData[i*cols+j].AsNumber()
				if err != nil {
					return nullValue(), err
				}
				rhsMat[i][j] = nv
			}
		}
	default:
		return nullValue(), fmt.Errorf("solve rhs must be a vector or matrix")
	}
	m := len(rhsMat[0])
	aug := make([][]float64, n)
	for i := 0; i < n; i++ {
		aug[i] = make([]float64, n+m)
		copy(aug[i], a[i])
		copy(aug[i][n:], rhsMat[i])
	}
	for i := 0; i < n; i++ {
		pivot := i
		maxAbs := math.Abs(aug[i][i])
		for r := i + 1; r < n; r++ {
			if cur := math.Abs(aug[r][i]); cur > maxAbs {
				maxAbs = cur
				pivot = r
			}
		}
		if maxAbs == 0 {
			return nullValue(), fmt.Errorf("solve matrix is singular")
		}
		if pivot != i {
			aug[i], aug[pivot] = aug[pivot], aug[i]
		}
		piv := aug[i][i]
		for c := i; c < n+m; c++ {
			aug[i][c] /= piv
		}
		for r := 0; r < n; r++ {
			if r == i {
				continue
			}
			factor := aug[r][i]
			for c := i; c < n+m; c++ {
				aug[r][c] -= factor * aug[i][c]
			}
		}
	}
	out := make([]Value, 0, n*m)
	for i := 0; i < n; i++ {
		for j := 0; j < m; j++ {
			out = append(out, numberValue(aug[i][n+j]))
		}
	}
	if m == 1 {
		return tensorFromShapeAndData([]int{n}, out)
	}
	return tensorFromShapeAndData([]int{n, m}, out)
}

func complexSolveValue(args []Value) (Value, error) {
	ashape, a, err := complexMatrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(ashape) != 2 || ashape[0] != ashape[1] {
		return nullValue(), fmt.Errorf("solve expects a square matrix")
	}
	rhsShape, rhsData, err := inferTensor(args[1])
	if err != nil {
		return nullValue(), err
	}
	n := len(a)
	m := 1
	switch len(rhsShape) {
	case 0, 1:
		if len(rhsData) != n {
			return nullValue(), fmt.Errorf("solve dimension mismatch")
		}
	case 2:
		if rhsShape[0] != n {
			return nullValue(), fmt.Errorf("solve dimension mismatch")
		}
		m = rhsShape[1]
	default:
		return nullValue(), fmt.Errorf("solve rhs must be a vector or matrix")
	}
	aug := make([][]complex128, n)
	for row := 0; row < n; row++ {
		aug[row] = make([]complex128, n+m)
		copy(aug[row], a[row])
		for col := 0; col < m; col++ {
			index := row
			if len(rhsShape) == 2 {
				index = row*m + col
			}
			value, err := complexFromValue(rhsData[index])
			if err != nil {
				return nullValue(), err
			}
			aug[row][n+col] = value
		}
	}
	for i := 0; i < n; i++ {
		pivot := i
		maxAbs := cmplx.Abs(aug[i][i])
		for row := i + 1; row < n; row++ {
			if current := cmplx.Abs(aug[row][i]); current > maxAbs {
				maxAbs = current
				pivot = row
			}
		}
		if maxAbs == 0 {
			return nullValue(), fmt.Errorf("solve matrix is singular")
		}
		if pivot != i {
			aug[i], aug[pivot] = aug[pivot], aug[i]
		}
		piv := aug[i][i]
		for col := i; col < n+m; col++ {
			aug[i][col] /= piv
		}
		for row := 0; row < n; row++ {
			if row == i {
				continue
			}
			factor := aug[row][i]
			for col := i; col < n+m; col++ {
				aug[row][col] -= factor * aug[i][col]
			}
		}
	}
	out := make([]Value, 0, n*m)
	for row := 0; row < n; row++ {
		for col := 0; col < m; col++ {
			out = append(out, complexValue(aug[row][n+col]))
		}
	}
	if m == 1 {
		return tensorFromShapeAndData([]int{n}, out)
	}
	return tensorFromShapeAndData([]int{n, m}, out)
}

// invValue handles the inv value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func invValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("inv expects 1 argument")
	}
	shape, _, err := inferTensor(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("inv expects a square matrix")
	}
	n := shape[0]
	eye := make([]Value, 0, n*n)
	for r := 0; r < n; r++ {
		for c := 0; c < n; c++ {
			if r == c {
				eye = append(eye, numberValue(1))
			} else {
				eye = append(eye, numberValue(0))
			}
		}
	}
	return solveValue([]Value{args[0], newTensorValue([]int{n, n}, eye)})
}

// qrValue handles the qr value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func qrValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("qr expects 1 argument")
	}
	complexMatrix, err := matrixContainsComplex(args[0])
	if err != nil {
		return nullValue(), err
	}
	if complexMatrix {
		return complexQRValue(args[0])
	}
	shape, a, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 {
		return nullValue(), fmt.Errorf("qr expects a matrix")
	}
	m, n := shape[0], shape[1]
	q := make([][]float64, m)
	for i := range q {
		q[i] = make([]float64, n)
	}
	r := make([][]float64, n)
	for i := range r {
		r[i] = make([]float64, n)
	}
	cols := make([][]float64, n)
	for j := 0; j < n; j++ {
		col := make([]float64, m)
		for i := 0; i < m; i++ {
			col[i] = a[i][j]
		}
		cols[j] = col
	}
	for j := 0; j < n; j++ {
		v := append([]float64(nil), cols[j]...)
		for i := 0; i < j; i++ {
			var dot float64
			for k := 0; k < m; k++ {
				dot += q[k][i] * cols[j][k]
			}
			r[i][j] = dot
			for k := 0; k < m; k++ {
				v[k] -= dot * q[k][i]
			}
		}
		var norm float64
		for _, x := range v {
			norm += x * x
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		r[j][j] = norm
		for k := 0; k < m; k++ {
			q[k][j] = v[k] / norm
		}
	}
	qv, err := floatMatrixValue(q)
	if err != nil {
		return nullValue(), err
	}
	rv, err := floatMatrixValue(r)
	if err != nil {
		return nullValue(), err
	}
	return listValue([]Value{qv, rv}), nil
}

func complexQRValue(input Value) (Value, error) {
	shape, a, err := complexMatrixValues(input)
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 {
		return nullValue(), fmt.Errorf("qr expects a matrix")
	}
	rows, cols := shape[0], shape[1]
	q := make([][]complex128, rows)
	for row := range q {
		q[row] = make([]complex128, cols)
	}
	r := make([][]complex128, cols)
	for row := range r {
		r[row] = make([]complex128, cols)
	}
	for column := 0; column < cols; column++ {
		v := make([]complex128, rows)
		for row := range v {
			v[row] = a[row][column]
		}
		for prior := 0; prior < column; prior++ {
			var projection complex128
			for row := 0; row < rows; row++ {
				projection += cmplx.Conj(q[row][prior]) * a[row][column]
			}
			r[prior][column] = projection
			for row := 0; row < rows; row++ {
				v[row] -= projection * q[row][prior]
			}
		}
		normSquared := 0.0
		for _, value := range v {
			normSquared += real(value)*real(value) + imag(value)*imag(value)
		}
		norm := math.Sqrt(normSquared)
		if norm == 0 {
			continue
		}
		r[column][column] = complex(norm, 0)
		for row := 0; row < rows; row++ {
			q[row][column] = v[row] / complex(norm, 0)
		}
	}
	qdata := make([]Value, 0, rows*cols)
	for row := range q {
		for _, value := range q[row] {
			qdata = append(qdata, complexValue(value))
		}
	}
	rdata := make([]Value, 0, cols*cols)
	for row := range r {
		for _, value := range r[row] {
			rdata = append(rdata, complexValue(value))
		}
	}
	qvalue, err := tensorFromShapeAndData([]int{rows, cols}, qdata)
	if err != nil {
		return nullValue(), err
	}
	rvalue, err := tensorFromShapeAndData([]int{cols, cols}, rdata)
	if err != nil {
		return nullValue(), err
	}
	return listValue([]Value{qvalue, rvalue}), nil
}

// choleskyValue handles the cholesky value step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func choleskyValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("cholesky expects 1 argument")
	}
	complexMatrix, err := matrixContainsComplex(args[0])
	if err != nil {
		return nullValue(), err
	}
	if complexMatrix {
		return complexCholeskyValue(args[0])
	}
	shape, a, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("cholesky expects a square matrix")
	}
	n := shape[0]
	l := make([][]float64, n)
	for i := range l {
		l[i] = make([]float64, n)
	}
	for i := 0; i < n; i++ {
		for j := 0; j <= i; j++ {
			sum := a[i][j]
			for k := 0; k < j; k++ {
				sum -= l[i][k] * l[j][k]
			}
			if i == j {
				if sum <= 0 {
					return nullValue(), fmt.Errorf("cholesky expects positive definite matrix")
				}
				l[i][j] = math.Sqrt(sum)
			} else {
				l[i][j] = sum / l[j][j]
			}
		}
	}
	return floatMatrixValue(l)
}

func complexCholeskyValue(input Value) (Value, error) {
	shape, a, err := complexMatrixValues(input)
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("cholesky expects a square matrix")
	}
	n := shape[0]
	for row := 0; row < n; row++ {
		for col := row + 1; col < n; col++ {
			if cmplx.Abs(a[row][col]-cmplx.Conj(a[col][row])) > 1e-12 {
				return nullValue(), fmt.Errorf("cholesky expects a Hermitian matrix")
			}
		}
		if math.Abs(imag(a[row][row])) > 1e-12 {
			return nullValue(), fmt.Errorf("cholesky expects a Hermitian matrix")
		}
	}
	l := make([][]complex128, n)
	for row := range l {
		l[row] = make([]complex128, n)
	}
	for row := 0; row < n; row++ {
		for col := 0; col <= row; col++ {
			sum := a[row][col]
			for prior := 0; prior < col; prior++ {
				sum -= l[row][prior] * cmplx.Conj(l[col][prior])
			}
			if row == col {
				if math.Abs(imag(sum)) > 1e-10 || real(sum) <= 0 {
					return nullValue(), fmt.Errorf("cholesky expects positive definite matrix")
				}
				l[row][col] = complex(math.Sqrt(real(sum)), 0)
			} else {
				l[row][col] = sum / l[col][col]
			}
		}
	}
	data := make([]Value, 0, n*n)
	for row := range l {
		for _, value := range l[row] {
			data = append(data, complexValue(value))
		}
	}
	return tensorFromShapeAndData([]int{n, n}, data)
}

// cloneFloatMatrix handles the clone float matrix step in the surrounding
// package and returns [][]float64.
//
// It returns [][]float64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func cloneFloatMatrix(a [][]float64) [][]float64 {
	out := make([][]float64, len(a))
	for i := range a {
		out[i] = append([]float64(nil), a[i]...)
	}
	return out
}

// identityFloatMatrix handles the identity float matrix step in the
// surrounding package and returns [][]float64.
//
// It returns [][]float64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func identityFloatMatrix(n int) [][]float64 {
	out := make([][]float64, n)
	for i := range out {
		out[i] = make([]float64, n)
		out[i][i] = 1
	}
	return out
}

// transposeFloatMatrix handles the transpose float matrix step in the
// surrounding package and returns [][]float64.
//
// It returns [][]float64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func transposeFloatMatrix(a [][]float64) [][]float64 {
	if len(a) == 0 {
		return nil
	}
	rows := len(a)
	cols := len(a[0])
	out := make([][]float64, cols)
	for c := 0; c < cols; c++ {
		out[c] = make([]float64, rows)
		for r := 0; r < rows; r++ {
			out[c][r] = a[r][c]
		}
	}
	return out
}

// multiplyFloatMatrices handles the multiply float matrices step in the
// surrounding package and returns [][]float64, error.
//
// It returns [][]float64 and an error. Callers should only trust the value
// when the error is nil.
func multiplyFloatMatrices(a, b [][]float64) ([][]float64, error) {
	if len(a) == 0 || len(b) == 0 {
		return nil, fmt.Errorf("matrix multiply expects non-empty matrices")
	}
	if len(a[0]) != len(b) {
		return nil, fmt.Errorf("matrix multiply shape mismatch")
	}
	rows := len(a)
	mid := len(a[0])
	cols := len(b[0])
	out := make([][]float64, rows)
	for r := 0; r < rows; r++ {
		out[r] = make([]float64, cols)
		for c := 0; c < cols; c++ {
			var total float64
			for k := 0; k < mid; k++ {
				total += a[r][k] * b[k][c]
			}
			out[r][c] = total
		}
	}
	return out, nil
}

// matrixVectorMultiply handles the matrix vector multiply step in the
// surrounding package and returns []float64, error.
//
// It returns []float64 and an error. Callers should only trust the value when
// the error is nil.
func matrixVectorMultiply(a [][]float64, v []float64) ([]float64, error) {
	if len(a) == 0 || len(a[0]) != len(v) {
		return nil, fmt.Errorf("matrix-vector shape mismatch")
	}
	out := make([]float64, len(a))
	for r := range a {
		var total float64
		for c := range v {
			total += a[r][c] * v[c]
		}
		out[r] = total
	}
	return out, nil
}

// qrDecomposeFloat handles the qr decompose float step in the surrounding
// package and returns [][]float64, [][]float64, error.
//
// It returns [][]float64, [][]float64, error so callers can continue with the
// next stage of the pipeline without re-deriving the same information.
func qrDecomposeFloat(a [][]float64) ([][]float64, [][]float64, error) {
	if len(a) == 0 || len(a[0]) == 0 {
		return nil, nil, fmt.Errorf("qr expects a non-empty matrix")
	}
	m := len(a)
	n := len(a[0])
	q := make([][]float64, m)
	for i := range q {
		q[i] = make([]float64, n)
	}
	r := make([][]float64, n)
	for i := range r {
		r[i] = make([]float64, n)
	}
	cols := make([][]float64, n)
	for j := 0; j < n; j++ {
		col := make([]float64, m)
		for i := 0; i < m; i++ {
			col[i] = a[i][j]
		}
		cols[j] = col
	}
	for j := 0; j < n; j++ {
		v := append([]float64(nil), cols[j]...)
		for i := 0; i < j; i++ {
			var dot float64
			for k := 0; k < m; k++ {
				dot += q[k][i] * cols[j][k]
			}
			r[i][j] = dot
			for k := 0; k < m; k++ {
				v[k] -= dot * q[k][i]
			}
		}
		var norm float64
		for _, x := range v {
			norm += x * x
		}
		norm = math.Sqrt(norm)
		if norm == 0 {
			continue
		}
		r[j][j] = norm
		for k := 0; k < m; k++ {
			q[k][j] = v[k] / norm
		}
	}
	return q, r, nil
}

// maxOffDiagAbs handles the max off diag abs step in the surrounding package
// and returns float64.
//
// It returns float64 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func maxOffDiagAbs(a [][]float64) float64 {
	var max float64
	for i := range a {
		for j := i + 1; j < len(a[i]); j++ {
			if cur := math.Abs(a[i][j]); cur > max {
				max = cur
			}
		}
	}
	return max
}

// qrEigenDecomposition handles the qr eigen decomposition step in the
// surrounding package and returns []float64, [][]float64, error.
//
// It returns []float64, [][]float64, error so callers can continue with the
// next stage of the pipeline without re-deriving the same information.
func qrEigenDecomposition(a [][]float64, iters int) ([]float64, [][]float64, error) {
	if len(a) == 0 || len(a) != len(a[0]) {
		return nil, nil, fmt.Errorf("eig expects a square matrix")
	}
	n := len(a)
	ak := cloneFloatMatrix(a)
	qTotal := identityFloatMatrix(n)
	for i := 0; i < iters; i++ {
		q, r, err := qrDecomposeFloat(ak)
		if err != nil {
			return nil, nil, err
		}
		next, err := multiplyFloatMatrices(r, q)
		if err != nil {
			return nil, nil, err
		}
		ak = next
		qTotal, err = multiplyFloatMatrices(qTotal, q)
		if err != nil {
			return nil, nil, err
		}
		if maxOffDiagAbs(ak) < 1e-10 {
			break
		}
	}
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = ak[i][i]
	}
	return vals, qTotal, nil
}

// jacobiEigenDecomposition handles the jacobi eigen decomposition step in the
// surrounding package and returns []float64, [][]float64, error.
//
// It returns []float64, [][]float64, error so callers can continue with the
// next stage of the pipeline without re-deriving the same information.
func jacobiEigenDecomposition(a [][]float64, iters int) ([]float64, [][]float64, error) {
	if len(a) == 0 || len(a) != len(a[0]) {
		return nil, nil, fmt.Errorf("eigh expects a square matrix")
	}
	n := len(a)
	ak := cloneFloatMatrix(a)
	v := identityFloatMatrix(n)
	for iter := 0; iter < iters; iter++ {
		p, q := 0, 1
		maxVal := 0.0
		for i := 0; i < n; i++ {
			for j := i + 1; j < n; j++ {
				if cur := math.Abs(ak[i][j]); cur > maxVal {
					maxVal = cur
					p, q = i, j
				}
			}
		}
		if maxVal < 1e-12 {
			break
		}
		app := ak[p][p]
		aqq := ak[q][q]
		apq := ak[p][q]
		if apq == 0 {
			continue
		}
		tau := (aqq - app) / (2 * apq)
		t := math.Copysign(1/(math.Abs(tau)+math.Sqrt(1+tau*tau)), tau)
		c := 1 / math.Sqrt(1+t*t)
		s := t * c
		for k := 0; k < n; k++ {
			if k != p && k != q {
				akpk := ak[k][p]
				akqk := ak[k][q]
				ak[k][p] = c*akpk - s*akqk
				ak[p][k] = ak[k][p]
				ak[k][q] = c*akqk + s*akpk
				ak[q][k] = ak[k][q]
			}
		}
		akpp := c*c*app - 2*s*c*apq + s*s*aqq
		akqq := s*s*app + 2*s*c*apq + c*c*aqq
		ak[p][p] = akpp
		ak[q][q] = akqq
		ak[p][q] = 0
		ak[q][p] = 0
		for k := 0; k < n; k++ {
			vkp := v[k][p]
			vkq := v[k][q]
			v[k][p] = c*vkp - s*vkq
			v[k][q] = s*vkp + c*vkq
		}
	}
	vals := make([]float64, n)
	for i := 0; i < n; i++ {
		vals[i] = ak[i][i]
	}
	return vals, v, nil
}

// eigValue handles the eig value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func eigValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("eig expects 1 argument")
	}
	shape, mat, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("eig expects a square matrix")
	}
	vals, vecs, err := qrEigenDecomposition(mat, 64)
	if err != nil {
		return nullValue(), err
	}
	valVals := make([]Value, len(vals))
	for i := range vals {
		valVals[i] = complexValue(complex(vals[i], 0))
	}
	valTensor, err := tensorFromShapeAndData([]int{len(vals)}, valVals)
	if err != nil {
		return nullValue(), err
	}
	vecVals := make([]Value, 0, len(vecs)*len(vecs))
	for _, row := range vecs {
		for _, value := range row {
			vecVals = append(vecVals, complexValue(complex(value, 0)))
		}
	}
	vv, err := tensorFromShapeAndData([]int{len(vecs), len(vecs)}, vecVals)
	if err != nil {
		return nullValue(), err
	}
	return listValue([]Value{valTensor, vv}), nil
}

// eighValue handles the eigh value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func eighValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("eigh expects 1 argument")
	}
	shape, mat, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 || shape[0] != shape[1] {
		return nullValue(), fmt.Errorf("eigh expects a square matrix")
	}
	for i := range mat {
		for j := i + 1; j < len(mat); j++ {
			if math.Abs(mat[i][j]-mat[j][i]) > 1e-9 {
				return nullValue(), fmt.Errorf("eigh expects a symmetric matrix")
			}
		}
	}
	vals, vecs, err := jacobiEigenDecomposition(mat, 128)
	if err != nil {
		return nullValue(), err
	}
	order := make([]int, len(vals))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return vals[order[i]] < vals[order[j]] })
	sortedVals := make([]Value, len(vals))
	sortedVecs := make([][]float64, len(vecs))
	for i := range sortedVecs {
		sortedVecs[i] = make([]float64, len(vecs[i]))
	}
	for col, idx := range order {
		sortedVals[col] = numberValue(vals[idx])
		for row := range vecs {
			sortedVecs[row][col] = vecs[row][idx]
		}
	}
	valTensor, err := tensorFromShapeAndData([]int{len(sortedVals)}, sortedVals)
	if err != nil {
		return nullValue(), err
	}
	vv, err := floatMatrixValue(sortedVecs)
	if err != nil {
		return nullValue(), err
	}
	return listValue([]Value{valTensor, vv}), nil
}

// svdValue handles the svd value step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func svdValue(args []Value) (Value, error) {
	if len(args) != 1 {
		return nullValue(), fmt.Errorf("svd expects 1 argument")
	}
	shape, a, err := matrixValues(args[0])
	if err != nil {
		return nullValue(), err
	}
	if len(shape) != 2 {
		return nullValue(), fmt.Errorf("svd expects a matrix")
	}
	at := transposeFloatMatrix(a)
	ata, err := multiplyFloatMatrices(at, a)
	if err != nil {
		return nullValue(), err
	}
	vals, vecs, err := jacobiEigenDecomposition(ata, 128)
	if err != nil {
		return nullValue(), err
	}
	order := make([]int, len(vals))
	for i := range order {
		order[i] = i
	}
	sort.Slice(order, func(i, j int) bool { return vals[order[i]] > vals[order[j]] })
	svals := make([]float64, len(vals))
	sortedVecs := make([][]float64, len(vecs))
	for i := range sortedVecs {
		sortedVecs[i] = make([]float64, len(vecs[i]))
	}
	for col, idx := range order {
		v := vals[idx]
		if v < 0 {
			v = 0
		}
		svals[col] = math.Sqrt(v)
		for row := range vecs {
			sortedVecs[row][col] = vecs[row][idx]
		}
	}
	u := make([][]float64, len(a))
	for i := range u {
		u[i] = make([]float64, len(svals))
	}
	for j, sigma := range svals {
		if sigma == 0 {
			continue
		}
		col := make([]float64, len(vecs))
		for i := range vecs {
			col[i] = sortedVecs[i][j]
		}
		prod, err := matrixVectorMultiply(a, col)
		if err != nil {
			return nullValue(), err
		}
		for i := range prod {
			u[i][j] = prod[i] / sigma
		}
	}
	uv, err := floatMatrixValue(u)
	if err != nil {
		return nullValue(), err
	}
	sv := make([]Value, len(svals))
	for i := range svals {
		sv[i] = numberValue(svals[i])
	}
	svTensor, err := tensorFromShapeAndData([]int{len(sv)}, sv)
	if err != nil {
		return nullValue(), err
	}
	vt, err := floatMatrixValue(transposeFloatMatrix(sortedVecs))
	if err != nil {
		return nullValue(), err
	}
	return listValue([]Value{uv, svTensor, vt}), nil
}
