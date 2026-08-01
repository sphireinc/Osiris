package osiris

import "fmt"

type shapeInfo struct {
	known     bool
	shape     []int
	tensorish bool
}

// knownScalarShape handles the known scalar shape step in the surrounding
// package and returns shapeInfo.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func knownScalarShape() shapeInfo {
	return shapeInfo{known: true, shape: nil}
}

// knownListShape handles the known list shape step in the surrounding package
// and returns shapeInfo.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func knownListShape(shape []int) shapeInfo {
	return shapeInfo{known: true, shape: append([]int(nil), shape...)}
}

// knownTensorShape handles the known tensor shape step in the surrounding
// package and returns shapeInfo.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func knownTensorShape(shape []int) shapeInfo {
	return shapeInfo{known: true, shape: append([]int(nil), shape...), tensorish: true}
}

// unknownShape handles the unknown shape step in the surrounding package and
// returns shapeInfo.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func unknownShape() shapeInfo {
	return shapeInfo{}
}

// copy operates on shapeInfo values and keeps the shapeinfo-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s shapeInfo) copy() shapeInfo {
	if !s.known {
		return unknownShape()
	}
	return shapeInfo{known: true, shape: append([]int(nil), s.shape...), tensorish: s.tensorish}
}

type shapeEnv struct {
	parent *shapeEnv
	items  map[string]shapeInfo
}

// child operates on shapeEnv values and keeps the shapeenv-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *shapeEnv so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (e *shapeEnv) child() *shapeEnv {
	return &shapeEnv{parent: e, items: make(map[string]shapeInfo)}
}

// lookup operates on shapeEnv values and keeps the shapeenv-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns shapeInfo, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (e *shapeEnv) lookup(name string) (shapeInfo, bool) {
	for cur := e; cur != nil; cur = cur.parent {
		if cur.items == nil {
			continue
		}
		if v, ok := cur.items[name]; ok {
			return v.copy(), true
		}
	}
	return unknownShape(), false
}

// bind operates on shapeEnv values and keeps the shapeenv-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (e *shapeEnv) bind(name string, info shapeInfo) {
	if e.items == nil {
		e.items = make(map[string]shapeInfo)
	}
	e.items[name] = info.copy()
}

// validateTensorShapes handles the validate tensor shapes step in the
// surrounding package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func validateTensorShapes(prog *Program) error {
	if prog == nil {
		return nil
	}
	return validateStatements(prog.Statements, &shapeEnv{items: make(map[string]shapeInfo)})
}

// validateStatements handles the validate statements step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func validateStatements(stmts []Stmt, env *shapeEnv) error {
	for _, stmt := range stmts {
		if err := validateStmtShapes(stmt, env); err != nil {
			return err
		}
	}
	return nil
}

// validateStmtShapes handles the validate stmt shapes step in the surrounding
// package and returns error.
//
// It returns an error when the operation cannot complete cleanly.
func validateStmtShapes(stmt Stmt, env *shapeEnv) error {
	switch st := stmt.(type) {
	case letStmt:
		info, err := shapeOfExpr(st.Expr, env)
		if err != nil {
			return err
		}
		if info.known {
			env.bind(st.Name, info)
		}
	case assignStmt:
		_, err := shapeOfExpr(st.Expr, env)
		return err
	case emitStmt:
		_, err := shapeOfExpr(st.Expr, env)
		return err
	case returnStmt:
		_, err := shapeOfExpr(st.Expr, env)
		return err
	case whileStmt:
		if _, err := shapeOfExpr(st.Cond, env); err != nil {
			return err
		}
		if _, err := shapeOfExpr(st.Body, env); err != nil {
			return err
		}
	case forStmt:
		if _, err := shapeOfExpr(st.Iter, env); err != nil {
			return err
		}
		child := env.child()
		child.bind(st.Name, unknownShape())
		if _, err := shapeOfExpr(st.Body, child); err != nil {
			return err
		}
	case funcStmt:
		child := env.child()
		for _, name := range st.Params {
			child.bind(name, unknownShape())
		}
		if _, err := shapeOfExpr(st.Body, child); err != nil {
			return err
		}
	case macroStmt:
		if _, err := shapeOfExpr(st.Body, env); err != nil {
			return err
		}
	case importStmt, packageStmt, exportStmt:
		return nil
	default:
		return nil
	}
	return nil
}

// shapeOfExpr handles the shape of expr step in the surrounding package and
// returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfExpr(expr Expr, env *shapeEnv) (shapeInfo, error) {
	switch n := expr.(type) {
	case *literalExpr:
		switch n.value.kind {
		case kindList:
			shape, _, err := inferTensor(n.value)
			if err != nil {
				return unknownShape(), err
			}
			return knownListShape(shape), nil
		case kindTensor:
			shape, _, err := inferTensor(n.value)
			if err != nil {
				return unknownShape(), err
			}
			return knownTensorShape(shape), nil
		default:
			return knownScalarShape(), nil
		}
	case *identExpr:
		if info, ok := env.lookup(n.name); ok {
			return info.copy(), nil
		}
		return unknownShape(), nil
	case *unaryExpr:
		return shapeOfExpr(n.expr, env)
	case *binaryExpr:
		left, err := shapeOfExpr(n.left, env)
		if err != nil {
			return unknownShape(), err
		}
		right, err := shapeOfExpr(n.right, env)
		if err != nil {
			return unknownShape(), err
		}
		if !left.known || !right.known {
			return unknownShape(), nil
		}
		switch n.op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash, tokenPercent:
			if left.tensorish || right.tensorish {
				out, err := tensorBroadcastShape(left.shape, right.shape)
				if err != nil {
					return unknownShape(), err
				}
				return shapeInfo{known: true, shape: out, tensorish: true}, nil
			}
			if len(left.shape) == 0 && len(right.shape) == 0 {
				return knownScalarShape(), nil
			}
			return unknownShape(), nil
		case tokenEqualEqual, tokenBangEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
			if left.tensorish || right.tensorish {
				if _, err := tensorBroadcastShape(left.shape, right.shape); err != nil {
					return unknownShape(), err
				}
			}
			return knownScalarShape(), nil
		default:
			return unknownShape(), nil
		}
	case *callExpr:
		if name, ok := calledShapeName(n.callee); ok {
			switch name {
			case "if":
				return shapeOfIfCall(n, env)
			case "and", "or":
				return knownScalarShape(), nil
			case "tensor":
				return shapeOfTensorCall(n.args, env, true)
			case "matrix":
				return shapeOfMatrixCall(n.args, env)
			case "reshape":
				return shapeOfReshapeCall(n.args, env)
			case "matmul":
				return shapeOfMatMulCall(n.args, env)
			case "transpose":
				return shapeOfTransposeCall(n.args, env)
			case "flatten":
				return shapeOfFlattenCall(n.args, env)
			case "shape":
				return shapeOfShapeCall(n.args, env)
			case "dot":
				return shapeOfDotCall(n.args, env)
			case "vector", "vec":
				return shapeOfVectorCall(n.args, env)
			case "view", "clone":
				return shapeOfPassthroughTensorCall(name, n.args, env)
			}
		}
		if _, err := shapeOfExpr(n.callee, env); err != nil {
			return unknownShape(), err
		}
		for _, arg := range n.args {
			if _, err := shapeOfExpr(arg, env); err != nil {
				return unknownShape(), err
			}
		}
		return unknownShape(), nil
	case *listExpr:
		if len(n.items) == 0 {
			return knownListShape([]int{0}), nil
		}
		var sub shapeInfo
		for i, item := range n.items {
			info, err := shapeOfExpr(item, env)
			if err != nil {
				return unknownShape(), err
			}
			if !info.known {
				return unknownShape(), nil
			}
			if i == 0 {
				sub = info
				continue
			}
			if !equalIntSlices(sub.shape, info.shape) || sub.tensorish != info.tensorish {
				return unknownShape(), nil
			}
		}
		return shapeInfo{known: true, shape: append([]int{len(n.items)}, sub.shape...), tensorish: false}, nil
	case *blockExpr:
		child := env.child()
		for _, stmt := range n.stmts {
			if err := validateStmtShapes(stmt, child); err != nil {
				return unknownShape(), err
			}
		}
		if n.tail == nil {
			return unknownShape(), nil
		}
		return shapeOfExpr(n.tail, child)
	case *tryExpr:
		body, err := shapeOfExpr(n.body, env)
		if err != nil {
			return unknownShape(), err
		}
		handler, err := shapeOfExpr(n.handler, env)
		if err != nil {
			return unknownShape(), err
		}
		if body.known && handler.known && equalIntSlices(body.shape, handler.shape) && body.tensorish == handler.tensorish {
			return body.copy(), nil
		}
		return unknownShape(), nil
	case *propagateExpr:
		return shapeOfExpr(n.expr, env)
	case *matchExpr:
		if _, err := shapeOfExpr(n.subject, env); err != nil {
			return unknownShape(), err
		}
		var (
			out  shapeInfo
			have bool
		)
		for _, arm := range n.arms {
			if arm.guard != nil {
				if _, err := shapeOfExpr(arm.guard, env); err != nil {
					return unknownShape(), err
				}
			}
			info, err := shapeOfExpr(arm.body, env)
			if err != nil {
				return unknownShape(), err
			}
			if !info.known {
				return unknownShape(), nil
			}
			if !have {
				out = info
				have = true
				continue
			}
			if !equalIntSlices(out.shape, info.shape) || out.tensorish != info.tensorish {
				return unknownShape(), nil
			}
		}
		if have {
			return out.copy(), nil
		}
		return unknownShape(), nil
	default:
		return unknownShape(), nil
	}
}

// calledShapeName handles the called shape name step in the surrounding
// package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func calledShapeName(expr Expr) (string, bool) {
	switch n := expr.(type) {
	case *identExpr:
		return n.name, true
	case *memberExpr:
		return n.name, true
	default:
		return "", false
	}
}

// shapeOfIfCall handles the shape of if call step in the surrounding package
// and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfIfCall(n *callExpr, env *shapeEnv) (shapeInfo, error) {
	if len(n.args) != 3 {
		return unknownShape(), fmt.Errorf("if expects 3 arguments")
	}
	if _, err := shapeOfExpr(n.args[0], env); err != nil {
		return unknownShape(), err
	}
	thenShape, err := shapeOfExpr(n.args[1], env)
	if err != nil {
		return unknownShape(), err
	}
	elseShape, err := shapeOfExpr(n.args[2], env)
	if err != nil {
		return unknownShape(), err
	}
	if thenShape.known && elseShape.known && equalIntSlices(thenShape.shape, elseShape.shape) && thenShape.tensorish == elseShape.tensorish {
		return thenShape.copy(), nil
	}
	return unknownShape(), nil
}

// shapeOfTensorCall handles the shape of tensor call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfTensorCall(args []Expr, env *shapeEnv, tensorish bool) (shapeInfo, error) {
	switch len(args) {
	case 1:
		info, err := shapeOfExpr(args[0], env)
		if err != nil {
			return unknownShape(), err
		}
		if !info.known {
			return unknownShape(), nil
		}
		info.tensorish = tensorish
		return info, nil
	case 2:
		dims, ok, err := intListLiteral(args[0], env)
		if err != nil {
			return unknownShape(), err
		}
		if !ok {
			return unknownShape(), nil
		}
		data, err := shapeOfExpr(args[1], env)
		if err != nil {
			return unknownShape(), err
		}
		if data.known && tensorShapeProduct(dims) != tensorShapeProduct(data.shape) {
			return unknownShape(), fmt.Errorf("tensor shape %v does not match %d elements", dims, tensorShapeProduct(data.shape))
		}
		return shapeInfo{known: true, shape: dims, tensorish: tensorish}, nil
	default:
		return unknownShape(), fmt.Errorf("tensor expects 1 or 2 arguments")
	}
}

// shapeOfMatrixCall handles the shape of matrix call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfMatrixCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	switch len(args) {
	case 1:
		return shapeOfTensorCall(args, env, true)
	case 2:
		return shapeOfTensorCall(args, env, true)
	case 3:
		rows, ok, err := intLiteralExpr(args[0], env)
		if err != nil {
			return unknownShape(), err
		}
		if !ok {
			return unknownShape(), nil
		}
		cols, ok, err := intLiteralExpr(args[1], env)
		if err != nil {
			return unknownShape(), err
		}
		if !ok {
			return unknownShape(), nil
		}
		data, err := shapeOfExpr(args[2], env)
		if err != nil {
			return unknownShape(), err
		}
		shape := []int{rows, cols}
		if data.known && tensorShapeProduct(shape) != tensorShapeProduct(data.shape) {
			return unknownShape(), fmt.Errorf("tensor shape %v does not match %d elements", shape, tensorShapeProduct(data.shape))
		}
		return shapeInfo{known: true, shape: shape, tensorish: true}, nil
	default:
		return unknownShape(), fmt.Errorf("matrix expects 1, 2, or 3 arguments")
	}
}

// shapeOfReshapeCall handles the shape of reshape call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfReshapeCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 2 {
		return unknownShape(), fmt.Errorf("reshape expects 2 arguments")
	}
	dstShape, ok, err := intListLiteral(args[1], env)
	if err != nil {
		return unknownShape(), err
	}
	if !ok {
		return unknownShape(), nil
	}
	srcShape, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	if srcShape.known && tensorShapeProduct(srcShape.shape) != tensorShapeProduct(dstShape) {
		return unknownShape(), fmt.Errorf("reshape shape %v does not match %d elements", dstShape, tensorShapeProduct(srcShape.shape))
	}
	return shapeInfo{known: true, shape: dstShape, tensorish: true}, nil
}

// shapeOfMatMulCall handles the shape of mat mul call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfMatMulCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 2 {
		return unknownShape(), fmt.Errorf("matmul expects 2 arguments")
	}
	left, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	right, err := shapeOfExpr(args[1], env)
	if err != nil {
		return unknownShape(), err
	}
	if !left.known || !right.known {
		return unknownShape(), nil
	}
	if len(left.shape) != 2 || len(right.shape) != 2 {
		return unknownShape(), fmt.Errorf("matmul expects 2D matrices")
	}
	if left.shape[1] != right.shape[0] {
		return unknownShape(), fmt.Errorf("matmul shape mismatch")
	}
	return shapeInfo{known: true, shape: []int{left.shape[0], right.shape[1]}, tensorish: true}, nil
}

// shapeOfTransposeCall handles the shape of transpose call step in the
// surrounding package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfTransposeCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 1 {
		return unknownShape(), fmt.Errorf("transpose expects 1 argument")
	}
	info, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	if !info.known {
		return unknownShape(), nil
	}
	if len(info.shape) != 2 {
		return unknownShape(), fmt.Errorf("transpose expects a 2D matrix")
	}
	return shapeInfo{known: true, shape: []int{info.shape[1], info.shape[0]}, tensorish: true}, nil
}

// shapeOfFlattenCall handles the shape of flatten call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfFlattenCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 1 {
		return unknownShape(), fmt.Errorf("flatten expects 1 argument")
	}
	info, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	if !info.known {
		return unknownShape(), nil
	}
	return shapeInfo{known: true, shape: []int{tensorShapeProduct(info.shape)}, tensorish: false}, nil
}

// shapeOfShapeCall handles the shape of shape call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfShapeCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 1 {
		return unknownShape(), fmt.Errorf("shape expects 1 argument")
	}
	info, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	if !info.known {
		return unknownShape(), nil
	}
	return shapeInfo{known: true, shape: []int{len(info.shape)}, tensorish: false}, nil
}

// shapeOfDotCall handles the shape of dot call step in the surrounding package
// and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfDotCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 2 {
		return unknownShape(), fmt.Errorf("dot expects 2 arguments")
	}
	left, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	right, err := shapeOfExpr(args[1], env)
	if err != nil {
		return unknownShape(), err
	}
	if left.known && right.known && len(left.shape) != len(right.shape) {
		return unknownShape(), fmt.Errorf("dot expects tensors with the same length")
	}
	if left.known && right.known && len(left.shape) == len(right.shape) {
		return knownScalarShape(), nil
	}
	return unknownShape(), nil
}

// shapeOfVectorCall handles the shape of vector call step in the surrounding
// package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfVectorCall(args []Expr, env *shapeEnv) (shapeInfo, error) {
	switch len(args) {
	case 0:
		return shapeInfo{known: true, shape: []int{0}, tensorish: true}, nil
	case 1:
		info, err := shapeOfExpr(args[0], env)
		if err != nil {
			return unknownShape(), err
		}
		if !info.known {
			return unknownShape(), nil
		}
		if info.tensorish || len(info.shape) > 0 {
			info.tensorish = true
			return info, nil
		}
		return shapeInfo{known: true, shape: []int{1}, tensorish: true}, nil
	default:
		return shapeInfo{known: true, shape: []int{len(args)}, tensorish: true}, nil
	}
}

// shapeOfPassthroughTensorCall handles the shape of passthrough tensor call
// step in the surrounding package and returns shapeInfo, error.
//
// It returns shapeInfo and an error. Callers should only trust the value when
// the error is nil.
func shapeOfPassthroughTensorCall(name string, args []Expr, env *shapeEnv) (shapeInfo, error) {
	if len(args) != 1 {
		return unknownShape(), fmt.Errorf("%s expects 1 argument", name)
	}
	info, err := shapeOfExpr(args[0], env)
	if err != nil {
		return unknownShape(), err
	}
	if !info.known {
		return unknownShape(), nil
	}
	info.tensorish = true
	return info, nil
}

// intListLiteral handles the int list literal step in the surrounding package
// and returns []int, bool, error.
//
// It returns []int, bool, error so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func intListLiteral(expr Expr, env *shapeEnv) ([]int, bool, error) {
	switch n := expr.(type) {
	case *listExpr:
		out := make([]int, len(n.items))
		for i, item := range n.items {
			n, ok, err := intLiteralExpr(item, env)
			if err != nil {
				return nil, false, err
			}
			if !ok {
				return nil, false, nil
			}
			out[i] = n
		}
		return out, true, nil
	case *literalExpr:
		items, err := n.value.AsList()
		if err != nil {
			return nil, false, nil
		}
		out := make([]int, len(items))
		for i, item := range items {
			n, err := item.Int()
			if err != nil {
				return nil, false, err
			}
			out[i] = n
		}
		return out, true, nil
	default:
		return nil, false, nil
	}
}

// intLiteralExpr handles the int literal expr step in the surrounding package
// and returns int, bool, error.
//
// It returns int, bool, error so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func intLiteralExpr(expr Expr, env *shapeEnv) (int, bool, error) {
	switch n := expr.(type) {
	case *literalExpr:
		switch n.value.kind {
		case kindUint, kindNumber, kindDecimal, kindBigInt, kindBigFloat, kindBool, kindQuantity, kindComplex:
			v, err := n.value.Int()
			if err != nil {
				return 0, false, err
			}
			return v, true, nil
		default:
			return 0, false, nil
		}
	case *unaryExpr:
		if n.op != tokenMinus {
			return 0, false, nil
		}
		v, ok, err := intLiteralExpr(n.expr, env)
		if err != nil || !ok {
			return 0, ok, err
		}
		return -v, true, nil
	case *identExpr:
		if info, ok := env.lookup(n.name); ok && info.known && len(info.shape) == 0 {
			return 0, false, nil
		}
		return 0, false, nil
	default:
		return 0, false, nil
	}
}
