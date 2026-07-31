package osiris

import (
	"context"
	"fmt"
	"strings"
)

// CheckedProgram is the shared typed semantic view consumed by inference,
// tooling, and backend preparation. Dynamic denotes a compatibility boundary
// whose type cannot be proven without executing user code.
type CheckedProgram struct {
	Program   *Program
	Bindings  []CheckedBinding
	Functions []CheckedFunction
}

type CheckedBinding struct {
	Name  string
	Scope string
	Type  string
	Span  sourceSpan
}

type CheckedFunction struct {
	Name       string
	Scope      string
	Signature  string
	Params     []CheckedBinding
	ReturnType string
	Span       sourceSpan
}

type staticType struct {
	name   string
	params []staticType
	result *staticType
}

var dynamicStaticType = staticType{name: "dynamic"}

func (t staticType) isDynamic() bool {
	return t.name == "" || t.name == "dynamic"
}

func (t staticType) isFunction() bool {
	return t.name == "func" && t.result != nil
}

func (t staticType) String() string {
	if t.isFunction() {
		items := make([]string, len(t.params))
		for i := range t.params {
			items[i] = t.params[i].String()
		}
		return "func(" + strings.Join(items, ", ") + ") " + t.result.String()
	}
	if t.name == "" {
		return "dynamic"
	}
	return t.name
}

func parseStaticType(text string) (staticType, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return dynamicStaticType, nil
	}
	if !strings.HasPrefix(text, "func(") {
		if !isBindingTypeName(text) {
			return staticType{}, fmt.Errorf("unknown type %q", text)
		}
		return staticType{name: text}, nil
	}
	depth := 0
	closeAt := -1
	for i := len("func"); i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			depth--
			if depth == 0 {
				closeAt = i
				i = len(text)
			}
		}
	}
	if closeAt < 0 {
		return staticType{}, fmt.Errorf("invalid function type %q", text)
	}
	paramText := strings.TrimSpace(text[len("func("):closeAt])
	resultText := strings.TrimSpace(text[closeAt+1:])
	if resultText == "" {
		return staticType{}, fmt.Errorf("function type requires a return type")
	}
	var params []staticType
	if paramText != "" {
		parts, err := splitStaticTypeList(paramText)
		if err != nil {
			return staticType{}, err
		}
		params = make([]staticType, 0, len(parts))
		for _, part := range parts {
			param, err := parseStaticType(part)
			if err != nil {
				return staticType{}, err
			}
			params = append(params, param)
		}
	}
	result, err := parseStaticType(resultText)
	if err != nil {
		return staticType{}, err
	}
	return staticType{name: "func", params: params, result: &result}, nil
}

func splitStaticTypeList(text string) ([]string, error) {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case ')':
			if depth == 0 {
				return nil, fmt.Errorf("invalid function type %q", text)
			}
			depth--
		case ',':
			if depth == 0 {
				part := strings.TrimSpace(text[start:i])
				if part == "" {
					return nil, fmt.Errorf("empty function parameter type")
				}
				parts = append(parts, part)
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, fmt.Errorf("invalid function type %q", text)
	}
	part := strings.TrimSpace(text[start:])
	if part == "" {
		return nil, fmt.Errorf("empty function parameter type")
	}
	return append(parts, part), nil
}

type typeScope struct {
	parent *typeScope
	name   string
	items  map[string]staticType
}

func newTypeScope(parent *typeScope, name string) *typeScope {
	return &typeScope{parent: parent, name: name, items: make(map[string]staticType)}
}

func (s *typeScope) lookup(name string) (staticType, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if value, ok := cur.items[name]; ok {
			return value, true
		}
	}
	return staticType{}, false
}

type typeChecker struct {
	ctx      context.Context
	rt       *Runtime
	prog     *Program
	out      *CheckedProgram
	exprs    map[Expr]checkedExpr
	function *funcStmt
	expected staticType
}

type checkedExpr struct {
	typ           staticType
	alwaysReturns bool
}

// CheckProgram checks every declaration and expression that can be proven
// statically. Existing unannotated/dynamic boundaries remain compatible, but
// all annotated function calls and returns are checked before execution or
// artifact emission.
func (rt *Runtime) CheckProgram(ctx context.Context, prog *Program) (*CheckedProgram, error) {
	if prog == nil {
		return nil, fmt.Errorf("cannot type-check nil program")
	}
	if rt == nil {
		rt = NewRuntime()
	}
	if ctx == nil {
		ctx = context.Background()
	}
	checker := &typeChecker{
		ctx:   ctx,
		rt:    rt,
		prog:  prog,
		out:   &CheckedProgram{Program: prog},
		exprs: make(map[Expr]checkedExpr),
	}
	root := newTypeScope(nil, "main")
	for name, value := range rt.Constants {
		root.items[name] = staticTypeFromValue(value)
	}
	if err := checker.checkSeq(prog.Statements, root); err != nil {
		return nil, err
	}
	return checker.out, nil
}

func (c *typeChecker) checkSeq(stmts []Stmt, scope *typeScope) error {
	if err := c.checkContext(); err != nil {
		return err
	}
	// Signatures are declared before bodies, so self recursion and mutually
	// recursive function groups never require evaluation for discovery.
	for _, stmt := range stmts {
		if fn, ok := stmt.(funcStmt); ok {
			signature, err := staticFunctionType(fn)
			if err != nil {
				return c.errorAt(fn.Span, "%v", err)
			}
			scope.items[fn.Name] = signature
		}
	}
	for _, stmt := range stmts {
		if err := c.checkStmt(stmt, scope); err != nil {
			return err
		}
	}
	return nil
}

func (c *typeChecker) checkStmt(stmt Stmt, scope *typeScope) error {
	if err := c.checkContext(); err != nil {
		return err
	}
	switch st := stmt.(type) {
	case letStmt:
		value, err := c.checkExpr(st.Expr, scope)
		if err != nil {
			return err
		}
		bindingType := value.typ
		if st.Type != "" {
			expected, parseErr := parseStaticType(st.Type)
			if parseErr != nil {
				return c.errorAt(st.Span, "%v", parseErr)
			}
			actual, assignErr := c.requireAssignable(expected, value.typ, st.Expr)
			if assignErr != nil {
				return c.errorAt(exprSpan(st.Expr), "binding type %s requires %s, got %s", expected.String(), expected.String(), actual.String())
			}
			bindingType = expected
		}
		scope.items[st.Name] = bindingType
		c.out.Bindings = append(c.out.Bindings, CheckedBinding{Name: st.Name, Scope: scope.name, Type: bindingType.String(), Span: st.NameSpan})
	case funcStmt:
		return c.checkFunction(st, scope)
	case emitStmt:
		_, err := c.checkExpr(st.Expr, scope)
		return err
	case returnStmt:
		if c.function == nil {
			return c.errorAt(st.Span, "return is only valid inside a function")
		}
		actual := staticType{name: "null"}
		if st.Expr != nil {
			value, err := c.checkExpr(st.Expr, scope)
			if err != nil {
				return err
			}
			actual = value.typ
		}
		if !c.expected.isDynamic() {
			got, err := c.requireAssignable(c.expected, actual, st.Expr)
			if err != nil {
				return c.errorAt(st.Span, "function %s returns %s, requires %s", c.function.Name, got.String(), c.expected.String())
			}
		}
	case whileStmt:
		if _, err := c.checkExpr(st.Cond, scope); err != nil {
			return err
		}
		_, err := c.checkExpr(st.Body, newTypeScope(scope, scope.name))
		return err
	case forStmt:
		if _, err := c.checkExpr(st.Iter, scope); err != nil {
			return err
		}
		child := newTypeScope(scope, scope.name)
		child.items[st.Name] = dynamicStaticType
		_, err := c.checkExpr(st.Body, child)
		return err
	case importStmt:
		scope.items[st.Alias] = staticType{name: "module"}
	case enumStmt:
		scope.items[st.Name] = staticType{name: "module"}
	case macroStmt, packageStmt, exportStmt, breakStmt, continueStmt:
		return nil
	}
	return nil
}

func (c *typeChecker) checkFunction(st funcStmt, parent *typeScope) error {
	signature, err := staticFunctionType(st)
	if err != nil {
		return c.errorAt(st.Span, "%v", err)
	}
	parent.items[st.Name] = signature
	child := newTypeScope(parent, st.Name)
	child.items[st.Name] = signature
	params := make([]CheckedBinding, len(st.Params))
	for i, name := range st.Params {
		paramType := dynamicStaticType
		if i < len(signature.params) {
			paramType = signature.params[i]
		}
		child.items[name] = paramType
		span := st.Span
		if i < len(st.ParamSpans) {
			span = st.ParamSpans[i]
		}
		params[i] = CheckedBinding{Name: name, Scope: st.Name, Type: paramType.String(), Span: span}
	}
	checked := CheckedFunction{
		Name:       st.Name,
		Scope:      parent.name,
		Signature:  signature.String(),
		Params:     params,
		ReturnType: signature.result.String(),
		Span:       st.Span,
	}
	c.out.Functions = append(c.out.Functions, checked)

	previousFunction, previousExpected := c.function, c.expected
	c.function, c.expected = &st, *signature.result
	body, checkErr := c.checkExpr(st.Body, child)
	c.function, c.expected = previousFunction, previousExpected
	if checkErr != nil {
		return checkErr
	}
	if !signature.result.isDynamic() && !body.alwaysReturns {
		got, assignErr := c.requireAssignable(*signature.result, body.typ, st.Body)
		if assignErr != nil {
			if block, ok := st.Body.(*blockExpr); ok && block.tail == nil {
				return c.errorAt(st.Span, "function %s can fall through with null, requires %s", st.Name, signature.result.String())
			}
			return c.errorAt(st.Span, "function %s returns %s, requires %s", st.Name, got.String(), signature.result.String())
		}
	}
	return nil
}

func staticFunctionType(st funcStmt) (staticType, error) {
	params := make([]staticType, len(st.Params))
	for i := range params {
		if i >= len(st.ParamTypes) || st.ParamTypes[i] == "" {
			params[i] = dynamicStaticType
			continue
		}
		param, err := parseStaticType(st.ParamTypes[i])
		if err != nil {
			return staticType{}, fmt.Errorf("parameter %s: %w", st.Params[i], err)
		}
		params[i] = param
	}
	result := dynamicStaticType
	if st.ReturnType != "" {
		parsed, err := parseStaticType(st.ReturnType)
		if err != nil {
			return staticType{}, fmt.Errorf("return type: %w", err)
		}
		result = parsed
	}
	return staticType{name: "func", params: params, result: &result}, nil
}

func (c *typeChecker) checkExpr(expr Expr, scope *typeScope) (result checkedExpr, err error) {
	defer func() {
		if expr != nil && err == nil {
			c.exprs[expr] = result
		}
	}()
	if err := c.checkContext(); err != nil {
		return checkedExpr{}, err
	}
	switch n := expr.(type) {
	case nil:
		return checkedExpr{typ: staticType{name: "null"}}, nil
	case *literalExpr:
		return checkedExpr{typ: staticTypeFromValue(n.value)}, nil
	case *identExpr:
		if value, ok := scope.lookup(n.name); ok {
			return checkedExpr{typ: value}, nil
		}
		if result, ok := builtinValueType(n.name); ok {
			return checkedExpr{typ: result}, nil
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	case *memberExpr:
		if _, err := c.checkExpr(n.object, scope); err != nil {
			return checkedExpr{}, err
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	case *unaryExpr:
		value, err := c.checkExpr(n.expr, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if n.op == tokenBang {
			return checkedExpr{typ: staticType{name: "bool"}}, nil
		}
		return value, nil
	case *binaryExpr:
		left, err := c.checkExpr(n.left, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		right, err := c.checkExpr(n.right, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		switch n.op {
		case tokenEqualEqual, tokenBangEqual, tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual, tokenAndAnd, tokenOrOr:
			return checkedExpr{typ: staticType{name: "bool"}}, nil
		case tokenPlus:
			if left.typ.name == "string" || right.typ.name == "string" {
				return checkedExpr{typ: staticType{name: "string"}}, nil
			}
		}
		if staticTypesEqual(left.typ, right.typ) {
			return checkedExpr{typ: left.typ}, nil
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	case *callExpr:
		return c.checkCall(n, scope)
	case *listExpr:
		for _, item := range n.items {
			if _, err := c.checkExpr(item, scope); err != nil {
				return checkedExpr{}, err
			}
		}
		return checkedExpr{typ: staticType{name: "list"}}, nil
	case *blockExpr:
		child := newTypeScope(scope, scope.name)
		for _, stmt := range n.stmts {
			fn, ok := stmt.(funcStmt)
			if !ok {
				continue
			}
			signature, signatureErr := staticFunctionType(fn)
			if signatureErr != nil {
				return checkedExpr{}, c.errorAt(fn.Span, "%v", signatureErr)
			}
			child.items[fn.Name] = signature
		}
		for _, stmt := range n.stmts {
			if ret, ok := stmt.(returnStmt); ok {
				if err := c.checkStmt(ret, child); err != nil {
					return checkedExpr{}, err
				}
				return checkedExpr{typ: c.expected, alwaysReturns: true}, nil
			}
			if err := c.checkStmt(stmt, child); err != nil {
				return checkedExpr{}, err
			}
		}
		if n.tail == nil {
			return checkedExpr{typ: staticType{name: "null"}}, nil
		}
		return c.checkExpr(n.tail, child)
	case *tryExpr:
		body, err := c.checkExpr(n.body, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		handlerScope := newTypeScope(scope, scope.name)
		handlerScope.items[n.catchName] = staticType{name: "error"}
		handler, err := c.checkExpr(n.handler, handlerScope)
		if err != nil {
			return checkedExpr{}, err
		}
		return checkedExpr{typ: joinStaticTypes(body.typ, handler.typ), alwaysReturns: body.alwaysReturns && handler.alwaysReturns}, nil
	case *propagateExpr:
		value, err := c.checkExpr(n.expr, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if value.typ.name == "result" || value.typ.name == "option" {
			return checkedExpr{typ: dynamicStaticType}, nil
		}
		return value, nil
	case *matchExpr:
		if _, err := c.checkExpr(n.subject, scope); err != nil {
			return checkedExpr{}, err
		}
		result := dynamicStaticType
		have := false
		allReturn := len(n.arms) > 0
		for _, arm := range n.arms {
			armScope := newTypeScope(scope, scope.name)
			for _, name := range patternBindingNames(arm.pattern) {
				armScope.items[name] = dynamicStaticType
			}
			if arm.guard != nil {
				if _, err := c.checkExpr(arm.guard, armScope); err != nil {
					return checkedExpr{}, err
				}
			}
			value, err := c.checkExpr(arm.body, armScope)
			if err != nil {
				return checkedExpr{}, err
			}
			if !have {
				result, have = value.typ, true
			} else {
				result = joinStaticTypes(result, value.typ)
			}
			allReturn = allReturn && value.alwaysReturns
		}
		return checkedExpr{typ: result, alwaysReturns: allReturn}, nil
	default:
		return checkedExpr{typ: dynamicStaticType}, nil
	}
}

func (c *typeChecker) checkCall(call *callExpr, scope *typeScope) (checkedExpr, error) {
	if name, ok := specialFormName(call.callee); ok {
		switch name {
		case "if":
			if len(call.args) != 3 {
				return checkedExpr{}, c.errorAt(call.Span, "if expects 3 arguments")
			}
			if _, err := c.checkExpr(call.args[0], scope); err != nil {
				return checkedExpr{}, err
			}
			left, err := c.checkExpr(call.args[1], scope)
			if err != nil {
				return checkedExpr{}, err
			}
			right, err := c.checkExpr(call.args[2], scope)
			if err != nil {
				return checkedExpr{}, err
			}
			return checkedExpr{typ: joinStaticTypes(left.typ, right.typ), alwaysReturns: left.alwaysReturns && right.alwaysReturns}, nil
		case "and", "or":
			for _, arg := range call.args {
				if _, err := c.checkExpr(arg, scope); err != nil {
					return checkedExpr{}, err
				}
			}
			return checkedExpr{typ: staticType{name: "bool"}}, nil
		case "spawn":
			for _, arg := range call.args {
				if _, err := c.checkExpr(arg, scope); err != nil {
					return checkedExpr{}, err
				}
			}
			return checkedExpr{typ: staticType{name: "task"}}, nil
		case "await", "timeout", "race", "select", "parallel", "cancel":
			for _, arg := range call.args {
				if _, err := c.checkExpr(arg, scope); err != nil {
					return checkedExpr{}, err
				}
			}
			return checkedExpr{typ: dynamicStaticType}, nil
		}
	}

	callee, err := c.checkExpr(call.callee, scope)
	if err != nil {
		return checkedExpr{}, err
	}
	if ident, ok := call.callee.(*identExpr); ok {
		if result, known := builtinCallType(ident.name, call.args); known {
			for _, arg := range call.args {
				if _, err := c.checkExpr(arg, scope); err != nil {
					return checkedExpr{}, err
				}
			}
			return checkedExpr{typ: result}, nil
		}
	}
	if !callee.typ.isFunction() {
		for _, arg := range call.args {
			if _, err := c.checkExpr(arg, scope); err != nil {
				return checkedExpr{}, err
			}
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	}
	name := "function value"
	if ident, ok := call.callee.(*identExpr); ok {
		name = ident.name
	}
	if len(call.args) != len(callee.typ.params) {
		return checkedExpr{}, c.errorAt(call.Span, "%s expects %d arguments, got %d", name, len(callee.typ.params), len(call.args))
	}
	for i, arg := range call.args {
		actual, checkErr := c.checkExpr(arg, scope)
		if checkErr != nil {
			return checkedExpr{}, checkErr
		}
		expected := callee.typ.params[i]
		if expected.isDynamic() {
			continue
		}
		got, assignErr := c.requireAssignable(expected, actual.typ, arg)
		if assignErr != nil {
			return checkedExpr{}, c.errorAt(exprSpan(arg), "argument %d to %s requires %s, got %s", i+1, name, expected.String(), got.String())
		}
	}
	return checkedExpr{typ: *callee.typ.result}, nil
}

func (c *typeChecker) requireAssignable(expected, actual staticType, expr Expr) (staticType, error) {
	if expected.isDynamic() || staticTypesEqual(expected, actual) {
		return actual, nil
	}
	if outcomes := staticOutcomeExpressions(expr); len(outcomes) > 0 {
		for _, outcome := range outcomes {
			value, ok := c.exprs[outcome]
			if !ok {
				return actual, nil
			}
			if _, err := c.requireAssignable(expected, value.typ, outcome); err != nil {
				return value.typ, err
			}
		}
		return expected, nil
	}
	if actual.isDynamic() {
		return actual, nil
	}
	if expected.name == "function" && actual.isFunction() {
		return actual, nil
	}
	if literal, ok := expr.(*literalExpr); ok && literal.defaultInteger && !expected.isFunction() {
		converted, err := checkBindingType(expected.name, literal.value, true)
		if err == nil {
			literal.value = converted
			literal.defaultInteger = false
			return expected, nil
		}
	}
	return actual, fmt.Errorf("not assignable")
}

// staticOutcomeExpressions returns the value-producing alternatives of a
// control-flow expression. Keeping these alternatives visible at an annotated
// boundary prevents a known branch mismatch from being mistaken for an
// unknowable dynamic value, and lets default integer literals inherit the
// boundary's exact width.
func staticOutcomeExpressions(expr Expr) []Expr {
	switch n := expr.(type) {
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok && name == "if" && len(n.args) == 3 {
			return []Expr{n.args[1], n.args[2]}
		}
	case *tryExpr:
		return []Expr{n.body, n.handler}
	case *matchExpr:
		out := make([]Expr, 0, len(n.arms))
		for _, arm := range n.arms {
			out = append(out, arm.body)
		}
		return out
	case *blockExpr:
		if n.tail != nil {
			return []Expr{n.tail}
		}
	}
	return nil
}

func staticTypesEqual(left, right staticType) bool {
	if left.isDynamic() || right.isDynamic() {
		return left.isDynamic() && right.isDynamic()
	}
	if left.name != right.name || len(left.params) != len(right.params) {
		return false
	}
	for i := range left.params {
		if !staticTypesEqual(left.params[i], right.params[i]) {
			return false
		}
	}
	if left.result == nil || right.result == nil {
		return left.result == nil && right.result == nil
	}
	return staticTypesEqual(*left.result, *right.result)
}

func joinStaticTypes(left, right staticType) staticType {
	if staticTypesEqual(left, right) {
		return left
	}
	return dynamicStaticType
}

func builtinCallType(name string, args []Expr) (staticType, bool) {
	if isBindingTypeName(name) && name != "function" && name != "module" {
		return staticType{name: name}, true
	}
	switch name {
	case "cat", "string", "text", "lower", "upper", "trim", "replace", "repeat", "join":
		return staticType{name: "string"}, true
	case "len":
		return staticType{name: "u64"}, true
	case "ok", "err":
		return staticType{name: "result"}, true
	case "some", "none":
		return staticType{name: "option"}, true
	case "error", "wrap":
		return staticType{name: "error"}, true
	}
	return staticType{}, false
}

func builtinValueType(name string) (staticType, bool) {
	return staticType{}, false
}

func staticTypeFromValue(value Value) staticType {
	switch value.kind {
	case kindUint:
		if value.packed {
			return staticType{name: value.Kind()}
		}
		bits := value.bits
		if value.ub != nil {
			bits = uint8(value.ubitsToUint16())
		}
		if bits == 0 {
			bits = 64
		}
		return staticType{name: fmt.Sprintf("u%d", bits)}
	case kindInt:
		bits := value.ibits
		if bits == 0 {
			bits = 64
		}
		return staticType{name: fmt.Sprintf("i%d", bits)}
	case kindNumber:
		if value.bfloat {
			return staticType{name: value.Kind()}
		}
		if value.fbits == 32 {
			return staticType{name: "f32"}
		}
		if value.fbits == 64 {
			return staticType{name: "f64"}
		}
		return staticType{name: "number"}
	case kindFunction:
		if value.fn != nil && value.fn.signature != "" {
			if parsed, err := parseStaticType(value.fn.signature); err == nil {
				return parsed
			}
		}
		return staticType{name: "function"}
	default:
		return staticType{name: value.Kind()}
	}
}

func (c *typeChecker) checkContext() error {
	select {
	case <-c.ctx.Done():
		return c.ctx.Err()
	default:
		return nil
	}
}

func (c *typeChecker) errorAt(span sourceSpan, format string, args ...any) error {
	err := newOsirisError(ErrorKindType, fmt.Sprintf(format, args...))
	return structuredErrorAt(err, c.prog.SourceName, span)
}

func patternBindingNames(pattern Pattern) []string {
	var names []string
	var walk func(Pattern)
	walk = func(item Pattern) {
		switch p := item.(type) {
		case bindPattern:
			if p.name != "" && p.name != "_" {
				names = append(names, p.name)
			}
		case typePattern:
			if p.binding != "" && p.binding != "_" {
				names = append(names, p.binding)
			}
		case enumPattern:
			for _, child := range p.items {
				walk(child)
			}
		case listPattern:
			for _, child := range p.items {
				walk(child)
			}
		case andPattern:
			walk(p.left)
			walk(p.right)
		case orPattern:
			if len(p.alternatives) > 0 {
				walk(p.alternatives[0])
			}
		case notPattern:
			return
		}
	}
	walk(pattern)
	return names
}
