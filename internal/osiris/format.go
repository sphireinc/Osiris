package osiris

import (
	"bytes"
	"fmt"
	"strings"
)

func FormatSource(sourceName, src string) (string, error) {
	prog, err := Parse(sourceName, src)
	if err != nil {
		return "", err
	}
	return FormatProgram(prog), nil
}

func FormatProgram(prog *Program) string {
	var b bytes.Buffer
	for i, stmt := range prog.Statements {
		if i > 0 {
			b.WriteByte('\n')
		}
		b.WriteString(formatStmt(stmt))
	}
	if len(prog.Statements) > 0 {
		b.WriteByte('\n')
	}
	return b.String()
}

func formatStmt(stmt Stmt) string {
	switch s := stmt.(type) {
	case packageStmt:
		if s.Version != "" {
			return "package " + s.Name + " " + s.Version
		}
		return "package " + s.Name
	case exportStmt:
		return "export " + strings.Join(s.Names, ", ")
	case letStmt:
		return "let " + s.Name + " = " + formatExpr(s.Expr, 0)
	case emitStmt:
		return "emit " + formatExpr(s.Expr, 0)
	case macroStmt:
		return "macro " + s.Name + "(" + strings.Join(s.Params, ", ") + ") => " + formatExpr(s.Body, 0)
	case funcStmt:
		if _, ok := s.Body.(*blockExpr); ok {
			return "func " + s.Name + "(" + strings.Join(s.Params, ", ") + ") " + formatExpr(s.Body, 0)
		}
		return "func " + s.Name + "(" + strings.Join(s.Params, ", ") + ") => " + formatExpr(s.Body, 0)
	case returnStmt:
		if s.Expr == nil {
			return "return"
		}
		return "return " + formatExpr(s.Expr, 0)
	case whileStmt:
		return "while " + formatExpr(s.Cond, 0) + " " + formatExpr(s.Body, 0)
	case forStmt:
		return "for " + s.Name + " in " + formatExpr(s.Iter, 0) + " " + formatExpr(s.Body, 0)
	case breakStmt:
		return "break"
	case continueStmt:
		return "continue"
	case importStmt:
		return fmt.Sprintf("import %s as %s", quoteString(s.Path), s.Alias)
	default:
		return "<unknown>"
	}
}

func formatExpr(expr Expr, parentPrec int) string {
	switch n := expr.(type) {
	case *literalExpr:
		return n.value.String()
	case *identExpr:
		return n.name
	case *memberExpr:
		s := formatExpr(n.object, precPostfix) + "." + n.name
		if precPostfix < parentPrec {
			return "(" + s + ")"
		}
		return s
	case *callExpr:
		var b strings.Builder
		b.WriteString(formatExpr(n.callee, precPostfix))
		b.WriteByte('(')
		for i, arg := range n.args {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatExpr(arg, 0))
		}
		b.WriteByte(')')
		s := b.String()
		if precPostfix < parentPrec {
			return "(" + s + ")"
		}
		return s
	case *listExpr:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range n.items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatExpr(item, 0))
		}
		b.WriteByte(']')
		return b.String()
	case *blockExpr:
		var b strings.Builder
		b.WriteByte('{')
		if len(n.stmts) > 0 || n.tail != nil {
			b.WriteByte(' ')
		}
		for i, stmt := range n.stmts {
			if i > 0 {
				b.WriteString("; ")
			}
			b.WriteString(formatStmt(stmt))
		}
		if n.tail != nil {
			if len(n.stmts) > 0 {
				b.WriteString("; ")
			}
			b.WriteString(formatExpr(n.tail, 0))
		}
		if len(n.stmts) > 0 || n.tail != nil {
			b.WriteByte(' ')
		}
		b.WriteByte('}')
		return b.String()
	case *matchExpr:
		var b strings.Builder
		if n.keyword == tokenSwitch {
			b.WriteString("switch ")
		} else {
			b.WriteString("match ")
		}
		b.WriteString(formatExpr(n.subject, precLowest))
		b.WriteString(" {")
		for i, arm := range n.arms {
			if i > 0 {
				b.WriteString("; ")
			} else {
				b.WriteByte(' ')
			}
			b.WriteString(formatPattern(arm.pattern))
			b.WriteString(" => ")
			b.WriteString(formatExpr(arm.body, precLowest))
		}
		if len(n.arms) > 0 {
			b.WriteByte(' ')
		}
		b.WriteByte('}')
		return b.String()
	case *unaryExpr:
		s := unarySymbol(n.op) + formatExpr(n.expr, precUnary)
		if precUnary < parentPrec {
			return "(" + s + ")"
		}
		return s
	case *binaryExpr:
		prec := binaryPrecedence(n.op)
		left := formatExpr(n.left, prec)
		right := formatExpr(n.right, prec+1)
		s := left + " " + n.op.String() + " " + right
		if prec < parentPrec {
			return "(" + s + ")"
		}
		return s
	default:
		return "<expr>"
	}
}

const (
	precLowest  = 0
	precBinary  = 10
	precUnary   = 20
	precPostfix = 30
)

func binaryPrecedence(op tokenKind) int {
	switch op {
	case tokenOrOr:
		return 1
	case tokenAndAnd:
		return 2
	case tokenPipe:
		return 3
	case tokenCaret:
		return 4
	case tokenAmp:
		return 5
	case tokenShiftLeft, tokenShiftRight:
		return 6
	case tokenEqualEqual, tokenBangEqual:
		return 7
	case tokenLess, tokenLessEqual, tokenGreater, tokenGreaterEqual:
		return 8
	case tokenPlus, tokenMinus:
		return 9
	case tokenStar, tokenSlash, tokenPercent:
		return 10
	default:
		return 0
	}
}

func unarySymbol(op tokenKind) string {
	switch op {
	case tokenMinus:
		return "-"
	case tokenBang:
		return "!"
	case tokenTilde:
		return "~"
	default:
		return op.String()
	}
}

func formatPattern(p Pattern) string {
	switch n := p.(type) {
	case wildcardPattern:
		return "_"
	case bindPattern:
		return n.name
	case literalPattern:
		return n.value.String()
	case listPattern:
		var b strings.Builder
		b.WriteByte('[')
		for i, item := range n.items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatPattern(item))
		}
		b.WriteByte(']')
		return b.String()
	default:
		return "<pattern>"
	}
}

func quoteString(s string) string {
	var b strings.Builder
	b.WriteByte('"')
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"':
			b.WriteByte('\\')
			b.WriteByte(s[i])
		case '\n':
			b.WriteString(`\n`)
		case '\r':
			b.WriteString(`\r`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(s[i])
		}
	}
	b.WriteByte('"')
	return b.String()
}
