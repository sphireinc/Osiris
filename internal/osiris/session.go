package osiris

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"unicode/utf8"
)

type Session struct {
	rt             *Runtime
	ctx            context.Context
	sourceName     string
	scope          *scope
	ev             *evaluator
	outputs        *[]Value
	outputsMu      sync.Mutex
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

func functionBindingName(st funcStmt) string {
	if st.ReceiverType != "" {
		return st.ReceiverType + "." + st.Name
	}
	return st.Name
}

// iterationCursor is the common lazy boundary used by the interpreter and VM.
// It keeps only cursor state, so finite collections are not copied and streams
// are not drained before the first loop body executes.
type iterationCursor interface {
	next(context.Context) (Value, bool, error)
}

type indexedIterationCursor struct {
	value Value
	index int
}

func (c *indexedIterationCursor) next(ctx context.Context) (Value, bool, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), false, err
	}
	switch c.value.kind {
	case kindList:
		if c.index >= len(c.value.list) {
			return nullValue(), true, nil
		}
		v := c.value.list[c.index]
		c.index++
		return v, false, nil
	case kindTuple:
		if c.index >= len(c.value.tuple) {
			return nullValue(), true, nil
		}
		v := c.value.tuple[c.index]
		c.index++
		return v, false, nil
	case kindTensor:
		if c.index >= len(c.value.data) {
			return nullValue(), true, nil
		}
		v := c.value.data[c.index]
		c.index++
		return v, false, nil
	case kindMap:
		if c.value.mapValue == nil || c.index >= len(c.value.mapValue.entries) {
			return nullValue(), true, nil
		}
		v := c.value.mapValue.entries[c.index].key
		c.index++
		return v, false, nil
	case kindSet:
		if c.value.setValue == nil || c.index >= len(c.value.setValue.items) {
			return nullValue(), true, nil
		}
		v := c.value.setValue.items[c.index]
		c.index++
		return v, false, nil
	default:
		return nullValue(), false, fmt.Errorf("value of type %s does not implement Iterable", c.value.Kind())
	}
}

type stringIterationCursor struct {
	value string
	index int
}

func (c *stringIterationCursor) next(ctx context.Context) (Value, bool, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), false, err
	}
	if c.index >= len(c.value) {
		return nullValue(), true, nil
	}
	r, size := utf8.DecodeRuneInString(c.value[c.index:])
	c.index += size
	return stringValue(string(r)), false, nil
}

type streamIterationCursor struct{ stream *streamValue }

func (c *streamIterationCursor) next(ctx context.Context) (Value, bool, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), false, err
	}
	if c.stream == nil {
		return nullValue(), false, fmt.Errorf("cannot iterate a nil stream")
	}
	v, err := c.stream.readLineContext(ctx)
	if errors.Is(err, io.EOF) || v.kind == kindNull {
		return nullValue(), true, nil
	}
	if err != nil {
		return nullValue(), false, err
	}
	return v, false, nil
}

type taskIterationCursor struct {
	task    Value
	started bool
	session *Session
}

func (c *taskIterationCursor) next(ctx context.Context) (Value, bool, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), false, err
	}
	if c.started {
		return nullValue(), true, nil
	}
	c.started = true
	v, err := c.session.eval(&callExpr{callee: &identExpr{name: "await"}, args: []Expr{&literalExpr{value: c.task}}}, c.session.scope)
	return v, false, err
}

type protocolIterationCursor struct {
	session *Session
	value   Value
	done    bool
}

func (c *protocolIterationCursor) next(ctx context.Context) (Value, bool, error) {
	if err := contextError(ctx); err != nil {
		return nullValue(), false, err
	}
	if c.done {
		return nullValue(), true, nil
	}
	v, err := c.session.eval(&callExpr{callee: &memberExpr{object: &literalExpr{value: c.value}, name: "next"}}, c.session.scope)
	if err != nil {
		return nullValue(), false, err
	}
	record := v.optionRecord()
	if record == nil {
		return nullValue(), false, fmt.Errorf("Iterator.next must return Option<T>, got %s", v.Kind())
	}
	if !record.present {
		c.done = true
		return nullValue(), true, nil
	}
	return record.val, false, nil
}

func contextError(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func newIterationCursor(s *Session, value Value) (iterationCursor, error) {
	switch value.kind {
	case kindList, kindTuple, kindTensor, kindMap, kindSet:
		return &indexedIterationCursor{value: value}, nil
	case kindString:
		return &stringIterationCursor{value: value.str}, nil
	case kindStream:
		return &streamIterationCursor{stream: value.stream}, nil
	case kindTask:
		return &taskIterationCursor{task: value, session: s}, nil
	case kindStruct:
		if s == nil {
			return nil, fmt.Errorf("value of type %s does not implement Iterable", value.Kind())
		}
		v, err := s.eval(&callExpr{callee: &memberExpr{object: &literalExpr{value: value}, name: "iter"}}, s.scope)
		if err != nil {
			return nil, fmt.Errorf("value of type %s does not implement Iterable: %w", value.Kind(), err)
		}
		return &protocolIterationCursor{session: s, value: v}, nil
	default:
		return nil, fmt.Errorf("value of type %s does not implement Iterable", value.Kind())
	}
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
	case traitStmt:
		return nil, false, nil
	case implStmt:
		for _, method := range st.Methods {
			scope, err := s.declareFunction(s.scope, method)
			if err != nil {
				return nil, false, err
			}
			s.scope = scope
		}
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
		if st.Mutable {
			value, err := s.ev.eval(expr, s.scope)
			if err != nil {
				return nil, false, err
			}
			s.scope = &scope{
				parent: s.scope,
				items: map[string]binding{st.Name: {
					expr: &literalExpr{value: value}, env: s.scope,
					cell: &bindingCell{value: value}, mutable: true,
				}},
			}
			return nil, false, nil
		}
		value, err := s.ev.eval(expr, s.scope)
		if err != nil {
			return nil, false, err
		}
		s.scope = &scope{
			parent: s.scope,
			items: map[string]binding{st.Name: {
				expr: &literalExpr{value: value}, env: s.scope,
				cell: &bindingCell{value: value},
			}},
		}
		return nil, false, nil
	case assignStmt:
		b, ok := s.scope.lookup(st.Name)
		if !ok || b.cell == nil || !b.mutable {
			return nil, false, fmt.Errorf("cannot assign to immutable or undefined name %q", st.Name)
		}
		if err := s.checkContext(); err != nil {
			return nil, false, err
		}
		expr, err := s.expandExpr(st.Expr, s.scope)
		if err != nil {
			return nil, false, err
		}
		value, err := s.ev.eval(expr, s.scope)
		if err != nil {
			return nil, false, err
		}
		if err := s.checkContext(); err != nil {
			return nil, false, err
		}
		b.cell.value = value
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
			s.outputsMu.Lock()
			*s.outputs = append(*s.outputs, v)
			s.outputsMu.Unlock()
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
	fn := typedGenericUserFunctionValue(st.Name, st.TypeParams, st.Params, st.ParamTypes, st.ReturnType, expanded, cur, s)
	if fn.fn != nil {
		fn.fn.receiver, fn.fn.receiverType = st.Receiver, st.ReceiverType
	}
	key := functionBindingName(st)
	for search := cur; search != nil; search = search.parent {
		candidate, ok := search.items[key]
		if !ok {
			continue
		}
		literal, ok := candidate.expr.(*literalExpr)
		if !ok || literal.value.kind != kindFunction || literal.value.fn == nil || !literal.value.fn.predeclared {
			continue
		}
		placeholder := literal.value.fn
		placeholder.signature = fn.fn.signature
		placeholder.typeParams = append(placeholder.typeParams[:0], st.TypeParams...)
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
	if st.ReceiverType != "" {
		s.rt.registerMethod(st.ReceiverType, st.Name, fn)
	}
	return &scope{
		parent: cur,
		items: map[string]binding{
			key: {expr: &literalExpr{value: fn}, env: cur},
		},
	}, nil
}

// predeclareFunctionScope creates callable placeholders for a lexical
// statement sequence. Declarations fill those placeholders when reached,
// preserving sequential capture points while allowing mutual recursion.
func (s *Session) predeclareFunctionScope(stmts []Stmt, parent *scope) *scope {
	group := &scope{parent: parent, items: make(map[string]binding)}
	for _, stmt := range stmts {
		methods := []funcStmt{}
		switch st := stmt.(type) {
		case funcStmt:
			methods = append(methods, st)
		case implStmt:
			methods = append(methods, st.Methods...)
		default:
			continue
		}
		for _, st := range methods {
			value := typedGenericUserFunctionValue(st.Name, st.TypeParams, st.Params, st.ParamTypes, st.ReturnType, st.Body, group, s)
			if value.fn != nil {
				value.fn.receiver, value.fn.receiverType = st.Receiver, st.ReceiverType
			}
			value.fn.group = group
			value.fn.predeclared = true
			key := functionBindingName(st)
			group.items[key] = binding{expr: &literalExpr{value: value, Span: st.NameSpan}, env: group}
			if st.ReceiverType != "" {
				s.rt.registerMethod(st.ReceiverType, st.Name, value)
			}
		}
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
	cursor, err := newIterationCursor(s, iter)
	if err != nil {
		return cur, err
	}
	for {
		if err := s.checkContext(); err != nil {
			return cur, err
		}
		item, done, err := cursor.next(s.ctx)
		if err != nil {
			return cur, err
		}
		if done {
			break
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
				value, err := s.ev.eval(expr, cur)
				if err != nil {
					return nullValue(), cur, err
				}
				cur = &scope{
					parent: cur,
					items: map[string]binding{st.Name: {
						expr: &literalExpr{value: value}, env: cur,
						cell: &bindingCell{value: value}, mutable: st.Mutable,
					}},
				}
			case assignStmt:
				b, ok := cur.lookup(st.Name)
				if !ok || b.cell == nil || !b.mutable {
					return nullValue(), cur, fmt.Errorf("cannot assign to immutable or undefined name %q", st.Name)
				}
				expr, err := s.expandExpr(st.Expr, cur)
				if err != nil {
					return nullValue(), cur, err
				}
				value, err := s.ev.eval(expr, cur)
				if err != nil {
					return nullValue(), cur, err
				}
				b.cell.value = value
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
					s.outputsMu.Lock()
					*s.outputs = append(*s.outputs, v)
					s.outputsMu.Unlock()
				}
			case packageStmt, exportStmt:
				return nullValue(), cur, fmt.Errorf("%s is not allowed in block expressions", statementKind(st))
			default:
				return nullValue(), cur, fmt.Errorf("block expressions only support let, var, assignment, import, macro, func, while, for, return, break, continue, and emit statements")
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
		child.scope = cloneScope(scope)
		child.packageName = s.packageName
		child.packageVersion = s.packageVersion
		child.macros = cloneMacroDefs(s.macros)
		v, err := child.eval(expr, child.scope)
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
