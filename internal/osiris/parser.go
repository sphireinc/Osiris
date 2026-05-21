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

func (letStmt) stmtNode() {}

type emitStmt struct {
	Expr Expr
	Span sourceSpan
}

func (emitStmt) stmtNode() {}

type macroStmt struct {
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

func (macroStmt) stmtNode() {}

type funcStmt struct {
	Name     string
	Params   []string
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

func (funcStmt) stmtNode() {}

type returnStmt struct {
	Expr Expr
	Span sourceSpan
}

func (returnStmt) stmtNode() {}

type whileStmt struct {
	Cond Expr
	Body Expr
	Span sourceSpan
}

func (whileStmt) stmtNode() {}

type forStmt struct {
	Name     string
	Iter     Expr
	Body     Expr
	Span     sourceSpan
	NameSpan sourceSpan
}

func (forStmt) stmtNode() {}

type breakStmt struct {
	Span sourceSpan
}

func (breakStmt) stmtNode() {}

type continueStmt struct {
	Span sourceSpan
}

func (continueStmt) stmtNode() {}

type importStmt struct {
	Path      string
	Alias     string
	Span      sourceSpan
	PathSpan  sourceSpan
	AliasSpan sourceSpan
}

func (importStmt) stmtNode() {}

type packageStmt struct {
	Name        string
	Version     string
	Span        sourceSpan
	NameSpan    sourceSpan
	VersionSpan sourceSpan
}

func (packageStmt) stmtNode() {}

type exportStmt struct {
	Names     []string
	Span      sourceSpan
	NameSpans []sourceSpan
}

func (exportStmt) stmtNode() {}

type Expr interface {
	exprNode()
}

type literalExpr struct {
	value Value
	Span  sourceSpan
}

func (literalExpr) exprNode() {}

type identExpr struct {
	name string
	Span sourceSpan
}

func (identExpr) exprNode() {}

type unaryExpr struct {
	op   tokenKind
	expr Expr
	Span sourceSpan
}

func (unaryExpr) exprNode() {}

type binaryExpr struct {
	op    tokenKind
	left  Expr
	right Expr
	Span  sourceSpan
}

func (binaryExpr) exprNode() {}

type callExpr struct {
	callee Expr
	args   []Expr
	Span   sourceSpan
}

func (callExpr) exprNode() {}

type blockExpr struct {
	stmts []Stmt
	tail  Expr
	Span  sourceSpan
}

func (blockExpr) exprNode() {}

type memberExpr struct {
	object   Expr
	name     string
	Span     sourceSpan
	NameSpan sourceSpan
}

func (memberExpr) exprNode() {}

type listExpr struct {
	items []Expr
	Span  sourceSpan
}

func (listExpr) exprNode() {}

type matchExpr struct {
	subject Expr
	arms    []matchArm
	Span    sourceSpan
	keyword tokenKind
}

func (matchExpr) exprNode() {}

type matchArm struct {
	pattern Pattern
	body    Expr
}

type Pattern interface {
	patternNode()
}

type wildcardPattern struct{}

func (wildcardPattern) patternNode() {}

type bindPattern struct {
	name string
	Span sourceSpan
}

func (bindPattern) patternNode() {}

type literalPattern struct {
	value Value
	Span  sourceSpan
}

func (literalPattern) patternNode() {}

type listPattern struct {
	items []Pattern
	Span  sourceSpan
}

func (listPattern) patternNode() {}

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
		return emitStmt{Expr: expr, Span: spanFromToken(start)}, nil
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
	case tokenMatch, tokenSwitch:
		return p.parseMatchExpr(tok.kind)
	default:
		return nil, p.errorf(tok, "expected expression")
	}
}

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
		case tokenLet, tokenImport, tokenMacro, tokenFunc, tokenReturn, tokenWhile, tokenFor, tokenBreak, tokenContinue:
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
	default:
		return sourceSpan{}
	}
}

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

func (p *parser) next() token {
	tok := p.peek()
	p.peeked = false
	return tok
}

func (p *parser) match(kind tokenKind) bool {
	if p.peek().kind != kind {
		return false
	}
	p.next()
	return true
}

func (p *parser) expect(kind tokenKind) (token, error) {
	tok := p.peek()
	if tok.kind != kind {
		return token{}, p.errorf(tok, "expected %s, found %s", kind, tok.kind)
	}
	p.next()
	return tok, nil
}

func (p *parser) errorf(tok token, format string, args ...any) error {
	base := fmt.Sprintf(format, args...)
	if tok.kind == tokenEOF {
		return fmt.Errorf("%s: %s", p.sourceName, base)
	}
	return fmt.Errorf("%s:%d:%d: %s", p.sourceName, tok.line, tok.col, base)
}
