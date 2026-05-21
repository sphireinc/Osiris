package osiris

import (
	"context"
	"fmt"
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
	seq        uint64
}

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

func cloneExpr(expr Expr) Expr {
	switch n := expr.(type) {
	case *literalExpr:
		return &literalExpr{value: n.value}
	case *identExpr:
		return &identExpr{name: n.name}
	case *unaryExpr:
		return &unaryExpr{op: n.op, expr: cloneExpr(n.expr)}
	case *binaryExpr:
		return &binaryExpr{op: n.op, left: cloneExpr(n.left), right: cloneExpr(n.right)}
	case *callExpr:
		args := make([]Expr, len(n.args))
		for i := range n.args {
			args[i] = cloneExpr(n.args[i])
		}
		return &callExpr{callee: cloneExpr(n.callee), args: args}
	case *memberExpr:
		return &memberExpr{object: cloneExpr(n.object), name: n.name}
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			items[i] = cloneExpr(n.items[i])
		}
		return &listExpr{items: items}
	case *matchExpr:
		arms := make([]matchArm, len(n.arms))
		for i := range n.arms {
			arms[i] = matchArm{pattern: clonePattern(n.arms[i].pattern), body: cloneExpr(n.arms[i].body)}
		}
		return &matchExpr{subject: cloneExpr(n.subject), arms: arms}
	case *blockExpr:
		stmts := make([]Stmt, len(n.stmts))
		for i := range n.stmts {
			stmts[i] = cloneStmt(n.stmts[i])
		}
		var tail Expr
		if n.tail != nil {
			tail = cloneExpr(n.tail)
		}
		return &blockExpr{stmts: stmts, tail: tail}
	default:
		return expr
	}
}

func cloneStmt(stmt Stmt) Stmt {
	switch s := stmt.(type) {
	case packageStmt:
		return s
	case exportStmt:
		names := append([]string(nil), s.Names...)
		return exportStmt{Names: names}
	case letStmt:
		return letStmt{Name: s.Name, Expr: cloneExpr(s.Expr)}
	case emitStmt:
		return emitStmt{Expr: cloneExpr(s.Expr)}
	case importStmt:
		return s
	case macroStmt:
		params := append([]string(nil), s.Params...)
		return macroStmt{Name: s.Name, Params: params, Body: cloneExpr(s.Body)}
	case funcStmt:
		params := append([]string(nil), s.Params...)
		return funcStmt{Name: s.Name, Params: params, Body: cloneExpr(s.Body)}
	case returnStmt:
		return returnStmt{Expr: cloneExpr(s.Expr)}
	case whileStmt:
		return whileStmt{Cond: cloneExpr(s.Cond), Body: cloneExpr(s.Body)}
	case forStmt:
		return forStmt{Name: s.Name, Iter: cloneExpr(s.Iter), Body: cloneExpr(s.Body)}
	case breakStmt:
		return breakStmt{}
	case continueStmt:
		return continueStmt{}
	default:
		return stmt
	}
}

func clonePattern(pattern Pattern) Pattern {
	switch p := pattern.(type) {
	case wildcardPattern:
		return p
	case bindPattern:
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

func (s *Session) ExpandProgram(prog *Program) (*Program, error) {
	exp := &macroExpander{rt: s.rt, ctx: s.ctx, sourceName: s.sourceName}
	return exp.expandProgram(prog, nil, cloneMacroDefs(s.macros), 0)
}

func (s *Session) expandExpr(expr Expr, env *scope) (Expr, error) {
	exp := &macroExpander{rt: s.rt, ctx: s.ctx, sourceName: s.sourceName}
	return exp.expandExpr(expr, env, cloneMacroDefs(s.macros), 0)
}

func (e *macroExpander) expandProgram(prog *Program, env *scope, defs map[string]*macroDef, depth int) (*Program, error) {
	stmts, _, _, err := e.expandStmtSeq(prog.Statements, env, defs, seqProgram, depth)
	if err != nil {
		return nil, err
	}
	return &Program{SourceName: prog.SourceName, Statements: stmts}, nil
}

type seqMode uint8

const (
	seqProgram seqMode = iota
	seqBlock
)

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
			out = append(out, letStmt{Name: st.Name, Expr: expr})
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: expr, env: curScope},
				},
			}
		case importStmt:
			mod, err := e.rt.loadModule(e.ctx, st.Path, e.sourceName, nil)
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
		case packageStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("package is only allowed at top level")
			}
			out = append(out, st)
		case exportStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("export is only allowed at top level")
			}
			out = append(out, st)
		case emitStmt:
			if mode == seqBlock {
				return nil, nil, nil, fmt.Errorf("emit is not allowed in block expressions")
			}
			expr, err := e.expandExpr(st.Expr, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, emitStmt{Expr: expr})
		case funcStmt:
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, funcStmt{Name: st.Name, Params: append([]string(nil), st.Params...), Body: body})
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
			out = append(out, returnStmt{Expr: expr})
		case whileStmt:
			cond, err := e.expandExpr(st.Cond, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, whileStmt{Cond: cond, Body: body})
		case forStmt:
			iter, err := e.expandExpr(st.Iter, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			body, err := e.expandExpr(st.Body, curScope, curDefs, depth+1)
			if err != nil {
				return nil, nil, nil, err
			}
			out = append(out, forStmt{Name: st.Name, Iter: iter, Body: body})
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

func (e *macroExpander) expandExpr(expr Expr, env *scope, defs map[string]*macroDef, depth int) (Expr, error) {
	if depth > macroExpansionLimit {
		return nil, fmt.Errorf("macro expansion exceeded %d steps", macroExpansionLimit)
	}
	switch n := expr.(type) {
	case *literalExpr:
		return &literalExpr{value: n.value}, nil
	case *identExpr:
		return &identExpr{name: n.name}, nil
	case *unaryExpr:
		inner, err := e.expandExpr(n.expr, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: n.op, expr: inner}, nil
	case *binaryExpr:
		left, err := e.expandExpr(n.left, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		right, err := e.expandExpr(n.right, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &binaryExpr{op: n.op, left: left, right: right}, nil
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
		return &callExpr{callee: callee, args: args}, nil
	case *memberExpr:
		obj, err := e.expandExpr(n.object, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		return &memberExpr{object: obj, name: n.name}, nil
	case *listExpr:
		items := make([]Expr, len(n.items))
		for i := range n.items {
			item, err := e.expandExpr(n.items[i], env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			items[i] = item
		}
		return &listExpr{items: items}, nil
	case *matchExpr:
		subject, err := e.expandExpr(n.subject, env, defs, depth+1)
		if err != nil {
			return nil, err
		}
		arms := make([]matchArm, len(n.arms))
		for i := range n.arms {
			body, err := e.expandExpr(n.arms[i].body, env, defs, depth+1)
			if err != nil {
				return nil, err
			}
			arms[i] = matchArm{pattern: clonePattern(n.arms[i].pattern), body: body}
		}
		return &matchExpr{subject: subject, arms: arms}, nil
	case *blockExpr:
		return e.expandBlockExpr(n, env, defs, depth+1)
	default:
		return nil, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

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

func macroCallName(expr Expr) (string, bool) {
	id, ok := expr.(*identExpr)
	if !ok {
		return "", false
	}
	return id.name, true
}

func (e *macroExpander) expandMacroCallRuntime(def *macroDef, args []Expr, env *scope, defs map[string]*macroDef, depth int) (Expr, error) {
	templ, err := e.expandMacroCallTemplate(def, args, depth+1)
	if err != nil {
		return nil, err
	}
	return e.expandExpr(templ, env, defs, depth+1)
}

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
		return &callExpr{callee: callee, args: callArgs}, nil
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
			body, err := e.expandTemplateExpr(n.arms[i].body, env, defs, params, args, bodyRenames, depth+1)
			if err != nil {
				return nil, err
			}
			arms[i] = matchArm{pattern: pat, body: body}
		}
		return &matchExpr{subject: subject, arms: arms}, nil
	case *blockExpr:
		return e.expandTemplateBlock(n, env, defs, params, args, renames, depth+1)
	default:
		return nil, fmt.Errorf("internal error: unknown expression type %T", expr)
	}
}

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
			out = append(out, letStmt{Name: fresh, Expr: expr})
			curRenames[st.Name] = fresh
			curScope = &scope{
				parent: curScope,
				items: map[string]binding{
					st.Name: {expr: expr, env: curScope},
				},
			}
		case importStmt:
			mod, err := e.rt.loadModule(e.ctx, st.Path, e.sourceName, nil)
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
			out = append(out, funcStmt{Name: fresh, Params: append([]string(nil), st.Params...), Body: body})
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
			out = append(out, returnStmt{Expr: expr})
		case whileStmt:
			cond, err := e.expandTemplateExpr(st.Cond, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, whileStmt{Cond: cond, Body: body})
		case forStmt:
			iter, err := e.expandTemplateExpr(st.Iter, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			body, err := e.expandTemplateExpr(st.Body, curScope, curDefs, params, args, curRenames, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, forStmt{Name: st.Name, Iter: iter, Body: body})
		case breakStmt:
			out = append(out, st)
		case continueStmt:
			out = append(out, st)
		default:
			return nil, fmt.Errorf("block expressions only support let, import, macro, func, while, for, return, break, and continue statements")
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

func (e *macroExpander) freshenPattern(pattern Pattern) (Pattern, map[string]string) {
	renames := make(map[string]string)
	return e.freshenPatternInto(pattern, renames), renames
}

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

func (e *macroExpander) freshName(base string) string {
	e.seq++
	return fmt.Sprintf("__osiris_m%d_%s", e.seq, base)
}
