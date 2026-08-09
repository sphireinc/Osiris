package osiris

import (
	"context"
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
	a := analyzeOnlyForLSPProtocol(t, prog)
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

func TestLSPEnumPatternTypeNavigationUsesExactSourceSpan(t *testing.T) {
	source := "enum Status { Ready, Done }\n" +
		"func classify(value) => match value { Status.Ready => 1, _ => 0 }\n"
	program, err := Parse("lsp-enum-pattern.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeDocumentSourceContext(context.Background(), NewRuntime(), "file:///tmp/lsp-enum-pattern.osr", source, program)
	var typeRef *symbolRef
	for _, ref := range analysis.refs {
		if ref != nil && ref.detail == "enum pattern type" {
			typeRef = ref
			break
		}
	}
	if typeRef == nil || typeRef.target == nil || typeRef.target.name != "Status" {
		t.Fatalf("enum pattern type reference = %#v, want Status definition", typeRef)
	}
	patternLine := strings.Split(source, "\n")[1]
	patternColumn := strings.Index(patternLine, "Status.Ready") + 1
	if typeRef.span.start.col != patternColumn || typeRef.span.end.col != typeRef.span.start.col+len("Status") {
		t.Fatalf("enum pattern type span = %#v, want exact Status identifier", typeRef.span)
	}
}

func TestLSPAnalyzerReportsUndefinedAssignmentWithTargetSpan(t *testing.T) {
	source := "missing = 1\n"
	prog, err := Parse("lsp-assignment.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	a := analyzeOnlyForLSPProtocol(t, prog)
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

	if _, err := analyzeStmt(a, scope, "unsupported.osr", lspProtocolUnknownStmt{span: sp}, false); err != nil {
		t.Fatal(err)
	}
	if err := analyzeExpr(a, scope, "unsupported.osr", lspProtocolUnknownExpr{span: sp}); err != nil {
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

func analyzeOnlyForLSPProtocol(t *testing.T, prog *Program) *documentAnalysis {
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

type lspProtocolUnknownStmt struct{ span sourceSpan }

func (lspProtocolUnknownStmt) stmtNode() {}

func (n lspProtocolUnknownStmt) lspAnalysisSpan() sourceSpan { return n.span }

func (n lspProtocolUnknownStmt) String() string {
	return fmt.Sprintf("unknown statement at %v", n.span)
}

type lspProtocolUnknownExpr struct{ span sourceSpan }

func (lspProtocolUnknownExpr) exprNode() {}

func (n lspProtocolUnknownExpr) lspAnalysisSpan() sourceSpan { return n.span }
