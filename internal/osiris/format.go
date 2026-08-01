package osiris

import (
	"bytes"
	"fmt"
	"strings"
)

// FormatSource formats the source into a stable textual rendering. The
// implementation is intentionally deterministic so round-tripping and
// diff-based review stay predictable.
//
// It returns string and an error. Callers should only trust the value when the
// error is nil.
func FormatSource(sourceName, src string) (string, error) {
	prog, err := Parse(sourceName, src)
	if err != nil {
		return "", err
	}
	return FormatProgram(prog), nil
}

// FormatProgram formats the program into a stable textual rendering. The
// implementation is intentionally deterministic so round-tripping and
// diff-based review stay predictable.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// formatStmt handles the format stmt step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatStmt(stmt Stmt) string {
	switch s := stmt.(type) {
	case packageStmt:
		if s.Version != "" {
			return "package " + s.Name + " " + s.Version
		}
		return "package " + s.Name
	case exportStmt:
		return "export " + strings.Join(s.Names, ", ")
	case enumStmt:
		cases := make([]string, len(s.Cases))
		for i, item := range s.Cases {
			cases[i] = item.Name
			if len(item.Params) > 0 {
				cases[i] += "(" + strings.Join(item.Params, ", ") + ")"
			}
			if item.BackingValue != nil {
				cases[i] += " = " + item.BackingValue.String()
			}
		}
		backing := ""
		if s.Backing != "" {
			backing = ": " + s.Backing
		}
		return "enum " + s.Name + backing + " { " + strings.Join(cases, ", ") + " }"
	case structStmt:
		fields := make([]string, len(s.Fields))
		for i, field := range s.Fields {
			fields[i] = field.Name + " " + field.Type
			if field.Tag != "" {
				fields[i] += " `" + field.Tag + "`"
			}
		}
		return "struct " + s.Name + " { " + strings.Join(fields, ", ") + " }"
	case typeDeclStmt:
		return "type " + s.Name + " = " + s.Target
	case newtypeStmt:
		return "newtype " + s.Name + " " + s.Target
	case letStmt:
		prefix := "let "
		if s.Unwrap {
			prefix = "let? "
		}
		if len(s.Names) > 0 {
			return prefix + "(" + strings.Join(s.Names, ", ") + ") = " + formatExpr(s.Expr, 0)
		}
		if s.Type != "" {
			return prefix + s.Name + " " + s.Type + " = " + formatExpr(s.Expr, 0)
		}
		return prefix + s.Name + " = " + formatExpr(s.Expr, 0)
	case emitStmt:
		return "emit " + formatEmitExpr(s.Expr)
	case macroStmt:
		return "macro " + s.Name + "(" + strings.Join(s.Params, ", ") + ") => " + formatExpr(s.Body, 0)
	case funcStmt:
		params := make([]string, len(s.Params))
		for i, name := range s.Params {
			params[i] = name
			if i < len(s.ParamTypes) && s.ParamTypes[i] != "" {
				params[i] += " " + s.ParamTypes[i]
			}
		}
		signature := "func " + s.Name
		if len(s.TypeParams) > 0 {
			typeParams := make([]string, len(s.TypeParams))
			for i, param := range s.TypeParams {
				typeParams[i] = param.Name
				if param.Bound != "" {
					typeParams[i] += ": " + param.Bound
				}
			}
			signature += "<" + strings.Join(typeParams, ", ") + ">"
		}
		signature += "(" + strings.Join(params, ", ") + ")"
		if s.ReturnType != "" {
			signature += " " + s.ReturnType
		}
		if _, ok := s.Body.(*blockExpr); ok {
			return signature + " " + formatExpr(s.Body, 0)
		}
		return signature + " => " + formatExpr(s.Body, 0)
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

// formatExpr handles the format expr step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatExpr(expr Expr, parentPrec int) string {
	switch n := expr.(type) {
	case *literalExpr:
		if n.value.kind == kindTensor {
			return formatTensorSource(n.value.shape, n.value.data)
		}
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
		if len(n.typeArgs) > 0 {
			b.WriteByte('<')
			for i, arg := range n.typeArgs {
				if i > 0 {
					b.WriteString(", ")
				}
				b.WriteString(arg)
			}
			b.WriteByte('>')
		}
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
	case *tupleExpr:
		var b strings.Builder
		b.WriteByte('(')
		for i, item := range n.items {
			if i > 0 {
				b.WriteString(", ")
			}
			b.WriteString(formatExpr(item, 0))
		}
		if len(n.items) == 1 {
			b.WriteByte(',')
		}
		b.WriteByte(')')
		return b.String()
	case *structExpr:
		return n.TypeName + " " + formatStructFieldExprs(n.Fields)
	case *structUpdateExpr:
		s := formatExpr(n.Base, precPostfix) + " with " + formatStructFieldExprs(n.Fields)
		if precPostfix < parentPrec {
			return "(" + s + ")"
		}
		return s
	case *indexExpr:
		s := formatExpr(n.Object, precPostfix) + "[" + formatExpr(n.Index, 0) + "]"
		if precPostfix < parentPrec {
			return "(" + s + ")"
		}
		return s
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
	case *tryExpr:
		s := "try " + formatExpr(n.body, precLowest) + " catch " + n.catchName + " " + formatExpr(n.handler, precLowest)
		if precLowest < parentPrec {
			return "(" + s + ")"
		}
		return s
	case *propagateExpr:
		s := formatExpr(n.expr, precLowest) + "?"
		if precLowest < parentPrec {
			return "(" + s + ")"
		}
		return s
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
			if arm.isDefault {
				b.WriteString("default")
			} else {
				b.WriteString(formatPattern(arm.pattern))
			}
			if arm.guard != nil {
				b.WriteString(" when ")
				b.WriteString(formatExpr(arm.guard, precLowest))
			}
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

// formatEmitExpr handles the format emit expr step in the surrounding package
// and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatEmitExpr(expr Expr) string {
	call, ok := expr.(*callExpr)
	if !ok {
		return formatExpr(expr, 0)
	}
	name, ok := call.callee.(*identExpr)
	if !ok || name.name != "cat" {
		return formatExpr(expr, 0)
	}
	if len(call.args) < 3 || len(call.args)%2 == 0 {
		return formatExpr(expr, 0)
	}
	parts := make([]string, 0, (len(call.args)+1)/2)
	for i := 0; i < len(call.args); i += 2 {
		if i > 0 {
			lit, ok := call.args[i-1].(*literalExpr)
			if !ok || lit.value.kind != kindString || lit.value.str != ", " {
				return formatExpr(expr, 0)
			}
		}
		parts = append(parts, formatExpr(call.args[i], 0))
	}
	return strings.Join(parts, ", ")
}

func formatStructFieldExprs(fields []structFieldExpr) string {
	var b strings.Builder
	b.WriteString("{ ")
	for i, field := range fields {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(field.Name)
		b.WriteString(": ")
		b.WriteString(formatExpr(field.Expr, 0))
	}
	b.WriteString(" }")
	return b.String()
}

// formatTensorSource handles the format tensor source step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatTensorSource(shape []int, data []Value) string {
	var b strings.Builder
	b.WriteString("tensor(")
	b.WriteString(formatShapeList(shape))
	b.WriteString(", ")
	b.WriteByte('[')
	for i, item := range data {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(item.String())
	}
	b.WriteByte(']')
	b.WriteByte(')')
	return b.String()
}

// formatShapeList handles the format shape list step in the surrounding
// package and returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatShapeList(shape []int) string {
	var b strings.Builder
	b.WriteByte('[')
	for i, dim := range shape {
		if i > 0 {
			b.WriteString(", ")
		}
		b.WriteString(fmt.Sprintf("%d", dim))
	}
	b.WriteByte(']')
	return b.String()
}

const (
	precLowest  = 0
	precBinary  = 10
	precUnary   = 20
	precPostfix = 30
)

// binaryPrecedence handles the binary precedence step in the surrounding
// package and returns int.
//
// It returns int so callers can continue with the next stage of the pipeline
// without re-deriving the same information.
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

// unarySymbol handles the unary symbol step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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

// formatPattern handles the format pattern step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func formatPattern(p Pattern) string {
	switch n := p.(type) {
	case wildcardPattern:
		return "_"
	case bindPattern:
		return n.name
	case enumPattern:
		name := n.typeName + "." + n.caseName
		if len(n.items) == 0 {
			return name
		}
		parts := make([]string, len(n.items))
		for i, item := range n.items {
			parts[i] = formatPattern(item)
		}
		return name + "(" + strings.Join(parts, ", ") + ")"
	case orPattern:
		parts := make([]string, len(n.alternatives))
		for i, alternative := range n.alternatives {
			parts[i] = formatPattern(alternative)
		}
		return strings.Join(parts, " | ")
	case andPattern:
		return formatPattern(n.left) + " and " + formatPattern(n.right)
	case notPattern:
		return "not " + formatPattern(n.inner)
	case typePattern:
		if n.binding == "" || n.binding == "_" {
			return n.typeName
		}
		return n.typeName + " " + n.binding
	case relationalPattern:
		return n.op.String() + " " + n.value.String()
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

// quoteString handles the quote string step in the surrounding package and
// returns string.
//
// It returns string so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
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
