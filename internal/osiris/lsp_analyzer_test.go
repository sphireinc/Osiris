package osiris

import (
	"fmt"
	"strings"
	"testing"
)

func TestLSPAnalyzerCoversDeferAssignmentsAndPatterns(t *testing.T) {
	source := `enum Status { Ready, Done }
func classify(value) => match value {
  Status.Ready => 1,
  Status.Done => 2,
  [head, tail] and not null => head,
  number n when n >= 0 => n,
  _ => 0
}
var value = 1
value = 2
defer { emit value }
`
	prog, err := Parse("lsp-analysis.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	a := analyzeOnlyForTask143(t, prog)
	if len(a.diagnostics) != 0 {
		t.Fatalf("valid analyzer input produced diagnostics: %#v", a.diagnostics)
	}

	var assignmentRef, enumPatternRef bool
	for _, ref := range a.refs {
		if ref.detail == "assignment" {
			assignmentRef = true
		}
		if ref.detail == "enum pattern" {
			enumPatternRef = true
		}
	}
	if !assignmentRef {
		t.Fatal("mutable assignment target was not indexed as a reference")
	}
	if !enumPatternRef {
		t.Fatal("enum pattern case was not indexed as a reference")
	}
}

func TestLSPAnalyzerReportsUndefinedAssignmentWithTargetSpan(t *testing.T) {
	source := "missing = 1\n"
	prog, err := Parse("lsp-assignment.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	a := analyzeOnlyForTask143(t, prog)
	if len(a.diagnostics) != 1 {
		t.Fatalf("diagnostics = %#v, want one analyzer diagnostic", a.diagnostics)
	}
	diagnostic := a.diagnostics[0]
	if diagnostic.Message != `undefined assignment target "missing"` {
		t.Fatalf("diagnostic message = %q", diagnostic.Message)
	}
	if diagnostic.Range.Start.Line != 0 || diagnostic.Range.Start.Character != 0 || diagnostic.Range.End.Character != len("missing") {
		t.Fatalf("diagnostic range = %#v, want the assignment target span", diagnostic.Range)
	}
}

func TestLSPAnalyzerReportsUnsupportedNodesAtTheirSourceSpan(t *testing.T) {
	sp := sourceSpan{start: sourcePos{line: 4, col: 3}, end: sourcePos{line: 4, col: 11}}
	a := &documentAnalysis{enumCases: make(map[string]*symbolDef)}
	scope := &lspScope{defs: make(map[string]*symbolDef)}

	if _, err := analyzeStmt(a, scope, "unsupported.osr", task143UnknownStmt{span: sp}, false); err != nil {
		t.Fatal(err)
	}
	if err := analyzeExpr(a, scope, "unsupported.osr", task143UnknownExpr{span: sp}); err != nil {
		t.Fatal(err)
	}
	if len(a.diagnostics) != 2 {
		t.Fatalf("diagnostics = %#v, want statement and expression diagnostics", a.diagnostics)
	}
	for _, diagnostic := range a.diagnostics {
		if diagnostic.Range.Start.Line != 3 || diagnostic.Range.Start.Character != 2 || diagnostic.Range.End.Line != 3 || diagnostic.Range.End.Character != 10 {
			t.Fatalf("diagnostic %q lost source span: %#v", diagnostic.Message, diagnostic.Range)
		}
		if !strings.Contains(diagnostic.Message, "LSP analysis does not support") {
			t.Fatalf("unexpected unsupported-node diagnostic: %q", diagnostic.Message)
		}
	}
}

func analyzeOnlyForTask143(t *testing.T, prog *Program) *documentAnalysis {
	t.Helper()
	a := &documentAnalysis{enumCases: make(map[string]*symbolDef)}
	scope := &lspScope{defs: make(map[string]*symbolDef)}
	for name := range NewRuntime().Builtins {
		scope.defs[name] = &symbolDef{name: name, kind: "builtin", detail: "builtin function"}
	}
	if _, err := analyzeStmtSeq(a, scope, prog.SourceName, prog.Statements, false); err != nil {
		t.Fatal(err)
	}
	return a
}

type task143UnknownStmt struct{ span sourceSpan }

func (task143UnknownStmt) stmtNode() {}

func (n task143UnknownStmt) lspAnalysisSpan() sourceSpan { return n.span }

func (n task143UnknownStmt) String() string { return fmt.Sprintf("unknown statement at %v", n.span) }

type task143UnknownExpr struct{ span sourceSpan }

func (task143UnknownExpr) exprNode() {}

func (n task143UnknownExpr) lspAnalysisSpan() sourceSpan { return n.span }
