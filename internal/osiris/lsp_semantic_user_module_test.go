package osiris

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLSPSemanticUserModuleNavigationSharesIdentityAndUTF16(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "greetings.osr")
	mainPath := filepath.Join(root, "main.osr")
	if err := os.WriteFile(modulePath, []byte("package greetings\nfunc Hello(name string) string => cat(\"Hello \", name)\nexport Hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	mainSource := "package main\nimport \"./greetings.osr\" as greetings\nlet message = cat(\"😀\", greetings.Hello(\"Ada\"))\nemit message\n"
	if err := os.WriteFile(mainPath, []byte(mainSource), 0o644); err != nil {
		t.Fatal(err)
	}

	server := NewLanguageServer(NewRuntime(), nil, nil)
	server.configureWorkspace(initializeParams{RootPath: root})
	server.refreshWorkspace(context.Background())
	mainURI := pathToURI(mainPath)
	if err := server.upsertDocumentContext(context.Background(), mainURI, mainSource, 9); err != nil {
		t.Fatal(err)
	}

	line := "let message = cat(\"😀\", greetings.Hello(\"Ada\"))"
	dotByte := strings.Index(line, "greetings.") + len("greetings.")
	position := lspPosition{Line: 2, Character: utf16Units(line[:dotByte])}
	items, err := server.completion(mainURI, position)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Label != "Hello" || !strings.Contains(items[0].Detail, "func") || items[0].Data == nil || items[0].Data.URI != pathToURI(modulePath) {
		t.Fatalf("user-module completion = %#v, want source-backed Hello", items)
	}

	location, err := server.definition(mainURI, lspPosition{Line: 2, Character: utf16Units(line[:dotByte])})
	if err != nil {
		t.Fatal(err)
	}
	definition, ok := location.(lspLocation)
	if !ok || definition.URI != pathToURI(modulePath) || definition.Range.Start.Line != 1 {
		t.Fatalf("user-module definition = %#v, want greetings.osr line 1", location)
	}

	hover, err := server.hover(mainURI, lspPosition{Line: 2, Character: utf16Units(line[:dotByte])})
	if err != nil {
		t.Fatal(err)
	}
	if hover == nil || !strings.Contains(hover.Contents.Value, "Hello") {
		t.Fatalf("user-module hover = %#v, want Hello", hover)
	}

	references, err := server.references(mainURI, lspPosition{Line: 2, Character: utf16Units(line[:dotByte])})
	if err != nil {
		t.Fatal(err)
	}
	if len(references) < 2 || references[0].URI != pathToURI(modulePath) || references[len(references)-1].URI != pathToURI(mainPath) {
		t.Fatalf("user-module references = %#v, want deterministic module/main locations", references)
	}

	module := server.documentForAnalysis(pathToURI(modulePath))
	main := server.document(mainURI)
	if module == nil || module.analysis == nil || main == nil || main.analysis == nil {
		t.Fatal("expected both module analyses")
	}
	ref, _ := main.analysis.findAt(sourcePos{line: 3, col: dotByte + 1})
	if ref == nil || ref.target == nil {
		t.Fatal("expected imported member reference")
	}
	moduleDef := lspExportedModuleMembers(module)["Hello"]
	if ref.target != moduleDef {
		t.Fatal("imported member did not retain defining-document symbol identity")
	}
}

func TestLSPSemanticUserModuleNavigationRejectsStaleMalformedAndCancelled(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "lib.osr")
	mainPath := filepath.Join(root, "main.osr")
	if err := os.WriteFile(modulePath, []byte("package lib\nfunc Value() u64 => 1u64\nexport Value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	source := "import \"./lib.osr\" as lib\nemit lib.Value()\n"
	if err := os.WriteFile(mainPath, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), nil, nil)
	server.configureWorkspace(initializeParams{RootPath: root})
	server.refreshWorkspace(context.Background())
	uri := pathToURI(mainPath)
	if err := server.upsertDocumentContext(context.Background(), uri, source, 4); err != nil {
		t.Fatal(err)
	}

	_, handled, err := server.handleIDERequest(context.Background(), "textDocument/documentSymbol", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":3}}`))
	if !handled || err == nil || err.(lspRPCError).code != -32602 {
		t.Fatalf("stale user-module request = handled %v err %v, want -32602", handled, err)
	}
	_, handled, err = server.handleIDERequest(context.Background(), "textDocument/documentSymbol", json.RawMessage(`{"textDocument":"malformed"}`))
	if !handled || err == nil || err.(lspRPCError).code != -32602 {
		t.Fatalf("malformed user-module request = handled %v err %v, want -32602", handled, err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, handled, err = server.handleIDERequest(cancelled, "textDocument/documentSymbol", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":4}}`))
	if !handled || err == nil || err.(lspRPCError).code != -32800 {
		t.Fatalf("cancelled user-module request = handled %v err %v, want -32800", handled, err)
	}
}

func TestLSPSemanticUserModuleNavigationFramedFixtureIsDeterministic(t *testing.T) {
	root := t.TempDir()
	modulePath := filepath.Join(root, "lib.osr")
	mainPath := filepath.Join(root, "main.osr")
	if err := os.WriteFile(modulePath, []byte("package lib\nfunc Value() u64 => 1u64\nexport Value\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(mainPath, []byte("import \"./lib.osr\" as lib\nemit lib.Value()\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-semantic-user-modules.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	fixtureText := strings.ReplaceAll(string(fixture), "{{ROOT}}", filepath.ToSlash(root))
	fixtureText = strings.ReplaceAll(fixtureText, "{{MAIN_URI}}", pathToURI(mainPath))
	first := runLSPProtocolFixture(t, []byte(fixtureText))
	second := runLSPProtocolFixture(t, []byte(fixtureText))
	if first != second {
		t.Fatalf("user-module fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawStale, sawMalformed bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2, 3, 4:
			if _, ok := frame["result"]; !ok {
				t.Fatalf("user-module fixture response %d = %#v", id, frame)
			}
		case 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		case 6:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawMalformed = true
		}
	}
	if !sawStale || !sawMalformed {
		t.Fatalf("user-module fixture errors: stale=%v malformed=%v", sawStale, sawMalformed)
	}
}
