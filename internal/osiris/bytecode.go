package osiris

import (
	"context"
	"fmt"
	"math"
)

type vmOp uint8

const (
	vmOpConst vmOp = iota
	vmOpLoadName
	vmOpBind
	vmOpResultBind
	vmOpTypeAssert
	vmOpUnary
	vmOpBinary
	vmOpCall
	vmOpList
	vmOpMember
	vmOpImport
	vmOpEmit
	vmOpBlock
	vmOpIf
	vmOpLogicalAnd
	vmOpLogicalOr
	vmOpMatch
	vmOpTuple
	vmOpStruct
	vmOpStructUpdate
	vmOpIndex
	vmOpTupleBind
)

type vmChunk struct {
	sourceName  string
	packageName string
	exports     []string
	result      int
	maxReg      int
	code        []vmInstr
}

type vmInstr struct {
	op         vmOp
	dst        int
	a          int
	b          int
	tok        tokenKind
	name       string
	text       string
	value      Value
	args       []int
	fieldNames []string
	names      []string
	body       *vmChunk
	alt        *vmChunk
	arms       []vmMatchArm
	span       sourceSpan
}

type vmMatchArm struct {
	pattern   Pattern
	guard     *vmChunk
	body      *vmChunk
	isDefault bool
}

type vmCompiler struct {
	chunk   *vmChunk
	next    int
	emitted bool
}

// newVMCompiler handles the new vmcompiler step in the surrounding package and
// returns *vmCompiler.
//
// It returns *vmCompiler so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newVMCompiler(sourceName string) *vmCompiler {
	return &vmCompiler{chunk: &vmChunk{sourceName: sourceName, result: -1}}
}

// alloc operates on vmCompiler values and keeps the vmcompiler-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns int so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (c *vmCompiler) alloc() int {
	reg := c.next
	c.next++
	if reg > c.chunk.maxReg {
		c.chunk.maxReg = reg
	}
	return reg
}

// emit operates on vmCompiler values and keeps the vmcompiler-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (c *vmCompiler) emit(instr vmInstr) {
	c.chunk.code = append(c.chunk.code, instr)
	c.emitted = true
}

// compileProgram operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *vmChunk and an error. Callers should only trust the value when
// the error is nil.
func (c *vmCompiler) compileProgram(prog *Program) (*vmChunk, error) {
	for _, stmt := range prog.Statements {
		if err := c.compileStmt(stmt); err != nil {
			return nil, err
		}
	}
	if len(c.chunk.exports) == 0 {
		c.chunk.exports = nil
	}
	return c.chunk, nil
}

// compileStmt operates on vmCompiler values and keeps the vmcompiler-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (c *vmCompiler) compileStmt(stmt Stmt) error {
	switch st := stmt.(type) {
	case packageStmt:
		if c.chunk.packageName != "" && c.chunk.packageName != st.Name {
			return fmt.Errorf("package redeclared as %q (was %q)", st.Name, c.chunk.packageName)
		}
		c.chunk.packageName = st.Name
		return nil
	case exportStmt:
		seen := make(map[string]struct{}, len(c.chunk.exports))
		for _, name := range c.chunk.exports {
			seen[name] = struct{}{}
		}
		for _, name := range st.Names {
			if _, ok := seen[name]; ok {
				continue
			}
			c.chunk.exports = append(c.chunk.exports, name)
			seen[name] = struct{}{}
		}
		return nil
	case enumStmt:
		reg := c.alloc()
		c.emit(vmInstr{op: vmOpConst, dst: reg, value: enumNamespaceValue(st.Name, st.Cases)})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		return nil
	case structStmt:
		reg := c.alloc()
		c.emit(vmInstr{op: vmOpConst, dst: reg, value: structTypeValueOf(runtimeStructType(st))})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		return nil
	case typeDeclStmt:
		reg := c.alloc()
		c.emit(vmInstr{op: vmOpConst, dst: reg, value: functionValueOf(st.Name, func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("%s expects 1 argument", st.Name)
			}
			return args[0], nil
		})})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		return nil
	case newtypeStmt:
		reg := c.alloc()
		typ := &structTypeValue{name: st.Name, target: st.Target, newtype: true, fields: []structFieldValue{{name: "value", typeName: st.Target}}, byName: map[string]int{"value": 0}}
		c.emit(vmInstr{op: vmOpConst, dst: reg, value: structTypeValueOf(typ)})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		return nil
	case letStmt:
		reg, err := c.compileExpr(st.Expr)
		if err != nil {
			return err
		}
		if st.Unwrap {
			unwrapped := c.alloc()
			c.emit(vmInstr{op: vmOpResultBind, dst: unwrapped, a: reg})
			reg = unwrapped
		}
		if st.Type != "" {
			checked := c.alloc()
			c.emit(vmInstr{op: vmOpTypeAssert, dst: checked, a: reg, text: st.Type})
			reg = checked
		}
		if len(st.Names) > 0 {
			c.emit(vmInstr{op: vmOpTupleBind, a: reg, names: append([]string(nil), st.Names...)})
		} else {
			c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		}
		return nil
	case importStmt:
		reg := c.alloc()
		c.emit(vmInstr{op: vmOpImport, dst: reg, name: st.Alias, text: st.Path})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Alias})
		return nil
	case emitStmt:
		reg, err := c.compileExpr(st.Expr)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpEmit, a: reg})
		return nil
	case macroStmt:
		return fmt.Errorf("macro statements must be expanded before bytecode compilation")
	default:
		return fmt.Errorf("internal error: unknown statement type %T", stmt)
	}
}

// compileExpr operates on vmCompiler values and keeps the vmcompiler-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileExpr(expr Expr) (int, error) {
	switch n := expr.(type) {
	case *literalExpr:
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpConst, dst: dst, value: n.value})
		return dst, nil
	case *identExpr:
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpLoadName, dst: dst, name: n.name})
		return dst, nil
	case *memberExpr:
		obj, err := c.compileExpr(n.object)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpMember, dst: dst, a: obj, name: n.name})
		return dst, nil
	case *unaryExpr:
		src, err := c.compileExpr(n.expr)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpUnary, dst: dst, a: src, tok: n.op})
		return dst, nil
	case *binaryExpr:
		if n.op == tokenAndAnd || n.op == tokenOrOr {
			return c.compileLogicalBinary(n)
		}
		left, err := c.compileExpr(n.left)
		if err != nil {
			return 0, err
		}
		right, err := c.compileExpr(n.right)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpBinary, dst: dst, a: left, b: right, tok: n.op})
		return dst, nil
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "if":
				return c.compileIfCall(n)
			case "and":
				return c.compileAndCall(n)
			case "or":
				return c.compileOrCall(n)
			}
		}
		callee, err := c.compileExpr(n.callee)
		if err != nil {
			return 0, err
		}
		args := make([]int, len(n.args))
		for i, arg := range n.args {
			reg, err := c.compileExpr(arg)
			if err != nil {
				return 0, err
			}
			args[i] = reg
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpCall, dst: dst, a: callee, args: args, span: n.Span})
		return dst, nil
	case *listExpr:
		items := make([]int, len(n.items))
		for i, item := range n.items {
			reg, err := c.compileExpr(item)
			if err != nil {
				return 0, err
			}
			items[i] = reg
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpList, dst: dst, args: items})
		return dst, nil
	case *tupleExpr:
		items := make([]int, len(n.items))
		for i, item := range n.items {
			reg, err := c.compileExpr(item)
			if err != nil {
				return 0, err
			}
			items[i] = reg
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpTuple, dst: dst, args: items})
		return dst, nil
	case *structExpr:
		args := make([]int, len(n.Fields))
		names := make([]string, len(n.Fields))
		for i, field := range n.Fields {
			reg, err := c.compileExpr(field.Expr)
			if err != nil {
				return 0, err
			}
			args[i], names[i] = reg, field.Name
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpStruct, dst: dst, name: n.TypeName, args: args, fieldNames: names})
		return dst, nil
	case *structUpdateExpr:
		base, err := c.compileExpr(n.Base)
		if err != nil {
			return 0, err
		}
		args := make([]int, len(n.Fields))
		names := make([]string, len(n.Fields))
		for i, field := range n.Fields {
			reg, err := c.compileExpr(field.Expr)
			if err != nil {
				return 0, err
			}
			args[i], names[i] = reg, field.Name
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpStructUpdate, dst: dst, a: base, args: args, fieldNames: names})
		return dst, nil
	case *indexExpr:
		object, err := c.compileExpr(n.Object)
		if err != nil {
			return 0, err
		}
		index, err := c.compileExpr(n.Index)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpIndex, dst: dst, a: object, b: index})
		return dst, nil
	case *blockExpr:
		return c.compileBlockExpr(n)
	case *matchExpr:
		subject, err := c.compileExpr(n.subject)
		if err != nil {
			return 0, err
		}
		arms := make([]vmMatchArm, len(n.arms))
		for i, arm := range n.arms {
			var guard *vmChunk
			if arm.guard != nil {
				guard, err = c.compileExprChunk(arm.guard)
				if err != nil {
					return 0, err
				}
			}
			body, err := c.compileExprChunk(arm.body)
			if err != nil {
				return 0, err
			}
			arms[i] = vmMatchArm{pattern: arm.pattern, guard: guard, body: body, isDefault: arm.isDefault}
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpMatch, dst: dst, a: subject, arms: arms})
		return dst, nil
	default:
		return 0, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// compileBlockExpr operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileBlockExpr(n *blockExpr) (int, error) {
	sub := newVMCompiler(c.chunk.sourceName)
	sub.chunk.packageName = c.chunk.packageName
	sub.chunk.exports = append([]string(nil), c.chunk.exports...)
	for _, stmt := range n.stmts {
		if err := sub.compileStmt(stmt); err != nil {
			return 0, err
		}
	}
	var result int = -1
	if n.tail != nil {
		reg, err := sub.compileExpr(n.tail)
		if err != nil {
			return 0, err
		}
		result = reg
	}
	sub.chunk.result = result
	dst := c.alloc()
	c.emit(vmInstr{op: vmOpBlock, dst: dst, body: sub.chunk})
	return dst, nil
}

// compileExprChunk operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *vmChunk and an error. Callers should only trust the value when
// the error is nil.
func (c *vmCompiler) compileExprChunk(expr Expr) (*vmChunk, error) {
	sub := newVMCompiler(c.chunk.sourceName)
	sub.chunk.packageName = c.chunk.packageName
	sub.chunk.exports = append([]string(nil), c.chunk.exports...)
	reg, err := sub.compileExpr(expr)
	if err != nil {
		return nil, err
	}
	sub.chunk.result = reg
	return sub.chunk, nil
}

// compileIfCall operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileIfCall(n *callExpr) (int, error) {
	if len(n.args) != 3 {
		return 0, fmt.Errorf("if expects 3 arguments")
	}
	cond, err := c.compileExpr(n.args[0])
	if err != nil {
		return 0, err
	}
	thenChunk, err := c.compileExprChunk(n.args[1])
	if err != nil {
		return 0, err
	}
	elseChunk, err := c.compileExprChunk(n.args[2])
	if err != nil {
		return 0, err
	}
	dst := c.alloc()
	c.emit(vmInstr{op: vmOpIf, dst: dst, a: cond, body: thenChunk, alt: elseChunk})
	return dst, nil
}

// compileAndCall operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileAndCall(n *callExpr) (int, error) {
	if len(n.args) != 2 {
		return 0, fmt.Errorf("and expects 2 arguments")
	}
	left, err := c.compileExpr(n.args[0])
	if err != nil {
		return 0, err
	}
	right, err := c.compileExprChunk(n.args[1])
	if err != nil {
		return 0, err
	}
	dst := c.alloc()
	c.emit(vmInstr{op: vmOpLogicalAnd, dst: dst, a: left, body: right})
	return dst, nil
}

// compileOrCall operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileOrCall(n *callExpr) (int, error) {
	if len(n.args) != 2 {
		return 0, fmt.Errorf("or expects 2 arguments")
	}
	left, err := c.compileExpr(n.args[0])
	if err != nil {
		return 0, err
	}
	right, err := c.compileExprChunk(n.args[1])
	if err != nil {
		return 0, err
	}
	dst := c.alloc()
	c.emit(vmInstr{op: vmOpLogicalOr, dst: dst, a: left, body: right})
	return dst, nil
}

// compileLogicalBinary operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (c *vmCompiler) compileLogicalBinary(n *binaryExpr) (int, error) {
	left, err := c.compileExpr(n.left)
	if err != nil {
		return 0, err
	}
	right, err := c.compileExprChunk(n.right)
	if err != nil {
		return 0, err
	}
	dst := c.alloc()
	switch n.op {
	case tokenAndAnd:
		c.emit(vmInstr{op: vmOpLogicalAnd, dst: dst, a: left, body: right})
	case tokenOrOr:
		c.emit(vmInstr{op: vmOpLogicalOr, dst: dst, a: left, body: right})
	}
	return dst, nil
}

type vmFrame struct {
	parent *vmFrame
	locals map[string]Value
}

// lookup operates on vmFrame values and keeps the vmframe-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (f *vmFrame) lookup(name string) (Value, bool) {
	for cur := f; cur != nil; cur = cur.parent {
		if cur.locals == nil {
			continue
		}
		if v, ok := cur.locals[name]; ok {
			return v, true
		}
	}
	return Value{}, false
}

// bind operates on vmFrame values and keeps the vmframe-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (f *vmFrame) bind(name string, v Value) {
	if f.locals == nil {
		f.locals = make(map[string]Value)
	}
	f.locals[name] = v
}

// child operates on vmFrame values and keeps the vmframe-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *vmFrame so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (f *vmFrame) child(bindings map[string]Value) *vmFrame {
	child := &vmFrame{parent: f}
	if len(bindings) > 0 {
		child.locals = make(map[string]Value, len(bindings))
		for k, v := range bindings {
			child.locals[k] = v
		}
	}
	return child
}

type vmRunner struct {
	rt        *Runtime
	ctx       context.Context
	source    string
	stack     []string
	frame     *vmFrame
	outputs   []Value
	constRegs []Value
}

// run operates on vmRunner values and keeps the vmrunner-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (r *vmRunner) run(chunk *vmChunk) (Value, error) {
	regs := make([]Value, chunk.maxReg+1)
	for _, instr := range chunk.code {
		if err := r.ctx.Err(); err != nil {
			return nullValue(), err
		}
		switch instr.op {
		case vmOpConst:
			regs[instr.dst] = instr.value
		case vmOpLoadName:
			if v, ok := r.frame.lookup(instr.name); ok {
				regs[instr.dst] = v
				continue
			}
			if v, ok := r.rt.Constants[instr.name]; ok {
				regs[instr.dst] = v
				continue
			}
			if fn, ok := r.rt.ContextBuiltins[instr.name]; ok {
				regs[instr.dst] = contextFunctionValueOf(instr.name, fn)
				continue
			}
			if fn, ok := r.rt.Builtins[instr.name]; ok {
				regs[instr.dst] = functionValueOf(instr.name, fn)
				continue
			}
			return nullValue(), fmt.Errorf("undefined name %q", instr.name)
		case vmOpBind:
			r.frame.bind(instr.name, regs[instr.a])
		case vmOpTupleBind:
			value := regs[instr.a]
			if value.kind != kindTuple || len(value.tuple) != len(instr.names) {
				return nullValue(), fmt.Errorf("tuple destructuring requires %d values", len(instr.names))
			}
			for i, name := range instr.names {
				r.frame.bind(name, value.tuple[i])
			}
		case vmOpResultBind:
			value, err := unwrapBinding(regs[instr.a])
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = value
		case vmOpTypeAssert:
			value, err := checkBindingType(instr.text, regs[instr.a], false)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = value
		case vmOpUnary:
			v, err := applyUnary(instr.tok, regs[instr.a])
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpBinary:
			switch instr.tok {
			case tokenAmp, tokenPipe, tokenCaret, tokenShiftLeft, tokenShiftRight:
				v, err := evalBitwiseBinary(instr.tok, regs[instr.a], regs[instr.b])
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = v
				continue
			}
			v, err := vmBinary(r.ctx, instr.tok, regs[instr.a], regs[instr.b])
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpCall:
			callee := regs[instr.a]
			if callee.kind == kindDuration {
				args := make([]Value, len(instr.args))
				for i, reg := range instr.args {
					args[i] = regs[reg]
				}
				out, err := callDurationComponent(callee, args)
				if err != nil {
					return nullValue(), structuredErrorAt(normalizeError(err, r.source), r.source, instr.span)
				}
				regs[instr.dst] = out
				continue
			}
			if callee.kind == kindStruct && callee.structType != nil {
				args := make([]Value, len(instr.args))
				for i, reg := range instr.args {
					args[i] = regs[reg]
				}
				out, err := constructStruct(callee.structType, args)
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = out
				continue
			}
			if callee.kind != kindFunction || callee.fn == nil {
				if callee.kind == kindModule {
					return nullValue(), fmt.Errorf("cannot call module %s", callee.String())
				}
				return nullValue(), fmt.Errorf("value %s is not callable", callee.Kind())
			}
			args := make([]Value, len(instr.args))
			for i, reg := range instr.args {
				args[i] = regs[reg]
			}
			if callee.fn.builtin != nil {
				out, err := callee.fn.builtin(args)
				if err != nil {
					return nullValue(), structuredErrorAt(normalizeError(err, r.source), r.source, instr.span)
				}
				regs[instr.dst] = out
				continue
			}
			if callee.fn.contextBuiltin != nil {
				out, err := callee.fn.contextBuiltin(r.ctx, args)
				if err != nil {
					return nullValue(), structuredErrorAt(normalizeError(err, r.source), r.source, instr.span)
				}
				regs[instr.dst] = out
				continue
			}
			if callee.fn.session == nil || callee.fn.body == nil {
				return nullValue(), fmt.Errorf("function %s is missing executable body", callee.fn.name)
			}
			checkedArgs, err := checkFunctionArguments(callee.fn, args)
			if err != nil {
				return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, err.Error()), r.source, instr.span)
			}
			args = checkedArgs
			base := callee.fn.closure
			if base == nil {
				base = &scope{}
			}
			callScope := &scope{
				parent: base,
				items:  make(map[string]binding, len(callee.fn.params)),
			}
			for i, name := range callee.fn.params {
				callScope.items[name] = binding{expr: &literalExpr{value: args[i]}, env: callScope}
			}
			callSession := callee.fn.session
			previousOutputs := callSession.outputs
			callSession.outputs = &r.outputs
			out, err := callSession.eval(callee.fn.body, callScope)
			callSession.outputs = previousOutputs
			if sig, ok := asFlow(err); ok {
				if sig.kind == flowReturn {
					value, typeErr := checkFunctionResult(callee.fn, sig.value)
					if typeErr != nil {
						return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, typeErr.Error()), r.source, instr.span)
					}
					regs[instr.dst] = value
					continue
				}
				return nullValue(), flowError(sig.kind, "function")
			}
			if err != nil {
				return nullValue(), err
			}
			out, err = checkFunctionResult(callee.fn, out)
			if err != nil {
				return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, err.Error()), r.source, instr.span)
			}
			regs[instr.dst] = out
		case vmOpList:
			items := make([]Value, len(instr.args))
			for i, reg := range instr.args {
				items[i] = regs[reg]
			}
			regs[instr.dst] = listValue(items)
		case vmOpTuple:
			items := make([]Value, len(instr.args))
			for i, reg := range instr.args {
				items[i] = regs[reg]
			}
			regs[instr.dst] = tupleValue(items)
		case vmOpStruct:
			callee, ok := r.frame.lookup(instr.name)
			if !ok {
				return nullValue(), fmt.Errorf("undefined name %q", instr.name)
			}
			if callee.kind != kindStruct || callee.structType == nil {
				return nullValue(), fmt.Errorf("%s is not a struct type", instr.name)
			}
			values := make([]Value, len(callee.structType.fields))
			for i := range values {
				values[i] = nullValue()
			}
			for i, reg := range instr.args {
				idx, ok := callee.structType.byName[instr.fieldNames[i]]
				if !ok {
					return nullValue(), fmt.Errorf("struct %s has no field %q", instr.name, instr.fieldNames[i])
				}
				values[idx] = regs[reg]
			}
			for i, value := range values {
				if value.kind == kindNull {
					return nullValue(), fmt.Errorf("struct %s is missing field %q", instr.name, callee.structType.fields[i].name)
				}
			}
			regs[instr.dst] = structInstanceValueOf(callee.structType, values)
		case vmOpStructUpdate:
			base := regs[instr.a]
			if base.kind != kindStruct || base.structType == nil || base.structValue == nil {
				return nullValue(), fmt.Errorf("struct update requires a struct value")
			}
			values := append([]Value(nil), base.structValue.fields...)
			for i, reg := range instr.args {
				idx, ok := base.structType.byName[instr.fieldNames[i]]
				if !ok {
					return nullValue(), fmt.Errorf("struct %s has no field %q", base.structType.name, instr.fieldNames[i])
				}
				values[idx] = regs[reg]
			}
			regs[instr.dst] = structInstanceValueOf(base.structType, values)
		case vmOpIndex:
			value, err := atValue([]Value{regs[instr.a], regs[instr.b]})
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = value
		case vmOpMember:
			obj := regs[instr.a]
			v, err := resolveMember(r.ctx, obj, instr.name)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpImport:
			mod, err := r.rt.loadModule(r.ctx, instr.text, r.source, r.stack)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = mod
		case vmOpEmit:
			r.outputs = append(r.outputs, regs[instr.a])
		case vmOpBlock:
			v, err := r.runChild(instr.body)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpIf:
			if regs[instr.a].Truthy() {
				v, err := r.runChild(instr.body)
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = v
			} else {
				v, err := r.runChild(instr.alt)
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = v
			}
		case vmOpLogicalAnd:
			if !regs[instr.a].Truthy() {
				regs[instr.dst] = boolValue(false)
				continue
			}
			v, err := r.runChild(instr.body)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = boolValue(v.Truthy())
		case vmOpLogicalOr:
			if regs[instr.a].Truthy() {
				regs[instr.dst] = boolValue(true)
				continue
			}
			v, err := r.runChild(instr.body)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = boolValue(v.Truthy())
		case vmOpMatch:
			subject := regs[instr.a]
			matched := false
			for _, arm := range instr.arms {
				state := matchState{bindings: make(map[string]Value, 4)}
				ok, err := matchPattern(arm.pattern, subject, &state)
				if err != nil {
					return nullValue(), err
				}
				if !ok {
					continue
				}
				if arm.guard != nil {
					guard, err := r.runChildWithBindings(arm.guard, state.bindings)
					if err != nil {
						return nullValue(), err
					}
					if !guard.Truthy() {
						continue
					}
				}
				v, err := r.runChildWithBindings(arm.body, state.bindings)
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = v
				matched = true
				break
			}
			if !matched {
				return nullValue(), noMatchError(subject, vmMatchArmsToMatchArms(instr.arms))
			}
		default:
			return nullValue(), fmt.Errorf("unknown opcode %d", instr.op)
		}
	}
	if chunk.result >= 0 && chunk.result < len(regs) {
		return regs[chunk.result], nil
	}
	return nullValue(), nil
}

func vmMatchArmsToMatchArms(arms []vmMatchArm) []matchArm {
	out := make([]matchArm, len(arms))
	for i, arm := range arms {
		out[i].pattern = arm.pattern
		out[i].isDefault = arm.isDefault
	}
	return out
}

// runChild operates on vmRunner values and keeps the vmrunner-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (r *vmRunner) runChild(chunk *vmChunk) (Value, error) {
	if chunk == nil {
		return nullValue(), nil
	}
	child := &vmRunner{
		rt:     r.rt,
		ctx:    r.ctx,
		source: r.source,
		stack:  r.stack,
		frame:  r.frame.child(nil),
	}
	value, err := child.run(chunk)
	r.outputs = append(r.outputs, child.outputs...)
	return value, err
}

// runChildWithBindings operates on vmRunner values and keeps the
// vmrunner-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (r *vmRunner) runChildWithBindings(chunk *vmChunk, bindings map[string]Value) (Value, error) {
	if chunk == nil {
		return nullValue(), nil
	}
	child := &vmRunner{
		rt:     r.rt,
		ctx:    r.ctx,
		source: r.source,
		stack:  r.stack,
		frame:  r.frame.child(bindings),
	}
	value, err := child.run(chunk)
	r.outputs = append(r.outputs, child.outputs...)
	return value, err
}

// vmBinary handles the vm binary step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func vmBinary(ctx context.Context, op tokenKind, left, right Value) (Value, error) {
	if left.kind == kindTensor || right.kind == kindTensor {
		return tensorBinaryValue(ctx, deterministicScheduler{}, op, left, right)
	}
	if left.kind == kindDual || right.kind == kindDual {
		switch op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash:
			return dualBinary(op, left, right)
		case tokenEqualEqual:
			return boolValue(left.Equal(right)), nil
		case tokenBangEqual:
			return boolValue(!left.Equal(right)), nil
		default:
			return nullValue(), fmt.Errorf("ordering is not defined for dual numbers")
		}
	}
	if left.kind == kindQuantity || right.kind == kindQuantity {
		return quantityBinary(op, left, right)
	}
	if left.kind == kindInterval || right.kind == kindInterval {
		return intervalBinary(op, left, right)
	}
	if left.kind == kindModInt || right.kind == kindModInt {
		return modIntBinary(op, left, right)
	}
	if left.kind == kindFixed || right.kind == kindFixed {
		return fixedBinary(op, left, right)
	}
	if left.kind == kindBigInt || left.kind == kindBigFloat || left.kind == kindInt || right.kind == kindBigInt || right.kind == kindBigFloat || right.kind == kindInt {
		return bigBinary(op, left, right)
	}
	if (left.kind == kindComplex || right.kind == kindComplex) && left.kind != kindString && right.kind != kindString {
		switch op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash:
			return complexBinary(op, left, right)
		case tokenEqualEqual:
			return boolValue(left.Equal(right)), nil
		case tokenBangEqual:
			return boolValue(!left.Equal(right)), nil
		case tokenPercent:
			return nullValue(), fmt.Errorf("complex modulo is not supported")
		default:
			return nullValue(), fmt.Errorf("ordering is not defined for complex numbers")
		}
	}
	if (left.kind == kindDecimal || right.kind == kindDecimal) && left.kind != kindComplex && right.kind != kindComplex {
		switch op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash:
			return decimalBinary(op, left, right)
		case tokenEqualEqual:
			return boolValue(left.Equal(right)), nil
		case tokenBangEqual:
			return boolValue(!left.Equal(right)), nil
		case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
			cmp, err := decimalCompare(left, right)
			if err != nil {
				return nullValue(), err
			}
			switch op {
			case tokenLess:
				return boolValue(cmp < 0), nil
			case tokenLessEqual:
				return boolValue(cmp <= 0), nil
			case tokenGreater:
				return boolValue(cmp > 0), nil
			case tokenGreaterEqual:
				return boolValue(cmp >= 0), nil
			}
		case tokenPercent:
			return nullValue(), fmt.Errorf("decimal modulo is not supported")
		}
	}
	if left.kind == kindUint && right.kind == kindUint {
		if isWideUintValue(left) || isWideUintValue(right) || left.packed || right.packed {
			switch op {
			case tokenPlus, tokenMinus, tokenStar, tokenPercent:
				return wideUnsignedBinary(op, left, right)
			case tokenSlash:
				return exactUnsignedDivisionValue(left, right)
			}
		}
		lu, lw, err := uintFromValue(left)
		if err != nil {
			return nullValue(), err
		}
		ru, rw, err := uintFromValue(right)
		if err != nil {
			return nullValue(), err
		}
		width := lw
		if rw > width {
			width = rw
		}
		switch op {
		case tokenPlus:
			return uintValue(lu+ru, width), nil
		case tokenMinus:
			return uintValue(lu-ru, width), nil
		case tokenStar:
			return uintValue(lu*ru, width), nil
		case tokenSlash:
			return exactUnsignedDivisionValue(left, right)
		case tokenPercent:
			if ru == 0 {
				return nullValue(), fmt.Errorf("modulo by zero")
			}
			return uintValue(lu%ru, width), nil
		}
	}
	switch op {
	case tokenPlus:
		if left.kind == kindString || right.kind == kindString {
			return stringValue(textify(left) + textify(right)), nil
		}
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return floatResultValue(ln+rn, left, right), nil
	case tokenMinus:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return floatResultValue(ln-rn, left, right), nil
	case tokenStar:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return floatResultValue(ln*rn, left, right), nil
	case tokenSlash:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if rn == 0 {
			return nullValue(), fmt.Errorf("division by zero")
		}
		return floatResultValue(ln/rn, left, right), nil
	case tokenPercent:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		if rn == 0 {
			return nullValue(), fmt.Errorf("modulo by zero")
		}
		return floatResultValue(math.Mod(ln, rn), left, right), nil
	case tokenEqualEqual:
		return boolValue(left.Equal(right)), nil
	case tokenBangEqual:
		return boolValue(!left.Equal(right)), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		cmp, err := left.Compare(right)
		if err != nil {
			return nullValue(), err
		}
		switch op {
		case tokenLess:
			return boolValue(cmp < 0), nil
		case tokenLessEqual:
			return boolValue(cmp <= 0), nil
		case tokenGreater:
			return boolValue(cmp > 0), nil
		case tokenGreaterEqual:
			return boolValue(cmp >= 0), nil
		}
	}
	return nullValue(), fmt.Errorf("unsupported operator %s", op)
}

// RunVM operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (r *Runtime) RunVM(ctx context.Context, prog *Program) ([]Value, error) {
	sess := NewSession(r, ctx, prog.SourceName)
	expanded, err := sess.ExpandProgram(prog)
	if err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	if err := validateTensorShapes(expanded); err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	if _, err := r.CheckProgram(ctx, expanded); err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	if containsControlFlow(expanded) {
		outputs, err := sess.ExecProgram(expanded, false, nil)
		if err != nil {
			return outputs, normalizeError(err, prog.SourceName)
		}
		return outputs, nil
	}
	compiler := newVMCompiler(prog.SourceName)
	chunk, err := compiler.compileProgram(expanded)
	if err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	runner := &vmRunner{
		rt:     r,
		ctx:    ctx,
		source: prog.SourceName,
		stack:  nil,
		frame:  &vmFrame{},
	}
	if _, err := runner.run(chunk); err != nil {
		return runner.outputs, normalizeError(err, prog.SourceName)
	}
	return runner.outputs, nil
}

// containsControlFlow handles the contains control flow step in the
// surrounding package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func containsControlFlow(prog *Program) bool {
	if prog == nil {
		return false
	}
	for _, stmt := range prog.Statements {
		if stmtContainsControl(stmt) {
			return true
		}
	}
	return false
}

// stmtContainsControl handles the stmt contains control step in the
// surrounding package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func stmtContainsControl(stmt Stmt) bool {
	switch st := stmt.(type) {
	case funcStmt, whileStmt, forStmt, returnStmt, breakStmt, continueStmt:
		return true
	case letStmt:
		return exprContainsControl(st.Expr)
	case emitStmt:
		return exprContainsControl(st.Expr)
	case macroStmt:
		return exprContainsControl(st.Body)
	case importStmt, packageStmt, exportStmt:
		return false
	default:
		return false
	}
}

// exprContainsControl handles the expr contains control step in the
// surrounding package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func exprContainsControl(expr Expr) bool {
	switch n := expr.(type) {
	case *blockExpr:
		for _, stmt := range n.stmts {
			if stmtContainsControl(stmt) {
				return true
			}
		}
		return n.tail != nil && exprContainsControl(n.tail)
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "spawn", "await", "cancel", "parallel", "timeout", "race", "select", "map", "map_error", "flat_map", "read_async", "readbytes_async", "write_async", "lines_async", "csv_async", "jsonparse_async":
				return true
			}
		}
		if exprContainsControl(n.callee) {
			return true
		}
		for _, arg := range n.args {
			if exprContainsControl(arg) {
				return true
			}
		}
		return false
	case *memberExpr:
		return exprContainsControl(n.object)
	case *unaryExpr:
		return exprContainsControl(n.expr)
	case *binaryExpr:
		return exprContainsControl(n.left) || exprContainsControl(n.right)
	case *listExpr:
		for _, item := range n.items {
			if exprContainsControl(item) {
				return true
			}
		}
		return false
	case *matchExpr:
		if exprContainsControl(n.subject) {
			return true
		}
		for _, arm := range n.arms {
			if exprContainsControl(arm.body) {
				return true
			}
		}
		return false
	case *tryExpr:
		return true
	case *propagateExpr:
		return true
	default:
		return false
	}
}
