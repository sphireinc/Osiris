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
	tokenEmit
	tokenMacro
	tokenFunc
	tokenReturn
	tokenWhile
	tokenFor
	tokenIn
	tokenBreak
	tokenContinue
	tokenPackage
	tokenExport
	tokenImport
	tokenMatch
	tokenSwitch
	tokenAs
	tokenTrue
	tokenFalse
	tokenNull
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
	line int
	col  int
}

func (t token) String() string {
	return fmt.Sprintf("%s(%q @ %d:%d)", t.kind, t.text, t.line, t.col)
}

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
	case tokenEmit:
		return "emit"
	case tokenMacro:
		return "macro"
	case tokenFunc:
		return "func"
	case tokenReturn:
		return "return"
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
	case tokenPackage:
		return "package"
	case tokenExport:
		return "export"
	case tokenImport:
		return "import"
	case tokenMatch:
		return "match"
	case tokenSwitch:
		return "switch"
	case tokenAs:
		return "as"
	case tokenTrue:
		return "true"
	case tokenFalse:
		return "false"
	case tokenNull:
		return "null"
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

func newLexer(src string) *lexer {
	return &lexer{src: src, line: 1, col: 1}
}

func (l *lexer) nextToken() (token, error) {
	l.skipSpace()
	if l.off >= len(l.src) {
		return token{kind: tokenEOF, line: l.line, col: l.col}, nil
	}

	ch, width := utf8.DecodeRuneInString(l.src[l.off:])
	startLine, startCol := l.line, l.col

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
	}

	if unicode.IsDigit(ch) || (ch == '.' && l.hasDigitAhead(1)) {
		return l.scanNumber()
	}
	if isIdentStart(ch) {
		return l.scanIdent()
	}
	return token{}, fmt.Errorf("unexpected character %q at %d:%d", ch, startLine, startCol)
}

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
	case "emit":
		return token{kind: tokenEmit, text: text, line: startLine, col: startCol}, nil
	case "macro":
		return token{kind: tokenMacro, text: text, line: startLine, col: startCol}, nil
	case "func":
		return token{kind: tokenFunc, text: text, line: startLine, col: startCol}, nil
	case "return":
		return token{kind: tokenReturn, text: text, line: startLine, col: startCol}, nil
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
	case "package":
		return token{kind: tokenPackage, text: text, line: startLine, col: startCol}, nil
	case "export":
		return token{kind: tokenExport, text: text, line: startLine, col: startCol}, nil
	case "import":
		return token{kind: tokenImport, text: text, line: startLine, col: startCol}, nil
	case "match":
		return token{kind: tokenMatch, text: text, line: startLine, col: startCol}, nil
	case "switch":
		return token{kind: tokenSwitch, text: text, line: startLine, col: startCol}, nil
	case "as":
		return token{kind: tokenAs, text: text, line: startLine, col: startCol}, nil
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

func (l *lexer) scanNumber() (token, error) {
	start := l.off
	startLine, startCol := l.line, l.col
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if unicode.IsDigit(r) || unicode.IsLetter(r) || r == '.' || r == '_' {
			l.advance(w)
			continue
		}
		break
	}
	text := l.src[start:l.off]
	return token{kind: tokenNumber, text: text, line: startLine, col: startCol}, nil
}

func (l *lexer) scanString() (token, error) {
	startLine, startCol := l.line, l.col
	l.advance(1)
	var out strings.Builder
	for l.off < len(l.src) {
		r, w := utf8.DecodeRuneInString(l.src[l.off:])
		if r == '"' {
			l.advance(w)
			return token{kind: tokenString, text: out.String(), line: startLine, col: startCol}, nil
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
	return token{}, fmt.Errorf("unterminated string at %d:%d", startLine, startCol)
}

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

func (l *lexer) hasDigitAhead(delta int) bool {
	off := l.off + delta
	if off >= len(l.src) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(l.src[off:])
	return unicode.IsDigit(r)
}

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

func (l *lexer) match(text string) bool {
	return strings.HasPrefix(l.src[l.off:], text)
}

func isIdentStart(r rune) bool {
	return r == '_' || unicode.IsLetter(r)
}

func isIdentPart(r rune) bool {
	return r == '_' || unicode.IsLetter(r) || unicode.IsDigit(r)
}
