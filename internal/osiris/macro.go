package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

const macroExpansionLimit = 1024

type macroDef struct {
	name   string
	params []string
	body   Expr
	scope  *scope
	defs   map[string]*macroDef
}

type macroExpander struct {
	rt         *Runtime
	ctx        context.Context
	sourceName string
	stack      []string
	seq        uint64
}

// cloneMacroDefs handles the clone macro defs step in the surrounding package
// and returns map[string]*macroDef.
//
// It returns map[string]*macroDef so callers can continue with the next stage
// of the pipeline without re-deriving the same information.
func cloneMacroDefs(src map[string]*macroDef) map[string]*macroDef {
	if len(src) == 0 {
		return make(map[string]*macroDef)
	}
	dst := make(map[string]*macroDef, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// cloneStringMap handles the clone string map step in the surrounding package
// and returns map[string]string.
//
// It returns map[string]string so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func cloneStringMap(src map[string]string) map[string]string {
	if len(src) == 0 {
		return make(map[string]string)
	}
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}

// cloneExpr handles the clone expr step in the surrounding package and returns
// Expr.
//
// It returns Expr so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func cloneExpr(expr Expr) Expr {
	switch n := expr.(type) {
	case *literalExpr:
		return &literalExpr{value: n.value, Span: n.Span, defaultInteger: n.defaultInteger}
	case *identExpr:
		return &identExpr{name: n.name, Span: n.Span}
	case *unaryExpr:
		return &unaryExpr{op: n.op, expr: cloneExpr(n.expr), Span: n.Span}
	case *binaryExpr:
		return &binaryExpr{op: n.op, left: cloneExpr(n.left), right: cloneExpr(n.right), Span: n.Span}
	case *callExpr:
		args := make([]Expr, len(n.args))
		for i := range n.args {
			args[i] = cloneExpr(n.args[i])
		}
		return &callExpr{callee: cloneExpr(n.callee), args: args, typeArgs: append([]string(nil), n.typeArgs...), Span: n.Span}
	case *memberExpr:
		return &memberExpr{object: cloneExpr(n.object), name: n.name, Span: n.Span}
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			items[i] = cloneExpr(n.items[i])
		}
		return &listExpr{items: items, Span: n.Span}
	case *matchExpr:
		arms := make([]matchArm, len(n.arms))
		for i := range n.arms {
			arms[i] = matchArm{pattern: clonePattern(n.arms[i].pattern), guard: cloneExpr(n.arms[i].guard), body: cloneExpr(n.arms[i].body), isDefault: n.arms[i].isDefault}
		}
		return &matchExpr{subject: cloneExpr(n.subject), arms: arms, keyword: n.keyword, Span: n.Span}
	case *blockExpr:
		stmts := make([]Stmt, len(n.stmts))
		for i := range n.stmts {
			stmts[i] = cloneStmt(n.stmts[i])
		}
		var tail Expr
		if n.tail != nil {
			tail = cloneExpr(n.tail)
		}
		return &blockExpr{stmts: stmts, tail: tail, Span: n.Span}
	case *tryExpr:
		return &tryExpr{body: cloneExpr(n.body), catchName: n.catchName, handler: cloneExpr(n.handler), Span: n.Span}
	case *propagateExpr:
		return &propagateExpr{expr: cloneExpr(n.expr), Span: n.Span}
	default:
		return expr
	}
}

// cloneStmt handles the clone stmt step in the surrounding package and returns
// Stmt.
//
// It returns Stmt so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func cloneStmt(stmt Stmt) Stmt {
	switch s := stmt.(type) {
	case packageStmt:
		return s
	case exportStmt:
		names := append([]string(nil), s.Names...)
		return exportStmt{Names: names}
	case letStmt:
		return letStmt{Name: s.Name, Names: append([]string(nil), s.Names...), Type: s.Type, Expr: cloneExpr(s.Expr), Unwrap: s.Unwrap, Mutable: s.Mutable, Const: s.Const, Span: s.Span, NameSpan: s.NameSpan}
	case assignStmt:
		return assignStmt{Name: s.Name, Expr: cloneExpr(s.Expr), Span: s.Span, NameSpan: s.NameSpan}
	case emitStmt:
		return emitStmt{Expr: cloneExpr(s.Expr), Span: s.Span}
	case importStmt:
		return s
	case macroStmt:
		params := append([]string(nil), s.Params...)
		return macroStmt{Name: s.Name, Params: params, Body: cloneExpr(s.Body), Span: s.Span, NameSpan: s.NameSpan}
	case funcStmt:
		params := append([]string(nil), s.Params...)
		return funcStmt{
			Name:           s.Name,
			TypeParams:     append([]typeParam(nil), s.TypeParams...),
			Params:         params,
			ParamTypes:     append([]string(nil), s.ParamTypes...),
			ParamSpans:     append([]sourceSpan(nil), s.ParamSpans...),
			ReturnType:     s.ReturnType,
			ReturnTypeSpan: s.ReturnTypeSpan,
			Body:           cloneExpr(s.Body),
			Span:           s.Span,
			NameSpan:       s.NameSpan,
		}
	case returnStmt:
		return returnStmt{Expr: cloneExpr(s.Expr), Span: s.Span}
	case whileStmt:
		return whileStmt{Cond: cloneExpr(s.Cond), Body: cloneExpr(s.Body), Span: s.Span}
	case forStmt:
		return forStmt{Name: s.Name, Iter: cloneExpr(s.Iter), Body: cloneExpr(s.Body), Span: s.Span}
	case breakStmt:
		return breakStmt{}
	case continueStmt:
		return continueStmt{}
	default:
		return stmt
	}
}

// clonePattern handles the clone pattern step in the surrounding package and
// returns Pattern.
//
// It returns Pattern so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func clonePattern(pattern Pattern) Pattern {
	switch p := pattern.(type) {
	case wildcardPattern:
		return p
	case bindPattern:
		return p
	case enumPattern:
		items := make([]Pattern, len(p.items))
		for i, item := range p.items {
			items[i] = clonePattern(item)
		}
		p.items = items
		return p
	case orPattern:
		alternatives := make([]Pattern, len(p.alternatives))
		for i, alternative := range p.alternatives {
			alternatives[i] = clonePattern(alternative)
		}
		p.alternatives = alternatives
		return p
	case andPattern:
		p.left = clonePattern(p.left)
		p.right = clonePattern(p.right)
		return p
	case notPattern:
		p.inner = clonePattern(p.inner)
		return p
	case typePattern, relationalPattern:
		return p
	case literalPattern:
		return literalPattern{value: p.value}
	case listPattern:
		items := make([]Pattern, len(p.items))
		for i := range p.items {
			items[i] = clonePattern(p.items[i])
		}
		return listPattern{items: items}
	default:
		return pattern
	}
}

// registerMacro operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (s *Session) registerMacro(name string, params []string, body Expr) error {
	if s.macros == nil {
		s.macros = make(map[string]*macroDef)
	}
	if _, exists := s.macros[name]; exists {
		return fmt.Errorf("macro redeclared as %q", name)
	}
	s.macros[name] = &macroDef{
		name:   name,
		params: append([]string(nil), params...),
		body:   body,
		scope:  s.scope,
		defs:   cloneMacroDefs(s.macros),
	}
	return nil
}

// ExpandProgram operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *Program and an error. Callers should only trust the value when
// the error is nil.
func (s *Session) ExpandProgram(prog *Program) (*Program, error) {
	expanded, _, err := s.expandProgramWithStack(prog, s.moduleStack)
	return expanded, err
}

func (s *Session) expandProgramWithStack(prog *Program, stack []string) (*Program, map[string]*macroDef, error) {
	exp := &macroExpander{
		rt:         s.rt,
		ctx:        s.ctx,
		sourceName: s.sourceName,
		stack:      append([]string(nil), stack...),
	}
	stmts, _, defs, err := exp.expandStmtSeq(prog.Statements, s.scope, cloneMacroDefs(s.macros), seqProgram, 0)
	if err != nil {
		return nil, nil, err
	}
	return &Program{SourceName: prog.SourceName, Statements: stmts}, defs, nil
}

// expandExpr operates on Session values and keeps the session-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (s *Session) expandExpr(expr Expr, env *scope) (Expr, error) {
	exp := &macroExpander{
		rt:         s.rt,
		ctx:        s.ctx,
		sourceName: s.sourceName,
		stack:      append([]string(nil), s.moduleStack...),
	}
	return exp.expandExpr(expr, env, cloneMacroDefs(s.macros), 0)
}

type seqMode uint8

const (
	seqProgram seqMode = iota
	seqBlock
)

// expandStmtSeq operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns []Stmt, *scope, map[string]*macroDef, error so callers can
// continue with the next stage of the pipeline without re-deriving the same
// information.
func (e *macroExpander) expandStmtSeq(stmts []Stmt, env *scope, defs map[string]*macroDef, mode seqMode, depth int) ([]Stmt, *scope, map[string]*macroDef, error) {
	curScope := env
	curDefs := cloneMacroDefs(defs)
	out := make([]Stmt, 0, len(stmts))
	for _, stmt := range stmts {
		switch st := stmt.(type) {
		case macroStmt:
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, nil, nil, nil, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			curDefs[st.Name] = &macroDef{
				name:   st.Name,
				params: append([]string(nil), st.Params...),
				body:   body,
				scope:  curScope,
				defs:   cloneMacroDefs(curDefs),
			}
		case letStmt:
			expr, err := e.expandExpr(st.Expr, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, letStmt{Name: st.Name, Names: append([]string(nil), st.Names...), Type: st.Type, Expr: expr, Unwrap: st.Unwrap, Mutable: st.Mutable, Const: st.Const, Span: st.Span, NameSpan: st.NameSpan})
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: expr, env: curScope},
				},
			}
		case assignStmt:
			expr, err := e.expandExpr(st.Expr, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, assignStmt{Name: st.Name, Expr: expr, Span: st.Span, NameSpan: st.NameSpan})
		case importStmt:
			mod, err := e.rt.loadModule(e.ctx, st.Path, e.sourceName, e.stack)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, st)
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Alias: {expr: &literalExpr{value: mod}, env: curScope},
				},
			}
		case enumStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("enum is only allowed at top level")
			}
			out = append(out, st)
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: &literalExpr{value: enumNamespaceValue(st.Name, st.Cases)}, env: curScope},
				},
			}
		case packageStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("package is only allowed at top level")
			}
			out = append(out, st)
		case structStmt, typeDeclStmt, newtypeStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("type declarations are only allowed at top level")
			}
			out = append(out, st)
			switch decl := st.(type) {
			case structStmt:
				curScope = &scope{parent: curScope, items: map[string]binding{decl.Name: {expr: &literalExpr{value: structTypeValueOf(runtimeStructType(decl))}, env: curScope}}}
			case newtypeStmt:
				typ := &structTypeValue{name: decl.Name, target: decl.Target, newtype: true, fields: []structFieldValue{{name: "value", typeName: decl.Target}}, byName: map[string]int{"value": 0}}
				curScope = &scope{parent: curScope, items: map[string]binding{decl.Name: {expr: &literalExpr{value: structTypeValueOf(typ)}, env: curScope}}}
			case typeDeclStmt:
				curScope = &scope{parent: curScope, items: map[string]binding{decl.Name: {expr: &literalExpr{value: functionValueOf(decl.Name, nil)}, env: curScope}}}
			}
		case traitStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("traits are only allowed at top level")
			}
			out = append(out, st)
		case implStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("implementations are only allowed at top level")
			}
			methods := make([]funcStmt, len(st.Methods))
			for i, method := range st.Methods {
				body, err := e.expandExpr(method.Body, curScope, curDefs, depth+1)
				if err != nil {
					return nil, nil, nil, err
				}
				method.Body = body
				methods[i] = method
			}
			st.Methods = methods
			out = append(out, st)
		case exportStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("export is only allowed at top level")
			}
			out = append(out, st)
		case emitStmt:
			expr, err := e.expandExpr(st.Expr, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, emitStmt{Expr: expr, Span: st.Span})
		case funcStmt:
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, funcStmt{
				Name:           st.Name,
				Receiver:       st.Receiver,
				ReceiverType:   st.ReceiverType,
				TypeParams:     append([]typeParam(nil), st.TypeParams...),
				Params:         append([]string(nil), st.Params...),
				ParamTypes:     append([]string(nil), st.ParamTypes...),
				ParamSpans:     append([]sourceSpan(nil), st.ParamSpans...),
				ReturnType:     st.ReturnType,
				ReturnTypeSpan: st.ReturnTypeSpan,
				Body:           body,
				Span:           st.Span,
				NameSpan:       st.NameSpan,
			})
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: &literalExpr{value: functionValueOf(st.Name, nil)}, env: curScope},
				},
			}
		case returnStmt:
			expr := st.Expr
			if expr != nil {
				var err error
				expr, err = e.expandExpr(expr, curScope, curDefs, depth+1)
				if err != nil {
					return nil, nil, nil, err
				}
			}
			out = append(out, returnStmt{Expr: expr, Span: st.Span})
		case whileStmt:
			cond, err := e.expandExpr(st.Cond, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, whileStmt{Cond: cond, Body: body, Span: st.Span})
		case forStmt:
			iter, err := e.expandExpr(st.Iter, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, forStmt{Name: st.Name, Iter: iter, Body: body, Span: st.Span})
		case breakStmt:
			out = append(out, st)
		case continueStmt:
			out = append(out, st)
		default:
			return nil, nil, nil, fmt.Errorf("internal error: unknown statement type %T", stmt)
		}
	}
	return out, curScope, curDefs, nil
}

// expandExpr operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandExpr(expr Expr, env *scope, defs map[string]*macroDef, depth int) (Expr, error) {
	if depth > macroExpansionLimit {
		return nil, fmt.Errorf("macro expansion exceeded %d steps", macroExpansionLimit)
	}
	switch n := expr.(type) {
	case *literalExpr:
		return &literalExpr{value: n.value, Span: n.Span}, nil
	case *identExpr:
		return &identExpr{name: n.name, Span: n.Span}, nil
	case *unaryExpr:
		inner, err := e.expandExpr(n.expr, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: n.op, expr: inner, Span: n.Span}, nil
	case *binaryExpr:
		left, err := e.expandExpr(n.left, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := e.expandExpr(n.right, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: n.op, left: left, right: right, Span: n.Span}, nil
	case *callExpr:
		if name, ok := macroCallName(n.callee); ok {
			if def, exists := defs[name]; exists {
				return e.expandMacroCallRuntime(def, n.args, env, defs, depth+1)
			}
		}
		callee, err := e.expandExpr(n.callee, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		args := make([]Expr, len(n.args))
		for i := range n.args {
			arg, err := e.expandExpr(n.args[i], env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			args[i] = arg
		}
		return &callExpr{callee: callee, args: args, typeArgs: append([]string(nil), n.typeArgs...), Span: n.Span}, nil
	case *memberExpr:
		obj, err := e.expandExpr(n.object, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &memberExpr{object: obj, name: n.name, Span: n.Span, NameSpan: n.NameSpan}, nil
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			item, err := e.expandExpr(n.items[i], env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return &listExpr{items: items, Span: n.Span}, nil
	case *tupleExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			item, err := e.expandExpr(n.items[i], env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return &tupleExpr{items: items, Span: n.Span}, nil
	case *structExpr:
		fields := make([]structFieldExpr, len(n.Fields))
		for i, field := range n.Fields {
			value, err := e.expandExpr(field.Expr, env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			fields[i] = structFieldExpr{Name: field.Name, Expr: value, Span: field.Span}
		}
		return &structExpr{TypeName: n.TypeName, Fields: fields, Span: n.Span}, nil
	case *structUpdateExpr:
		base, err := e.expandExpr(n.Base, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		fields := make([]structFieldExpr, len(n.Fields))
		for i, field := range n.Fields {
			value, err := e.expandExpr(field.Expr, env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			fields[i] = structFieldExpr{Name: field.Name, Expr: value, Span: field.Span}
		}
		return &structUpdateExpr{Base: base, Fields: fields, Span: n.Span}, nil
	case *indexExpr:
		object, err := e.expandExpr(n.Object, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		index, err := e.expandExpr(n.Index, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &indexExpr{Object: object, Index: index, Span: n.Span}, nil
	case *matchExpr:
		subject, err := e.expandExpr(n.subject, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		arms := make([]matchArm, len(n.arms))
		for i := range n.arms {
			if err := validatePatternBindings(n.arms[i].pattern, e.sourceName); err != nil {
				return nil, err
			}
			if err := validateEnumPattern(n.arms[i].pattern, env, e.sourceName); err != nil {
				return nil, err
			}
			var guard Expr
			if n.arms[i].guard != nil {
				guard, err = e.expandExpr(n.arms[i].guard, env, defs, depth+1)
				if err != nil {
					return nil, err
				}
			}
			body, err := e.expandExpr(n.arms[i].body, env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			arms[i] = matchArm{pattern: clonePattern(n.arms[i].pattern), guard: guard, body: body, isDefault: n.arms[i].isDefault}
		}
		if err := validateEnumMatchExhaustiveness(subject, arms, env, e.sourceName); err != nil {
			return nil, err
		}
		return &matchExpr{subject: subject, arms: arms, Span: n.Span, keyword: n.keyword}, nil
	case *blockExpr:
		return e.expandBlockExpr(n, env, defs, depth+1)
	case *tryExpr:
		body, err := e.expandExpr(n.body, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		handler, err := e.expandExpr(n.handler, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &tryExpr{body: body, catchName: n.catchName, handler: handler}, nil
	case *propagateExpr:
		expr, err := e.expandExpr(n.expr, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &propagateExpr{expr: expr, Span: n.Span}, nil
	default:
		return nil, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// validateEnumPattern checks qualified enum cases before execution. Pattern
// names are deliberately lexical, so a misspelt type or case cannot quietly
// become an arm that never matches.
func validateEnumPattern(pattern Pattern, env *scope, source string) error {
	switch p := pattern.(type) {
	case enumPattern:
		if env == nil {
			return enumPatternError(source, p.Span, "undefined enum %q in pattern", p.typeName)
		}
		binding, ok := env.lookup(p.typeName)
		if !ok {
			return enumPatternError(source, p.Span, "undefined enum %q in pattern", p.typeName)
		}
		literal, ok := binding.expr.(*literalExpr)
		if !ok || literal.value.kind != kindModule || literal.value.mod == nil {
			return enumPatternError(source, p.Span, "%q is not an enum in pattern", p.typeName)
		}
		candidate, ok := literal.value.mod.members[p.caseName]
		caseDef, defined := literal.value.mod.enumCases[p.caseName]
		if !ok || !defined || (candidate.kind != kindEnum && candidate.kind != kindFunction) {
			return enumPatternError(source, p.Span, "enum %s has no case %q", p.typeName, p.caseName)
		}
		if len(p.items) != caseDef.Arity() {
			return enumPatternError(source, p.Span, "enum case %s.%s pattern expects %d values", p.typeName, p.caseName, caseDef.Arity())
		}
		for _, item := range p.items {
			if err := validateEnumPattern(item, env, source); err != nil {
				return err
			}
		}
	case listPattern:
		for _, item := range p.items {
			if err := validateEnumPattern(item, env, source); err != nil {
				return err
			}
		}
	case orPattern:
		for _, alternative := range p.alternatives {
			if err := validateEnumPattern(alternative, env, source); err != nil {
				return err
			}
		}
	case andPattern:
		if err := validateEnumPattern(p.left, env, source); err != nil {
			return err
		}
		return validateEnumPattern(p.right, env, source)
	case notPattern:
		return validateEnumPattern(p.inner, env, source)
	}
	return nil
}

func validatePatternBindings(pattern Pattern, source string) error {
	switch p := pattern.(type) {
	case orPattern:
		if len(p.alternatives) == 0 {
			return nil
		}
		want := patternBindingSet(p.alternatives[0])
		for _, alternative := range p.alternatives[1:] {
			if !samePatternBindingSet(want, patternBindingSet(alternative)) {
				return enumPatternError(source, p.Span, "all alternatives in a grouped pattern must bind the same names")
			}
		}
		for _, alternative := range p.alternatives {
			if err := validatePatternBindings(alternative, source); err != nil {
				return err
			}
		}
	case enumPattern:
		for _, item := range p.items {
			if err := validatePatternBindings(item, source); err != nil {
				return err
			}
		}
	case listPattern:
		for _, item := range p.items {
			if err := validatePatternBindings(item, source); err != nil {
				return err
			}
		}
	case andPattern:
		if err := validatePatternBindings(p.left, source); err != nil {
			return err
		}
		return validatePatternBindings(p.right, source)
	case notPattern:
		if len(patternBindingSet(p.inner)) != 0 {
			return enumPatternError(source, p.Span, "not pattern cannot bind names")
		}
		return validatePatternBindings(p.inner, source)
	}
	return nil
}

func patternBindingSet(pattern Pattern) map[string]struct{} {
	set := make(map[string]struct{})
	var visit func(Pattern)
	visit = func(item Pattern) {
		switch p := item.(type) {
		case bindPattern:
			set[p.name] = struct{}{}
		case enumPattern:
			for _, child := range p.items {
				visit(child)
			}
		case listPattern:
			for _, child := range p.items {
				visit(child)
			}
		case orPattern:
			for _, child := range p.alternatives {
				visit(child)
			}
		case andPattern:
			visit(p.left)
			visit(p.right)
		case typePattern:
			if p.binding != "" && p.binding != "_" {
				set[p.binding] = struct{}{}
			}
		case notPattern:
			visit(p.inner)
		}
	}
	visit(pattern)
	return set
}

func samePatternBindingSet(left, right map[string]struct{}) bool {
	if len(left) != len(right) {
		return false
	}
	for name := range left {
		if _, ok := right[name]; !ok {
			return false
		}
	}
	return true
}

func validateEnumMatchExhaustiveness(subject Expr, arms []matchArm, env *scope, source string) error {
	enum, ok := staticEnumValue(subject, env)
	if !ok || enum.enum == nil {
		return nil
	}
	namespace, ok := staticEnumNamespace(enum.enum.typeName, env)
	if !ok {
		return nil
	}
	covered := make(map[string]struct{}, len(arms))
	catchAll := false
	for _, arm := range arms {
		if catchAll {
			return enumPatternError(source, matchArmSpan(arm), "unreachable match arm after an unguarded catch-all")
		}
		if arm.guard != nil {
			continue
		}
		switch arm.pattern.(type) {
		case wildcardPattern, bindPattern:
			catchAll = true
			continue
		}
		armCases := enumPatternCases(arm.pattern)
		if len(armCases) == 0 {
			continue
		}
		newCase := false
		for _, pattern := range armCases {
			if pattern.typeName == enum.enum.typeName {
				if _, exists := covered[pattern.caseName]; !exists {
					newCase = true
				}
				covered[pattern.caseName] = struct{}{}
			}
		}
		if !newCase {
			return enumPatternError(source, matchArmSpan(arm), "unreachable match arm; its enum cases are already covered")
		}
	}
	if catchAll {
		return nil
	}
	missing := make([]string, 0, len(namespace.mod.enumCases))
	for name := range namespace.mod.enumCases {
		if _, found := covered[name]; !found {
			missing = append(missing, name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return enumPatternError(source, exprSpan(subject), "non-exhaustive match for enum %s; missing cases: %s", enum.enum.typeName, strings.Join(missing, ", "))
}

func matchArmSpan(arm matchArm) sourceSpan {
	if span := patternSpan(arm.pattern); span.start.line != 0 || span.start.col != 0 {
		return span
	}
	return exprSpan(arm.body)
}

func enumPatternCases(pattern Pattern) []enumPattern {
	switch p := pattern.(type) {
	case enumPattern:
		return []enumPattern{p}
	case orPattern:
		var out []enumPattern
		for _, alternative := range p.alternatives {
			out = append(out, enumPatternCases(alternative)...)
		}
		return out
	default:
		return nil
	}
}

func enumPatternError(source string, span sourceSpan, format string, args ...any) error {
	return newOsirisError(ErrorKindType, fmt.Sprintf(format, args...)).withSource(source, span)
}

func staticEnumValue(expr Expr, env *scope) (Value, bool) {
	switch node := expr.(type) {
	case *identExpr:
		if env == nil {
			return Value{}, false
		}
		binding, ok := env.lookup(node.name)
		if !ok {
			return Value{}, false
		}
		return staticEnumValue(binding.expr, binding.env)
	case *memberExpr:
		namespace, ok := staticEnumNamespaceExpr(node.object, env)
		if !ok || namespace.mod == nil {
			return Value{}, false
		}
		value, ok := namespace.mod.members[node.name]
		return value, ok && value.kind == kindEnum
	case *callExpr:
		member, ok := node.callee.(*memberExpr)
		if !ok {
			return Value{}, false
		}
		namespace, ok := staticEnumNamespaceExpr(member.object, env)
		if !ok || namespace.mod == nil {
			return Value{}, false
		}
		caseDef, ok := namespace.mod.enumCases[member.name]
		if !ok || caseDef.Arity() != len(node.args) {
			return Value{}, false
		}
		return enumCaseValue(namespace.mod.name, member.name), true
	case *literalExpr:
		return node.value, node.value.kind == kindEnum
	default:
		return Value{}, false
	}
}

func staticEnumNamespace(typeName string, env *scope) (Value, bool) {
	if env == nil {
		return Value{}, false
	}
	binding, ok := env.lookup(typeName)
	if !ok {
		return Value{}, false
	}
	literal, ok := binding.expr.(*literalExpr)
	if !ok || literal.value.kind != kindModule || literal.value.mod == nil {
		return Value{}, false
	}
	return literal.value, true
}

func staticEnumNamespaceExpr(expr Expr, env *scope) (Value, bool) {
	ident, ok := expr.(*identExpr)
	if !ok {
		return Value{}, false
	}
	return staticEnumNamespace(ident.name, env)
}

// expandBlockExpr operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandBlockExpr(n *blockExpr, env *scope, defs map[string]*macroDef, depth int) (Expr, error) {
	stmts, curScope, curDefs, err := e.expandStmtSeq(n.stmts, env, defs, seqBlock, depth)
	if err != nil {
		return nil, err
	}
	var tail Expr
	if n.tail != nil {
		tail, err = e.expandExpr(n.tail, curScope, curDefs, depth+1)
		if err != nil {
			return nil, err
		}
	}
	return &blockExpr{stmts: stmts, tail: tail}, nil
}

// macroCallName handles the macro call name step in the surrounding package
// and returns string, bool.
//
// It returns string, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func macroCallName(expr Expr) (string, bool) {
	id, ok := expr.(*identExpr)
	if !ok {
		return "", false
	}
	return id.name, true
}

// expandMacroCallRuntime operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandMacroCallRuntime(def *macroDef, args []Expr, env *scope, defs map[string]*macroDef, depth int) (Expr, error) {
	templ, err := e.expandMacroCallTemplate(def, args, depth+1)
	if err != nil {
		return nil, err
	}
	return e.expandExpr(templ, env, defs, depth+1)
}

// expandMacroCallTemplate operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandMacroCallTemplate(def *macroDef, args []Expr, depth int) (Expr, error) {
	if len(args) != len(def.params) {
		return nil, fmt.Errorf("macro %s expects %d arguments", def.name, len(def.params))
	}
	params := make(map[string]int, len(def.params))
	for i, name := range def.params {
		params[name] = i
	}
	return e.expandTemplateExpr(def.body, def.scope, def.defs, params, args, nil, depth+1)
}

// expandTemplateExpr operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandTemplateExpr(expr Expr, env *scope, defs map[string]*macroDef, params map[string]int, args []Expr, renames map[string]string, depth int) (Expr, error) {
	if depth > macroExpansionLimit {
		return nil, fmt.Errorf("macro expansion exceeded %d steps", macroExpansionLimit)
	}
	switch n := expr.(type) {
	case *literalExpr:
		return &literalExpr{value: n.value}, nil
	case *identExpr:
		if idx, ok := params[n.name]; ok {
			return cloneExpr(args[idx]), nil
		}
		if renames != nil {
			if next, ok := renames[n.name]; ok {
				return &identExpr{name: next}, nil
			}
		}
		if b, ok := env.lookup(n.name); ok {
			return cloneExpr(b.expr), nil
		}
		if fn, ok := e.rt.Builtins[n.name]; ok {
			return &literalExpr{value: functionValueOf(n.name, fn)}, nil
		}
		return &identExpr{name: n.name}, nil
	case *unaryExpr:
		inner, err := e.expandTemplateExpr(n.expr, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: n.op, expr: inner}, nil
	case *binaryExpr:
		left, err := e.expandTemplateExpr(n.left, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := e.expandTemplateExpr(n.right, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: n.op, left: left, right: right}, nil
	case *callExpr:
		if name, ok := macroCallName(n.callee); ok {
			if def, exists := defs[name]; exists {
				callArgs := make([]Expr, len(n.args))
				for i := range n.args {
					arg, err := e.expandTemplateExpr(n.args[i], env, defs, params, args, renames, depth+1)
					if err != nil {
						return nil, err
					}
					callArgs[i] = arg
				}
				return e.expandMacroCallTemplate(def, callArgs, depth+1)
			}
		}
		callee, err := e.expandTemplateExpr(n.callee, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		callArgs := make([]Expr, len(n.args))
		for i := range n.args {
			arg, err := e.expandTemplateExpr(n.args[i], env, defs, params, args, renames, depth+1)
			if err != nil {
				return nil, err
			}
			callArgs[i] = arg
		}
		return &callExpr{callee: callee, args: callArgs, typeArgs: append([]string(nil), n.typeArgs...), Span: n.Span}, nil
	case *memberExpr:
		obj, err := e.expandTemplateExpr(n.object, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		return &memberExpr{object: obj, name: n.name}, nil
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			item, err := e.expandTemplateExpr(n.items[i], env, defs, params, args, renames, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return &listExpr{items: items}, nil
	case *matchExpr:
		subject, err := e.expandTemplateExpr(n.subject, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		arms := make([]matchArm, len(n.arms))
		for i := range n.arms {
			pat, added := e.freshenPattern(n.arms[i].pattern)
			bodyRenames := cloneStringMap(renames)
			for k, v := range added {
				bodyRenames[k] = v
			}
			var guard Expr
			if n.arms[i].guard != nil {
				guard, err = e.expandTemplateExpr(n.arms[i].guard, env, defs, params, args, bodyRenames, depth+1)
				if err != nil {
					return nil, err
				}
			}
			body, err := e.expandTemplateExpr(n.arms[i].body, env, defs, params, args, bodyRenames, depth+1)
			if err != nil {
				return nil, err
			}
			arms[i] = matchArm{pattern: pat, guard: guard, body: body, isDefault: n.arms[i].isDefault}
		}
		return &matchExpr{subject: subject, arms: arms}, nil
	case *blockExpr:
		return e.expandTemplateBlock(n, env, defs, params, args, renames, depth+1)
	case *tryExpr:
		body, err := e.expandTemplateExpr(n.body, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		handler, err := e.expandTemplateExpr(n.handler, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		return &tryExpr{body: body, catchName: n.catchName, handler: handler}, nil
	case *propagateExpr:
		expr, err := e.expandTemplateExpr(n.expr, env, defs, params, args, renames, depth+1)
		if err != nil {
			return nil, err
		}
		return &propagateExpr{expr: expr, Span: n.Span}, nil
	default:
		return nil, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

// expandTemplateBlock operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (e *macroExpander) expandTemplateBlock(n *blockExpr, env *scope, defs map[string]*macroDef, params map[string]int, args []Expr, renames map[string]string, depth int) (Expr, error) {
	curScope := env
	curDefs := cloneMacroDefs(defs)
	curRenames := cloneStringMap(renames)
	out := make([]Stmt, 0, len(n.stmts))
	for _, stmt := range n.stmts {
		switch st := stmt.(type) {
		case macroStmt:
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			curDefs[st.Name] = &macroDef{
				name:   st.Name,
				params: append([]string(nil), st.Params...),
				body:   body,
				scope:  curScope,
				defs:   cloneMacroDefs(curDefs),
			}
		case letStmt:
			expr, err := e.expandTemplateExpr(st.Expr, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			fresh := e.freshName(st.Name)
			out = append(out, letStmt{Name: fresh, Type: st.Type, Expr: expr, Unwrap: st.Unwrap, Mutable: st.Mutable, Const: st.Const, Span: st.Span, NameSpan: st.NameSpan})
			curRenames[st.Name] = fresh
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: expr, env: curScope},
				},
			}
		case assignStmt:
			expr, err := e.expandTemplateExpr(st.Expr, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			name := st.Name
			if renamed, ok := curRenames[name]; ok {
				name = renamed
			}
			out = append(out, assignStmt{Name: name, Expr: expr, Span: st.Span, NameSpan: st.NameSpan})
		case importStmt:
			mod, err := e.rt.loadModule(e.ctx, st.Path, e.sourceName, e.stack)
			if err != nil {
				return nil, err
			}
			out = append(out, st)
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Alias: {expr: &literalExpr{value: mod}, env: curScope},
				},
			}
		case funcStmt:
			fresh := e.freshName(st.Name)
			nextRenames := cloneStringMap(curRenames)
			nextRenames[st.Name] = fresh
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, nextRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, funcStmt{
				Name:           fresh,
				Receiver:       st.Receiver,
				ReceiverType:   st.ReceiverType,
				TypeParams:     append([]typeParam(nil), st.TypeParams...),
				Params:         append([]string(nil), st.Params...),
				ParamTypes:     append([]string(nil), st.ParamTypes...),
				ParamSpans:     append([]sourceSpan(nil), st.ParamSpans...),
				ReturnType:     st.ReturnType,
				ReturnTypeSpan: st.ReturnTypeSpan,
				Body:           body,
				Span:           st.Span,
				NameSpan:       st.NameSpan,
			})
			curRenames = nextRenames
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					fresh: {expr: &literalExpr{value: functionValueOf(fresh, nil)}, env: curScope},
				},
			}
		case returnStmt:
			expr := st.Expr
			if expr != nil {
				var err error
				expr, err = e.expandTemplateExpr(expr, curScope, curDefs, params, args, curRenames, depth+1)
				if err != nil {
					return nil, err
				}
			}
			out = append(out, returnStmt{Expr: expr, Span: st.Span})
		case whileStmt:
			cond, err := e.expandTemplateExpr(st.Cond, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, whileStmt{Cond: cond, Body: body, Span: st.Span})
		case forStmt:
			iter, err := e.expandTemplateExpr(st.Iter, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, forStmt{Name: st.Name, Iter: iter, Body: body, Span: st.Span})
		case emitStmt:
			expr, err := e.expandTemplateExpr(st.Expr, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, emitStmt{Expr: expr, Span: st.Span})
		case breakStmt:
			out = append(out, st)
		case continueStmt:
			out = append(out, st)
		default:
			return nil, fmt.Errorf("block expressions only support let, import, macro, func, while, for, return, break, continue, and emit statements")
		}
	}
	var tail Expr
	var err error
	if n.tail != nil {
		tail, err = e.expandTemplateExpr(n.tail, curScope, curDefs, params, args, curRenames, depth+1)
		if err != nil {
			return nil, err
		}
	}
	return &blockExpr{stmts: out, tail: tail}, nil
}

// freshenPattern operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Pattern, map[string]string so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func (e *macroExpander) freshenPattern(pattern Pattern) (Pattern, map[string]string) {
	renames := make(map[string]string)
	return e.freshenPatternInto(pattern, renames), renames
}

// freshenPatternInto operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns Pattern so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (e *macroExpander) freshenPatternInto(pattern Pattern, renames map[string]string) Pattern {
	switch p := pattern.(type) {
	case wildcardPattern:
		return p
	case bindPattern:
		if next, ok := renames[p.name]; ok {
			return bindPattern{name: next}
		}
		fresh := e.freshName(p.name)
		renames[p.name] = fresh
		return bindPattern{name: fresh}
	case enumPattern:
		items := make([]Pattern, len(p.items))
		for i, item := range p.items {
			items[i] = e.freshenPatternInto(item, renames)
		}
		p.items = items
		return p
	case orPattern:
		alternatives := make([]Pattern, len(p.alternatives))
		for i, alternative := range p.alternatives {
			alternatives[i] = e.freshenPatternInto(alternative, renames)
		}
		p.alternatives = alternatives
		return p
	case andPattern:
		p.left = e.freshenPatternInto(p.left, renames)
		p.right = e.freshenPatternInto(p.right, renames)
		return p
	case notPattern:
		p.inner = e.freshenPatternInto(p.inner, renames)
		return p
	case typePattern:
		if p.binding == "" || p.binding == "_" {
			return p
		}
		if next, ok := renames[p.binding]; ok {
			p.binding = next
			return p
		}
		fresh := e.freshName(p.binding)
		renames[p.binding] = fresh
		p.binding = fresh
		return p
	case relationalPattern:
		return p
	case literalPattern:
		return literalPattern{value: p.value}
	case listPattern:
		items := make([]Pattern, len(p.items))
		for i := range p.items {
			items[i] = e.freshenPatternInto(p.items[i], renames)
		}
		return listPattern{items: items}
	default:
		return pattern
	}
}

// freshName operates on macroExpander values and keeps the
// macroexpander-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (e *macroExpander) freshName(base string) string {
	e.seq++
	return fmt.Sprintf("__osiris_m%d_%s", e.seq, base)
}
