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
	op    vmOp
	dst   int
	a     int
	b     int
	tok   tokenKind
	name  string
	text  string
	value Value
	args  []int
	body  *vmChunk
	alt   *vmChunk
	arms  []vmMatchArm
}

type vmMatchArm struct {
	pattern Pattern
	body    *vmChunk
}

type vmCompiler struct {
	chunk   *vmChunk
	next    int
	emitted bool
}

func newVMCompiler(sourceName string) *vmCompiler {
	return &vmCompiler{chunk: &vmChunk{sourceName: sourceName, result: -1}}
}

func (c *vmCompiler) alloc() int {
	reg := c.next
	c.next++
	if reg > c.chunk.maxReg {
		c.chunk.maxReg = reg
	}
	return reg
}

func (c *vmCompiler) emit(instr vmInstr) {
	c.chunk.code = append(c.chunk.code, instr)
	c.emitted = true
}

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
	case letStmt:
		reg, err := c.compileExpr(st.Expr)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
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
		c.emit(vmInstr{op: vmOpCall, dst: dst, a: callee, args: args})
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
	case *blockExpr:
		return c.compileBlockExpr(n)
	case *matchExpr:
		subject, err := c.compileExpr(n.subject)
		if err != nil {
			return 0, err
		}
		arms := make([]vmMatchArm, len(n.arms))
		for i, arm := range n.arms {
			body, err := c.compileExprChunk(arm.body)
			if err != nil {
				return 0, err
			}
			arms[i] = vmMatchArm{pattern: arm.pattern, body: body}
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpMatch, dst: dst, a: subject, arms: arms})
		return dst, nil
	default:
		return 0, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

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

func (f *vmFrame) bind(name string, v Value) {
	if f.locals == nil {
		f.locals = make(map[string]Value)
	}
	f.locals[name] = v
}

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
			if fn, ok := r.rt.Builtins[instr.name]; ok {
				regs[instr.dst] = functionValueOf(instr.name, fn)
				continue
			}
			return nullValue(), fmt.Errorf("undefined name %q", instr.name)
		case vmOpBind:
			r.frame.bind(instr.name, regs[instr.a])
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
			v, err := vmBinary(instr.tok, regs[instr.a], regs[instr.b])
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpCall:
			callee := regs[instr.a]
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
					return nullValue(), err
				}
				regs[instr.dst] = out
				continue
			}
			if callee.fn.session == nil || callee.fn.body == nil {
				return nullValue(), fmt.Errorf("function %s is missing executable body", callee.fn.name)
			}
			if len(args) != len(callee.fn.params) {
				return nullValue(), fmt.Errorf("function %s expects %d arguments", callee.fn.name, len(callee.fn.params))
			}
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
			out, err := callee.fn.session.eval(callee.fn.body, callScope)
			if sig, ok := asFlow(err); ok {
				if sig.kind == flowReturn {
					regs[instr.dst] = sig.value
					continue
				}
				return nullValue(), flowError(sig.kind, "function")
			}
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = out
		case vmOpList:
			items := make([]Value, len(instr.args))
			for i, reg := range instr.args {
				items[i] = regs[reg]
			}
			regs[instr.dst] = listValue(items)
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
				v, err := r.runChildWithBindings(arm.body, state.bindings)
				if err != nil {
					return nullValue(), err
				}
				regs[instr.dst] = v
				matched = true
				break
			}
			if !matched {
				return nullValue(), fmt.Errorf("no match for %s", subject.Kind())
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
	return child.run(chunk)
}

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
	return child.run(chunk)
}

func vmBinary(op tokenKind, left, right Value) (Value, error) {
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
		return numberValue(ln + rn), nil
	case tokenMinus:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return numberValue(ln - rn), nil
	case tokenStar:
		ln, err := left.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		rn, err := right.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return numberValue(ln * rn), nil
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
		return numberValue(ln / rn), nil
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
		return numberValue(math.Mod(ln, rn)), nil
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

func (r *Runtime) RunVM(ctx context.Context, prog *Program) ([]Value, error) {
	sess := NewSession(r, ctx, prog.SourceName)
	expanded, err := sess.ExpandProgram(prog)
	if err != nil {
		return nil, err
	}
	if containsControlFlow(expanded) {
		return sess.ExecProgram(expanded, false, nil)
	}
	compiler := newVMCompiler(prog.SourceName)
	chunk, err := compiler.compileProgram(expanded)
	if err != nil {
		return nil, err
	}
	runner := &vmRunner{
		rt:     r,
		ctx:    ctx,
		source: prog.SourceName,
		stack:  nil,
		frame:  &vmFrame{},
	}
	if _, err := runner.run(chunk); err != nil {
		return nil, err
	}
	return runner.outputs, nil
}

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
	default:
		return false
	}
}
