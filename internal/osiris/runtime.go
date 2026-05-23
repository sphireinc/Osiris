package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math"
	"math/big"
	"strings"
	"sync"
	"unicode"
)

type Builtin func(args []Value) (Value, error)

type Runtime struct {
	Builtins   map[string]Builtin
	Constants  map[string]Value
	moduleMu   sync.Mutex
	modules    map[string]Value
	registryMu sync.Mutex
	registries map[string]*moduleRegistry
}

// NewRuntime constructs and returns a new the time with the default internal state already wired together so callers do not need to assemble caches or dependencies by hand.. It returns *Runtime so callers can continue with the next stage of the pipeline without re-deriving the same information.
func NewRuntime() *Runtime {
	r := &Runtime{
		Builtins:   make(map[string]Builtin),
		Constants:  make(map[string]Value),
		modules:    make(map[string]Value),
		registries: make(map[string]*moduleRegistry),
	}
	registerBuiltins(r.Builtins)
	registerConstants(r.Constants)
	return r
}

// Run run on Runtime the requested work. It keeps the Runtime subsystem self-contained by handling the requested work-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (r *Runtime) Run(ctx context.Context, prog *Program) ([]Value, error) {
	return NewSession(r, ctx, prog.SourceName).ExecProgram(prog, false, nil)
}

type binding struct {
	expr Expr
	env  *scope
}

type scope struct {
	parent *scope
	items  map[string]binding
}

// lookup handles on scope the lookup. It keeps the scope subsystem self-contained by handling the lookup-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *scope) lookup(name string) (binding, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if b, ok := cur.items[name]; ok {
			return b, true
		}
	}
	return binding{}, false
}

type fpKey struct {
	expr  Expr
	scope *scope
}

type promise struct {
	done chan struct{}
	val  Value
	err  error
}

type evaluator struct {
	ctx     context.Context
	rt      *Runtime
	session *Session
	builtin map[string]Builtin
	scope   *scope
	sched   deterministicScheduler

	mu      sync.Mutex
	cache   map[string]Value
	pending map[string]*promise
	fpMu    sync.Mutex
	fpMemo  map[fpKey]string
	fpBusy  map[fpKey]struct{}

	traceMu     sync.Mutex
	callStack   []string
	tracePeak   []string
	traceActive bool
}

// eval handles on evaluator the eval. It keeps the evaluator subsystem self-contained by handling the eval-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) eval(expr Expr, scope *scope) (Value, error) {
	if ev.session != nil {
		expanded, err := ev.session.expandExpr(expr, scope)
		if err != nil {
			return nullValue(), err
		}
		expr = expanded
	}
	if m, ok := expr.(*matchExpr); ok {
		return evalMatch(ev, m, scope)
	}
	key, err := ev.fingerprint(expr, scope)
	if err != nil {
		return nullValue(), err
	}
	return ev.evalWithKey(expr, scope, key)
}

// evalWithKey handles on evaluator the eval with key. It keeps the evaluator subsystem self-contained by handling the eval with key-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) evalWithKey(expr Expr, scope *scope, key string) (Value, error) {
	if err := ev.ctx.Err(); err != nil {
		return nullValue(), err
	}
	ev.mu.Lock()
	if v, ok := ev.cache[key]; ok {
		ev.mu.Unlock()
		return v, nil
	}
	if p, ok := ev.pending[key]; ok {
		ev.mu.Unlock()
		<-p.done
		return p.val, p.err
	}
	p := &promise{done: make(chan struct{})}
	ev.pending[key] = p
	ev.mu.Unlock()

	v, err := computeExpr(ev, expr, scope, key)

	ev.mu.Lock()
	if err == nil {
		ev.cache[key] = v
	}
	delete(ev.pending, key)
	p.val, p.err = v, err
	close(p.done)
	ev.mu.Unlock()
	return v, err
}

// fingerprint handles on evaluator the fingerprint. It keeps the evaluator subsystem self-contained by handling the fingerprint-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) fingerprint(expr Expr, env *scope) (string, error) {
	key := fpKey{expr: expr, scope: env}
	ev.fpMu.Lock()
	if v, ok := ev.fpMemo[key]; ok {
		ev.fpMu.Unlock()
		return v, nil
	}
	if ev.fpBusy == nil {
		ev.fpBusy = make(map[fpKey]struct{})
	}
	if _, busy := ev.fpBusy[key]; busy {
		ev.fpMu.Unlock()
		return hashParts("cycle", fmt.Sprintf("%T", expr)), nil
	}
	ev.fpBusy[key] = struct{}{}
	ev.fpMu.Unlock()
	defer func() {
		ev.fpMu.Lock()
		delete(ev.fpBusy, key)
		ev.fpMu.Unlock()
	}()

	var (
		out string
		err error
	)
	switch n := expr.(type) {
	case *literalExpr:
		out = hashParts("lit", n.value.Kind(), n.value.String())
	case *identExpr:
		b, ok := env.lookup(n.name)
		if !ok {
			if v, exists := ev.rt.Constants[n.name]; exists {
				return hashParts("const", n.name, v.String()), nil
			}
			if _, exists := ev.builtin[n.name]; exists {
				return hashParts("builtin", n.name), nil
			}
			return "", fmt.Errorf("undefined name %q", n.name)
		}
		out, err = ev.fingerprint(b.expr, b.env)
	case *memberExpr:
		obj, ferr := ev.fingerprint(n.object, env)
		if ferr != nil {
			return "", ferr
		}
		out = hashParts("member", obj, n.name)
	case *unaryExpr:
		inner, ferr := ev.fingerprint(n.expr, env)
		if ferr != nil {
			return "", ferr
		}
		out = hashParts("unary", n.op.String(), inner)
	case *binaryExpr:
		left, ferr := ev.fingerprint(n.left, env)
		if ferr != nil {
			return "", ferr
		}
		right, ferr := ev.fingerprint(n.right, env)
		if ferr != nil {
			return "", ferr
		}
		out = hashParts("binary", n.op.String(), left, right)
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			parts := make([]string, 0, len(n.args)+2)
			parts = append(parts, "special", name)
			for _, arg := range n.args {
				fp, ferr := ev.fingerprint(arg, env)
				if ferr != nil {
					return "", ferr
				}
				parts = append(parts, fp)
			}
			out = hashParts(parts...)
			break
		}
		callee, ferr := ev.fingerprint(n.callee, env)
		if ferr != nil {
			return "", ferr
		}
		parts := make([]string, 0, len(n.args)+2)
		parts = append(parts, "call", callee)
		for _, arg := range n.args {
			fp, ferr := ev.fingerprint(arg, env)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, fp)
		}
		out = hashParts(parts...)
	case *listExpr:
		parts := make([]string, 0, len(n.items)+1)
		parts = append(parts, "list")
		for _, item := range n.items {
			fp, ferr := ev.fingerprint(item, env)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, fp)
		}
		out = hashParts(parts...)
	case *blockExpr:
		cur := env
		parts := make([]string, 0, len(n.stmts)+2)
		parts = append(parts, "block")
		for _, stmt := range n.stmts {
			parts = append(parts, fingerprintStmt(ev, stmt, cur))
			switch st := stmt.(type) {
			case letStmt:
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Name: {expr: st.Expr, env: cur},
					},
				}
			case importStmt:
				mod, err := ev.rt.loadModule(ev.ctx, st.Path, ev.session.sourceName, nil)
				if err != nil {
					return "", err
				}
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Alias: {expr: &literalExpr{value: mod}, env: cur},
					},
				}
			case funcStmt:
				bodyFp, err := ev.fingerprint(st.Body, cur)
				if err != nil {
					return "", err
				}
				parts = append(parts, hashParts("func", st.Name, strings.Join(st.Params, ","), bodyFp))
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Name: {expr: &literalExpr{value: functionValueOf(st.Name, nil)}, env: cur},
					},
				}
			case whileStmt:
				cond, err := ev.fingerprint(st.Cond, cur)
				if err != nil {
					return "", err
				}
				body, err := ev.fingerprint(st.Body, cur)
				if err != nil {
					return "", err
				}
				parts = append(parts, hashParts("while", cond, body))
			case forStmt:
				iter, err := ev.fingerprint(st.Iter, cur)
				if err != nil {
					return "", err
				}
				body, err := ev.fingerprint(st.Body, cur)
				if err != nil {
					return "", err
				}
				parts = append(parts, hashParts("for", st.Name, iter, body))
			case returnStmt:
				if st.Expr != nil {
					fp, err := ev.fingerprint(st.Expr, cur)
					if err != nil {
						return "", err
					}
					parts = append(parts, hashParts("return", fp))
				} else {
					parts = append(parts, "return")
				}
			case breakStmt:
				parts = append(parts, "break")
			case continueStmt:
				parts = append(parts, "continue")
			case macroStmt:
				return "", fmt.Errorf("macro statements must be expanded before evaluation")
			}
		}
		if n.tail != nil {
			tail, ferr := ev.fingerprint(n.tail, cur)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, tail)
		}
		out = hashParts(parts...)
	case *tryExpr:
		body, err := ev.fingerprint(n.body, env)
		if err != nil {
			return "", err
		}
		handler, err := ev.fingerprint(n.handler, &scope{
			parent: env,
			items: map[string]binding{
				n.catchName: {expr: &literalExpr{value: errorValue("fingerprint")}, env: env},
			},
		})
		if err != nil {
			return "", err
		}
		out = hashParts("try", n.catchName, body, handler)
	default:
		return "", fmt.Errorf("internal error: unknown expression type %T", expr)
	}
	if err != nil {
		return "", err
	}
	ev.fpMu.Lock()
	ev.fpMemo[key] = out
	ev.fpMu.Unlock()
	return out, nil
}

// computeExpr performs the compute expr step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func computeExpr(ev *evaluator, expr Expr, curScope *scope, key string) (Value, error) {
	switch n := expr.(type) {
	case *literalExpr:
		return n.value, nil
	case *identExpr:
		b, ok := curScope.lookup(n.name)
		if !ok {
			if v, exists := ev.rt.Constants[n.name]; exists {
				return v, nil
			}
			if fn, exists := ev.builtin[n.name]; exists {
				return functionValueOf(n.name, fn), nil
			}
			return nullValue(), fmt.Errorf("undefined name %q", n.name)
		}
		return computeExpr(ev, b.expr, b.env, key)
	case *memberExpr:
		obj, err := ev.eval(n.object, curScope)
		if err != nil {
			return nullValue(), err
		}
		return resolveMember(ev.ctx, obj, n.name)
	case *unaryExpr:
		v, err := ev.eval(n.expr, curScope)
		if err != nil {
			return nullValue(), err
		}
		return applyUnary(n.op, v)
	case *binaryExpr:
		return evalBinary(ev, n, curScope)
	case *callExpr:
		return evalCall(ev, n, curScope)
	case *listExpr:
		items, err := evalMany(ev, n.items, curScope)
		if err != nil {
			return nullValue(), err
		}
		return listValue(items), nil
	case *blockExpr:
		v, _, err := ev.session.evalBlockExpr(n, curScope, ev.session.sourceName, nil)
		return v, err
	case *tryExpr:
		v, err := ev.eval(n.body, curScope)
		if err == nil {
			return v, nil
		}
		if sig, ok := asFlow(err); ok {
			return nullValue(), flowError(sig.kind, "try")
		}
		catchScope := &scope{
			parent: curScope,
			items: map[string]binding{
				n.catchName: {expr: &literalExpr{value: errorValue(err.Error())}, env: curScope},
			},
		}
		return ev.eval(n.handler, catchScope)
	default:
		return nullValue(), fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// evalBlock performs the eval block step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func evalBlock(ev *evaluator, n *blockExpr, env *scope) (Value, error) {
	cur := env
	for _, stmt := range n.stmts {
		switch st := stmt.(type) {
		case letStmt:
			cur = &scope{
				parent: cur,
				items: map[string]binding{
					st.Name: {expr: st.Expr, env: cur},
				},
			}
		case importStmt:
			mod, err := ev.rt.loadModule(ev.ctx, st.Path, ev.session.sourceName, nil)
			if err != nil {
				return nullValue(), err
			}
			cur = &scope{
				parent: cur,
				items: map[string]binding{
					st.Alias: {expr: &literalExpr{value: mod}, env: cur},
				},
			}
		case macroStmt:
			return nullValue(), fmt.Errorf("macro statements must be expanded before evaluation")
		default:
			return nullValue(), fmt.Errorf("block expressions only support let and import statements")
		}
	}
	if n.tail == nil {
		return nullValue(), nil
	}
	return ev.eval(n.tail, cur)
}

// fingerprintStmt performs the fingerprint stmt step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func fingerprintStmt(ev *evaluator, stmt Stmt, scope *scope) string {
	switch st := stmt.(type) {
	case packageStmt:
		return hashParts("package", st.Name)
	case exportStmt:
		return hashParts("export", strings.Join(st.Names, ","))
	case letStmt:
		fp, err := ev.fingerprint(st.Expr, scope)
		if err != nil {
			return hashParts("let", st.Name, "<err>")
		}
		return hashParts("let", st.Name, fp)
	case emitStmt:
		fp, err := ev.fingerprint(st.Expr, scope)
		if err != nil {
			return hashParts("emit", "<err>")
		}
		return hashParts("emit", fp)
	case importStmt:
		return hashParts("import", st.Path, st.Alias)
	case macroStmt:
		return hashParts("macro", st.Name, strings.Join(st.Params, ","), "<body>")
	case funcStmt:
		fp, err := ev.fingerprint(st.Body, scope)
		if err != nil {
			return hashParts("func", st.Name, "<err>")
		}
		return hashParts("func", st.Name, strings.Join(st.Params, ","), fp)
	case returnStmt:
		if st.Expr == nil {
			return hashParts("return")
		}
		fp, err := ev.fingerprint(st.Expr, scope)
		if err != nil {
			return hashParts("return", "<err>")
		}
		return hashParts("return", fp)
	case whileStmt:
		cond, err := ev.fingerprint(st.Cond, scope)
		if err != nil {
			return hashParts("while", "<cond>")
		}
		body, err := ev.fingerprint(st.Body, scope)
		if err != nil {
			return hashParts("while", cond, "<body>")
		}
		return hashParts("while", cond, body)
	case forStmt:
		iter, err := ev.fingerprint(st.Iter, scope)
		if err != nil {
			return hashParts("for", st.Name, "<iter>")
		}
		body, err := ev.fingerprint(st.Body, scope)
		if err != nil {
			return hashParts("for", st.Name, iter, "<body>")
		}
		return hashParts("for", st.Name, iter, body)
	case breakStmt:
		return hashParts("break")
	case continueStmt:
		return hashParts("continue")
	default:
		return hashParts("stmt", fmt.Sprintf("%T", stmt))
	}
}

// evalBinary performs the eval binary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func evalBinary(ev *evaluator, n *binaryExpr, scope *scope) (Value, error) {
	switch n.op {
	case tokenAndAnd:
		left, err := ev.eval(n.left, scope)
		if err != nil {
			return nullValue(), err
		}
		if !left.Truthy() {
			return boolValue(false), nil
		}
		right, err := ev.eval(n.right, scope)
		if err != nil {
			return nullValue(), err
		}
		return boolValue(right.Truthy()), nil
	case tokenOrOr:
		left, err := ev.eval(n.left, scope)
		if err != nil {
			return nullValue(), err
		}
		if left.Truthy() {
			return boolValue(true), nil
		}
		right, err := ev.eval(n.right, scope)
		if err != nil {
			return nullValue(), err
		}
		return boolValue(right.Truthy()), nil
	}

	if n.op == tokenAmp || n.op == tokenPipe || n.op == tokenCaret || n.op == tokenShiftLeft || n.op == tokenShiftRight {
		left, err := ev.eval(n.left, scope)
		if err != nil {
			return nullValue(), err
		}
		right, err := ev.eval(n.right, scope)
		if err != nil {
			return nullValue(), err
		}
		return evalBitwiseBinary(n.op, left, right)
	}

	values, err := ev.sched.run(ev.ctx, []evalTask{
		{key: "left", run: func() (Value, error) { return ev.eval(n.left, scope) }},
		{key: "right", run: func() (Value, error) { return ev.eval(n.right, scope) }},
	})
	if err != nil {
		return nullValue(), err
	}
	left, right := values[0], values[1]

	if left.kind == kindTensor || right.kind == kindTensor {
		return tensorBinaryValue(ev.ctx, ev.sched, n.op, left, right)
	}
	if left.kind == kindQuantity || right.kind == kindQuantity {
		return quantityBinary(n.op, left, right)
	}
	if left.kind == kindInterval || right.kind == kindInterval {
		return intervalBinary(n.op, left, right)
	}
	if left.kind == kindModInt || right.kind == kindModInt {
		return modIntBinary(n.op, left, right)
	}
	if left.kind == kindFixed || right.kind == kindFixed {
		return fixedBinary(n.op, left, right)
	}
	if left.kind == kindBigInt || left.kind == kindBigFloat || left.kind == kindInt || right.kind == kindBigInt || right.kind == kindBigFloat || right.kind == kindInt {
		return bigBinary(n.op, left, right)
	}

	if n.op != tokenPlus || (left.kind != kindString && right.kind != kindString) {
		if (left.kind == kindDecimal || right.kind == kindDecimal) && left.kind != kindComplex && right.kind != kindComplex {
			switch n.op {
			case tokenPlus, tokenMinus, tokenStar, tokenSlash:
				return decimalBinary(n.op, left, right)
			case tokenEqualEqual:
				return boolValue(left.Equal(right)), nil
			case tokenBangEqual:
				return boolValue(!left.Equal(right)), nil
			case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
				cmp, err := decimalCompare(left, right)
				if err != nil {
					return nullValue(), err
				}
				switch n.op {
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
	}

	if (left.kind == kindComplex || right.kind == kindComplex) && left.kind != kindString && right.kind != kindString {
		switch n.op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash:
			return complexBinary(n.op, left, right)
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

	if left.kind == kindUint && right.kind == kindUint {
		if isWideUintValue(left) || isWideUintValue(right) || left.packed || right.packed {
			switch n.op {
			case tokenPlus, tokenMinus, tokenStar, tokenPercent:
				return wideUnsignedBinary(n.op, left, right)
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
		switch n.op {
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

	switch n.op {
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
	case tokenAmp, tokenPipe, tokenCaret, tokenShiftLeft, tokenShiftRight:
		return evalBitwiseBinary(n.op, left, right)
	case tokenEqualEqual:
		return boolValue(left.Equal(right)), nil
	case tokenBangEqual:
		return boolValue(!left.Equal(right)), nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		cmp, err := left.Compare(right)
		if err != nil {
			return nullValue(), err
		}
		switch n.op {
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
	return nullValue(), fmt.Errorf("unsupported operator %s", n.op)
}

// evalCall performs the eval call step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func evalCall(ev *evaluator, n *callExpr, curScope *scope) (Value, error) {
	if name, ok := specialFormName(n.callee); ok {
		return evalSpecialForm(ev, name, n.args, curScope)
	}

	callee, err := ev.eval(n.callee, curScope)
	if err != nil {
		return nullValue(), err
	}
	if callee.kind == kindFunction && callee.fn != nil {
		args, err := evalMany(ev, n.args, curScope)
		if err != nil {
			return nullValue(), err
		}
		ev.pushCall(callee.fn.name)
		defer ev.popCall()
		if callee.fn.builtin != nil {
			return callee.fn.builtin(args)
		}
		if callee.fn.body == nil || callee.fn.session == nil {
			return nullValue(), fmt.Errorf("function %s is missing executable body", callee.fn.name)
		}
		if len(args) != len(callee.fn.params) {
			return nullValue(), fmt.Errorf("function %s expects %d arguments", callee.fn.name, len(callee.fn.params))
		}
		base := callee.fn.closure
		if base == nil {
			base = curScope
		}
		callScope := &scope{
			parent: base,
			items:  make(map[string]binding, len(callee.fn.params)),
		}
		for i, name := range callee.fn.params {
			callScope.items[name] = binding{expr: &literalExpr{value: args[i]}, env: callScope}
		}
		v, err := callee.fn.session.eval(callee.fn.body, callScope)
		if sig, ok := asFlow(err); ok {
			if sig.kind == flowReturn {
				return sig.value, nil
			}
			return nullValue(), flowError(sig.kind, "function")
		}
		return v, err
	}
	if callee.kind == kindModule {
		return nullValue(), fmt.Errorf("cannot call module %s", callee.String())
	}
	return nullValue(), fmt.Errorf("value %s is not callable", callee.Kind())
}

// beginTraceFrame handles on evaluator the begin trace frame. It keeps the evaluator subsystem self-contained by handling the begin trace frame-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) beginTraceFrame() {
	ev.traceMu.Lock()
	ev.traceActive = true
	ev.tracePeak = nil
	ev.traceMu.Unlock()
}

// endTraceFrame handles on evaluator the end trace frame. It keeps the evaluator subsystem self-contained by handling the end trace frame-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) endTraceFrame() []string {
	ev.traceMu.Lock()
	defer ev.traceMu.Unlock()
	out := append([]string(nil), ev.tracePeak...)
	ev.traceActive = false
	ev.tracePeak = nil
	return out
}

// pushCall handles on evaluator the push call. It keeps the evaluator subsystem self-contained by handling the push call-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) pushCall(name string) {
	ev.traceMu.Lock()
	ev.callStack = append(ev.callStack, name)
	if ev.traceActive && len(ev.callStack) > len(ev.tracePeak) {
		ev.tracePeak = append([]string(nil), ev.callStack...)
	}
	ev.traceMu.Unlock()
}

// popCall handles on evaluator the pop call. It keeps the evaluator subsystem self-contained by handling the pop call-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (ev *evaluator) popCall() {
	ev.traceMu.Lock()
	if len(ev.callStack) > 0 {
		ev.callStack = ev.callStack[:len(ev.callStack)-1]
	}
	ev.traceMu.Unlock()
}

// specialFormName performs the special form name step in the surrounding package and returns string, bool.. It returns string, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func specialFormName(expr Expr) (string, bool) {
	id, ok := expr.(*identExpr)
	if !ok {
		return "", false
	}
	switch id.name {
	case "if", "and", "or":
		return id.name, true
	default:
		return "", false
	}
}

// evalSpecialForm performs the eval special form step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func evalSpecialForm(ev *evaluator, name string, args []Expr, scope *scope) (Value, error) {
	switch name {
	case "if":
		if len(args) != 3 {
			return nullValue(), fmt.Errorf("if expects 3 arguments")
		}
		cond, err := ev.eval(args[0], scope)
		if err != nil {
			return nullValue(), err
		}
		if cond.Truthy() {
			return ev.eval(args[1], scope)
		}
		return ev.eval(args[2], scope)
	case "and":
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("and expects 2 arguments")
		}
		left, err := ev.eval(args[0], scope)
		if err != nil {
			return nullValue(), err
		}
		if !left.Truthy() {
			return boolValue(false), nil
		}
		right, err := ev.eval(args[1], scope)
		if err != nil {
			return nullValue(), err
		}
		return boolValue(right.Truthy()), nil
	case "or":
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("or expects 2 arguments")
		}
		left, err := ev.eval(args[0], scope)
		if err != nil {
			return nullValue(), err
		}
		if left.Truthy() {
			return boolValue(true), nil
		}
		right, err := ev.eval(args[1], scope)
		if err != nil {
			return nullValue(), err
		}
		return boolValue(right.Truthy()), nil
	default:
		return nullValue(), fmt.Errorf("unknown special form %q", name)
	}
}

// resolveMember performs the resolve member step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func resolveMember(ctx context.Context, obj Value, name string) (Value, error) {
	mod, err := obj.AsModule()
	if err != nil {
		return nullValue(), fmt.Errorf("cannot access member %q on %s", name, obj.Kind())
	}
	if mod.lookup == nil {
		return nullValue(), fmt.Errorf("module %s has no exports", mod.name)
	}
	return mod.lookup(ctx, name)
}

// evalMany performs the eval many step in the surrounding package and returns []Value, error.. It returns []Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func evalMany(ev *evaluator, exprs []Expr, scope *scope) ([]Value, error) {
	if len(exprs) == 0 {
		return nil, nil
	}
	tasks := make([]evalTask, len(exprs))
	for i, expr := range exprs {
		i, expr := i, expr
		tasks[i] = evalTask{
			key: fmt.Sprintf("%08d", i),
			run: func() (Value, error) {
				return ev.eval(expr, scope)
			},
		}
	}
	return ev.sched.run(ev.ctx, tasks)
}

// applyUnary performs the apply unary step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func applyUnary(op tokenKind, v Value) (Value, error) {
	switch op {
	case tokenBang:
		return boolValue(!v.Truthy()), nil
	case tokenMinus:
		if v.kind == kindBigInt {
			return bigIntValue(new(big.Int).Neg(v.bi)), nil
		}
		if v.kind == kindInt {
			return intValue(new(big.Int).Neg(v.i), v.ibits), nil
		}
		if v.kind == kindBigFloat {
			return bigFloatValue(new(big.Float).Neg(v.bf)), nil
		}
		if v.kind == kindQuantity {
			return quantityValueOf(new(big.Rat).Neg(v.qty.mag), v.qty.dims), nil
		}
		if v.kind == kindInterval {
			if v.iv == nil {
				return intervalValueOf(newIntervalValue(new(big.Rat), new(big.Rat))), nil
			}
			return intervalValueOf(newIntervalValue(new(big.Rat).Neg(v.iv.hi), new(big.Rat).Neg(v.iv.lo))), nil
		}
		if v.kind == kindModInt {
			if v.mi == nil {
				return modIntValueOf(big.NewInt(1), new(big.Int))
			}
			return modIntValueOf(v.mi.mod, new(big.Int).Neg(v.mi.val))
		}
		if v.kind == kindDecimal {
			r, err := decimalFromValue(v)
			if err != nil {
				return nullValue(), err
			}
			return decimalValue(new(big.Rat).Neg(r)), nil
		}
		if v.kind == kindFixed {
			if v.fx == nil {
				return fixedValueOf(64, 0, new(big.Int))
			}
			return fixedValueOf(v.fx.width, v.fx.frac, new(big.Int).Neg(v.fx.raw))
		}
		if v.kind == kindComplex {
			return complexValue(-v.cx), nil
		}
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return floatValueStyled(-n, floatUnaryBits(v), floatUnaryBFloat(v)), nil
	case tokenTilde:
		return evalBitwiseUnary(v)
	default:
		return nullValue(), fmt.Errorf("unsupported unary operator %s", op)
	}
}

// hashParts performs the hash parts step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// numericBuiltin performs the numeric builtin step in the surrounding package and returns Value, error.. It returns Value and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func numericBuiltin(args []Value, fn func([]float64) (Value, error)) (Value, error) {
	nums := make([]float64, len(args))
	for i, arg := range args {
		n, err := arg.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		nums[i] = n
	}
	return fn(nums)
}

// registerBuiltins performs the register builtins step in the surrounding package and returns no result.. It does not return a value; it communicates success or failure through its side effects or through a separate error path when one exists.
func registerBuiltins(dst map[string]Builtin) {
	dst["hash"] = func(args []Value) (Value, error) {
		return stringValue(hashValue(args)), nil
	}
	dst["mix"] = func(args []Value) (Value, error) {
		h := fnv.New64a()
		for _, arg := range args {
			_, _ = h.Write([]byte(arg.String()))
			_, _ = h.Write([]byte{0})
		}
		return stringValue(fmt.Sprintf("%016x", h.Sum64())), nil
	}
	dst["cat"] = func(args []Value) (Value, error) {
		var b strings.Builder
		for _, arg := range args {
			b.WriteString(textify(arg))
		}
		return stringValue(b.String()), nil
	}
	dst["upper"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("upper expects 1 argument")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		return stringValue(strings.ToUpper(s)), nil
	}
	dst["lower"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("lower expects 1 argument")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		return stringValue(strings.ToLower(s)), nil
	}
	dst["trim"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("trim expects 1 argument")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		return stringValue(strings.TrimSpace(s)), nil
	}
	dst["replace"] = func(args []Value) (Value, error) {
		if len(args) != 3 {
			return nullValue(), fmt.Errorf("replace expects 3 arguments")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		old, err := args[1].AsString()
		if err != nil {
			return nullValue(), err
		}
		newv, err := args[2].AsString()
		if err != nil {
			return nullValue(), err
		}
		return stringValue(strings.ReplaceAll(s, old, newv)), nil
	}
	dst["repeat"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("repeat expects 2 arguments")
		}
		if args[0].kind == kindString {
			s, err := args[0].AsString()
			if err != nil {
				return nullValue(), err
			}
			n, err := args[1].Int()
			if err != nil {
				return nullValue(), err
			}
			if n < 0 {
				return nullValue(), fmt.Errorf("repeat count must be non-negative")
			}
			return stringValue(strings.Repeat(s, n)), nil
		}
		return tensorRepeatValue(args)
	}
	dst["split"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("split expects 2 arguments")
		}
		if args[0].kind == kindString {
			s, err := args[0].AsString()
			if err != nil {
				return nullValue(), err
			}
			sep, err := args[1].AsString()
			if err != nil {
				return nullValue(), err
			}
			parts := strings.Split(s, sep)
			out := make([]Value, len(parts))
			for i := range parts {
				out[i] = stringValue(parts[i])
			}
			return listValue(out), nil
		}
		return tensorSplitValue(args)
	}
	dst["chunks"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("chunks expects 2 arguments")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		n, err := args[1].Int()
		if err != nil {
			return nullValue(), err
		}
		if n <= 0 {
			return nullValue(), fmt.Errorf("chunk size must be positive")
		}
		runes := []rune(s)
		out := make([]Value, 0, (len(runes)+n-1)/n)
		for i := 0; i < len(runes); i += n {
			j := i + n
			if j > len(runes) {
				j = len(runes)
			}
			out = append(out, stringValue(string(runes[i:j])))
		}
		return listValue(out), nil
	}
	dst["join"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("join expects 2 arguments")
		}
		sep, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		items, err := args[1].AsList()
		if err != nil {
			return nullValue(), err
		}
		parts := make([]string, len(items))
		for i := range items {
			parts[i] = textify(items[i])
		}
		return stringValue(strings.Join(parts, sep)), nil
	}
	dst["len"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("len expects 1 argument")
		}
		switch args[0].kind {
		case kindList:
			return numberValue(float64(len(args[0].list))), nil
		case kindTensor:
			return numberValue(float64(len(args[0].data))), nil
		case kindString:
			return numberValue(float64(len([]rune(args[0].str)))), nil
		default:
			return nullValue(), fmt.Errorf("len expects a string, list, or tensor")
		}
	}
	dst["take"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("take expects 2 arguments")
		}
		n, err := args[1].Int()
		if err != nil {
			return nullValue(), err
		}
		if n < 0 {
			return nullValue(), fmt.Errorf("take count must be non-negative")
		}
		switch args[0].kind {
		case kindString:
			runes := []rune(args[0].str)
			if n > len(runes) {
				n = len(runes)
			}
			return stringValue(string(runes[:n])), nil
		case kindList:
			if n > len(args[0].list) {
				n = len(args[0].list)
			}
			return listValue(append([]Value(nil), args[0].list[:n]...)), nil
		case kindTensor:
			if n > len(args[0].data) {
				n = len(args[0].data)
			}
			return newTensorValue([]int{n}, append([]Value(nil), args[0].data[:n]...)), nil
		default:
			return nullValue(), fmt.Errorf("take expects a string, list, or tensor")
		}
	}
	dst["range"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("range expects 1 argument")
		}
		n, err := args[0].Int()
		if err != nil {
			return nullValue(), err
		}
		if n < 0 {
			return nullValue(), fmt.Errorf("range count must be non-negative")
		}
		out := make([]Value, n)
		for i := 0; i < n; i++ {
			out[i] = numberValue(float64(i))
		}
		return listValue(out), nil
	}
	dst["sum"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("sum expects 1 argument")
		}
		items, err := args[0].AsList()
		if err != nil {
			return nullValue(), err
		}
		var total float64
		for _, item := range items {
			n, err := item.AsNumber()
			if err != nil {
				return nullValue(), err
			}
			total += n
		}
		return numberValue(total), nil
	}
	dst["product"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("product expects 1 argument")
		}
		items, err := args[0].AsList()
		if err != nil {
			return nullValue(), err
		}
		total := 1.0
		for _, item := range items {
			n, err := item.AsNumber()
			if err != nil {
				return nullValue(), err
			}
			total *= n
		}
		return numberValue(total), nil
	}
	dst["json"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("json expects 1 argument")
		}
		data, err := args[0].MarshalJSON()
		if err != nil {
			return nullValue(), err
		}
		return stringValue(string(data)), nil
	}
	dst["format"] = func(args []Value) (Value, error) {
		if len(args) == 0 {
			return nullValue(), fmt.Errorf("format expects at least 1 argument")
		}
		verb, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		rendered := make([]any, 0, len(args)-1)
		for _, arg := range args[1:] {
			rendered = append(rendered, arg.toInterface())
		}
		return stringValue(fmt.Sprintf(verb, rendered...)), nil
	}
	dst["slug"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("slug expects 1 argument")
		}
		s, err := args[0].AsString()
		if err != nil {
			return nullValue(), err
		}
		s = strings.ToLower(s)
		var b strings.Builder
		lastDash := false
		for _, r := range s {
			switch {
			case unicode.IsLetter(r) || unicode.IsDigit(r):
				b.WriteRune(r)
				lastDash = false
			case !lastDash:
				b.WriteByte('-')
				lastDash = true
			}
		}
		out := strings.Trim(b.String(), "-")
		return stringValue(out), nil
	}
	dst["noise"] = func(args []Value) (Value, error) {
		h := fnv.New64a()
		for _, arg := range args {
			_, _ = h.Write([]byte(arg.String()))
			_, _ = h.Write([]byte{0})
		}
		n := h.Sum64()
		return stringValue(fmt.Sprintf("%016x", n^((n<<13)|(n>>51)))), nil
	}
	dst["zeros"] = func(args []Value) (Value, error) { return zerosValue(args) }
	dst["ones"] = func(args []Value) (Value, error) { return onesValue(args) }
	dst["full"] = func(args []Value) (Value, error) { return fullValue(args) }
	dst["eye"] = func(args []Value) (Value, error) { return eyeValue(args) }
	dst["identity"] = func(args []Value) (Value, error) { return identityValue(args) }
	dst["arange"] = func(args []Value) (Value, error) { return arangeValue(args) }
	dst["linspace"] = func(args []Value) (Value, error) { return linspaceValue(args) }
	dst["logspace"] = func(args []Value) (Value, error) { return logspaceValue(args) }
	dst["sinh"] = func(args []Value) (Value, error) { return sinhValue(args) }
	dst["cosh"] = func(args []Value) (Value, error) { return coshValue(args) }
	dst["tanh"] = func(args []Value) (Value, error) { return tanhValue(args) }
	dst["asinh"] = func(args []Value) (Value, error) { return asinhValue(args) }
	dst["acosh"] = func(args []Value) (Value, error) { return acoshValue(args) }
	dst["atanh"] = func(args []Value) (Value, error) { return atanhValue(args) }
	dst["expm1"] = func(args []Value) (Value, error) { return expm1Value(args) }
	dst["log1p"] = func(args []Value) (Value, error) { return log1pValue(args) }
	dst["cbrt"] = func(args []Value) (Value, error) { return cbrtValue(args) }
	dst["erf"] = func(args []Value) (Value, error) { return erfValue(args) }
	dst["erfc"] = func(args []Value) (Value, error) { return erfcValue(args) }
	dst["gamma"] = func(args []Value) (Value, error) { return gammaValue(args) }
	dst["lgamma"] = func(args []Value) (Value, error) { return lgammaValue(args) }
	dst["sign"] = func(args []Value) (Value, error) { return signValue(args) }
	dst["step"] = func(args []Value) (Value, error) { return stepValue(args) }
	dst["heaviside"] = func(args []Value) (Value, error) { return heavisideValue(args) }
	dst["relu"] = func(args []Value) (Value, error) { return reluValue(args) }
	dst["sigmoid"] = func(args []Value) (Value, error) { return sigmoidValue(args) }
	dst["softplus"] = func(args []Value) (Value, error) { return softplusValue(args) }
	dst["mean"] = func(args []Value) (Value, error) { return meanValue(args) }
	dst["var"] = func(args []Value) (Value, error) { return varValue(args) }
	dst["std"] = func(args []Value) (Value, error) { return stdValue(args) }
	dst["median"] = func(args []Value) (Value, error) { return medianValue(args) }
	dst["quantile"] = func(args []Value) (Value, error) { return quantileValue(args) }
	dst["percentile"] = func(args []Value) (Value, error) { return percentileValue(args) }
	dst["argmin"] = func(args []Value) (Value, error) { return argminValue(args) }
	dst["argmax"] = func(args []Value) (Value, error) { return argmaxValue(args) }
	dst["any"] = func(args []Value) (Value, error) { return anyValue(args) }
	dst["all"] = func(args []Value) (Value, error) { return allValue(args) }
	dst["cumsum"] = func(args []Value) (Value, error) { return cumsumValue(args) }
	dst["cumprod"] = func(args []Value) (Value, error) { return cumprodValue(args) }
	dst["u8"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }
	dst["u16"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 16) }
	dst["u32"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 32) }
	dst["u64"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }
	dst["u128"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 128) }
	dst["u256"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 256) }
	dst["u512"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 512) }
	dst["u1024"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 1024) }
	dst["uint"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 64) }
	dst["byte"] = func(args []Value) (Value, error) { return castUnsignedWidthValue(args, 8) }
	dst["uf8"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 8, false) }
	dst["uf16"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 16, false) }
	dst["uf32"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 32, false) }
	dst["ufnum"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 64, true) }
	dst["uf64"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 64, false) }
	dst["uf128"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 128, false) }
	dst["uf256"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 256, false) }
	dst["uf512"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 512, false) }
	dst["uf1024"] = func(args []Value) (Value, error) { return castPackedUnsignedWidthValue(args, 1024, false) }
	dst["f32"] = func(args []Value) (Value, error) { return castFloatWidthValue(args, 32) }
	dst["f64"] = func(args []Value) (Value, error) { return castFloatWidthValue(args, 64) }
	dst["bf8"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 8) }
	dst["bf16"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 16) }
	dst["bf32"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 32) }
	dst["bf64"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 64) }
	dst["bf128"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 128) }
	dst["bf256"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 256) }
	dst["bf512"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 512) }
	dst["bf1024"] = func(args []Value) (Value, error) { return castBFloatWidthValue(args, 1024) }
	dst["i8"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 8) }
	dst["i16"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 16) }
	dst["i32"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 32) }
	dst["i64"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 64) }
	dst["i128"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 128) }
	dst["i256"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 256) }
	dst["i512"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 512) }
	dst["i1024"] = func(args []Value) (Value, error) { return castSignedWidthValue(args, 1024) }
	dst["complex"] = func(args []Value) (Value, error) { return complexValueFromArgs(args) }
	dst["real"] = func(args []Value) (Value, error) { return realValue(args) }
	dst["imag"] = func(args []Value) (Value, error) { return imagValue(args) }
	dst["magnitude"] = func(args []Value) (Value, error) { return magnitudeValue(args) }
	dst["abs"] = func(args []Value) (Value, error) { return magnitudeValue(args) }
	dst["exp"] = func(args []Value) (Value, error) { return expValue(args) }
	dst["log"] = func(args []Value) (Value, error) { return logValue(args) }
	dst["log10"] = func(args []Value) (Value, error) { return log10Value(args) }
	dst["log2"] = func(args []Value) (Value, error) { return log2Value(args) }
	dst["pow"] = func(args []Value) (Value, error) { return powValue(args) }
	dst["expt"] = func(args []Value) (Value, error) { return powValue(args) }
	dst["sin"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "sin", math.Sin, false) }
	dst["cos"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "cos", math.Cos, false) }
	dst["tan"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "tan", math.Tan, false) }
	dst["asin"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "asin", math.Asin, true) }
	dst["acos"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "acos", math.Acos, true) }
	dst["atan"] = func(args []Value) (Value, error) { return mathUnaryValue(args, "atan", math.Atan, false) }
	dst["hypot"] = func(args []Value) (Value, error) { return hypotValue(args) }
	dst["norm"] = func(args []Value) (Value, error) { return normValue(args) }
	dst["vector"] = func(args []Value) (Value, error) { return vectorValueFromArgs(args) }
	dst["vec"] = func(args []Value) (Value, error) { return vectorValueFromArgs(args) }
	dst["clone"] = func(args []Value) (Value, error) { return tensorCloneValue(args) }
	dst["view"] = func(args []Value) (Value, error) { return tensorViewValue(args) }
	dst["tensor"] = func(args []Value) (Value, error) { return tensorValueFromArgs(args) }
	dst["matrix"] = func(args []Value) (Value, error) { return matrixValueFromArgs(args) }
	dst["shape"] = func(args []Value) (Value, error) { return tensorShapeValue(args) }
	dst["flatten"] = func(args []Value) (Value, error) { return tensorFlattenValue(args) }
	dst["squeeze"] = func(args []Value) (Value, error) { return tensorSqueezeValue(args) }
	dst["expand_dims"] = func(args []Value) (Value, error) { return tensorExpandDimsValue(args) }
	dst["concat"] = func(args []Value) (Value, error) { return tensorConcatValue(args) }
	dst["stack"] = func(args []Value) (Value, error) { return tensorStackValue(args) }
	dst["tile"] = func(args []Value) (Value, error) { return tensorTileValue(args) }
	dst["broadcast_to"] = func(args []Value) (Value, error) { return tensorBroadcastToValue(args) }
	dst["where"] = func(args []Value) (Value, error) { return tensorWhereValue(args) }
	dst["clip"] = func(args []Value) (Value, error) { return tensorClipValue(args) }
	dst["gather"] = func(args []Value) (Value, error) { return tensorGatherValue(args) }
	dst["scatter"] = func(args []Value) (Value, error) { return tensorScatterValue(args) }
	dst["pad"] = func(args []Value) (Value, error) { return tensorPadValue(args) }
	dst["transpose"] = func(args []Value) (Value, error) { return tensorTransposeValue(args) }
	dst["dot"] = func(args []Value) (Value, error) { return tensorDotValue(args) }
	dst["matmul"] = func(args []Value) (Value, error) { return tensorMatMulValue(args) }
	dst["outer"] = func(args []Value) (Value, error) { return outerValue(args) }
	dst["trace"] = func(args []Value) (Value, error) { return traceValue(args) }
	dst["det"] = func(args []Value) (Value, error) { return detValue(args) }
	dst["solve"] = func(args []Value) (Value, error) { return solveValue(args) }
	dst["inv"] = func(args []Value) (Value, error) { return invValue(args) }
	dst["qr"] = func(args []Value) (Value, error) { return qrValue(args) }
	dst["cholesky"] = func(args []Value) (Value, error) { return choleskyValue(args) }
	dst["eig"] = func(args []Value) (Value, error) { return eigValue(args) }
	dst["eigh"] = func(args []Value) (Value, error) { return eighValue(args) }
	dst["svd"] = func(args []Value) (Value, error) { return svdValue(args) }
	dst["reshape"] = func(args []Value) (Value, error) { return tensorReshapeValue(args) }
	dst["dec"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("dec expects 1 argument")
		}
		switch args[0].kind {
		case kindDecimal:
			return decimalValue(args[0].dec), nil
		case kindString:
			return decimalValueFromString(args[0].str)
		default:
			rat, err := decimalFromValue(args[0])
			if err != nil {
				return nullValue(), err
			}
			return decimalValue(rat), nil
		}
	}
	dst["decimal"] = func(args []Value) (Value, error) { return dst["dec"](args) }
	dst["rat"] = func(args []Value) (Value, error) { return dst["dec"](args) }
	dst["rational"] = func(args []Value) (Value, error) { return dst["dec"](args) }
	dst["interval"] = func(args []Value) (Value, error) { return intervalValueFromArgs(args) }
	dst["modint"] = func(args []Value) (Value, error) { return modIntValueFromArgs(args) }
	dst["fixed"] = func(args []Value) (Value, error) { return fixedValueFromArgs(args) }
	dst["bigint"] = func(args []Value) (Value, error) { return bigIntValueFromArgs(args) }
	dst["bigfloat"] = func(args []Value) (Value, error) { return bigFloatValueFromArgs(args) }
	dst["band"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("band expects at least 2 arguments")
		}
		acc := args[0]
		var err error
		for _, arg := range args[1:] {
			acc, err = evalBitwiseBinary(tokenAmp, acc, arg)
			if err != nil {
				return nullValue(), err
			}
		}
		return acc, nil
	}
	dst["bor"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("bor expects at least 2 arguments")
		}
		acc := args[0]
		var err error
		for _, arg := range args[1:] {
			acc, err = evalBitwiseBinary(tokenPipe, acc, arg)
			if err != nil {
				return nullValue(), err
			}
		}
		return acc, nil
	}
	dst["bxor"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("bxor expects at least 2 arguments")
		}
		acc := args[0]
		var err error
		for _, arg := range args[1:] {
			acc, err = evalBitwiseBinary(tokenCaret, acc, arg)
			if err != nil {
				return nullValue(), err
			}
		}
		return acc, nil
	}
	dst["bnot"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("bnot expects 1 argument")
		}
		return evalBitwiseUnary(args[0])
	}
	dst["shl"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("shl expects 2 arguments")
		}
		return evalBitwiseBinary(tokenShiftLeft, args[0], args[1])
	}
	dst["shr"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("shr expects 2 arguments")
		}
		return evalBitwiseBinary(tokenShiftRight, args[0], args[1])
	}
	dst["rotl"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("rotl expects 2 arguments")
		}
		return rotlValue(args[0], args[1])
	}
	dst["rotr"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("rotr expects 2 arguments")
		}
		return rotrValue(args[0], args[1])
	}
	dst["hex"] = func(args []Value) (Value, error) { return hexValue(args) }
	dst["bin"] = func(args []Value) (Value, error) { return binValue(args) }
	dst["bytes"] = func(args []Value) (Value, error) { return bytesValue(args) }
	dst["pack"] = func(args []Value) (Value, error) { return packValue(args) }
	dst["unpack"] = func(args []Value) (Value, error) { return unpackValue(args) }
	dst["at"] = func(args []Value) (Value, error) { return atValue(args) }
	dst["setat"] = func(args []Value) (Value, error) { return setAtValue(args) }
	dst["encode"] = func(args []Value) (Value, error) { return encodeValue(args) }
	dst["decode"] = func(args []Value) (Value, error) { return decodeValue(args) }
	dst["read"] = func(args []Value) (Value, error) { return readValue(args) }
	dst["readbytes"] = func(args []Value) (Value, error) { return readBytesValue(args) }
	dst["write"] = func(args []Value) (Value, error) { return writeValue(args) }
	dst["lines"] = func(args []Value) (Value, error) { return linesValue(args) }
	dst["csv"] = func(args []Value) (Value, error) { return csvValue(args) }
	dst["jsonparse"] = func(args []Value) (Value, error) { return jsonParseValue(args) }
}

// hashValue performs the hash value step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func hashValue(args []Value) string {
	h := sha256.New()
	for _, arg := range args {
		_, _ = h.Write([]byte(arg.String()))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
