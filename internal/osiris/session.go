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
	packageName    string
	packageVersion string
	exportOrder    []string
	exportSet      map[string]struct{}
	macros         map[string]*macroDef
}

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
		builtin: rt.Builtins,
	}
	sess.ev = ev
	return sess
}

func (s *Session) ExecSource(src string) ([]Value, error) {
	prog, err := Parse(s.sourceName, src)
	if err != nil {
		return nil, err
	}
	return s.ExecProgram(prog, false, nil)
}

func (s *Session) ExecProgram(prog *Program, moduleMode bool, stack []string) ([]Value, error) {
	var outputs []Value
	for _, stmt := range prog.Statements {
		v, ok, err := s.execStmt(stmt, prog.SourceName, moduleMode, stack)
		if err != nil {
			return nil, err
		}
		if ok {
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
		return []Value{v}, true, nil
	default:
		return nil, false, fmt.Errorf("internal error: unknown statement type %T", stmt)
	}
}

func (s *Session) eval(expr Expr, scope *scope) (Value, error) {
	return s.ev.eval(expr, scope)
}

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
	}), nil
}

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
