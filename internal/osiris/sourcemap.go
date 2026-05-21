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
		case emitStmt:
			walkExpr(st.Expr, idx)
		case macroStmt:
			walkExpr(st.Body, idx)
		case importStmt, packageStmt, exportStmt:
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

func stmtSpan(stmt Stmt) sourceSpan {
	switch st := stmt.(type) {
	case letStmt:
		return st.Span
	case emitStmt:
		return st.Span
	case macroStmt:
		return st.Span
	case importStmt:
		return st.Span
	case packageStmt:
		return st.Span
	case exportStmt:
		return st.Span
	default:
		return sourceSpan{}
	}
}

func toSourceRange(sp sourceSpan) SourceRange {
	if sp.start.line == 0 && sp.start.col == 0 && sp.end.line == 0 && sp.end.col == 0 {
		return SourceRange{}
	}
	return SourceRange{
		Start: SourcePosition{Line: sp.start.line, Column: sp.start.col},
		End:   SourcePosition{Line: sp.end.line, Column: sp.end.col},
	}
}
