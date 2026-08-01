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
	"time"
	"unicode"
)

type Builtin func(args []Value) (Value, error)

type Runtime struct {
	Builtins        map[string]Builtin
	ContextBuiltins map[string]contextCallable
	Constants       map[string]Value
	methodMu        sync.RWMutex
	methods         map[string]map[string]Value
	moduleMu        sync.Mutex
	modules         map[string]Value
	registryMu      sync.Mutex
	registries      map[string]*moduleRegistry
	taskMu          sync.Mutex
	taskSeq         uint64
	traceMu         sync.Mutex
	traceSeq        uint64
	taskTrace       []TaskTraceEvent
	taskTraceOn     bool
	taskDebug       bool
	clockMu         sync.RWMutex
	clock           func() time.Time
	locationMu      sync.RWMutex
	locationLoader  func(string) (*time.Location, error)
}

// NewRuntime constructs a new runtime and wires together the internal caches,
// registries, and default tables that the caller would otherwise have to
// assemble by hand.
//
// It returns *Runtime so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func NewRuntime() *Runtime {
	r := &Runtime{
		Builtins:        make(map[string]Builtin),
		ContextBuiltins: make(map[string]contextCallable),
		Constants:       make(map[string]Value),
		methods:         make(map[string]map[string]Value),
		modules:         make(map[string]Value),
		registries:      make(map[string]*moduleRegistry),
		clock:           time.Now,
		locationLoader:  time.LoadLocation,
	}
	registerBuiltins(r.Builtins)
	registerContextBuiltins(r.ContextBuiltins)
	registerConstants(r.Constants)
	return r
}

func (r *Runtime) registerMethod(receiverType, name string, method Value) {
	if r == nil || receiverType == "" || name == "" {
		return
	}
	r.methodMu.Lock()
	defer r.methodMu.Unlock()
	methods := r.methods[receiverType]
	if methods == nil {
		methods = make(map[string]Value)
		r.methods[receiverType] = methods
	}
	methods[name] = method
}

func (r *Runtime) lookupMethod(receiverType, name string) (Value, bool) {
	if r == nil {
		return Value{}, false
	}
	r.methodMu.RLock()
	method, ok := r.methods[receiverType][name]
	r.methodMu.RUnlock()
	return method, ok
}

// SetClock installs the source used by std/time.now. Passing nil restores the
// host clock. The function is read for each call, so cached modules never
// snapshot a time value.
func (r *Runtime) SetClock(clock func() time.Time) {
	if r == nil {
		return
	}
	if clock == nil {
		clock = time.Now
	}
	r.clockMu.Lock()
	r.clock = clock
	r.clockMu.Unlock()
}

func (r *Runtime) clockNow() time.Time {
	if r == nil {
		return time.Now()
	}
	r.clockMu.RLock()
	clock := r.clock
	r.clockMu.RUnlock()
	if clock == nil {
		return time.Now()
	}
	return clock()
}

// SetTimeLocationLoader installs the IANA database lookup used by
// std/time.load_location. Passing nil restores the embedded/default loader.
// It exists so deterministic hosts and tests can pin or deny named zones.
func (r *Runtime) SetTimeLocationLoader(loader func(string) (*time.Location, error)) {
	if r == nil {
		return
	}
	if loader == nil {
		loader = time.LoadLocation
	}
	r.locationMu.Lock()
	r.locationLoader = loader
	r.locationMu.Unlock()
}

func (r *Runtime) loadTimeLocation(name string) (*time.Location, error) {
	if r == nil {
		return time.LoadLocation(name)
	}
	r.locationMu.RLock()
	loader := r.locationLoader
	r.locationMu.RUnlock()
	if loader == nil {
		loader = time.LoadLocation
	}
	return loader(name)
}

// SetTaskTrace enables or disables task lifecycle tracing on the runtime.
// When tracing is enabled, the runtime records task lifecycle events such as
// spawn, await, cancel, completion, and scheduler dispatch so callers can
// inspect deterministic execution behavior after a run finishes.
func (r *Runtime) SetTaskTrace(enabled bool) {
	if r == nil {
		return
	}
	r.traceMu.Lock()
	r.taskTraceOn = enabled
	if !enabled {
		r.taskTrace = nil
		r.traceSeq = 0
	}
	r.traceMu.Unlock()
}

// SetTaskDebug enables or disables deterministic debug scheduling on the
// runtime. When debug scheduling is enabled, structured parallel execution
// runs in source order on the current goroutine instead of fanning out across
// the worker pool, which makes concurrent control flow easier to inspect in
// tests and diagnostics.
func (r *Runtime) SetTaskDebug(enabled bool) {
	if r == nil {
		return
	}
	r.traceMu.Lock()
	r.taskDebug = enabled
	r.traceMu.Unlock()
}

// TaskTrace returns a copy of the recorded task lifecycle trace. The trace is
// ordered by the runtime's internal event sequence and is safe to read after
// execution has completed.
func (r *Runtime) TaskTrace() []TaskTraceEvent {
	if r == nil {
		return nil
	}
	r.traceMu.Lock()
	defer r.traceMu.Unlock()
	return append([]TaskTraceEvent(nil), r.taskTrace...)
}

// recordTaskTrace appends a single task lifecycle event to the runtime trace
// buffer when task tracing is enabled.
func (r *Runtime) recordTaskTrace(kind, taskID, status, detail string) {
	if r == nil {
		return
	}
	r.traceMu.Lock()
	defer r.traceMu.Unlock()
	if !r.taskTraceOn {
		return
	}
	r.traceSeq++
	r.taskTrace = append(r.taskTrace, TaskTraceEvent{
		Seq:    r.traceSeq,
		Kind:   kind,
		TaskID: taskID,
		Status: status,
		Detail: detail,
	})
}

// Run operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (r *Runtime) Run(ctx context.Context, prog *Program) ([]Value, error) {
	return NewSession(r, ctx, prog.SourceName).ExecProgram(prog, false, nil)
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
	fpDepth map[fpKey]int
	fpBusy  map[fpKey]struct{}

	traceMu     sync.Mutex
	callStack   []string
	tracePeak   []string
	traceActive bool
}

// eval operates on evaluator values and keeps the evaluator-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
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
	if binary, ok := expr.(*binaryExpr); ok && (binary.op == tokenAndAnd || binary.op == tokenOrOr) {
		// Logical expressions must not fingerprint the deferred operand: doing so
		// resolves names before evalBinary can short-circuit them.
		return computeExpr(ev, expr, scope, "")
	}
	if exprHasTaskEffect(ev, expr, scope, nil) {
		return computeExpr(ev, expr, scope, "")
	}
	// Emission is an observable sequencing barrier. Expressions which directly
	// contain emit or call a statically known emitting function must execute on
	// every demand rather than entering the pure-expression memoization cache.
	if _, emits := firstEmitSpan(ev, expr, scope, nil); emits {
		return computeExpr(ev, expr, scope, "")
	}
	if scopeHasMutableCell(scope) {
		return computeExpr(ev, expr, scope, "")
	}
	key, err := ev.fingerprint(expr, scope)
	if err != nil {
		return nullValue(), err
	}
	return ev.evalWithKey(expr, scope, key)
}

func scopeHasMutableCell(s *scope) bool {
	for cur := s; cur != nil; cur = cur.parent {
		for _, b := range cur.items {
			if b.cell != nil && b.mutable {
				return true
			}
		}
	}
	return false
}

// evalWithKey operates on evaluator values and keeps the evaluator-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
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

// fingerprint operates on evaluator values and keeps the evaluator-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func (ev *evaluator) fingerprint(expr Expr, env *scope) (string, error) {
	if fingerprint, _, ok, err := ev.fingerprintSimple(expr, env); ok || err != nil {
		return fingerprint, err
	}
	return ev.fingerprintRecursive(expr, env)
}

func (ev *evaluator) fingerprintRecursive(expr Expr, env *scope) (string, error) {
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
			if _, exists := ev.rt.ContextBuiltins[n.name]; exists {
				return hashParts("context-builtin", n.name), nil
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
	case *tupleExpr:
		parts := []string{"tuple"}
		for _, item := range n.items {
			fp, ferr := ev.fingerprint(item, env)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, fp)
		}
		out = hashParts(parts...)
	case *structExpr:
		parts := []string{"struct", n.TypeName}
		for _, field := range n.Fields {
			fp, ferr := ev.fingerprint(field.Expr, env)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, field.Name, fp)
		}
		out = hashParts(parts...)
	case *structUpdateExpr:
		base, ferr := ev.fingerprint(n.Base, env)
		if ferr != nil {
			return "", ferr
		}
		parts := []string{"struct-update", base}
		for _, field := range n.Fields {
			fp, ferr := ev.fingerprint(field.Expr, env)
			if ferr != nil {
				return "", ferr
			}
			parts = append(parts, field.Name, fp)
		}
		out = hashParts(parts...)
	case *indexExpr:
		object, ferr := ev.fingerprint(n.Object, env)
		if ferr != nil {
			return "", ferr
		}
		index, ferr := ev.fingerprint(n.Index, env)
		if ferr != nil {
			return "", ferr
		}
		out = hashParts("index", object, index)
	case *matchExpr:
		subject, ferr := ev.fingerprint(n.subject, env)
		if ferr != nil {
			return "", ferr
		}
		parts := []string{"match", n.keyword.String(), subject}
		for _, arm := range n.arms {
			armScope := &scope{parent: env, items: make(map[string]binding)}
			for _, name := range patternBindingNames(arm.pattern) {
				marker := stringValue("$pattern:" + name)
				armScope.items[name] = binding{expr: &literalExpr{value: marker}, env: armScope}
			}
			parts = append(parts, formatPattern(arm.pattern))
			if arm.guard != nil {
				guard, err := ev.fingerprint(arm.guard, armScope)
				if err != nil {
					return "", err
				}
				parts = append(parts, guard)
			}
			body, err := ev.fingerprint(arm.body, armScope)
			if err != nil {
				return "", err
			}
			parts = append(parts, body)
		}
		out = hashParts(parts...)
	case *blockExpr:
		cur := env
		functions := &scope{parent: cur, items: make(map[string]binding)}
		for _, stmt := range n.stmts {
			st, ok := stmt.(funcStmt)
			if !ok {
				continue
			}
			value := typedGenericUserFunctionValue(st.Name, st.TypeParams, st.Params, st.ParamTypes, st.ReturnType, st.Body, functions, ev.session)
			functions.items[st.Name] = binding{expr: &literalExpr{value: value}, env: functions}
		}
		if len(functions.items) > 0 {
			cur = functions
		}
		parts := make([]string, 0, len(n.stmts)+2)
		parts = append(parts, "block")
		for _, stmt := range n.stmts {
			if _, isFunction := stmt.(funcStmt); !isFunction {
				parts = append(parts, fingerprintStmt(ev, stmt, cur))
			}
			switch st := stmt.(type) {
			case letStmt:
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Name: {expr: st.Expr, env: cur},
					},
				}
			case assignStmt:
				fp, err := ev.fingerprint(st.Expr, cur)
				if err != nil {
					return "", err
				}
				parts = append(parts, hashParts("assign", st.Name, fp))
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
				fnScope := &scope{parent: cur, items: make(map[string]binding, len(st.Params))}
				for _, param := range st.Params {
					marker := stringValue("$parameter:" + param)
					fnScope.items[param] = binding{expr: &literalExpr{value: marker}, env: fnScope}
				}
				bodyFp, err := ev.fingerprint(st.Body, fnScope)
				if err != nil {
					return "", err
				}
				parts = append(parts, hashParts(
					"func",
					st.Name,
					strings.Join(st.Params, ","),
					strings.Join(st.ParamTypes, ","),
					st.ReturnType,
					bodyFp,
				))
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
			case structStmt:
				parts = append(parts, hashParts("struct", st.Name, formatStmt(st)))
			case typeDeclStmt:
				parts = append(parts, hashParts("type", st.Name, st.Target))
			case newtypeStmt:
				parts = append(parts, hashParts("newtype", st.Name, st.Target))
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
	case *propagateExpr:
		inner, err := ev.fingerprint(n.expr, env)
		if err != nil {
			return "", err
		}
		out = hashParts("propagate", inner)
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

// computeExpr handles the compute expr step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func computeExpr(ev *evaluator, expr Expr, curScope *scope, key string) (Value, error) {
	if ev.shouldEvaluateSimpleIteratively(expr, curScope) {
		return ev.evalSimpleIterative(expr, curScope)
	}
	switch n := expr.(type) {
	case *literalExpr:
		return n.value, nil
	case *identExpr:
		b, ok := curScope.lookup(n.name)
		if !ok {
			if v, exists := ev.rt.Constants[n.name]; exists {
				return v, nil
			}
			if fn, exists := ev.rt.ContextBuiltins[n.name]; exists {
				return contextFunctionValueOf(n.name, fn), nil
			}
			if fn, exists := ev.builtin[n.name]; exists {
				return functionValueOf(n.name, fn), nil
			}
			return nullValue(), fmt.Errorf("undefined name %q", n.name)
		}
		if b.cell != nil {
			return b.cell.value, nil
		}
		return computeExpr(ev, b.expr, b.env, key)
	case *memberExpr:
		obj, err := ev.eval(n.object, curScope)
		if err != nil {
			return nullValue(), err
		}
		return resolveMember(ev.ctx, obj, n.name, ev.rt)
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
	case *tupleExpr:
		items, err := evalMany(ev, n.items, curScope)
		if err != nil {
			return nullValue(), err
		}
		return tupleValue(items), nil
	case *structExpr:
		base, err := ev.eval(&identExpr{name: n.TypeName}, curScope)
		if err != nil {
			return nullValue(), err
		}
		if base.kind != kindStruct || base.structType == nil {
			return nullValue(), fmt.Errorf("%s is not a struct type", n.TypeName)
		}
		values, err := evalStructFields(ev, base.structType, n.Fields, curScope, nil)
		if err != nil {
			return nullValue(), err
		}
		return structInstanceValueOf(base.structType, values), nil
	case *structUpdateExpr:
		base, err := ev.eval(n.Base, curScope)
		if err != nil {
			return nullValue(), err
		}
		if base.kind != kindStruct || base.structType == nil || base.structValue == nil {
			return nullValue(), fmt.Errorf("struct update requires a struct value")
		}
		values := append([]Value(nil), base.structValue.fields...)
		updated, err := evalStructFields(ev, base.structType, n.Fields, curScope, values)
		if err != nil {
			return nullValue(), err
		}
		return structInstanceValueOf(base.structType, updated), nil
	case *indexExpr:
		object, err := ev.eval(n.Object, curScope)
		if err != nil {
			return nullValue(), err
		}
		index, err := ev.eval(n.Index, curScope)
		if err != nil {
			return nullValue(), err
		}
		return atValue([]Value{object, index})
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
		if isFatalError(err) {
			return nullValue(), err
		}
		catchScope := &scope{
			parent: curScope,
			items: map[string]binding{
				n.catchName: {expr: &literalExpr{value: errorValueFromGoError(normalizeError(err, func() string {
					if ev.session != nil {
						return ev.session.sourceName
					}
					return ""
				}()))}, env: curScope},
			},
		}
		return ev.eval(n.handler, catchScope)
	case *propagateExpr:
		v, err := ev.eval(n.expr, curScope)
		if err != nil {
			if _, ok := asFlow(err); ok {
				return nullValue(), err
			}
			rec := errorValueFromGoError(normalizeError(err, func() string {
				if ev.session != nil {
					return ev.session.sourceName
				}
				return ""
			}()))
			if rec.kind == kindError {
				return nullValue(), newFlow(flowReturn, rec)
			}
			return nullValue(), newFlow(flowReturn, errorValueFromRecord(newOsirisError(ErrorKindUnknown, err.Error())))
		}
		switch v.kind {
		case kindResult:
			if v.res == nil || v.res.ok {
				return v, nil
			}
			return nullValue(), newFlow(flowReturn, errorValueFromRecord(v.res.err))
		case kindOption:
			if v.opt == nil || !v.opt.present {
				return nullValue(), newFlow(flowReturn, errorValueFromRecord(newOsirisError(ErrorKindNotFound, "option is empty")))
			}
			return v.opt.val, nil
		case kindError:
			if rec := v.errorRecord(); rec != nil {
				return nullValue(), newFlow(flowReturn, errorValueFromRecord(rec))
			}
			return nullValue(), newFlow(flowReturn, errorValue(v.errMsg))
		default:
			return v, nil
		}
	default:
		return nullValue(), fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// evalBlock handles the eval block step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalBlock(ev *evaluator, n *blockExpr, env *scope) (Value, error) {
	if ev.session != nil {
		value, _, err := ev.session.evalBlockExpr(n, env, ev.session.sourceName, nil)
		return value, err
	}
	cur := env
	for _, stmt := range n.stmts {
		switch st := stmt.(type) {
		case letStmt:
			expr := st.Expr
			if st.Type != "" {
				value, err := ev.eval(expr, cur)
				if err != nil {
					return nullValue(), err
				}
				value, err = checkBindingType(st.Type, value, false)
				if err != nil {
					return nullValue(), fmt.Errorf("let %s: %w", st.Name, err)
				}
				expr = &literalExpr{value: value}
			}
			cur = &scope{
				parent: cur,
				items: map[string]binding{
					st.Name: {expr: expr, env: cur},
				},
			}
		case assignStmt:
			b, ok := cur.lookup(st.Name)
			if !ok || b.cell == nil || !b.mutable {
				return nullValue(), fmt.Errorf("cannot assign to immutable or undefined name %q", st.Name)
			}
			value, err := ev.eval(st.Expr, cur)
			if err != nil {
				return nullValue(), err
			}
			b.cell.value = value
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

// fingerprintStmt handles the fingerprint stmt step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
	case assignStmt:
		fp, err := ev.fingerprint(st.Expr, scope)
		if err != nil {
			return hashParts("assign", st.Name, "<err>")
		}
		return hashParts("assign", st.Name, fp)
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

// evalBinary handles the eval binary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
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
	if left.kind == kindDual || right.kind == kindDual {
		switch n.op {
		case tokenPlus, tokenMinus, tokenStar, tokenSlash:
			return dualBinary(n.op, left, right)
		case tokenEqualEqual:
			return boolValue(left.Equal(right)), nil
		case tokenBangEqual:
			return boolValue(!left.Equal(right)), nil
		default:
			return nullValue(), fmt.Errorf("ordering is not defined for dual numbers")
		}
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

// evalCall handles the eval call step in the surrounding package and returns
// Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalCall(ev *evaluator, n *callExpr, curScope *scope) (Value, error) {
	if name, ok := specialFormName(n.callee); ok {
		return evalSpecialForm(ev, name, n.args, curScope, n.Span)
	}

	callee, err := ev.eval(n.callee, curScope)
	if err != nil {
		return nullValue(), err
	}
	sourceName := ""
	if ev.session != nil {
		sourceName = ev.session.sourceName
	}
	if callee.kind == kindDuration {
		args, err := evalMany(ev, n.args, curScope)
		if err != nil {
			return nullValue(), err
		}
		value, err := callDurationComponent(callee, args)
		return value, structuredErrorAt(normalizeError(err, sourceName), sourceName, n.Span)
	}
	if callee.kind == kindStruct && callee.structType != nil {
		args, err := evalMany(ev, n.args, curScope)
		if err != nil {
			return nullValue(), err
		}
		return constructStruct(callee.structType, args)
	}
	if callee.kind == kindFunction && callee.fn != nil {
		args, err := evalMany(ev, n.args, curScope)
		if err != nil {
			return nullValue(), err
		}
		ev.pushCall(callee.fn.name)
		defer ev.popCall()
		if callee.fn.builtin != nil {
			value, err := callee.fn.builtin(args)
			return value, structuredErrorAt(normalizeError(err, sourceName), sourceName, n.Span)
		}
		if callee.fn.contextBuiltin != nil {
			value, err := callee.fn.contextBuiltin(ev.ctx, args)
			return value, structuredErrorAt(normalizeError(err, sourceName), sourceName, n.Span)
		}
		if callee.fn.body == nil {
			return nullValue(), fmt.Errorf("function %s is missing executable body", callee.fn.name)
		}
		if callee.fn.boundReceiver != nil {
			args = append([]Value{*callee.fn.boundReceiver}, args...)
		}
		args, err = checkFunctionArguments(callee.fn, args)
		if err != nil {
			return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, err.Error()), sourceName, n.Span)
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
		sess := ev.session
		if sess == nil {
			sess = callee.fn.session
		}
		if sess == nil {
			return nullValue(), fmt.Errorf("function %s is missing an execution session", callee.fn.name)
		}
		v, err := sess.eval(callee.fn.body, callScope)
		if sig, ok := asFlow(err); ok {
			if sig.kind == flowReturn {
				value, typeErr := checkFunctionResult(callee.fn, sig.value)
				if typeErr != nil {
					return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, typeErr.Error()), sourceName, n.Span)
				}
				return value, nil
			}
			return nullValue(), flowError(sig.kind, "function")
		}
		if err != nil {
			return v, err
		}
		value, typeErr := checkFunctionResult(callee.fn, v)
		if typeErr != nil {
			return nullValue(), structuredErrorAt(newOsirisError(ErrorKindType, typeErr.Error()), sourceName, n.Span)
		}
		return value, nil
	}
	if callee.kind == kindModule {
		return nullValue(), fmt.Errorf("cannot call module %s", callee.String())
	}
	return nullValue(), fmt.Errorf("value %s is not callable", callee.Kind())
}

func constructStruct(typ *structTypeValue, args []Value) (Value, error) {
	if typ == nil {
		return nullValue(), fmt.Errorf("missing struct type")
	}
	if typ.newtype {
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("%s expects 1 argument", typ.name)
		}
		value := args[0]
		if isBindingTypeName(typ.target) || strings.HasPrefix(typ.target, "func(") {
			if _, err := checkBindingType(typ.target, value, false); err != nil {
				return nullValue(), fmt.Errorf("%s requires %s: %w", typ.name, typ.target, err)
			}
		} else if value.kind != kindStruct || value.structType == nil || value.structType.name != typ.target || value.structValue == nil {
			return nullValue(), fmt.Errorf("%s requires %s, got %s", typ.name, typ.target, value.Kind())
		}
		return structInstanceValueOf(typ, args), nil
	}
	if len(args) != len(typ.fields) {
		return nullValue(), fmt.Errorf("%s expects %d arguments, got %d", typ.name, len(typ.fields), len(args))
	}
	for i, arg := range args {
		if err := requireRuntimeDeclaredType(typ.fields[i].typeName, arg); err != nil {
			return nullValue(), fmt.Errorf("%s field %s: %w", typ.name, typ.fields[i].name, err)
		}
	}
	return structInstanceValueOf(typ, args), nil
}

func requireRuntimeDeclaredType(typeName string, value Value) error {
	if isBindingTypeName(typeName) || strings.HasPrefix(typeName, "func(") {
		if _, err := checkBindingType(typeName, value, false); err != nil {
			return err
		}
		return nil
	}
	if value.kind == kindStruct && value.structType != nil && value.structValue != nil && value.structType.name == typeName {
		return nil
	}
	return fmt.Errorf("requires %s, got %s", typeName, value.Kind())
}

func evalStructFields(ev *evaluator, typ *structTypeValue, fields []structFieldExpr, scope *scope, values []Value) ([]Value, error) {
	if typ == nil || typ.newtype {
		return nil, fmt.Errorf("field initializer requires an ordinary struct")
	}
	if values == nil {
		values = make([]Value, len(typ.fields))
		for i := range values {
			values[i] = nullValue()
		}
	}
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		idx, ok := typ.byName[field.Name]
		if !ok {
			return nil, fmt.Errorf("struct %s has no field %q", typ.name, field.Name)
		}
		if _, exists := seen[field.Name]; exists {
			return nil, fmt.Errorf("struct initializer repeats field %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		value, err := ev.eval(field.Expr, scope)
		if err != nil {
			return nil, err
		}
		if err := requireRuntimeDeclaredType(typ.fields[idx].typeName, value); err != nil {
			return nil, fmt.Errorf("field %s.%s: %w", typ.name, field.Name, err)
		}
		values[idx] = value
	}
	for i, value := range values {
		if value.kind == kindNull {
			return nil, fmt.Errorf("struct %s is missing field %q", typ.name, typ.fields[i].name)
		}
	}
	return values, nil
}

// beginTraceFrame operates on evaluator values and keeps the
// evaluator-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (ev *evaluator) beginTraceFrame() {
	ev.traceMu.Lock()
	ev.traceActive = true
	ev.tracePeak = nil
	ev.traceMu.Unlock()
}

// endTraceFrame operates on evaluator values and keeps the evaluator-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (ev *evaluator) endTraceFrame() []string {
	ev.traceMu.Lock()
	defer ev.traceMu.Unlock()
	out := append([]string(nil), ev.tracePeak...)
	ev.traceActive = false
	ev.tracePeak = nil
	return out
}

// pushCall operates on evaluator values and keeps the evaluator-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (ev *evaluator) pushCall(name string) {
	ev.traceMu.Lock()
	ev.callStack = append(ev.callStack, name)
	if ev.traceActive && len(ev.callStack) > len(ev.tracePeak) {
		ev.tracePeak = append([]string(nil), ev.callStack...)
	}
	ev.traceMu.Unlock()
}

// popCall operates on evaluator values and keeps the evaluator-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (ev *evaluator) popCall() {
	ev.traceMu.Lock()
	if len(ev.callStack) > 0 {
		ev.callStack = ev.callStack[:len(ev.callStack)-1]
	}
	ev.traceMu.Unlock()
}

// specialFormName handles the special form name step in the surrounding
// package and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func specialFormName(expr Expr) (string, bool) {
	id, ok := expr.(*identExpr)
	if !ok {
		return "", false
	}
	switch id.name {
	case "if", "and", "or", "spawn", "await", "cancel":
		return id.name, true
	case "parallel", "timeout", "race", "select":
		return id.name, true
	case "map", "map_error", "flat_map":
		return id.name, true
	case "read_async", "readbytes_async", "write_async", "lines_async", "csv_async", "jsonparse_async":
		return id.name, true
	default:
		return "", false
	}
}

// builtinHasTaskEffect reports whether a builtin name performs observable I/O
// or otherwise depends on mutable runtime state. This keeps memoization from
// eliding effects such as file access and streaming reads or writes.
func builtinHasTaskEffect(name string) bool {
	switch name {
	case "read", "readbytes", "write", "openread", "openwrite", "readline", "readchunk", "writechunk", "transfer", "flush", "close", "tcp_listen", "tcp_accept", "tcp_dial":
		return true
	case "append", "read_limit", "copy_file", "truncate_file", "exists", "file_size", "is_file", "is_dir", "mkdir", "mkdir_all", "read_dir", "rename", "remove", "openappend", "seek", "tell", "truncate":
		return true
	default:
		return false
	}
}

// evalSpecialForm handles the eval special form step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalSpecialForm(ev *evaluator, name string, args []Expr, scope *scope, callSpan sourceSpan) (Value, error) {
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
	case "spawn":
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("spawn expects 1 argument")
		}
		if ev.session == nil {
			return nullValue(), fmt.Errorf("spawn requires an active session")
		}
		if span, ok := firstEmitSpan(ev, args[0], scope, nil); ok {
			return nullValue(), structuredErrorAt(concurrentEmitError(), ev.session.sourceName, chooseSourceSpan(span, callSpan))
		}
		return ev.session.spawnTask(args[0], scope), nil
	case "await":
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("await expects 1 argument")
		}
		taskVal, err := ev.eval(args[0], scope)
		if err != nil {
			return nullValue(), err
		}
		task, err := taskVal.AsTask()
		if err != nil {
			return nullValue(), err
		}
		return task.await(ev.ctx, ev.session.outputs)
	case "cancel":
		if len(args) != 1 {
			return nullValue(), fmt.Errorf("cancel expects 1 argument")
		}
		taskVal, err := ev.eval(args[0], scope)
		if err != nil {
			return nullValue(), err
		}
		task, err := taskVal.AsTask()
		if err != nil {
			return nullValue(), err
		}
		return boolValue(task.cancelTask()), nil
	case "parallel":
		if span, ok := firstEmitSpanIn(ev, args, scope); ok {
			return nullValue(), structuredErrorAt(concurrentEmitError(), ev.session.sourceName, chooseSourceSpan(span, callSpan))
		}
		return evalParallel(ev, args, scope)
	case "timeout":
		if len(args) == 2 {
			if span, ok := firstEmitSpan(ev, args[1], scope, nil); ok {
				return nullValue(), structuredErrorAt(concurrentEmitError(), ev.session.sourceName, chooseSourceSpan(span, callSpan))
			}
		}
		return evalTimeout(ev, args, scope)
	case "race":
		if span, ok := firstEmitSpanIn(ev, args, scope); ok {
			return nullValue(), structuredErrorAt(concurrentEmitError(), ev.session.sourceName, chooseSourceSpan(span, callSpan))
		}
		return evalRace(ev, args, scope, false)
	case "select":
		if span, ok := firstEmitSpanIn(ev, args, scope); ok {
			return nullValue(), structuredErrorAt(concurrentEmitError(), ev.session.sourceName, chooseSourceSpan(span, callSpan))
		}
		return evalRace(ev, args, scope, true)
	case "map":
		return evalResultCombinator(ev, args, scope, "map")
	case "map_error":
		return evalResultCombinator(ev, args, scope, "map_error")
	case "flat_map":
		return evalResultCombinator(ev, args, scope, "flat_map")
	case "read_async":
		return evalAsyncRead(ev, args, scope)
	case "readbytes_async":
		return evalAsyncReadBytes(ev, args, scope)
	case "write_async":
		return evalAsyncWrite(ev, args, scope)
	case "lines_async":
		return evalAsyncLines(ev, args, scope)
	case "csv_async":
		return evalAsyncCSV(ev, args, scope)
	case "jsonparse_async":
		return evalAsyncJSONParse(ev, args, scope)
	default:
		return nullValue(), fmt.Errorf("unknown special form %q", name)
	}
}

func evalResultCombinator(ev *evaluator, args []Expr, scope *scope, kind string) (Value, error) {
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("%s expects result and function", kind)
	}
	input, err := ev.eval(args[0], scope)
	if err != nil {
		return nullValue(), err
	}
	if input.kind != kindResult || input.res == nil {
		return nullValue(), fmt.Errorf("%s expects a result", kind)
	}
	callback, err := ev.eval(args[1], scope)
	if err != nil {
		return nullValue(), err
	}
	call := func(value Value) (Value, error) {
		return evalCall(ev, &callExpr{callee: &literalExpr{value: callback}, args: []Expr{&literalExpr{value: value}}}, scope)
	}
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
		if rec, recErr := value.AsError(); recErr == nil && rec != nil {
			return inheritMustHandle(input, resultErrValue(rec)), nil
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

// evalParallel handles the eval parallel step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalParallel(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	if ev == nil || ev.session == nil {
		return nullValue(), fmt.Errorf("parallel requires an active session")
	}
	if len(args) == 0 {
		return listValue(nil), nil
	}
	groupCtx, cancel := context.WithCancel(ev.ctx)
	defer cancel()
	type branchResult struct {
		val  Value
		err  error
		outs []Value
	}
	branches := make([]branchResult, len(args))
	tasks := make([]evalTask, len(args))
	for i, expr := range args {
		i, expr := i, expr
		tasks[i] = evalTask{
			key: fmt.Sprintf("%08d", i),
			run: func() (Value, error) {
				childCtx, branchCancel := context.WithCancel(groupCtx)
				defer branchCancel()
				child := NewSession(ev.rt, childCtx, ev.session.sourceName)
				child.emitAllowed = false
				child.scope = cloneScope(scope)
				child.packageName = ev.session.packageName
				child.packageVersion = ev.session.packageVersion
				child.exportOrder = append([]string(nil), ev.session.exportOrder...)
				if len(ev.session.exportSet) > 0 {
					child.exportSet = make(map[string]struct{}, len(ev.session.exportSet))
					for k := range ev.session.exportSet {
						child.exportSet[k] = struct{}{}
					}
				}
				child.macros = cloneMacroDefs(ev.session.macros)
				var outputs []Value
				child.outputs = &outputs
				v, err := child.eval(expr, child.scope)
				if sig, ok := asFlow(err); ok {
					if sig.kind == flowReturn {
						v = sig.value
						err = nil
					} else {
						err = flowError(sig.kind, "parallel")
					}
				}
				branches[i] = branchResult{val: v, err: err, outs: append([]Value(nil), outputs...)}
				if err != nil {
					cancel()
					return nullValue(), err
				}
				return v, nil
			},
		}
	}
	values, err := ev.sched.run(groupCtx, tasks)
	if err != nil {
		return nullValue(), err
	}
	out := make([]Value, len(values))
	copy(out, values)
	if ev.session.outputs != nil {
		for i := range branches {
			if len(branches[i].outs) > 0 {
				*ev.session.outputs = append(*ev.session.outputs, branches[i].outs...)
			}
		}
	}
	return listValue(out), nil
}

// evalTimeout handles the eval timeout step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalTimeout(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	if ev == nil || ev.session == nil {
		return nullValue(), fmt.Errorf("timeout requires an active session")
	}
	if len(args) != 2 {
		return nullValue(), fmt.Errorf("timeout expects 2 arguments")
	}
	durVal, err := ev.eval(args[0], scope)
	if err != nil {
		return nullValue(), err
	}
	ms, err := durVal.Int()
	if err != nil {
		return nullValue(), fmt.Errorf("timeout expects integer milliseconds: %w", err)
	}
	if ms < 0 {
		return nullValue(), fmt.Errorf("timeout expects non-negative milliseconds")
	}
	childCtx, cancel := context.WithTimeout(ev.ctx, time.Duration(ms)*time.Millisecond)
	defer cancel()
	child := NewSession(ev.rt, childCtx, ev.session.sourceName)
	child.emitAllowed = false
	child.scope = cloneScope(scope)
	child.packageName = ev.session.packageName
	child.packageVersion = ev.session.packageVersion
	child.exportOrder = append([]string(nil), ev.session.exportOrder...)
	if len(ev.session.exportSet) > 0 {
		child.exportSet = make(map[string]struct{}, len(ev.session.exportSet))
		for k := range ev.session.exportSet {
			child.exportSet[k] = struct{}{}
		}
	}
	child.macros = cloneMacroDefs(ev.session.macros)
	var outputs []Value
	child.outputs = &outputs
	v, err := child.eval(args[1], child.scope)
	if sig, ok := asFlow(err); ok {
		if sig.kind == flowReturn {
			return sig.value, nil
		}
		return nullValue(), flowError(sig.kind, "timeout")
	}
	if err != nil {
		if childCtx.Err() == context.DeadlineExceeded {
			return nullValue(), fmt.Errorf("timeout after %dms", ms)
		}
		return nullValue(), err
	}
	if ev.session.outputs != nil && len(outputs) > 0 {
		*ev.session.outputs = append(*ev.session.outputs, outputs...)
	}
	return v, nil
}

// evalAsyncRead handles the eval async read step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncRead(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "read_async", func(values []Value) (Value, error) { return readValue(values) })
}

// evalAsyncReadBytes handles the eval async read bytes step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncReadBytes(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "readbytes_async", func(values []Value) (Value, error) { return readBytesValue(values) })
}

// evalAsyncWrite handles the eval async write step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncWrite(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "write_async", func(values []Value) (Value, error) { return writeValue(values) })
}

// evalAsyncLines handles the eval async lines step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncLines(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "lines_async", func(values []Value) (Value, error) { return linesValue(values) })
}

// evalAsyncCSV handles the eval async csv step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncCSV(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "csv_async", func(values []Value) (Value, error) { return csvValue(values) })
}

// evalAsyncJSONParse handles the eval async jsonparse step in the surrounding
// package and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncJSONParse(ev *evaluator, args []Expr, scope *scope) (Value, error) {
	return evalAsyncIO(ev, args, scope, "jsonparse_async", func(values []Value) (Value, error) { return jsonParseValue(values) })
}

// evalRace handles the eval race/select step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalRace(ev *evaluator, args []Expr, scope *scope, selectMode bool) (Value, error) {
	if ev == nil || ev.session == nil {
		return nullValue(), fmt.Errorf("race/select requires an active session")
	}
	if len(args) == 0 {
		return nullValue(), fmt.Errorf("race/select expects at least 1 argument")
	}
	if ev.rt != nil && ev.rt.taskDebug {
		for i, arg := range args {
			child := NewSession(ev.rt, ev.ctx, ev.session.sourceName)
			child.emitAllowed = false
			child.scope = cloneScope(scope)
			child.packageName = ev.session.packageName
			child.packageVersion = ev.session.packageVersion
			child.exportOrder = append([]string(nil), ev.session.exportOrder...)
			if len(ev.session.exportSet) > 0 {
				child.exportSet = make(map[string]struct{}, len(ev.session.exportSet))
				for k := range ev.session.exportSet {
					child.exportSet[k] = struct{}{}
				}
			}
			child.macros = cloneMacroDefs(ev.session.macros)
			var outputs []Value
			child.outputs = &outputs
			v, err := child.eval(arg, child.scope)
			if sig, ok := asFlow(err); ok {
				if sig.kind == flowReturn {
					v = sig.value
					err = nil
				} else {
					err = flowError(sig.kind, "race/select")
				}
			}
			if err != nil {
				return nullValue(), err
			}
			if ev.session.outputs != nil && len(outputs) > 0 {
				*ev.session.outputs = append(*ev.session.outputs, outputs...)
			}
			if selectMode {
				return listValue([]Value{uintValue(uint64(i), 64), v}), nil
			}
			return v, nil
		}
	}
	groupCtx, cancel := context.WithCancel(ev.ctx)
	defer cancel()
	type raceResult struct {
		idx  int
		val  Value
		err  error
		outs []Value
	}
	results := make(chan raceResult, len(args))
	var wg sync.WaitGroup
	for i, expr := range args {
		i, expr := i, expr
		wg.Add(1)
		go func() {
			defer wg.Done()
			childCtx, branchCancel := context.WithCancel(groupCtx)
			defer branchCancel()
			child := NewSession(ev.rt, childCtx, ev.session.sourceName)
			child.emitAllowed = false
			child.scope = cloneScope(scope)
			child.packageName = ev.session.packageName
			child.packageVersion = ev.session.packageVersion
			child.exportOrder = append([]string(nil), ev.session.exportOrder...)
			if len(ev.session.exportSet) > 0 {
				child.exportSet = make(map[string]struct{}, len(ev.session.exportSet))
				for k := range ev.session.exportSet {
					child.exportSet[k] = struct{}{}
				}
			}
			child.macros = cloneMacroDefs(ev.session.macros)
			var outputs []Value
			child.outputs = &outputs
			v, err := child.eval(expr, child.scope)
			if sig, ok := asFlow(err); ok {
				if sig.kind == flowReturn {
					v = sig.value
					err = nil
				} else {
					err = flowError(sig.kind, "race/select")
				}
			}
			if err != nil {
				select {
				case results <- raceResult{idx: i, err: err, outs: append([]Value(nil), outputs...)}:
				case <-groupCtx.Done():
				}
				cancel()
				return
			}
			select {
			case results <- raceResult{idx: i, val: v, outs: append([]Value(nil), outputs...)}:
			case <-groupCtx.Done():
			}
		}()
	}
	go func() {
		wg.Wait()
		close(results)
	}()
	res, ok := <-results
	if !ok {
		return nullValue(), fmt.Errorf("race/select produced no result")
	}
	cancel()
	if res.err != nil {
		return nullValue(), res.err
	}
	if ev.session.outputs != nil && len(res.outs) > 0 {
		*ev.session.outputs = append(*ev.session.outputs, res.outs...)
	}
	if selectMode {
		return listValue([]Value{uintValue(uint64(res.idx), 64), res.val}), nil
	}
	return res.val, nil
}

// evalAsyncIO handles the eval async io step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func evalAsyncIO(ev *evaluator, args []Expr, scope *scope, op string, run func([]Value) (Value, error)) (Value, error) {
	if ev == nil || ev.session == nil {
		return nullValue(), fmt.Errorf("%s requires an active session", op)
	}
	values, err := evalMany(ev, args, scope)
	if err != nil {
		return nullValue(), err
	}
	return ev.session.spawnValueTask(func(ctx context.Context) (Value, error, []Value) {
		if err := ctx.Err(); err != nil {
			return nullValue(), err, nil
		}
		v, err := run(values)
		return v, err, nil
	}), nil
}

func firstEmitSpanIn(ev *evaluator, exprs []Expr, scope *scope) (sourceSpan, bool) {
	for _, expr := range exprs {
		if span, ok := firstEmitSpan(ev, expr, scope, nil); ok {
			return span, true
		}
	}
	return sourceSpan{}, false
}

func chooseSourceSpan(primary, fallback sourceSpan) sourceSpan {
	if primary.start.line > 0 {
		return primary
	}
	return fallback
}

// firstEmitSpan conservatively follows executable expression structure and
// statically known user functions. The runtime emit guard remains authoritative
// for dynamically obtained callables.
func firstEmitSpan(ev *evaluator, expr Expr, scope *scope, seen map[*functionValue]struct{}) (sourceSpan, bool) {
	switch n := expr.(type) {
	case *callExpr:
		if id, ok := n.callee.(*identExpr); ok && scope != nil {
			if binding, ok := scope.lookup(id.name); ok {
				if literal, ok := binding.expr.(*literalExpr); ok && literal.value.kind == kindFunction && literal.value.fn != nil && literal.value.fn.body != nil {
					fn := literal.value.fn
					if seen == nil {
						seen = make(map[*functionValue]struct{})
					}
					if _, active := seen[fn]; !active {
						next := make(map[*functionValue]struct{}, len(seen)+1)
						for item := range seen {
							next[item] = struct{}{}
						}
						next[fn] = struct{}{}
						if span, found := firstEmitSpan(ev, fn.body, fn.closure, next); found {
							return chooseSourceSpan(n.Span, span), true
						}
					}
				}
			}
		}
		if span, ok := firstEmitSpan(ev, n.callee, scope, seen); ok {
			return span, true
		}
		return firstEmitSpanInSeen(ev, n.args, scope, seen)
	case *blockExpr:
		for _, stmt := range n.stmts {
			switch st := stmt.(type) {
			case emitStmt:
				return st.Span, true
			case letStmt:
				if span, ok := firstEmitSpan(ev, st.Expr, scope, seen); ok {
					return span, true
				}
			case assignStmt:
				if span, ok := firstEmitSpan(ev, st.Expr, scope, seen); ok {
					return span, true
				}
			case whileStmt:
				if span, ok := firstEmitSpan(ev, st.Cond, scope, seen); ok {
					return span, true
				}
				if span, ok := firstEmitSpan(ev, st.Body, scope, seen); ok {
					return span, true
				}
			case forStmt:
				if span, ok := firstEmitSpan(ev, st.Iter, scope, seen); ok {
					return span, true
				}
				if span, ok := firstEmitSpan(ev, st.Body, scope, seen); ok {
					return span, true
				}
			case returnStmt:
				if st.Expr != nil {
					if span, ok := firstEmitSpan(ev, st.Expr, scope, seen); ok {
						return span, true
					}
				}
			}
		}
		if n.tail != nil {
			return firstEmitSpan(ev, n.tail, scope, seen)
		}
	case *memberExpr:
		return firstEmitSpan(ev, n.object, scope, seen)
	case *unaryExpr:
		return firstEmitSpan(ev, n.expr, scope, seen)
	case *binaryExpr:
		if span, ok := firstEmitSpan(ev, n.left, scope, seen); ok {
			return span, true
		}
		return firstEmitSpan(ev, n.right, scope, seen)
	case *listExpr:
		return firstEmitSpanInSeen(ev, n.items, scope, seen)
	case *matchExpr:
		if span, ok := firstEmitSpan(ev, n.subject, scope, seen); ok {
			return span, true
		}
		for _, arm := range n.arms {
			if arm.guard != nil {
				if span, ok := firstEmitSpan(ev, arm.guard, scope, seen); ok {
					return span, true
				}
			}
			if span, ok := firstEmitSpan(ev, arm.body, scope, seen); ok {
				return span, true
			}
		}
	case *tryExpr:
		if span, ok := firstEmitSpan(ev, n.body, scope, seen); ok {
			return span, true
		}
		return firstEmitSpan(ev, n.handler, scope, seen)
	case *propagateExpr:
		return firstEmitSpan(ev, n.expr, scope, seen)
	}
	return sourceSpan{}, false
}

func firstEmitSpanInSeen(ev *evaluator, exprs []Expr, scope *scope, seen map[*functionValue]struct{}) (sourceSpan, bool) {
	for _, expr := range exprs {
		if span, ok := firstEmitSpan(ev, expr, scope, seen); ok {
			return span, true
		}
	}
	return sourceSpan{}, false
}

// exprHasTaskEffect handles the expr has task effect step in the surrounding
// package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func exprHasTaskEffect(ev *evaluator, expr Expr, scope *scope, seen map[*functionValue]struct{}) bool {
	switch n := expr.(type) {
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "spawn", "await", "cancel", "parallel", "timeout", "race", "select", "read_async", "readbytes_async", "write_async", "lines_async", "csv_async", "jsonparse_async":
				return true
			}
		}
		if id, ok := n.callee.(*identExpr); ok && builtinHasTaskEffect(id.name) {
			return true
		}
		if id, ok := n.callee.(*identExpr); ok && ev != nil && scope != nil {
			if b, ok := scope.lookup(id.name); ok {
				if lit, ok := b.expr.(*literalExpr); ok && lit != nil && lit.value.kind == kindFunction && lit.value.fn != nil && lit.value.fn.body != nil {
					if seen == nil {
						seen = make(map[*functionValue]struct{})
					}
					if _, ok := seen[lit.value.fn]; ok {
						return false
					}
					nextSeen := make(map[*functionValue]struct{}, len(seen)+1)
					for fn := range seen {
						nextSeen[fn] = struct{}{}
					}
					nextSeen[lit.value.fn] = struct{}{}
					if exprHasTaskEffect(ev, lit.value.fn.body, lit.value.fn.closure, nextSeen) {
						return true
					}
				}
			}
		}
		if exprHasTaskEffect(ev, n.callee, scope, seen) {
			return true
		}
		for _, arg := range n.args {
			if exprHasTaskEffect(ev, arg, scope, seen) {
				return true
			}
		}
		return false
	case *blockExpr:
		for _, stmt := range n.stmts {
			if stmtHasTaskEffect(ev, stmt, scope, seen) {
				return true
			}
		}
		return n.tail != nil && exprHasTaskEffect(ev, n.tail, scope, seen)
	case *memberExpr:
		return exprHasTaskEffect(ev, n.object, scope, seen)
	case *unaryExpr:
		return exprHasTaskEffect(ev, n.expr, scope, seen)
	case *binaryExpr:
		return exprHasTaskEffect(ev, n.left, scope, seen) || exprHasTaskEffect(ev, n.right, scope, seen)
	case *listExpr:
		for _, item := range n.items {
			if exprHasTaskEffect(ev, item, scope, seen) {
				return true
			}
		}
		return false
	case *matchExpr:
		if exprHasTaskEffect(ev, n.subject, scope, seen) {
			return true
		}
		for _, arm := range n.arms {
			if arm.guard != nil && exprHasTaskEffect(ev, arm.guard, scope, seen) {
				return true
			}
			if exprHasTaskEffect(ev, arm.body, scope, seen) {
				return true
			}
		}
		return false
	case *tryExpr:
		return exprHasTaskEffect(ev, n.body, scope, seen) || exprHasTaskEffect(ev, n.handler, scope, seen)
	case *propagateExpr:
		return exprHasTaskEffect(ev, n.expr, scope, seen)
	default:
		return false
	}
}

// stmtHasTaskEffect handles the stmt has task effect step in the surrounding
// package and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func stmtHasTaskEffect(ev *evaluator, stmt Stmt, scope *scope, seen map[*functionValue]struct{}) bool {
	switch st := stmt.(type) {
	case letStmt:
		return exprHasTaskEffect(ev, st.Expr, scope, seen)
	case assignStmt:
		return exprHasTaskEffect(ev, st.Expr, scope, seen)
	case emitStmt:
		return exprHasTaskEffect(ev, st.Expr, scope, seen)
	case macroStmt:
		return exprHasTaskEffect(ev, st.Body, scope, seen)
	case funcStmt:
		return exprHasTaskEffect(ev, st.Body, scope, seen)
	case whileStmt:
		return exprHasTaskEffect(ev, st.Cond, scope, seen) || exprHasTaskEffect(ev, st.Body, scope, seen)
	case forStmt:
		return exprHasTaskEffect(ev, st.Iter, scope, seen) || exprHasTaskEffect(ev, st.Body, scope, seen)
	case returnStmt:
		return st.Expr != nil && exprHasTaskEffect(ev, st.Expr, scope, seen)
	case breakStmt, continueStmt, importStmt, packageStmt, exportStmt:
		return false
	default:
		return false
	}
}

// resolveMember handles the resolve member step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func resolveMember(ctx context.Context, obj Value, name string, runtimes ...*Runtime) (Value, error) {
	if obj.kind == kindError {
		if v, ok := obj.errorRecord().member(name); ok {
			return v, nil
		}
		return nullValue(), fmt.Errorf("error values do not have member %q", name)
	}
	if obj.kind == kindStruct && obj.structType != nil {
		if obj.structValue != nil {
			idx, ok := obj.structType.byName[name]
			if !ok {
				for _, rt := range runtimes {
					if method, ok := rt.lookupMethod(obj.structType.name, name); ok {
						return bindMethodReceiver(method, obj), nil
					}
				}
				return nullValue(), fmt.Errorf("struct %s has no field %q", obj.structType.name, name)
			}
			return obj.structValue.fields[idx], nil
		}
		if name == "name" {
			return stringValue(obj.structType.name), nil
		}
		if name == "fields" {
			items := make([]Value, len(obj.structType.fields))
			for i, field := range obj.structType.fields {
				items[i] = stringValue(field.name)
			}
			return listValue(items), nil
		}
		for _, rt := range runtimes {
			if method, ok := rt.lookupMethod(obj.structType.name, name); ok {
				return bindMethodReceiver(method, obj), nil
			}
		}
		return nullValue(), fmt.Errorf("struct type %s has no member %q", obj.structType.name, name)
	}
	// Primitive receiver methods use the same nominal registry as struct
	// methods. Interface values retain their concrete tagged value, so dynamic
	// dispatch deliberately resolves the concrete static type here.
	receiverType := staticTypeFromValue(obj).name
	for _, rt := range runtimes {
		if method, ok := rt.lookupMethod(receiverType, name); ok {
			return bindMethodReceiver(method, obj), nil
		}
	}
	mod, err := obj.AsModule()
	if err != nil {
		return nullValue(), fmt.Errorf("cannot access member %q on %s", name, obj.Kind())
	}
	if mod.lookup == nil {
		return nullValue(), fmt.Errorf("module %s has no exports", mod.name)
	}
	return mod.lookup(ctx, name)
}

// evalMany handles the eval many step in the surrounding package and returns
// []Value, error.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
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

// applyUnary handles the apply unary step in the surrounding package and
// returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
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
			return complexValueStyled(-v.cx, complexComponentBits(v)), nil
		}
		if v.kind == kindDual {
			if v.dual == nil {
				return dualValueOf(0, 0), nil
			}
			return dualValueOf(-v.dual.primal, -v.dual.tangent), nil
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

// hashParts handles the hash parts step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func hashParts(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// numericBuiltin handles the numeric builtin step in the surrounding package
// and returns Value, error.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
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

// registerBuiltins handles the register builtins step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func registerBuiltins(dst map[string]Builtin) {
	dst["map_new"] = mapNewValue
	dst["map_set"] = mapSetValue
	dst["map_get"] = mapGetValue
	dst["map_keys"] = mapKeysValue
	dst["map_values"] = mapValuesValue
	dst["set_new"] = setNewValue
	dst["set_add"] = setAddValue
	dst["set_has"] = setHasValue
	dst["set_values"] = setValuesValue
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
		case kindMap:
			if args[0].mapValue == nil {
				return numberValue(0), nil
			}
			return numberValue(float64(len(args[0].mapValue.entries))), nil
		case kindSet:
			if args[0].setValue == nil {
				return numberValue(0), nil
			}
			return numberValue(float64(len(args[0].setValue.items))), nil
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
	dst["slice"] = func(args []Value) (Value, error) { return sliceValue(args) }
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
	dst["sum"] = sumValue
	dst["product"] = productValue
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
	dst["random"] = seededRandomValue
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
	dst["number"] = func(args []Value) (Value, error) {
		value, err := castFloatWidthValue(args, 64)
		if err != nil {
			return nullValue(), err
		}
		value.fbits = 0
		return value, nil
	}
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
	dst["dual"] = func(args []Value) (Value, error) { return dualValueFromArgs(args) }
	dst["primal"] = func(args []Value) (Value, error) { return primalValue(args) }
	dst["tangent"] = func(args []Value) (Value, error) { return tangentValue(args) }
	dst["real"] = func(args []Value) (Value, error) { return realValue(args) }
	dst["imag"] = func(args []Value) (Value, error) { return imagValue(args) }
	dst["conjugate"] = func(args []Value) (Value, error) { return conjugateValue(args) }
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
	dst["atan2"] = atan2Value
	dst["exp2"] = exp2Value
	dst["trunc"] = truncValue
	dst["fmod"] = fmodValue
	dst["remainder"] = remainderValue
	dst["fma"] = fmaValue
	dst["positive_difference"] = positiveDifferenceValue
	dst["copysign"] = copySignValue
	dst["nextafter"] = nextAfterValue
	dst["frexp"] = frexpValue
	dst["ldexp"] = ldexpValue
	dst["modf"] = modfValue
	dst["isfinite"] = isFiniteValue
	dst["isinf"] = isInfValue
	dst["isnan"] = isNaNValue
	dst["isnormal"] = isNormalValue
	dst["signbit"] = signBitValue
	dst["degrees"] = degreesValue
	dst["radians"] = radiansValue
	dst["sinpi"] = sinPiValue
	dst["cospi"] = cosPiValue
	dst["tanpi"] = tanPiValue
	dst["divmod"] = divmodValue
	dst["gcd"] = gcdValue
	dst["lcm"] = lcmValue
	dst["count_ones"] = countOnesValue
	dst["count_zeros"] = countZerosValue
	dst["leading_zeros"] = leadingZerosValue
	dst["trailing_zeros"] = trailingZerosValue
	dst["bit_width"] = bitWidthValue
	dst["bit_floor"] = bitFloorValue
	dst["bit_ceil"] = bitCeilValue
	dst["has_single_bit"] = hasSingleBitValue
	dst["byte_swap"] = byteSwapValue
	dst["phase"] = phaseValue
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
	dst["openread"] = func(args []Value) (Value, error) { return openReadStreamValue(args) }
	dst["openwrite"] = func(args []Value) (Value, error) { return openWriteStreamValue(args) }
	dst["readline"] = func(args []Value) (Value, error) { return readLineValue(args) }
	dst["readchunk"] = func(args []Value) (Value, error) { return readChunkValue(args) }
	dst["writechunk"] = func(args []Value) (Value, error) { return writeChunkValue(args) }
	dst["flush"] = func(args []Value) (Value, error) { return flushStreamValue(args) }
	dst["close"] = func(args []Value) (Value, error) {
		if len(args) == 1 && args[0].kind == kindTCPListener {
			return closeTCPListenerValue(args)
		}
		return closeStreamValue(args)
	}
	dst["error"] = func(args []Value) (Value, error) { return errorBuiltin(args) }
	dst["wrap"] = func(args []Value) (Value, error) { return wrapErrorBuiltin(args) }
	dst["cause"] = func(args []Value) (Value, error) { return causeErrorBuiltin(args) }
	dst["unwrap"] = func(args []Value) (Value, error) { return unwrapResultBuiltin(args) }
	dst["withmeta"] = func(args []Value) (Value, error) { return withMetaBuiltin(args) }
	dst["withdebug"] = func(args []Value) (Value, error) { return withDebugMetaBuiltin(args) }
	dst["errmeta"] = func(args []Value) (Value, error) { return errMetaBuiltin(args) }
	dst["redact"] = func(args []Value) (Value, error) { return redactErrorBuiltin(args) }
	dst["ok"] = func(args []Value) (Value, error) { return okBuiltin(args) }
	dst["err"] = func(args []Value) (Value, error) { return errBuiltin(args) }
	dst["some"] = func(args []Value) (Value, error) { return someBuiltin(args) }
	dst["none"] = func(args []Value) (Value, error) { return noneBuiltin(args) }
	dst["isok"] = func(args []Value) (Value, error) { return isOkBuiltin(args) }
	dst["iserr"] = func(args []Value) (Value, error) { return isErrBuiltin(args) }
	dst["issome"] = func(args []Value) (Value, error) { return isSomeBuiltin(args) }
	dst["isnone"] = func(args []Value) (Value, error) { return isNoneBuiltin(args) }
	dst["expect"] = func(args []Value) (Value, error) { return expectBuiltin(args) }
	dst["or_else"] = func(args []Value) (Value, error) { return orElseBuiltin(args) }
	dst["to_option"] = func(args []Value) (Value, error) { return toOptionBuiltin(args) }
	dst["to_result"] = func(args []Value) (Value, error) { return toResultBuiltin(args) }
	dst["collect"] = func(args []Value) (Value, error) { return collectResultsBuiltin(args) }
	dst["discard"] = func(args []Value) (Value, error) { return discardBuiltin(args) }
	dst["must_handle"] = func(args []Value) (Value, error) { return mustHandleBuiltin(args) }
	dst["is_must_handle"] = func(args []Value) (Value, error) { return isMustHandleBuiltin(args) }
	dst["panic"] = func(args []Value) (Value, error) { return panicBuiltin(args) }
	dst["trap"] = func(args []Value) (Value, error) { return trapBuiltin(args) }
	dst["assert"] = func(args []Value) (Value, error) { return assertBuiltin(args) }
	dst["matches"] = func(args []Value) (Value, error) { return matchesErrorBuiltin(args) }
	dst["is"] = func(args []Value) (Value, error) { return isErrorBuiltin(args) }
}

func registerContextBuiltins(dst map[string]contextCallable) {
	dst["tcp_listen"] = tcpListenContextValue
	dst["tcp_accept"] = tcpAcceptContextValue
	dst["tcp_dial"] = tcpDialContextValue
	dst["readline"] = readLineContextValue
	dst["readchunk"] = readChunkContextValue
	dst["transfer"] = transferContextValue
	dst["append"] = appendContextValue
	dst["read_limit"] = readLimitContextValue
	dst["copy_file"] = copyFileContextValue
	dst["truncate_file"] = truncateFileContextValue
	dst["exists"] = func(ctx context.Context, args []Value) (Value, error) {
		return filesystemContextCall(ctx, args, "exists", existsValue)
	}
	dst["file_size"] = func(ctx context.Context, args []Value) (Value, error) {
		return filesystemContextCall(ctx, args, "file_size", fileSizeValue)
	}
	dst["is_file"] = func(ctx context.Context, args []Value) (Value, error) {
		return filesystemContextCall(ctx, args, "is_file", isFileValue)
	}
	dst["is_dir"] = func(ctx context.Context, args []Value) (Value, error) {
		return filesystemContextCall(ctx, args, "is_dir", isDirValue)
	}
	dst["mkdir"] = mkdirContextValue
	dst["mkdir_all"] = mkdirAllContextValue
	dst["read_dir"] = readDirContextValue
	dst["rename"] = renameContextValue
	dst["remove"] = removeContextValue
	dst["openappend"] = openAppendStreamContextValue
	dst["seek"] = seekContextValue
	dst["tell"] = tellContextValue
	dst["truncate"] = truncateStreamContextValue
}

// hashValue handles the hash value step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func hashValue(args []Value) string {
	h := sha256.New()
	for _, arg := range args {
		_, _ = h.Write([]byte(arg.String()))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}
