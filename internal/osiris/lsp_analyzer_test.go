package osiris

import (
	"context"
	"encoding/json"
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

func TestLSPAnalyzerPreservesBindingMutabilityAndRejectsImmutableAssignments(t *testing.T) {
	source := `const base = 2 + 3
let runtime = base + 1
var current = runtime
current = current + base
runtime = 9
base = 10
emit current
`
	prog, err := Parse("lsp-binding-contracts.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	a := analyzeOnlyForLSPProtocol(t, prog)
	if len(a.diagnostics) != 2 {
		t.Fatalf("immutable assignments produced diagnostics = %#v, want two", a.diagnostics)
	}
	for _, name := range []string{"runtime", "base"} {
		found := false
		for _, diagnostic := range a.diagnostics {
			if strings.Contains(diagnostic.Message, fmt.Sprintf("immutable binding %q", name)) {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing immutable assignment diagnostic for %q: %#v", name, a.diagnostics)
		}
	}
	want := map[string]string{"base": "constant binding", "runtime": "immutable binding", "current": "mutable binding"}
	for _, def := range a.defs {
		if detail, ok := want[def.name]; ok && def.detail != detail {
			t.Fatalf("binding %q detail = %q, want %q", def.name, def.detail, detail)
		}
	}
}

func TestLSPBindingContractsProtocolIsDeterministicAndReportsImmutableAssignments(t *testing.T) {
	uri := "file:///tmp/lsp-binding-contracts.osr"
	source := "const base = 2 + 3\n" +
		"let runtime = base + 1\n" +
		"var current = runtime\n" +
		"current = current + base\n" +
		"runtime = 9\n" +
		"base = 10\n"
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	fixture := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":3,"text":%s}}}
{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":3},"position":{"line":0,"character":6}}}
{"jsonrpc":"2.0","id":3,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":3},"position":{"line":1,"character":5}}}
{"jsonrpc":"2.0","id":4,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":3},"position":{"line":2,"character":5}}}
{"jsonrpc":"2.0","id":5,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`,
		uri, encoded, uri, uri, uri)
	first := runLSPProtocolFixture(t, []byte(fixture))
	second := runLSPProtocolFixture(t, []byte(fixture))
	if first != second {
		t.Fatalf("binding protocol output is not deterministic:\nfirst=%s\nsecond=%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	wantDetails := map[int]string{2: "constant binding", 3: "immutable binding", 4: "mutable binding"}
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		if detail, wanted := wantDetails[id]; wanted {
			var hover lspHover
			payload, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(payload, &hover); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(hover.Contents.Value, detail) {
				t.Fatalf("hover %d = %q, want %q", id, hover.Contents.Value, detail)
			}
			seen[id] = true
		}
	}
	for id := range wantDetails {
		if !seen[id] {
			t.Fatalf("missing binding hover response %d: %#v", id, frames)
		}
	}
	var diagnosticMessages []string
	for _, frame := range frames {
		if frame["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params, ok := frame["params"].(map[string]any)
		if !ok {
			t.Fatalf("diagnostics params = %#v", frame["params"])
		}
		items, ok := params["diagnostics"].([]any)
		if !ok {
			t.Fatalf("diagnostics items = %#v", params["diagnostics"])
		}
		for _, item := range items {
			entry, ok := item.(map[string]any)
			if ok {
				if message, ok := entry["message"].(string); ok {
					diagnosticMessages = append(diagnosticMessages, message)
				}
			}
		}
	}
	joinedDiagnostics := strings.Join(diagnosticMessages, "\n")
	if len(diagnosticMessages) < 2 || !strings.Contains(joinedDiagnostics, `immutable binding "runtime"`) || !strings.Contains(joinedDiagnostics, `immutable binding "base"`) {
		t.Fatalf("binding diagnostics = %#v", diagnosticMessages)
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
