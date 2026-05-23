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

// stmtNode handles on letStmt the stmt node. It keeps the letStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (letStmt) stmtNode() {}

type emitStmt struct {
	Expr Expr
	Span sourceSpan
}

// stmtNode handles on emitStmt the stmt node. It keeps the emitStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (emitStmt) stmtNode() {}

type macroStmt struct {
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode handles on macroStmt the stmt node. It keeps the macroStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (macroStmt) stmtNode() {}

type funcStmt struct {
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode handles on funcStmt the stmt node. It keeps the funcStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (funcStmt) stmtNode() {}

type returnStmt struct {
	Expr Expr
	Span sourceSpan
}

// stmtNode handles on returnStmt the stmt node. It keeps the returnStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (returnStmt) stmtNode() {}

type whileStmt struct {
	Cond Expr
	Body Expr
	Span sourceSpan
}

// stmtNode handles on whileStmt the stmt node. It keeps the whileStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (whileStmt) stmtNode() {}

type forStmt struct {
	Name     string
	Iter     Expr
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

// stmtNode handles on forStmt the stmt node. It keeps the forStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (forStmt) stmtNode() {}

type breakStmt struct {
	Span sourceSpan
}

// stmtNode handles on breakStmt the stmt node. It keeps the breakStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (breakStmt) stmtNode() {}

type continueStmt struct {
	Span sourceSpan
}

// stmtNode handles on continueStmt the stmt node. It keeps the continueStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (continueStmt) stmtNode() {}

type importStmt struct {
	Path      string
	Alias     string
	Span      sourceSpan
	PathSpan  sourceSpan
	AliasSpan sourceSpan
}

// stmtNode handles on importStmt the stmt node. It keeps the importStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (importStmt) stmtNode() {}

type packageStmt struct {
	Name        string
	Version     string
	Span        sourceSpan
	NameSpan    sourceSpan
	VersionSpan sourceSpan
}

// stmtNode handles on packageStmt the stmt node. It keeps the packageStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (packageStmt) stmtNode() {}

type exportStmt struct {
	Names     []string
	Span      sourceSpan
	NameSpans []sourceSpan
}

// stmtNode handles on exportStmt the stmt node. It keeps the exportStmt subsystem self-contained by handling the stmt node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (exportStmt) stmtNode() {}

type Expr interface {
	exprNode()
}

type literalExpr struct {
	value Value
	Span  sourceSpan
}

// exprNode handles on literalExpr the expr node. It keeps the literalExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (literalExpr) exprNode() {}

type identExpr struct {
	name string
	Span sourceSpan
}

// exprNode handles on identExpr the expr node. It keeps the identExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (identExpr) exprNode() {}

type unaryExpr struct {
	op   tokenKind
	expr Expr
	Span sourceSpan
}

// exprNode handles on unaryExpr the expr node. It keeps the unaryExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (unaryExpr) exprNode() {}

type binaryExpr struct {
	op    tokenKind
	left  Expr
	right Expr
	Span  sourceSpan
}

// exprNode handles on binaryExpr the expr node. It keeps the binaryExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (binaryExpr) exprNode() {}

type callExpr struct {
	callee Expr
	args   []Expr
	Span   sourceSpan
}

// exprNode handles on callExpr the expr node. It keeps the callExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (callExpr) exprNode() {}

type blockExpr struct {
	stmts []Stmt
	tail  Expr
	Span  sourceSpan
}

// exprNode handles on blockExpr the expr node. It keeps the blockExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (blockExpr) exprNode() {}

type tryExpr struct {
	body      Expr
	catchName string
	handler   Expr
	Span      sourceSpan
}

// exprNode handles on tryExpr the expr node. It keeps the tryExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (tryExpr) exprNode() {}

type memberExpr struct {
	object   Expr
	name     string
	Span     sourceSpan
	NameSpan sourceSpan
}

// exprNode handles on memberExpr the expr node. It keeps the memberExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (memberExpr) exprNode() {}

type listExpr struct {
	items []Expr
	Span  sourceSpan
}

// exprNode handles on listExpr the expr node. It keeps the listExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (listExpr) exprNode() {}

type matchExpr struct {
	subject Expr
	arms    []matchArm
	Span    sourceSpan
	keyword tokenKind
}

// exprNode handles on matchExpr the expr node. It keeps the matchExpr subsystem self-contained by handling the expr node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (matchExpr) exprNode() {}

type matchArm struct {
	pattern Pattern
	body    Expr
}

type Pattern interface {
	patternNode()
}

type wildcardPattern struct{}

// patternNode handles on wildcardPattern the pattern node. It keeps the wildcardPattern subsystem self-contained by handling the pattern node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (wildcardPattern) patternNode() {}

type bindPattern struct {
	name string
	Span sourceSpan
}

// patternNode handles on bindPattern the pattern node. It keeps the bindPattern subsystem self-contained by handling the pattern node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (bindPattern) patternNode() {}

type literalPattern struct {
	value Value
	Span  sourceSpan
}

// patternNode handles on literalPattern the pattern node. It keeps the literalPattern subsystem self-contained by handling the pattern node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (literalPattern) patternNode() {}

type listPattern struct {
	items []Pattern
	Span  sourceSpan
}

// patternNode handles on listPattern the pattern node. It keeps the listPattern subsystem self-contained by handling the pattern node-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (listPattern) patternNode() {}

// Parse parses the supplied the requested work and returns the structured form that the evaluator, optimizer, or analyzer expects to work on.. It returns *Program and an error so callers can inspect the computed value while still receiving a failure report when one occurs.
func Parse(sourceName, src string) (*Program, error) {
	p := &parser{sourceName: sourceName, lexer: newLexer(src)}
	return p.parse()
}

type parser struct {
	sourceName string
	lexer      *lexer
	cur        token
	peeked     bool
	lexErr     error
}

// parse handles on parser the parse. It keeps the parser subsystem self-contained by handling the parse-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseStmt handles on parser the parse stmt. It keeps the parser subsystem self-contained by handling the parse stmt-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseExpr handles on parser the parse expr. It keeps the parser subsystem self-contained by handling the parse expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parsePostfix handles on parser the parse postfix. It keeps the parser subsystem self-contained by handling the parse postfix-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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
		default:
			return left, nil
		}
	}
}

// parsePrefix handles on parser the parse prefix. It keeps the parser subsystem self-contained by handling the parse prefix-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseTryExpr handles on parser the parse try expr. It keeps the parser subsystem self-contained by handling the parse try expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseCallArgs handles on parser the parse call args. It keeps the parser subsystem self-contained by handling the parse call args-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseNameList handles on parser the parse name list. It keeps the parser subsystem self-contained by handling the parse name list-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseNameListAllowEmpty handles on parser the parse name list allow empty. It keeps the parser subsystem self-contained by handling the parse name list allow empty-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseList handles on parser the parse list. It keeps the parser subsystem self-contained by handling the parse list-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseBlockExpr handles on parser the parse block expr. It keeps the parser subsystem self-contained by handling the parse block expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// parseMatchExpr handles on parser the parse match expr. It keeps the parser subsystem self-contained by handling the parse match expr-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// exprSpan performs the expr span step in the surrounding package and returns sourceSpan.. It returns sourceSpan so callers can continue with the next stage of the pipeline without re-deriving the same information.
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
	default:
		return sourceSpan{}
	}
}

// parsePattern handles on parser the parse pattern. It keeps the parser subsystem self-contained by handling the parse pattern-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// precedence performs the precedence step in the surrounding package and returns int, bool.. It returns int, bool so callers can continue with the next stage of the pipeline without re-deriving the same information.
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

// peek handles on parser the peek. It keeps the parser subsystem self-contained by handling the peek-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
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

// next handles on parser the next. It keeps the parser subsystem self-contained by handling the next-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *parser) next() token {
	tok := p.peek()
	p.peeked = false
	return tok
}

// match handles on parser the match. It keeps the parser subsystem self-contained by handling the match-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *parser) match(kind tokenKind) bool {
	if p.peek().kind != kind {
		return false
	}
	p.next()
	return true
}

// expect handles on parser the expect. It keeps the parser subsystem self-contained by handling the expect-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *parser) expect(kind tokenKind) (token, error) {
	tok := p.peek()
	if tok.kind != kind {
		return token{}, p.errorf(tok, "expected %s, found %s", kind, tok.kind)
	}
	p.next()
	return tok, nil
}

// errorf handles on parser the errorf. It keeps the parser subsystem self-contained by handling the errorf-specific logic behind a narrow method boundary and returning whatever the surrounding caller needs to continue.
func (p *parser) errorf(tok token, format string, args ...any) error {
	base := fmt.Sprintf(format, args...)
	if tok.kind == tokenEOF {
		return fmt.Errorf("%s: %s", p.sourceName, base)
	}
	return fmt.Errorf("%s:%d:%d: %s", p.sourceName, tok.line, tok.col, base)
}
