package osiris

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"hash/fnv"
	"math"
	"strings"
	"sync"
	"unicode"
)

type Builtin func(args []Value) (Value, error)

type Runtime struct {
	Builtins   map[string]Builtin
	moduleMu   sync.Mutex
	modules    map[string]Value
	registryMu sync.Mutex
	registries map[string]*moduleRegistry
}

func NewRuntime() *Runtime {
	r := &Runtime{Builtins: make(map[string]Builtin), modules: make(map[string]Value), registries: make(map[string]*moduleRegistry)}
	registerBuiltins(r.Builtins)
	return r
}

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

	traceMu     sync.Mutex
	callStack   []string
	tracePeak   []string
	traceActive bool
}

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

func (ev *evaluator) fingerprint(expr Expr, env *scope) (string, error) {
	key := fpKey{expr: expr, scope: env}
	ev.fpMu.Lock()
	if v, ok := ev.fpMemo[key]; ok {
		ev.fpMu.Unlock()
		return v, nil
	}
	ev.fpMu.Unlock()

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

func computeExpr(ev *evaluator, expr Expr, scope *scope, key string) (Value, error) {
	switch n := expr.(type) {
	case *literalExpr:
		return n.value, nil
	case *identExpr:
		b, ok := scope.lookup(n.name)
		if !ok {
			if fn, exists := ev.builtin[n.name]; exists {
				return functionValueOf(n.name, fn), nil
			}
			return nullValue(), fmt.Errorf("undefined name %q", n.name)
		}
		return computeExpr(ev, b.expr, b.env, key)
	case *memberExpr:
		obj, err := ev.eval(n.object, scope)
		if err != nil {
			return nullValue(), err
		}
		return resolveMember(ev.ctx, obj, n.name)
	case *unaryExpr:
		v, err := ev.eval(n.expr, scope)
		if err != nil {
			return nullValue(), err
		}
		return applyUnary(n.op, v)
	case *binaryExpr:
		return evalBinary(ev, n, scope)
	case *callExpr:
		return evalCall(ev, n, scope)
	case *listExpr:
		items, err := evalMany(ev, n.items, scope)
		if err != nil {
			return nullValue(), err
		}
		return listValue(items), nil
	case *blockExpr:
		return evalBlock(ev, n, scope)
	default:
		return nullValue(), fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

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
	default:
		return hashParts("stmt", fmt.Sprintf("%T", stmt))
	}
}

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

func evalCall(ev *evaluator, n *callExpr, scope *scope) (Value, error) {
	if name, ok := specialFormName(n.callee); ok {
		return evalSpecialForm(ev, name, n.args, scope)
	}

	callee, err := ev.eval(n.callee, scope)
	if err != nil {
		return nullValue(), err
	}
	if callee.kind == kindFunction && callee.fn != nil {
		args, err := evalMany(ev, n.args, scope)
		if err != nil {
			return nullValue(), err
		}
		ev.pushCall(callee.fn.name)
		defer ev.popCall()
		return callee.fn.fn(args)
	}
	if callee.kind == kindModule {
		return nullValue(), fmt.Errorf("cannot call module %s", callee.String())
	}
	return nullValue(), fmt.Errorf("value %s is not callable", callee.Kind())
}

func (ev *evaluator) beginTraceFrame() {
	ev.traceMu.Lock()
	ev.traceActive = true
	ev.tracePeak = nil
	ev.traceMu.Unlock()
}

func (ev *evaluator) endTraceFrame() []string {
	ev.traceMu.Lock()
	defer ev.traceMu.Unlock()
	out := append([]string(nil), ev.tracePeak...)
	ev.traceActive = false
	ev.tracePeak = nil
	return out
}

func (ev *evaluator) pushCall(name string) {
	ev.traceMu.Lock()
	ev.callStack = append(ev.callStack, name)
	if ev.traceActive && len(ev.callStack) > len(ev.tracePeak) {
		ev.tracePeak = append([]string(nil), ev.callStack...)
	}
	ev.traceMu.Unlock()
}

func (ev *evaluator) popCall() {
	ev.traceMu.Lock()
	if len(ev.callStack) > 0 {
		ev.callStack = ev.callStack[:len(ev.callStack)-1]
	}
	ev.traceMu.Unlock()
}

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

func applyUnary(op tokenKind, v Value) (Value, error) {
	switch op {
	case tokenBang:
		return boolValue(!v.Truthy()), nil
	case tokenMinus:
		n, err := v.AsNumber()
		if err != nil {
			return nullValue(), err
		}
		return numberValue(-n), nil
	case tokenTilde:
		return evalBitwiseUnary(v)
	default:
		return nullValue(), fmt.Errorf("unsupported unary operator %s", op)
	}
}

func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

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
	dst["split"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("split expects 2 arguments")
		}
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
		case kindString:
			return numberValue(float64(len([]rune(args[0].str)))), nil
		default:
			return nullValue(), fmt.Errorf("len expects a string or list")
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
		default:
			return nullValue(), fmt.Errorf("take expects a string or list")
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
	dst["u8"] = func(args []Value) (Value, error) { return castWidthValue(args, 8) }
	dst["u16"] = func(args []Value) (Value, error) { return castWidthValue(args, 16) }
	dst["u32"] = func(args []Value) (Value, error) { return castWidthValue(args, 32) }
	dst["u64"] = func(args []Value) (Value, error) { return castWidthValue(args, 64) }
	dst["band"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("band expects at least 2 arguments")
		}
		acc, bitsWidth, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		for _, arg := range args[1:] {
			u, width, err := uintFromValue(arg)
			if err != nil {
				return nullValue(), err
			}
			if width < bitsWidth {
				bitsWidth = width
			}
			acc &= u
		}
		return uintValue(acc, bitsWidth), nil
	}
	dst["bor"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("bor expects at least 2 arguments")
		}
		acc, bitsWidth, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		for _, arg := range args[1:] {
			u, width, err := uintFromValue(arg)
			if err != nil {
				return nullValue(), err
			}
			if width > bitsWidth {
				bitsWidth = width
			}
			acc |= u
		}
		return uintValue(acc, bitsWidth), nil
	}
	dst["bxor"] = func(args []Value) (Value, error) {
		if len(args) < 2 {
			return nullValue(), fmt.Errorf("bxor expects at least 2 arguments")
		}
		acc, bitsWidth, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		for _, arg := range args[1:] {
			u, width, err := uintFromValue(arg)
			if err != nil {
				return nullValue(), err
			}
			if width > bitsWidth {
				bitsWidth = width
			}
			acc ^= u
		}
		return uintValue(acc, bitsWidth), nil
	}
	dst["bnot"] = func(args []Value) (Value, error) {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("bnot expects 1 argument")
		}
		return uintUnary(args[0], func(u uint64, width uint8) uint64 { return ^u & maskForWidth(width) })
	}
	dst["shl"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("shl expects 2 arguments")
		}
		left, width, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		shift, _, err := uintFromValue(args[1])
		if err != nil {
			return nullValue(), err
		}
		return uintValue((left<<uint(shift))&maskForWidth(width), width), nil
	}
	dst["shr"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("shr expects 2 arguments")
		}
		left, width, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		shift, _, err := uintFromValue(args[1])
		if err != nil {
			return nullValue(), err
		}
		return uintValue((left>>uint(shift))&maskForWidth(width), width), nil
	}
	dst["rotl"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("rotl expects 2 arguments")
		}
		left, width, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		shift, _, err := uintFromValue(args[1])
		if err != nil {
			return nullValue(), err
		}
		return uintValue(rotl(left, width, shift), width), nil
	}
	dst["rotr"] = func(args []Value) (Value, error) {
		if len(args) != 2 {
			return nullValue(), fmt.Errorf("rotr expects 2 arguments")
		}
		left, width, err := uintFromValue(args[0])
		if err != nil {
			return nullValue(), err
		}
		shift, _, err := uintFromValue(args[1])
		if err != nil {
			return nullValue(), err
		}
		return uintValue(rotr(left, width, shift), width), nil
	}
	dst["hex"] = func(args []Value) (Value, error) { return hexValue(args) }
	dst["bin"] = func(args []Value) (Value, error) { return binValue(args) }
	dst["bytes"] = func(args []Value) (Value, error) { return bytesValue(args) }
	dst["pack"] = func(args []Value) (Value, error) { return packValue(args) }
	dst["unpack"] = func(args []Value) (Value, error) { return unpackValue(args) }
	dst["encode"] = func(args []Value) (Value, error) { return encodeValue(args) }
	dst["decode"] = func(args []Value) (Value, error) { return decodeValue(args) }
	dst["read"] = func(args []Value) (Value, error) { return readValue(args) }
	dst["readbytes"] = func(args []Value) (Value, error) { return readBytesValue(args) }
	dst["write"] = func(args []Value) (Value, error) { return writeValue(args) }
	dst["lines"] = func(args []Value) (Value, error) { return linesValue(args) }
	dst["csv"] = func(args []Value) (Value, error) { return csvValue(args) }
	dst["jsonparse"] = func(args []Value) (Value, error) { return jsonParseValue(args) }
}

func hashValue(args []Value) string {
	h := sha256.New()
	for _, arg := range args {
		_, _ = h.Write([]byte(arg.String()))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
