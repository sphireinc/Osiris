package osiris

import (
	"fmt"
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
	Expr     Expr
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
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
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

type Expr interface {
	exprNode()
}

type literalExpr struct {
	value Value
	Span  sourceSpan
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
	callee Expr
	args   []Expr
	Span   sourceSpan
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
	pattern Pattern
	body    Expr
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
	p := &parser{sourceName: sourceName, lexer: newLexer(src)}
	prog, err := p.parse()
	if err != nil {
		return nil, normalizeError(err, sourceName)
	}
	return prog, nil
}

type parser struct {
	sourceName string
	lexer      *lexer
	cur        token
	peeked     bool
	lexErr     error
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
		nameTok, err := p.expect(tokenIdent)
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
		return letStmt{Name: nameTok.text, Expr: expr, Span: spanFromTokens(start, nameTok), NameSpan: spanFromToken(nameTok)}, nil
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
		if _, err := p.expect(tokenLParen); err != nil {
			return nil, err
		}
		params, err := p.parseNameListAllowEmpty()
		if err != nil {
			return nil, err
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
		return funcStmt{Name: nameTok.text, Params: params, Body: body, Span: spanFromTokens(start, nameTok), NameSpan: spanFromToken(nameTok)}, nil
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
		return nil, p.errorf(p.peek(), "expected statement starting with let, emit, func, return, while, for, break, continue, package, export, or import")
	}
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
		case tokenLParen:
			args, err := p.parseCallArgs()
			if err != nil {
				return nil, err
			}
			left = &callExpr{callee: left, args: args}
		case tokenDot:
			p.next()
			nameTok, err := p.expect(tokenIdent)
			if err != nil {
				return nil, err
			}
			left = &memberExpr{object: left, name: nameTok.text, NameSpan: spanFromToken(nameTok)}
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
		return &literalExpr{value: v, Span: spanFromToken(tok)}, nil
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
		return &identExpr{name: tok.text, Span: spanFromToken(tok)}, nil
	case tokenLParen:
		p.next()
		expr, err := p.parseExpr(0)
		if err != nil {
			return nil, err
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
	if _, err := p.expect(keyword); err != nil {
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
	for {
		if p.match(tokenRBrace) {
			break
		}
		pat, err := p.parsePattern()
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
		arms = append(arms, matchArm{pattern: pat, body: body})
		if p.match(tokenComma) || p.match(tokenSemicolon) {
			continue
		}
		if p.match(tokenRBrace) {
			break
		}
		return nil, p.errorf(p.peek(), "expected , ; or } after match arm")
	}
	if len(arms) == 0 {
		return nil, p.errorf(p.peek(), "match requires at least one arm")
	}
	return &matchExpr{subject: subject, arms: arms, keyword: keyword}, nil
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
	tok := p.peek()
	switch tok.kind {
	case tokenIdent:
		p.next()
		if tok.text == "_" {
			return wildcardPattern{}, nil
		}
		return bindPattern{name: tok.text, Span: spanFromToken(tok)}, nil
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
