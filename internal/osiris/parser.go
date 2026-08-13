package osiris

import (
	"fmt"
	"strconv"
	"strings"
)

type Program struct {
	SourceName string
	Statements []Stmt
}

type Stmt interface {
	stmtNode()
}

type letStmt struct {
	Name     string
	Names    []string
	Type     string
	Expr     Expr
	Unwrap   bool
	Mutable  bool
	Const    bool
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode operates on letStmt values and keeps the letstmt-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (letStmt) stmtNode() {}

type assignStmt struct {
	Name     string
	Expr     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

func (assignStmt) stmtNode() {}

type emitStmt struct {
	Expr Expr
	Span sourceSpan
}

// stmtNode operates on emitStmt values and keeps the emitstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (emitStmt) stmtNode() {}

type deferStmt struct {
	Body Expr
	Span sourceSpan
}

func (deferStmt) stmtNode() {}

type macroStmt struct {
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode operates on macroStmt values and keeps the macrostmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (macroStmt) stmtNode() {}

type funcStmt struct {
	Name           string
	Receiver       string
	ReceiverType   string
	TypeParams     []typeParam
	Params         []string
	ParamTypes     []string
	ParamSpans     []sourceSpan
	ReturnType     string
	ReturnTypeSpan sourceSpan
	Body           Expr
	Span           sourceSpan
	NameSpan       sourceSpan
}

// traitStmt declares a nominal behavioral contract. Method signatures are
// intentionally represented with funcStmt so the checker and tooling share
// the same parameter and return-type surface as ordinary functions.
type traitStmt struct {
	Name       string
	TypeParams []typeParam
	Methods    []funcStmt
	Span       sourceSpan
	NameSpan   sourceSpan
}

func (traitStmt) stmtNode() {}

// implStmt is expanded by the runtime and checker as a deterministic group of
// receiver methods. Keeping the group in the AST preserves source structure
// for formatting and diagnostics.
type implStmt struct {
	Trait      string
	Target     string
	Methods    []funcStmt
	Span       sourceSpan
	TraitSpan  sourceSpan
	TargetSpan sourceSpan
}

func (implStmt) stmtNode() {}

// typeParam describes one generic function parameter. Bounds are ordinary
// Osiris type expressions; an omitted bound means any concrete type.
type typeParam struct {
	Name  string
	Bound string
	Span  sourceSpan
}

// stmtNode operates on funcStmt values and keeps the funcstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (funcStmt) stmtNode() {}

type returnStmt struct {
	Expr Expr
	Span sourceSpan
}

// stmtNode operates on returnStmt values and keeps the returnstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (returnStmt) stmtNode() {}

type whileStmt struct {
	Cond Expr
	Body Expr
	Span sourceSpan
}

// stmtNode operates on whileStmt values and keeps the whilestmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (whileStmt) stmtNode() {}

type forStmt struct {
	Name     string
	Iter     Expr
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode operates on forStmt values and keeps the forstmt-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (forStmt) stmtNode() {}

type breakStmt struct {
	Span sourceSpan
}

// stmtNode operates on breakStmt values and keeps the breakstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (breakStmt) stmtNode() {}

type continueStmt struct {
	Span sourceSpan
}

// stmtNode operates on continueStmt values and keeps the continuestmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (continueStmt) stmtNode() {}

type importStmt struct {
	Path      string
	Alias     string
	Span      sourceSpan
	PathSpan  sourceSpan
	AliasSpan sourceSpan
}

// stmtNode operates on importStmt values and keeps the importstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (importStmt) stmtNode() {}

type packageStmt struct {
	Name        string
	Version     string
	Span        sourceSpan
	NameSpan    sourceSpan
	VersionSpan sourceSpan
}

// stmtNode operates on packageStmt values and keeps the packagestmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (packageStmt) stmtNode() {}

type exportStmt struct {
	Names     []string
	Span      sourceSpan
	NameSpans []sourceSpan
}

// stmtNode operates on exportStmt values and keeps the exportstmt-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (exportStmt) stmtNode() {}

type enumStmt struct {
	Name     string
	Backing  string
	Cases    []enumCase
	Span     sourceSpan
	NameSpan sourceSpan
}

type enumCase struct {
	Name         string
	Params       []string
	BackingValue *Value
	Span         sourceSpan
}

func (c enumCase) Arity() int { return len(c.Params) }

func isEnumBackingType(name string) bool {
	switch name {
	case "string", "int", "integer", "i8", "i16", "i32", "i64", "u8", "u16", "u32", "u64", "byte":
		return true
	default:
		return false
	}
}

func matchesEnumBackingType(backing string, value Value) bool {
	if backing == "string" {
		return value.kind == kindString
	}
	return value.kind == kindInt || value.kind == kindUint
}

func isDefaultIntegerLiteral(text string) bool {
	lower := strings.ToLower(text)
	for _, suffix := range []string{"u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64", "bf16", "bf32", "bf64", "n"} {
		if strings.HasSuffix(lower, suffix) {
			return false
		}
	}
	return !strings.ContainsAny(lower, ".e")
}

func (enumStmt) stmtNode() {}

type structField struct {
	Name      string
	Type      string
	TypeSpan  sourceSpan
	Tag       string
	TagValues map[string]string
	Span      sourceSpan
	NameSpan  sourceSpan
}

type structStmt struct {
	Name     string
	Fields   []structField
	Span     sourceSpan
	NameSpan sourceSpan
}

func (structStmt) stmtNode() {}

type typeDeclStmt struct {
	Name     string
	Target   string
	Alias    bool
	Span     sourceSpan
	NameSpan sourceSpan
}

func (typeDeclStmt) stmtNode() {}

type newtypeStmt struct {
	Name     string
	Target   string
	Span     sourceSpan
	NameSpan sourceSpan
}

func (newtypeStmt) stmtNode() {}

type Expr interface {
	exprNode()
}

type literalExpr struct {
	value          Value
	defaultInteger bool
	Span           sourceSpan
}

// exprNode operates on literalExpr values and keeps the literalexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (literalExpr) exprNode() {}

type identExpr struct {
	name string
	Span sourceSpan
}

// exprNode operates on identExpr values and keeps the identexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (identExpr) exprNode() {}

type unaryExpr struct {
	op   tokenKind
	expr Expr
	Span sourceSpan
}

// exprNode operates on unaryExpr values and keeps the unaryexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (unaryExpr) exprNode() {}

type binaryExpr struct {
	op    tokenKind
	left  Expr
	right Expr
	Span  sourceSpan
}

// exprNode operates on binaryExpr values and keeps the binaryexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (binaryExpr) exprNode() {}

type callExpr struct {
	callee   Expr
	args     []Expr
	typeArgs []string
	Span     sourceSpan
}

// exprNode operates on callExpr values and keeps the callexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (callExpr) exprNode() {}

type blockExpr struct {
	stmts []Stmt
	tail  Expr
	Span  sourceSpan
}

// exprNode operates on blockExpr values and keeps the blockexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (blockExpr) exprNode() {}

// lazyExpr is an explicit deferred computation. Its body is deliberately an
// AST node rather than a parser-synthesized call so each backend can lower the
// lazy boundary using its own runtime representation.
type lazyExpr struct {
	Body Expr
	Span sourceSpan
}

func (lazyExpr) exprNode() {}

type tryExpr struct {
	body      Expr
	catchName string
	handler   Expr
	Span      sourceSpan
}

// exprNode operates on tryExpr values and keeps the tryexpr-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (tryExpr) exprNode() {}

type propagateExpr struct {
	expr Expr
	Span sourceSpan
}

// exprNode operates on propagateExpr values and keeps the propagateexpr-
// specific behavior attached to the receiver so callers can use the method as
// the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (propagateExpr) exprNode() {}

type memberExpr struct {
	object   Expr
	name     string
	Span     sourceSpan
	NameSpan sourceSpan
}

// exprNode operates on memberExpr values and keeps the memberexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (memberExpr) exprNode() {}

type listExpr struct {
	items []Expr
	Span  sourceSpan
}

// exprNode operates on listExpr values and keeps the listexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (listExpr) exprNode() {}

type tupleExpr struct {
	items []Expr
	Span  sourceSpan
}

func (tupleExpr) exprNode() {}

type structFieldExpr struct {
	Name string
	Expr Expr
	Span sourceSpan
}

type structExpr struct {
	TypeName string
	Fields   []structFieldExpr
	Span     sourceSpan
}

func (structExpr) exprNode() {}

type structUpdateExpr struct {
	Base   Expr
	Fields []structFieldExpr
	Span   sourceSpan
}

func (structUpdateExpr) exprNode() {}

type indexExpr struct {
	Object Expr
	Index  Expr
	Span   sourceSpan
}

func (indexExpr) exprNode() {}

type matchExpr struct {
	subject Expr
	arms    []matchArm
	Span    sourceSpan
	keyword tokenKind
}

// exprNode operates on matchExpr values and keeps the matchexpr-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (matchExpr) exprNode() {}

type matchArm struct {
	pattern   Pattern
	guard     Expr
	body      Expr
	isDefault bool
}

type Pattern interface {
	patternNode()
}

type wildcardPattern struct {
	Span sourceSpan
}

// patternNode operates on wildcardPattern values and keeps the
// wildcardpattern-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (wildcardPattern) patternNode() {}

type bindPattern struct {
	name string
	Span sourceSpan
}

// patternNode operates on bindPattern values and keeps the
// bindpattern-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (bindPattern) patternNode() {}

// enumPattern names a zero-payload case of a nominal enum.  It deliberately
// carries names rather than a runtime value so a match remains independent of
// evaluation order and can be lowered by every execution engine.
type enumPattern struct {
	typeName string
	caseName string
	items    []Pattern
	Span     sourceSpan
}

func (enumPattern) patternNode() {}

type orPattern struct {
	alternatives []Pattern
	Span         sourceSpan
}

func (orPattern) patternNode() {}

type andPattern struct {
	left  Pattern
	right Pattern
	Span  sourceSpan
}

func (andPattern) patternNode() {}

type notPattern struct {
	inner Pattern
	Span  sourceSpan
}

func (notPattern) patternNode() {}

type typePattern struct {
	typeName string
	binding  string
	Span     sourceSpan
}

func (typePattern) patternNode() {}

type relationalPattern struct {
	op    tokenKind
	value Value
	Span  sourceSpan
}

func (relationalPattern) patternNode() {}

type literalPattern struct {
	value Value
	Span  sourceSpan
}

// patternNode operates on literalPattern values and keeps the
// literalpattern-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (literalPattern) patternNode() {}

type listPattern struct {
	items []Pattern
	Span  sourceSpan
}

// patternNode operates on listPattern values and keeps the
// listpattern-specific behavior attached to the receiver so callers can use
// the method as the natural entry point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (listPattern) patternNode() {}

// structPattern matches a named product/struct by nominal type and by the
// explicitly listed fields. Omitted fields are intentionally unconstrained;
// each listed field is matched recursively using the same pattern language as
// enum and list payloads.
type structPattern struct {
	typeName string
	fields   []structPatternField
	Span     sourceSpan
}

type structPatternField struct {
	name    string
	pattern Pattern
	Span    sourceSpan
}

func (structPattern) patternNode() {}

// Parse parses the supplied work and returns the structured form that the
// evaluator, optimizer, or analyzer expects to work on.
//
// It returns *Program and an error. Callers should only trust the value when
// the error is nil.
func Parse(sourceName, src string) (*Program, error) {
	prog, _, _, err := parseWithDeclarationContext(sourceName, src, builtinTypeSet(), nil)
	return prog, err
}

// parseWithDeclarationContext parses one source fragment using declaration
// names accumulated by a long-lived frontend such as the REPL. The returned
// maps are updated only when parsing succeeds; callers can therefore keep the
// previous context intact after a malformed submission.
func parseWithDeclarationContext(sourceName, src string, knownTypes, knownStructs map[string]struct{}) (*Program, map[string]struct{}, map[string]struct{}, error) {
	types := cloneStringSet(knownTypes)
	if len(types) == 0 {
		types = builtinTypeSet()
	}
	structs := cloneStringSet(knownStructs)
	p := &parser{sourceName: sourceName, lexer: newLexer(src), knownTypes: types, knownStructs: structs}
	prog, err := p.parse()
	if err != nil {
		return nil, nil, nil, normalizeError(err, sourceName)
	}
	return prog, p.knownTypes, p.knownStructs, nil
}

func cloneStringSet(values map[string]struct{}) map[string]struct{} {
	clone := make(map[string]struct{}, len(values))
	for name := range values {
		clone[name] = struct{}{}
	}
	return clone
}

// parserSpanFromToken returns a byte-column source span for one token. The
// lexer keeps semantic token text separate from raw source spelling so decoded
// strings and field tags retain their complete source width.
func parserSpanFromToken(tok token) sourceSpan {
	text := tok.raw
	if text == "" {
		text = tok.text
	}
	return sourceSpan{
		start: sourcePos{line: tok.line, col: tok.col},
		end:   sourcePos{line: tok.line, col: tok.col + len(text)},
	}
}

func parserSpanFromTokens(start, end token) sourceSpan {
	return spanCover(parserSpanFromToken(start), parserSpanFromToken(end))
}

type parser struct {
	sourceName   string
	lexer        *lexer
	knownTypes   map[string]struct{}
	knownStructs map[string]struct{}
	cur          token
	peeked       bool
	lookahead    token
	hasLookahead bool
	lexErr       error
	typeArgDepth int
}

// parse operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns *Program and an error. Callers should only trust the value when
// the error is nil.
func (p *parser) parse() (*Program, error) {
	var stmts []Stmt
	for {
		if p.lexErr != nil {
			return nil, p.lexErr
		}
		if p.match(tokenEOF) {
			break
		}
		if p.match(tokenSemicolon) {
			continue
		}
		stmt, err := p.parseStmt()
		if err != nil {
			return nil, err
		}
		stmts = append(stmts, stmt)
		for p.match(tokenSemicolon) {
		}
	}
	if p.lexErr != nil {
		return nil, p.lexErr
	}
	return &Program{SourceName: p.sourceName, Statements: stmts}, nil
}

// parseStmt operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Stmt and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseStmt() (Stmt, error) {
	if tok := p.peek(); tok.kind == tokenIdent {
		if tok.text == "var" && p.peekSecond().kind != tokenEqual {
			// var is contextual so var(...) remains available as the statistical
			// builtin in expression position.
			p.cur.kind = tokenVar
		}
		if p.peekSecond().kind == tokenEqual {
			name := p.next()
			p.next()
			expr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			return assignStmt{Name: name.text, Expr: expr, Span: spanCover(parserSpanFromToken(name), exprSpan(expr)), NameSpan: parserSpanFromToken(name)}, nil
		}
		switch tok.text {
		case "trait", "interface", "impl":
			return p.parseBehaviorStmt()
		}
	}
	switch p.peek().kind {
	case tokenLet, tokenVar, tokenConst:
		start := p.next()
		mutable := start.kind == tokenVar
		constant := start.kind == tokenConst
		unwrap := p.match(tokenQuestion)
		if p.match(tokenLParen) {
			names, err := p.parseNameListAllowEmpty()
			if err != nil {
				return nil, err
			}
			if _, err := p.expect(tokenEqual); err != nil {
				return nil, err
			}
			expr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if mutable || constant {
				return nil, p.errorf(start, "%s does not support tuple destructuring", start.text)
			}
			return letStmt{Names: names, Expr: expr, Unwrap: unwrap, Span: spanCover(parserSpanFromToken(start), exprSpan(expr))}, nil
		}
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		typeName := ""
		if isTypeStart(p.peek().kind) {
			var typeSpan sourceSpan
			typeName, typeSpan, err = p.parseTypeName()
			if err != nil {
				return nil, err
			}
			_ = typeSpan
		}
		if _, err := p.expect(tokenEqual); err != nil {
			return nil, err
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if unwrap {
			if literal, ok := expr.(*literalExpr); ok && literal.value.kind != kindResult && literal.value.kind != kindOption {
				return nil, p.errorf(nameTok, "let? requires a result or option expression")
			}
		}
		if typeName != "" {
			if literal, ok := expr.(*literalExpr); ok {
				value, typeErr := checkBindingType(typeName, literal.value, literal.defaultInteger)
				if typeErr != nil {
					return nil, p.errorf(nameTok, "%v", typeErr)
				}
				literal.value = value
				literal.defaultInteger = false
			}
		}
		return letStmt{Name: nameTok.text, Type: typeName, Expr: expr, Unwrap: unwrap, Mutable: mutable, Const: constant, Span: spanCover(parserSpanFromToken(start), exprSpan(expr)), NameSpan: parserSpanFromToken(nameTok)}, nil
	case tokenEmit:
		start := p.next()
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		exprs := []Expr{expr}
		for p.match(tokenComma) {
			nextExpr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			exprs = append(exprs, nextExpr)
		}
		if len(exprs) == 1 {
			return emitStmt{Expr: expr, Span: spanCover(parserSpanFromToken(start), exprSpan(expr))}, nil
		}
		args := make([]Expr, 0, len(exprs)*2-1)
		for i, item := range exprs {
			if i > 0 {
				args = append(args, &literalExpr{value: stringValue(", "), Span: parserSpanFromToken(start)})
			}
			args = append(args, item)
		}
		return emitStmt{
			Expr: &callExpr{
				callee: &identExpr{name: "cat", Span: parserSpanFromToken(start)},
				args:   args,
				Span:   spanCover(parserSpanFromToken(start), exprSpan(exprs[len(exprs)-1])),
			},
			Span: spanCover(parserSpanFromToken(start), exprSpan(exprs[len(exprs)-1])),
		}, nil
	case tokenDefer:
		start := p.next()
		body, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return deferStmt{Body: body, Span: spanCover(parserSpanFromToken(start), exprSpan(body))}, nil
	case tokenMacro:
		start := p.next()
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenLParen); err != nil {
			return nil, err
		}
		params, err := p.parseNameListAllowEmpty()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenArrow); err != nil {
			return nil, err
		}
		body, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return macroStmt{Name: nameTok.text, Params: params, Body: body, Span: spanCover(parserSpanFromToken(start), exprSpan(body)), NameSpan: parserSpanFromToken(nameTok)}, nil
	case tokenFunc:
		return p.parseFunctionStmt("", "", true)
	case tokenReturn:
		start := p.next()
		if p.peek().kind == tokenSemicolon || p.peek().kind == tokenRBrace || p.peek().kind == tokenEOF {
			return returnStmt{Span: parserSpanFromToken(start)}, nil
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return returnStmt{Expr: expr, Span: spanCover(parserSpanFromToken(start), exprSpan(expr))}, nil
	case tokenWhile:
		start := p.next()
		cond, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		body, err := p.parseBlockExpr()
		if err != nil {
			return nil, err
		}
		return whileStmt{Cond: cond, Body: body, Span: spanCover(parserSpanFromToken(start), exprSpan(body))}, nil
	case tokenFor:
		start := p.next()
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenIn); err != nil {
			return nil, err
		}
		iter, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		body, err := p.parseBlockExpr()
		if err != nil {
			return nil, err
		}
		return forStmt{Name: nameTok.text, Iter: iter, Body: body, Span: spanCover(parserSpanFromToken(start), exprSpan(body)), NameSpan: parserSpanFromToken(nameTok)}, nil
	case tokenBreak:
		start := p.next()
		return breakStmt{Span: parserSpanFromToken(start)}, nil
	case tokenContinue:
		start := p.next()
		return continueStmt{Span: parserSpanFromToken(start)}, nil
	case tokenPackage:
		start := p.next()
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		stmt := packageStmt{Name: nameTok.text, Span: parserSpanFromTokens(start, nameTok), NameSpan: parserSpanFromToken(nameTok)}
		if p.peek().kind == tokenNumber || p.peek().kind == tokenString {
			verTok := p.next()
			stmt.Version = verTok.text
			stmt.VersionSpan = parserSpanFromToken(verTok)
			stmt.Span = spanCover(stmt.Span, stmt.VersionSpan)
		}
		return stmt, nil
	case tokenExport:
		start := p.next()
		first, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		names := []string{first.text}
		spans := []sourceSpan{parserSpanFromToken(first)}
		for p.match(tokenComma) {
			tok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			names = append(names, tok.text)
			spans = append(spans, parserSpanFromToken(tok))
		}
		return exportStmt{Names: names, Span: spanCover(parserSpanFromToken(start), spans[len(spans)-1]), NameSpans: spans}, nil
	case tokenEnum:
		start := p.next()
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		backing := ""
		if p.match(tokenColon) {
			backingTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			if !isEnumBackingType(backingTok.text) {
				return nil, p.errorf(backingTok, "enum backing type must be string or an integer family")
			}
			backing = backingTok.text
		}
		if _, err := p.expect(tokenLBrace); err != nil {
			return nil, err
		}
		var cases []enumCase
		seen := make(map[string]struct{})
		backedValues := make([]Value, 0, 4)
		for p.peek().kind != tokenRBrace {
			caseTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			if _, exists := seen[caseTok.text]; exists {
				return nil, p.errorf(caseTok, "enum %q declares case %q more than once", name.text, caseTok.text)
			}
			seen[caseTok.text] = struct{}{}
			item := enumCase{Name: caseTok.text, Span: parserSpanFromToken(caseTok)}
			if p.match(tokenLParen) && !p.match(tokenRParen) {
				if backing != "" {
					return nil, p.errorf(caseTok, "backed enum case %s.%s cannot declare a payload", name.text, caseTok.text)
				}
				params, err := p.parseNameListAllowEmpty()
				if err != nil {
					return nil, err
				}
				item.Params = params
			}
			if backing != "" {
				if _, err := p.expect(tokenEqual); err != nil {
					return nil, p.errorf(caseTok, "backed enum case %s.%s requires a value", name.text, caseTok.text)
				}
				expr, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				literal, ok := expr.(*literalExpr)
				if !ok || !matchesEnumBackingType(backing, literal.value) {
					return nil, p.errorf(caseTok, "backed enum case %s.%s value must be a %s literal", name.text, caseTok.text, backing)
				}
				for _, previous := range backedValues {
					if previous.Equal(literal.value) {
						return nil, p.errorf(caseTok, "backed enum %s reuses value %s", name.text, literal.value.String())
					}
				}
				value := literal.value
				item.BackingValue = &value
				backedValues = append(backedValues, value)
			} else if p.peek().kind == tokenEqual {
				return nil, p.errorf(caseTok, "enum case values require an enum backing type")
			}
			cases = append(cases, item)
			if !p.match(tokenComma) {
				break
			}
		}
		end, err := p.expect(tokenRBrace)
		if err != nil {
			return nil, err
		}
		if len(cases) == 0 {
			return nil, p.errorf(name, "enum %q must declare at least one case", name.text)
		}
		return enumStmt{Name: name.text, Backing: backing, Cases: cases, NameSpan: parserSpanFromToken(name), Span: parserSpanFromTokens(start, end)}, nil
	case tokenStruct:
		return p.parseStructStmt()
	case tokenType:
		start := p.next()
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenEqual); err != nil {
			return nil, err
		}
		target, targetSpan, err := p.parseTypeName()
		if err != nil {
			return nil, err
		}
		p.knownTypes[name.text] = struct{}{}
		if _, isStruct := p.knownStructs[target]; isStruct {
			p.knownStructs[name.text] = struct{}{}
		}
		return typeDeclStmt{Name: name.text, Target: target, Alias: true, NameSpan: parserSpanFromToken(name), Span: spanCover(parserSpanFromToken(start), targetSpan)}, nil
	case tokenNewtype:
		start := p.next()
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		target, targetSpan, err := p.parseTypeName()
		if err != nil {
			return nil, err
		}
		p.knownTypes[name.text] = struct{}{}
		return newtypeStmt{Name: name.text, Target: target, NameSpan: parserSpanFromToken(name), Span: spanCover(parserSpanFromToken(start), targetSpan)}, nil
	case tokenImport:
		start := p.next()
		pathTok, err := p.expect(tokenString)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenAs); err != nil {
			return nil, err
		}
		aliasTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		return importStmt{Path: pathTok.text, Alias: aliasTok.text, Span: parserSpanFromTokens(start, aliasTok), PathSpan: parserSpanFromToken(pathTok), AliasSpan: parserSpanFromToken(aliasTok)}, nil
	case tokenFFI:
		start := p.next()
		pathTok, err := p.expect(tokenString)
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenAs); err != nil {
			return nil, err
		}
		aliasTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		return letStmt{
			Name: aliasTok.text,
			Expr: &callExpr{callee: &identExpr{name: "ffi_load", Span: parserSpanFromToken(start)}, args: []Expr{&literalExpr{value: stringValue(pathTok.text), Span: parserSpanFromToken(pathTok)}}, Span: parserSpanFromTokens(start, aliasTok)},
			Span: parserSpanFromTokens(start, aliasTok), NameSpan: parserSpanFromToken(aliasTok),
		}, nil
	default:
		return nil, p.errorf(p.peek(), "expected statement starting with let, var, ffi, emit, defer, func, return, while, for, break, continue, package, export, enum, struct, type, newtype, or import")
	}
}

func (p *parser) parseBehaviorStmt() (Stmt, error) {
	keyword := p.next()
	if keyword.text == "trait" || keyword.text == "interface" {
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		params, err := p.parseTypeParams()
		if err != nil {
			return nil, err
		}
		p.knownTypes[name.text] = struct{}{}
		if _, err := p.expect(tokenLBrace); err != nil {
			return nil, err
		}
		var methods []funcStmt
		for p.peek().kind != tokenRBrace {
			method, err := p.parseFunctionStmt("", "", false)
			if err != nil {
				return nil, err
			}
			methods = append(methods, method)
			if !p.match(tokenComma) {
				p.match(tokenSemicolon)
			}
		}
		end, err := p.expect(tokenRBrace)
		if err != nil {
			return nil, err
		}
		return traitStmt{Name: name.text, TypeParams: params, Methods: methods, Span: parserSpanFromTokens(keyword, end), NameSpan: parserSpanFromToken(name)}, nil
	}

	trait, err := p.expect(tokenIdent)
	if err != nil {
		return nil, err
	}
	forTok, err := p.expect(tokenFor)
	if err != nil {
		return nil, err
	}
	if forTok.text != "for" {
		return nil, p.errorf(forTok, "impl requires 'for' between trait and target type")
	}
	target, targetSpan, err := p.parseTypeName()
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenLBrace); err != nil {
		return nil, err
	}
	var methods []funcStmt
	for p.peek().kind != tokenRBrace {
		method, err := p.parseFunctionStmt("self", target, true)
		if err != nil {
			return nil, err
		}
		methods = append(methods, method)
		p.match(tokenComma)
		p.match(tokenSemicolon)
	}
	end, err := p.expect(tokenRBrace)
	if err != nil {
		return nil, err
	}
	return implStmt{Trait: trait.text, Target: target, Methods: methods, Span: parserSpanFromTokens(trait, end), TraitSpan: parserSpanFromToken(trait), TargetSpan: targetSpan}, nil
}

func (p *parser) parseFunctionStmt(receiver, receiverType string, bodyRequired bool) (funcStmt, error) {
	start := p.peek()
	if _, err := p.expect(tokenFunc); err != nil {
		return funcStmt{}, err
	}
	if p.match(tokenLParen) {
		receiverTok, err := p.expect(tokenIdent)
		if err != nil {
			return funcStmt{}, err
		}
		typ, _, err := p.parseTypeName()
		if err != nil {
			return funcStmt{}, err
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return funcStmt{}, err
		}
		if receiver != "" && (receiverTok.text != receiver || typ != receiverType) {
			return funcStmt{}, p.errorf(receiverTok, "impl receiver must be %s %s", receiver, receiverType)
		}
		receiver, receiverType = receiverTok.text, typ
	}
	nameTok, err := p.expect(tokenIdent)
	if err != nil {
		return funcStmt{}, err
	}
	typeParams, err := p.parseTypeParams()
	if err != nil {
		return funcStmt{}, err
	}
	if _, err := p.expect(tokenLParen); err != nil {
		return funcStmt{}, err
	}
	params, paramTypes, paramSpans, err := p.parseFunctionParams()
	if err != nil {
		return funcStmt{}, err
	}
	returnType := ""
	var returnTypeSpan sourceSpan
	if isTypeStart(p.peek().kind) {
		returnType, returnTypeSpan, err = p.parseTypeName()
		if err != nil {
			return funcStmt{}, err
		}
	}
	var body Expr
	if p.match(tokenArrow) {
		body, err = p.parseExpr(0)
	} else if bodyRequired {
		body, err = p.parseBlockExpr()
	}
	if err != nil {
		return funcStmt{}, err
	}
	endSpan := returnTypeSpan
	if body != nil {
		endSpan = exprSpan(body)
	}
	return funcStmt{Name: nameTok.text, Receiver: receiver, ReceiverType: receiverType, TypeParams: typeParams,
		Params: params, ParamTypes: paramTypes, ParamSpans: paramSpans, ReturnType: returnType,
		ReturnTypeSpan: returnTypeSpan, Body: body, Span: spanCover(parserSpanFromToken(start), endSpan), NameSpan: parserSpanFromToken(nameTok)}, nil
}

func (p *parser) parseStructStmt() (Stmt, error) {
	start := p.next()
	name, err := p.expect(tokenIdent)
	if err != nil {
		return nil, err
	}
	// Register before parsing fields so a declaration may contain a recursive
	// field reference. Recursive values are still rejected by construction;
	// this only makes the type name available to the parser and checker.
	p.knownTypes[name.text] = struct{}{}
	p.knownStructs[name.text] = struct{}{}
	if _, err := p.expect(tokenLBrace); err != nil {
		return nil, err
	}
	fields := make([]structField, 0, 4)
	seen := make(map[string]struct{})
	for p.peek().kind != tokenRBrace {
		fieldName, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[fieldName.text]; exists {
			return nil, p.errorf(fieldName, "struct %q declares field %q more than once", name.text, fieldName.text)
		}
		seen[fieldName.text] = struct{}{}
		fieldType, fieldTypeSpan, err := p.parseTypeName()
		if err != nil {
			return nil, err
		}
		field := structField{Name: fieldName.text, Type: fieldType, TypeSpan: fieldTypeSpan, NameSpan: parserSpanFromToken(fieldName)}
		if p.peek().kind == tokenTag {
			tagTok := p.next()
			values, tagErr := parseStructTag(tagTok.text)
			if tagErr != nil {
				return nil, p.errorf(tagTok, "%v", tagErr)
			}
			field.Tag, field.TagValues = tagTok.text, values
			field.Span = spanCover(field.NameSpan, parserSpanFromToken(tagTok))
		} else {
			field.Span = field.NameSpan
		}
		fields = append(fields, field)
		if !p.match(tokenComma) && !p.match(tokenSemicolon) && p.peek().kind != tokenRBrace && p.peek().kind != tokenIdent {
			return nil, p.errorf(p.peek(), "expected , ; or } after struct field")
		}
	}
	end, err := p.expect(tokenRBrace)
	if err != nil {
		return nil, err
	}
	if len(fields) == 0 {
		return nil, p.errorf(name, "struct %q must declare at least one field", name.text)
	}
	return structStmt{Name: name.text, Fields: fields, NameSpan: parserSpanFromToken(name), Span: parserSpanFromTokens(start, end)}, nil
}

// parseExpr operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseExpr(minPrec int) (Expr, error) {
	left, err := p.parsePrefix()
	if err != nil {
		return nil, err
	}
	left, err = p.parsePostfix(left)
	if err != nil {
		return nil, err
	}

	for {
		tok := p.peek()
		prec, ok := precedence(tok.kind)
		if !ok || prec < minPrec {
			break
		}
		p.next()
		right, err := p.parseExpr(prec + 1)
		if err != nil {
			return nil, err
		}
		left = &binaryExpr{op: tok.kind, left: left, right: right, Span: spanCover(exprSpan(left), exprSpan(right))}
	}

	return left, nil
}

// parsePostfix operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parsePostfix(left Expr) (Expr, error) {
	for {
		switch p.peek().kind {
		case tokenLess:
			// `<` is also the ordinary relational operator. Speculatively parse
			// type arguments and restore the lexer/parser state unless the full
			// `name<types>(...)` shape is present.
			lexerState := *p.lexer
			cur, peeked, lexErr := p.cur, p.peeked, p.lexErr
			span := exprSpan(left)
			typeArgs, _, err := p.parseTypeArgs()
			if err == nil && p.peek().kind == tokenLParen {
				args, callEnd, callErr := p.parseCallArgs()
				if callErr != nil {
					return nil, callErr
				}
				left = &callExpr{callee: left, args: args, typeArgs: typeArgs, Span: spanCover(span, parserSpanFromToken(callEnd))}
				continue
			}
			*p.lexer = lexerState
			p.cur, p.peeked, p.lexErr = cur, peeked, lexErr
			return left, nil
		case tokenLParen:
			span := exprSpan(left)
			args, end, err := p.parseCallArgs()
			if err != nil {
				return nil, err
			}
			left = &callExpr{callee: left, args: args, Span: spanCover(span, parserSpanFromToken(end))}
		case tokenDot:
			p.next()
			nameTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			left = &memberExpr{object: left, name: nameTok.text, Span: spanCover(exprSpan(left), parserSpanFromToken(nameTok)), NameSpan: parserSpanFromToken(nameTok)}
		case tokenLBracket:
			p.next()
			index, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			end, err := p.expect(tokenRBracket)
			if err != nil {
				return nil, err
			}
			left = &indexExpr{Object: left, Index: index, Span: spanCover(exprSpan(left), parserSpanFromToken(end))}
		case tokenWith:
			p.next()
			fields, end, err := p.parseStructFieldExprs()
			if err != nil {
				return nil, err
			}
			left = &structUpdateExpr{Base: left, Fields: fields, Span: spanCover(exprSpan(left), parserSpanFromToken(end))}
		case tokenQuestion:
			tok := p.next()
			left = &propagateExpr{expr: left, Span: spanCover(exprSpan(left), parserSpanFromToken(tok))}
		default:
			return left, nil
		}
	}
}

// parsePrefix operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parsePrefix() (Expr, error) {
	tok := p.peek()
	switch tok.kind {
	case tokenMinus, tokenBang:
		p.next()
		expr, err := p.parseExpr(7)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: tok.kind, expr: expr, Span: spanCover(parserSpanFromToken(tok), exprSpan(expr))}, nil
	case tokenTilde:
		p.next()
		expr, err := p.parseExpr(7)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: tok.kind, expr: expr, Span: spanCover(parserSpanFromToken(tok), exprSpan(expr))}, nil
	case tokenNumber:
		p.next()
		v, err := parseNumericLiteral(tok.text)
		if err != nil {
			return nil, p.errorf(tok, "%v", err)
		}
		return &literalExpr{value: v, defaultInteger: isDefaultIntegerLiteral(tok.text), Span: parserSpanFromToken(tok)}, nil
	case tokenString:
		p.next()
		return &literalExpr{value: stringValue(tok.text), Span: parserSpanFromToken(tok)}, nil
	case tokenTrue:
		p.next()
		return &literalExpr{value: boolValue(true), Span: parserSpanFromToken(tok)}, nil
	case tokenFalse:
		p.next()
		return &literalExpr{value: boolValue(false), Span: parserSpanFromToken(tok)}, nil
	case tokenNull:
		p.next()
		return &literalExpr{value: nullValue(), Span: parserSpanFromToken(tok)}, nil
	case tokenIdent:
		p.next()
		if p.peek().kind == tokenLBrace {
			if _, known := p.knownStructs[tok.text]; !known {
				return &identExpr{name: tok.text, Span: parserSpanFromToken(tok)}, nil
			}
			fields, end, err := p.parseStructFieldExprs()
			if err != nil {
				return nil, err
			}
			return &structExpr{TypeName: tok.text, Fields: fields, Span: spanCover(parserSpanFromToken(tok), parserSpanFromToken(end))}, nil
		}
		return &identExpr{name: tok.text, Span: parserSpanFromToken(tok)}, nil
	case tokenLParen:
		p.next()
		if p.peek().kind == tokenRParen {
			end := p.next()
			return &tupleExpr{Span: parserSpanFromTokens(tok, end)}, nil
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.match(tokenComma) {
			items := []Expr{expr}
			for {
				if p.peek().kind == tokenRParen {
					end := p.next()
					return &tupleExpr{items: items, Span: parserSpanFromTokens(tok, end)}, nil
				}
				item, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
				if !p.match(tokenComma) {
					end, err := p.expect(tokenRParen)
					if err != nil {
						return nil, err
					}
					return &tupleExpr{items: items, Span: parserSpanFromTokens(tok, end)}, nil
				}
			}
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return expr, nil
	case tokenLBracket:
		return p.parseList()
	case tokenLBrace:
		return p.parseBlockExpr()
	case tokenLazy:
		start := p.next()
		if p.peek().kind != tokenLBrace {
			return nil, p.errorf(p.peek(), "lazy requires a braced expression body")
		}
		body, err := p.parseBlockExpr()
		if err != nil {
			return nil, err
		}
		return &lazyExpr{Body: body, Span: spanCover(parserSpanFromToken(start), exprSpan(body))}, nil
	case tokenTry:
		return p.parseTryExpr()
	case tokenMatch, tokenSwitch:
		return p.parseMatchExpr(tok.kind)
	default:
		return nil, p.errorf(tok, "expected expression")
	}
}

// parseTryExpr operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseTryExpr() (Expr, error) {
	start := p.next()
	body, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenCatch); err != nil {
		return nil, err
	}
	nameTok, err := p.expect(tokenIdent)
	if err != nil {
		return nil, err
	}
	handler, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	return &tryExpr{body: body, catchName: nameTok.text, handler: handler, Span: spanCover(parserSpanFromToken(start), exprSpan(handler))}, nil
}

// parseCallArgs operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []Expr, the closing delimiter, and an error. Callers should only
// trust the values when the error is nil.
func (p *parser) parseCallArgs() ([]Expr, token, error) {
	if _, err := p.expect(tokenLParen); err != nil {
		return nil, token{}, err
	}
	if p.match(tokenRParen) {
		return nil, p.cur, nil
	}
	var args []Expr
	for {
		arg, err := p.parseExpr(0)
		if err != nil {
			return nil, token{}, err
		}
		args = append(args, arg)
		if p.match(tokenComma) {
			continue
		}
		end, err := p.expect(tokenRParen)
		if err != nil {
			return nil, token{}, err
		}
		return args, end, nil
	}
}

func (p *parser) parseStructFieldExprs() ([]structFieldExpr, token, error) {
	_, err := p.expect(tokenLBrace)
	if err != nil {
		return nil, token{}, err
	}
	fields := make([]structFieldExpr, 0, 4)
	seen := make(map[string]struct{})
	for p.peek().kind != tokenRBrace {
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, token{}, err
		}
		if _, exists := seen[name.text]; exists {
			return nil, token{}, p.errorf(name, "struct initializer repeats field %q", name.text)
		}
		seen[name.text] = struct{}{}
		if _, err := p.expect(tokenColon); err != nil {
			return nil, token{}, err
		}
		value, err := p.parseExpr(0)
		if err != nil {
			return nil, token{}, err
		}
		fields = append(fields, structFieldExpr{Name: name.text, Expr: value, Span: spanCover(parserSpanFromToken(name), exprSpan(value))})
		if !p.match(tokenComma) && !p.match(tokenSemicolon) && p.peek().kind != tokenRBrace {
			return nil, token{}, p.errorf(p.peek(), "expected , ; or } after struct initializer")
		}
	}
	end, err := p.expect(tokenRBrace)
	if err != nil {
		return nil, token{}, err
	}
	return fields, end, nil
}

// parseNameList operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []string and an error. Callers should only trust the value when
// the error is nil.
func (p *parser) parseNameList() ([]string, error) {
	first, err := p.expect(tokenIdent)
	if err != nil {
		return nil, err
	}
	names := []string{first.text}
	for p.match(tokenComma) {
		tok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		names = append(names, tok.text)
	}
	return names, nil
}

// parseNameListAllowEmpty operates on parser values and keeps the
// parser-specific behavior attached to the receiver so callers can use the
// method as the natural entry point for that data structure.
//
// It returns []string and an error. Callers should only trust the value when
// the error is nil.
func (p *parser) parseNameListAllowEmpty() ([]string, error) {
	if p.match(tokenRParen) {
		return nil, nil
	}
	first, err := p.expect(tokenIdent)
	if err != nil {
		return nil, err
	}
	names := []string{first.text}
	for p.match(tokenComma) {
		tok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		names = append(names, tok.text)
	}
	if _, err := p.expect(tokenRParen); err != nil {
		return nil, err
	}
	return names, nil
}

// parseFunctionParams parses the Go-style function parameter surface. A
// parameter type is optional for compatibility with existing dynamic
// functions; when present it follows the name without a colon.
func (p *parser) parseFunctionParams() ([]string, []string, []sourceSpan, error) {
	if p.match(tokenRParen) {
		return nil, nil, nil, nil
	}
	var names []string
	var types []string
	var spans []sourceSpan
	for {
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, nil, nil, err
		}
		typeName := ""
		typeSpan := sourceSpan{}
		if isTypeStart(p.peek().kind) {
			typeName, typeSpan, err = p.parseTypeName()
			if err != nil {
				return nil, nil, nil, err
			}
		}
		names = append(names, name.text)
		types = append(types, typeName)
		spans = append(spans, spanCover(parserSpanFromToken(name), typeSpan))
		if p.match(tokenComma) {
			continue
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, nil, nil, err
		}
		return names, types, spans, nil
	}
}

func (p *parser) parseTypeParams() ([]typeParam, error) {
	if !p.match(tokenLess) {
		return nil, nil
	}
	var params []typeParam
	seen := make(map[string]struct{})
	for {
		name, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[name.text]; exists {
			return nil, p.errorf(name, "duplicate type parameter %q", name.text)
		}
		seen[name.text] = struct{}{}
		bound := ""
		endSpan := parserSpanFromToken(name)
		p.knownTypes[name.text] = struct{}{}
		if p.match(tokenColon) {
			var boundSpan sourceSpan
			bound, boundSpan, err = p.parseTypeName()
			if err != nil {
				return nil, err
			}
			endSpan = boundSpan
		}
		params = append(params, typeParam{Name: name.text, Bound: bound, Span: spanCover(parserSpanFromToken(name), endSpan)})
		if p.match(tokenComma) {
			continue
		}
		if _, err := p.expectTypeCloser(); err != nil {
			return nil, err
		}
		return params, nil
	}
}

func (p *parser) parseTypeArgs() ([]string, token, error) {
	if _, err := p.expect(tokenLess); err != nil {
		return nil, token{}, err
	}
	p.typeArgDepth++
	defer func() { p.typeArgDepth-- }()
	var args []string
	for {
		arg, _, err := p.parseTypeName()
		if err != nil {
			return nil, token{}, err
		}
		args = append(args, arg)
		if p.match(tokenComma) {
			continue
		}
		end, err := p.expectTypeCloser()
		if err != nil {
			return nil, token{}, err
		}
		return args, end, nil
	}
}

// expectTypeCloser consumes one generic closing delimiter. The lexer emits
// adjacent greater-than characters as tokenShiftRight for ordinary expressions
// such as a >> b; nested generic types need the same source spelling to mean
// two independent closing delimiters. Preserve the second delimiter as parser
// lookahead so nested type parsing can consume it on the next call.
func (p *parser) expectTypeCloser() (token, error) {
	tok := p.peek()
	if tok.kind == tokenGreater {
		p.next()
		return tok, nil
	}
	if tok.kind != tokenShiftRight {
		return token{}, p.errorf(tok, "expected >, found %s", tok.kind)
	}
	p.next()
	first := tok
	first.kind = tokenGreater
	first.text = ">"
	first.raw = ">"
	second := first
	second.col++
	p.cur = second
	p.peeked = true
	return first, nil
}

// parseTypeName parses canonical scalar/runtime type names, parameterized
// containers, and Go-style function types such as func(u32, string) bool.
func (p *parser) parseTypeName() (string, sourceSpan, error) {
	start := p.peek()
	if start.kind == tokenNumber && p.typeArgDepth > 0 {
		tok := p.next()
		return tok.text, parserSpanFromToken(tok), nil
	}
	if start.kind == tokenIdent || start.kind == tokenNull || start.kind == tokenEnum {
		tok := p.next()
		if _, known := p.knownTypes[tok.text]; !known {
			return "", sourceSpan{}, p.errorf(tok, "unknown type %q", tok.text)
		}
		name := tok.text
		endSpan := parserSpanFromToken(tok)
		if p.match(tokenLess) {
			p.typeArgDepth++
			defer func() { p.typeArgDepth-- }()
			var args []string
			for {
				arg, argSpan, err := p.parseTypeName()
				if err != nil {
					return "", sourceSpan{}, err
				}
				args = append(args, arg)
				endSpan = argSpan
				if p.match(tokenComma) {
					continue
				}
				end, err := p.expectTypeCloser()
				if err != nil {
					return "", sourceSpan{}, err
				}
				endSpan = parserSpanFromToken(end)
				break
			}
			name += "<" + strings.Join(args, ", ") + ">"
		}
		return name, spanCover(parserSpanFromToken(tok), endSpan), nil
	}
	if start.kind != tokenFunc {
		return "", sourceSpan{}, p.errorf(start, "expected type name")
	}
	p.next()
	if _, err := p.expect(tokenLParen); err != nil {
		return "", sourceSpan{}, err
	}
	var params []string
	if !p.match(tokenRParen) {
		for {
			param, _, err := p.parseTypeName()
			if err != nil {
				return "", sourceSpan{}, err
			}
			params = append(params, param)
			if p.match(tokenComma) {
				continue
			}
			if _, err := p.expect(tokenRParen); err != nil {
				return "", sourceSpan{}, err
			}
			break
		}
	}
	if !isTypeStart(p.peek().kind) {
		return "", sourceSpan{}, p.errorf(p.peek(), "function type requires a return type")
	}
	result, resultSpan, err := p.parseTypeName()
	if err != nil {
		return "", sourceSpan{}, err
	}
	return "func(" + strings.Join(params, ", ") + ") " + result, spanCover(parserSpanFromToken(start), resultSpan), nil
}

func builtinTypeSet() map[string]struct{} {
	set := make(map[string]struct{})
	for _, name := range []string{
		"number", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64",
		"string", "bool", "null", "list", "tuple", "tensor", "enum", "function", "error", "result", "option",
		"stream", "task", "map", "set", "module", "decimal", "fixed", "interval", "modint", "complex", "bigint",
		"bigfloat", "quantity", "dual", "duration", "datetime", "location", "Lazy",
		"poly", "quat", "simd", "sparse_matrix", "sparse_tensor",
	} {
		set[name] = struct{}{}
	}
	return set
}

// parseStructTag validates the namespace:"value" pairs accepted by Go-style
// field tags. The raw spelling is retained separately so unknown namespaces
// can survive a parse/format round trip.
func parseStructTag(raw string) (map[string]string, error) {
	values := make(map[string]string)
	for i := 0; i < len(raw); {
		for i < len(raw) && (raw[i] == ' ' || raw[i] == '\t' || raw[i] == '\n' || raw[i] == '\r') {
			i++
		}
		if i == len(raw) {
			break
		}
		start := i
		for i < len(raw) && raw[i] > ' ' && raw[i] != ':' && raw[i] != '"' && raw[i] != '\'' && raw[i] != 0x7f {
			i++
		}
		if start == i || i >= len(raw) || raw[i] != ':' {
			return nil, fmt.Errorf("malformed struct tag near %q", raw[start:])
		}
		key := raw[start:i]
		if _, exists := values[key]; exists {
			return nil, fmt.Errorf("duplicate struct tag namespace %q", key)
		}
		i++
		if i >= len(raw) || raw[i] != '"' {
			return nil, fmt.Errorf("struct tag namespace %q requires a quoted value", key)
		}
		valueStart := i
		i++
		escaped := false
		for i < len(raw) {
			if raw[i] == '"' && !escaped {
				i++
				break
			}
			if raw[i] == '\\' && !escaped {
				escaped = true
			} else {
				escaped = false
			}
			i++
		}
		if i > len(raw) || raw[i-1] != '"' {
			return nil, fmt.Errorf("unterminated struct tag value for namespace %q", key)
		}
		quoted := raw[valueStart:i]
		value, err := strconv.Unquote(quoted)
		if err != nil {
			return nil, fmt.Errorf("invalid struct tag value for namespace %q: %v", key, err)
		}
		values[key] = value
	}
	return values, nil
}

func isTypeStart(kind tokenKind) bool {
	return kind == tokenIdent || kind == tokenFunc || kind == tokenNull || kind == tokenEnum
}

// parseList operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseList() (Expr, error) {
	start, err := p.expect(tokenLBracket)
	if err != nil {
		return nil, err
	}
	if p.match(tokenRBracket) {
		return &listExpr{Span: parserSpanFromTokens(start, p.cur)}, nil
	}
	var items []Expr
	for {
		item, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
		if p.match(tokenComma) {
			continue
		}
		end, err := p.expect(tokenRBracket)
		if err != nil {
			return nil, err
		}
		return &listExpr{items: items, Span: parserSpanFromTokens(start, end)}, nil
	}
}

// parseBlockExpr operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseBlockExpr() (Expr, error) {
	start, err := p.expect(tokenLBrace)
	if err != nil {
		return nil, err
	}
	var stmts []Stmt
	var tail Expr
	for {
		if p.match(tokenRBrace) {
			return &blockExpr{stmts: stmts, tail: tail, Span: parserSpanFromTokens(start, p.cur)}, nil
		}
		if tail != nil {
			for p.match(tokenSemicolon) {
			}
			end, err := p.expect(tokenRBrace)
			if err != nil {
				return nil, err
			}
			return &blockExpr{stmts: stmts, tail: tail, Span: parserSpanFromTokens(start, end)}, nil
		}
		switch p.peek().kind {
		case tokenLet, tokenVar, tokenConst, tokenImport, tokenFFI, tokenMacro, tokenFunc, tokenReturn, tokenWhile, tokenFor, tokenBreak, tokenContinue, tokenEmit, tokenDefer:
			stmt, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, stmt)
			for p.match(tokenSemicolon) {
			}
		case tokenIdent:
			if p.peek().text == "var" && p.peekSecond().kind != tokenEqual {
				stmt, err := p.parseStmt()
				if err != nil {
					return nil, err
				}
				stmts = append(stmts, stmt)
				for p.match(tokenSemicolon) {
				}
				continue
			}
			if p.peekSecond().kind == tokenEqual {
				stmt, err := p.parseStmt()
				if err != nil {
					return nil, err
				}
				stmts = append(stmts, stmt)
				for p.match(tokenSemicolon) {
				}
				continue
			}
			fallthrough
		default:
			expr, err := p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			tail = expr
			continue
		}
	}
}

// parseMatchExpr operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseMatchExpr(keyword tokenKind) (Expr, error) {
	start, err := p.expect(keyword)
	if err != nil {
		return nil, err
	}
	subject, err := p.parseExpr(0)
	if err != nil {
		return nil, err
	}
	if _, err := p.expect(tokenLBrace); err != nil {
		return nil, err
	}
	var arms []matchArm
	var end token
	seenDefault := false
	for {
		if p.peek().kind == tokenRBrace {
			end = p.next()
			break
		}
		var (
			pat       Pattern
			isDefault bool
		)
		if p.peek().kind == tokenIdent && p.peek().text == "default" {
			defaultTok := p.next()
			pat = wildcardPattern{Span: parserSpanFromToken(defaultTok)}
			isDefault = true
		} else {
			var err error
			pat, err = p.parsePattern()
			if err != nil {
				return nil, err
			}
		}
		var guard Expr
		if p.peek().kind == tokenIdent && p.peek().text == "when" {
			p.next()
			guard, err = p.parseExpr(0)
			if err != nil {
				return nil, err
			}
			if isDefault {
				return nil, p.errorf(p.peek(), "default match arm cannot have a guard")
			}
		}
		if _, err := p.expect(tokenArrow); err != nil {
			return nil, err
		}
		body, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		arms = append(arms, matchArm{pattern: pat, guard: guard, body: body, isDefault: isDefault})
		if seenDefault {
			return nil, p.errorf(p.peek(), "default match arm must be last")
		}
		seenDefault = isDefault
		if p.match(tokenComma) || p.match(tokenSemicolon) {
			continue
		}
		if p.peek().kind == tokenRBrace {
			end = p.next()
			break
		}
		return nil, p.errorf(p.peek(), "expected , ; or } after match arm")
	}
	if len(arms) == 0 {
		return nil, p.errorf(p.peek(), "match requires at least one arm")
	}
	return &matchExpr{subject: subject, arms: arms, keyword: keyword, Span: parserSpanFromTokens(start, end)}, nil
}

// exprSpan handles the expr span step in the surrounding package and returns
// sourceSpan.
//
// It returns sourceSpan so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func exprSpan(expr Expr) sourceSpan {
	switch n := expr.(type) {
	case *literalExpr:
		return n.Span
	case *identExpr:
		return n.Span
	case *unaryExpr:
		return n.Span
	case *binaryExpr:
		return n.Span
	case *callExpr:
		return n.Span
	case *blockExpr:
		return n.Span
	case *lazyExpr:
		return n.Span
	case *memberExpr:
		return n.Span
	case *listExpr:
		return n.Span
	case *tupleExpr:
		return n.Span
	case *structExpr:
		return n.Span
	case *structUpdateExpr:
		return n.Span
	case *indexExpr:
		return n.Span
	case *matchExpr:
		return n.Span
	case *tryExpr:
		return n.Span
	case *propagateExpr:
		return n.Span
	default:
		return sourceSpan{}
	}
}

// parsePattern operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Pattern and an error. Callers should only trust the value when
// the error is nil.
func (p *parser) parsePattern() (Pattern, error) {
	first, err := p.parsePatternConjunction()
	if err != nil {
		return nil, err
	}
	alternatives := []Pattern{first}
	for p.match(tokenPipe) {
		next, err := p.parsePatternConjunction()
		if err != nil {
			return nil, err
		}
		alternatives = append(alternatives, next)
	}
	if len(alternatives) == 1 {
		return first, nil
	}
	return orPattern{alternatives: alternatives, Span: spanCover(patternSpan(first), patternSpan(alternatives[len(alternatives)-1]))}, nil
}

func (p *parser) parsePatternConjunction() (Pattern, error) {
	left, err := p.parsePatternAtom()
	if err != nil {
		return nil, err
	}
	for p.peek().kind == tokenIdent && p.peek().text == "and" {
		p.next()
		right, err := p.parsePatternAtom()
		if err != nil {
			return nil, err
		}
		left = andPattern{left: left, right: right, Span: spanCover(patternSpan(left), patternSpan(right))}
	}
	return left, nil
}

func (p *parser) parsePatternAtom() (Pattern, error) {
	tok := p.peek()
	switch tok.kind {
	case tokenIdent:
		p.next()
		if tok.text == "not" {
			inner, err := p.parsePatternAtom()
			if err != nil {
				return nil, err
			}
			return notPattern{inner: inner, Span: spanCover(parserSpanFromToken(tok), patternSpan(inner))}, nil
		}
		if tok.text == "_" {
			return wildcardPattern{Span: parserSpanFromToken(tok)}, nil
		}
		if p.peek().kind == tokenLBrace {
			if _, known := p.knownStructs[tok.text]; known {
				return p.parseStructPattern(tok)
			}
		}
		if p.match(tokenDot) {
			caseTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			item := enumPattern{typeName: tok.text, caseName: caseTok.text, Span: parserSpanFromTokens(tok, caseTok)}
			if p.match(tokenLParen) {
				if p.peek().kind == tokenRParen {
					item.Span = parserSpanFromTokens(tok, p.next())
					return item, nil
				}
				for {
					child, err := p.parsePattern()
					if err != nil {
						return nil, err
					}
					item.items = append(item.items, child)
					if p.match(tokenComma) {
						continue
					}
					end, err := p.expect(tokenRParen)
					if err != nil {
						return nil, err
					}
					item.Span = parserSpanFromTokens(tok, end)
					break
				}
			}
			return item, nil
		}
		if p.peek().kind == tokenIdent && p.peek().text != "when" && p.peek().text != "and" {
			binding := p.next()
			return typePattern{typeName: tok.text, binding: binding.text, Span: parserSpanFromTokens(tok, binding)}, nil
		}
		return bindPattern{name: tok.text, Span: parserSpanFromToken(tok)}, nil
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		op := p.next()
		literal, err := p.parsePatternAtom()
		if err != nil {
			return nil, err
		}
		value, ok := literal.(literalPattern)
		if !ok {
			return nil, p.errorf(op, "relational patterns require a literal value")
		}
		return relationalPattern{op: op.kind, value: value.value, Span: spanCover(parserSpanFromToken(op), patternSpan(literal))}, nil
	case tokenNumber:
		p.next()
		v, err := parseNumericLiteral(tok.text)
		if err != nil {
			return nil, p.errorf(tok, "%v", err)
		}
		return literalPattern{value: v, Span: parserSpanFromToken(tok)}, nil
	case tokenString:
		p.next()
		return literalPattern{value: stringValue(tok.text), Span: parserSpanFromToken(tok)}, nil
	case tokenTrue:
		p.next()
		return literalPattern{value: boolValue(true), Span: parserSpanFromToken(tok)}, nil
	case tokenFalse:
		p.next()
		return literalPattern{value: boolValue(false), Span: parserSpanFromToken(tok)}, nil
	case tokenNull:
		p.next()
		return literalPattern{value: nullValue(), Span: parserSpanFromToken(tok)}, nil
	case tokenLBracket:
		p.next()
		if p.match(tokenRBracket) {
			return listPattern{Span: parserSpanFromTokens(tok, p.cur)}, nil
		}
		var items []Pattern
		for {
			item, err := p.parsePattern()
			if err != nil {
				return nil, err
			}
			items = append(items, item)
			if p.match(tokenComma) {
				continue
			}
			end, err := p.expect(tokenRBracket)
			if err != nil {
				return nil, err
			}
			return listPattern{items: items, Span: parserSpanFromTokens(tok, end)}, nil
		}
	case tokenLParen:
		p.next()
		pat, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return pat, nil
	default:
		return nil, p.errorf(tok, "expected pattern")
	}
}

func (p *parser) parseStructPattern(typeTok token) (Pattern, error) {
	if _, err := p.expect(tokenLBrace); err != nil {
		return nil, err
	}
	fields := make([]structPatternField, 0, 4)
	seen := make(map[string]struct{})
	for p.peek().kind != tokenRBrace {
		fieldTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		if _, exists := seen[fieldTok.text]; exists {
			return nil, p.errorf(fieldTok, "struct pattern repeats field %q", fieldTok.text)
		}
		seen[fieldTok.text] = struct{}{}
		if _, err := p.expect(tokenColon); err != nil {
			return nil, err
		}
		child, err := p.parsePattern()
		if err != nil {
			return nil, err
		}
		fields = append(fields, structPatternField{
			name:    fieldTok.text,
			pattern: child,
			Span:    spanCover(parserSpanFromToken(fieldTok), patternSpan(child)),
		})
		if !p.match(tokenComma) && !p.match(tokenSemicolon) && p.peek().kind != tokenRBrace {
			return nil, p.errorf(p.peek(), "expected , ; or } after struct pattern field")
		}
	}
	end, err := p.expect(tokenRBrace)
	if err != nil {
		return nil, err
	}
	return structPattern{typeName: typeTok.text, fields: fields, Span: parserSpanFromTokens(typeTok, end)}, nil
}

func patternSpan(pattern Pattern) sourceSpan {
	switch p := pattern.(type) {
	case bindPattern:
		return p.Span
	case enumPattern:
		return p.Span
	case literalPattern:
		return p.Span
	case listPattern:
		return p.Span
	case structPattern:
		return p.Span
	case orPattern:
		return p.Span
	case andPattern:
		return p.Span
	case notPattern:
		return p.Span
	case typePattern:
		return p.Span
	case relationalPattern:
		return p.Span
	case wildcardPattern:
		return p.Span
	default:
		return sourceSpan{}
	}
}

// precedence handles the precedence step in the surrounding package and
// returns int, bool.
//
// It returns int, bool so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func precedence(kind tokenKind) (int, bool) {
	switch kind {
	case tokenOrOr:
		return 1, true
	case tokenAndAnd:
		return 2, true
	case tokenPipe:
		return 3, true
	case tokenCaret:
		return 4, true
	case tokenAmp:
		return 5, true
	case tokenShiftLeft, tokenShiftRight:
		return 6, true
	case tokenEqualEqual, tokenBangEqual:
		return 7, true
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		return 8, true
	case tokenPlus, tokenMinus:
		return 9, true
	case tokenStar, tokenSlash, tokenPercent:
		return 10, true
	default:
		return 0, false
	}
}

// peek operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (p *parser) peek() token {
	if p.peeked {
		return p.cur
	}
	tok, err := p.lexer.nextToken()
	if err != nil {
		p.lexErr = err
		p.cur = token{kind: tokenEOF, text: err.Error(), line: p.lexer.line, col: p.lexer.col}
		if scanErr, ok := err.(*lexerError); ok {
			p.cur.line, p.cur.col = scanErr.line, scanErr.col
		}
		p.peeked = true
		return p.cur
	}
	p.cur = tok
	p.peeked = true
	return tok
}

func (p *parser) peekSecond() token {
	if !p.peeked {
		p.peek()
	}
	if p.hasLookahead {
		return p.lookahead
	}
	tok, err := p.lexer.nextToken()
	if err != nil {
		p.lexErr = err
		tok = token{kind: tokenEOF, text: err.Error()}
	}
	p.lookahead = tok
	p.hasLookahead = true
	return tok
}

// next operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (p *parser) next() token {
	tok := p.peek()
	if p.hasLookahead {
		p.cur = p.lookahead
		p.hasLookahead = false
		p.peeked = true
	} else {
		p.peeked = false
	}
	return tok
}

// match operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (p *parser) match(kind tokenKind) bool {
	if p.peek().kind != kind {
		return false
	}
	p.next()
	return true
}

// expect operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) expect(kind tokenKind) (token, error) {
	tok := p.peek()
	if tok.kind != kind {
		return token{}, p.errorf(tok, "expected %s, found %s", kind, tok.kind)
	}
	p.next()
	return tok, nil
}

// errorf operates on parser values and keeps the parser-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns an error when the operation cannot complete cleanly.
func (p *parser) errorf(tok token, format string, args ...any) error {
	if p.lexErr != nil {
		if scanErr, ok := p.lexErr.(*lexerError); ok {
			return fmt.Errorf("%s:%d:%d: %s", p.sourceName, scanErr.line, scanErr.col, scanErr.message)
		}
		return p.lexErr
	}
	base := fmt.Sprintf(format, args...)
	if tok.kind == tokenEOF {
		return fmt.Errorf("%s:%d:%d: %s", p.sourceName, tok.line, tok.col, base)
	}
	return fmt.Errorf("%s:%d:%d: %s", p.sourceName, tok.line, tok.col, base)
}
