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

type wildcardPattern struct{}

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

// Parse parses the supplied work and returns the structured form that the
// evaluator, optimizer, or analyzer expects to work on.
//
// It returns *Program and an error. Callers should only trust the value when
// the error is nil.
func Parse(sourceName, src string) (*Program, error) {
	p := &parser{sourceName: sourceName, lexer: newLexer(src), knownTypes: builtinTypeSet(), knownStructs: make(map[string]struct{})}
	prog, err := p.parse()
	if err != nil {
		return nil, normalizeError(err, sourceName)
	}
	return prog, nil
}

type parser struct {
	sourceName   string
	lexer        *lexer
	knownTypes   map[string]struct{}
	knownStructs map[string]struct{}
	cur          token
	peeked       bool
	lexErr       error
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
	switch p.peek().kind {
	case tokenLet:
		start := p.next()
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
			return letStmt{Names: names, Expr: expr, Unwrap: unwrap, Span: spanFromToken(start)}, nil
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
		return letStmt{Name: nameTok.text, Type: typeName, Expr: expr, Unwrap: unwrap, Span: spanFromTokens(start, nameTok), NameSpan: spanFromToken(nameTok)}, nil
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
			return emitStmt{Expr: expr, Span: spanFromToken(start)}, nil
		}
		args := make([]Expr, 0, len(exprs)*2-1)
		for i, item := range exprs {
			if i > 0 {
				args = append(args, &literalExpr{value: stringValue(", "), Span: spanFromToken(start)})
			}
			args = append(args, item)
		}
		return emitStmt{
			Expr: &callExpr{
				callee: &identExpr{name: "cat", Span: spanFromToken(start)},
				args:   args,
				Span:   spanFromToken(start),
			},
			Span: spanFromToken(start),
		}, nil
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
		return macroStmt{Name: nameTok.text, Params: params, Body: body, Span: spanFromTokens(start, nameTok), NameSpan: spanFromToken(nameTok)}, nil
	case tokenFunc:
		start := p.next()
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		typeParams, err := p.parseTypeParams()
		if err != nil {
			return nil, err
		}
		if _, err := p.expect(tokenLParen); err != nil {
			return nil, err
		}
		params, paramTypes, paramSpans, err := p.parseFunctionParams()
		if err != nil {
			return nil, err
		}
		returnType := ""
		var returnTypeSpan sourceSpan
		if isTypeStart(p.peek().kind) {
			returnType, returnTypeSpan, err = p.parseTypeName()
			if err != nil {
				return nil, err
			}
		}
		var body Expr
		if p.match(tokenArrow) {
			body, err = p.parseExpr(0)
			if err != nil {
				return nil, err
			}
		} else {
			body, err = p.parseBlockExpr()
			if err != nil {
				return nil, err
			}
		}
		return funcStmt{
			Name:           nameTok.text,
			TypeParams:     typeParams,
			Params:         params,
			ParamTypes:     paramTypes,
			ParamSpans:     paramSpans,
			ReturnType:     returnType,
			ReturnTypeSpan: returnTypeSpan,
			Body:           body,
			Span:           spanCover(spanFromTokens(start, nameTok), exprSpan(body)),
			NameSpan:       spanFromToken(nameTok),
		}, nil
	case tokenReturn:
		start := p.next()
		if p.peek().kind == tokenSemicolon || p.peek().kind == tokenRBrace || p.peek().kind == tokenEOF {
			return returnStmt{Span: spanFromToken(start)}, nil
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		return returnStmt{Expr: expr, Span: spanFromToken(start)}, nil
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
		return whileStmt{Cond: cond, Body: body, Span: spanCover(spanFromToken(start), exprSpan(body))}, nil
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
		return forStmt{Name: nameTok.text, Iter: iter, Body: body, Span: spanCover(spanFromToken(start), exprSpan(body)), NameSpan: spanFromToken(nameTok)}, nil
	case tokenBreak:
		start := p.next()
		return breakStmt{Span: spanFromToken(start)}, nil
	case tokenContinue:
		start := p.next()
		return continueStmt{Span: spanFromToken(start)}, nil
	case tokenPackage:
		start := p.next()
		nameTok, err := p.expect(tokenIdent)
		if err != nil {
			return nil, err
		}
		stmt := packageStmt{Name: nameTok.text, Span: spanFromTokens(start, nameTok), NameSpan: spanFromToken(nameTok)}
		if p.peek().kind == tokenNumber || p.peek().kind == tokenString {
			verTok := p.next()
			stmt.Version = verTok.text
			stmt.VersionSpan = spanFromToken(verTok)
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
		spans := []sourceSpan{spanFromToken(first)}
		for p.match(tokenComma) {
			tok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			names = append(names, tok.text)
			spans = append(spans, spanFromToken(tok))
		}
		return exportStmt{Names: names, Span: spanCover(spanFromToken(start), spans[len(spans)-1]), NameSpans: spans}, nil
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
			item := enumCase{Name: caseTok.text, Span: spanFromToken(caseTok)}
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
		return enumStmt{Name: name.text, Backing: backing, Cases: cases, NameSpan: spanFromToken(name), Span: spanFromTokens(start, end)}, nil
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
		return typeDeclStmt{Name: name.text, Target: target, Alias: true, NameSpan: spanFromToken(name), Span: spanCover(spanFromToken(start), targetSpan)}, nil
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
		return newtypeStmt{Name: name.text, Target: target, NameSpan: spanFromToken(name), Span: spanCover(spanFromToken(start), targetSpan)}, nil
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
		return importStmt{Path: pathTok.text, Alias: aliasTok.text, Span: spanFromTokens(start, aliasTok), PathSpan: spanFromToken(pathTok), AliasSpan: spanFromToken(aliasTok)}, nil
	default:
		return nil, p.errorf(p.peek(), "expected statement starting with let, emit, func, return, while, for, break, continue, package, export, enum, struct, type, newtype, or import")
	}
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
		fieldType, _, err := p.parseTypeName()
		if err != nil {
			return nil, err
		}
		field := structField{Name: fieldName.text, Type: fieldType, NameSpan: spanFromToken(fieldName)}
		if p.peek().kind == tokenTag {
			tagTok := p.next()
			values, tagErr := parseStructTag(tagTok.text)
			if tagErr != nil {
				return nil, p.errorf(tagTok, "%v", tagErr)
			}
			field.Tag, field.TagValues = tagTok.text, values
			field.Span = spanCover(field.NameSpan, spanFromToken(tagTok))
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
	return structStmt{Name: name.text, Fields: fields, NameSpan: spanFromToken(name), Span: spanFromTokens(start, end)}, nil
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
		left = &binaryExpr{op: tok.kind, left: left, right: right}
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
			typeArgs, end, err := p.parseTypeArgs()
			if err == nil && p.peek().kind == tokenLParen {
				args, callErr := p.parseCallArgs()
				if callErr != nil {
					return nil, callErr
				}
				left = &callExpr{callee: left, args: args, typeArgs: typeArgs, Span: spanCover(span, spanFromToken(end))}
				continue
			}
			*p.lexer = lexerState
			p.cur, p.peeked, p.lexErr = cur, peeked, lexErr
			return left, nil
		case tokenLParen:
			span := exprSpan(left)
			args, err := p.parseCallArgs()
			if err != nil {
				return nil, err
			}
			left = &callExpr{callee: left, args: args, Span: span}
		case tokenDot:
			p.next()
			nameTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			left = &memberExpr{object: left, name: nameTok.text, Span: spanCover(exprSpan(left), spanFromToken(nameTok)), NameSpan: spanFromToken(nameTok)}
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
			left = &indexExpr{Object: left, Index: index, Span: spanCover(exprSpan(left), spanFromToken(end))}
		case tokenWith:
			p.next()
			fields, end, err := p.parseStructFieldExprs()
			if err != nil {
				return nil, err
			}
			left = &structUpdateExpr{Base: left, Fields: fields, Span: spanCover(exprSpan(left), spanFromToken(end))}
		case tokenQuestion:
			tok := p.next()
			left = &propagateExpr{expr: left, Span: spanFromToken(tok)}
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
		return &unaryExpr{op: tok.kind, expr: expr}, nil
	case tokenTilde:
		p.next()
		expr, err := p.parseExpr(7)
		if err != nil {
			return nil, err
		}
		return &unaryExpr{op: tok.kind, expr: expr}, nil
	case tokenNumber:
		p.next()
		v, err := parseNumericLiteral(tok.text)
		if err != nil {
			return nil, p.errorf(tok, "%v", err)
		}
		return &literalExpr{value: v, defaultInteger: isDefaultIntegerLiteral(tok.text), Span: spanFromToken(tok)}, nil
	case tokenString:
		p.next()
		return &literalExpr{value: stringValue(tok.text), Span: spanFromToken(tok)}, nil
	case tokenTrue:
		p.next()
		return &literalExpr{value: boolValue(true), Span: spanFromToken(tok)}, nil
	case tokenFalse:
		p.next()
		return &literalExpr{value: boolValue(false), Span: spanFromToken(tok)}, nil
	case tokenNull:
		p.next()
		return &literalExpr{value: nullValue(), Span: spanFromToken(tok)}, nil
	case tokenIdent:
		p.next()
		if p.peek().kind == tokenLBrace {
			if _, known := p.knownStructs[tok.text]; !known {
				return &identExpr{name: tok.text, Span: spanFromToken(tok)}, nil
			}
			fields, end, err := p.parseStructFieldExprs()
			if err != nil {
				return nil, err
			}
			return &structExpr{TypeName: tok.text, Fields: fields, Span: spanCover(spanFromToken(tok), spanFromToken(end))}, nil
		}
		return &identExpr{name: tok.text, Span: spanFromToken(tok)}, nil
	case tokenLParen:
		p.next()
		if p.match(tokenRParen) {
			return &tupleExpr{Span: spanFromToken(tok)}, nil
		}
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		if p.match(tokenComma) {
			items := []Expr{expr}
			for {
				if p.match(tokenRParen) {
					return &tupleExpr{items: items, Span: spanFromToken(tok)}, nil
				}
				item, err := p.parseExpr(0)
				if err != nil {
					return nil, err
				}
				items = append(items, item)
				if !p.match(tokenComma) {
					if _, err := p.expect(tokenRParen); err != nil {
						return nil, err
					}
					return &tupleExpr{items: items, Span: spanFromToken(tok)}, nil
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
	return &tryExpr{body: body, catchName: nameTok.text, handler: handler, Span: spanCover(spanFromToken(start), exprSpan(handler))}, nil
}

// parseCallArgs operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns []Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseCallArgs() ([]Expr, error) {
	if _, err := p.expect(tokenLParen); err != nil {
		return nil, err
	}
	if p.match(tokenRParen) {
		return nil, nil
	}
	var args []Expr
	for {
		arg, err := p.parseExpr(0)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
		if p.match(tokenComma) {
			continue
		}
		if _, err := p.expect(tokenRParen); err != nil {
			return nil, err
		}
		return args, nil
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
		fields = append(fields, structFieldExpr{Name: name.text, Expr: value, Span: spanCover(spanFromToken(name), exprSpan(value))})
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
		spans = append(spans, spanCover(spanFromToken(name), typeSpan))
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
		endSpan := spanFromToken(name)
		p.knownTypes[name.text] = struct{}{}
		if p.match(tokenColon) {
			var boundSpan sourceSpan
			bound, boundSpan, err = p.parseTypeName()
			if err != nil {
				return nil, err
			}
			endSpan = boundSpan
		}
		params = append(params, typeParam{Name: name.text, Bound: bound, Span: spanCover(spanFromToken(name), endSpan)})
		if p.match(tokenComma) {
			continue
		}
		if _, err := p.expect(tokenGreater); err != nil {
			return nil, err
		}
		return params, nil
	}
}

func (p *parser) parseTypeArgs() ([]string, token, error) {
	if _, err := p.expect(tokenLess); err != nil {
		return nil, token{}, err
	}
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
		end, err := p.expect(tokenGreater)
		if err != nil {
			return nil, token{}, err
		}
		return args, end, nil
	}
}

// parseTypeName parses canonical scalar/runtime type names, parameterized
// containers, and Go-style function types such as func(u32, string) bool.
func (p *parser) parseTypeName() (string, sourceSpan, error) {
	start := p.peek()
	if start.kind == tokenIdent || start.kind == tokenNull || start.kind == tokenEnum {
		tok := p.next()
		if _, known := p.knownTypes[tok.text]; !known {
			return "", sourceSpan{}, p.errorf(tok, "unknown type %q", tok.text)
		}
		name := tok.text
		endSpan := spanFromToken(tok)
		if p.match(tokenLess) {
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
				end, err := p.expect(tokenGreater)
				if err != nil {
					return "", sourceSpan{}, err
				}
				endSpan = spanFromToken(end)
				break
			}
			name += "<" + strings.Join(args, ", ") + ">"
		}
		return name, spanCover(spanFromToken(tok), endSpan), nil
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
	return "func(" + strings.Join(params, ", ") + ") " + result, spanCover(spanFromToken(start), resultSpan), nil
}

func builtinTypeSet() map[string]struct{} {
	set := make(map[string]struct{})
	for _, name := range []string{
		"number", "u8", "u16", "u32", "u64", "i8", "i16", "i32", "i64", "f32", "f64",
		"string", "bool", "null", "list", "tensor", "enum", "function", "error", "result", "option",
		"stream", "task", "map", "set", "module", "decimal", "fixed", "interval", "modint", "complex", "bigint",
		"bigfloat", "quantity", "dual", "duration", "datetime", "location",
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
	if _, err := p.expect(tokenLBracket); err != nil {
		return nil, err
	}
	if p.match(tokenRBracket) {
		return &listExpr{Span: spanFromToken(token{line: 0, col: 0})}, nil
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
		if _, err := p.expect(tokenRBracket); err != nil {
			return nil, err
		}
		return &listExpr{items: items}, nil
	}
}

// parseBlockExpr operates on parser values and keeps the parser-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns Expr and an error. Callers should only trust the value when the
// error is nil.
func (p *parser) parseBlockExpr() (Expr, error) {
	if _, err := p.expect(tokenLBrace); err != nil {
		return nil, err
	}
	var stmts []Stmt
	var tail Expr
	for {
		if p.match(tokenRBrace) {
			return &blockExpr{stmts: stmts, tail: tail}, nil
		}
		if tail != nil {
			for p.match(tokenSemicolon) {
			}
			if _, err := p.expect(tokenRBrace); err != nil {
				return nil, err
			}
			return &blockExpr{stmts: stmts, tail: tail}, nil
		}
		switch p.peek().kind {
		case tokenLet, tokenImport, tokenMacro, tokenFunc, tokenReturn, tokenWhile, tokenFor, tokenBreak, tokenContinue, tokenEmit:
			stmt, err := p.parseStmt()
			if err != nil {
				return nil, err
			}
			stmts = append(stmts, stmt)
			for p.match(tokenSemicolon) {
			}
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
			p.next()
			pat = wildcardPattern{}
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
	return &matchExpr{subject: subject, arms: arms, keyword: keyword, Span: spanFromTokens(start, end)}, nil
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
	return orPattern{alternatives: alternatives, Span: patternSpan(first)}, nil
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
		left = andPattern{left: left, right: right, Span: patternSpan(left)}
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
			return notPattern{inner: inner, Span: spanFromToken(tok)}, nil
		}
		if tok.text == "_" {
			return wildcardPattern{}, nil
		}
		if p.match(tokenDot) {
			caseTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			item := enumPattern{typeName: tok.text, caseName: caseTok.text, Span: spanFromTokens(tok, caseTok)}
			if p.match(tokenLParen) && !p.match(tokenRParen) {
				for {
					child, err := p.parsePattern()
					if err != nil {
						return nil, err
					}
					item.items = append(item.items, child)
					if p.match(tokenComma) {
						continue
					}
					if _, err := p.expect(tokenRParen); err != nil {
						return nil, err
					}
					break
				}
			}
			return item, nil
		}
		if p.peek().kind == tokenIdent && p.peek().text != "when" && p.peek().text != "and" {
			binding := p.next()
			return typePattern{typeName: tok.text, binding: binding.text, Span: spanFromTokens(tok, binding)}, nil
		}
		return bindPattern{name: tok.text, Span: spanFromToken(tok)}, nil
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
		return relationalPattern{op: op.kind, value: value.value, Span: spanFromToken(op)}, nil
	case tokenNumber:
		p.next()
		v, err := parseNumericLiteral(tok.text)
		if err != nil {
			return nil, p.errorf(tok, "%v", err)
		}
		return literalPattern{value: v, Span: spanFromToken(tok)}, nil
	case tokenString:
		p.next()
		return literalPattern{value: stringValue(tok.text), Span: spanFromToken(tok)}, nil
	case tokenTrue:
		p.next()
		return literalPattern{value: boolValue(true), Span: spanFromToken(tok)}, nil
	case tokenFalse:
		p.next()
		return literalPattern{value: boolValue(false), Span: spanFromToken(tok)}, nil
	case tokenNull:
		p.next()
		return literalPattern{value: nullValue(), Span: spanFromToken(tok)}, nil
	case tokenLBracket:
		p.next()
		if p.match(tokenRBracket) {
			return listPattern{Span: spanFromToken(tok)}, nil
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
			if _, err := p.expect(tokenRBracket); err != nil {
				return nil, err
			}
			return listPattern{items: items, Span: spanFromToken(tok)}, nil
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
		p.cur = token{kind: tokenEOF}
		p.peeked = true
		p.cur.text = err.Error()
		return p.cur
	}
	p.cur = tok
	p.peeked = true
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
	p.peeked = false
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
	base := fmt.Sprintf(format, args...)
	if tok.kind == tokenEOF {
		return fmt.Errorf("%s: %s", p.sourceName, base)
	}
	return fmt.Errorf("%s:%d:%d: %s", p.sourceName, tok.line, tok.col, base)
}
