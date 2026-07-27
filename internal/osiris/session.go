package osiris

import (
	"context"
	"fmt"
)

type Session struct {
	rt             *Runtime
	ctx            context.Context
	sourceName     string
	scope          *scope
	ev             *evaluator
	outputs        *[]Value
	packageName    string
	packageVersion string
	exportOrder    []string
	exportSet      map[string]struct{}
	macros         map[string]*macroDef
}

// NewSession constructs a new session and wires together the internal caches,
// registries, and default tables that the caller would otherwise have to
// assemble by hand.
//
// It returns *Session so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func NewSession(rt *Runtime, ctx context.Context, sourceName string) *Session {
	sess := &Session{
		rt:         rt,
		ctx:        ctx,
		sourceName: sourceName,
		exportSet:  make(map[string]struct{}),
		macros:     make(map[string]*macroDef),
	}
	ev := &evaluator{
		ctx:     ctx,
		rt:      rt,
		session: sess,
		cache:   make(map[string]Value),
		pending: make(map[string]*promise),
		fpMemo:  make(map[fpKey]string),
		fpBusy:  make(map[fpKey]struct{}),
		builtin: rt.Builtins,
		sched: deterministicScheduler{
			debug: rt != nil && rt.taskDebug,
			trace: func(ev TaskTraceEvent) {
				if rt != nil {
					rt.recordTaskTrace(ev.Kind, ev.TaskID, ev.Status, ev.Detail)
				}
			},
		},
	}
	sess.ev = ev
	return sess
}

// ExecSource operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (s *Session) ExecSource(src string) ([]Value, error) {
	prog, err := Parse(s.sourceName, src)
	if err != nil {
		return nil, err
	}
	return s.ExecProgram(prog, false, nil)
}

// ExecProgram operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []Value and an error. Callers should only trust the value when
// the error is nil.
func (s *Session) ExecProgram(prog *Program, moduleMode bool, stack []string) ([]Value, error) {
	var outputs []Value
	prevOutputs := s.outputs
	s.outputs = &outputs
	defer func() {
		s.outputs = prevOutputs
	}()
	for _, stmt := range prog.Statements {
		if err := s.checkContext(); err != nil {
			return nil, err
		}
		v, ok, err := s.execStmt(stmt, prog.SourceName, moduleMode, stack)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				return nil, flowError(sig.kind, "top-level")
			}
			return nil, normalizeError(err, s.sourceName)
		}
		if ok {
			if _, isEmit := stmt.(emitStmt); isEmit {
				continue
			}
			outputs = append(outputs, v...)
		}
	}
	if moduleMode {
		if s.packageName == "" {
			return nil, normalizeError(fmt.Errorf("module %s is missing a package declaration", prog.SourceName), prog.SourceName)
		}
		if len(s.exportOrder) == 0 {
			return nil, normalizeError(fmt.Errorf("module %s must declare at least one export", prog.SourceName), prog.SourceName)
		}
	}
	return outputs, nil
}

// execStmt operates on Session values and keeps the session-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns []Value, bool, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func (s *Session) execStmt(stmt Stmt, baseSource string, moduleMode bool, stack []string) ([]Value, bool, error) {
	switch st := stmt.(type) {
	case packageStmt:
		if s.packageName != "" && (s.packageName != st.Name || s.packageVersion != st.Version) {
			return nil, false, fmt.Errorf("package redeclared as %q %q (was %q %q)", st.Name, st.Version, s.packageName, s.packageVersion)
		}
		s.packageName = st.Name
		s.packageVersion = st.Version
		return nil, false, nil
	case exportStmt:
		for _, name := range st.Names {
			if _, exists := s.exportSet[name]; exists {
				continue
			}
			s.exportSet[name] = struct{}{}
			s.exportOrder = append(s.exportOrder, name)
		}
		return nil, false, nil
	case enumStmt:
		s.scope = &scope{
			parent: s.scope,
			items: map[string]binding{
				st.Name: {expr: &literalExpr{value: enumNamespaceValue(st.Name, st.Cases)}, env: s.scope},
			},
		}
		return nil, false, nil
	case macroStmt:
		if err := s.registerMacro(st.Name, st.Params, st.Body); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	case funcStmt:
		scope, err := s.declareFunction(s.scope, st.Name, st.Params, st.Body)
		if err != nil {
			return nil, false, err
		}
		s.scope = scope
		return nil, false, nil
	case returnStmt, whileStmt, forStmt, breakStmt, continueStmt:
		nextScope, err := s.execControlStmt(stmt, s.scope, baseSource, stack)
		if err != nil {
			return nil, false, err
		}
		if nextScope != nil {
			s.scope = nextScope
		}
		return nil, false, nil
	case letStmt:
		expr, err := s.expandExpr(st.Expr, s.scope)
		if err != nil {
			return nil, false, err
		}
		if st.Unwrap {
			value, err := s.ev.eval(expr, s.scope)
			if err != nil {
				return nil, false, err
			}
			value, err = unwrapBinding(value)
			if err != nil {
				return nil, false, err
			}
			expr = &literalExpr{value: value}
		}
		s.scope = &scope{
			parent: s.scope,
			items: map[string]binding{
				st.Name: {expr: expr, env: s.scope},
			},
		}
		return nil, false, nil
	case importStmt:
		mod, err := s.rt.loadModule(s.ctx, st.Path, baseSource, stack)
		if err != nil {
			return nil, false, err
		}
		s.scope = &scope{
			parent: s.scope,
			items: map[string]binding{
				st.Alias: {expr: &literalExpr{value: mod}, env: s.scope},
			},
		}
		return nil, false, nil
	case emitStmt:
		if moduleMode {
			return nil, false, fmt.Errorf("modules cannot emit values")
		}
		v, err := s.ev.eval(st.Expr, s.scope)
		if err != nil {
			return nil, false, err
		}
		if s.outputs != nil {
			*s.outputs = append(*s.outputs, v)
		}
		return []Value{v}, true, nil
	default:
		return nil, false, fmt.Errorf("internal error: unknown statement type %T", stmt)
	}
}

// execControlStmt operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *scope and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) execControlStmt(stmt Stmt, curScope *scope, baseSource string, stack []string) (*scope, error) {
	switch st := stmt.(type) {
	case returnStmt:
		if st.Expr == nil {
			return curScope, newFlow(flowReturn, nullValue())
		}
		v, err := s.eval(st.Expr, curScope)
		if err != nil {
			return curScope, err
		}
		return curScope, newFlow(flowReturn, v)
	case whileStmt:
		return s.execWhile(st, curScope, baseSource, stack)
	case forStmt:
		return s.execFor(st, curScope, baseSource, stack)
	case breakStmt:
		return curScope, newFlow(flowBreak, nullValue())
	case continueStmt:
		return curScope, newFlow(flowContinue, nullValue())
	default:
		return curScope, fmt.Errorf("internal error: unsupported control statement %T", stmt)
	}
}

// declareFunction operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *scope and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) declareFunction(cur *scope, name string, params []string, body Expr) (*scope, error) {
	expanded, err := s.expandExpr(body, cur)
	if err != nil {
		return nil, err
	}
	fn := userFunctionValue(name, params, expanded, cur, s)
	selfScope := &scope{
		parent: cur,
		items: map[string]binding{
			name: {expr: &literalExpr{value: fn}, env: cur},
		},
	}
	if fn.fn != nil {
		fn.fn.closure = selfScope
	}
	return &scope{
		parent: cur,
		items: map[string]binding{
			name: {expr: &literalExpr{value: fn}, env: cur},
		},
	}, nil
}

// execWhile operates on Session values and keeps the session-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *scope and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) execWhile(st whileStmt, curScope *scope, baseSource string, stack []string) (*scope, error) {
	cur := curScope
	for {
		if err := s.checkContext(); err != nil {
			return cur, err
		}
		cond, err := s.eval(st.Cond, cur)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				return cur, flowError(sig.kind, "while")
			}
			return cur, err
		}
		if !cond.Truthy() {
			return cur, nil
		}
		next, err := s.execLoopBodyWithScope(st.Body, cur, baseSource, stack)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				switch sig.kind {
				case flowBreak:
					return next, nil
				case flowContinue:
					cur = next
					continue
				case flowReturn:
					return cur, err
				}
			}
			return cur, err
		}
		cur = next
	}
}

// execFor operates on Session values and keeps the session-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *scope and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) execFor(st forStmt, curScope *scope, baseSource string, stack []string) (*scope, error) {
	cur := curScope
	iter, err := s.eval(st.Iter, cur)
	if err != nil {
		if sig, ok := asFlow(err); ok {
			return cur, flowError(sig.kind, "for")
		}
		return cur, err
	}
	items, err := iter.AsList()
	if err != nil {
		return cur, err
	}
	for _, item := range items {
		if err := s.checkContext(); err != nil {
			return cur, err
		}
		iterScope := &scope{
			parent: cur,
			items: map[string]binding{
				st.Name: {expr: &literalExpr{value: item}, env: cur},
			},
		}
		next, err := s.execLoopBodyWithScope(st.Body, iterScope, baseSource, stack)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				switch sig.kind {
				case flowBreak:
					return next, nil
				case flowContinue:
					cur = next
					continue
				case flowReturn:
					return cur, err
				}
			}
			return cur, err
		}
		cur = next
	}
	return cur, nil
}

// execLoopBodyWithScope operates on Session values and keeps the
// session-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *scope and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) execLoopBodyWithScope(body Expr, curScope *scope, baseSource string, stack []string) (*scope, error) {
	_, next, err := s.evalBlockExpr(body, curScope, baseSource, stack)
	return next, err
}

// evalBlockExpr operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value, *scope, error so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func (s *Session) evalBlockExpr(body Expr, curScope *scope, baseSource string, stack []string) (Value, *scope, error) {
	switch n := body.(type) {
	case *blockExpr:
		cur := curScope
		for _, stmt := range n.stmts {
			if err := s.checkContext(); err != nil {
				return nullValue(), cur, err
			}
			switch st := stmt.(type) {
			case letStmt:
				expr, err := s.expandExpr(st.Expr, cur)
				if err != nil {
					return nullValue(), cur, err
				}
				if st.Unwrap {
					value, err := s.ev.eval(expr, cur)
					if err != nil {
						return nullValue(), cur, err
					}
					value, err = unwrapBinding(value)
					if err != nil {
						return nullValue(), cur, err
					}
					expr = &literalExpr{value: value}
				}
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Name: {expr: expr, env: cur},
					},
				}
			case importStmt:
				mod, err := s.rt.loadModule(s.ctx, st.Path, baseSource, stack)
				if err != nil {
					return nullValue(), cur, err
				}
				cur = &scope{
					parent: cur,
					items: map[string]binding{
						st.Alias: {expr: &literalExpr{value: mod}, env: cur},
					},
				}
			case macroStmt:
				if err := s.registerMacro(st.Name, st.Params, st.Body); err != nil {
					return nullValue(), cur, err
				}
			case funcStmt:
				next, err := s.declareFunction(cur, st.Name, st.Params, st.Body)
				if err != nil {
					return nullValue(), cur, err
				}
				cur = next
			case returnStmt, whileStmt, forStmt, breakStmt, continueStmt:
				next, err := s.execControlStmt(st, cur, baseSource, stack)
				if err != nil {
					return nullValue(), cur, err
				}
				if next != nil {
					cur = next
				}
			case emitStmt:
				v, err := s.ev.eval(st.Expr, cur)
				if err != nil {
					return nullValue(), cur, err
				}
				if s.outputs != nil {
					*s.outputs = append(*s.outputs, v)
				}
			case packageStmt, exportStmt:
				return nullValue(), cur, fmt.Errorf("%s is not allowed in block expressions", statementKind(st))
			default:
				return nullValue(), cur, fmt.Errorf("block expressions only support let, import, macro, func, while, for, return, break, continue, and emit statements")
			}
		}
		if n.tail == nil {
			return nullValue(), cur, nil
		}
		v, err := s.eval(n.tail, cur)
		return v, cur, err
	default:
		v, err := s.eval(body, curScope)
		return v, curScope, err
	}
}

// checkContext operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns an error when the active context has been canceled or expired.
func (s *Session) checkContext() error {
	if s == nil || s.ctx == nil {
		return nil
	}
	return s.ctx.Err()
}

// eval operates on Session values and keeps the session-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) eval(expr Expr, scope *scope) (Value, error) {
	return s.ev.eval(expr, scope)
}

// spawnTask operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (s *Session) spawnTask(expr Expr, scope *scope) Value {
	return s.spawnValueTask(func(ctx context.Context) (Value, error, []Value) {
		child := NewSession(s.rt, ctx, s.sourceName)
		child.scope = scope
		child.packageName = s.packageName
		child.packageVersion = s.packageVersion
		child.macros = cloneMacroDefs(s.macros)
		var outputs []Value
		child.outputs = &outputs
		v, err := child.eval(expr, scope)
		if sig, ok := asFlow(err); ok {
			if sig.kind == flowReturn {
				return sig.value, nil, append([]Value(nil), outputs...)
			}
			return nullValue(), flowError(sig.kind, "task"), append([]Value(nil), outputs...)
		}
		if err != nil {
			return nullValue(), err, append([]Value(nil), outputs...)
		}
		return v, nil, append([]Value(nil), outputs...)
	})
}

// moduleValue operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) moduleValue() (Value, error) {
	if s.packageName == "" {
		return nullValue(), fmt.Errorf("module has no package name")
	}
	if len(s.exportOrder) == 0 {
		return nullValue(), fmt.Errorf("module %s exported nothing", s.packageName)
	}
	exports := make(map[string]Value, len(s.exportOrder))
	for _, name := range s.exportOrder {
		b, ok := s.scope.lookup(name)
		if !ok {
			return nullValue(), fmt.Errorf("module %s exports undefined name %q", s.packageName, name)
		}
		v, err := s.eval(b.expr, b.env)
		if err != nil {
			return nullValue(), err
		}
		exports[name] = v
	}
	return moduleValueOf(s.packageName, s.packageVersion, func(ctx context.Context, name string) (Value, error) {
		if v, ok := exports[name]; ok {
			return v, nil
		}
		return nullValue(), fmt.Errorf("module %s has no member %q", s.packageName, name)
	}, exports), nil
}

// EvalAt operates on Runtime values and keeps the runtime-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Value and an error. Callers should only trust the value when the
// error is nil.
func (rt *Runtime) EvalAt(ctx context.Context, prog *Program, frameIndex int, expr string) (Value, error) {
	if rt == nil {
		rt = NewRuntime()
	}
	sess := NewSession(rt, ctx, prog.SourceName)
	limit := frameIndex + 1
	if limit < 0 {
		limit = 0
	}
	if limit > len(prog.Statements) {
		limit = len(prog.Statements)
	}
	for i := 0; i < limit; i++ {
		if _, _, err := sess.execStmt(prog.Statements[i], prog.SourceName, false, nil); err != nil {
			return nullValue(), err
		}
	}
	watchProg, err := Parse("<watch>", "emit "+expr)
	if err != nil {
		return nullValue(), err
	}
	emit, ok := watchProg.Statements[0].(emitStmt)
	if !ok {
		return nullValue(), fmt.Errorf("watch parser failed")
	}
	return sess.eval(emit.Expr, sess.scope)
}
