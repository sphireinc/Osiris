package osiris

import (
	"fmt"
	"strings"
	"unicode"
	"unicode/utf8"
)

type tokenKind uint8

const (
	tokenEOF tokenKind = iota
	tokenIdent
	tokenNumber
	tokenString
	tokenLet
	tokenVar
	tokenConst
	tokenLazy
	tokenEmit
	tokenMacro
	tokenFunc
	tokenReturn
	tokenTry
	tokenCatch
	tokenWhile
	tokenFor
	tokenIn
	tokenBreak
	tokenContinue
	tokenDefer
	tokenPackage
	tokenExport
	tokenEnum
	tokenStruct
	tokenType
	tokenNewtype
	tokenImport
	tokenFFI
	tokenMatch
	tokenSwitch
	tokenAs
	tokenWith
	tokenTrue
	tokenFalse
	tokenNull
	tokenTag
	tokenLParen
	tokenRParen
	tokenLBrace
	tokenRBrace
	tokenLBracket
	tokenRBracket
	tokenDot
	tokenComma
	tokenColon
	tokenSemicolon
	tokenPlus
	tokenMinus
	tokenStar
	tokenSlash
	tokenPercent
	tokenBang
	tokenQuestion
	tokenEqual
	tokenArrow
	tokenEqualEqual
	tokenBangEqual
	tokenLess
	tokenLessEqual
	tokenGreater
	tokenGreaterEqual
	tokenAmp
	tokenPipe
	tokenCaret
	tokenTilde
	tokenShiftLeft
	tokenShiftRight
	tokenAndAnd
	tokenOrOr
)

type token struct {
	kind tokenKind
	text string
	// raw preserves the source spelling when text is normalized for the
	// parser, such as decoded strings and field tags. It is used only for
	// source ranges; semantic consumers continue to use text.
	raw  string
	line int
	col  int
}

// String operates on token values and keeps the token-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (t token) String() string {
	return fmt.Sprintf("%s(%q @ %d:%d)", t.kind, t.text, t.line, t.col)
}

// String operates on tokenKind values and keeps the tokenkind-specific
// behavior attached to the receiver so callers can use the method as the
// natural entry point for that data structure.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func (k tokenKind) String() string {
	switch k {
	case tokenEOF:
		return "eof"
	case tokenIdent:
		return "ident"
	case tokenNumber:
		return "number"
	case tokenString:
		return "string"
	case tokenLet:
		return "let"
	case tokenVar:
		return "var"
	case tokenConst:
		return "const"
	case tokenLazy:
		return "lazy"
	case tokenEmit:
		return "emit"
	case tokenMacro:
		return "macro"
	case tokenFunc:
		return "func"
	case tokenReturn:
		return "return"
	case tokenTry:
		return "try"
	case tokenCatch:
		return "catch"
	case tokenWhile:
		return "while"
	case tokenFor:
		return "for"
	case tokenIn:
		return "in"
	case tokenBreak:
		return "break"
	case tokenContinue:
		return "continue"
	case tokenDefer:
		return "defer"
	case tokenPackage:
		return "package"
	case tokenExport:
		return "export"
	case tokenEnum:
		return "enum"
	case tokenStruct:
		return "struct"
	case tokenType:
		return "type"
	case tokenNewtype:
		return "newtype"
	case tokenImport:
		return "import"
	case tokenFFI:
		return "ffi"
	case tokenMatch:
		return "match"
	case tokenSwitch:
		return "switch"
	case tokenAs:
		return "as"
	case tokenWith:
		return "with"
	case tokenTrue:
		return "true"
	case tokenFalse:
		return "false"
	case tokenNull:
		return "null"
	case tokenTag:
		return "tag"
	case tokenLParen:
		return "("
	case tokenRParen:
		return ")"
	case tokenLBrace:
		return "{"
	case tokenRBrace:
		return "}"
	case tokenLBracket:
		return "["
	case tokenRBracket:
		return "]"
	case tokenDot:
		return "."
	case tokenComma:
		return ","
	case tokenColon:
		return ":"
	case tokenSemicolon:
		return ";"
	case tokenPlus:
		return "+"
	case tokenMinus:
		return "-"
	case tokenStar:
		return "*"
	case tokenSlash:
		return "/"
	case tokenPercent:
		return "%"
	case tokenBang:
		return "!"
	case tokenQuestion:
		return "?"
	case tokenEqual:
		return "="
	case tokenArrow:
		return "=>"
	case tokenEqualEqual:
		return "=="
	case tokenBangEqual:
		return "!="
	case tokenLess:
		return "<"
	case tokenLessEqual:
		return "<="
	case tokenGreater:
		return ">"
	case tokenGreaterEqual:
		return ">="
	case tokenAmp:
		return "&"
	case tokenPipe:
		return "|"
	case tokenCaret:
		return "^"
	case tokenTilde:
		return "~"
	case tokenShiftLeft:
		return "<<"
	case tokenShiftRight:
		return ">>"
	case tokenAndAnd:
		return "&&"
	case tokenOrOr:
		return "||"
	default:
		return "<?>"
	}
}

type lexer struct {
	src  string
	off  int
	line int
	col  int
}

// lexerError preserves the source position of a failure that happens while
// scanning a token. The parser can then report the lexer failure with the
// same source-qualified diagnostic shape as parser errors.
type lexerError struct {
	message string
	line    int
	col     int
}

func (e *lexerError) Error() string {
	return fmt.Sprintf("%s at %d:%d", e.message, e.line, e.col)
}

// newLexer handles the new lexer step in the surrounding package and returns
// *lexer.
//
// It returns *lexer so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func newLexer(src string) *lexer {
	return &lexer{src: src, line: 1, col: 1}
}

// nextToken operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token and an error. Callers should only trust the value when the
// error is nil.
func (l *lexer) nextToken() (token, error) {
	l.skipSpace()
	if l.off >= len(l.src) {
		return token{kind: tokenEOF, line: l.line, col: l.col}, nil
	}

	ch, width := utf8.DecodeRuneInString(l.src[l.off:])
	startLine, startCol := l.line, l.col
	// A leading dot belongs to a numeric literal when it is followed by a
	// digit. Keep this check ahead of the punctuation switch so `.5` cannot be
	// split into a dot token and an incomplete expression.
	if ch == '.' && l.hasDigitAhead(1) {
		return l.scanNumber()
	}

	switch ch {
	case '(':
		l.advance(width)
		return token{kind: tokenLParen, text: "(", line: startLine, col: startCol}, nil
	case ')':
		l.advance(width)
		return token{kind: tokenRParen, text: ")", line: startLine, col: startCol}, nil
	case '{':
		l.advance(width)
		return token{kind: tokenLBrace, text: "{", line: startLine, col: startCol}, nil
	case '}':
		l.advance(width)
		return token{kind: tokenRBrace, text: "}", line: startLine, col: startCol}, nil
	case '[':
		l.advance(width)
		return token{kind: tokenLBracket, text: "[", line: startLine, col: startCol}, nil
	case ']':
		l.advance(width)
		return token{kind: tokenRBracket, text: "]", line: startLine, col: startCol}, nil
	case '.':
		l.advance(width)
		return token{kind: tokenDot, text: ".", line: startLine, col: startCol}, nil
	case ',':
		l.advance(width)
		return token{kind: tokenComma, text: ",", line: startLine, col: startCol}, nil
	case ':':
		l.advance(width)
		return token{kind: tokenColon, text: ":", line: startLine, col: startCol}, nil
	case ';':
		l.advance(width)
		return token{kind: tokenSemicolon, text: ";", line: startLine, col: startCol}, nil
	case '+':
		l.advance(width)
		return token{kind: tokenPlus, text: "+", line: startLine, col: startCol}, nil
	case '-':
		l.advance(width)
		return token{kind: tokenMinus, text: "-", line: startLine, col: startCol}, nil
	case '*':
		l.advance(width)
		return token{kind: tokenStar, text: "*", line: startLine, col: startCol}, nil
	case '%':
		l.advance(width)
		return token{kind: tokenPercent, text: "%", line: startLine, col: startCol}, nil
	case '~':
		l.advance(width)
		return token{kind: tokenTilde, text: "~", line: startLine, col: startCol}, nil
	case '^':
		l.advance(width)
		return token{kind: tokenCaret, text: "^", line: startLine, col: startCol}, nil
	case '!':
		l.advance(width)
		if l.match("=") {
			l.advance(1)
			return token{kind: tokenBangEqual, text: "!=", line: startLine, col: startCol}, nil
		}
		return token{kind: tokenBang, text: "!", line: startLine, col: startCol}, nil
	case '?':
		l.advance(width)
		return token{kind: tokenQuestion, text: "?", line: startLine, col: startCol}, nil
	case '=':
		l.advance(width)
		if l.match(">") {
			l.advance(1)
			return token{kind: tokenArrow, text: "=>", line: startLine, col: startCol}, nil
		}
		if l.match("=") {
			l.advance(1)
			return token{kind: tokenEqualEqual, text: "==", line: startLine, col: startCol}, nil
		}
		return token{kind: tokenEqual, text: "=", line: startLine, col: startCol}, nil
	case '<':
		l.advance(width)
		if l.match("<") {
			l.advance(1)
			return token{kind: tokenShiftLeft, text: "<<", line: startLine, col: startCol}, nil
		}
		if l.match("=") {
			l.advance(1)
			return token{kind: tokenLessEqual, text: "<=", line: startLine, col: startCol}, nil
		}
		return token{kind: tokenLess, text: "<", line: startLine, col: startCol}, nil
	case '>':
		l.advance(width)
		if l.match(">") {
			l.advance(1)
			return token{kind: tokenShiftRight, text: ">>", line: startLine, col: startCol}, nil
		}
		if l.match("=") {
			l.advance(1)
			return token{kind: tokenGreaterEqual, text: ">=", line: startLine, col: startCol}, nil
		}
		return token{kind: tokenGreater, text: ">", line: startLine, col: startCol}, nil
	case '&':
		if l.match("&&") {
			l.advance(2)
			return token{kind: tokenAndAnd, text: "&&", line: startLine, col: startCol}, nil
		}
		l.advance(width)
		return token{kind: tokenAmp, text: "&", line: startLine, col: startCol}, nil
	case '|':
		if l.match("||") {
			l.advance(2)
			return token{kind: tokenOrOr, text: "||", line: startLine, col: startCol}, nil
		}
		l.advance(width)
		return token{kind: tokenPipe, text: "|", line: startLine, col: startCol}, nil
	case '/':
		if l.match("//") {
			l.advance(2)
			for l.off < len(l.src) {
				r, w := utf8.DecodeRuneInString(l.src[l.off:])
				if r == '\n' {
					break
				}
				l.advance(w)
			}
			return l.nextToken()
		}
		l.advance(width)
		return token{kind: tokenSlash, text: "/", line: startLine, col: startCol}, nil
	case '#':
		for l.off < len(l.src) {
			r, w := utf8.DecodeRuneInString(l.src[l.off:])
			if r == '\n' {
				break
			}
			l.advance(w)
		}
		return l.nextToken()
	case '"':
		return l.scanString()
	case '`':
		return l.scanTag()
	}

	if unicode.IsDigit(ch) {
		return l.scanNumber()
	}
	if isIdentStart(ch) {
		return l.scanIdent()
	}
	return token{}, &lexerError{message: fmt.Sprintf("unexpected character %q", ch), line: startLine, col: startCol}
}

// scanIdent operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token and an error. Callers should only trust the value when the
// error is nil.
func (l *lexer) scanIdent() (token, error) {
	start := l.off
	startLine, startCol := l.line, l.col
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if !isIdentPart(r) {
			break
		}
		l.advance(w)
	}
	text := l.src[start:l.off]
	switch text {
	case "let":
		return token{kind: tokenLet, text: text, line: startLine, col: startCol}, nil
	case "var":
		// var remains contextual so the statistical var(...) builtin is still
		// callable in expression position.
		return token{kind: tokenIdent, text: text, line: startLine, col: startCol}, nil
	case "const":
		return token{kind: tokenConst, text: text, line: startLine, col: startCol}, nil
	case "lazy":
		return token{kind: tokenLazy, text: text, line: startLine, col: startCol}, nil
	case "emit":
		return token{kind: tokenEmit, text: text, line: startLine, col: startCol}, nil
	case "macro":
		return token{kind: tokenMacro, text: text, line: startLine, col: startCol}, nil
	case "func":
		return token{kind: tokenFunc, text: text, line: startLine, col: startCol}, nil
	case "return":
		return token{kind: tokenReturn, text: text, line: startLine, col: startCol}, nil
	case "try":
		return token{kind: tokenTry, text: text, line: startLine, col: startCol}, nil
	case "catch":
		return token{kind: tokenCatch, text: text, line: startLine, col: startCol}, nil
	case "while":
		return token{kind: tokenWhile, text: text, line: startLine, col: startCol}, nil
	case "for":
		return token{kind: tokenFor, text: text, line: startLine, col: startCol}, nil
	case "in":
		return token{kind: tokenIn, text: text, line: startLine, col: startCol}, nil
	case "break":
		return token{kind: tokenBreak, text: text, line: startLine, col: startCol}, nil
	case "continue":
		return token{kind: tokenContinue, text: text, line: startLine, col: startCol}, nil
	case "defer":
		return token{kind: tokenDefer, text: text, line: startLine, col: startCol}, nil
	case "package":
		return token{kind: tokenPackage, text: text, line: startLine, col: startCol}, nil
	case "export":
		return token{kind: tokenExport, text: text, line: startLine, col: startCol}, nil
	case "enum":
		return token{kind: tokenEnum, text: text, line: startLine, col: startCol}, nil
	case "struct":
		return token{kind: tokenStruct, text: text, line: startLine, col: startCol}, nil
	case "type":
		return token{kind: tokenType, text: text, line: startLine, col: startCol}, nil
	case "newtype":
		return token{kind: tokenNewtype, text: text, line: startLine, col: startCol}, nil
	case "import":
		return token{kind: tokenImport, text: text, line: startLine, col: startCol}, nil
	case "ffi":
		return token{kind: tokenFFI, text: text, line: startLine, col: startCol}, nil
	case "match":
		return token{kind: tokenMatch, text: text, line: startLine, col: startCol}, nil
	case "switch":
		return token{kind: tokenSwitch, text: text, line: startLine, col: startCol}, nil
	case "as":
		return token{kind: tokenAs, text: text, line: startLine, col: startCol}, nil
	case "with":
		return token{kind: tokenWith, text: text, line: startLine, col: startCol}, nil
	case "true":
		return token{kind: tokenTrue, text: text, line: startLine, col: startCol}, nil
	case "false":
		return token{kind: tokenFalse, text: text, line: startLine, col: startCol}, nil
	case "null":
		return token{kind: tokenNull, text: text, line: startLine, col: startCol}, nil
	default:
		return token{kind: tokenIdent, text: text, line: startLine, col: startCol}, nil
	}
}

// scanNumber operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token and an error. Callers should only trust the value when the
// error is nil.
func (l *lexer) scanNumber() (token, error) {
	start := l.off
	startLine, startCol := l.line, l.col
	seenExp := false
	allowExpSign := false
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		switch {
		case unicode.IsDigit(r) || r == '_' || r == '.':
			l.advance(w)
			allowExpSign = false
			continue
		case (r == 'e' || r == 'E') && !seenExp:
			seenExp = true
			allowExpSign = true
			l.advance(w)
			continue
		case allowExpSign && (r == '+' || r == '-'):
			allowExpSign = false
			l.advance(w)
			continue
		case unicode.IsLetter(r):
			l.advance(w)
			allowExpSign = false
			continue
		default:
			text := l.src[start:l.off]
			return token{kind: tokenNumber, text: text, line: startLine, col: startCol}, nil
		}
	}
	text := l.src[start:l.off]
	return token{kind: tokenNumber, text: text, line: startLine, col: startCol}, nil
}

// scanString operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns token and an error. Callers should only trust the value when the
// error is nil.
func (l *lexer) scanString() (token, error) {
	start := l.off
	startLine, startCol := l.line, l.col
	l.advance(1)
	var out strings.Builder
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if r == '"' {
			l.advance(w)
			return token{kind: tokenString, text: out.String(), raw: l.src[start:l.off], line: startLine, col: startCol}, nil
		}
		if r == '\\' {
			l.advance(w)
			if l.off >= len(l.src) {
				break
			}
			esc, ew := utf8.DecodeRuneInString(l.src[l.off:])
			switch esc {
			case 'n':
				out.WriteByte('\n')
			case 'r':
				out.WriteByte('\r')
			case 't':
				out.WriteByte('\t')
			case '\\':
				out.WriteByte('\\')
			case '"':
				out.WriteByte('"')
			default:
				out.WriteRune(esc)
			}
			l.advance(ew)
			continue
		}
		out.WriteRune(r)
		l.advance(w)
	}
	return token{}, &lexerError{message: "unterminated string", line: startLine, col: startCol}
}

// scanTag scans a Go-style raw field tag. The delimiters are not part of the
// token text; preserving the contents verbatim lets the parser validate and
// retain unknown namespaces without changing their spelling.
func (l *lexer) scanTag() (token, error) {
	startLine, startCol := l.line, l.col
	l.advance(1)
	start := l.off
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if r == '`' {
			text := l.src[start:l.off]
			l.advance(w)
			return token{kind: tokenTag, text: text, raw: l.src[start-1 : l.off], line: startLine, col: startCol}, nil
		}
		if r == '\n' || r == '\r' {
			return token{}, &lexerError{message: "unterminated field tag", line: startLine, col: startCol}
		}
		l.advance(w)
	}
	return token{}, &lexerError{message: "unterminated field tag", line: startLine, col: startCol}
}

// skipSpace operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (l *lexer) skipSpace() {
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if unicode.IsSpace(r) {
			l.advance(w)
			continue
		}
		break
	}
}

// hasDigitAhead operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (l *lexer) hasDigitAhead(delta int) bool {
	off := l.off + delta
	if off >= len(l.src) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(l.src[off:])
	return unicode.IsDigit(r)
}

// advance operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
func (l *lexer) advance(width int) {
	for i := 0; i < width && l.off < len(l.src); i++ {
		if l.src[l.off] == '\n' {
			l.line++
			l.col = 1
		} else {
			l.col++
		}
		l.off++
	}
}

// match operates on lexer values and keeps the lexer-specific behavior
// attached to the receiver so callers can use the method as the natural entry
// point for that data structure.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func (l *lexer) match(text string) bool {
	return strings.HasPrefix(l.src[l.off:], text)
}

// isIdentStart handles the is ident start step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

// isIdentPart handles the is ident part step in the surrounding package and
// returns bool.
//
// It returns bool so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
