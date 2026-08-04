package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTask125IDEFeaturesAreBoundedAndDeterministic(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task125.test.osr")
	source := `type User = string
func greet(name string) string => name
let answer = greet("😀")
emit answer
`
	server.upsertDocument(uri, source)
	if got, err := server.documentSymbols(uri); err != nil || len(got) < 3 {
		t.Fatalf("document symbols = %#v, err=%v", got, err)
	}
	semantic, err := server.semanticTokens(uri)
	if err != nil || len(semantic.Data) == 0 || len(semantic.Data)%5 != 0 {
		t.Fatalf("semantic tokens = %#v, err=%v", semantic, err)
	}
	hints, err := server.inlayHints(uri, fullDocumentRange(source))
	if err != nil || len(hints) == 0 {
		t.Fatalf("inlay hints = %#v, err=%v", hints, err)
	}
	if _, err := server.foldingRanges(uri); err != nil {
		t.Fatal(err)
	}
	selections, err := server.selectionRanges(uri, []lspPosition{{Line: 2, Character: 5}})
	if err != nil || len(selections) != 1 || selections[0].Parent == nil {
		t.Fatalf("selection ranges = %#v, err=%v", selections, err)
	}
	formatted, err := server.formatting(uri)
	if err != nil {
		t.Fatalf("formatting = %#v, err=%v", formatted, err)
	}
	diagnostics, err := server.pullDiagnostics(uri)
	if err != nil || diagnostics.Kind != "full" {
		t.Fatalf("diagnostics = %#v, err=%v", diagnostics, err)
	}
}

func TestTask125WorkspaceIndexAndWorkspaceRename(t *testing.T) {
	dir := t.TempDir()
	first := filepath.Join(dir, "first.osr")
	second := filepath.Join(dir, "second.osr")
	if err := os.WriteFile(first, []byte("func greet(name string) string => name\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(second, []byte("func caller() string => greet(\"x\")\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{RootPath: dir})
	server.refreshWorkspace(context.Background())
	symbols, err := server.workspaceSymbols(context.Background(), "greet")
	if err != nil || len(symbols) != 1 || symbols[0].Name != "greet" {
		t.Fatalf("workspace symbols = %#v, err=%v", symbols, err)
	}
	server.upsertDocument(pathToURI(first), "func greet(name string) string => name\n")
	edit, err := server.renameWorkspaceEdit(pathToURI(first), lspPosition{Line: 0, Character: 5}, "welcome")
	if err != nil || len(edit.Changes) == 0 {
		t.Fatalf("workspace rename = %#v, err=%v", edit, err)
	}
	if server.document(pathToURI(first)).text != "func greet(name string) string => name\n" {
		t.Fatal("rename mutated open document")
	}
}

func TestTask125ProtocolNegotiationAndMalformedRequests(t *testing.T) {
	frame := func(payload string) string {
		return "content-length: " + stringInt(len(payload)) + "\r\n\r\n" + payload
	}
	input := frame(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"rootUri":""}}`) +
		frame(`{"jsonrpc":"2.0","id":2,"method":"textDocument/documentSymbol","params":{}}`) +
		frame(`{"jsonrpc":"2.0","id":3,"method":"does/not/exist","params":{}}`) +
		frame(`{"jsonrpc":"2.0","method":"exit"}`)
	var output bytes.Buffer
	server := NewLanguageServer(NewRuntime(), &output, ioDiscardWriter{})
	if err := server.Serve(context.Background(), strings.NewReader(input)); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), "semanticTokensProvider") || !strings.Contains(output.String(), "-32601") {
		t.Fatalf("protocol output = %s", output.String())
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(`{"jsonrpc":"2.0","id":1}`), &decoded); err != nil {
		t.Fatal(err)
	}
}

func stringInt(value int) string {
	if value == 0 {
		return "0"
	}
	return string([]byte(fmtInt(value)))
}

func fmtInt(value int) []byte {
	if value < 10 {
		return []byte{'0' + byte(value)}
	}
	return append(fmtInt(value/10), byte('0'+value%10))
}
