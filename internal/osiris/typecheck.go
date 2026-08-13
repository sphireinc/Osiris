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
	Types     *TypeContext
	Nominals  map[string]TypeID
	Aliases   map[string]TypeID
	Bindings  []CheckedBinding
	Functions []CheckedFunction
}

type CheckedBinding struct {
	Symbol   SymbolID
	TypeID   TypeID
	Name     string
	Scope    string
	Type     string
	Mutable  bool
	Constant bool
	Span     sourceSpan
}

type CheckedFunction struct {
	TypeID       TypeID
	ReturnTypeID TypeID
	Name         string
	Scope        string
	Signature    string
	TypeParams   []string
	Params       []CheckedBinding
	ReturnType   string
	Span         sourceSpan
}

// DisplayType formats the canonical type carried by a checked binding. The
// textual Type field is retained temporarily for source compatibility, but
// tooling must prefer this graph-backed presentation.
func (p *CheckedProgram) DisplayType(binding CheckedBinding, mode TypeFormatMode) string {
	if p != nil && p.Types != nil && binding.TypeID != 0 {
		return p.Types.Format(binding.TypeID, mode)
	}
	return binding.Type
}

type staticType struct {
	name   string
	params []staticType
	result *staticType
}

var dynamicStaticType = staticType{name: "dynamic"}

// staticTypeLazy is the nominal constructor for explicit deferred values. Its
// payload stays structural in staticType.params: no later checker, inference,
// or backend boundary needs to reconstruct a Lazy type by concatenating a
// rendered type string.
const staticTypeLazy = "Lazy"

func lazyStaticType(element staticType) staticType {
	return staticType{name: staticTypeLazy, params: []staticType{element}}
}

func (t staticType) lazyElement() (staticType, bool) {
	if t.name != staticTypeLazy || t.result != nil || len(t.params) != 1 {
		return staticType{}, false
	}
	return t.params[0], true
}

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
	if len(t.params) == 0 {
		return t.name
	}
	items := make([]string, len(t.params))
	for i := range t.params {
		items[i] = t.params[i].String()
	}
	return t.name + "<" + strings.Join(items, ", ") + ">"
}

func parseStaticType(text string) (staticType, error) {
	text = strings.TrimSpace(text)
	if text == "" {
		return dynamicStaticType, nil
	}
	if !strings.HasPrefix(text, "func(") {
		open := strings.IndexByte(text, '<')
		if open < 0 {
			if err := validateParameterizedStaticType(text, nil); err != nil {
				return staticType{}, err
			}
			return staticType{name: text}, nil
		}
		if !strings.HasSuffix(text, ">") || open == 0 {
			return staticType{}, fmt.Errorf("invalid generic type %q", text)
		}
		base := strings.TrimSpace(text[:open])
		parts, err := splitStaticTypeList(strings.TrimSpace(text[open+1 : len(text)-1]))
		if err != nil {
			return staticType{}, err
		}
		params := make([]staticType, len(parts))
		for i, part := range parts {
			params[i], err = parseStaticType(part)
			if err != nil {
				return staticType{}, err
			}
		}
		if err := validateParameterizedStaticType(base, params); err != nil {
			return staticType{}, err
		}
		return staticType{name: base, params: params}, nil
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

func validateParameterizedStaticType(name string, params []staticType) error {
	if name == staticTypeLazy {
		if len(params) != 1 {
			return fmt.Errorf("Lazy requires exactly one type argument")
		}
		return nil
	}
	return validateFutureStaticType(name, params)
}

func splitStaticTypeList(text string) ([]string, error) {
	var parts []string
	depth := 0
	start := 0
	for i := 0; i < len(text); i++ {
		switch text[i] {
		case '(':
			depth++
		case '<':
			depth++
		case ')':
			if depth == 0 {
				return nil, fmt.Errorf("invalid function type %q", text)
			}
			depth--
		case '>':
			if depth == 0 {
				return nil, fmt.Errorf("invalid generic type %q", text)
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
	parent   *typeScope
	name     string
	items    map[string]staticType
	declared map[string]struct{}
	mutable  map[string]bool
	constant map[string]bool
}

func newTypeScope(parent *typeScope, name string) *typeScope {
	return &typeScope{parent: parent, name: name, items: make(map[string]staticType), declared: make(map[string]struct{}), mutable: make(map[string]bool), constant: make(map[string]bool)}
}

func (s *typeScope) lookup(name string) (staticType, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if value, ok := cur.items[name]; ok {
			return value, true
		}
	}
	return staticType{}, false
}

func (s *typeScope) lookupMutable(name string) (bool, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.items[name]; ok {
			return cur.mutable[name], true
		}
	}
	return false, false
}

func (s *typeScope) lookupConstant(name string) (bool, bool) {
	for cur := s; cur != nil; cur = cur.parent {
		if _, ok := cur.items[name]; ok {
			return cur.constant[name], true
		}
	}
	return false, false
}

func isConstExpr(expr Expr, scope *typeScope) bool {
	switch n := expr.(type) {
	case *literalExpr:
		return true
	case *identExpr:
		constant, ok := scope.lookupConstant(n.name)
		return ok && constant
	case *unaryExpr:
		return isConstExpr(n.expr, scope)
	case *binaryExpr:
		return isConstExpr(n.left, scope) && isConstExpr(n.right, scope)
	case *listExpr:
		for _, item := range n.items {
			if !isConstExpr(item, scope) {
				return false
			}
		}
		return true
	case *tupleExpr:
		for _, item := range n.items {
			if !isConstExpr(item, scope) {
				return false
			}
		}
		return true
	default:
		return false
	}
}

type typeChecker struct {
	ctx         context.Context
	rt          *Runtime
	prog        *Program
	out         *CheckedProgram
	exprs       map[Expr]checkedExpr
	function    *funcStmt
	expected    staticType
	structs     map[string]staticStructDef
	aliases     map[string]string
	newtypes    map[string]string
	functions   map[string]funcStmt
	traits      map[string]traitStmt
	methods     map[string]map[string]staticType
	methodIDs   map[string]map[string]TypeID
	impls       map[string]map[string]struct{}
	types       *TypeContext
	nominals    map[string]SymbolID
	nextNominal SymbolID
}

type staticStructDef struct {
	fields map[string]checkedStaticType
	order  []string
}

// checkedStaticType preserves source-adapter syntax only while carrying the
// canonical ID that checker tables use for semantic identity.
type checkedStaticType struct {
	syntax staticType
	id     TypeID
}

type checkedExpr struct {
	typ           staticType
	typeID        TypeID
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
		ctx:         ctx,
		rt:          rt,
		prog:        prog,
		out:         &CheckedProgram{Program: prog, Types: NewTypeContext()},
		exprs:       make(map[Expr]checkedExpr),
		structs:     make(map[string]staticStructDef),
		aliases:     make(map[string]string),
		newtypes:    make(map[string]string),
		functions:   make(map[string]funcStmt),
		traits:      make(map[string]traitStmt),
		methods:     make(map[string]map[string]staticType),
		methodIDs:   make(map[string]map[string]TypeID),
		impls:       make(map[string]map[string]struct{}),
		nominals:    make(map[string]SymbolID),
		nextNominal: 1,
		types:       nil,
	}
	checker.types = checker.out.Types
	root := newTypeScope(nil, "main")
	for name, value := range rt.Constants {
		root.items[name] = staticTypeFromValue(value)
		root.constant[name] = true
	}
	if err := checker.checkSeq(prog.Statements, root); err != nil {
		return nil, err
	}
	checker.out.Nominals = make(map[string]TypeID, len(checker.nominals))
	for name := range checker.nominals {
		checker.out.Nominals[name] = checker.typeID(staticType{name: name})
	}
	checker.out.Aliases = make(map[string]TypeID, len(checker.aliases))
	for name, target := range checker.aliases {
		if typ, err := parseStaticType(target); err == nil {
			checker.out.Aliases[name] = checker.typeID(typ)
		}
	}
	return checker.out, nil
}

func (c *typeChecker) typeEqual(left, right staticType) bool {
	return c.types.Equal(c.types.InternStatic(c.resolveAlias(left)), c.types.InternStatic(c.resolveAlias(right)))
}

func (c *typeChecker) typeID(typ staticType) TypeID {
	return c.internType(typ, nil, 0)
}

func (c *typeChecker) typeIDForDeclaration(typ staticType, parameters []typeParam, owner SymbolID) TypeID {
	parameterIndexes := make(map[string]int, len(parameters))
	for index, parameter := range parameters {
		parameterIndexes[parameter.Name] = index
	}
	return c.internType(typ, parameterIndexes, owner)
}

func (c *typeChecker) internType(typ staticType, parameters map[string]int, owner SymbolID) TypeID {
	typ = c.resolveAlias(typ)
	if index, ok := parameters[typ.name]; ok && len(typ.params) == 0 && typ.result == nil {
		return c.types.Intern(TypeNode{Kind: TypeGenericParameter, Name: typ.name, Declaration: owner, Args: []TypeID{TypeID(index + 1)}})
	}
	args := make([]TypeID, len(typ.params))
	for i := range typ.params {
		args[i] = c.internType(typ.params[i], parameters, owner)
	}
	result := TypeID(0)
	if typ.result != nil {
		result = c.internType(*typ.result, parameters, owner)
	}
	return c.types.Intern(TypeNode{Kind: typeKindForStatic(typ), Name: typ.name, Declaration: c.nominals[typ.name], Args: args, Result: result})
}

func (c *typeChecker) registerNominal(name string) {
	if name == "" || c.nominals[name] != 0 {
		return
	}
	c.nominals[name] = c.nextNominal
	c.nextNominal++
}

func (c *typeChecker) joinTypes(left, right staticType) staticType {
	if c.typeEqual(left, right) {
		return left
	}
	return dynamicStaticType
}

func (c *typeChecker) checkSeq(stmts []Stmt, scope *typeScope) error {
	if err := c.checkContext(); err != nil {
		return err
	}
	// Declarations are collected in source order before any body is checked.
	// This makes trait conformance and method lookup independent of execution.
	for _, stmt := range stmts {
		switch st := stmt.(type) {
		case structStmt:
			c.registerNominal(st.Name)
			def := staticStructDef{fields: make(map[string]checkedStaticType), order: make([]string, len(st.Fields))}
			for i, field := range st.Fields {
				typ, err := parseStaticType(field.Type)
				if err != nil {
					return c.errorAt(field.Span, "%v", err)
				}
				def.fields[field.Name], def.order[i] = checkedStaticType{syntax: typ, id: c.typeID(typ)}, field.Name
			}
			c.structs[st.Name] = def
			scope.items[st.Name] = staticType{name: st.Name}
		case typeDeclStmt:
			c.aliases[st.Name] = st.Target
			scope.items[st.Name] = staticType{name: st.Target}
		case newtypeStmt:
			c.registerNominal(st.Name)
			c.newtypes[st.Name] = st.Target
			scope.items[st.Name] = staticType{name: st.Name}
		case enumStmt:
			c.registerNominal(st.Name)
			scope.items[st.Name] = staticType{name: st.Name}
		case traitStmt:
			c.registerNominal(st.Name)
			if _, exists := c.traits[st.Name]; exists {
				return c.errorAt(st.NameSpan, "trait %s is declared more than once", st.Name)
			}
			c.traits[st.Name] = st
			scope.items[st.Name] = staticType{name: st.Name}
			for _, method := range st.Methods {
				signature, err := staticFunctionType(method)
				if err != nil {
					return c.errorAt(method.Span, "%v", err)
				}
				if c.methods[st.Name] == nil {
					c.methods[st.Name] = make(map[string]staticType)
					c.methodIDs[st.Name] = make(map[string]TypeID)
				}
				if _, exists := c.methods[st.Name][method.Name]; exists {
					return c.errorAt(method.NameSpan, "trait %s declares method %s more than once", st.Name, method.Name)
				}
				c.methods[st.Name][method.Name] = signature
				c.methodIDs[st.Name][method.Name] = c.typeID(signature)
			}
		case implStmt:
			trait, exists := c.traits[st.Trait]
			if !exists {
				return c.errorAt(st.TraitSpan, "cannot implement unknown trait %s", st.Trait)
			}
			key := st.Target + "::" + st.Trait
			if c.impls[key] != nil {
				return c.errorAt(st.Span, "trait %s is implemented more than once for %s", st.Trait, st.Target)
			}
			seen := make(map[string]struct{}, len(st.Methods))
			for _, method := range st.Methods {
				if _, duplicate := seen[method.Name]; duplicate {
					return c.errorAt(method.NameSpan, "method %s is implemented more than once for %s", method.Name, st.Target)
				}
				seen[method.Name] = struct{}{}
				expected, required := c.methods[trait.Name][method.Name]
				if !required {
					return c.errorAt(method.NameSpan, "method %s is not declared by trait %s", method.Name, trait.Name)
				}
				actual, err := staticFunctionType(method)
				if err != nil {
					return c.errorAt(method.Span, "%v", err)
				}
				expectedID := c.methodIDs[trait.Name][method.Name]
				if expectedID == 0 || !c.types.Equal(c.typeID(actual), expectedID) {
					return c.errorAt(method.Span, "method %s for %s has signature %s, requires %s", method.Name, st.Target, actual.String(), expected.String())
				}
				if c.methods[st.Target] == nil {
					c.methods[st.Target] = make(map[string]staticType)
					c.methodIDs[st.Target] = make(map[string]TypeID)
				}
				c.methods[st.Target][method.Name] = actual
				c.methodIDs[st.Target][method.Name] = c.typeID(actual)
			}
			for method := range c.methods[trait.Name] {
				if _, ok := seen[method]; !ok {
					return c.errorAt(st.Span, "trait %s requires method %s for %s", trait.Name, method, st.Target)
				}
			}
			c.impls[key] = seen
		}
	}
	for _, stmt := range stmts {
		if fn, ok := stmt.(funcStmt); ok {
			c.functions[functionBindingName(fn)] = fn
			signature, err := staticFunctionType(fn)
			if err != nil {
				return c.errorAt(fn.Span, "%v", err)
			}
			scope.items[functionBindingName(fn)] = signature
			if fn.ReceiverType != "" {
				if c.methods[fn.ReceiverType] == nil {
					c.methods[fn.ReceiverType] = make(map[string]staticType)
					c.methodIDs[fn.ReceiverType] = make(map[string]TypeID)
				}
				if _, exists := c.methods[fn.ReceiverType][fn.Name]; exists {
					return c.errorAt(fn.NameSpan, "method %s is declared more than once for %s", fn.Name, fn.ReceiverType)
				}
				c.methods[fn.ReceiverType][fn.Name] = signature
				c.methodIDs[fn.ReceiverType][fn.Name] = c.typeID(signature)
			}
		}
		if impl, ok := stmt.(implStmt); ok {
			for _, method := range impl.Methods {
				c.functions[functionBindingName(method)] = method
				signature, err := staticFunctionType(method)
				if err != nil {
					return c.errorAt(method.Span, "%v", err)
				}
				// Methods are found through the receiver type, not as free
				// functions, but retaining the qualified name makes recursive
				// lookup and diagnostics deterministic.
				scope.items[functionBindingName(method)] = signature
			}
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
		names := st.Names
		if len(names) == 0 {
			names = []string{st.Name}
		}
		for _, name := range names {
			if _, exists := scope.declared[name]; exists {
				return c.errorAt(st.NameSpan, "redeclaration of %q in the same scope", name)
			}
		}
		value, err := c.checkExpr(st.Expr, scope)
		if err != nil {
			return err
		}
		if len(st.Names) > 0 {
			if value.typ.name != "tuple" || len(value.typ.params) != len(st.Names) {
				return c.errorAt(st.Span, "tuple destructuring requires %d values", len(st.Names))
			}
			for i, name := range st.Names {
				scope.items[name] = value.typ.params[i]
				scope.declared[name] = struct{}{}
				c.out.Bindings = append(c.out.Bindings, CheckedBinding{Name: name, Scope: scope.name, TypeID: c.typeID(value.typ.params[i]), Type: value.typ.params[i].String(), Span: st.Span})
			}
			return nil
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
		scope.declared[st.Name] = struct{}{}
		scope.mutable[st.Name] = st.Mutable
		if st.Const {
			if st.Unwrap {
				return c.errorAt(st.Span, "const does not support let? propagation")
			}
			if !isConstExpr(st.Expr, scope) {
				return c.errorAt(st.Span, "const initializer must be statically evaluable")
			}
			scope.constant[st.Name] = true
		}
		c.out.Bindings = append(c.out.Bindings, CheckedBinding{Name: st.Name, Scope: scope.name, TypeID: c.typeID(bindingType), Type: bindingType.String(), Mutable: st.Mutable, Constant: st.Const, Span: st.NameSpan})
	case assignStmt:
		mutable, exists := scope.lookupMutable(st.Name)
		if !exists {
			return c.errorAt(st.NameSpan, "cannot assign to undefined name %q", st.Name)
		}
		if !mutable {
			return c.errorAt(st.NameSpan, "cannot assign to immutable name %q", st.Name)
		}
		value, err := c.checkExpr(st.Expr, scope)
		if err != nil {
			return err
		}
		want, _ := scope.lookup(st.Name)
		if _, err := c.requireAssignable(want, value.typ, st.Expr); err != nil {
			return c.errorAt(st.Span, "assignment to %s requires %s", st.Name, want.String())
		}
	case funcStmt:
		return c.checkFunction(st, scope)
	case traitStmt, implStmt:
		if impl, ok := stmt.(implStmt); ok {
			for _, method := range impl.Methods {
				if err := c.checkFunction(method, scope); err != nil {
					return err
				}
			}
		}
		return nil
	case emitStmt:
		_, err := c.checkExpr(st.Expr, scope)
		return err
	case deferStmt:
		_, err := c.checkExpr(st.Body, scope)
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
	case structStmt, typeDeclStmt, newtypeStmt, macroStmt, packageStmt, exportStmt, breakStmt, continueStmt:
		return nil
	}
	return nil
}

func (c *typeChecker) checkFunction(st funcStmt, parent *typeScope) error {
	signature, err := staticFunctionType(st)
	if err != nil {
		return c.errorAt(st.Span, "%v", err)
	}
	parent.items[functionBindingName(st)] = signature
	child := newTypeScope(parent, st.Name)
	child.items[functionBindingName(st)] = signature
	if st.ReceiverType != "" {
		child.items[st.Receiver] = staticType{name: st.ReceiverType}
	}
	for _, param := range st.TypeParams {
		child.items[param.Name] = staticType{name: param.Name}
	}
	// This source-order declaration ID scopes generic parameter identities in
	// the compilation-local type graph. It is intentionally independent of a
	// later resolver/HIR representation.
	declarationID := SymbolID(len(c.out.Functions) + 1)
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
		params[i] = CheckedBinding{Name: name, Scope: st.Name, TypeID: c.typeIDForDeclaration(paramType, st.TypeParams, declarationID), Type: paramType.String(), Span: span}
	}
	checked := CheckedFunction{
		TypeID:       c.typeIDForDeclaration(signature, st.TypeParams, declarationID),
		ReturnTypeID: c.typeIDForDeclaration(*signature.result, st.TypeParams, declarationID),
		Name:         st.Name,
		Scope:        parent.name,
		Signature:    signature.String(),
		TypeParams:   typeParamNames(st.TypeParams),
		Params:       params,
		ReturnType:   signature.result.String(),
		Span:         st.Span,
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
			result.typeID = c.typeID(result.typ)
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
		// A function's explicit result annotation provides the contextual type
		// for every default integer literal in its body, including values nested
		// under if/match alternatives. This is intentionally done at the source
		// literal boundary; canonical type identity is assigned after conversion.
		if n.defaultInteger && !c.expected.isDynamic() && !c.expected.isFunction() {
			if converted, convertErr := checkBindingType(c.expected.name, n.value, true); convertErr == nil {
				n.value, n.defaultInteger = converted, false
			}
		}
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
		object, err := c.checkExpr(n.object, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if def, ok := c.structs[object.typ.name]; ok {
			if field, exists := def.fields[n.name]; exists {
				return checkedExpr{typ: field.syntax, typeID: field.id}, nil
			}
			if method, exists := c.methods[object.typ.name][n.name]; exists {
				return checkedExpr{typ: method}, nil
			}
			return checkedExpr{}, c.errorAt(n.NameSpan, "struct %s has no field %q", object.typ.name, n.name)
		}
		if method, exists := c.methods[object.typ.name][n.name]; exists {
			return checkedExpr{typ: method}, nil
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
		if c.typeEqual(left.typ, right.typ) {
			return checkedExpr{typ: left.typ}, nil
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	case *callExpr:
		return c.checkCall(n, scope)
	case *listExpr:
		element := dynamicStaticType
		haveElement := false
		for _, item := range n.items {
			checked, err := c.checkExpr(item, scope)
			if err != nil {
				return checkedExpr{}, err
			}
			if !haveElement {
				element, haveElement = checked.typ, true
			} else {
				element = c.joinTypes(element, checked.typ)
			}
		}
		return checkedExpr{typ: staticType{name: "list", params: []staticType{element}}}, nil
	case *tupleExpr:
		items := make([]staticType, len(n.items))
		for i, item := range n.items {
			value, err := c.checkExpr(item, scope)
			if err != nil {
				return checkedExpr{}, err
			}
			items[i] = value.typ
		}
		return checkedExpr{typ: staticType{name: "tuple", params: items}}, nil
	case *structExpr:
		return c.checkStructExpr(n, scope)
	case *structUpdateExpr:
		base, err := c.checkExpr(n.Base, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if _, ok := c.structs[base.typ.name]; !ok {
			return checkedExpr{}, c.errorAt(n.Span, "struct update requires a struct value")
		}
		if err := c.checkStructFields(base.typ.name, n.Fields, scope, true); err != nil {
			return checkedExpr{}, err
		}
		return checkedExpr{typ: base.typ}, nil
	case *indexExpr:
		object, err := c.checkExpr(n.Object, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if _, err := c.checkExpr(n.Index, scope); err != nil {
			return checkedExpr{}, err
		}
		if object.typ.name == "tuple" {
			return checkedExpr{typ: dynamicStaticType}, nil
		}
		return checkedExpr{typ: dynamicStaticType}, nil
	case *blockExpr:
		child := newTypeScope(scope, scope.name)
		for _, stmt := range n.stmts {
			switch st := stmt.(type) {
			case funcStmt:
				signature, signatureErr := staticFunctionType(st)
				if signatureErr != nil {
					return checkedExpr{}, c.errorAt(st.Span, "%v", signatureErr)
				}
				child.items[functionBindingName(st)] = signature
			case implStmt:
				for _, method := range st.Methods {
					signature, signatureErr := staticFunctionType(method)
					if signatureErr != nil {
						return checkedExpr{}, c.errorAt(method.Span, "%v", signatureErr)
					}
					child.items[functionBindingName(method)] = signature
				}
			}
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
		return checkedExpr{typ: c.joinTypes(body.typ, handler.typ), alwaysReturns: body.alwaysReturns && handler.alwaysReturns}, nil
	case *propagateExpr:
		value, err := c.checkExpr(n.expr, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if value.typ.name == "result" || value.typ.name == "option" {
			return checkedExpr{typ: dynamicStaticType}, nil
		}
		return value, nil
	case *lazyExpr:
		body, err := c.checkExpr(n.Body, scope)
		if err != nil {
			return checkedExpr{}, err
		}
		return checkedExpr{typ: lazyStaticType(body.typ)}, nil
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
				result = c.joinTypes(result, value.typ)
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
			return checkedExpr{typ: c.joinTypes(left.typ, right.typ), alwaysReturns: left.alwaysReturns && right.alwaysReturns}, nil
		case "and", "or":
			for _, arg := range call.args {
				if _, err := c.checkExpr(arg, scope); err != nil {
					return checkedExpr{}, err
				}
			}
			return checkedExpr{typ: staticType{name: "bool"}}, nil
		case "spawn":
			if len(call.args) != 1 {
				return checkedExpr{}, c.errorAt(call.Span, "spawn expects exactly one expression")
			}
			body, err := c.checkExpr(call.args[0], scope)
			if err != nil {
				return checkedExpr{}, err
			}
			return checkedExpr{typ: staticType{name: "task", params: []staticType{body.typ}}}, nil
		case "await", "timeout", "race", "select", "parallel", "cancel":
			for i, arg := range call.args {
				checked, err := c.checkExpr(arg, scope)
				if err != nil {
					return checkedExpr{}, err
				}
				if name == "await" && i == 0 && checked.typ.name == "task" && len(checked.typ.params) == 1 {
					return checkedExpr{typ: checked.typ.params[0]}, nil
				}
			}
			return checkedExpr{typ: dynamicStaticType}, nil
		}
	}
	if ident, ok := call.callee.(*identExpr); ok && ident.name == "force" {
		if len(call.args) != 1 {
			return checkedExpr{}, c.errorAt(call.Span, "force expects exactly one Lazy<T> argument")
		}
		value, err := c.checkExpr(call.args[0], scope)
		if err != nil {
			return checkedExpr{}, err
		}
		if element, ok := value.typ.lazyElement(); ok {
			return checkedExpr{typ: element}, nil
		}
		return checkedExpr{}, c.errorAt(exprSpan(call.args[0]), "force expects Lazy<T>, got %s", value.typ.String())
	}

	callee, err := c.checkExpr(call.callee, scope)
	if err != nil {
		return checkedExpr{}, err
	}
	if ident, ok := call.callee.(*identExpr); ok {
		if isFutureConstructorName(ident.name) {
			actual := make([]staticType, len(call.args))
			for i, arg := range call.args {
				checked, checkErr := c.checkExpr(arg, scope)
				if checkErr != nil {
					return checkedExpr{}, checkErr
				}
				actual[i] = checked.typ
			}
			result, typeErr := futureCallStaticType(ident.name, call.typeArgs, call.args, actual)
			if typeErr != nil {
				return checkedExpr{}, c.errorAt(call.Span, "%v", typeErr)
			}
			return checkedExpr{typ: result}, nil
		}
		if fn, exists := c.functions[ident.name]; exists && len(fn.TypeParams) > 0 {
			return c.checkGenericCall(call, fn, scope)
		}
		if def, exists := c.structs[ident.name]; exists {
			if len(call.args) != len(def.order) {
				return checkedExpr{}, c.errorAt(call.Span, "%s expects %d arguments, got %d", ident.name, len(def.order), len(call.args))
			}
			for i, arg := range call.args {
				actual, err := c.checkExpr(arg, scope)
				if err != nil {
					return checkedExpr{}, err
				}
				field := def.fields[def.order[i]]
				if _, err := c.requireAssignable(field.syntax, actual.typ, arg); err != nil {
					return checkedExpr{}, c.errorAt(exprSpan(arg), "field %s.%s requires %s, got %s", ident.name, def.order[i], field.syntax.String(), actual.typ.String())
				}
			}
			return checkedExpr{typ: staticType{name: ident.name}}, nil
		}
		if target, exists := c.newtypes[ident.name]; exists {
			if len(call.args) != 1 {
				return checkedExpr{}, c.errorAt(call.Span, "%s expects 1 argument, got %d", ident.name, len(call.args))
			}
			actual, err := c.checkExpr(call.args[0], scope)
			if err != nil {
				return checkedExpr{}, err
			}
			targetType, _ := parseStaticType(target)
			if _, err := c.requireAssignable(targetType, actual.typ, call.args[0]); err != nil {
				return checkedExpr{}, c.errorAt(exprSpan(call.args[0]), "%s requires %s, got %s", ident.name, targetType.String(), actual.typ.String())
			}
			return checkedExpr{typ: staticType{name: ident.name}}, nil
		}
		if target, exists := c.aliases[ident.name]; exists {
			if len(call.args) != 1 {
				return checkedExpr{}, c.errorAt(call.Span, "%s expects 1 argument, got %d", ident.name, len(call.args))
			}
			actual, err := c.checkExpr(call.args[0], scope)
			if err != nil {
				return checkedExpr{}, err
			}
			targetType, _ := parseStaticType(target)
			if _, err := c.requireAssignable(targetType, actual.typ, call.args[0]); err != nil {
				return checkedExpr{}, c.errorAt(exprSpan(call.args[0]), "%s requires %s, got %s", ident.name, targetType.String(), actual.typ.String())
			}
			return checkedExpr{typ: targetType}, nil
		}
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

func typeParamNames(params []typeParam) []string {
	out := make([]string, len(params))
	for i := range params {
		out[i] = params[i].Name
	}
	return out
}

func (c *typeChecker) checkGenericCall(call *callExpr, fn funcStmt, scope *typeScope) (checkedExpr, error) {
	if len(call.typeArgs) != 0 && len(call.typeArgs) != len(fn.TypeParams) {
		return checkedExpr{}, c.errorAt(call.Span, "%s expects %d type arguments, got %d", fn.Name, len(fn.TypeParams), len(call.typeArgs))
	}
	signature, err := staticFunctionType(fn)
	if err != nil {
		return checkedExpr{}, c.errorAt(call.Span, "%v", err)
	}
	actuals := make([]staticType, len(call.args))
	for i, arg := range call.args {
		checked, checkErr := c.checkExpr(arg, scope)
		if checkErr != nil {
			return checkedExpr{}, checkErr
		}
		actuals[i] = checked.typ
	}
	if len(actuals) != len(signature.params) {
		return checkedExpr{}, c.errorAt(call.Span, "%s expects %d arguments, got %d", fn.Name, len(signature.params), len(actuals))
	}
	subst := make(map[string]staticType, len(fn.TypeParams))
	for i, text := range call.typeArgs {
		arg, parseErr := parseStaticType(text)
		if parseErr != nil {
			return checkedExpr{}, c.errorAt(call.Span, "invalid type argument %q: %v", text, parseErr)
		}
		subst[fn.TypeParams[i].Name] = arg
	}
	for i, expected := range signature.params {
		if err := c.inferGenericType(expected, actuals[i], subst, fn.TypeParams); err != nil {
			return checkedExpr{}, c.errorAt(exprSpan(call.args[i]), "cannot infer type arguments for %s: %v", fn.Name, err)
		}
		resolved := substituteStaticType(expected, subst)
		if _, assignErr := c.requireAssignable(resolved, actuals[i], call.args[i]); assignErr != nil {
			return checkedExpr{}, c.errorAt(exprSpan(call.args[i]), "argument %d to %s requires %s, got %s", i+1, fn.Name, resolved.String(), actuals[i].String())
		}
	}
	for _, param := range fn.TypeParams {
		arg, ok := subst[param.Name]
		if !ok || arg.isDynamic() {
			return checkedExpr{}, c.errorAt(call.Span, "cannot infer type parameter %s for %s", param.Name, fn.Name)
		}
		if param.Bound != "" {
			bound, parseErr := parseStaticType(param.Bound)
			if parseErr != nil {
				return checkedExpr{}, c.errorAt(param.Span, "invalid bound for %s: %v", param.Name, parseErr)
			}
			if !c.satisfiesGenericBound(bound, arg) {
				return checkedExpr{}, c.errorAt(call.Span, "type argument %s for %s does not satisfy bound %s", arg.String(), param.Name, bound.String())
			}
		}
	}
	return checkedExpr{typ: substituteStaticType(*signature.result, subst)}, nil
}

func (c *typeChecker) inferGenericType(expected, actual staticType, subst map[string]staticType, params []typeParam) error {
	for _, param := range params {
		if expected.name == param.Name && len(expected.params) == 0 && !expected.isFunction() {
			if previous, exists := subst[param.Name]; exists {
				if previous.name == param.Name && actual.name != param.Name && !actual.isDynamic() {
					subst[param.Name] = actual
					return nil
				}
				if !c.typeEqual(previous, actual) {
					return fmt.Errorf("type parameter %s has conflicting arguments %s and %s", param.Name, previous.String(), actual.String())
				}
			}
			if !actual.isDynamic() {
				subst[param.Name] = actual
			}
			return nil
		}
	}
	if expected.isDynamic() || actual.isDynamic() {
		return nil
	}
	if expected.isFunction() {
		if !actual.isFunction() || len(expected.params) != len(actual.params) {
			return fmt.Errorf("expected %s, got %s", expected.String(), actual.String())
		}
		for i := range expected.params {
			if err := c.inferGenericType(expected.params[i], actual.params[i], subst, params); err != nil {
				return err
			}
		}
		return c.inferGenericType(*expected.result, *actual.result, subst, params)
	}
	if expected.name != actual.name || len(expected.params) != len(actual.params) {
		return fmt.Errorf("expected %s, got %s", expected.String(), actual.String())
	}
	for i := range expected.params {
		if err := c.inferGenericType(expected.params[i], actual.params[i], subst, params); err != nil {
			return err
		}
	}
	return nil
}

func substituteStaticType(t staticType, subst map[string]staticType) staticType {
	if replacement, ok := subst[t.name]; ok && len(t.params) == 0 && t.result == nil {
		return replacement
	}
	out := t
	if len(t.params) > 0 {
		out.params = make([]staticType, len(t.params))
		for i := range t.params {
			out.params[i] = substituteStaticType(t.params[i], subst)
		}
	}
	if t.result != nil {
		result := substituteStaticType(*t.result, subst)
		out.result = &result
	}
	return out
}

func satisfiesGenericBound(bound, actual staticType) bool {
	if bound.isDynamic() || actual.isDynamic() {
		return true
	}
	if bound.name == "any" {
		return true
	}
	if bound.name == "number" {
		switch actual.name {
		case "number", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64":
			return true
		default:
			return false
		}
	}
	return false
}

func (c *typeChecker) satisfiesGenericBound(bound, actual staticType) bool {
	if satisfiesGenericBound(bound, actual) || c.typeEqual(bound, actual) {
		return true
	}
	if _, ok := c.traits[bound.name]; !ok {
		return false
	}
	return c.impls[actual.name+"::"+bound.name] != nil
}

func (c *typeChecker) checkStructExpr(expr *structExpr, scope *typeScope) (checkedExpr, error) {
	if _, ok := c.structs[expr.TypeName]; !ok {
		return checkedExpr{}, c.errorAt(expr.Span, "unknown struct %q", expr.TypeName)
	}
	if err := c.checkStructFields(expr.TypeName, expr.Fields, scope, false); err != nil {
		return checkedExpr{}, err
	}
	return checkedExpr{typ: staticType{name: expr.TypeName}}, nil
}

func (c *typeChecker) checkStructFields(typeName string, fields []structFieldExpr, scope *typeScope, update bool) error {
	def := c.structs[typeName]
	seen := make(map[string]struct{}, len(fields))
	for _, field := range fields {
		expected, ok := def.fields[field.Name]
		if !ok {
			return c.errorAt(field.Span, "struct %s has no field %q", typeName, field.Name)
		}
		if _, exists := seen[field.Name]; exists {
			return c.errorAt(field.Span, "struct initializer repeats field %q", field.Name)
		}
		seen[field.Name] = struct{}{}
		actual, err := c.checkExpr(field.Expr, scope)
		if err != nil {
			return err
		}
		if !actual.typ.isDynamic() && expected.id != 0 && actual.typeID != 0 && !c.types.Equal(expected.id, actual.typeID) {
			if _, err := c.requireAssignable(expected.syntax, actual.typ, field.Expr); err != nil {
				return c.errorAt(field.Span, "field %s.%s requires %s, got %s", typeName, field.Name, expected.syntax.String(), actual.typ.String())
			}
		}
	}
	if !update && len(seen) != len(def.fields) {
		return c.errorAt(sourceSpan{}, "struct %s initializer is missing fields", typeName)
	}
	return nil
}

func (c *typeChecker) requireAssignable(expected, actual staticType, expr Expr) (staticType, error) {
	expected = c.resolveAlias(expected)
	actual = c.resolveAlias(actual)
	if expected.isDynamic() || c.typeEqual(expected, actual) {
		return actual, nil
	}
	if expected.name == actual.name && len(expected.params) == 0 && actual.result == nil {
		return actual, nil
	}
	if _, trait := c.traits[expected.name]; trait {
		if c.impls[actual.name+"::"+expected.name] == nil {
			return actual, fmt.Errorf("type %s does not implement trait %s", actual.name, expected.name)
		}
		return expected, nil
	}
	if expected.name == actual.name && len(expected.params) > 0 && len(actual.params) == 0 && actual.result == nil {
		return expected, nil
	}
	if !expected.isFunction() && expected.name == actual.name && len(expected.params) == len(actual.params) {
		for i := range expected.params {
			if actual.params[i].isDynamic() {
				continue
			}
			if !c.typeEqual(expected.params[i], actual.params[i]) {
				return actual, fmt.Errorf("parameterized types are invariant")
			}
		}
		return expected, nil
	}
	if outcomes := staticOutcomeExpressions(expr); len(outcomes) > 0 {
		for _, outcome := range outcomes {
			if literal, ok := outcome.(*literalExpr); ok && literal.defaultInteger && !expected.isFunction() {
				converted, convertErr := checkBindingType(expected.name, literal.value, true)
				if convertErr == nil {
					literal.value, literal.defaultInteger = converted, false
					continue
				}
			}
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

func (c *typeChecker) resolveAlias(t staticType) staticType {
	seen := make(map[string]struct{})
	for {
		target, ok := c.aliases[t.name]
		if !ok {
			return t
		}
		if _, exists := seen[t.name]; exists {
			return t
		}
		seen[t.name] = struct{}{}
		parsed, err := parseStaticType(target)
		if err != nil {
			return t
		}
		t = parsed
	}
}

// staticOutcomeExpressions returns the value-producing alternatives of a
// control-flow expression. Keeping these alternatives visible at an annotated
// boundary prevents a known branch mismatch from being mistaken for an
// unknowable dynamic value, and lets default integer literals inherit the
// boundary's exact width.
func staticOutcomeExpressions(expr Expr) []Expr {
	var flatten func([]Expr) []Expr
	flatten = func(expressions []Expr) []Expr {
		var out []Expr
		for _, expression := range expressions {
			if nested := staticOutcomeExpressions(expression); len(nested) > 0 {
				out = append(out, flatten(nested)...)
			} else {
				out = append(out, expression)
			}
		}
		return out
	}
	switch n := expr.(type) {
	case *callExpr:
		if name, ok := specialFormName(n.callee); ok && name == "if" && len(n.args) == 3 {
			return flatten([]Expr{n.args[1], n.args[2]})
		}
	case *tryExpr:
		return []Expr{n.body, n.handler}
	case *matchExpr:
		out := make([]Expr, 0, len(n.arms))
		for _, arm := range n.arms {
			out = append(out, arm.body)
		}
		return flatten(out)
	case *blockExpr:
		if n.tail != nil {
			return []Expr{n.tail}
		}
	}
	return nil
}

// runtimeStaticTypesEqual belongs to the Value/runtime compatibility boundary.
// Compiler semantic equality uses typeChecker.typeEqual and TypeID instead.
func runtimeStaticTypesEqual(left, right staticType) bool {
	if left.isDynamic() || right.isDynamic() {
		return left.isDynamic() && right.isDynamic()
	}
	if left.name != right.name || len(left.params) != len(right.params) {
		return false
	}
	for i := range left.params {
		if !runtimeStaticTypesEqual(left.params[i], right.params[i]) {
			return false
		}
	}
	if left.result == nil || right.result == nil {
		return left.result == nil && right.result == nil
	}
	return runtimeStaticTypesEqual(*left.result, *right.result)
}

func joinRuntimeStaticTypes(left, right staticType) staticType {
	if runtimeStaticTypesEqual(left, right) {
		return left
	}
	return dynamicStaticType
}

func builtinCallType(name string, args []Expr) (staticType, bool) {
	if name != "map" && name != "set" && isBindingTypeName(name) && name != "function" && name != "module" {
		return staticType{name: name}, true
	}
	switch name {
	case "map_new":
		return staticType{name: "map", params: []staticType{dynamicStaticType, dynamicStaticType}}, true
	case "map_set":
		return staticType{name: "map", params: []staticType{dynamicStaticType, dynamicStaticType}}, true
	case "map_get":
		return staticType{name: "option", params: []staticType{dynamicStaticType}}, true
	case "map_keys":
		return staticType{name: "list", params: []staticType{dynamicStaticType}}, true
	case "map_values":
		return staticType{name: "list", params: []staticType{dynamicStaticType}}, true
	case "set_new", "set_add":
		return staticType{name: "set", params: []staticType{dynamicStaticType}}, true
	case "set_has":
		return staticType{name: "bool"}, true
	case "set_values":
		return staticType{name: "list", params: []staticType{dynamicStaticType}}, true
	case "cat", "string", "text", "lower", "upper", "trim", "replace", "repeat", "join":
		return staticType{name: "string"}, true
	case "len":
		return staticType{name: "u64"}, true
	case "ok", "err":
		if len(args) == 1 {
			if name == "ok" {
				return staticType{name: "result", params: []staticType{dynamicStaticType, staticType{name: "error"}}}, true
			}
			return staticType{name: "result", params: []staticType{dynamicStaticType, dynamicStaticType}}, true
		}
		return staticType{name: "result", params: []staticType{dynamicStaticType, dynamicStaticType}}, true
	case "some", "none":
		if name == "some" && len(args) == 1 {
			return staticType{name: "option", params: []staticType{dynamicStaticType}}, true
		}
		return staticType{name: "option", params: []staticType{dynamicStaticType}}, true
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
	case kindTuple:
		params := make([]staticType, len(value.tuple))
		for i, item := range value.tuple {
			params[i] = staticTypeFromValue(item)
		}
		return staticType{name: "tuple", params: params}
	case kindList:
		element := dynamicStaticType
		if len(value.list) > 0 {
			element = staticTypeFromValue(value.list[0])
			for _, item := range value.list[1:] {
				element = joinRuntimeStaticTypes(element, staticTypeFromValue(item))
			}
		}
		return staticType{name: "list", params: []staticType{element}}
	case kindResult:
		if value.res != nil && value.res.ok {
			return staticType{name: "result", params: []staticType{staticTypeFromValue(value.res.val), staticType{name: "error"}}}
		}
		return staticType{name: "result", params: []staticType{dynamicStaticType, staticType{name: "error"}}}
	case kindOption:
		if value.opt != nil && value.opt.present {
			return staticType{name: "option", params: []staticType{staticTypeFromValue(value.opt.val)}}
		}
		return staticType{name: "option", params: []staticType{dynamicStaticType}}
	case kindMap:
		keyType, valueType := dynamicStaticType, dynamicStaticType
		if value.mapValue != nil && len(value.mapValue.entries) > 0 {
			keyType = staticTypeFromValue(value.mapValue.entries[0].key)
			valueType = staticTypeFromValue(value.mapValue.entries[0].value)
			for _, entry := range value.mapValue.entries[1:] {
				keyType = joinRuntimeStaticTypes(keyType, staticTypeFromValue(entry.key))
				valueType = joinRuntimeStaticTypes(valueType, staticTypeFromValue(entry.value))
			}
		}
		return staticType{name: "map", params: []staticType{keyType, valueType}}
	case kindSet:
		element := dynamicStaticType
		if value.setValue != nil && len(value.setValue.items) > 0 {
			element = staticTypeFromValue(value.setValue.items[0])
			for _, item := range value.setValue.items[1:] {
				element = joinRuntimeStaticTypes(element, staticTypeFromValue(item))
			}
		}
		return staticType{name: "set", params: []staticType{element}}
	case kindStruct:
		if value.structType != nil {
			return staticType{name: value.structType.name}
		}
		return staticType{name: "struct"}
	case kindFuture:
		if value.future == nil {
			return staticType{name: "future"}
		}
		params := make([]staticType, len(value.future.params))
		for i, param := range value.future.params {
			params[i] = staticType{name: param}
		}
		return staticType{name: value.future.name, params: params}
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
		case structPattern:
			for _, field := range p.fields {
				walk(field.pattern)
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
