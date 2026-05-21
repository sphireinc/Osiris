package osiris

import (
	"strings"
	"testing"
)

func TestLanguageServerHoverAndDefinition(t *testing.T) {
	rt := NewRuntime()
	server := NewLanguageServer(rt, ioDiscardWriter{}, ioDiscardWriter{})
	source := `
package demo 1.2.3
import "std/text@1" as text
let answer = 42
emit text.slug(answer)
emit missing
`
	prog, err := Parse("/tmp/demo.osr", source)
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	server.upsertDocument(pathToURI("/tmp/demo.osr"), source)
	doc := server.docs[pathToURI("/tmp/demo.osr")]
	if doc == nil || doc.analysis == nil {
		t.Fatalf("analysis not recorded")
	}
	if got, want := len(doc.diagnostics), 1; got != want {
		t.Fatalf("diagnostics = %d, want %d", got, want)
	}
	letStmt := prog.Statements[2].(letStmt)
	emitStmt := prog.Statements[3].(emitStmt)
	call := emitStmt.Expr.(*callExpr)
	answerUse := call.args[0].(*identExpr)
	hover, err := server.hover(pathToURI("/tmp/demo.osr"), lspPosition{Line: answerUse.Span.start.line - 1, Character: answerUse.Span.start.col - 1})
	if err != nil {
		t.Fatalf("hover failed: %v", err)
	}
	if hover == nil || !strings.Contains(hover.Contents.Value, "answer") || !strings.Contains(hover.Contents.Value, "binding") {
		t.Fatalf("unexpected hover contents: %#v", hover)
	}
	def, err := server.definition(pathToURI("/tmp/demo.osr"), lspPosition{Line: answerUse.Span.start.line - 1, Character: answerUse.Span.start.col - 1})
	if err != nil {
		t.Fatalf("definition failed: %v", err)
	}
	loc, ok := def.(lspLocation)
	if !ok {
		t.Fatalf("definition = %#v, want lspLocation", def)
	}
	if loc.URI != pathToURI("/tmp/demo.osr") {
		t.Fatalf("definition uri = %s, want %s", loc.URI, pathToURI("/tmp/demo.osr"))
	}
	if loc.Range.Start.Line != letStmt.NameSpan.start.line-1 || loc.Range.Start.Character != letStmt.NameSpan.start.col-1 {
		t.Fatalf("definition range = %#v, want start at let binding", loc.Range)
	}
}

type ioDiscardWriter struct{}

func (ioDiscardWriter) Write(p []byte) (int, error) { return len(p), nil }
