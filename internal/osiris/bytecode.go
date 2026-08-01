package osiris

import (
	"context"
	"fmt"
	"math"
	"os"
	"strings"
	"sync"
	"time"
)

type vmOp uint8

const (
	vmOpConst vmOp = iota
	vmOpLoadName
	vmOpBind
	vmOpAssign
	vmOpResultBind
	vmOpTypeAssert
	vmOpUnary
	vmOpBinary
	vmOpCall
	vmOpList
	vmOpMember
	vmOpImport
	vmOpEmit
	vmOpDefer
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
	vmOpFunc
	vmOpReturn
	vmOpWhile
	vmOpFor
	vmOpBreak
	vmOpContinue
	vmOpSpawn
	vmOpAwait
	vmOpCancel
	vmOpTimeout
	vmOpParallel
	vmOpRace
	vmOpSelect
	vmOpResultMap
	vmOpAsyncBuiltin
	vmOpTry
	vmOpPropagate
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
	op           vmOp
	dst          int
	a            int
	b            int
	tok          tokenKind
	name         string
	text         string
	value        Value
	mutable      bool
	args         []int
	fieldNames   []string
	names        []string
	params       []string
	paramTypes   []string
	typeParams   []typeParam
	returnType   string
	receiver     string
	receiverType string
	iter         int
	chunks       []*vmChunk
	body         *vmChunk
	alt          *vmChunk
	arms         []vmMatchArm
	span         sourceSpan
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
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name, mutable: false})
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
			c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name, mutable: st.Mutable})
		}
		return nil
	case funcStmt:
		bodyCompiler := newVMCompiler(c.chunk.sourceName)
		bodyCompiler.chunk.packageName = c.chunk.packageName
		if body, ok := st.Body.(*blockExpr); ok {
			for _, bodyStmt := range body.stmts {
				if err := bodyCompiler.compileStmt(bodyStmt); err != nil {
					return err
				}
			}
			if body.tail != nil {
				reg, err := bodyCompiler.compileExpr(body.tail)
				if err != nil {
					return err
				}
				bodyCompiler.emit(vmInstr{op: vmOpReturn, a: reg})
			}
		} else if st.Body != nil {
			reg, err := bodyCompiler.compileExpr(st.Body)
			if err != nil {
				return err
			}
			bodyCompiler.emit(vmInstr{op: vmOpReturn, a: reg})
		}
		reg := c.alloc()
		c.emit(vmInstr{op: vmOpFunc, dst: reg, name: st.Name, body: bodyCompiler.chunk,
			params: append([]string(nil), st.Params...), paramTypes: append([]string(nil), st.ParamTypes...),
			typeParams: append([]typeParam(nil), st.TypeParams...), returnType: st.ReturnType,
			receiver: st.Receiver, receiverType: st.ReceiverType})
		c.emit(vmInstr{op: vmOpBind, a: reg, name: st.Name})
		return nil
	case implStmt:
		for _, method := range st.Methods {
			if err := c.compileStmt(method); err != nil {
				return err
			}
		}
		return nil
	case traitStmt:
		return nil
	case returnStmt:
		if st.Expr == nil {
			c.emit(vmInstr{op: vmOpReturn, a: -1})
			return nil
		}
		reg, err := c.compileExpr(st.Expr)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpReturn, a: reg})
		return nil
	case whileStmt:
		cond, err := c.compileExprChunk(st.Cond)
		if err != nil {
			return err
		}
		body, err := c.compileExprChunk(st.Body)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpWhile, body: cond, alt: body, span: st.Span})
		return nil
	case forStmt:
		iter, err := c.compileExpr(st.Iter)
		if err != nil {
			return err
		}
		body, err := c.compileExprChunk(st.Body)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpFor, a: iter, name: st.Name, body: body, span: st.Span})
		return nil
	case breakStmt:
		c.emit(vmInstr{op: vmOpBreak})
		return nil
	case continueStmt:
		c.emit(vmInstr{op: vmOpContinue})
		return nil
	case assignStmt:
		reg, err := c.compileExpr(st.Expr)
		if err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpAssign, a: reg, name: st.Name})
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
	case deferStmt:
		body, ok := st.Body.(*blockExpr)
		if !ok {
			return fmt.Errorf("defer requires a block expression")
		}
		child := newVMCompiler(c.chunk.sourceName)
		if _, err := child.compileExpr(body); err != nil {
			return err
		}
		c.emit(vmInstr{op: vmOpDefer, body: child.chunk})
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
			default:
				return c.compileConcurrentCall(name, n)
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
	case *tryExpr:
		body, err := c.compileExprChunk(n.body)
		if err != nil {
			return 0, err
		}
		handler, err := c.compileExprChunk(n.handler)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpTry, dst: dst, body: body, alt: handler, name: n.catchName, span: n.Span})
		return dst, nil
	case *propagateExpr:
		body, err := c.compileExprChunk(n.expr)
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpPropagate, dst: dst, body: body, span: n.Span})
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
	sub, err := c.compileBlockChunk(n)
	if err != nil {
		return 0, err
	}
	dst := c.alloc()
	c.emit(vmInstr{op: vmOpBlock, dst: dst, body: sub})
	return dst, nil
}

func (c *vmCompiler) compileBlockChunk(n *blockExpr) (*vmChunk, error) {
	sub := newVMCompiler(c.chunk.sourceName)
	sub.chunk.packageName = c.chunk.packageName
	sub.chunk.exports = append([]string(nil), c.chunk.exports...)
	for _, stmt := range n.stmts {
		if err := sub.compileStmt(stmt); err != nil {
			return nil, err
		}
	}
	var result int = -1
	if n.tail != nil {
		reg, err := sub.compileExpr(n.tail)
		if err != nil {
			return nil, err
		}
		result = reg
	}
	sub.chunk.result = result
	return sub.chunk, nil
}

// compileExprChunk operates on vmCompiler values and keeps the
// vmcompiler-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *vmChunk and an error. Callers should only trust the value when
// the error is nil.
func (c *vmCompiler) compileExprChunk(expr Expr) (*vmChunk, error) {
	if block, ok := expr.(*blockExpr); ok {
		return c.compileBlockChunk(block)
	}
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

func (c *vmCompiler) compileConcurrentCall(name string, n *callExpr) (int, error) {
	if name == "spawn" || name == "await" || name == "cancel" {
		if len(n.args) != 1 {
			return 0, fmt.Errorf("%s expects 1 argument", name)
		}
		body, err := c.compileExprChunk(n.args[0])
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		op := vmOpSpawn
		if name == "await" {
			op = vmOpAwait
		} else if name == "cancel" {
			op = vmOpCancel
		}
		c.emit(vmInstr{op: op, dst: dst, body: body, span: n.Span})
		return dst, nil
	}
	if name == "timeout" {
		if len(n.args) != 2 {
			return 0, fmt.Errorf("timeout expects 2 arguments")
		}
		duration, err := c.compileExprChunk(n.args[0])
		if err != nil {
			return 0, err
		}
		body, err := c.compileExprChunk(n.args[1])
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpTimeout, dst: dst, body: duration, alt: body, span: n.Span})
		return dst, nil
	}
	if name == "parallel" || name == "race" || name == "select" {
		if len(n.args) == 0 {
			return 0, fmt.Errorf("%s expects at least 1 argument", name)
		}
		chunks := make([]*vmChunk, len(n.args))
		for i, arg := range n.args {
			chunk, err := c.compileExprChunk(arg)
			if err != nil {
				return 0, err
			}
			chunks[i] = chunk
		}
		dst := c.alloc()
		op := vmOpParallel
		if name == "race" {
			op = vmOpRace
		} else if name == "select" {
			op = vmOpSelect
		}
		c.emit(vmInstr{op: op, dst: dst, chunks: chunks, span: n.Span})
		return dst, nil
	}
	if name == "map" || name == "map_error" || name == "flat_map" {
		if len(n.args) != 2 {
			return 0, fmt.Errorf("%s expects result and function", name)
		}
		input, err := c.compileExprChunk(n.args[0])
		if err != nil {
			return 0, err
		}
		callback, err := c.compileExprChunk(n.args[1])
		if err != nil {
			return 0, err
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpResultMap, dst: dst, text: name, body: input, alt: callback, span: n.Span})
		return dst, nil
	}
	if strings.HasSuffix(name, "_async") {
		chunks := make([]*vmChunk, len(n.args))
		for i, arg := range n.args {
			chunk, err := c.compileExprChunk(arg)
			if err != nil {
				return 0, err
			}
			chunks[i] = chunk
		}
		dst := c.alloc()
		c.emit(vmInstr{op: vmOpAsyncBuiltin, dst: dst, text: name, chunks: chunks, span: n.Span})
		return dst, nil
	}
	return 0, fmt.Errorf("unsupported VM special form %q", name)
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
	parent  *vmFrame
	locals  map[string]Value
	mutable map[string]bool
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

func (f *vmFrame) assign(name string, v Value) bool {
	for cur := f; cur != nil; cur = cur.parent {
		if _, ok := cur.locals[name]; ok {
			if !cur.mutable[name] {
				return false
			}
			cur.locals[name] = v
			return true
		}
	}
	return false
}

// bind operates on vmFrame values and keeps the vmframe-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (f *vmFrame) bind(name string, v Value, mutable bool) {
	if f.locals == nil {
		f.locals = make(map[string]Value)
	}
	if f.mutable == nil {
		f.mutable = make(map[string]bool)
	}
	f.locals[name] = v
	f.mutable[name] = mutable
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
		child.mutable = make(map[string]bool, len(bindings))
		for k, v := range bindings {
			child.locals[k] = v
		}
	}
	return child
}

func snapshotVMFrame(frame *vmFrame) *vmFrame {
	return snapshotVMFrameWithMemo(frame, make(map[*vmFrame]*vmFrame))
}

// snapshotVMFrameWithMemo copies the mutable frame graph at an ownership
// boundary. Function values need special handling: their VM closure is a
// frame pointer, so copying only the Value would leave a spawned task sharing
// the parent's mutable locals through a captured function. The memo also
// preserves recursive closures without recursing forever.
func snapshotVMFrameWithMemo(frame *vmFrame, memo map[*vmFrame]*vmFrame) *vmFrame {
	if frame == nil {
		return &vmFrame{}
	}
	if copyFrame, ok := memo[frame]; ok {
		return copyFrame
	}
	copyFrame := &vmFrame{}
	memo[frame] = copyFrame
	copyFrame.parent = snapshotVMFrameWithMemo(frame.parent, memo)
	if len(frame.locals) > 0 {
		copyFrame.locals = make(map[string]Value, len(frame.locals))
		for name, value := range frame.locals {
			copyFrame.locals[name] = snapshotVMValue(value, memo)
		}
	}
	if len(frame.mutable) > 0 {
		copyFrame.mutable = make(map[string]bool, len(frame.mutable))
		for name, mutable := range frame.mutable {
			copyFrame.mutable[name] = mutable
		}
	}
	return copyFrame
}

func snapshotVMValue(value Value, memo map[*vmFrame]*vmFrame) Value {
	if value.kind != kindFunction || value.fn == nil || value.fn.vmClosure == nil {
		return value
	}
	copyFunction := *value.fn
	copyFunction.typeParams = append([]typeParam(nil), value.fn.typeParams...)
	copyFunction.paramTypes = append([]string(nil), value.fn.paramTypes...)
	copyFunction.params = append([]string(nil), value.fn.params...)
	copyFunction.vmClosure = snapshotVMFrameWithMemo(value.fn.vmClosure, memo)
	if value.fn.boundReceiver != nil {
		receiver := snapshotVMValue(*value.fn.boundReceiver, memo)
		copyFunction.boundReceiver = &receiver
	}
	copy := value
	copy.fn = &copyFunction
	return copy
}

type vmRunner struct {
	rt            *Runtime
	ctx           context.Context
	source        string
	stack         []string
	frame         *vmFrame
	outputs       []Value
	constRegs     []Value
	cleanups      []*vmChunk
	cleanupActive bool
	emitAllowed   bool
}

type vmReturnSignal struct{ value Value }

func (s *vmReturnSignal) Error() string { return "vm function returned" }

type vmBreakSignal struct{}

func (s *vmBreakSignal) Error() string { return "vm loop break" }

type vmContinueSignal struct{}

func (s *vmContinueSignal) Error() string { return "vm loop continue" }

// run operates on vmRunner values and keeps the vmrunner-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (r *vmRunner) run(chunk *vmChunk) (value Value, err error) {
	defer func() {
		if cleanupErr := r.runCleanups(); cleanupErr != nil {
			value = nullValue()
			err = combineCleanupErrors(err, cleanupErr)
		}
	}()
	regs := make([]Value, chunk.maxReg+1)
	for _, instr := range chunk.code {
		if err := r.ctx.Err(); err != nil && !r.cleanupActive {
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
			// An immutable binding introduces a new lexical scope node. Function
			// declarations remain in the current node so their closure can see
			// the self-binding for recursion; subsequent `let` bindings cannot
			// retroactively change the scope captured by an earlier closure.
			if !instr.mutable && regs[instr.a].kind != kindFunction {
				r.frame = r.frame.child(nil)
			}
			r.frame.bind(instr.name, regs[instr.a], instr.mutable)
		case vmOpAssign:
			if !r.frame.assign(instr.name, regs[instr.a]) {
				return nullValue(), fmt.Errorf("cannot assign to immutable or undefined name %q", instr.name)
			}
		case vmOpTupleBind:
			value := regs[instr.a]
			if value.kind != kindTuple || len(value.tuple) != len(instr.names) {
				return nullValue(), fmt.Errorf("tuple destructuring requires %d values", len(instr.names))
			}
			for i, name := range instr.names {
				r.frame.bind(name, value.tuple[i], false)
			}
		case vmOpResultBind:
			value, err := unwrapBinding(regs[instr.a])
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = value
		case vmOpTypeAssert:
			// Trait/interface assignments are proven by the shared checker and
			// represented at runtime by the concrete receiver value. The VM does
			// not need to manufacture a second nominal wrapper for that proof.
			if regs[instr.a].kind == kindStruct && !bindingTypeMatches(instr.text, regs[instr.a]) {
				regs[instr.dst] = regs[instr.a]
				continue
			}
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
			if callee.fn.boundReceiver != nil {
				args = append([]Value{*callee.fn.boundReceiver}, args...)
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
			if callee.fn.vmBody != nil {
				checkedArgs, err := checkFunctionArguments(callee.fn, args)
				if err != nil {
					return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, err.Error()), r.source, instr.span)
				}
				parent := callee.fn.vmClosure
				if parent == nil {
					parent = r.frame
				}
				child := &vmRunner{rt: r.rt, ctx: r.ctx, source: r.source, stack: r.stack,
					frame: &vmFrame{parent: parent}, cleanupActive: r.cleanupActive, emitAllowed: r.emitAllowed}
				for i, name := range callee.fn.params {
					child.frame.bind(name, checkedArgs[i], false)
				}
				out, err := child.run(callee.fn.vmBody)
				r.outputs = append(r.outputs, child.outputs...)
				if signal, ok := err.(*vmReturnSignal); ok {
					out, err = signal.value, nil
				}
				if err != nil {
					return nullValue(), err
				}
				out, err = checkFunctionResult(callee.fn, out)
				if err != nil {
					return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, err.Error()), r.source, instr.span)
				}
				regs[instr.dst] = out
				continue
			}
			return nullValue(), fmt.Errorf("function %s is not available to the VM", callee.fn.name)
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
			v, err := resolveMember(r.ctx, obj, instr.name, r.rt)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = v
		case vmOpImport:
			mod, err := r.rt.loadModuleVM(r.ctx, instr.text, r.source, r.stack)
			if err != nil {
				return nullValue(), err
			}
			regs[instr.dst] = mod
		case vmOpEmit:
			if !r.emitAllowed {
				return nullValue(), concurrentEmitError()
			}
			r.outputs = append(r.outputs, regs[instr.a])
		case vmOpDefer:
			if instr.body == nil {
				return nullValue(), fmt.Errorf("defer has no cleanup body")
			}
			r.cleanups = append(r.cleanups, instr.body)
		case vmOpFunc:
			function := typedGenericUserFunctionValue(instr.name, instr.typeParams, instr.params, instr.paramTypes, instr.returnType, nil, nil, nil)
			function.fn.vmBody = instr.body
			function.fn.vmClosure = r.frame
			function.fn.receiver = instr.receiver
			function.fn.receiverType = instr.receiverType
			if instr.receiverType != "" {
				r.rt.registerMethod(instr.receiverType, instr.name, function)
			}
			regs[instr.dst] = function
		case vmOpReturn:
			if instr.a < 0 || instr.a >= len(regs) {
				return nullValue(), &vmReturnSignal{value: nullValue()}
			}
			return nullValue(), &vmReturnSignal{value: regs[instr.a]}
		case vmOpBreak:
			return nullValue(), &vmBreakSignal{}
		case vmOpContinue:
			return nullValue(), &vmContinueSignal{}
		case vmOpSpawn:
			if instr.body == nil {
				return nullValue(), fmt.Errorf("spawn has no body")
			}
			regs[instr.dst] = r.rt.spawnVMTask(r.ctx, r.source, instr.body, snapshotVMFrame(r.frame))
		case vmOpAwait:
			value, awaitErr := r.runChild(instr.body)
			if awaitErr != nil {
				return nullValue(), awaitErr
			}
			task, taskErr := value.AsTask()
			if taskErr != nil {
				return nullValue(), taskErr
			}
			out, taskErr := task.await(r.ctx, &r.outputs)
			if taskErr != nil {
				return nullValue(), taskErr
			}
			regs[instr.dst] = out
		case vmOpCancel:
			value, cancelErr := r.runChild(instr.body)
			if cancelErr != nil {
				return nullValue(), cancelErr
			}
			task, taskErr := value.AsTask()
			if taskErr != nil {
				return nullValue(), taskErr
			}
			regs[instr.dst] = boolValue(task.cancelTask())
		case vmOpTimeout:
			duration, durationErr := r.runChild(instr.body)
			if durationErr != nil {
				return nullValue(), durationErr
			}
			ms, durationErr := duration.Int()
			if durationErr != nil || ms < 0 {
				return nullValue(), fmt.Errorf("timeout expects non-negative integer milliseconds")
			}
			childCtx, cancel := context.WithTimeout(r.ctx, time.Duration(ms)*time.Millisecond)
			child := &vmRunner{rt: r.rt, ctx: childCtx, source: r.source, stack: r.stack,
				frame: r.frame.child(nil), emitAllowed: false}
			out, bodyErr := child.run(instr.alt)
			cancel()
			r.outputs = append(r.outputs, child.outputs...)
			if signal, ok := bodyErr.(*vmReturnSignal); ok {
				out, bodyErr = signal.value, nil
			}
			if bodyErr != nil {
				if childCtx.Err() == context.DeadlineExceeded {
					return nullValue(), fmt.Errorf("timeout after %dms", ms)
				}
				return nullValue(), bodyErr
			}
			regs[instr.dst] = out
		case vmOpParallel, vmOpRace, vmOpSelect:
			out, branchErr := r.runVMConcurrent(instr.op, instr.chunks)
			if branchErr != nil {
				return nullValue(), branchErr
			}
			regs[instr.dst] = out
		case vmOpResultMap:
			out, mapErr := r.runVMResultMap(instr.text, instr.body, instr.alt)
			if mapErr != nil {
				return nullValue(), mapErr
			}
			regs[instr.dst] = out
		case vmOpAsyncBuiltin:
			out, asyncErr := r.runVMAsyncBuiltin(instr.text, instr.chunks)
			if asyncErr != nil {
				return nullValue(), asyncErr
			}
			regs[instr.dst] = out
		case vmOpTry:
			out, bodyErr := r.runChild(instr.body)
			if bodyErr == nil {
				regs[instr.dst] = out
				continue
			}
			if _, flow := bodyErr.(*vmReturnSignal); flow {
				return nullValue(), bodyErr
			}
			if _, flow := bodyErr.(*vmBreakSignal); flow {
				return nullValue(), bodyErr
			}
			if _, flow := bodyErr.(*vmContinueSignal); flow {
				return nullValue(), bodyErr
			}
			errorValue := errorValueFromGoError(normalizeError(bodyErr, r.source))
			out, handlerErr := r.runChildWithBindings(instr.alt, map[string]Value{instr.name: errorValue})
			if handlerErr != nil {
				return nullValue(), handlerErr
			}
			regs[instr.dst] = out
		case vmOpPropagate:
			out, bodyErr := r.runChild(instr.body)
			if bodyErr != nil {
				return nullValue(), bodyErr
			}
			switch out.kind {
			case kindResult:
				if out.res == nil || out.res.ok {
					regs[instr.dst] = out
					continue
				}
				return nullValue(), &vmReturnSignal{value: errorValueFromRecord(out.res.err)}
			case kindOption:
				if out.opt != nil && out.opt.present {
					regs[instr.dst] = out.opt.val
					continue
				}
				return nullValue(), &vmReturnSignal{value: errorValueFromRecord(newOsirisError(ErrorKindNotFound, "option is empty"))}
			case kindError:
				if record := out.errorRecord(); record != nil {
					return nullValue(), &vmReturnSignal{value: errorValueFromRecord(record)}
				}
				return nullValue(), &vmReturnSignal{value: errorValue(out.errMsg)}
			default:
				regs[instr.dst] = out
			}
		case vmOpWhile:
			loopFrame := r.frame
			carryNames := vmLoopCarryNames(r.frame)
		whileLoop:
			for {
				conditionRunner := &vmRunner{rt: r.rt, ctx: r.ctx, source: r.source, stack: r.stack,
					frame: loopFrame, cleanupActive: r.cleanupActive, emitAllowed: r.emitAllowed}
				condition, conditionErr := conditionRunner.run(instr.body)
				r.outputs = append(r.outputs, conditionRunner.outputs...)
				if conditionErr != nil {
					return nullValue(), conditionErr
				}
				if !condition.Truthy() {
					break
				}
				iterationFrame := loopFrame.child(nil)
				bodyRunner := &vmRunner{rt: r.rt, ctx: r.ctx, source: r.source, stack: r.stack,
					frame: iterationFrame, cleanupActive: r.cleanupActive, emitAllowed: r.emitAllowed}
				_, bodyErr := bodyRunner.run(instr.alt)
				r.outputs = append(r.outputs, bodyRunner.outputs...)
				loopFrame = bodyRunner.frame
				vmPropagateLoopCarries(loopFrame, r.frame, carryNames)
				if bodyErr != nil {
					switch bodyErr.(type) {
					case *vmBreakSignal:
						break whileLoop
					case *vmContinueSignal:
						continue
					default:
						return nullValue(), bodyErr
					}
				}
			}
		case vmOpFor:
			value := regs[instr.a]
			cursor, cursorErr := newIterationCursor(NewSession(r.rt, r.ctx, r.source), value)
			if cursorErr != nil {
				return nullValue(), cursorErr
			}
			loopFrame := r.frame.child(nil)
			carryNames := vmLoopCarryNames(r.frame)
			delete(carryNames, instr.name)
		forLoop:
			for {
				item, done, nextErr := cursor.next(r.ctx)
				if nextErr != nil {
					return nullValue(), nextErr
				}
				if done {
					break
				}
				iterationFrame := loopFrame.child(nil)
				iterationFrame.bind(instr.name, item, false)
				child := &vmRunner{rt: r.rt, ctx: r.ctx, source: r.source, stack: r.stack,
					frame: iterationFrame, cleanupActive: r.cleanupActive, emitAllowed: r.emitAllowed}
				_, bodyErr := child.run(instr.body)
				r.outputs = append(r.outputs, child.outputs...)
				loopFrame = child.frame
				vmPropagateLoopCarries(loopFrame, r.frame, carryNames)
				if bodyErr != nil {
					switch bodyErr.(type) {
					case *vmBreakSignal:
						break forLoop
					case *vmContinueSignal:
						continue
					default:
						return nullValue(), bodyErr
					}
				}
			}
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
	if err := r.ctx.Err(); err != nil && !r.cleanupActive {
		return nullValue(), err
	}
	if chunk.result >= 0 && chunk.result < len(regs) {
		return regs[chunk.result], nil
	}
	return nullValue(), nil
}

func vmLoopCarryNames(frame *vmFrame) map[string]struct{} {
	carryNames := make(map[string]struct{})
	for current := frame; current != nil; current = current.parent {
		for name := range current.locals {
			carryNames[name] = struct{}{}
		}
	}
	return carryNames
}

func vmPropagateLoopCarries(loopFrame, target *vmFrame, names map[string]struct{}) {
	for name := range names {
		updated, ok := loopFrame.lookup(name)
		if !ok {
			continue
		}
		for current := target; current != nil; current = current.parent {
			if _, exists := current.locals[name]; exists {
				current.locals[name] = updated
				break
			}
		}
	}
}

// loadModuleVM evaluates a source module through the same VM compiler and
// runner as the entry program. Stdlib modules remain runtime-provided values;
// source modules never re-enter Session.eval merely because they were reached
// through an import.
func (rt *Runtime) loadModuleVM(ctx context.Context, ref, base string, stack []string) (Value, error) {
	ref, selector := splitModuleRef(ref)
	if strings.HasPrefix(ref, "std/") {
		if selector != "" {
			ref += "@" + selector
		}
		return rt.loadModule(ctx, ref, base, stack)
	}
	path, _, err := resolveModulePath(ref, base)
	if err != nil {
		return nullValue(), err
	}
	rt.moduleMu.Lock()
	if cached, ok := rt.modules[path]; ok {
		rt.moduleMu.Unlock()
		return cached, nil
	}
	rt.moduleMu.Unlock()
	for _, seen := range stack {
		if seen == path {
			return nullValue(), importCycleError(stack, path)
		}
	}
	source, err := os.ReadFile(path)
	if err != nil {
		return nullValue(), err
	}
	program, err := Parse(path, string(source))
	if err != nil {
		return nullValue(), err
	}
	session := NewSession(rt, ctx, path)
	expanded, err := session.ExpandProgram(program)
	if err != nil {
		return nullValue(), err
	}
	if _, err := rt.CheckProgram(ctx, expanded); err != nil {
		return nullValue(), err
	}
	compiler := newVMCompiler(path)
	chunk, err := compiler.compileProgram(expanded)
	if err != nil {
		return nullValue(), err
	}
	frame := &vmFrame{}
	runner := &vmRunner{rt: rt, ctx: ctx, source: path, stack: append(append([]string(nil), stack...), path), frame: frame, emitAllowed: false}
	if _, err := runner.run(chunk); err != nil {
		return nullValue(), err
	}
	if chunk.packageName == "" || len(chunk.exports) == 0 {
		return nullValue(), fmt.Errorf("module %s exported nothing", path)
	}
	exports := make(map[string]Value, len(chunk.exports))
	for _, name := range chunk.exports {
		value, ok := frame.lookup(name)
		if !ok {
			return nullValue(), fmt.Errorf("module %s exports undefined name %q", chunk.packageName, name)
		}
		exports[name] = value
	}
	module := moduleValueOf(chunk.packageName, packageVersion(expanded), func(_ context.Context, name string) (Value, error) {
		if value, ok := exports[name]; ok {
			return value, nil
		}
		return nullValue(), fmt.Errorf("module %s has no member %q", chunk.packageName, name)
	}, exports)
	rt.moduleMu.Lock()
	rt.modules[path] = module
	rt.moduleMu.Unlock()
	return module, nil
}

func packageVersion(program *Program) string {
	if packageStmt, ok := firstPackage(program); ok {
		return packageStmt.Version
	}
	return ""
}

func (r *vmRunner) runVMChunk(ctx context.Context, chunk *vmChunk, frame *vmFrame, emitAllowed bool) (Value, error, []Value) {
	child := &vmRunner{rt: r.rt, ctx: ctx, source: r.source, stack: r.stack, frame: frame, emitAllowed: emitAllowed}
	value, err := child.run(chunk)
	if signal, ok := err.(*vmReturnSignal); ok {
		value, err = signal.value, nil
	}
	return value, err, child.outputs
}

func (r *vmRunner) runVMConcurrent(op vmOp, chunks []*vmChunk) (Value, error) {
	if len(chunks) == 0 {
		if op == vmOpParallel {
			return listValue(nil), nil
		}
		return nullValue(), fmt.Errorf("concurrent form expects at least one branch")
	}
	type result struct {
		index   int
		value   Value
		err     error
		outputs []Value
	}
	runBranch := func(ctx context.Context, index int) result {
		value, err, outputs := r.runVMChunk(ctx, chunks[index], snapshotVMFrame(r.frame), false)
		return result{index: index, value: value, err: err, outputs: outputs}
	}
	if r.rt != nil && r.rt.taskDebug {
		branches := make([]result, 0, len(chunks))
		for i := range chunks {
			branch := runBranch(r.ctx, i)
			if branch.err != nil {
				return nullValue(), branch.err
			}
			r.outputs = append(r.outputs, branch.outputs...)
			branches = append(branches, branch)
			if op == vmOpRace {
				return branch.value, nil
			}
			if op == vmOpSelect {
				return listValue([]Value{uintValue(uint64(i), 64), branch.value}), nil
			}
		}
		values := make([]Value, len(chunks))
		for i, branch := range branches {
			values[i] = branch.value
		}
		return listValue(values), nil
	}
	groupCtx, cancel := context.WithCancel(r.ctx)
	defer cancel()
	results := make(chan result, len(chunks))
	var wait sync.WaitGroup
	for i := range chunks {
		i := i
		wait.Add(1)
		go func() {
			defer wait.Done()
			branch := runBranch(groupCtx, i)
			select {
			case results <- branch:
			case <-groupCtx.Done():
			}
		}()
	}
	go func() {
		wait.Wait()
		close(results)
	}()
	if op == vmOpRace || op == vmOpSelect {
		for branch := range results {
			if branch.err != nil {
				cancel()
				return nullValue(), branch.err
			}
			cancel()
			r.outputs = append(r.outputs, branch.outputs...)
			if op == vmOpSelect {
				return listValue([]Value{uintValue(uint64(branch.index), 64), branch.value}), nil
			}
			return branch.value, nil
		}
		return nullValue(), r.ctx.Err()
	}
	branches := make([]result, len(chunks))
	for branch := range results {
		if branch.err != nil {
			cancel()
			return nullValue(), branch.err
		}
		branches[branch.index] = branch
	}
	values := make([]Value, len(branches))
	for i, branch := range branches {
		values[i] = branch.value
		r.outputs = append(r.outputs, branch.outputs...)
	}
	return listValue(values), nil
}

func (r *vmRunner) runVMCallable(function Value, args []Value) (Value, error) {
	if function.kind != kindFunction || function.fn == nil {
		return nullValue(), fmt.Errorf("value %s is not callable", function.Kind())
	}
	fn := function.fn
	if fn.boundReceiver != nil {
		args = append([]Value{*fn.boundReceiver}, args...)
	}
	checked, err := checkFunctionArguments(fn, args)
	if err != nil {
		return nullValue(), err
	}
	if fn.builtin != nil {
		return fn.builtin(checked)
	}
	if fn.contextBuiltin != nil {
		return fn.contextBuiltin(r.ctx, checked)
	}
	if fn.vmBody == nil {
		return nullValue(), fmt.Errorf("function %s is not available to the VM", fn.name)
	}
	parent := fn.vmClosure
	if parent == nil {
		parent = r.frame
	}
	frame := &vmFrame{parent: parent}
	for i, name := range fn.params {
		frame.bind(name, checked[i], false)
	}
	value, err, outputs := r.runVMChunk(r.ctx, fn.vmBody, frame, r.emitAllowed)
	r.outputs = append(r.outputs, outputs...)
	if err != nil {
		return nullValue(), err
	}
	return checkFunctionResult(fn, value)
}

func (r *vmRunner) runVMResultMap(kind string, inputChunk, callbackChunk *vmChunk) (Value, error) {
	input, err := r.runChild(inputChunk)
	if err != nil {
		return nullValue(), err
	}
	callback, err := r.runChild(callbackChunk)
	if err != nil {
		return nullValue(), err
	}
	if input.kind != kindResult || input.res == nil {
		return nullValue(), fmt.Errorf("%s expects a result", kind)
	}
	call := func(value Value) (Value, error) { return r.runVMCallable(callback, []Value{value}) }
	switch kind {
	case "map":
		if !input.res.ok {
			return inheritMustHandle(input, resultErrValue(input.res.err)), nil
		}
		value, err := call(input.res.val)
		if err != nil {
			return nullValue(), err
		}
		return inheritMustHandle(input, resultOkValue(value)), nil
	case "map_error":
		if input.res.ok {
			return inheritMustHandle(input, resultOkValue(input.res.val)), nil
		}
		value, err := call(errorValueFromRecord(input.res.err))
		if err != nil {
			return nullValue(), err
		}
		if record, recordErr := value.AsError(); recordErr == nil && record != nil {
			return inheritMustHandle(input, resultErrValue(record)), nil
		}
		message, messageErr := value.AsString()
		if messageErr != nil {
			return nullValue(), fmt.Errorf("map_error callback must return error or string")
		}
		return inheritMustHandle(input, resultErrValue(newOsirisError(ErrorKindUnknown, message))), nil
	case "flat_map":
		if !input.res.ok {
			return inheritMustHandle(input, resultErrValue(input.res.err)), nil
		}
		value, err := call(input.res.val)
		if err != nil {
			return nullValue(), err
		}
		switch value.kind {
		case kindResult:
			return inheritMustHandle(input, value), nil
		case kindOption:
			if value.opt != nil && value.opt.present {
				return inheritMustHandle(input, resultOkValue(value.opt.val)), nil
			}
			return inheritMustHandle(input, resultErrValue(newOsirisError(ErrorKindNotFound, "option is empty"))), nil
		default:
			return nullValue(), fmt.Errorf("flat_map callback must return result or option")
		}
	}
	return nullValue(), fmt.Errorf("unknown result combinator %q", kind)
}

func (r *vmRunner) runVMAsyncBuiltin(name string, chunks []*vmChunk) (Value, error) {
	values := make([]Value, len(chunks))
	for i, chunk := range chunks {
		value, err := r.runChild(chunk)
		if err != nil {
			return nullValue(), err
		}
		values[i] = value
	}
	var call Builtin
	switch name {
	case "read_async":
		call = readValue
	case "readbytes_async":
		call = readBytesValue
	case "write_async":
		call = writeValue
	case "lines_async":
		call = linesValue
	case "csv_async":
		call = csvValue
	case "jsonparse_async":
		call = jsonParseValue
	default:
		return nullValue(), fmt.Errorf("unknown async builtin %q", name)
	}
	return NewSession(r.rt, r.ctx, r.source).spawnValueTask(func(context.Context) (Value, error, []Value) {
		value, err := call(values)
		return value, err, nil
	}), nil
}

func (r *vmRunner) runCleanups() error {
	previousCleanup := r.cleanupActive
	r.cleanupActive = true
	defer func() { r.cleanupActive = previousCleanup }()
	var cleanupErr error
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		_, err := r.runChild(r.cleanups[i])
		if err != nil {
			cleanupErr = combineCleanupErrors(cleanupErr, err)
		}
	}
	r.cleanups = nil
	return cleanupErr
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
		rt:            r.rt,
		ctx:           r.ctx,
		source:        r.source,
		stack:         r.stack,
		frame:         r.frame.child(nil),
		cleanupActive: r.cleanupActive,
		emitAllowed:   r.emitAllowed,
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
		rt:            r.rt,
		ctx:           r.ctx,
		source:        r.source,
		stack:         r.stack,
		frame:         r.frame.child(bindings),
		cleanupActive: r.cleanupActive,
		emitAllowed:   r.emitAllowed,
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
	compiler := newVMCompiler(prog.SourceName)
	chunk, err := compiler.compileProgram(expanded)
	if err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	runner := &vmRunner{
		rt:          r,
		ctx:         ctx,
		source:      prog.SourceName,
		stack:       nil,
		frame:       &vmFrame{},
		emitAllowed: true,
	}
	if _, err := runner.run(chunk); err != nil {
		return runner.outputs, normalizeError(err, prog.SourceName)
	}
	return runner.outputs, nil
}
