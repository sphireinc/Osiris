package osiris

type SourcePosition struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

type SourceRange struct {
	Start SourcePosition `json:"start"`
	End   SourcePosition `json:"end"`
}

type SourceMapEntry struct {
	Index     int         `json:"index"`
	Parent    int         `json:"parent,omitempty"`
	Kind      string      `json:"kind"`
	Statement string      `json:"statement"`
	Range     SourceRange `json:"range"`
}

type SourceMap struct {
	SourceName string           `json:"sourceName"`
	Entries    []SourceMapEntry `json:"entries"`
}

// BuildSourceMap builds the source map representation from its input form,
// carrying structural information forward and returning an error if the source
// cannot be lowered safely.
//
// It returns SourceMap so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func BuildSourceMap(prog *Program) SourceMap {
	sm := SourceMap{SourceName: prog.SourceName}
	var walkStmt func(stmt Stmt, parent int)
	var walkExpr func(expr Expr, parent int)
	nextIndex := 0

	addEntry := func(stmt Stmt, parent int) int {
		entry := SourceMapEntry{
			Index:     nextIndex,
			Parent:    parent,
			Kind:      statementKind(stmt),
			Statement: formatStmt(stmt),
			Range:     toSourceRange(stmtSpan(stmt)),
		}
		sm.Entries = append(sm.Entries, entry)
		nextIndex++
		return entry.Index
	}

	walkStmt = func(stmt Stmt, parent int) {
		idx := addEntry(stmt, parent)
		switch st := stmt.(type) {
		case letStmt:
			walkExpr(st.Expr, idx)
		case assignStmt:
			walkExpr(st.Expr, idx)
		case structStmt, typeDeclStmt, newtypeStmt:
		case emitStmt:
			walkExpr(st.Expr, idx)
		case macroStmt:
			walkExpr(st.Body, idx)
		case funcStmt:
			walkExpr(st.Body, idx)
		case returnStmt:
			if st.Expr != nil {
				walkExpr(st.Expr, idx)
			}
		case whileStmt:
			walkExpr(st.Cond, idx)
			walkExpr(st.Body, idx)
		case forStmt:
			walkExpr(st.Iter, idx)
			walkExpr(st.Body, idx)
		case breakStmt, continueStmt, importStmt, packageStmt, exportStmt:
		}
	}

	walkExpr = func(expr Expr, parent int) {
		switch n := expr.(type) {
		case *blockExpr:
			for _, stmt := range n.stmts {
				walkStmt(stmt, parent)
			}
			if n.tail != nil {
				walkExpr(n.tail, parent)
			}
		case *callExpr:
			walkExpr(n.callee, parent)
			for _, arg := range n.args {
				walkExpr(arg, parent)
			}
		case *memberExpr:
			walkExpr(n.object, parent)
		case *binaryExpr:
			walkExpr(n.left, parent)
			walkExpr(n.right, parent)
		case *unaryExpr:
			walkExpr(n.expr, parent)
		case *listExpr:
			for _, item := range n.items {
				walkExpr(item, parent)
			}
		case *tupleExpr:
			for _, item := range n.items {
				walkExpr(item, parent)
			}
		case *structExpr:
			for _, field := range n.Fields {
				walkExpr(field.Expr, parent)
			}
		case *structUpdateExpr:
			walkExpr(n.Base, parent)
			for _, field := range n.Fields {
				walkExpr(field.Expr, parent)
			}
		case *indexExpr:
			walkExpr(n.Object, parent)
			walkExpr(n.Index, parent)
		case *tryExpr:
			walkExpr(n.body, parent)
			walkExpr(n.handler, parent)
		case *propagateExpr:
			walkExpr(n.expr, parent)
		case *matchExpr:
			walkExpr(n.subject, parent)
			for _, arm := range n.arms {
				walkExpr(arm.body, parent)
			}
		}
	}

	for _, stmt := range prog.Statements {
		walkStmt(stmt, -1)
	}
	return sm
}

// stmtSpan handles the stmt span step in the surrounding package and returns
// sourceSpan.
//
// It returns sourceSpan so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func stmtSpan(stmt Stmt) sourceSpan {
	switch st := stmt.(type) {
	case letStmt:
		return st.Span
	case assignStmt:
		return st.Span
	case structStmt:
		return st.Span
	case typeDeclStmt:
		return st.Span
	case newtypeStmt:
		return st.Span
	case emitStmt:
		return st.Span
	case macroStmt:
		return st.Span
	case funcStmt:
		return st.Span
	case returnStmt:
		return st.Span
	case whileStmt:
		return st.Span
	case forStmt:
		return st.Span
	case breakStmt:
		return st.Span
	case continueStmt:
		return st.Span
	case importStmt:
		return st.Span
	case packageStmt:
		return st.Span
	case exportStmt:
		return st.Span
	case traitStmt:
		return st.Span
	case implStmt:
		return st.Span
	default:
		return sourceSpan{}
	}
}

// toSourceRange handles the to source range step in the surrounding package
// and returns SourceRange.
//
// It returns SourceRange so callers can continue with the next stage of the
// pipeline without re-deriving the same information.
func toSourceRange(sp sourceSpan) SourceRange {
	if sp.start.line == 0 && sp.start.col == 0 && sp.end.line == 0 && sp.end.col == 0 {
		return SourceRange{}
	}
	return SourceRange{
		Start: SourcePosition{Line: sp.start.line, Column: sp.start.col},
		End:   SourcePosition{Line: sp.end.line, Column: sp.end.col},
	}
}
