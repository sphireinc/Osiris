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
	Signature     string
	Params        []InferBinding
	Result        string
	Captures      []string
	Escapes       []string
	CaptureKinds  []string
	CaptureShapes []string
}

// String operates on InferProgram values and keeps the inferprogram-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (p *InferProgram) String() string {
	return FormatInferProgram(p)
}

// InferProgram operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *InferProgram and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) InferProgram(ctx context.Context, prog *Program) (*InferProgram, error) {
	typed, err := rt.frontendProgram(ctx, prog)
	if err != nil {
		return nil, err
	}
	return inferProgram(typed.resolved.expanded.program), nil
}

// FormatInferProgram formats the infer program into a stable textual
// rendering. The implementation is intentionally deterministic so
// round-tripping and diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
			if fn.Signature != "" {
				b.WriteString(" signature=")
				b.WriteString(fn.Signature)
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
	module   string
}

// inferProgram handles the infer program step in the surrounding package and
// returns *InferProgram.
//
// It returns *InferProgram so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// inferSeq handles the infer seq step in the surrounding package and returns
// inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferSeq(stmts []Stmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	var last inferTypeInfo
	for _, stmt := range stmts {
		last = inferStmt(stmt, scope, out)
	}
	return last
}

// inferStmt handles the infer stmt step in the surrounding package and returns
// inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferStmt(stmt Stmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	switch st := stmt.(type) {
	case letStmt:
		info := inferExpr(st.Expr, scope, out)
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: info.KindString(), Shape: info.ShapeString(), Units: info.UnitsString()}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: info, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return info
	case assignStmt:
		info := inferExpr(st.Expr, scope, out)
		if state, ok := scope.lookup(st.Name); ok {
			state.info = info
		}
		return info
	case importStmt:
		bind := &InferBinding{Name: st.Alias, Scope: scope.name, Kind: "module"}
		moduleName, _ := splitModuleRef(st.Path)
		scope.items[st.Alias] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: "module", module: moduleName}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: "module"}
	case enumStmt:
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: "enum"}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: "enum"}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: "enum"}
	case structStmt:
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: "struct"}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: "struct"}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: "struct"}
	case typeDeclStmt:
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: st.Target}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: st.Target}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: st.Target}
	case newtypeStmt:
		bind := &InferBinding{Name: st.Name, Scope: scope.name, Kind: st.Name}
		scope.items[st.Name] = &inferBindingState{binding: bind, info: inferTypeInfo{known: true, kind: st.Name}, owner: scope.fn}
		out.Bindings = append(out.Bindings, *bind)
		return inferTypeInfo{known: true, kind: st.Name}
	case funcStmt:
		return inferFunctionStmt(st, scope, out)
	case traitStmt, implStmt:
		return inferTypeInfo{known: true, kind: "behavior"}
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

// inferFunctionStmt handles the infer function stmt step in the surrounding
// package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferFunctionStmt(st funcStmt, scope *inferScope, out *InferProgram) inferTypeInfo {
	signature, _ := staticFunctionType(st)
	fnState := &inferFunctionState{
		report: &InferFunction{
			Name:       st.Name,
			SourceName: st.Name,
			Signature:  signature.String(),
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
	for i, param := range st.Params {
		paramInfo := inferTypeInfo{known: false, kind: "unknown"}
		if i < len(signature.params) && !signature.params[i].isDynamic() {
			paramInfo = inferInfoFromStaticType(signature.params[i])
		}
		pb := InferBinding{Name: param, Scope: st.Name, Kind: paramInfo.KindString()}
		if i < len(signature.params) && !signature.params[i].isDynamic() {
			pb.Kind = signature.params[i].String()
		}
		fnState.report.Params = append(fnState.report.Params, pb)
		child.bindLocal(param, paramInfo)
	}
	result := inferExpr(st.Body, child, out)
	if signature.result != nil && !signature.result.isDynamic() {
		result = inferInfoFromStaticType(*signature.result)
		fnState.report.Result = signature.result.String()
	} else {
		fnState.report.Result = result.KindString()
	}
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

// childForBlock operates on inferScope values and keeps the
// inferscope-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *inferScope so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *inferScope) childForBlock() *inferScope {
	return &inferScope{parent: s, items: make(map[string]*inferBindingState), fn: s.fn, name: s.name}
}

// bindLocal operates on inferScope values and keeps the inferscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *inferBindingState so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (s *inferScope) bindLocal(name string, info inferTypeInfo) *inferBindingState {
	bind := &InferBinding{Name: name, Scope: s.name, Kind: info.KindString(), Shape: info.ShapeString(), Units: info.UnitsString()}
	state := &inferBindingState{binding: bind, info: info, owner: s.fn}
	if s.items == nil {
		s.items = make(map[string]*inferBindingState)
	}
	s.items[name] = state
	return state
}

// lookup operates on inferScope values and keeps the inferscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *inferBindingState, bool so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
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

// lookupLocal operates on inferScope values and keeps the inferscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *inferBindingState so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
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

// inferExpr handles the infer expr step in the surrounding package and returns
// inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
			case "spawn", "await", "cancel", "read_async", "readbytes_async", "write_async", "lines_async", "csv_async", "jsonparse_async":
				return inferTypeInfo{known: true, kind: "task"}
			case "race":
				var joined inferTypeInfo
				for i, arg := range n.args {
					info := inferExpr(arg, scope, out)
					if i == 0 {
						joined = info
						continue
					}
					joined = inferJoinType(joined, info)
				}
				return joined
			case "select":
				return inferTypeInfo{known: true, kind: "list", shape: []int{2}}
			case "parallel":
				if len(n.args) == 0 {
					return inferTypeInfo{known: true, kind: "list", shape: []int{0}}
				}
				var sub inferTypeInfo
				for i, arg := range n.args {
					info := inferExpr(arg, scope, out)
					if i == 0 {
						sub = info
						continue
					}
					sub = inferJoinType(sub, info)
				}
				_ = sub
				return inferTypeInfo{known: true, kind: "list", shape: []int{len(n.args)}}
			case "timeout":
				if len(n.args) != 2 {
					return inferTypeInfo{kind: "unknown"}
				}
				_ = inferExpr(n.args[0], scope, out)
				return inferExpr(n.args[1], scope, out)
			}
		}
		calleeInfo := inferExpr(n.callee, scope, out)
		if calleeInfo.function != nil && calleeInfo.function.Result != "" && calleeInfo.function.Result != "unknown" {
			for _, arg := range n.args {
				_ = inferExpr(arg, scope, out)
			}
			if parsed, err := parseStaticType(calleeInfo.function.Result); err == nil {
				return inferInfoFromStaticType(parsed)
			}
		}
		if name, ok := calledShapeName(n.callee); ok {
			if member, ok := n.callee.(*memberExpr); ok {
				object := inferExpr(member.object, scope, out)
				if object.module == "std/time" {
					return inferStdTimeCall(name, n.args, scope, out)
				}
				if object.module == "std/text" {
					return inferStdTextCall(name, n.args, scope, out)
				}
			}
			if info, ok := inferBuiltinCall(name, n.typeArgs, n.args, scope, out); ok {
				return info
			}
		}
		for _, arg := range n.args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{kind: "unknown"}
	case *memberExpr:
		object := inferExpr(n.object, scope, out)
		if object.module == "std/time" {
			switch n.name {
			case "nanosecond", "microsecond", "millisecond", "second", "minute", "hour":
				return inferTypeInfo{known: true, kind: "duration"}
			case "rfc3339", "rfc3339_nano":
				return inferTypeInfo{known: true, kind: "string"}
			case "utc_location":
				return inferTypeInfo{known: true, kind: "location"}
			}
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
	case *tupleExpr:
		for _, item := range n.items {
			_ = inferExpr(item, scope, out)
		}
		return inferTypeInfo{known: true, kind: "tuple", shape: []int{len(n.items)}}
	case *structExpr:
		for _, field := range n.Fields {
			_ = inferExpr(field.Expr, scope, out)
		}
		return inferTypeInfo{known: true, kind: n.TypeName}
	case *structUpdateExpr:
		base := inferExpr(n.Base, scope, out)
		for _, field := range n.Fields {
			_ = inferExpr(field.Expr, scope, out)
		}
		return base
	case *indexExpr:
		_ = inferExpr(n.Object, scope, out)
		_ = inferExpr(n.Index, scope, out)
		return inferTypeInfo{kind: "unknown"}
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
	case *propagateExpr:
		return inferExpr(n.expr, scope, out)
	case *matchExpr:
		_ = inferExpr(n.subject, scope, out)
		var outInfo inferTypeInfo
		have := false
		for _, arm := range n.arms {
			if arm.guard != nil {
				_ = inferExpr(arm.guard, scope, out)
			}
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

// inferBuiltinInfo handles the infer builtin info step in the surrounding
// package and returns inferTypeInfo, bool.
//
// It returns inferTypeInfo, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func inferBuiltinInfo(name string) (inferTypeInfo, bool) {
	switch name {
	case "true", "false":
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "null":
		return inferTypeInfo{known: true, kind: "null"}, true
	case "len", "sum", "product", "abs", "sqrt", "pow", "expt", "exp", "log", "log10", "log2", "hypot", "norm", "noise", "random", "real", "imag", "magnitude":
		return inferTypeInfo{known: true, kind: "number"}, true
	case "conjugate":
		return inferTypeInfo{known: true, kind: "complex"}, true
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
	case "openread", "openwrite", "openappend":
		return inferTypeInfo{known: true, kind: "stream"}, true
	case "readline":
		return inferTypeInfo{known: true, kind: "string"}, true
	case "readchunk", "read_limit", "read_dir":
		return inferTypeInfo{known: true, kind: "list"}, true
	case "writechunk", "append", "copy_file", "file_size", "seek", "tell":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "flush", "close", "truncate_file", "exists", "is_file", "is_dir", "mkdir", "mkdir_all", "rename", "remove", "truncate":
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "vector", "vec", "tensor", "matrix", "reshape", "transpose", "flatten", "view", "clone", "dot", "matmul":
		return inferTypeInfo{known: true, kind: "tensor"}, true
	default:
		if info, ok := namedConstantInfo(name); ok {
			return info, true
		}
		return inferTypeInfo{}, false
	}
}

// inferBuiltinCall handles the infer builtin call step in the surrounding
// package and returns inferTypeInfo, bool.
//
// It returns inferTypeInfo, bool so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func inferStdTimeCall(name string, args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	switch name {
	case "duration", "parse_duration", "sub":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "duration"}
	case "date", "unix", "unix_milli", "unix_micro", "parse", "add_date", "utc", "in_location", "now":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "datetime"}
	case "add":
		if len(args) > 0 {
			first := inferExpr(args[0], scope, out)
			for _, arg := range args[1:] {
				_ = inferExpr(arg, scope, out)
			}
			if first.kind == "datetime" {
				return inferTypeInfo{known: true, kind: "datetime"}
			}
		}
		return inferTypeInfo{known: true, kind: "duration"}
	case "round", "truncate":
		if len(args) > 0 {
			first := inferExpr(args[0], scope, out)
			for _, arg := range args[1:] {
				_ = inferExpr(arg, scope, out)
			}
			if first.kind == "duration" || first.kind == "datetime" {
				return first
			}
		}
		return inferTypeInfo{kind: "unknown"}
	case "duration_string", "format", "weekday":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "string"}
	case "fixed_zone", "load_location":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "location"}
	case "equal", "before", "after":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "bool"}
	case "compare":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "int", bits: 64}
	case "year", "month", "day", "hour", "minute", "second", "nanosecond", "year_day":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "uint", bits: 64}
	case "zone":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "list", shape: []int{2}}
	default:
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{kind: "unknown"}
	}
}

func inferStdTextCall(name string, args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	for _, arg := range args {
		_ = inferExpr(arg, scope, out)
	}
	switch name {
	case "compare":
		return inferTypeInfo{known: true, kind: "int", bits: 64}
	case "equal_fold", "contains", "contains_any", "has_prefix", "has_suffix":
		return inferTypeInfo{known: true, kind: "bool"}
	case "count":
		return inferTypeInfo{known: true, kind: "uint", bits: 64}
	case "index", "index_any", "last_index", "last_index_any", "cut", "cut_prefix", "cut_suffix":
		return inferTypeInfo{known: true, kind: "option"}
	case "fields", "split_after", "split_n", "split_after_n":
		return inferTypeInfo{known: true, kind: "list"}
	case "trim", "upper", "lower", "replace", "repeat", "join", "slug", "cat",
		"trim_space", "trim_left", "trim_right", "trim_prefix", "trim_suffix", "replace_n":
		return inferTypeInfo{known: true, kind: "string"}
	case "split", "chunks":
		return inferTypeInfo{known: true, kind: "list"}
	default:
		return inferTypeInfo{kind: "unknown"}
	}
}

func inferBuiltinCall(name string, typeArgs []string, args []Expr, scope *inferScope, out *InferProgram) (inferTypeInfo, bool) {
	if isFutureConstructorName(name) {
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		base := futureConstructorName(name)
		if len(typeArgs) > 0 {
			return inferTypeInfo{known: true, kind: base + "<" + strings.Join(typeArgs, ", ") + ">"}, true
		}
		if element, ok := inferFutureElementName(args); ok {
			if base == "simd" {
				lanes := 0
				if len(args) == 2 {
					if literal, ok := args[0].(*literalExpr); ok {
						lanes, _ = literal.value.Int()
					}
				} else if len(args) == 1 {
					lanes = futureInferValueCount(args[0])
				}
				if lanes > 0 {
					return inferTypeInfo{known: true, kind: fmt.Sprintf("simd<%d, %s>", lanes, element)}, true
				}
			} else if base == "poly" || base == "quat" {
				return inferTypeInfo{known: true, kind: base + "<" + element + ">"}, true
			}
		}
		return inferTypeInfo{known: true, kind: base}, true
	}
	switch name {
	case "openappend":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "stream"}, true
	case "read_limit", "read_dir":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "list"}, true
	case "append", "copy_file", "file_size", "seek", "tell":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "truncate_file", "exists", "is_file", "is_dir", "mkdir", "mkdir_all", "rename", "remove", "truncate":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "round":
		return inferUnaryLikeBuiltin(args, scope, out, "number"), true
	case "nnz":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "to_dense":
		if len(args) > 0 {
			_ = inferExpr(args[0], scope, out)
		}
		return inferTypeInfo{known: true, kind: "tensor"}, true
	case "to_sparse":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "sparse_tensor"}, true
	case "format":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "string"}, true
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
	case "atan2", "fmod", "remainder", "fma", "positive_difference", "copysign", "nextafter":
		return inferMathFloatResult(args, scope, out), true
	case "exp2", "trunc", "degrees", "radians", "sinpi", "cospi", "tanpi", "ldexp":
		return inferMathFloatResult(args[:min(len(args), 1)], scope, out), true
	case "frexp", "modf", "divmod":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "list", shape: []int{2}}, true
	case "isfinite", "isinf", "isnan", "isnormal", "signbit", "has_single_bit":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "bool"}, true
	case "count_ones", "count_zeros", "leading_zeros", "trailing_zeros", "bit_width":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "uint", bits: 64}, true
	case "bit_floor", "bit_ceil", "byte_swap":
		if len(args) == 1 {
			return inferExpr(args[0], scope, out), true
		}
		return inferTypeInfo{kind: "unknown"}, true
	case "gcd", "lcm":
		infos := make([]inferTypeInfo, 0, len(args))
		for _, arg := range args {
			infos = append(infos, inferExpr(arg, scope, out))
		}
		if len(infos) == 2 && (infos[0].kind == "uint" || infos[0].kind == "packeduint") &&
			(infos[1].kind == "uint" || infos[1].kind == "packeduint") {
			kind := "uint"
			if infos[0].kind == "packeduint" || infos[1].kind == "packeduint" {
				kind = "packeduint"
			}
			return inferTypeInfo{known: true, kind: kind, bits: max(infos[0].bits, infos[1].bits)}, true
		}
		return inferTypeInfo{known: true, kind: "bigint"}, true
	case "phase":
		if len(args) == 1 {
			info := inferExpr(args[0], scope, out)
			if info.bits == 32 || info.bits == 64 {
				return inferTypeInfo{known: true, kind: "float", bits: info.bits}, true
			}
		}
		return inferTypeInfo{known: true, kind: "number"}, true
	case "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "sign", "step", "heaviside", "relu", "sigmoid", "softplus":
		if (name == "sinh" || name == "cosh" || name == "tanh") && inferAnyComplexArgument(args, scope, out) {
			return inferTypeInfo{known: true, kind: "complex", bits: inferCallBits(args, scope, out)}, true
		}
		return inferUnaryLikeBuiltin(args, scope, out, "number"), true
	case "sqrt", "pow", "expt", "exp", "log":
		if inferAnyComplexArgument(args, scope, out) {
			return inferTypeInfo{known: true, kind: "complex", bits: inferCallBits(args, scope, out)}, true
		}
		return inferMathFloatResult(args, scope, out), true
	case "sin", "cos", "tan":
		if inferAnyComplexArgument(args, scope, out) {
			return inferTypeInfo{known: true, kind: "complex", bits: inferCallBits(args, scope, out)}, true
		}
		return inferMathFloatResult(args, scope, out), true
	case "mean", "var", "std", "median", "quantile", "percentile", "dot", "norm", "sum", "product", "len", "hypot", "abs", "log10", "log2", "noise", "real", "imag", "magnitude":
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
		return inferComplexCall(args, scope, out), true
	case "dual":
		for _, arg := range args {
			_ = inferExpr(arg, scope, out)
		}
		return inferTypeInfo{known: true, kind: "dual"}, true
	case "primal", "tangent":
		if len(args) == 1 {
			_ = inferExpr(args[0], scope, out)
		}
		return inferTypeInfo{known: true, kind: "number"}, true
	case "bigint":
		return inferTypeInfo{known: true, kind: "bigint"}, true
	case "bigfloat":
		return inferTypeInfo{known: true, kind: "bigfloat"}, true
	default:
		return inferTypeInfo{}, false
	}
}

func inferFutureElementName(args []Expr) (string, bool) {
	if len(args) == 2 && futureConstructorName("simd") == "simd" {
		args = args[1:]
	}
	if len(args) != 1 {
		return "", false
	}
	valuesExpr := args[0]
	var items []Expr
	switch n := valuesExpr.(type) {
	case *listExpr:
		items = n.items
	case *tupleExpr:
		items = n.items
	default:
		return "", false
	}
	values := make([]Value, len(items))
	for i, item := range items {
		literal, ok := item.(*literalExpr)
		if !ok {
			return "", false
		}
		values[i] = literal.value
	}
	element, err := futureElementType(values)
	return element, err == nil
}

func futureInferValueCount(expr Expr) int {
	switch n := expr.(type) {
	case *listExpr:
		return len(n.items)
	case *tupleExpr:
		return len(n.items)
	default:
		return 0
	}
}

// inferComplexCall retains an explicitly selected f32 or f64 component width
// from the constructor arguments. It follows runtime promotion: a default
// numeric argument widens a solely-f32 construction to f64.
func inferComplexCall(args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	infos := make([]inferTypeInfo, 0, len(args))
	for _, arg := range args {
		infos = append(infos, inferExpr(arg, scope, out))
	}
	return inferTypeInfo{known: true, kind: "complex", bits: inferComplexResultBits(infos...)}
}

func inferComplexResultBits(values ...inferTypeInfo) uint16 {
	var bits uint16
	sawExplicit := false
	sawDefault := false
	for _, value := range values {
		if value.bits == 0 {
			sawDefault = true
			continue
		}
		sawExplicit = true
		if value.bits > bits {
			bits = value.bits
		}
	}
	if !sawExplicit {
		return 0
	}
	if sawDefault && bits < 64 {
		return 64
	}
	return bits
}

// inferFixedCall handles the infer fixed call step in the surrounding package
// and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// inferTensorCall handles the infer tensor call step in the surrounding
// package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// inferShapeInfo handles the infer shape info step in the surrounding package
// and returns shapeInfo.
//
// It returns shapeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferShapeInfo(expr Expr, scope *inferScope, out *InferProgram) shapeInfo {
	info, err := shapeOfExpr(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil {
		return unknownShape()
	}
	return info
}

// inferShapeLiteral handles the infer shape literal step in the surrounding
// package and returns []int, bool.
//
// It returns []int, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferShapeLiteral(expr Expr, scope *inferScope, out *InferProgram) ([]int, bool) {
	dims, ok, err := intListLiteral(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil || !ok {
		return nil, false
	}
	return dims, true
}

// inferIntLiteral handles the infer int literal step in the surrounding
// package and returns int, bool.
//
// It returns int, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferIntLiteral(expr Expr, scope *inferScope, out *InferProgram) (int, bool) {
	n, ok, err := intLiteralExpr(expr, &shapeEnv{items: make(map[string]shapeInfo)})
	if err != nil || !ok {
		return 0, false
	}
	return n, true
}

// inferShapeArgs handles the infer shape args step in the surrounding package
// and returns []int, bool.
//
// It returns []int, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// inferUnaryLikeBuiltin handles the infer unary like builtin step in the
// surrounding package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

func inferMathFloatResult(args []Expr, scope *inferScope, out *InferProgram) inferTypeInfo {
	if len(args) == 0 {
		return inferTypeInfo{known: true, kind: "number"}
	}
	sawDefault := false
	sawFloat := false
	sawBFloat := false
	var bits uint16
	for _, arg := range args {
		info := inferExpr(arg, scope, out)
		switch info.kind {
		case "float":
			sawFloat = true
		case "bfloat":
			sawBFloat = true
		case "number":
			sawDefault = true
		}
		if info.bits > bits {
			bits = info.bits
		}
	}
	if !sawFloat && !sawBFloat {
		return inferTypeInfo{known: true, kind: "number"}
	}
	if sawDefault && bits < 64 {
		bits = 64
	}
	kind := "float"
	if sawBFloat {
		kind = "bfloat"
	}
	return inferTypeInfo{known: true, kind: kind, bits: bits}
}

func inferAnyComplexArgument(args []Expr, scope *inferScope, out *InferProgram) bool {
	for _, arg := range args {
		if inferExpr(arg, scope, out).kind == "complex" {
			return true
		}
	}
	return false
}

func inferCallBits(args []Expr, scope *inferScope, out *InferProgram) uint16 {
	infos := make([]inferTypeInfo, 0, len(args))
	for _, arg := range args {
		infos = append(infos, inferExpr(arg, scope, out))
	}
	return inferComplexResultBits(infos...)
}

// inferPreserveContainerBuiltin handles the infer preserve container builtin
// step in the surrounding package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// inferBinaryType handles the infer binary type step in the surrounding
// package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferBinaryType(op tokenKind, left, right inferTypeInfo) inferTypeInfo {
	switch op {
	case tokenEqualEqual, tokenBangEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual, tokenAndAnd, tokenOrOr:
		return inferTypeInfo{known: true, kind: "bool"}
	case tokenPlus, tokenMinus, tokenStar, tokenSlash, tokenPercent:
		if left.kind == "tensor" || right.kind == "tensor" {
			out := inferTypeInfo{known: true, kind: "tensor"}
			if left.kind == "tensor" && right.kind == "tensor" && len(left.shape) > 0 && len(right.shape) > 0 {
				if shape, err := tensorBroadcastShape(left.shape, right.shape); err == nil {
					out.shape = shape
				}
			} else if len(left.shape) > 0 {
				out.shape = append([]int(nil), left.shape...)
			} else if len(right.shape) > 0 {
				out.shape = append([]int(nil), right.shape...)
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

// promoteNumeric handles the promote numeric step in the surrounding package
// and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func promoteNumeric(left, right inferTypeInfo) inferTypeInfo {
	switch {
	case left.kind == "dual" || right.kind == "dual":
		return inferTypeInfo{known: true, kind: "dual"}
	case left.kind == "complex" || right.kind == "complex":
		return inferTypeInfo{known: true, kind: "complex", bits: inferComplexResultBits(left, right)}
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

// inferJoinType handles the infer join type step in the surrounding package
// and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func inferJoinType(a, b inferTypeInfo) inferTypeInfo {
	if a.kind == b.kind && a.ShapeString() == b.ShapeString() && formatDims(a.units) == formatDims(b.units) && a.bits == b.bits && a.frac == b.frac && a.module == b.module {
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

// inferTypeFromValue handles the infer type from value step in the surrounding
// package and returns inferTypeInfo.
//
// It returns inferTypeInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
		info.bits = v.fbits
	case kindDual:
		info.kind = "dual"
	case kindDuration:
		info.kind = "duration"
	case kindDateTime:
		info.kind = "datetime"
	case kindTimeLocation:
		info.kind = "location"
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
		if v.mod != nil {
			info.module = v.mod.name
		}
	case kindFuture:
		if v.future != nil {
			info.kind = futureTypeString(v.future)
			if v.future.name == "poly" {
				info.shape = []int{len(v.future.data)}
			} else {
				info.shape = append([]int(nil), v.future.shape...)
			}
		} else {
			info.kind = "future"
		}
	default:
		info.kind = "unknown"
	}
	return info
}

func inferInfoFromStaticType(value staticType) inferTypeInfo {
	if value.isDynamic() {
		return inferTypeInfo{kind: "unknown"}
	}
	if value.isFunction() {
		report := &InferFunction{Signature: value.String()}
		if value.result != nil {
			report.Result = value.result.String()
		}
		return inferTypeInfo{known: true, kind: "function", function: report}
	}
	switch value.name {
	case "u8", "u16", "u32", "u64":
		var bits uint16
		_, _ = fmt.Sscanf(value.name, "u%d", &bits)
		return inferTypeInfo{known: true, kind: "uint", bits: bits}
	case "i8", "i16", "i32", "i64":
		var bits uint16
		_, _ = fmt.Sscanf(value.name, "i%d", &bits)
		return inferTypeInfo{known: true, kind: "int", bits: bits}
	case "f32", "f64":
		var bits uint16
		_, _ = fmt.Sscanf(value.name, "f%d", &bits)
		return inferTypeInfo{known: true, kind: "float", bits: bits}
	default:
		if value.name == "poly" || value.name == "quat" || value.name == "simd" || value.name == "sparse_matrix" || value.name == "sparse_tensor" {
			return inferTypeInfo{known: true, kind: value.String()}
		}
		return inferTypeInfo{known: true, kind: value.name}
	}
}

// KindString operates on inferTypeInfo values and keeps the
// infertypeinfo-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
	case "complex":
		if i.bits == 0 {
			return "complex"
		}
		return fmt.Sprintf("complex<f%d>", i.bits)
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

// ShapeString operates on inferTypeInfo values and keeps the
// infertypeinfo-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (i inferTypeInfo) ShapeString() string {
	if len(i.shape) == 0 {
		return ""
	}
	return tensorShapeString(i.shape)
}

// UnitsString operates on inferTypeInfo values and keeps the
// infertypeinfo-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (i inferTypeInfo) UnitsString() string {
	return formatDims(i.units)
}

// maxBits handles the max bits step in the surrounding package and returns
// uint16.
//
// It returns uint16 so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func maxBits(a, b uint16) uint16 {
	if a > b {
		return a
	}
	return b
}
