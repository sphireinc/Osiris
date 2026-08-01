package osiris

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"strconv"
	"strings"
	"testing"
)

// TestLanguageServerHoverAndDefinition exercises the behavior described by its
// body and locks in the observable language, runtime, or CLI semantics that
// the surrounding package depends on.
//
// It does not return a value. Success or failure is communicated through side
// effects or through a separate error path when one exists.
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

func TestLanguageServerRenameRespectsLexicalScopeAndFormatsDryRun(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/rename.osr")
	source := `let answer=42
func show(answer) => answer
emit answer
`
	server.upsertDocument(uri, source)
	// The final answer reference resolves to the outer binding, not the
	// same-spelled function parameter.
	rewritten, err := server.RenameDocument(uri, lspPosition{Line: 2, Character: 5}, "result")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "let result = 42") || !strings.Contains(rewritten, "emit result") {
		t.Fatalf("outer rename missing from rewrite:\n%s", rewritten)
	}
	if !strings.Contains(rewritten, "func show(answer) => answer") {
		t.Fatalf("shadowed parameter was renamed:\n%s", rewritten)
	}
	if server.docs[uri].text != source {
		t.Fatal("dry-run rename mutated open document")
	}
	if _, err := server.RenameDocument(uri, lspPosition{Line: 2, Character: 5}, "123bad"); err == nil {
		t.Fatal("invalid identifier accepted")
	}
}

func TestLanguageServerRenameIncludesImportAliasReferences(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/import-rename.osr")
	source := "import \"std/text\" as text\nemit text.slug(\"Osiris\")\n"
	server.upsertDocument(uri, source)
	rewritten, err := server.RenameDocument(uri, lspPosition{Line: 1, Character: 6}, "words")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rewritten, "as words") || !strings.Contains(rewritten, "words.slug") {
		t.Fatalf("import alias rename missing:\n%s", rewritten)
	}
}

func TestLanguageServerRecognizesEnumDeclaration(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/enum.osr")
	source := "enum Status { Ready, Done }\nemit Status.Ready\n"
	server.upsertDocument(uri, source)
	doc := server.docs[uri]
	if doc == nil || doc.analysis == nil {
		t.Fatal("enum analysis not recorded")
	}
	if len(doc.diagnostics) != 0 {
		t.Fatalf("enum diagnostics = %#v", doc.diagnostics)
	}
	hover, err := server.hover(uri, lspPosition{Line: 1, Character: 6})
	if err != nil || hover == nil || !strings.Contains(hover.Contents.Value, "enum") {
		t.Fatalf("enum hover = %#v, %v", hover, err)
	}
	caseHover, err := server.hover(uri, lspPosition{Line: 1, Character: 12})
	if err != nil || caseHover == nil || !strings.Contains(caseHover.Contents.Value, "Status.Ready") {
		t.Fatalf("enum case hover = %#v, %v", caseHover, err)
	}
	def, err := server.definition(uri, lspPosition{Line: 1, Character: 12})
	if err != nil {
		t.Fatal(err)
	}
	loc, ok := def.(lspLocation)
	if !ok || loc.Range.Start.Line != 0 {
		t.Fatalf("enum case definition = %#v", def)
	}
}

func TestLanguageServerUsesUTF16Positions(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/unicode.osr")
	// The astral character occupies two UTF-16 code units before missing.
	text := "emit \"😀\" + missing\n"
	server.upsertDocument(uri, text)
	pos := toSourcePosInText(text, lspPosition{Line: 0, Character: 12})
	if pos.line != 1 || pos.col != 15 {
		t.Fatalf("UTF-16 position mapped to %#v, want line 1 byte column 15", pos)
	}
	doc := server.docs[uri]
	if got, want := doc.diagnostics[0].Range.Start.Character, 12; got != want {
		t.Fatalf("UTF-16 diagnostic start = %d, want %d", got, want)
	}
}

func TestLSPIncrementalChangesUseUTF16Ranges(t *testing.T) {
	text := "emit 😀old\n"
	rng := &lspRange{Start: lspPosition{Line: 0, Character: 7}, End: lspPosition{Line: 0, Character: 10}}
	got, err := applyLSPChange(text, rng, "new")
	if err != nil || got != "emit 😀new\n" {
		t.Fatalf("incremental change = %q, %v", got, err)
	}
	invalid := &lspRange{Start: lspPosition{Line: 0, Character: 6}, End: lspPosition{Line: 0, Character: 7}}
	if _, err := applyLSPChange(text, invalid, "x"); err == nil {
		t.Fatal("half-surrogate UTF-16 range unexpectedly accepted")
	}
}

func TestLanguageServerHonorsCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	if err := server.Serve(ctx, strings.NewReader("")); !errors.Is(err, context.Canceled) {
		t.Fatalf("Serve cancellation error = %v", err)
	}
}

func TestLanguageServerKeepsMalformedPartialDocument(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/partial.osr")
	server.upsertDocument(uri, "func unfinished(")
	doc := server.docs[uri]
	if doc == nil || doc.text != "func unfinished(" {
		t.Fatalf("partial document was not retained: %#v", doc)
	}
	if len(doc.diagnostics) != 1 || doc.diagnostics[0].Message == "" {
		t.Fatalf("partial document diagnostics = %#v", doc.diagnostics)
	}
	server.upsertDocument(uri, "let answer = 42\nemit answer\n")
	doc = server.docs[uri]
	if doc.analysis == nil || len(doc.diagnostics) != 0 {
		t.Fatalf("valid replacement did not recover: %#v", doc)
	}
}

func TestLanguageServerAnalyzesModulesMacrosAndGeneratedSpans(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/features.osr")
	source := "import \"std/text\" as text\nmacro shout(x) => text.upper(x)\nemit shout(\"hi\")\nemit abs(1)\n"
	server.upsertDocument(uri, source)
	doc := server.docs[uri]
	if doc.analysis == nil || len(doc.diagnostics) != 0 {
		t.Fatalf("module/macro analysis = %#v", doc)
	}
	hover, err := server.hover(uri, lspPosition{Line: 2, Character: 5})
	if err != nil || hover == nil || !strings.Contains(hover.Contents.Value, "macro") {
		t.Fatalf("macro hover = %#v, %v", hover, err)
	}
	// Builtins carry generated/no-source definitions. They must return no
	// location rather than an invalid zero-span location.
	def, err := server.definition(uri, lspPosition{Line: 3, Character: 5})
	if err != nil || def != nil {
		t.Fatalf("generated builtin definition = %#v, %v", def, err)
	}
}

func TestLanguageServerFramedInitializeFixture(t *testing.T) {
	frame := func(payload string) string {
		return "Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + payload
	}
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`) + frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) + frame(`{"jsonrpc":"2.0","method":"exit"}`)
	var output bytes.Buffer
	server := NewLanguageServer(NewRuntime(), &output, ioDiscardWriter{})
	if err := server.Serve(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	raw := output.String()
	if !strings.Contains(raw, "\"change\":2") || !strings.Contains(raw, "hoverProvider") || !strings.Contains(raw, "definitionProvider") || !strings.Contains(raw, "completionProvider") || !strings.Contains(raw, "signatureHelpProvider") || !strings.Contains(raw, "referencesProvider") {
		t.Fatalf("capabilities response = %s", raw)
	}
}

func TestLanguageServerCompletionReferencesAndSignatureHelp(t *testing.T) {
	uri := pathToURI("/tmp/features.osr")
	source := "func greet(name string) string => name\nlet value = greet(\"x\")\nemit value\n"
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.upsertDocument(uri, source)
	items, err := server.completion(uri, lspPosition{Line: 1, Character: 4})
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	for _, item := range items {
		if item.Label == "greet" {
			found = true
		}
	}
	if !found {
		t.Fatalf("completion items %v do not contain greet", items)
	}
	refs, err := server.references(uri, lspPosition{Line: 1, Character: 12})
	if err != nil || len(refs) < 2 {
		t.Fatalf("references = %#v, %v", refs, err)
	}
	help, err := server.signatureHelp(uri, lspPosition{Line: 1, Character: 17})
	if err != nil || help == nil || len(help.Signatures) != 1 || !strings.Contains(help.Signatures[0].Label, "greet") {
		t.Fatalf("signature help = %#v, %v", help, err)
	}
}

func TestLanguageServerInvalidIncrementalChangePreservesDocument(t *testing.T) {
	frame := func(payload string) string {
		return "Content-Length: " + strconv.Itoa(len(payload)) + "\r\n\r\n" + payload
	}
	uri := pathToURI("/tmp/change.osr")
	input := frame(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"`+uri+`","text":"emit 😀old\n"}}}`) +
		frame(`{"jsonrpc":"2.0","method":"textDocument/didChange","params":{"textDocument":{"uri":"`+uri+`"},"contentChanges":[{"range":{"start":{"line":0,"character":6},"end":{"line":0,"character":7}},"text":"x"}]}}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	var log bytes.Buffer
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, &log)
	if err := server.Serve(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatalf("Serve returned after invalid notification: %v", err)
	}
	if got := server.docs[uri].text; got != "emit 😀old\n" {
		t.Fatalf("invalid didChange corrupted document: %q", got)
	}
	if !strings.Contains(log.String(), "ignored invalid didChange") {
		t.Fatalf("invalid change was not logged: %q", log.String())
	}
}

func TestReadLSPMessageRejectsOversizedPayload(t *testing.T) {
	reader := bufio.NewReader(strings.NewReader("Content-Length: " + strconv.Itoa(maxLSPMessageBytes+1) + "\r\n\r\n"))
	if _, err := readLSPMessage(reader); err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("oversized LSP message error = %v", err)
	}
}

type ioDiscardWriter struct{}

// Write operates on ioDiscardWriter values and keeps the
// iodiscardwriter-specific behavior attached to the receiver so callers can
// use the method as the natural entry point for that data structure.
//
// It returns int and an error. Callers should only trust the value when the
// error is nil.
func (ioDiscardWriter) Write(p []byte) (int, error) { return len(p), nil }
