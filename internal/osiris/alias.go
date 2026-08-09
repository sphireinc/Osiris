package osiris

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type AliasProgram struct {
	Program   *Program
	Bindings  []AliasBinding
	Functions []AliasFunction
	Classes   []*AliasClass
}

type AliasClass struct {
	Label   string
	Kind    string
	Origin  string
	Fresh   bool
	Members []string
}

type AliasBinding struct {
	Name    string
	Scope   string
	Kind    string
	Class   string
	Status  string
	Origin  string
	Escapes bool
}

type AliasFunction struct {
	Name            string
	SourceName      string
	Params          []string
	Result          string
	Captures        []string
	CaptureBindings []string
	Escapes         []string
}

// String operates on AliasProgram values and keeps the aliasprogram-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (p *AliasProgram) String() string {
	return FormatAliasProgram(p)
}

// AnalyzeAlias operates on Runtime values and keeps the runtime-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *AliasProgram and an error. Callers should only trust the value
// when the error is nil.
func (rt *Runtime) AnalyzeAlias(ctx context.Context, prog *Program) (*AliasProgram, error) {
	_ = ctx
	if prog == nil {
		return nil, fmt.Errorf("cannot analyze aliases for nil program")
	}
	return analyzeAliasProgram(prog), nil
}

// FormatAliasProgram formats the alias program into a stable textual
// rendering. The implementation is intentionally deterministic so
// round-tripping and diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func FormatAliasProgram(prog *AliasProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("alias ")
	if prog.Program != nil && prog.Program.SourceName != "" {
		b.WriteString(prog.Program.SourceName)
	} else {
		b.WriteString("<unknown>")
	}
	b.WriteByte('\n')
	if prog.Program != nil {
		if p, ok := firstPackage(prog.Program); ok {
			b.WriteString("package ")
			b.WriteString(p.Name)
			if p.Version != "" {
				b.WriteByte(' ')
				b.WriteString(p.Version)
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Classes) > 0 {
		b.WriteByte('\n')
		b.WriteString("classes\n")
		for _, cls := range prog.Classes {
			b.WriteString("  ")
			b.WriteString(cls.Label)
			if cls.Kind != "" {
				b.WriteString(" [")
				b.WriteString(cls.Kind)
				b.WriteByte(']')
			}
			if cls.Fresh {
				b.WriteString(" fresh")
			}
			if cls.Origin != "" {
				b.WriteString(" origin=")
				b.WriteString(cls.Origin)
			}
			if len(cls.Members) > 0 {
				b.WriteString(" members [")
				b.WriteString(strings.Join(cls.Members, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Bindings) > 0 {
		b.WriteByte('\n')
		b.WriteString("bindings\n")
		for _, bind := range prog.Bindings {
			b.WriteString("  ")
			if bind.Scope != "" {
				b.WriteString(bind.Scope)
				b.WriteByte('.')
			}
			b.WriteString(bind.Name)
			b.WriteString(": ")
			if bind.Kind != "" {
				b.WriteString(bind.Kind)
			} else {
				b.WriteString("value")
			}
			switch bind.Status {
			case "fresh":
				b.WriteString(" fresh")
			case "shared":
				b.WriteString(" shared with ")
				b.WriteString(bind.Class)
			default:
				b.WriteString(" unknown")
			}
			if bind.Class != "" {
				b.WriteString(" class=")
				b.WriteString(bind.Class)
			}
			if bind.Origin != "" {
				b.WriteString(" origin=")
				b.WriteString(bind.Origin)
			}
			if bind.Escapes {
				b.WriteString(" escapes")
			}
			b.WriteByte('\n')
		}
	}
	if len(prog.Functions) > 0 {
		b.WriteByte('\n')
		b.WriteString("functions\n")
		for _, fn := range prog.Functions {
			b.WriteString("  fn ")
			b.WriteString(fn.Name)
			if fn.SourceName != "" && fn.SourceName != fn.Name {
				b.WriteString(" (")
				b.WriteString(fn.SourceName)
				b.WriteByte(')')
			}
			hasMeta := len(fn.Params) > 0 || fn.Result != "" || len(fn.Captures) > 0 || len(fn.Escapes) > 0
			if hasMeta {
				b.WriteByte(':')
			}
			if len(fn.Params) > 0 {
				b.WriteString(" params [")
				b.WriteString(strings.Join(fn.Params, ", "))
				b.WriteByte(']')
			}
			if fn.Result != "" {
				b.WriteString(" result=")
				b.WriteString(fn.Result)
			}
			if len(fn.Captures) > 0 {
				b.WriteString(" captures [")
				b.WriteString(strings.Join(fn.Captures, ", "))
				b.WriteByte(']')
			}
			if len(fn.Escapes) > 0 {
				b.WriteString(" escapes [")
				b.WriteString(strings.Join(fn.Escapes, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type aliasBindingState struct {
	binding *AliasBinding
	info    aliasInfo
	owner   *aliasFunctionState
}

type aliasScope struct {
	parent   *aliasScope
	items    map[string]*aliasBindingState
	fn       *aliasFunctionState
	name     string
	report   bool
	blockSeq int
}

type aliasFunctionState struct {
	report            *AliasFunction
	scope             *aliasScope
	capturesSeen      map[string]struct{}
	paramByClass      map[*AliasClass]string
	captureByClass    map[*AliasClass]string
	captureInfoByName map[string]aliasInfo
	resultMode        string
	resultName        string
	resultKind        string
}

type aliasInfo struct {
	known    bool
	kind     string
	class    *AliasClass
	origin   string
	function *aliasFunctionState
	builtin  string
}

// analyzeAliasProgram handles the analyze alias program step in the
// surrounding package and returns *AliasProgram.
//
// It returns *AliasProgram so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func analyzeAliasProgram(prog *Program) *AliasProgram {
	out := &AliasProgram{Program: prog}
	root := &aliasScope{
		items:  make(map[string]*aliasBindingState),
		name:   "main",
		report: true,
	}
	aliasSeq(prog.Statements, root, out)
	sort.Slice(out.Bindings, func(i, j int) bool {
		if out.Bindings[i].Scope == out.Bindings[j].Scope {
			return out.Bindings[i].Name < out.Bindings[j].Name
		}
		return out.Bindings[i].Scope < out.Bindings[j].Scope
	})
	sort.Slice(out.Functions, func(i, j int) bool {
		return out.Functions[i].Name < out.Functions[j].Name
	})
	sort.Slice(out.Classes, func(i, j int) bool {
		return out.Classes[i].Label < out.Classes[j].Label
	})
	applyAliasCaptureEscapes(out)
	return out
}

// aliasSeq handles the alias seq step in the surrounding package and returns
// aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasSeq(stmts []Stmt, scope *aliasScope, out *AliasProgram) aliasInfo {
	var last aliasInfo
	for _, stmt := range stmts {
		last = aliasStmt(stmt, scope, out)
	}
	return last
}

// aliasStmt handles the alias stmt step in the surrounding package and returns
// aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasStmt(stmt Stmt, scope *aliasScope, out *AliasProgram) aliasInfo {
	switch st := stmt.(type) {
	case letStmt:
		info := aliasExpr(st.Expr, scope, out)
		scope.bindLocal(st.Name, info, out, true)
		return info
	case assignStmt:
		return aliasExpr(st.Expr, scope, out)
	case importStmt:
		info := freshAliasInfo("module", "import "+st.Path)
		scope.bindLocal(st.Alias, info, out, true)
		return info
	case funcStmt:
		return aliasFunctionStmt(st, scope, out)
	case emitStmt:
		return aliasExpr(st.Expr, scope, out)
	case returnStmt:
		if st.Expr == nil {
			return freshAliasInfo("scalar", "return")
		}
		return aliasExpr(st.Expr, scope, out)
	case whileStmt:
		_ = aliasExpr(st.Cond, scope, out)
		_ = aliasExpr(st.Body, scope.childForLoop(), out)
		return unknownAliasInfo("control")
	case forStmt:
		_ = aliasExpr(st.Iter, scope, out)
		child := scope.childForLoop()
		child.bindLocal(st.Name, unknownAliasInfo("value"), out, false)
		_ = aliasExpr(st.Body, child, out)
		return unknownAliasInfo("control")
	case breakStmt, continueStmt, packageStmt, exportStmt, macroStmt:
		return unknownAliasInfo("control")
	default:
		return unknownAliasInfo("value")
	}
}

// aliasFunctionStmt handles the alias function stmt step in the surrounding
// package and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasFunctionStmt(st funcStmt, scope *aliasScope, out *AliasProgram) aliasInfo {
	fnState := &aliasFunctionState{
		report: &AliasFunction{
			Name:       st.Name,
			SourceName: st.Name,
			Params:     append([]string(nil), st.Params...),
		},
		capturesSeen:      make(map[string]struct{}),
		paramByClass:      make(map[*AliasClass]string),
		captureByClass:    make(map[*AliasClass]string),
		captureInfoByName: make(map[string]aliasInfo),
	}
	fnInfo := freshAliasInfo("function", "function "+st.Name)
	fnInfo.function = fnState

	scope.bindLocal(st.Name, fnInfo, out, scope.report)

	child := &aliasScope{
		parent: scope,
		items:  make(map[string]*aliasBindingState),
		fn:     fnState,
		name:   st.Name,
		report: true,
	}
	fnState.scope = child
	child.bindLocal(st.Name, fnInfo, out, false)
	for _, param := range st.Params {
		pinfo := freshAliasInfo("value", "parameter "+param)
		pstate := child.bindLocal(param, pinfo, out, false)
		if pstate.info.class != nil {
			fnState.paramByClass[pstate.info.class] = param
		}
	}
	result := aliasExpr(st.Body, child, out)
	fnState.resultKind = result.kind
	fnState.classifyResult(result)
	fnState.report.Result = fnState.formatResult()
	if scope.report {
		out.Functions = append(out.Functions, *fnState.report)
	}
	return fnInfo
}

// classifyResult operates on aliasFunctionState values and keeps the
// aliasfunctionstate-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (fn *aliasFunctionState) classifyResult(info aliasInfo) {
	if info.class == nil {
		fn.resultMode = "unknown"
		return
	}
	if name, ok := fn.paramByClass[info.class]; ok {
		fn.resultMode = "param"
		fn.resultName = name
		return
	}
	if name, ok := fn.captureByClass[info.class]; ok {
		fn.resultMode = "capture"
		fn.resultName = name
		return
	}
	if info.class.Fresh {
		fn.resultMode = "fresh"
		return
	}
	fn.resultMode = "unknown"
}

// formatResult operates on aliasFunctionState values and keeps the
// aliasfunctionstate-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (fn *aliasFunctionState) formatResult() string {
	switch fn.resultMode {
	case "param":
		return "aliases param " + fn.resultName
	case "capture":
		return "aliases capture " + fn.resultName
	case "fresh":
		if fn.resultKind != "" && fn.resultKind != "value" {
			return "fresh " + fn.resultKind
		}
		return "fresh"
	default:
		return "unknown"
	}
}

// childForBlock operates on aliasScope values and keeps the
// aliasscope-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns *aliasScope so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *aliasScope) childForBlock() *aliasScope {
	label := fmt.Sprintf("%s.b%d", s.name, s.blockSeq)
	s.blockSeq++
	return &aliasScope{
		parent: s,
		items:  make(map[string]*aliasBindingState),
		fn:     s.fn,
		name:   label,
		report: s.report,
	}
}

// childForLoop operates on aliasScope values and keeps the aliasscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *aliasScope so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *aliasScope) childForLoop() *aliasScope {
	child := s.childForBlock()
	child.report = false
	return child
}

// bindLocal operates on aliasScope values and keeps the aliasscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *aliasBindingState so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func (s *aliasScope) bindLocal(name string, info aliasInfo, out *AliasProgram, report bool) *aliasBindingState {
	fullName := s.fullName(name)
	if info.class != nil {
		if info.class.Label == "" || strings.HasPrefix(info.class.Label, "tmp") {
			info.class.Label = fullName
		}
		if report {
			info.class.addMember(fullName)
			if info.class.Origin == "" {
				info.class.Origin = info.origin
			}
			registerAliasClass(out, info.class)
		}
	}
	bind := &AliasBinding{
		Name:   name,
		Scope:  s.name,
		Kind:   info.kind,
		Class:  classLabel(info.class),
		Origin: info.origin,
	}
	switch {
	case info.class == nil:
		bind.Status = "unknown"
	case info.class.Label == fullName:
		bind.Status = "fresh"
	default:
		bind.Status = "shared"
	}
	state := &aliasBindingState{binding: bind, info: info, owner: s.fn}
	if s.items == nil {
		s.items = make(map[string]*aliasBindingState)
	}
	s.items[name] = state
	if report {
		out.Bindings = append(out.Bindings, *bind)
	}
	return state
}

// fullName operates on aliasScope values and keeps the aliasscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (s *aliasScope) fullName(name string) string {
	if s == nil || s.name == "" {
		return name
	}
	return s.name + "." + name
}

// lookup operates on aliasScope values and keeps the aliasscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *aliasBindingState, bool so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func (s *aliasScope) lookup(name string) (*aliasBindingState, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.items == nil {
			continue
		}
		if state, ok := cur.items[name]; ok {
			if s.fn != nil && state.owner != s.fn {
				state.binding.Escapes = true
				if _, seen := s.fn.capturesSeen[name]; !seen {
					s.fn.capturesSeen[name] = struct{}{}
					s.fn.report.Captures = append(s.fn.report.Captures, name)
				}
				if !containsString(s.fn.report.Escapes, name) {
					s.fn.report.Escapes = append(s.fn.report.Escapes, name)
				}
				if state.info.class != nil {
					s.fn.captureByClass[state.info.class] = name
					s.fn.captureInfoByName[name] = state.info
					fullName := state.binding.Scope + "." + state.binding.Name
					if !containsString(s.fn.report.CaptureBindings, fullName) {
						s.fn.report.CaptureBindings = append(s.fn.report.CaptureBindings, fullName)
					}
				}
			}
			return state, true
		}
	}
	return nil, false
}

// lookupLocal operates on aliasScope values and keeps the aliasscope-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns *aliasBindingState, bool so callers can continue with the next
// stage of the pipeline without re-deriving the same information.
func (s *aliasScope) lookupLocal(name string) (*aliasBindingState, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if cur.items == nil {
			continue
		}
		if state, ok := cur.items[name]; ok {
			return state, true
		}
	}
	return nil, false
}

// aliasExpr handles the alias expr step in the surrounding package and returns
// aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasExpr(expr Expr, scope *aliasScope, out *AliasProgram) aliasInfo {
	switch n := expr.(type) {
	case *literalExpr:
		return aliasInfoFromValue(n.value, literalOrigin(n.value))
	case *identExpr:
		if state, ok := scope.lookup(n.name); ok {
			return state.info
		}
		if info, ok := aliasBuiltinValueInfo(n.name); ok {
			return info
		}
		return unknownAliasInfo("value")
	case *unaryExpr:
		inner := aliasExpr(n.expr, scope, out)
		switch n.op {
		case tokenBang:
			return freshAliasInfo("scalar", "not("+inner.origin+")")
		case tokenMinus, tokenTilde:
			if inner.kind == "tensor" || inner.kind == "list" {
				return freshAliasInfo(inner.kind, opOrigin(n.op, inner.origin))
			}
			return freshAliasInfo(aliasScalarKind(inner.kind), opOrigin(n.op, inner.origin))
		default:
			return unknownAliasInfo("value")
		}
	case *binaryExpr:
		left := aliasExpr(n.left, scope, out)
		right := aliasExpr(n.right, scope, out)
		if left.kind == "tensor" || right.kind == "tensor" {
			if n.op == tokenPlus || n.op == tokenMinus || n.op == tokenStar || n.op == tokenSlash || n.op == tokenPercent {
				return freshAliasInfo("tensor", binaryOrigin(n.op, left.origin, right.origin))
			}
		}
		switch n.op {
		case tokenPlus:
			if left.kind == "string" || right.kind == "string" {
				return freshAliasInfo("string", binaryOrigin(n.op, left.origin, right.origin))
			}
		case tokenEqualEqual, tokenBangEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual, tokenAndAnd, tokenOrOr:
			return freshAliasInfo("scalar", binaryOrigin(n.op, left.origin, right.origin))
		case tokenAmp, tokenPipe, tokenCaret, tokenShiftLeft, tokenShiftRight:
			return freshAliasInfo("scalar", binaryOrigin(n.op, left.origin, right.origin))
		}
		return freshAliasInfo(aliasScalarKind(joinKinds(left.kind, right.kind)), binaryOrigin(n.op, left.origin, right.origin))
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			switch name {
			case "if":
				if len(n.args) != 3 {
					return unknownAliasInfo("value")
				}
				_ = aliasExpr(n.args[0], scope, out)
				thenInfo := aliasExpr(n.args[1], scope, out)
				elseInfo := aliasExpr(n.args[2], scope, out)
				return joinAliasInfo(thenInfo, elseInfo)
			case "and", "or":
				if len(n.args) == 2 {
					_, _ = aliasExpr(n.args[0], scope, out), aliasExpr(n.args[1], scope, out)
				}
				return freshAliasInfo("scalar", "special "+name)
			case "parallel":
				for _, arg := range n.args {
					_ = aliasExpr(arg, scope, out)
				}
				return freshAliasInfo("list", "special parallel")
			case "race":
				for _, arg := range n.args {
					_ = aliasExpr(arg, scope, out)
				}
				if len(n.args) == 0 {
					return unknownAliasInfo("value")
				}
				info := aliasExpr(n.args[0], scope, out)
				for _, arg := range n.args[1:] {
					info = joinAliasInfo(info, aliasExpr(arg, scope, out))
				}
				return info
			case "select":
				for _, arg := range n.args {
					_ = aliasExpr(arg, scope, out)
				}
				return freshAliasInfo("list", "special select")
			case "timeout":
				if len(n.args) != 2 {
					return unknownAliasInfo("value")
				}
				_ = aliasExpr(n.args[0], scope, out)
				return aliasExpr(n.args[1], scope, out)
			case "spawn", "await", "cancel", "read_async", "readbytes_async", "write_async", "lines_async", "csv_async", "jsonparse_async":
				for _, arg := range n.args {
					_ = aliasExpr(arg, scope, out)
				}
				return freshAliasInfo("task", "special "+name)
			}
		}
		callee := aliasExpr(n.callee, scope, out)
		args := aliasExprs(n.args, scope, out)
		if callee.function != nil {
			return callFunctionAlias(callee.function, args)
		}
		if callee.builtin != "" {
			if info, ok := aliasBuiltinCall(callee.builtin, args, scope, out); ok {
				return info
			}
		}
		if name, ok := calledShapeName(n.callee); ok {
			if info, ok := aliasBuiltinCall(name, args, scope, out); ok {
				return info
			}
		}
		return unknownAliasInfo("value")
	case *listExpr:
		_ = aliasExprs(n.items, scope, out)
		return freshAliasInfo("list", "list literal")
	case *blockExpr:
		child := scope.childForBlock()
		return aliasBlockExpr(n, child, out)
	case *tryExpr:
		catchScope := scope.childForBlock()
		catchScope.bindLocal(n.catchName, freshAliasInfo("scalar", "error"), out, false)
		handler := aliasExpr(n.handler, catchScope, out)
		body := aliasExpr(n.body, scope, out)
		return joinAliasInfo(body, handler)
	case *propagateExpr:
		return aliasExpr(n.expr, scope, out)
	case *matchExpr:
		subject := aliasExpr(n.subject, scope, out)
		var outInfo aliasInfo
		have := false
		for _, arm := range n.arms {
			armScope := scope.childForBlock()
			bindPatternAliases(arm.pattern, subject, armScope, out)
			if arm.guard != nil {
				_ = aliasExpr(arm.guard, armScope, out)
			}
			info := aliasExpr(arm.body, armScope, out)
			if !have {
				outInfo = info
				have = true
				continue
			}
			outInfo = joinAliasInfo(outInfo, info)
		}
		if have {
			return outInfo
		}
		return unknownAliasInfo("value")
	case *memberExpr:
		_ = aliasExpr(n.object, scope, out)
		return unknownAliasInfo("value")
	default:
		return unknownAliasInfo("value")
	}
}

// aliasBlockExpr handles the alias block expr step in the surrounding package
// and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasBlockExpr(expr *blockExpr, scope *aliasScope, out *AliasProgram) aliasInfo {
	cur := scope
	for _, stmt := range expr.stmts {
		switch st := stmt.(type) {
		case letStmt:
			info := aliasExpr(st.Expr, cur, out)
			cur.bindLocal(st.Name, info, out, cur.report)
		case assignStmt:
			_ = aliasExpr(st.Expr, cur, out)
		case importStmt:
			info := freshAliasInfo("module", "import "+st.Path)
			cur.bindLocal(st.Alias, info, out, cur.report)
		case funcStmt:
			_ = aliasFunctionStmt(st, cur, out)
		case returnStmt:
			if st.Expr == nil {
				return freshAliasInfo("scalar", "return")
			}
			return aliasExpr(st.Expr, cur, out)
		case whileStmt:
			_ = aliasExpr(st.Cond, cur, out)
			_ = aliasExpr(st.Body, cur.childForLoop(), out)
		case forStmt:
			_ = aliasExpr(st.Iter, cur, out)
			loopScope := cur.childForLoop()
			loopScope.bindLocal(st.Name, unknownAliasInfo("value"), out, false)
			_ = aliasExpr(st.Body, loopScope, out)
		case breakStmt, continueStmt, macroStmt, packageStmt, exportStmt:
		default:
		}
	}
	if expr.tail == nil {
		return freshAliasInfo("scalar", "null")
	}
	return aliasExpr(expr.tail, cur, out)
}

// aliasExprs handles the alias exprs step in the surrounding package and
// returns []aliasInfo.
//
// It returns []aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasExprs(exprs []Expr, scope *aliasScope, out *AliasProgram) []aliasInfo {
	if len(exprs) == 0 {
		return nil
	}
	outInfos := make([]aliasInfo, len(exprs))
	for i, expr := range exprs {
		outInfos[i] = aliasExpr(expr, scope, out)
	}
	return outInfos
}

// bindPatternAliases handles the bind pattern aliases step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func bindPatternAliases(pattern Pattern, subject aliasInfo, scope *aliasScope, out *AliasProgram) {
	switch p := pattern.(type) {
	case wildcardPattern, literalPattern:
		return
	case bindPattern:
		scope.bindLocal(p.name, subject, out, false)
	case listPattern:
		for _, item := range p.items {
			bindPatternAliases(item, subject, scope, out)
		}
	case enumPattern:
		for _, item := range p.items {
			bindPatternAliases(item, subject, scope, out)
		}
	case structPattern:
		for _, field := range p.fields {
			bindPatternAliases(field.pattern, subject, scope, out)
		}
	case typePattern:
		if p.binding != "" && p.binding != "_" {
			scope.bindLocal(p.binding, subject, out, false)
		}
	case andPattern:
		bindPatternAliases(p.left, subject, scope, out)
		bindPatternAliases(p.right, subject, scope, out)
	case orPattern:
		if len(p.alternatives) > 0 {
			bindPatternAliases(p.alternatives[0], subject, scope, out)
		}
	}
}

// callFunctionAlias handles the call function alias step in the surrounding
// package and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func callFunctionAlias(fn *aliasFunctionState, args []aliasInfo) aliasInfo {
	if fn == nil {
		return unknownAliasInfo("value")
	}
	switch fn.resultMode {
	case "param":
		if fn.resultName == "" {
			return unknownAliasInfo("value")
		}
		for i, name := range fn.report.Params {
			if name == fn.resultName {
				if i < len(args) {
					return args[i]
				}
				break
			}
		}
		return unknownAliasInfo("value")
	case "capture":
		if info, ok := fn.captureInfoByName[fn.resultName]; ok {
			return info
		}
		return unknownAliasInfo("value")
	case "fresh":
		return freshAliasInfo(fn.resultKind, "call "+fn.report.Name)
	default:
		return unknownAliasInfo("value")
	}
}

// aliasBuiltinCall handles the alias builtin call step in the surrounding
// package and returns aliasInfo, bool.
//
// It returns aliasInfo, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func aliasBuiltinCall(name string, args []aliasInfo, scope *aliasScope, out *AliasProgram) (aliasInfo, bool) {
	switch name {
	case "view":
		if len(args) != 1 {
			return unknownAliasInfo("value"), true
		}
		return args[0], true
	case "clone":
		if len(args) != 1 {
			return unknownAliasInfo("value"), true
		}
		return freshLikeInfo(args[0], "clone"), true
	case "tensor", "matrix", "vector", "vec", "squeeze", "expand_dims", "concat", "stack", "broadcast_to", "where", "clip", "gather", "scatter", "pad", "tile", "outer", "inv", "solve", "cholesky":
		return freshAliasInfo("tensor", "builtin "+name), true
	case "reshape", "transpose", "matmul":
		return freshAliasInfo("tensor", "builtin "+name), true
	case "zeros", "ones", "full", "eye", "identity", "arange", "linspace", "logspace":
		return freshAliasInfo("tensor", "builtin "+name), true
	case "flatten", "shape", "split", "chunks", "range", "bytes", "unpack", "lines", "csv", "readbytes", "read_limit", "read_dir", "qr", "eig", "eigh", "svd":
		return freshAliasInfo("list", "builtin "+name), true
	case "openread", "openwrite", "openappend":
		return freshAliasInfo("stream", "builtin "+name), true
	case "readline":
		return freshAliasInfo("string", "builtin "+name), true
	case "readchunk":
		return freshAliasInfo("list", "builtin "+name), true
	case "writechunk", "append", "copy_file", "file_size", "seek", "tell":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "flush", "close", "truncate_file", "exists", "is_file", "is_dir", "mkdir", "mkdir_all", "rename", "remove", "truncate":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "pack", "cat", "join", "hash", "mix", "slug", "upper", "lower", "trim", "replace", "json", "format", "read":
		return freshAliasInfo("string", "builtin "+name), true
	case "repeat":
		if len(args) > 0 && args[0].kind == "string" {
			return freshAliasInfo("string", "builtin "+name), true
		}
		if len(args) > 0 {
			return freshLikeInfo(args[0], "builtin "+name), true
		}
		return unknownAliasInfo("value"), true
	case "take":
		if len(args) == 0 {
			return unknownAliasInfo("value"), true
		}
		return freshLikeInfo(args[0], "builtin "+name), true
	case "slice":
		return freshAliasInfo("tensor", "builtin "+name), true
	case "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "sign", "step", "heaviside", "relu", "sigmoid", "softplus", "cumsum", "cumprod":
		if len(args) == 0 {
			return unknownAliasInfo("value"), true
		}
		return freshLikeInfo(args[0], "builtin "+name), true
	case "mean", "var", "std", "median", "quantile", "percentile", "argmin", "argmax", "any", "all", "trace", "det":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "len", "sum", "product", "dot", "norm", "hypot", "abs", "sqrt", "pow", "expt", "exp", "log", "log10", "log2", "noise", "real", "imag", "conjugate", "magnitude", "uint", "u8", "u16", "u32", "u64", "byte", "uf8", "uf16", "uf32", "ufnum", "uf64", "uf128", "uf256", "uf512", "uf1024", "f32", "f64", "bf8", "bf16", "bf32", "bf64", "bf128", "bf256", "bf512", "bf1024", "dec", "decimal", "rat", "rational", "interval", "modint", "fixed", "complex", "bigint", "bigfloat", "band", "bor", "bxor", "bnot", "shl", "shr", "rotl", "rotr", "write":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "jsonparse":
		return freshAliasInfo("value", "builtin "+name), true
	case "setat":
		if len(args) == 0 {
			return unknownAliasInfo("value"), true
		}
		return freshLikeInfo(args[0], "setat"), true
	case "at":
		return unknownAliasInfo("value"), true
	default:
		if _, ok := pureBuiltinByName(name); ok {
			return freshAliasInfo("value", "builtin "+name), true
		}
		_ = scope
		_ = out
		return unknownAliasInfo("value"), false
	}
}

// aliasBuiltinValueInfo handles the alias builtin value info step in the
// surrounding package and returns aliasInfo, bool.
//
// It returns aliasInfo, bool so callers can continue with the next stage of
// the pipeline without re-deriving the same information.
func aliasBuiltinValueInfo(name string) (aliasInfo, bool) {
	switch name {
	case "true", "false", "null":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "i8", "i16", "i32", "i64", "i128", "i256", "i512", "i1024", "u8", "u16", "u32", "u64", "u128", "u256", "u512", "u1024", "uf8", "uf16", "uf32", "ufnum", "uf64", "uf128", "uf256", "uf512", "uf1024", "f32", "f64", "bf8", "bf16", "bf32", "bf64", "bf128", "bf256", "bf512", "bf1024", "fixed":
		return freshAliasInfo("scalar", "builtin "+name), true
	case "Meter", "meter", "Second", "second", "Kilogram", "kilogram", "Ampere", "ampere", "Kelvin", "kelvin", "Mole", "mole", "Candela", "candela":
		return freshAliasInfo("quantity", "unit "+name), true
	default:
		if _, ok := pureBuiltinByName(name); ok {
			info := freshAliasInfo("function", "builtin "+name)
			info.builtin = name
			return info, true
		}
	}
	return unknownAliasInfo("value"), false
}

// aliasInfoFromValue handles the alias info from value step in the surrounding
// package and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasInfoFromValue(v Value, origin string) aliasInfo {
	switch v.kind {
	case kindFunction:
		return freshAliasInfo("function", origin)
	case kindModule:
		return freshAliasInfo("module", origin)
	case kindTask:
		return freshAliasInfo("task", origin)
	case kindList:
		return freshAliasInfo("list", origin)
	case kindTensor:
		return freshAliasInfo("tensor", origin)
	case kindString:
		return freshAliasInfo("string", origin)
	case kindQuantity:
		return freshAliasInfo("quantity", origin)
	default:
		return freshAliasInfo("scalar", origin)
	}
}

// literalOrigin handles the literal origin step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func literalOrigin(v Value) string {
	switch v.kind {
	case kindString:
		return quoteString(v.str)
	case kindList:
		return "list literal"
	case kindTensor:
		return "tensor literal"
	case kindFunction:
		return "function literal"
	case kindModule:
		return "module literal"
	case kindTask:
		return "task literal"
	case kindQuantity:
		return "quantity literal"
	default:
		return v.Kind()
	}
}

// opOrigin handles the op origin step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func opOrigin(op tokenKind, origin string) string {
	return fmt.Sprintf("%s(%s)", op.String(), origin)
}

// binaryOrigin handles the binary origin step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func binaryOrigin(op tokenKind, left, right string) string {
	if left == "" {
		left = "?"
	}
	if right == "" {
		right = "?"
	}
	return fmt.Sprintf("%s(%s,%s)", op.String(), left, right)
}

// aliasScalarKind handles the alias scalar kind step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func aliasScalarKind(kind string) string {
	if kind == "" {
		return "scalar"
	}
	switch kind {
	case "tensor", "list", "string", "module", "function", "quantity":
		return kind
	default:
		return "scalar"
	}
}

// joinKinds handles the join kinds step in the surrounding package and returns
// string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func joinKinds(a, b string) string {
	if a == b {
		return a
	}
	if a == "" {
		return b
	}
	if b == "" {
		return a
	}
	return "value"
}

// freshLikeInfo handles the fresh like info step in the surrounding package
// and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func freshLikeInfo(info aliasInfo, origin string) aliasInfo {
	kind := info.kind
	if kind == "" || kind == "value" || !info.known {
		kind = "value"
	}
	return freshAliasInfo(kind, origin)
}

// freshAliasInfo handles the fresh alias info step in the surrounding package
// and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func freshAliasInfo(kind, origin string) aliasInfo {
	if kind == "" {
		kind = "value"
	}
	cls := &AliasClass{
		Label:   tempClassLabel(),
		Kind:    kind,
		Origin:  origin,
		Fresh:   true,
		Members: nil,
	}
	return aliasInfo{known: true, kind: kind, class: cls, origin: origin}
}

// unknownAliasInfo handles the unknown alias info step in the surrounding
// package and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func unknownAliasInfo(kind string) aliasInfo {
	if kind == "" {
		kind = "value"
	}
	return aliasInfo{known: false, kind: kind}
}

// joinAliasInfo handles the join alias info step in the surrounding package
// and returns aliasInfo.
//
// It returns aliasInfo so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func joinAliasInfo(a, b aliasInfo) aliasInfo {
	if a.class != nil && b.class != nil && a.class == b.class {
		return a
	}
	if a.class != nil && b.class != nil && a.class.Fresh && b.class.Fresh {
		kind := joinKinds(a.kind, b.kind)
		return freshAliasInfo(kind, "join")
	}
	if a.known && a.class != nil && a.class.Fresh && !b.known {
		return unknownAliasInfo(joinKinds(a.kind, b.kind))
	}
	if b.known && b.class != nil && b.class.Fresh && !a.known {
		return unknownAliasInfo(joinKinds(a.kind, b.kind))
	}
	return unknownAliasInfo(joinKinds(a.kind, b.kind))
}

// classLabel handles the class label step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func classLabel(cls *AliasClass) string {
	if cls == nil {
		return ""
	}
	return cls.Label
}

// addMember operates on AliasClass values and keeps the aliasclass-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (c *AliasClass) addMember(name string) {
	if c == nil {
		return
	}
	for _, existing := range c.Members {
		if existing == name {
			return
		}
	}
	c.Members = append(c.Members, name)
}

// registerAliasClass handles the register alias class step in the surrounding
// package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func registerAliasClass(out *AliasProgram, cls *AliasClass) {
	if out == nil || cls == nil {
		return
	}
	for _, existing := range out.Classes {
		if existing == cls {
			return
		}
	}
	out.Classes = append(out.Classes, cls)
}

// applyAliasCaptureEscapes handles the apply alias capture escapes step in the
// surrounding package and returns no result.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func applyAliasCaptureEscapes(out *AliasProgram) {
	if out == nil {
		return
	}
	for _, fn := range out.Functions {
		for _, fullName := range fn.CaptureBindings {
			for i := range out.Bindings {
				if bindingFullName(out.Bindings[i]) == fullName {
					out.Bindings[i].Escapes = true
				}
			}
		}
	}
}

// bindingFullName handles the binding full name step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func bindingFullName(bind AliasBinding) string {
	if bind.Scope == "" {
		return bind.Name
	}
	return bind.Scope + "." + bind.Name
}

// containsString handles the contains string step in the surrounding package
// and returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func containsString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

var aliasTempSeq int

// tempClassLabel handles the temp class label step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func tempClassLabel() string {
	aliasTempSeq++
	return "tmp" + strconv.Itoa(aliasTempSeq)
}
