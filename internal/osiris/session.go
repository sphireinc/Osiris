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

// NewSession constructs and returns a new the session with the default internal state already wired together so callers do not need to assemble caches or dependencies by hand.. It returns *Session so callers can continue with the next stage of the pipeline without re-deriving the same information.
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
	}
	sess.ev = ev
	return sess
}

// ExecSource handles on Session the exec source. It keeps the Session subsystem self-contained by handling the exec source-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) ExecSource(src string) ([]Value, error) {
	prog, err := Parse(s.sourceName, src)
	if err != nil {
		return nil, err
	}
	return s.ExecProgram(prog, false, nil)
}

// ExecProgram handles on Session the exec program. It keeps the Session subsystem self-contained by handling the exec program-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) ExecProgram(prog *Program, moduleMode bool, stack []string) ([]Value, error) {
	var outputs []Value
	prevOutputs := s.outputs
	s.outputs = &outputs
	defer func() {
		s.outputs = prevOutputs
	}()
	for _, stmt := range prog.Statements {
		v, ok, err := s.execStmt(stmt, prog.SourceName, moduleMode, stack)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				return nil, flowError(sig.kind, "top-level")
			}
			return nil, err
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
			return nil, fmt.Errorf("module %s is missing a package declaration", prog.SourceName)
		}
		if len(s.exportOrder) == 0 {
			return nil, fmt.Errorf("module %s must declare at least one export", prog.SourceName)
		}
	}
	return outputs, nil
}

// execStmt handles on Session the exec stmt. It keeps the Session subsystem self-contained by handling the exec stmt-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// execControlStmt handles on Session the exec control stmt. It keeps the Session subsystem self-contained by handling the exec control stmt-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// declareFunction handles on Session the declare function. It keeps the Session subsystem self-contained by handling the declare function-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// execWhile handles on Session the exec while. It keeps the Session subsystem self-contained by handling the exec while-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) execWhile(st whileStmt, curScope *scope, baseSource string, stack []string) (*scope, error) {
	cur := curScope
	for {
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

// execFor handles on Session the exec for. It keeps the Session subsystem self-contained by handling the exec for-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// execLoopBodyWithScope handles on Session the exec loop body with scope. It keeps the Session subsystem self-contained by handling the exec loop body with scope-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) execLoopBodyWithScope(body Expr, curScope *scope, baseSource string, stack []string) (*scope, error) {
	_, next, err := s.evalBlockExpr(body, curScope, baseSource, stack)
	return next, err
}

// evalBlockExpr handles on Session the eval block expr. It keeps the Session subsystem self-contained by handling the eval block expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) evalBlockExpr(body Expr, curScope *scope, baseSource string, stack []string) (Value, *scope, error) {
	switch n := body.(type) {
	case *blockExpr:
		cur := curScope
		for _, stmt := range n.stmts {
			switch st := stmt.(type) {
			case letStmt:
				expr, err := s.expandExpr(st.Expr, cur)
				if err != nil {
					return nullValue(), cur, err
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

// eval handles on Session the eval. It keeps the Session subsystem self-contained by handling the eval-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *Session) eval(expr Expr, scope *scope) (Value, error) {
	return s.ev.eval(expr, scope)
}

// moduleValue handles on Session the module value. It keeps the Session subsystem self-contained by handling the module value-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// EvalAt eval on Runtime the at. It keeps the Runtime subsystem self-contained by handling the at-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
