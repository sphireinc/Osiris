package osiris

import (
	"context"
	"fmt"
	"sort"
	"strings"
)

type PurityProgram struct {
	Program   *Program
	Bindings  []PurityBinding
	Functions []PurityFunction
}

type PurityBinding struct {
	Name   string
	Scope  string
	Kind   string
	Status string
	Reason string
}

type PurityFunction struct {
	Name   string
	Scope  string
	Params []string
	Status string
	Reason string
	Calls  []string
}

// String handles on PurityProgram the string. It keeps the PurityProgram subsystem self-contained by handling the string-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *PurityProgram) String() string {
	return FormatPurityProgram(p)
}

// AnalyzePurity analyze on Runtime the purity. It keeps the Runtime subsystem self-contained by handling the purity-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (rt *Runtime) AnalyzePurity(ctx context.Context, prog *Program) (*PurityProgram, error) {
	_ = ctx
	if prog == nil {
		return nil, fmt.Errorf("cannot analyze purity for nil program")
	}
	return analyzePurityProgram(prog), nil
}

// FormatPurityProgram formats the purity program into a stable textual rendering. The implementation is intentionally deterministic so round-tripping and diff-based review stay predictable.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func FormatPurityProgram(prog *PurityProgram) string {
	if prog == nil {
		return ""
	}
	var b strings.Builder
	b.WriteString("purity ")
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
			b.WriteString(" ")
			b.WriteString(bind.Status)
			if bind.Reason != "" {
				b.WriteString(" (")
				b.WriteString(bind.Reason)
				b.WriteByte(')')
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
			if fn.Scope != "" {
				b.WriteString(" @ ")
				b.WriteString(fn.Scope)
			}
			if len(fn.Params) > 0 {
				b.WriteString(" params [")
				b.WriteString(strings.Join(fn.Params, ", "))
				b.WriteByte(']')
			}
			b.WriteString(" ")
			b.WriteString(fn.Status)
			if fn.Reason != "" {
				b.WriteString(" (")
				b.WriteString(fn.Reason)
				b.WriteByte(')')
			}
			if len(fn.Calls) > 0 {
				b.WriteString(" calls [")
				b.WriteString(strings.Join(fn.Calls, ", "))
				b.WriteByte(']')
			}
			b.WriteByte('\n')
		}
	}
	return b.String()
}

type purityBindingState struct {
	binding *PurityBinding
	info    purityInfo
	owner   *purityFunctionState
}

type purityScope struct {
	parent   *purityScope
	items    map[string]*purityBindingState
	fn       *purityFunctionState
	name     string
	report   bool
	blockSeq int
}

type purityFunctionState struct {
	report       *PurityFunction
	scope        *purityScope
	directImpure bool
	unknown      bool
	callsSeen    map[string]struct{}
}

type purityInfo struct {
	known    bool
	pure     bool
	kind     string
	reason   string
	function *purityFunctionState
}

// analyzePurityProgram performs the analyze purity program step in the surrounding package and returns *PurityProgram.. It returns *PurityProgram so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePurityProgram(prog *Program) *PurityProgram {
	out := &PurityProgram{Program: prog}
	root := &purityScope{
		items:  make(map[string]*purityBindingState),
		name:   "main",
		report: true,
	}
	analyzePuritySeq(prog.Statements, root, out)
	sort.Slice(out.Bindings, func(i, j int) bool {
		if out.Bindings[i].Scope == out.Bindings[j].Scope {
			return out.Bindings[i].Name < out.Bindings[j].Name
		}
		return out.Bindings[i].Scope < out.Bindings[j].Scope
	})
	sort.Slice(out.Functions, func(i, j int) bool {
		if out.Functions[i].Scope == out.Functions[j].Scope {
			return out.Functions[i].Name < out.Functions[j].Name
		}
		return out.Functions[i].Scope < out.Functions[j].Scope
	})
	return out
}

// analyzePuritySeq performs the analyze purity seq step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePuritySeq(stmts []Stmt, scope *purityScope, out *PurityProgram) purityInfo {
	var last purityInfo
	for _, stmt := range stmts {
		last = analyzePurityStmt(stmt, scope, out)
	}
	return last
}

// analyzePurityStmt performs the analyze purity stmt step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePurityStmt(stmt Stmt, scope *purityScope, out *PurityProgram) purityInfo {
	switch st := stmt.(type) {
	case letStmt:
		info := analyzePurityExpr(st.Expr, scope, out)
		scope.bindLocal(st.Name, info, out, true)
		return info
	case importStmt:
		info := purityInfo{known: true, pure: true, kind: "module", reason: "imported value"}
		scope.bindImport(st.Alias, st.Path, info, out, true)
		return purityInfo{known: true, pure: false, kind: "module", reason: "imports module"}
	case funcStmt:
		return analyzePurityFunctionStmt(st, scope, out)
	case emitStmt:
		info := analyzePurityExpr(st.Expr, scope, out)
		return purityInfo{known: info.known, pure: false, kind: info.kind, reason: "emits output"}
	case returnStmt:
		if st.Expr == nil {
			return purityInfo{known: true, pure: true, kind: "null", reason: "return"}
		}
		return analyzePurityExpr(st.Expr, scope, out)
	case whileStmt:
		cond := analyzePurityExpr(st.Cond, scope, out)
		body := analyzePurityExpr(st.Body, scope.childForLoop(), out)
		return combinePurity("while", cond, body)
	case forStmt:
		iter := analyzePurityExpr(st.Iter, scope, out)
		child := scope.childForLoop()
		child.bindLocal(st.Name, purityInfo{known: true, pure: true, kind: "value", reason: "loop variable"}, out, false)
		body := analyzePurityExpr(st.Body, child, out)
		return combinePurity("for", iter, body)
	case breakStmt, continueStmt, packageStmt, exportStmt, macroStmt:
		return purityInfo{known: true, pure: true, kind: "control", reason: statementKind(stmt)}
	default:
		return purityInfo{kind: "unknown"}
	}
}

// analyzePurityFunctionStmt performs the analyze purity function stmt step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePurityFunctionStmt(st funcStmt, scope *purityScope, out *PurityProgram) purityInfo {
	fnState := &purityFunctionState{
		report: &PurityFunction{
			Name:   st.Name,
			Scope:  scope.name,
			Params: append([]string(nil), st.Params...),
		},
		callsSeen: make(map[string]struct{}),
	}
	fnInfo := purityInfo{known: true, pure: true, kind: "function", function: fnState, reason: "function value"}
	scope.bindLocal(st.Name, fnInfo, out, scope.report)

	child := &purityScope{
		parent: scope,
		items:  make(map[string]*purityBindingState),
		fn:     fnState,
		name:   scope.name,
		report: true,
	}
	fnState.scope = child
	child.bindLocal(st.Name, fnInfo, out, false)
	for _, param := range st.Params {
		child.bindLocal(param, purityInfo{known: true, pure: true, kind: "value", reason: "parameter"}, out, false)
	}
	result := analyzePurityExpr(st.Body, child, out)
	fnState.report.Status = purityStatus(result)
	fnState.report.Reason = purityReason(result, "function body")
	if fnState.directImpure {
		fnState.report.Status = "impure"
		if fnState.report.Reason == "" {
			fnState.report.Reason = "contains side effects"
		}
	}
	if fnState.unknown && fnState.report.Status == "pure" {
		fnState.report.Status = "unknown"
		if fnState.report.Reason == "" {
			fnState.report.Reason = "contains unresolved calls"
		}
	}
	out.Functions = append(out.Functions, *fnState.report)
	return fnInfo
}

// childForBlock handles on purityScope the child for block. It keeps the purityScope subsystem self-contained by handling the child for block-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *purityScope) childForBlock() *purityScope {
	label := fmt.Sprintf("%s.b%d", s.name, s.blockSeq)
	s.blockSeq++
	return &purityScope{
		parent: s,
		items:  make(map[string]*purityBindingState),
		fn:     s.fn,
		name:   label,
		report: s.report,
	}
}

// childForLoop handles on purityScope the child for loop. It keeps the purityScope subsystem self-contained by handling the child for loop-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *purityScope) childForLoop() *purityScope {
	child := s.childForBlock()
	child.report = false
	return child
}

// bindLocal handles on purityScope the bind local. It keeps the purityScope subsystem self-contained by handling the bind local-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *purityScope) bindLocal(name string, info purityInfo, out *PurityProgram, report bool) *purityBindingState {
	bind := &PurityBinding{
		Name:   name,
		Scope:  s.name,
		Kind:   info.kind,
		Status: purityStatus(info),
		Reason: purityReason(info, "value"),
	}
	state := &purityBindingState{binding: bind, info: info, owner: s.fn}
	if s.items == nil {
		s.items = make(map[string]*purityBindingState)
	}
	s.items[name] = state
	if report {
		out.Bindings = append(out.Bindings, *bind)
	}
	return state
}

// bindImport handles on purityScope the bind import. It keeps the purityScope subsystem self-contained by handling the bind import-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *purityScope) bindImport(alias, path string, info purityInfo, out *PurityProgram, report bool) *purityBindingState {
	bind := &PurityBinding{
		Name:   alias,
		Scope:  s.name,
		Kind:   "module",
		Status: "impure",
		Reason: "imports " + path,
	}
	state := &purityBindingState{binding: bind, info: info, owner: s.fn}
	if s.items == nil {
		s.items = make(map[string]*purityBindingState)
	}
	s.items[alias] = state
	if report {
		out.Bindings = append(out.Bindings, *bind)
	}
	return state
}

// lookup handles on purityScope the lookup. It keeps the purityScope subsystem self-contained by handling the lookup-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (s *purityScope) lookup(name string) (*purityBindingState, bool) {
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

// analyzePurityExpr performs the analyze purity expr step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePurityExpr(expr Expr, scope *purityScope, out *PurityProgram) purityInfo {
	switch n := expr.(type) {
	case *literalExpr:
		return purityInfo{known: true, pure: true, kind: purityKindFromValue(n.value), reason: "literal"}
	case *identExpr:
		if state, ok := scope.lookup(n.name); ok {
			return state.info
		}
		if info, ok := purityBuiltinValueInfo(n.name); ok {
			return info
		}
		return purityInfo{kind: "unknown", reason: "undefined name"}
	case *unaryExpr:
		inner := analyzePurityExpr(n.expr, scope, out)
		if !inner.pure {
			return inner
		}
		return purityInfo{known: inner.known, pure: true, kind: inner.kind, reason: "unary"}
	case *binaryExpr:
		left := analyzePurityExpr(n.left, scope, out)
		right := analyzePurityExpr(n.right, scope, out)
		if !left.pure || !right.pure {
			return combinePurity("binary", left, right)
		}
		return purityInfo{known: left.known && right.known, pure: true, kind: combinePurityKind(left.kind, right.kind), reason: "binary"}
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok {
			return analyzePuritySpecialForm(name, n.args, scope, out)
		}
		if ident, ok := n.callee.(*identExpr); ok {
			if info, ok := purityBuiltinCall(ident.name, n.args, scope, out); ok {
				return info
			}
		}
		if name, ok := calledShapeName(n.callee); ok {
			if info, ok := purityBuiltinCall(name, n.args, scope, out); ok {
				return info
			}
		}
		callee := analyzePurityExpr(n.callee, scope, out)
		args := analyzePurityExprs(n.args, scope, out)
		if callee.function != nil {
			if scope.fn != nil {
				recordPurityCall(scope.fn, callee.function.report.Name)
			}
			return callPurityFunction(callee.function, args, scope.fn)
		}
		if !callee.pure {
			return purityInfo{kind: "unknown", reason: "impure callee"}
		}
		return purityInfo{kind: "unknown", reason: "unknown call"}
	case *listExpr:
		items := analyzePurityExprs(n.items, scope, out)
		return combinePurityList("list", items)
	case *blockExpr:
		child := scope.childForBlock()
		var acc purityInfo
		have := false
		for _, stmt := range n.stmts {
			info := analyzePurityStmt(stmt, child, out)
			if !have {
				acc = info
				have = true
				continue
			}
			acc = combinePurity("block", acc, info)
		}
		if n.tail == nil {
			if len(n.stmts) == 0 {
				return purityInfo{known: true, pure: true, kind: "null", reason: "empty block"}
			}
			return acc
		}
		tail := analyzePurityExpr(n.tail, child, out)
		if len(n.stmts) == 0 {
			return tail
		}
		return combinePurity("block", acc, tail)
	case *tryExpr:
		body := analyzePurityExpr(n.body, scope, out)
		catchScope := scope.childForBlock()
		catchScope.bindLocal(n.catchName, purityInfo{known: true, pure: true, kind: "error", reason: "catch binding"}, out, false)
		handler := analyzePurityExpr(n.handler, catchScope, out)
		return combinePurity("try", body, handler)
	case *matchExpr:
		subject := analyzePurityExpr(n.subject, scope, out)
		arms := make([]purityInfo, 0, len(n.arms))
		for _, arm := range n.arms {
			armScope := scope.childForBlock()
			bindPurityPattern(arm.pattern, subject, armScope, out)
			arms = append(arms, analyzePurityExpr(arm.body, armScope, out))
		}
		return combinePurity("match", append([]purityInfo{subject}, arms...)...)
	case *memberExpr:
		obj := analyzePurityExpr(n.object, scope, out)
		if !obj.pure {
			return obj
		}
		return purityInfo{known: obj.known, pure: true, kind: "value", reason: "member access"}
	default:
		return purityInfo{kind: "unknown", reason: fmt.Sprintf("expr %T", expr)}
	}
}

// analyzePurityExprs performs the analyze purity exprs step in the surrounding package and returns []purityInfo.. It returns []purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePurityExprs(exprs []Expr, scope *purityScope, out *PurityProgram) []purityInfo {
	if len(exprs) == 0 {
		return nil
	}
	outInfos := make([]purityInfo, len(exprs))
	for i, expr := range exprs {
		outInfos[i] = analyzePurityExpr(expr, scope, out)
	}
	return outInfos
}

// analyzePuritySpecialForm performs the analyze purity special form step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func analyzePuritySpecialForm(name string, args []Expr, scope *purityScope, out *PurityProgram) purityInfo {
	switch name {
	case "if":
		if len(args) != 3 {
			return purityInfo{kind: "unknown", reason: "if arity"}
		}
		cond := analyzePurityExpr(args[0], scope, out)
		thenInfo := analyzePurityExpr(args[1], scope, out)
		elseInfo := analyzePurityExpr(args[2], scope, out)
		return combinePurity("if", cond, thenInfo, elseInfo)
	case "and", "or":
		if len(args) != 2 {
			return purityInfo{kind: "unknown", reason: name + " arity"}
		}
		left := analyzePurityExpr(args[0], scope, out)
		right := analyzePurityExpr(args[1], scope, out)
		return combinePurity(name, left, right)
	default:
		return purityInfo{kind: "unknown", reason: "special form"}
	}
}

// purityBuiltinValueInfo performs the purity builtin value info step in the surrounding package and returns purityInfo, bool.. It returns purityInfo, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityBuiltinValueInfo(name string) (purityInfo, bool) {
	switch name {
	case "true", "false", "null":
		return purityInfo{known: true, pure: true, kind: "bool", reason: "builtin constant"}, true
	case "Meter", "meter", "Second", "second", "Kilogram", "kilogram", "Ampere", "ampere", "Kelvin", "kelvin", "Mole", "mole", "Candela", "candela":
		return purityInfo{known: true, pure: true, kind: "quantity", reason: "unit constant"}, true
	case "u8", "u16", "u32", "u64", "u128", "u256", "u512", "u1024":
		return purityInfo{known: true, pure: true, kind: "uint", reason: "builtin constant"}, true
	default:
		if _, ok := pureBuiltinByName(name); ok || purityBuiltinDeterministic(name) {
			return purityInfo{known: true, pure: true, kind: "function", reason: "builtin function"}, true
		}
	}
	return purityInfo{}, false
}

// purityBuiltinCall performs the purity builtin call step in the surrounding package and returns purityInfo, bool.. It returns purityInfo, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityBuiltinCall(name string, args []Expr, scope *purityScope, out *PurityProgram) (purityInfo, bool) {
	var info purityInfo
	switch name {
	case "read", "readbytes", "write":
		for _, arg := range args {
			_ = analyzePurityExpr(arg, scope, out)
		}
		return purityInfo{known: true, pure: false, kind: "string", reason: "filesystem I/O"}, true
	case "setat":
		if len(args) == 0 {
			return purityInfo{kind: "unknown", reason: "setat arity"}, true
		}
		for _, arg := range args {
			_ = analyzePurityExpr(arg, scope, out)
		}
		return purityInfo{known: true, pure: true, kind: "value", reason: "pure copy"}, true
	}
	if builtin, ok := pureBuiltinByName(name); ok {
		allPure := true
		for _, arg := range args {
			if !analyzePurityExpr(arg, scope, out).pure {
				allPure = false
			}
		}
		if !allPure {
			return purityInfo{kind: "unknown", reason: "impure argument"}, true
		}
		_ = builtin
		return purityInfo{known: true, pure: true, kind: builtinPurityKind(name), reason: "pure builtin"}, true
	}
	if purityBuiltinDeterministic(name) {
		allPure := true
		for _, arg := range args {
			if !analyzePurityExpr(arg, scope, out).pure {
				allPure = false
			}
		}
		if !allPure {
			return purityInfo{kind: "unknown", reason: "impure argument"}, true
		}
		return purityInfo{known: true, pure: true, kind: builtinPurityKind(name), reason: "pure builtin"}, true
	}
	return info, false
}

// callPurityFunction performs the call purity function step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func callPurityFunction(fn *purityFunctionState, args []purityInfo, current *purityFunctionState) purityInfo {
	if fn == nil {
		return purityInfo{kind: "unknown", reason: "unknown function"}
	}
	for _, arg := range args {
		if !arg.pure {
			return purityInfo{known: arg.known, pure: false, kind: "function", reason: "impure argument"}
		}
	}
	if current != nil && fn == current {
		return purityInfo{known: true, pure: true, kind: "function", reason: "recursive pure function"}
	}
	switch fn.report.Status {
	case "pure":
		return purityInfo{known: true, pure: true, kind: "function", reason: "pure function"}
	case "impure":
		return purityInfo{known: true, pure: false, kind: "function", reason: "function body has side effects"}
	case "unknown":
		return purityInfo{kind: "unknown", reason: "unresolved function call"}
	}
	_ = args
	return purityInfo{kind: "unknown", reason: "unresolved function call"}
}

// bindPurityPattern performs the bind purity pattern step in the surrounding package and returns no result.. It does not return a value; it communicates success or failure through its side effects or through a separate error path when one exists.
func bindPurityPattern(pattern Pattern, subject purityInfo, scope *purityScope, out *PurityProgram) {
	switch p := pattern.(type) {
	case wildcardPattern, literalPattern:
		return
	case bindPattern:
		scope.bindLocal(p.name, subject, out, false)
	case listPattern:
		for _, item := range p.items {
			bindPurityPattern(item, subject, scope, out)
		}
	}
}

// combinePurity performs the combine purity step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func combinePurity(label string, infos ...purityInfo) purityInfo {
	out := purityInfo{known: true, pure: true, kind: "value", reason: label}
	for _, info := range infos {
		if !info.known {
			out.known = false
		}
		if !info.pure {
			out.pure = false
		}
		if info.kind != "" && out.kind == "value" {
			out.kind = info.kind
		}
		if info.reason != "" {
			out.reason = info.reason
		}
	}
	if !out.pure && out.reason == "" {
		out.reason = label
	}
	return out
}

// combinePurityList performs the combine purity list step in the surrounding package and returns purityInfo.. It returns purityInfo so callers can continue with the next stage of the pipeline without re-deriving the same information.
func combinePurityList(label string, infos []purityInfo) purityInfo {
	return combinePurity(label, infos...)
}

// combinePurityKind performs the combine purity kind step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func combinePurityKind(left, right string) string {
	if left == right {
		return left
	}
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	return "value"
}

// purityStatus performs the purity status step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityStatus(info purityInfo) string {
	switch {
	case !info.known:
		return "unknown"
	case info.pure:
		return "pure"
	default:
		return "impure"
	}
}

// purityReason performs the purity reason step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityReason(info purityInfo, fallback string) string {
	if info.reason != "" {
		return info.reason
	}
	if fallback != "" {
		return fallback
	}
	if info.pure {
		return "pure"
	}
	return "side effects"
}

// purityKindFromValue performs the purity kind from value step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityKindFromValue(v Value) string {
	switch v.kind {
	case kindNull:
		return "null"
	case kindBool:
		return "bool"
	case kindNumber:
		return "number"
	case kindDecimal:
		return "decimal"
	case kindComplex:
		return "complex"
	case kindError:
		return "error"
	case kindBigInt:
		return "bigint"
	case kindBigFloat:
		return "bigfloat"
	case kindQuantity:
		return "quantity"
	case kindUint:
		return "uint"
	case kindString:
		return "string"
	case kindList:
		return "list"
	case kindTensor:
		return "tensor"
	case kindFunction:
		return "function"
	case kindModule:
		return "module"
	default:
		return "value"
	}
}

// builtinPurityKind performs the builtin purity kind step in the surrounding package and returns string.. It returns string so callers can continue with the next stage of the pipeline without re-deriving the same information.
func builtinPurityKind(name string) string {
	switch name {
	case "tensor", "matrix", "vector", "vec", "reshape", "transpose", "flatten", "shape", "dot", "matmul", "clone", "view", "zeros", "ones", "full", "eye", "identity", "arange", "linspace", "logspace", "squeeze", "expand_dims", "concat", "stack", "broadcast_to", "where", "clip", "gather", "scatter", "pad", "tile", "outer", "inv", "solve", "cholesky":
		return "tensor"
	case "split", "chunks", "bytes", "pack", "unpack", "lines", "csv", "jsonparse", "range", "take", "qr", "eig", "eigh", "svd":
		return "list"
	case "cat", "join", "hash", "mix", "slug", "upper", "lower", "trim", "replace", "json", "format":
		return "string"
	case "read", "readbytes", "write":
		return "string"
	case "len", "sum", "product", "norm", "hypot", "abs", "sqrt", "pow", "expt", "exp", "log", "log10", "log2", "noise", "real", "imag", "magnitude", "uint", "u8", "u16", "u32", "u64", "byte", "uf8", "uf16", "uf32", "ufnum", "uf64", "uf128", "uf256", "uf512", "uf1024", "f32", "f64", "bf8", "bf16", "bf32", "bf64", "bf128", "bf256", "bf512", "bf1024", "i8", "i16", "i32", "i64", "i128", "i256", "i512", "i1024", "dec", "decimal", "rat", "rational", "interval", "modint", "fixed", "complex", "bigint", "bigfloat", "band", "bor", "bxor", "bnot", "shl", "shr", "rotl", "rotr", "setat", "at", "repeat", "sinh", "cosh", "tanh", "asinh", "acosh", "atanh", "expm1", "log1p", "cbrt", "erf", "erfc", "gamma", "lgamma", "sign", "step", "heaviside", "relu", "sigmoid", "softplus", "mean", "var", "std", "median", "quantile", "percentile", "argmin", "argmax", "any", "all", "cumsum", "cumprod", "trace", "det":
		return "value"
	default:
		return "value"
	}
}

// purityBuiltinDeterministic performs the purity builtin deterministic step in the surrounding package and returns bool.. It returns bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func purityBuiltinDeterministic(name string) bool {
	switch name {
	case "hash", "mix", "slug", "upper", "lower", "trim", "replace", "repeat",
		"hex", "bin", "bytes", "pack", "unpack", "encode", "decode",
		"json", "format", "noise", "lines", "csv", "jsonparse",
		"pow", "expt":
		return true
	default:
		return false
	}
}

// containsPurityCall performs the contains purity call step in the surrounding package and returns bool.. It returns bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
func containsPurityCall(calls map[string]struct{}, want string) bool {
	_, ok := calls[want]
	return ok
}

// recordPurityCall performs the record purity call step in the surrounding package and returns no result.. It does not return a value; it communicates success or failure through its side effects or through a separate error path when one exists.
func recordPurityCall(fn *purityFunctionState, name string) {
	if fn == nil || name == "" {
		return
	}
	if fn.callsSeen == nil {
		fn.callsSeen = make(map[string]struct{})
	}
	if _, ok := fn.callsSeen[name]; ok {
		return
	}
	fn.callsSeen[name] = struct{}{}
	fn.report.Calls = append(fn.report.Calls, name)
}
