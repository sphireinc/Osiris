package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestTask161SemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task161-semantic-tokens.osr")
	source := "let name = \"😀\" + 42\nemit name // comment\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 3); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":3}}`)
	first, handled, err := server.handleIDERequest(context.Background(), "textDocument/semanticTokens/full", params)
	if err != nil || !handled {
		t.Fatalf("semantic-token request = %#v, handled=%v, err=%v", first, handled, err)
	}
	second, _, err := server.handleIDERequest(context.Background(), "textDocument/semanticTokens/full", params)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("semantic-token response is nondeterministic: %#v != %#v", first, second)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"data":[0,0,3,13,0,0,4,4,8,0,0,5,1,18,0,0,2,4,16,0,0,5,1,18,0,0,2,2,17,0,1,0,4,13,0,0,5,4,8,0,0,5,10,15,0]}`
	if string(wire) != want {
		t.Fatalf("semantic-token wire response = %s, want %s", wire, want)
	}
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/semanticTokens/full", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":2}}`)); err == nil {
		t.Fatal("stale semantic-token request was accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(cancelled, "textDocument/semanticTokens/full", params)
	if result != nil || !handled {
		t.Fatalf("cancelled semantic-token result = %#v, handled=%v", result, handled)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled semantic-token error = %v", err)
	}
}

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

func TestTask160StdlibExportsNavigateWithUTF16AndIdentity(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/task160-stdlib.osr")
	source := "import \"std/text\" as text\n" +
		"let prefix = \"😀\"\n" +
		"let first = text.trim(\" x \")\n" +
		"let second = text.trim(prefix)\n" +
		"emit second\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}
	callLine := "let first = text.trim(\" x \")"
	member := strings.Index(callLine, "trim")
	position := lspPosition{Line: 2, Character: utf16Units(callLine[:member]) + 1}
	items, err := server.completion(uri, lspPosition{Line: 2, Character: utf16Units("let first = text.")})
	if err != nil {
		t.Fatal(err)
	}
	var foundTrim bool
	for _, item := range items {
		if item.Label == "trim" && item.Kind == 3 && strings.Contains(item.Detail, "std/text.trim") {
			foundTrim = true
		}
	}
	if !foundTrim {
		t.Fatalf("stdlib completion did not include proven trim export: %#v", items)
	}
	hover, err := server.hover(uri, position)
	if err != nil || hover == nil || !strings.Contains(hover.Contents.Value, "std/text.trim") {
		t.Fatalf("stdlib hover = %#v, %v", hover, err)
	}
	definition, err := server.definition(uri, position)
	if err != nil {
		t.Fatal(err)
	}
	location, ok := definition.(lspLocation)
	if !ok || location.URI != "osiris://stdlib/text" || location.Range == (lspRange{}) {
		t.Fatalf("stdlib definition = %#v, want virtual std/text export location", definition)
	}
	references, err := server.references(uri, position)
	if err != nil || len(references) != 2 {
		t.Fatalf("stdlib references = %#v, %v; want both calls", references, err)
	}
	if got := toSourcePosInText(source, lspPosition{Line: 1, Character: utf16Units("let prefix = ") + 1}); got.col <= len("let prefix = ") {
		t.Fatalf("UTF-16 source conversion did not preserve astral code point boundary: %#v", got)
	}
}

func TestTask160StdlibExportsFramedUTF16AndStale(t *testing.T) {
	uri := pathToURI("/tmp/task160-stdlib-framed.osr")
	source := "import \"std/text\" as text\n" +
		"let prefix = \"😀\"\n" +
		"let value = text.trim(prefix)\n" +
		"emit value\n"
	encodedSource, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	line := "let value = text.trim(prefix)"
	member := strings.Index(line, "trim")
	position := fmt.Sprintf(`{"line":2,"character":%d}`, utf16Units(line[:member])+1)
	fixture := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":7,"text":%s}}}`, uri, encodedSource) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/completion","params":{"textDocument":{"uri":%q,"version":7},"position":{"line":2,"character":%d}}}`, uri, utf16Units("let value = text.")+1) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"textDocument/references","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":6},"position":%s}}`, uri, position) + "\n" +
		`{"jsonrpc":"2.0","id":7,"method":"shutdown","params":{}}` + "\n" +
		`{"jsonrpc":"2.0","method":"exit"}`
	frames := decodeTask143Frames(t, runTask143ProtocolFixture(t, []byte(fixture)))
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := task143FrameID(frame)
		if !ok {
			continue
		}
		seen[id] = true
		if id == 6 {
			task143AssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{2, 3, 4, 5, 6, 7} {
		if !seen[id] {
			t.Fatalf("stdlib framed request %d did not receive a response: %#v", id, frames)
		}
	}
	var completions []lspCompletionItem
	for _, frame := range frames {
		if id, _ := task143FrameID(frame); id != 2 {
			continue
		}
		data, err := json.Marshal(frame["result"])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(data, &completions); err != nil {
			t.Fatal(err)
		}
	}
	var foundTrim bool
	for _, item := range completions {
		if item.Label == "trim" && strings.Contains(item.Detail, "std/text.trim") {
			foundTrim = true
		}
	}
	if !foundTrim {
		t.Fatalf("stdlib framed completion = %#v", completions)
	}
}

func TestTask160StdlibExportsFramedCancellation(t *testing.T) {
	uri := pathToURI("/tmp/task160-stdlib-cancel.osr")
	openText, err := json.Marshal("import \"std/text\" as text\nlet value = text.trim(\"x\")\n")
	if err != nil {
		t.Fatal(err)
	}
	frames := []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":1,"text":%s}}}`, uri, openText),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":1},"position":{"line":1,"character":22}}}`, uri),
	}
	var inputChunks [][]byte
	for _, payload := range frames {
		inputChunks = append(inputChunks, []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: inputChunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server := NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled framed Serve error = %v", err)
	}
	responses := decodeTask143Frames(t, output.String())
	var sawCancelled bool
	for _, frame := range responses {
		if id, ok := task143FrameID(frame); ok && id == 2 {
			task143AssertErrorCode(t, frame, -32800)
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Fatalf("cancelled framed request produced no cancellation response: %#v", responses)
	}
}

type cancelAfterLSPRead struct {
	chunks      [][]byte
	cancel      context.CancelFunc
	cancelAfter int
	reads       int
}

func (r *cancelAfterLSPRead) Read(p []byte) (int, error) {
	if len(r.chunks) == 0 {
		return 0, io.EOF
	}
	if r.reads == r.cancelAfter {
		r.cancel()
	}
	r.reads++
	chunk := r.chunks[0]
	r.chunks = r.chunks[1:]
	n := copy(p, chunk)
	if n != len(chunk) {
		panic("cancelAfterLSPRead buffer too small")
	}
	return n, nil
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

func TestTask161ColocatedInventoryDrivesCodeLensesAndUsesOverlay(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "osiris.mod"), []byte("module example.com/acme/lsp\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sourcePath := filepath.Join(dir, "greetings.osr")
	testPath := filepath.Join(dir, "greetings.test.osr")
	if err := os.WriteFile(sourcePath, []byte("package greetings 1.0.0\nfunc greet() string => \"hi\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(testPath, []byte("// osiris:test name=from-disk\nemit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{RootPath: dir})
	server.refreshWorkspace(context.Background())
	sourceURI, testURI := pathToURI(sourcePath), pathToURI(testPath)
	if err := server.upsertDocumentContext(context.Background(), sourceURI, "package greetings 1.0.0\nfunc greet() string => \"hi\"\n", 7); err != nil {
		t.Fatal(err)
	}
	if err := server.upsertDocumentContext(context.Background(), testURI, "// osiris:test name=overlay\n// osiris:benchmark warmup=0 samples=1\nemit 1\n", 3); err != nil {
		t.Fatal(err)
	}
	inventory, err := server.testInventoryContext(context.Background(), sourceURI)
	if err != nil {
		t.Fatal(err)
	}
	if inventory.Module != "example.com/acme/lsp" || len(inventory.Files) != 1 || inventory.Files[0] != testPath || len(inventory.Tests) != 1 || inventory.Tests[0].Name != "overlay" || !inventory.Tests[0].Benchmark {
		t.Fatalf("colocated overlay inventory = %#v", inventory)
	}
	lenses, err := server.codeLensesContext(context.Background(), sourceURI)
	if err != nil || len(lenses) != 1 || lenses[0].Command == nil || lenses[0].Command.Command != "osiris.benchmark" || lenses[0].Command.Arguments[0] != testPath {
		t.Fatalf("colocated code lenses = %#v, err=%v", lenses, err)
	}
}

func TestTask161TestInventoryHonorsCancellationAndSymlinkRootBoundary(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	sourcePath := filepath.Join(root, "main.osr")
	companionPath := filepath.Join(root, "main.test.osr")
	outsidePath := filepath.Join(outside, "outside.test.osr")
	if err := os.WriteFile(sourcePath, []byte("emit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(outsidePath, []byte("// osiris:test name=escaped\nemit 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, companionPath); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	server.configureWorkspace(initializeParams{RootPath: root})
	server.upsertDocument(pathToURI(sourcePath), "emit 1\n")
	if _, err := server.testInventoryContext(context.Background(), pathToURI(sourcePath)); err == nil || !strings.Contains(err.Error(), "outside configured workspace roots") {
		t.Fatalf("symlink escape error = %v", err)
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := server.testInventoryContext(cancelled, pathToURI(sourcePath))
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled inventory error = %v", err)
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
	for _, name := range []string{"task160-semantic.jsonl", "task161-workspace.jsonl", "task161-import-actions.jsonl", "task161-test-inventory.jsonl", "task161-semantic-tokens.jsonl"} {
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
