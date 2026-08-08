package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTask160LSPVersionsCancellationAndTypeHierarchy(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task160-types.osr")
	source := `trait Render { func render() string }
struct User { name string }
impl Render for User { func render() string => "ok" }
emit User
`
	if err := server.upsertDocumentContext(context.Background(), uri, source, 1); err != nil {
		t.Fatal(err)
	}
	if err := server.upsertDocumentContext(context.Background(), uri, source, 1); err == nil || !strings.Contains(err.Error(), "stale document version") {
		t.Fatalf("duplicate version error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	if err := server.upsertDocumentContext(cancelled, uri, source, 2); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled upsert error = %v", err)
	}
	if server.document(uri).version != 1 {
		t.Fatal("cancelled analysis replaced the current document")
	}
	result, handled, err := server.handleIDERequest(cancelled, "textDocument/documentSymbol", json.RawMessage(`{"textDocument":{"uri":"`+uri+`"}}`))
	if !handled || result != nil {
		t.Fatalf("cancelled request result = %#v, handled=%v", result, handled)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled request error = %v", err)
	}

	trait := findLSPType(t, server, "Render")
	subtypes, err := server.typeSubtypes(lspTypeHierarchyItem{Name: "Render", Kind: lspSymbolKind("trait"), Data: "Render"})
	if err != nil || len(subtypes) != 1 || subtypes[0].Name != "User" {
		t.Fatalf("trait subtypes = %#v, err=%v", subtypes, err)
	}
	supertypes, err := server.typeSupertypes(lspTypeHierarchyItem{Name: "User", Kind: lspSymbolKind("struct"), Data: "User"})
	if err != nil || len(supertypes) != 1 || supertypes[0].Name != trait.name || supertypes[0].URI != trait.uri {
		t.Fatalf("User supertypes = %#v, err=%v", supertypes, err)
	}
}

func TestTask160DidSaveRevalidatesOverlayWithoutAdvancingVersion(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task160-save.osr")
	valid := "let answer = 1\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, valid, 7); err != nil {
		t.Fatal(err)
	}
	invalid := "let =\n"
	if err := server.revalidateDocumentContext(context.Background(), uri, &invalid); err != nil {
		t.Fatal(err)
	}
	state := server.document(uri)
	if state == nil || state.text != invalid || state.version != 7 || len(state.diagnostics) == 0 {
		t.Fatalf("saved overlay = %#v, want invalid text, version 7, and diagnostics", state)
	}
	if err := server.upsertDocumentContext(context.Background(), uri, valid, 8); err != nil {
		t.Fatalf("next didChange version was rejected after didSave: %v", err)
	}
}

func TestTask160DidSaveProtocolNotificationRevalidatesAndDoesNotRespond(t *testing.T) {
	fixture := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///tmp/task160-save-protocol.osr","languageId":"osiris","version":7,"text":"let answer = 1\nemit answer\n"}}}
{"jsonrpc":"2.0","method":"textDocument/didSave","params":{"textDocument":{"uri":"file:///tmp/task160-save-protocol.osr"},"text":"let =\n"}}
{"jsonrpc":"2.0","id":2,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`)
	frames := decodeTask143Frames(t, runTask143ProtocolFixture(t, fixture))
	if got := countTask143Notifications(frames, "textDocument/publishDiagnostics"); got != 2 {
		t.Fatalf("publishDiagnostics notifications = %d, want 2", got)
	}
	for _, frame := range frames {
		if method, ok := frame["method"].(string); ok && method == "textDocument/didSave" {
			t.Fatalf("didSave notification produced a protocol frame: %#v", frame)
		}
	}
	var sawSavedDiagnostic bool
	for _, frame := range frames {
		if frame["method"] != "textDocument/publishDiagnostics" {
			continue
		}
		params, ok := frame["params"].(map[string]any)
		if !ok {
			continue
		}
		if diagnostics, ok := params["diagnostics"].([]any); ok && len(diagnostics) > 0 {
			sawSavedDiagnostic = true
		}
	}
	if !sawSavedDiagnostic {
		t.Fatal("didSave text was not revalidated into a diagnostic publication")
	}
}

func TestTask161VersionedWorkspaceEditsAndColocatedInventory(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "osiris.mod"), []byte("module example.com/acme/test\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "greetings.osr")
	testPath := filepath.Join(dir, "greetings.test.osr")
	if err := os.WriteFile(sourcePath, []byte("package greetings 1.0.0\nlet answer = 1\nemit answer\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testPath, []byte("// osiris:test name=smoke\nemit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{WorkspaceFolders: []struct {
		URI  string `json:"uri"`
		Name string `json:"name"`
	}{{URI: pathToURI(dir), Name: "workspace"}}})
	server.refreshWorkspace(context.Background())
	uri := pathToURI(sourcePath)
	server.upsertDocument(uri, "package greetings 1.0.0\nlet answer = 1\nemit answer\n")
	version := 0
	edit, err := server.renameWorkspaceEditAtVersion(uri, lspPosition{Line: 1, Character: 5}, "result", &version)
	if err != nil || len(edit.DocumentChanges) != 1 || edit.DocumentChanges[0].TextDocument.Version != 0 {
		t.Fatalf("versioned rename = %#v, err=%v", edit, err)
	}
	if _, err := server.renameWorkspaceEditAtVersion(uri, lspPosition{Line: 1, Character: 5}, "result", intPointer(1)); err == nil {
		t.Fatal("stale rename request was accepted")
	}
	inventory, err := server.testInventory(uri)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Module != "example.com/acme/test" || inventory.Package != "greetings" || len(inventory.Files) != 1 || inventory.Files[0] != testPath || len(inventory.Tests) != 1 || inventory.Tests[0].Name != "smoke" {
		t.Fatalf("colocated inventory = %#v", inventory)
	}
	if _, err := server.formattingAtVersion(uri, intPointer(1)); err == nil {
		t.Fatal("stale formatting request was accepted")
	}
}

func TestTask161OrganizeImportsProducesMinimalVersionedEdits(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task161-imports.osr")
	source := "import \"std/time\" as time\nimport \"std/math\" as math\nlet answer = 1\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 4); err != nil {
		t.Fatal(err)
	}
	selection := lspRange{Start: lspPosition{Line: 0, Character: 0}, End: lspPosition{Line: 1, Character: 0}}
	actions, err := server.codeActionsAtVersion(uri, selection, nil, intPointer(4))
	if err != nil {
		t.Fatal(err)
	}
	if len(actions) != 1 || actions[0].Kind != "source.organizeImports" {
		t.Fatalf("organize-import actions = %#v", actions)
	}
	edits := actions[0].Edit.Changes[uri]
	if len(edits) != 2 {
		t.Fatalf("organize-import edits = %#v, want two import-only edits", edits)
	}
	if edits[0].Range.Start.Line != 1 || edits[0].NewText != "import \"std/time\" as time" || edits[1].Range.Start.Line != 0 || edits[1].NewText != "import \"std/math\" as math" {
		t.Fatalf("organize-import edits = %#v, want descending minimal edits", edits)
	}
	if len(actions[0].Edit.DocumentChanges) != 1 || actions[0].Edit.DocumentChanges[0].TextDocument.Version != 4 {
		t.Fatalf("versioned organize-import edit = %#v", actions[0].Edit.DocumentChanges)
	}
	if _, err := server.codeActionsAtVersion(uri, selection, nil, intPointer(5)); err == nil {
		t.Fatal("stale organize-import request was accepted")
	}
}

func TestTask168StableLSPCellsRequireFixtureEvidence(t *testing.T) {
	root, matrix := loadSupportMatrix(t)
	for _, feature := range matrix.Features {
		if feature.Support["lsp"] != "stable" {
			continue
		}
		if len(feature.Documentation) == 0 {
			t.Fatalf("stable LSP feature %q has no documentation route", feature.ID)
		}
		foundFixture := false
		for _, evidence := range feature.Evidence.Tests {
			parts := strings.SplitN(evidence, "#", 2)
			if len(parts) != 2 || !strings.Contains(parts[0], "lsp") {
				continue
			}
			data, err := os.ReadFile(filepath.Join(root, parts[0]))
			if err == nil && strings.Contains(string(data), "func "+parts[1]+"(") {
				foundFixture = true
				break
			}
		}
		if !foundFixture {
			t.Fatalf("stable LSP feature %q has no LSP fixture evidence", feature.ID)
		}
	}
}

func TestTask160And161ProtocolFixturesAreDeterministic(t *testing.T) {
	for _, name := range []string{"task160-semantic.jsonl", "task161-workspace.jsonl", "task161-import-actions.jsonl"} {
		fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", name))
		if err != nil {
			t.Fatal(err)
		}
		first := runTask143ProtocolFixture(t, fixture)
		second := runTask143ProtocolFixture(t, fixture)
		if first != second {
			t.Fatalf("fixture %s is not deterministic", name)
		}
		frames := decodeTask143Frames(t, first)
		if len(frames) == 0 {
			t.Fatalf("fixture %s produced no responses", name)
		}
		for _, frame := range frames {
			if _, ok := frame["id"]; !ok {
				continue
			}
			if _, ok := frame["error"]; ok {
				continue
			}
			if _, ok := frame["result"]; !ok {
				t.Fatalf("fixture %s response has neither result nor error: %#v", name, frame)
			}
		}
	}
}

func findLSPType(t *testing.T, server *languageServer, name string) *symbolDef {
	t.Helper()
	def := server.typeDefinitionByName(name)
	if def == nil {
		t.Fatalf("type %q was not indexed", name)
	}
	return def
}

func intPointer(value int) *int { return &value }
