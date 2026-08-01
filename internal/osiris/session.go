package osiris

import (
	"context"
	"fmt"
	"strings"
)

type Session struct {
	rt             *Runtime
	ctx            context.Context
	sourceName     string
	scope          *scope
	ev             *evaluator
	outputs        *[]Value
	emitAllowed    bool
	moduleStack    []string
	packageName    string
	packageVersion string
	exportOrder    []string
	exportSet      map[string]struct{}
	macros         map[string]*macroDef
}

func runtimeStructType(st structStmt) *structTypeValue {
	typ := &structTypeValue{name: st.Name, fields: make([]structFieldValue, len(st.Fields)), byName: make(map[string]int, len(st.Fields))}
	for i, field := range st.Fields {
		typ.fields[i] = structFieldValue{name: field.Name, typeName: field.Type, tag: field.Tag, tagValues: cloneStringMap(field.TagValues)}
		typ.byName[field.Name] = i
	}
	return typ
}

// NewSession constructs a new session and wires together the internal caches,
// registries, and default tables that the caller would otherwise have to
// assemble by hand.
//
// It returns *Session so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func NewSession(rt *Runtime, ctx context.Context, sourceName string) *Session {
	sess := &Session{
		rt:          rt,
		ctx:         ctx,
		sourceName:  sourceName,
		emitAllowed: true,
		exportSet:   make(map[string]struct{}),
		macros:      make(map[string]*macroDef),
	}
	ev := &evaluator{
		ctx:     ctx,
		rt:      rt,
		session: sess,
		cache:   make(map[string]Value),
		pending: make(map[string]*promise),
		fpMemo:  make(map[fpKey]string),
		fpDepth: make(map[fpKey]int),
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
// It returns the values committed before execution stopped and an error. A
// non-nil error does not discard earlier successful emit statements.
func (s *Session) ExecProgram(prog *Program, moduleMode bool, stack []string) ([]Value, error) {
	if _, err := s.rt.CheckProgram(s.ctx, prog); err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	previousStack := s.moduleStack
	s.moduleStack = append([]string(nil), stack...)
	defer func() {
		s.moduleStack = previousStack
	}()
	expanded, expandedMacros, err := s.expandProgramWithStack(prog, stack)
	if err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	prog = expanded
	if _, err := s.rt.CheckProgram(s.ctx, prog); err != nil {
		return nil, normalizeError(err, prog.SourceName)
	}
	s.macros = cloneMacroDefs(expandedMacros)
	s.scope = s.predeclareFunctionScope(prog.Statements, s.scope)
	var outputs []Value
	prevOutputs := s.outputs
	s.outputs = &outputs
	defer func() {
		s.outputs = prevOutputs
	}()
	for _, stmt := range prog.Statements {
		if err := s.checkContext(); err != nil {
			return outputs, err
		}
		v, ok, err := s.execStmt(stmt, prog.SourceName, moduleMode, stack)
		if err != nil {
			if sig, ok := asFlow(err); ok {
				return outputs, flowError(sig.kind, "top-level")
			}
			return outputs, normalizeError(err, s.sourceName)
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
			return outputs, normalizeError(fmt.Errorf("module %s is missing a package declaration", prog.SourceName), prog.SourceName)
		}
		if len(s.exportOrder) == 0 {
			return outputs, normalizeError(fmt.Errorf("module %s must declare at least one export", prog.SourceName), prog.SourceName)
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
	case structStmt:
		typ := runtimeStructType(st)
		s.scope = &scope{parent: s.scope, items: map[string]binding{
			st.Name: {expr: &literalExpr{value: structTypeValueOf(typ)}, env: s.scope},
		}}
		return nil, false, nil
	case typeDeclStmt:
		// Transparent aliases are constructor aliases at runtime; the static
		// checker remains responsible for their type identity.
		alias := functionValueOf(st.Name, func(args []Value) (Value, error) {
			if len(args) != 1 {
				return nullValue(), fmt.Errorf("%s expects 1 argument", st.Name)
			}
			return args[0], nil
		})
		s.scope = &scope{parent: s.scope, items: map[string]binding{st.Name: {expr: &literalExpr{value: alias}, env: s.scope}}}
		return nil, false, nil
	case newtypeStmt:
		typ := &structTypeValue{name: st.Name, target: st.Target, newtype: true, fields: []structFieldValue{{name: "value", typeName: st.Target}}, byName: map[string]int{"value": 0}}
		s.scope = &scope{parent: s.scope, items: map[string]binding{st.Name: {expr: &literalExpr{value: structTypeValueOf(typ)}, env: s.scope}}}
		return nil, false, nil
	case macroStmt:
		if err := s.registerMacro(st.Name, st.Params, st.Body); err != nil {
			return nil, false, err
		}
		return nil, false, nil
	case funcStmt:
		scope, err := s.declareFunction(s.scope, st)
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
		if st.Type != "" {
			value, err := s.ev.eval(expr, s.scope)
			if err != nil {
				return nil, false, err
			}
			if isBindingTypeName(st.Type) || strings.HasPrefix(st.Type, "func(") {
				value, err = checkBindingType(st.Type, value, false)
				if err != nil {
					return nil, false, fmt.Errorf("let %s: %w", st.Name, err)
				}
			}
			expr = &literalExpr{value: value}
		}
		if len(st.Names) > 0 {
			value, err := s.ev.eval(expr, s.scope)
			if err != nil {
				return nil, false, err
			}
			if value.kind != kindTuple || len(value.tuple) != len(st.Names) {
				return nil, false, fmt.Errorf("tuple destructuring requires %d values", len(st.Names))
			}
			cur := s.scope
			for i, name := range st.Names {
				cur = &scope{parent: cur, items: map[string]binding{name: {expr: &literalExpr{value: value.tuple[i]}, env: cur}}}
			}
			s.scope = cur
			return nil, false, nil
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
			return nil, false, structuredErrorAt(newOsirisError(ErrorKindUnsupported, "module initialization cannot emit values"), s.sourceName, st.Span)
		}
		if !s.emitAllowed {
			return nil, false, structuredErrorAt(concurrentEmitError(), s.sourceName, st.Span)
		}
		v, err := s.ev.eval(st.Expr, s.scope)
		if err != nil {
			if _, ok := asFlow(err); ok {
				return nil, false, err
			}
			return nil, false, structuredErrorAt(normalizeError(err, s.sourceName), s.sourceName, st.Span)
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
func (s *Session) declareFunction(cur *scope, st funcStmt) (*scope, error) {
	expanded, err := s.expandExpr(st.Body, cur)
	if err != nil {
		return nil, err
	}
	fn := typedUserFunctionValue(st.Name, st.Params, st.ParamTypes, st.ReturnType, expanded, cur, s)
	for search := cur; search != nil; search = search.parent {
		candidate, ok := search.items[st.Name]
		if !ok {
			continue
		}
		literal, ok := candidate.expr.(*literalExpr)
		if !ok || literal.value.kind != kindFunction || literal.value.fn == nil || !literal.value.fn.predeclared {
			continue
		}
		placeholder := literal.value.fn
		placeholder.signature = fn.fn.signature
		placeholder.paramTypes = append(placeholder.paramTypes[:0], st.ParamTypes...)
		placeholder.returnType = st.ReturnType
		placeholder.params = append(placeholder.params[:0], st.Params...)
		placeholder.body = expanded
		placeholder.session = s
		fn = literal.value
		break
	}
	group := fn.fn.group
	if group == nil {
		group = &scope{items: make(map[string]binding)}
	}
	selfScope := &scope{
		parent: cur,
		items:  group.items,
	}
	if fn.fn != nil {
		fn.fn.closure = selfScope
		fn.fn.predeclared = false
	}
	return &scope{
		parent: cur,
		items: map[string]binding{
			st.Name: {expr: &literalExpr{value: fn}, env: cur},
		},
	}, nil
}

// predeclareFunctionScope creates callable placeholders for a lexical
// statement sequence. Declarations fill those placeholders when reached,
// preserving sequential capture points while allowing mutual recursion.
func (s *Session) predeclareFunctionScope(stmts []Stmt, parent *scope) *scope {
	group := &scope{parent: parent, items: make(map[string]binding)}
	for _, stmt := range stmts {
		st, ok := stmt.(funcStmt)
		if !ok {
			continue
		}
		value := typedUserFunctionValue(st.Name, st.Params, st.ParamTypes, st.ReturnType, st.Body, group, s)
		value.fn.group = group
		value.fn.predeclared = true
		group.items[st.Name] = binding{expr: &literalExpr{value: value, Span: st.NameSpan}, env: group}
	}
	if len(group.items) == 0 {
		return parent
	}
	return group
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
		cur := s.predeclareFunctionScope(n.stmts, curScope)
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
				next, err := s.declareFunction(cur, st)
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
				if !s.emitAllowed {
					return nullValue(), cur, structuredErrorAt(concurrentEmitError(), s.sourceName, st.Span)
				}
				v, err := s.ev.eval(st.Expr, cur)
				if err != nil {
					if _, ok := asFlow(err); ok {
						return nullValue(), cur, err
					}
					return nullValue(), cur, structuredErrorAt(normalizeError(err, s.sourceName), s.sourceName, st.Span)
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
		child.emitAllowed = false
		child.scope = scope
		child.packageName = s.packageName
		child.packageVersion = s.packageVersion
		child.macros = cloneMacroDefs(s.macros)
		v, err := child.eval(expr, scope)
		if sig, ok := asFlow(err); ok {
			if sig.kind == flowReturn {
				return sig.value, nil, nil
			}
			return nullValue(), flowError(sig.kind, "task"), nil
		}
		if err != nil {
			return nullValue(), err, nil
		}
		return v, nil, nil
	})
}

func concurrentEmitError() error {
	return newOsirisError(ErrorKindUnsupported, "emit is not allowed during concurrent execution; return a value and emit it after await")
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
