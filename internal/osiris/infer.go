package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type InferProgram struct {
	Program   *Program
	Bindings  []InferBinding
	Functions []InferFunction
}

type InferBinding struct {
	Name    string
	Scope   string
	Kind    string
	Shape   string
	Units   string
	Escapes bool
}

type InferFunction struct {
	Name          string
	SourceName    string
	Params        []InferBinding
	Result        string
	Captures      []string
	Escapes       []string
	CaptureKinds  []string
	CaptureShapes []string
}

func (p *InferProgram) String() string {
	return FormatInferProgram(p)
}

func (rt *Runtime) InferProgram(ctx context.Context, prog *Program) (*InferProgram, error) {
	_ = ctx
	if prog == nil {
		return nil, fmt.Errorf("cannot infer types for nil program")
	}
	if err := validateTensorShapes(prog); err != nil {
		return nil, err
	}
	return inferProgram(prog), nil
}

func FormatInferProgram(prog *InferProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("infer ")
	if prog.Program != nil && prog.Program.SourceName != "" {
		b.WriteString(prog.Program.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.Program != nil {
		if p, ok := firstPackage(prog.Program); ok {
			b.WriteString("package ")
			b.WriteString(p.Name)
			if p.Version != "" {
				b.WriteByte(' ')
				b.WriteString(p.Version)
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Bindings) > 0 {
		b.WriteByte('\n')
		b.WriteString("bindings\n")
		for _, bind := range prog.Bindings {
			b.WriteString("  ")
			if bind.Scope != "" {
				b.WriteString(bind.Scope)
				b.WriteByte('.')
			}
			b.WriteString(bind.Name)
			b.WriteString(": ")
			b.WriteString(bind.Kind)
			if bind.Escapes {
				b.WriteString(" escapes")
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
		b.WriteString("functions\n")
		for _, fn := range prog.Functions {
			b.WriteString("  fn ")
			b.WriteString(fn.Name)
			if fn.SourceName != "" && fn.SourceName != fn.Name {
				b.WriteString(" (")
				b.WriteString(fn.SourceName)
				b.WriteByte(')')
			}
			if fn.Result != "" {
				b.WriteString(": result=")
				b.WriteString(fn.Result)
			}
			if len(fn.Params) > 0 {
				b.WriteString(" params [")
				for i, p := range fn.Params {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(p.Name)
					b.WriteString(":")
					b.WriteString(p.Kind)
				}
				b.WriteByte(']')
			}
			if len(fn.Captures) > 0 {
				b.WriteString(" captures [")
				for i, name := range fn.Captures {
					if i > 0 {
						b.WriteString(", ")
					}
					b.WriteString(name)
					if i < len(fn.CaptureKinds) {
						b.WriteString(":")
						b.WriteString(fn.CaptureKinds[i])
					}
				}
				b.WriteByte(']')
			}
			if len(fn.Escapes) > 0 {
				b.WriteString(" escapes [")
				b.WriteString(strings.Join(fn.Escapes, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type inferBindingState struct {
	binding *InferBinding
	info    inferTypeInfo
	owner   *inferFunctionState
}

type inferScope struct {
	parent *inferScope
	items  map[string]*inferBindingState
	fn     *inferFunctionState
	name   string
}

type inferFunctionState struct {
	report       *InferFunction
	scope        *inferScope
	capturesSeen map[string]struct{}
}

type inferTypeInfo struct {
	known    bool
	kind     string
	shape    []int
	units    map[string]int
	bits     uint16
	frac     uint16
	function *InferFunction
}

func inferProgram(prog *Program) *InferProgram {
	out := &InferProgram{Program: prog}
	root := &inferScope{items: make(map[string]*inferBindingState), name: "main"}
	inferSeq(prog.Statements, root, out)
	sort.Slice(out.Bindings, func(i, j int) bool {
		if out.Bindings[i].Scope == out.Bindings[j].Scope {
			return out.Bindings[i].Name < out.Bindings[j].Name
		}
		return out.Bindings[i].Scope < out.Bindings[j].Scope
	})
	sort.Slice(out.Functions, func(i, j int) bool {
		return out.Functions[i].Name < out.Functions[j].Name
	})
	return out
}

func inferSeq(stmts []Stmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	var last inferTypeInfo
	for _, stmt := range stmts {
		last = inferStmt(stmt, scope, out)
	}
	return last
}

func inferStmt(stmt Stmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	switch st := stmt.(type) {
	case letStmt:
		info := inferExpr(st.Expr, scope, out)
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: info.KindString(), Shape: info.ShapeString(), Units: info.UnitsString()}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: info, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return info
	case importStmt:
		bind := &InferBinding{Name: st.Alias, Scope: scope.name, Kind: "module"}
		scope.items[st.Alias] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: "module"}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: "module"}
	case funcStmt:
		return inferFunctionStmt(st, scope, out)
	case emitStmt:
		return inferExpr(st.Expr, scope, out)
	case returnStmt:
		if st.Expr == nil {
			return inferTypeInfo{known: true, kind: "null"}
		}
		return inferExpr(st.Expr, scope, out)
	case whileStmt:
		_ = inferExpr(st.Cond, scope, out)
		return inferExpr(st.Body, scope.childForBlock(), out)
	case forStmt:
		_ = inferExpr(st.Iter, scope, out)
		child := scope.childForBlock()
		child.bindLocal(st.Name, inferTypeInfo{known: false, kind: "unknown"})
		return inferExpr(st.Body, child, out)
	case macroStmt, packageStmt, exportStmt, breakStmt, continueStmt:
		return inferTypeInfo{known: true, kind: "control"}
	default:
		return inferTypeInfo{}
	}
}

func inferFunctionStmt(st funcStmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	fnState := &inferFunctionState{
		report: &InferFunction{
			Name:       st.Name,
			SourceName: st.Name,
			Params:     make([]InferBinding, 0, len(st.Params)),
		},
		capturesSeen: make(map[string]struct{}),
	}
	child := &inferScope{parent: scope, items: make(map[string]*inferBindingState), fn: fnState, name: st.Name}
	fnState.scope = child
	fnInfo := inferTypeInfo{known: true, kind: "function", function: fnState.report}
	scope.bindLocal(st.Name, fnInfo)
	scope.items[st.Name].binding.Kind = "function"
	out.Bindings = append(out.Bindings, InferBinding{Name: st.Name, Scope: scope.name, Kind: "function"})
	child.bindLocal(st.Name, fnInfo)
	for _, param := range st.Params {
		pb := InferBinding{Name: param, Scope: st.Name, Kind: "unknown"}
		fnState.report.Params = append(fnState.report.Params, pb)
		child.bindLocal(param, inferTypeInfo{known: false, kind: "unknown"})
	}
	result := inferExpr(st.Body, child, out)
	fnState.report.Result = result.KindString()
	for _, name := range fnState.report.Captures {
		if bs := child.lookupLocal(name); bs != nil {
			fnState.report.CaptureKinds = append(fnState.report.CaptureKinds, bs.info.KindString())
			fnState.report.CaptureShapes = append(fnState.report.CaptureShapes, bs.info.ShapeString())
			if bs.binding.Escapes {
				fnState.report.Escapes = append(fnState.report.Escapes, name)
			}
		}
	}
	out.Functions = append(out.Functions, *fnState.report)
	return fnInfo
}

func (s *inferScope) childForBlock() *inferScope {
	return &inferScope{parent: s, items: make(map[string]*inferBindingState), fn: s.fn, name: s.name}
}

func (s *inferScope) bindLocal(name string, info inferTypeInfo) *inferBindingState {
	bind := &InferBinding{Name: name, Scope: s.name, Kind: info.KindString(), Shape: info.ShapeString(), Units: info.UnitsString()}
	state := &inferBindingState{binding: bind, info: info, owner: s.fn}
	if s.items == nil {
		s.items = make(map[string]*inferBindingState)
	}
	s.items[name] = state
	return state
}

func (s *inferScope) lookup(name string) (*inferBindingState, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.items == nil {
			continue
		}
		if state, ok := cur.items[name]; ok {
			if s.fn != nil && state.owner != s.fn {
				state.binding.Escapes = true
				if _, seen := s.fn.capturesSeen[name]; !seen {
					s.fn.capturesSeen[name] = struct{}{}
					s.fn.report.Captures = append(s.fn.report.Captures, name)
				}
			}
			return state, true
		}
	}
	return nil, false
}

func (s *inferScope) lookupLocal(name string) *inferBindingState {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.items == nil {
			continue
		}
		if state, ok := cur.items[name]; ok {
			return state
		}
	}
	return nil
}

func inferExpr(expr Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	switch n := expr.(type) {
	case *literalExpr:
		return inferTypeFromValue(n.value)
	case *identExpr:
		if state, ok := scope.lookup(n.name); ok {
			return state.info
		}
		if info, ok := inferBuiltinInfo(n.name); ok {
			return info
		}
		return inferTypeInfo{kind: "unknown"}
	case *unaryExpr:
		inner := inferExpr(n.expr, scope, out)
		switch n.op {
		case tokenBang:
			return inferTypeInfo{known: true, kind: "bool"}
		case tokenMinus, tokenTilde:
			return inner
		default:
			return inferTypeInfo{kind: "unknown"}
		}
	case *binaryExpr:
		left := inferExpr(n.left, scope, out)
		right := inferExpr(n.right, scope, out)
		return inferBinaryType(n.op, left, right)
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "if":
				if len(n.args) != 3 {
					return inferTypeInfo{kind: "unknown"}
				}
				thenInfo := inferExpr(n.args[1], scope, out)
				elseInfo := inferExpr(n.args[2], scope, out)
				return inferJoinType(thenInfo, elseInfo)
			case "and", "or":
				return inferTypeInfo{known: true, kind: "bool"}
			}
		}
		if name, ok := calledShapeName(n.callee); ok {
			if info, ok := inferBuiltinCall(name, n.args, scope, out); ok {
				return info
			}
		}
		for _, arg := range n.args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{kind: "unknown"}
	case *listExpr:
		if len(n.items) == 0 {
			return inferTypeInfo{known: true, kind: "list", shape: []int{0}}
		}
		var sub inferTypeInfo
		for i, item := range n.items {
			info := inferExpr(item, scope, out)
			if i == 0 {
				sub = info
				continue
			}
			sub = inferJoinType(sub, info)
		}
		outKind := "list"
		if sub.kind == "tensor" {
			outKind = "tensor"
		}
		return inferTypeInfo{known: true, kind: outKind, shape: append([]int{len(n.items)}, sub.shape...)}
	case *blockExpr:
		child := scope.childForBlock()
		for _, stmt := range n.stmts {
			inferStmt(stmt, child, out)
		}
		if n.tail == nil {
			return inferTypeInfo{known: true, kind: "null"}
		}
		return inferExpr(n.tail, child, out)
	case *tryExpr:
		body := inferExpr(n.body, scope, out)
		handler := inferExpr(n.handler, scope.childForBlock(), out)
		return inferJoinType(body, handler)
	case *matchExpr:
		_ = inferExpr(n.subject, scope, out)
		var outInfo inferTypeInfo
		have := false
		for _, arm := range n.arms {
			info := inferExpr(arm.body, scope, out)
			if !have {
				outInfo = info
				have = true
				continue
			}
			outInfo = inferJoinType(outInfo, info)
		}
		return outInfo
	default:
		return inferTypeInfo{kind: "unknown"}
	}
}

func inferBuiltinInfo(name string) (inferTypeInfo, bool) {
	switch name {
	case "true", "false":
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "null":
		return inferTypeInfo{known: true, kind: "null"}, true
	case "len", "sum", "product", "abs", "sqrt", "pow", "expt", "exp", "log", "log10", "log2", "hypot", "norm", "noise", "real", "imag", "magnitude":
		return inferTypeInfo{known: true, kind: "number"}, true
	case "f32":
		return inferTypeInfo{known: true, kind: "float", bits: 32}, true
	case "f64":
		return inferTypeInfo{known: true, kind: "float", bits: 64}, true
	case "bf8":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 8}, true
	case "bf16":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 16}, true
	case "bf32":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 32}, true
	case "bf64":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 64}, true
	case "bf128":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 128}, true
	case "bf256":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 256}, true
	case "bf512":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 512}, true
	case "bf1024":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 1024}, true
	case "uint":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "byte":
		return inferTypeInfo{known: true, kind: "uint", bits: 8}, true
	case "uf8":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 8}, true
	case "uf16":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 16}, true
	case "uf32":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 32}, true
	case "ufnum":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 64}, true
	case "uf64":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 64}, true
	case "uf128":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 128}, true
	case "uf256":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 256}, true
	case "uf512":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 512}, true
	case "uf1024":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 1024}, true
	case "u8":
		return inferTypeInfo{known: true, kind: "uint", bits: 8}, true
	case "u16":
		return inferTypeInfo{known: true, kind: "uint", bits: 16}, true
	case "u32":
		return inferTypeInfo{known: true, kind: "uint", bits: 32}, true
	case "u64":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "u128":
		return inferTypeInfo{known: true, kind: "uint", bits: 128}, true
	case "u256":
		return inferTypeInfo{known: true, kind: "uint", bits: 256}, true
	case "u512":
		return inferTypeInfo{known: true, kind: "uint", bits: 512}, true
	case "u1024":
		return inferTypeInfo{known: true, kind: "uint", bits: 1024}, true
	case "i8":
		return inferTypeInfo{known: true, kind: "int", bits: 8}, true
	case "i16":
		return inferTypeInfo{known: true, kind: "int", bits: 16}, true
	case "i32":
		return inferTypeInfo{known: true, kind: "int", bits: 32}, true
	case "i64":
		return inferTypeInfo{known: true, kind: "int", bits: 64}, true
	case "i128":
		return inferTypeInfo{known: true, kind: "int", bits: 128}, true
	case "i256":
		return inferTypeInfo{known: true, kind: "int", bits: 256}, true
	case "i512":
		return inferTypeInfo{known: true, kind: "int", bits: 512}, true
	case "i1024":
		return inferTypeInfo{known: true, kind: "int", bits: 1024}, true
	case "dec", "decimal":
		return inferTypeInfo{known: true, kind: "decimal"}, true
	case "rat", "rational":
		return inferTypeInfo{known: true, kind: "decimal"}, true
	case "interval":
		return inferTypeInfo{known: true, kind: "interval"}, true
	case "modint":
		return inferTypeInfo{known: true, kind: "modint"}, true
	case "complex":
		return inferTypeInfo{known: true, kind: "complex"}, true
	case "bigint":
		return inferTypeInfo{known: true, kind: "bigint"}, true
	case "bigfloat":
		return inferTypeInfo{known: true, kind: "bigfloat"}, true
	case "shape":
		return inferTypeInfo{known: true, kind: "list", shape: []int{1}}, true
	case "vector", "vec", "tensor", "matrix", "reshape", "transpose", "flatten", "view", "clone", "dot", "matmul":
		return inferTypeInfo{known: true, kind: "tensor"}, true
	default:
		if info, ok := namedConstantInfo(name); ok {
			return info, true
		}
		return inferTypeInfo{}, false
	}
}

func inferBuiltinCall(name string, args []Expr, scope *inferScope, out *InferProgram) (inferTypeInfo, bool) {
	switch name {
	case "repeat":
		if len(args) > 0 {
			info := inferExpr(args[0], scope, out)
			switch info.kind {
			case "string":
				return inferTypeInfo{known: true, kind: "string"}, true
			case "tensor", "list":
				return inferPreserveContainerBuiltin(args, scope, out), true
			}
		}
		return inferTypeInfo{known: true, kind: "string"}, true
	case "split":
		return inferTypeInfo{known: true, kind: "list"}, true
	case "squeeze":
		return inferPreserveContainerBuiltin(args, scope, out), true
	case "expand_dims":
		return inferPreserveContainerBuiltin(args, scope, out), true
	case "tile":
		return inferPreserveContainerBuiltin(args, scope, out), true
	case "concat", "stack", "broadcast_to", "clip", "gather", "scatter", "pad", "outer", "inv", "solve", "cholesky":
		info := inferTypeInfo{known: true, kind: "tensor"}
		if len(args) == 0 {
			return info, true
		}
		switch name {
		case "broadcast_to":
			if dims, ok := inferShapeArgs(args[1:], scope, out); ok {
				info.shape = append([]int(nil), dims...)
			}
		case "concat":
			if len(args) >= 2 {
				first := inferExpr(args[0], scope, out)
				if len(first.shape) > 0 {
					tail := append([]int(nil), first.shape[1:]...)
					total := first.shape[0]
					ok := true
					for _, arg := range args[1:] {
						cur := inferExpr(arg, scope, out)
						if len(cur.shape) == 0 || !equalIntSlices(cur.shape[1:], tail) {
							ok = false
							break
						}
						total += cur.shape[0]
					}
					if ok {
						info.shape = append([]int{total}, tail...)
					}
				}
			}
		case "stack":
			first := inferExpr(args[0], scope, out)
			ok := true
			for _, arg := range args[1:] {
				cur := inferExpr(arg, scope, out)
				if !equalIntSlices(cur.shape, first.shape) {
					ok = false
					break
				}
			}
			if ok {
				info.shape = append([]int{len(args)}, first.shape...)
			}
		case "clip", "scatter", "pad", "tile", "cholesky":
			if len(args) > 0 {
				info.shape = append([]int(nil), inferExpr(args[0], scope, out).shape...)
			}
		case "gather":
			if len(args) > 1 {
				info.shape = append([]int(nil), inferExpr(args[1], scope, out).shape...)
			}
		case "solve":
			if len(args) > 1 {
				info.shape = append([]int(nil), inferExpr(args[1], scope, out).shape...)
			}
		case "outer":
			if len(args) == 2 {
				left := inferExpr(args[0], scope, out)
				right := inferExpr(args[1], scope, out)
				info.shape = []int{tensorShapeProduct(left.shape), tensorShapeProduct(right.shape)}
			}
		case "inv":
			if len(args) == 1 {
				info.shape = append([]int(nil), inferExpr(args[0], scope, out).shape...)
			}
		}
		return info, true
	case "where":
		if len(args) != 3 {
			return inferTypeInfo{kind: "unknown"}, true
		}
		c := inferExpr(args[0], scope, out)
		t := inferExpr(args[1], scope, out)
		f := inferExpr(args[2], scope, out)
		info := inferTypeInfo{known: true, kind: "tensor"}
		shape, err := tensorBroadcastShape(c.shape, t.shape)
		if err == nil {
			if outShape, err := tensorBroadcastShape(shape, f.shape); err == nil {
				info.shape = outShape
			}
		}
		return info, true
	case "zeros", "ones", "eye", "identity":
		info := inferTypeInfo{known: true, kind: "tensor"}
		if dims, ok := inferShapeArgs(args, scope, out); ok {
			if name == "eye" || name == "identity" {
				if len(dims) == 1 {
					info.shape = []int{dims[0], dims[0]}
				} else if len(dims) >= 2 {
					info.shape = []int{dims[0], dims[1]}
				}
			} else {
				info.shape = append([]int(nil), dims...)
			}
		}
		return info, true
	case "full":
		info := inferTypeInfo{known: true, kind: "tensor"}
		if len(args) >= 2 {
			if dims, ok := inferShapeArgs(args[:len(args)-1], scope, out); ok {
				info.shape = append([]int(nil), dims...)
			}
		}
		return info, true
	case "arange":
		info := inferTypeInfo{known: true, kind: "tensor"}
		if len(args) >= 1 {
			if n, ok := inferIntLiteral(args[len(args)-1], scope, out); ok && n >= 0 && len(args) == 1 {
				info.shape = []int{n}
			}
		}
		return info, true
	case "linspace", "logspace":
		info := inferTypeInfo{known: true, kind: "tensor"}
		if len(args) >= 3 {
			if n, ok := inferIntLiteral(args[2], scope, out); ok && n >= 0 {
				info.shape = []int{n}
			}
		}
		return info, true
	case "tensor":
		info := inferTensorCall(args, scope, out)
		info.kind = "tensor"
		return info, true
	case "matrix":
		info := inferTensorCall(args, scope, out)
		info.kind = "tensor"
		return info, true
	case "vector", "vec":
		info := inferTensorCall(args, scope, out)
		if !info.known {
			return inferTypeInfo{known: true, kind: "tensor"}, true
		}
		info.kind = "tensor"
		return info, true
	case "reshape":
		if len(args) != 2 {
			return inferTypeInfo{kind: "unknown"}, true
		}
		shape, ok := inferShapeLiteral(args[1], scope, out)
		if ok {
			return inferTypeInfo{known: true, kind: "tensor", shape: shape}, true
		}
		return inferTypeInfo{known: true, kind: "tensor"}, true
	case "transpose":
		if len(args) == 1 {
			info := inferExpr(args[0], scope, out)
			if len(info.shape) == 2 {
				return inferTypeInfo{known: true, kind: "tensor", shape: []int{info.shape[1], info.shape[0]}}, true
			}
		}
		return inferTypeInfo{known: true, kind: "tensor"}, true
	case "flatten":
		if len(args) == 1 {
			info := inferExpr(args[0], scope, out)
			if info.known {
				return inferTypeInfo{known: true, kind: "list", shape: []int{tensorShapeProduct(info.shape)}}, true
			}
		}
		return inferTypeInfo{known: true, kind: "list"}, true
	case "shape":
		if len(args) == 1 {
			info := inferExpr(args[0], scope, out)
			if info.known {
				return inferTypeInfo{known: true, kind: "list", shape: []int{len(info.shape)}}, true
			}
		}
		return inferTypeInfo{known: true, kind: "list"}, true
	case "trace", "det":
		return inferTypeInfo{known: true, kind: "number"}, true
	case "qr":
		return inferTypeInfo{known: true, kind: "list"}, true
	case "eig", "eigh", "svd":
		return inferTypeInfo{known: true, kind: "list"}, true
	case "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "sign", "step", "heaviside", "relu", "sigmoid", "softplus":
		return inferUnaryLikeBuiltin(args, scope, out, "number"), true
	case "mean", "var", "std", "median", "quantile", "percentile", "dot", "norm", "sum", "product", "len", "hypot", "abs", "sqrt", "pow", "expt", "exp", "log", "log10", "log2", "noise", "real", "imag", "magnitude":
		return inferTypeInfo{known: true, kind: "number"}, true
	case "argmin", "argmax":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "any", "all":
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "cumsum", "cumprod":
		return inferPreserveContainerBuiltin(args, scope, out), true
	case "interval":
		return inferTypeInfo{known: true, kind: "interval"}, true
	case "modint":
		return inferTypeInfo{known: true, kind: "modint"}, true
	case "fixed":
		return inferFixedCall(args, scope, out), true
	case "f32":
		return inferTypeInfo{known: true, kind: "float", bits: 32}, true
	case "f64":
		return inferTypeInfo{known: true, kind: "float", bits: 64}, true
	case "bf8":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 8}, true
	case "bf16":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 16}, true
	case "bf32":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 32}, true
	case "bf64":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 64}, true
	case "bf128":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 128}, true
	case "bf256":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 256}, true
	case "bf512":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 512}, true
	case "bf1024":
		return inferTypeInfo{known: true, kind: "bfloat", bits: 1024}, true
	case "uint":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "byte":
		return inferTypeInfo{known: true, kind: "uint", bits: 8}, true
	case "uf8":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 8}, true
	case "uf16":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 16}, true
	case "uf32":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 32}, true
	case "ufnum":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 64}, true
	case "uf64":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 64}, true
	case "uf128":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 128}, true
	case "uf256":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 256}, true
	case "uf512":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 512}, true
	case "uf1024":
		return inferTypeInfo{known: true, kind: "packeduint", bits: 1024}, true
	case "u8":
		return inferTypeInfo{known: true, kind: "uint", bits: 8}, true
	case "u16":
		return inferTypeInfo{known: true, kind: "uint", bits: 16}, true
	case "u32":
		return inferTypeInfo{known: true, kind: "uint", bits: 32}, true
	case "u64":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "u128":
		return inferTypeInfo{known: true, kind: "uint", bits: 128}, true
	case "u256":
		return inferTypeInfo{known: true, kind: "uint", bits: 256}, true
	case "u512":
		return inferTypeInfo{known: true, kind: "uint", bits: 512}, true
	case "u1024":
		return inferTypeInfo{known: true, kind: "uint", bits: 1024}, true
	case "i8":
		return inferTypeInfo{known: true, kind: "int", bits: 8}, true
	case "i16":
		return inferTypeInfo{known: true, kind: "int", bits: 16}, true
	case "i32":
		return inferTypeInfo{known: true, kind: "int", bits: 32}, true
	case "i64":
		return inferTypeInfo{known: true, kind: "int", bits: 64}, true
	case "i128":
		return inferTypeInfo{known: true, kind: "int", bits: 128}, true
	case "i256":
		return inferTypeInfo{known: true, kind: "int", bits: 256}, true
	case "i512":
		return inferTypeInfo{known: true, kind: "int", bits: 512}, true
	case "i1024":
		return inferTypeInfo{known: true, kind: "int", bits: 1024}, true
	case "dec", "decimal":
		return inferTypeInfo{known: true, kind: "decimal"}, true
	case "rat", "rational":
		return inferTypeInfo{known: true, kind: "decimal"}, true
	case "complex":
		return inferTypeInfo{known: true, kind: "complex"}, true
	case "bigint":
		return inferTypeInfo{known: true, kind: "bigint"}, true
	case "bigfloat":
		return inferTypeInfo{known: true, kind: "bigfloat"}, true
	default:
		return inferTypeInfo{}, false
	}
}

func inferFixedCall(args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	info := inferTypeInfo{known: true, kind: "fixed"}
	if len(args) >= 2 {
		if width, ok := inferIntLiteral(args[0], scope, out); ok {
			info.bits = uint16(width)
		}
		if frac, ok := inferIntLiteral(args[1], scope, out); ok {
			info.frac = uint16(frac)
		}
	}
	if info.bits == 0 {
		info.bits = 64
	}
	return info
}

func inferTensorCall(args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	info := inferTypeInfo{known: true, kind: "tensor"}
	if len(args) == 0 {
		info.shape = []int{0}
		return info
	}
	if len(args) == 1 {
		shape := inferShapeInfo(args[0], scope, out)
		if shape.known {
			info.shape = append([]int(nil), shape.shape...)
		}
		return info
	}
	if len(args) == 2 {
		if dims, ok := inferShapeLiteral(args[0], scope, out); ok {
			info.shape = append([]int(nil), dims...)
		}
		return info
	}
	if len(args) == 3 {
		info.shape = []int{}
		if rows, ok := inferIntLiteral(args[0], scope, out); ok {
			if cols, ok := inferIntLiteral(args[1], scope, out); ok {
				info.shape = []int{rows, cols}
			}
		}
		return info
	}
	return inferTypeInfo{kind: "tensor"}
}

func inferShapeInfo(expr Expr, scope *inferScope, out *InferProgram) shapeInfo {
	info, err := shapeOfExpr(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil {
		return unknownShape()
	}
	return info
}

func inferShapeLiteral(expr Expr, scope *inferScope, out *InferProgram) ([]int, bool) {
	dims, ok, err := intListLiteral(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil || !ok {
		return nil, false
	}
	return dims, true
}

func inferIntLiteral(expr Expr, scope *inferScope, out *InferProgram) (int, bool) {
	n, ok, err := intLiteralExpr(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil || !ok {
		return 0, false
	}
	return n, true
}

func inferShapeArgs(args []Expr, scope *inferScope, out *InferProgram) ([]int, bool) {
	if len(args) == 0 {
		return nil, false
	}
	if len(args) == 1 {
		if dims, ok := inferShapeLiteral(args[0], scope, out); ok {
			return dims, true
		}
		if n, ok := inferIntLiteral(args[0], scope, out); ok {
			if n < 0 {
				return nil, false
			}
			return []int{n}, true
		}
		return nil, false
	}
	dims := make([]int, len(args))
	for i, arg := range args {
		n, ok := inferIntLiteral(arg, scope, out)
		if !ok || n < 0 {
			return nil, false
		}
		dims[i] = n
	}
	return dims, true
}

func inferUnaryLikeBuiltin(args []Expr, scope *inferScope, out *InferProgram, scalarKind string) inferTypeInfo {
	if len(args) != 1 {
		return inferTypeInfo{kind: "unknown"}
	}
	info := inferExpr(args[0], scope, out)
	switch info.kind {
	case "tensor":
		return inferTypeInfo{known: true, kind: "tensor", shape: append([]int(nil), info.shape...)}
	case "list":
		return inferTypeInfo{known: true, kind: "list", shape: append([]int(nil), info.shape...)}
	default:
		return inferTypeInfo{known: true, kind: scalarKind}
	}
}

func inferPreserveContainerBuiltin(args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	if len(args) != 1 {
		return inferTypeInfo{kind: "unknown"}
	}
	info := inferExpr(args[0], scope, out)
	switch info.kind {
	case "tensor":
		return inferTypeInfo{known: true, kind: "tensor", shape: append([]int(nil), info.shape...)}
	case "list":
		return inferTypeInfo{known: true, kind: "list", shape: append([]int(nil), info.shape...)}
	default:
		return inferTypeInfo{known: true, kind: "list"}
	}
}

func inferBinaryType(op tokenKind, left, right inferTypeInfo) inferTypeInfo {
	switch op {
	case tokenEqualEqual, tokenBangEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual, tokenAndAnd, tokenOrOr:
		return inferTypeInfo{known: true, kind: "bool"}
	case tokenPlus, tokenMinus, tokenStar, tokenSlash, tokenPercent:
		if left.kind == "tensor" || right.kind == "tensor" {
			out := inferTypeInfo{known: true, kind: "tensor"}
			if len(left.shape) > 0 {
				out.shape = append([]int(nil), left.shape...)
			}
			if len(right.shape) > 0 {
				if len(out.shape) == 0 {
					out.shape = append([]int(nil), right.shape...)
				}
			}
			return out
		}
		if left.kind == "quantity" || right.kind == "quantity" {
			out := inferTypeInfo{known: true, kind: "quantity"}
			switch op {
			case tokenPlus, tokenMinus:
				if formatDims(left.units) == formatDims(right.units) {
					out.units = cloneDims(left.units)
				}
			case tokenStar:
				out.units = combineDims(left.units, right.units, 1)
			case tokenSlash:
				out.units = combineDims(left.units, right.units, -1)
			}
			return out
		}
		if left.kind == "interval" || right.kind == "interval" {
			if op == tokenPercent {
				return inferTypeInfo{known: true, kind: "unknown"}
			}
			return inferTypeInfo{known: true, kind: "interval"}
		}
		if left.kind == "modint" || right.kind == "modint" {
			return inferTypeInfo{known: true, kind: "modint"}
		}
		if left.kind == "fixed" || right.kind == "fixed" {
			bits := maxBits(left.bits, right.bits)
			frac := maxBits(left.frac, right.frac)
			if bits == 0 {
				bits = 64
			}
			return inferTypeInfo{known: true, kind: "fixed", bits: bits, frac: frac}
		}
		return promoteNumeric(left, right)
	case tokenPipe, tokenAmp, tokenCaret, tokenShiftLeft, tokenShiftRight:
		if (left.kind == "uint" || left.kind == "packeduint") && (right.kind == "uint" || right.kind == "packeduint") {
			if left.kind == "packeduint" || right.kind == "packeduint" {
				return inferTypeInfo{known: true, kind: "packeduint", bits: maxBits(left.bits, right.bits)}
			}
			return inferTypeInfo{known: true, kind: "uint", bits: maxBits(left.bits, right.bits)}
		}
		return inferTypeInfo{known: true, kind: "number"}
	default:
		return inferTypeInfo{kind: "unknown"}
	}
}

func promoteNumeric(left, right inferTypeInfo) inferTypeInfo {
	switch {
	case left.kind == "complex" || right.kind == "complex":
		return inferTypeInfo{known: true, kind: "complex"}
	case left.kind == "bigfloat" || right.kind == "bigfloat":
		return inferTypeInfo{known: true, kind: "bigfloat"}
	case left.kind == "bigint" || right.kind == "bigint":
		return inferTypeInfo{known: true, kind: "bigint"}
	case left.kind == "decimal" || right.kind == "decimal":
		return inferTypeInfo{known: true, kind: "decimal"}
	case left.kind == "interval" || right.kind == "interval":
		return inferTypeInfo{known: true, kind: "interval"}
	case left.kind == "modint" || right.kind == "modint":
		return inferTypeInfo{known: true, kind: "modint"}
	case left.kind == "fixed" || right.kind == "fixed":
		bits := maxBits(left.bits, right.bits)
		frac := maxBits(left.frac, right.frac)
		if bits == 0 {
			bits = 64
		}
		return inferTypeInfo{known: true, kind: "fixed", bits: bits, frac: frac}
	case left.kind == "float" || right.kind == "float":
		bits := maxBits(left.bits, right.bits)
		if left.kind == "number" || right.kind == "number" {
			if bits < 64 {
				bits = 64
			}
		}
		if bits == 0 {
			bits = 64
		}
		return inferTypeInfo{known: true, kind: "float", bits: bits}
	case left.kind == "bfloat" || right.kind == "bfloat":
		bits := maxBits(left.bits, right.bits)
		if left.kind == "number" || right.kind == "number" {
			if bits < 64 {
				bits = 64
			}
		}
		if bits == 0 {
			bits = 64
		}
		return inferTypeInfo{known: true, kind: "bfloat", bits: bits}
	case left.kind == "number" || right.kind == "number":
		return inferTypeInfo{known: true, kind: "number"}
	case left.kind == "uint" && right.kind == "uint":
		return inferTypeInfo{known: true, kind: "uint", bits: maxBits(left.bits, right.bits)}
	case left.kind == "packeduint" && right.kind == "packeduint":
		return inferTypeInfo{known: true, kind: "packeduint", bits: maxBits(left.bits, right.bits)}
	case left.kind == "packeduint" || right.kind == "packeduint":
		if left.kind == "uint" || right.kind == "uint" {
			return inferTypeInfo{known: true, kind: "packeduint", bits: maxBits(left.bits, right.bits)}
		}
		return inferTypeInfo{known: true, kind: "number"}
	case left.kind == "bool" && right.kind == "bool":
		return inferTypeInfo{known: true, kind: "bool"}
	default:
		return inferTypeInfo{known: true, kind: "number"}
	}
}

func inferJoinType(a, b inferTypeInfo) inferTypeInfo {
	if a.kind == b.kind && a.ShapeString() == b.ShapeString() && formatDims(a.units) == formatDims(b.units) && a.bits == b.bits && a.frac == b.frac {
		return a
	}
	if a.kind == "unknown" {
		return b
	}
	if b.kind == "unknown" {
		return a
	}
	return inferTypeInfo{kind: "unknown"}
}

func inferTypeFromValue(v Value) inferTypeInfo {
	info := inferTypeInfo{known: true}
	switch v.kind {
	case kindNull:
		info.kind = "null"
	case kindBool:
		info.kind = "bool"
	case kindNumber:
		if v.bfloat {
			info.kind = "bfloat"
			info.bits = v.fbits
		} else if v.fbits != 0 {
			info.kind = "float"
			info.bits = v.fbits
		} else {
			info.kind = "number"
		}
	case kindDecimal:
		info.kind = "decimal"
	case kindFixed:
		info.kind = "fixed"
		if v.fx != nil {
			info.bits = v.fx.width
			info.frac = v.fx.frac
		}
	case kindComplex:
		info.kind = "complex"
	case kindError:
		info.kind = "error"
	case kindBigInt:
		info.kind = "bigint"
	case kindBigFloat:
		info.kind = "bigfloat"
	case kindInt:
		info.kind = "int"
		info.bits = v.ibits
	case kindQuantity:
		info.kind = "quantity"
		if v.qty != nil {
			info.units = cloneDims(v.qty.dims)
		}
	case kindUint:
		if v.packed {
			info.kind = "packeduint"
		} else {
			info.kind = "uint"
		}
		if v.ub != nil {
			info.bits = v.ubitsToUint16()
		} else {
			info.bits = uint16(v.bits)
		}
	case kindString:
		info.kind = "string"
	case kindList:
		info.kind = "list"
		info.shape = []int{len(v.list)}
	case kindTensor:
		info.kind = "tensor"
		info.shape = append([]int(nil), v.shape...)
	case kindFunction:
		info.kind = "function"
	case kindModule:
		info.kind = "module"
	default:
		info.kind = "unknown"
	}
	return info
}

func (i inferTypeInfo) KindString() string {
	switch i.kind {
	case "int":
		if i.bits == 0 {
			return "int64"
		}
		return fmt.Sprintf("int%d", i.bits)
	case "uint":
		if i.bits == 0 {
			return "uint64"
		}
		return fmt.Sprintf("uint%d", i.bits)
	case "packeduint":
		if i.bits == 0 {
			return "ufnum"
		}
		switch i.bits {
		case 64:
			return "ufnum"
		default:
			return fmt.Sprintf("uf%d", i.bits)
		}
	case "float":
		if i.bits == 0 {
			return "number"
		}
		return fmt.Sprintf("f%d", i.bits)
	case "bfloat":
		if i.bits == 0 {
			return "bf64"
		}
		return fmt.Sprintf("bf%d", i.bits)
	case "fixed":
		if i.bits == 0 {
			i.bits = 64
		}
		return fmt.Sprintf("fixed[%d,%d]", i.bits, i.frac)
	case "tensor":
		if len(i.shape) == 0 {
			return "tensor"
		}
		return "tensor" + tensorShapeString(i.shape)
	case "list":
		if len(i.shape) == 0 {
			return "list"
		}
		return "list" + tensorShapeString(i.shape)
	case "quantity":
		if len(i.units) == 0 {
			return "quantity"
		}
		return "quantity[" + formatDims(i.units) + "]"
	default:
		return i.kind
	}
}

func (i inferTypeInfo) ShapeString() string {
	if len(i.shape) == 0 {
		return ""
	}
	return tensorShapeString(i.shape)
}

func (i inferTypeInfo) UnitsString() string {
	return formatDims(i.units)
}

func maxBits(a, b uint16) uint16 {
	if a > b {
		return a
	}
	return b
}
