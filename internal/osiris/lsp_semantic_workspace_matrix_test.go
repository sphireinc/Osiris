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
	"sort"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestLSPWorkspaceSemanticTokensHaveExactShapeUTF16ClassificationAndDeterminism(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-semantic-tokens.osr")
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
	for name, raw := range map[string]string{
		"missing params":   `{}`,
		"missing uri":      `{"textDocument":{}}`,
		"stale version":    `{"textDocument":{"uri":"` + uri + `","version":2}}`,
		"unknown document": `{"textDocument":{"uri":"file:///tmp/not-open.osr"}}`,
	} {
		_, handled, err := server.handleIDERequest(context.Background(), "textDocument/semanticTokens/full", json.RawMessage(raw))
		if !handled {
			t.Fatalf("%s request was not handled", name)
		}
		var coded interface{ LSPCode() int }
		if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("%s error = %v, want -32602", name, err)
		}
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

func TestLSPSemanticTokenEncodingHonorsCancellationBeforePublication(t *testing.T) {
	ctx := &semanticTokenCancellationContext{remaining: 2}
	tokens := []lspSemanticToken{
		{Line: 0, Character: 0, Length: 2, TokenType: 16},
		{Line: 0, Character: 3, Length: 1, TokenType: 17},
	}
	result, err := encodeSemanticTokensContext(ctx, "😀 + 1", tokens)
	if result.Data != nil {
		t.Fatalf("cancelled semantic-token encoding published data: %#v", result)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled semantic-token encoding error = %v, want -32800", err)
	}
}

func TestLSPSemanticTokenProtocolFixtureHasExactValidationContract(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-semantic-tokens.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, fixture))
	byID := make(map[int]map[string]any)
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok {
			byID[id] = frame
		}
	}
	valid, ok := byID[2]
	if !ok {
		t.Fatalf("valid semantic-token response missing: %#v", frames)
	}
	result, ok := valid["result"].(map[string]any)
	if !ok {
		t.Fatalf("valid semantic-token frame = %#v", valid)
	}
	data, ok := result["data"].([]any)
	if !ok || len(data) == 0 || len(data)%5 != 0 {
		t.Fatalf("semantic-token result data = %#v, want non-empty five-integer groups", result["data"])
	}
	if _, ok := result["resultId"]; ok {
		t.Fatalf("semantic-token result unexpectedly has resultId: %#v", result)
	}
	for _, id := range []int{3, 4, 5, 6} {
		frame, ok := byID[id]
		if !ok {
			t.Fatalf("semantic-token validation response %d missing: %#v", id, frames)
		}
		lspProtocolAssertErrorCode(t, frame, -32602)
	}
}

type semanticTokenCancellationContext struct {
	mu        sync.Mutex
	remaining int
}

func (c *semanticTokenCancellationContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *semanticTokenCancellationContext) Done() <-chan struct{}       { return nil }
func (c *semanticTokenCancellationContext) Value(any) any               { return nil }
func (c *semanticTokenCancellationContext) Err() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.remaining == 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestLSPWorkspaceDocumentAndWorkspaceSymbolsHaveExactShapeUTF16StaleCancellationAndDeterminism(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-symbols.osr")
	source := "let café = \"😀\"\n" +
		"struct User { name string }\n" +
		"func greet(name string) string => name\n" +
		"let answer = greet(café)\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 4); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":4}}`, uri))
	first, handled, err := server.handleIDERequest(context.Background(), "textDocument/documentSymbol", params)
	if err != nil || !handled {
		t.Fatalf("document-symbol request = %#v, handled=%v, err=%v", first, handled, err)
	}
	second, _, err := server.handleIDERequest(context.Background(), "textDocument/documentSymbol", params)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("document-symbol response is nondeterministic: %#v != %#v", first, second)
	}
	symbols, ok := first.([]lspDocumentSymbol)
	if !ok || len(symbols) != 5 {
		t.Fatalf("document-symbol response = %#v, want five typed symbols", first)
	}
	wantNames := []string{"café", "User", "name", "greet", "answer"}
	for i, symbol := range symbols {
		if symbol.Name != wantNames[i] {
			t.Fatalf("document symbol order = %#v, want %q at index %d", symbols, wantNames[i], i)
		}
		if symbol.Name == "" || symbol.Kind == 0 || symbol.Range == (lspRange{}) || symbol.SelectionRange == (lspRange{}) {
			t.Fatalf("document symbol %d has incomplete LSP shape: %#v", i, symbol)
		}
		if symbol.Range.Start.Line != symbol.SelectionRange.Start.Line || symbol.Range.Start.Character != symbol.SelectionRange.Start.Character {
			t.Fatalf("document symbol %d selection range does not identify its definition: %#v", i, symbol)
		}
	}
	if symbols[0].Name != "café" || symbols[0].Range.Start.Character != 4 || symbols[0].Range.End.Character != 8 {
		t.Fatalf("UTF-16 document symbol range = %#v, want café definition [4,8]", symbols[0])
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var wireSymbols []map[string]any
	if err := json.Unmarshal(wire, &wireSymbols); err != nil {
		t.Fatalf("document-symbol wire response = %s: %v", wire, err)
	}
	for i, item := range wireSymbols {
		for _, field := range []string{"name", "kind", "range", "selectionRange"} {
			if _, exists := item[field]; !exists {
				t.Fatalf("document-symbol wire item %d missing %q: %s", i, field, wire)
			}
		}
		if _, exists := item["children"]; exists {
			t.Fatalf("document-symbol wire item %d unexpectedly has children: %s", i, wire)
		}
	}

	workspace, handled, err := server.handleIDERequest(context.Background(), "workspace/symbol", json.RawMessage(`{"query":" greet "}`))
	if err != nil || !handled {
		t.Fatalf("workspace-symbol request = %#v, handled=%v, err=%v", workspace, handled, err)
	}
	workspaceSymbols, ok := workspace.([]lspWorkspaceSymbol)
	if !ok || len(workspaceSymbols) != 1 || workspaceSymbols[0].Name != "greet" {
		t.Fatalf("workspace-symbol response = %#v, want one greet symbol", workspace)
	}
	if workspaceSymbols[0].Location.URI != uri || workspaceSymbols[0].Location.Range.Start.Line != 2 {
		t.Fatalf("workspace-symbol location = %#v, want source line 2", workspaceSymbols[0].Location)
	}
	parameterQuery, handled, err := server.handleIDERequest(context.Background(), "workspace/symbol", json.RawMessage(`{"query":"name"}`))
	if err != nil || !handled {
		t.Fatalf("parameter-name workspace-symbol request = %#v, handled=%v, err=%v", parameterQuery, handled, err)
	}
	parameterSymbols, ok := parameterQuery.([]lspWorkspaceSymbol)
	if !ok || len(parameterSymbols) != 1 || parameterSymbols[0].Name != "name" || parameterSymbols[0].Detail != "field string" {
		t.Fatalf("parameter-name workspace symbols = %#v, want only the struct field declaration", parameterQuery)
	}
	stale := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":3}}`, uri))
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/documentSymbol", stale); err == nil {
		t.Fatal("stale document-symbol request was accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(cancelled, "textDocument/documentSymbol", params)
	if result != nil || !handled {
		t.Fatalf("cancelled document-symbol result = %#v, handled=%v", result, handled)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled document-symbol error = %v", err)
	}
	result, handled, err = server.handleIDERequest(cancelled, "workspace/symbol", json.RawMessage(`{"query":"greet"}`))
	if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled workspace-symbol result = %#v, handled=%v, err=%v", result, handled, err)
	}
}

func TestLSPWorkspaceSymbolsProtocolFixtureHasExactResultsAndStaleError(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-symbols.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, fixture))
	var sawDocument, sawWorkspace, sawParameterFilter, sawStale bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var symbols []lspDocumentSymbol
			if err := json.Unmarshal(data, &symbols); err != nil || len(symbols) != 5 {
				t.Fatalf("fixture document symbols = %s, err=%v", data, err)
			}
			if symbols[0].Name != "café" || symbols[0].Range.End.Character != 8 {
				t.Fatalf("fixture UTF-16 document symbol = %#v", symbols[0])
			}
			sawDocument = true
		case 3:
			data, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var symbols []lspWorkspaceSymbol
			if err := json.Unmarshal(data, &symbols); err != nil || len(symbols) != 1 || symbols[0].Name != "greet" {
				t.Fatalf("fixture workspace symbols = %s, err=%v", data, err)
			}
			if symbols[0].Location.URI != "file:///tmp/lsp-workspace-symbols.osr" || symbols[0].Location.Range.Start.Line != 2 {
				t.Fatalf("fixture workspace symbol location = %#v", symbols[0])
			}
			sawWorkspace = true
		case 5:
			data, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var symbols []lspWorkspaceSymbol
			if err := json.Unmarshal(data, &symbols); err != nil || len(symbols) != 1 || symbols[0].Name != "name" || symbols[0].Detail != "field string" {
				t.Fatalf("fixture parameter-name workspace symbols = %s, err=%v", data, err)
			}
			sawParameterFilter = true
		case 4:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		}
	}
	if !sawDocument || !sawWorkspace || !sawParameterFilter || !sawStale {
		t.Fatalf("symbols fixture frames = %#v; document=%v workspace=%v parameter-filter=%v stale=%v", frames, sawDocument, sawWorkspace, sawParameterFilter, sawStale)
	}
}

func TestLSPWorkspacePullDiagnosticsHaveExactShapeUTF16StaleCancellationAndDeterminism(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-diagnostics.osr")
	source := "emit \"😀\" + missing\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 9); err != nil {
		t.Fatal(err)
	}
	params := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":9}}`, uri))
	first, handled, err := server.handleIDERequest(context.Background(), "textDocument/diagnostic", params)
	if err != nil || !handled {
		t.Fatalf("pull-diagnostics request = %#v, handled=%v, err=%v", first, handled, err)
	}
	report, ok := first.(lspDiagnosticReport)
	if !ok || report.Kind != "full" || len(report.Items) != 1 {
		t.Fatalf("pull-diagnostics report = %#v, want one full report", first)
	}
	item := report.Items[0]
	if item.Range.Start != (lspPosition{Line: 0, Character: 12}) || item.Range.End != (lspPosition{Line: 0, Character: 19}) {
		t.Fatalf("diagnostic UTF-16 range = %#v, want [12,19] after astral literal", item.Range)
	}
	if item.Severity != 1 || item.Source != "osiris" || !strings.Contains(item.Message, `undefined name "missing"`) {
		t.Fatalf("diagnostic metadata = %#v", item)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var shape map[string]json.RawMessage
	if err := json.Unmarshal(wire, &shape); err != nil {
		t.Fatalf("diagnostic wire response = %s: %v", wire, err)
	}
	if len(shape) != 2 {
		t.Fatalf("diagnostic wire shape = %s, want exactly kind and items", wire)
	}
	for _, field := range []string{"kind", "items"} {
		if _, exists := shape[field]; !exists {
			t.Fatalf("diagnostic wire response missing %q: %s", field, wire)
		}
	}
	var items []map[string]json.RawMessage
	if err := json.Unmarshal(shape["items"], &items); err != nil || len(items) != 1 {
		t.Fatalf("diagnostic items wire shape = %s, err=%v", shape["items"], err)
	}
	for _, field := range []string{"range", "severity", "source", "message"} {
		if _, exists := items[0][field]; !exists {
			t.Fatalf("diagnostic item missing %q: %s", field, shape["items"])
		}
	}

	second, _, err := server.handleIDERequest(context.Background(), "textDocument/diagnostic", params)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated diagnostic response = %#v, err=%v; first=%#v", second, err, first)
	}
	report.Items[0].Message = "client mutation"
	third, _, err := server.handleIDERequest(context.Background(), "textDocument/diagnostic", params)
	if err != nil {
		t.Fatal(err)
	}
	if third.(lspDiagnosticReport).Items[0].Message == "client mutation" {
		t.Fatal("pull diagnostics exposed mutable server state")
	}

	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/diagnostic", json.RawMessage(`{"textDocument":{"uri":`+fmt.Sprintf("%q", uri)+`,"version":8}}`)); err == nil {
		t.Fatal("stale pull-diagnostics request was accepted")
	} else {
		var coded interface{ LSPCode() int }
		if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("stale pull-diagnostics error = %v", err)
		}
	}
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/diagnostic", json.RawMessage(`{"textDocument":{}}`)); err == nil {
		t.Fatal("malformed pull-diagnostics request was accepted")
	} else {
		var coded interface{ LSPCode() int }
		if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("malformed pull-diagnostics error = %v", err)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(cancelled, "textDocument/diagnostic", params)
	if result != nil || !handled {
		t.Fatalf("cancelled pull-diagnostics result = %#v, handled=%v", result, handled)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled pull-diagnostics error = %v", err)
	}
	postCopyCancelled := &lspWorkspaceDiagnosticCancelContext{cancelAtCheck: 2}
	if _, err := server.pullDiagnosticsContext(postCopyCancelled, uri); err == nil {
		t.Fatal("pull diagnostics returned a report after post-copy cancellation")
	} else if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("post-copy cancellation error = %v", err)
	}
}

type lspWorkspaceDiagnosticCancelContext struct {
	checks        int
	cancelAtCheck int
}

func (c *lspWorkspaceDiagnosticCancelContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (c *lspWorkspaceDiagnosticCancelContext) Done() <-chan struct{}       { return nil }
func (c *lspWorkspaceDiagnosticCancelContext) Value(any) any               { return nil }
func (c *lspWorkspaceDiagnosticCancelContext) Err() error {
	c.checks++
	if c.checks >= c.cancelAtCheck {
		return context.Canceled
	}
	return nil
}

func TestLSPWorkspacePullDiagnosticsProtocolFixtureHasExactResultsAndErrors(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-diagnostics.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("diagnostic fixture is nondeterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawDiagnostic, sawMalformed, sawStale bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var report map[string]json.RawMessage
			if err := json.Unmarshal(data, &report); err != nil || len(report) != 2 {
				t.Fatalf("fixture diagnostic report = %s, err=%v", data, err)
			}
			var kind string
			if err := json.Unmarshal(report["kind"], &kind); err != nil || kind != "full" {
				t.Fatalf("fixture diagnostic kind = %s, err=%v", report["kind"], err)
			}
			var items []lspDiagnostic
			if err := json.Unmarshal(report["items"], &items); err != nil || len(items) != 1 {
				t.Fatalf("fixture diagnostic items = %s, err=%v", report["items"], err)
			}
			if items[0].Range.Start.Character != 12 || items[0].Range.End.Character != 19 {
				t.Fatalf("fixture diagnostic UTF-16 range = %#v", items[0].Range)
			}
			sawDiagnostic = true
		case 3:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawMalformed = true
		case 4:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		}
	}
	if !sawDiagnostic || !sawMalformed || !sawStale {
		t.Fatalf("diagnostic fixture frames = %#v; diagnostic=%v malformed=%v stale=%v", frames, sawDiagnostic, sawMalformed, sawStale)
	}
}

func TestLSPWorkspaceIDEHintsFoldingAndSelectionHaveExactUTF16Shapes(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-ide-ranges.osr")
	source := "// first 😀 comment\n// second comment\n\nfunc greet(name string) string {\n  let café = name; café\n}\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 12); err != nil {
		t.Fatal(err)
	}
	full := fullDocumentRange(source)
	inlayParams := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12},"range":%s}`, uri, mustJSON(t, full)))
	first, handled, err := server.handleIDERequest(context.Background(), "textDocument/inlayHint", inlayParams)
	if err != nil || !handled {
		t.Fatalf("inlay-hint request = %#v, handled=%v, err=%v", first, handled, err)
	}
	hints, ok := first.([]lspInlayHint)
	if !ok || len(hints) == 0 {
		t.Fatalf("inlay-hint response = %#v, want checked binding hints", first)
	}
	for _, hint := range hints {
		if hint.Label == "" || hint.Kind != 1 || !hint.PaddingLeft {
			t.Fatalf("inlay-hint shape = %#v", hint)
		}
	}
	second, _, err := server.handleIDERequest(context.Background(), "textDocument/inlayHint", inlayParams)
	if err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated inlay-hint response = %#v, err=%v; first=%#v", second, err, first)
	}
	wire, err := json.Marshal(first)
	if err != nil {
		t.Fatal(err)
	}
	var wireHints []map[string]json.RawMessage
	if err := json.Unmarshal(wire, &wireHints); err != nil || len(wireHints) != len(hints) {
		t.Fatalf("inlay-hint wire shape = %s, err=%v", wire, err)
	}
	for _, item := range wireHints {
		for _, field := range []string{"position", "label", "kind", "paddingLeft"} {
			if _, exists := item[field]; !exists {
				t.Fatalf("inlay-hint item missing %q: %s", field, wire)
			}
		}
	}

	folding, handled, err := server.handleIDERequest(context.Background(), "textDocument/foldingRange", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12}}`, uri)))
	if err != nil || !handled {
		t.Fatalf("folding-range request = %#v, handled=%v, err=%v", folding, handled, err)
	}
	folds, ok := folding.([]lspFoldingRange)
	if !ok || len(folds) < 2 {
		t.Fatalf("folding-range response = %#v, diagnostics=%#v, want comment and function folds", folding, server.document(uri).diagnostics)
	}
	if folds[0].Kind != "comment" || folds[0].StartLine != 0 || folds[0].EndLine != 1 {
		t.Fatalf("comment folding range = %#v, want lines 0..1", folds[0])
	}
	var sawRegion bool
	for _, fold := range folds {
		if fold.Kind == "region" && fold.StartLine == 3 && fold.EndLine == 5 {
			sawRegion = true
		}
	}
	if !sawRegion {
		t.Fatalf("folding ranges = %#v, want multiline function region", folds)
	}

	selectionParams := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12},"positions":[{"line":4,"character":7},{"line":4,"character":12}]}`, uri))
	selection, handled, err := server.handleIDERequest(context.Background(), "textDocument/selectionRange", selectionParams)
	if err != nil || !handled {
		t.Fatalf("selection-range request = %#v, handled=%v, err=%v", selection, handled, err)
	}
	selections, ok := selection.([]lspSelectionRange)
	if !ok || len(selections) != 2 {
		t.Fatalf("selection-range response = %#v, want two results", selection)
	}
	if selections[0].Range.Start != (lspPosition{Line: 4, Character: 6}) || selections[0].Range.End != (lspPosition{Line: 4, Character: 10}) {
		t.Fatalf("selection UTF-16 range = %#v, want café [6,10]", selections[0].Range)
	}
	if selections[0].Parent == nil || selections[0].Parent.Parent == nil {
		t.Fatalf("selection range has incomplete parent chain: %#v", selections[0])
	}
	if selections[1].Range.Start != selections[1].Range.End {
		t.Fatalf("punctuation selection should be a point range: %#v", selections[1])
	}

	for _, request := range []struct {
		method string
		params string
	}{
		{"textDocument/inlayHint", fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12}}`, uri)},
		{"textDocument/selectionRange", fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12}}`, uri)},
	} {
		if _, _, err := server.handleIDERequest(context.Background(), request.method, json.RawMessage(request.params)); err == nil {
			t.Fatalf("malformed %s request was accepted", request.method)
		} else {
			var coded interface{ LSPCode() int }
			if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
				t.Fatalf("malformed %s error = %v", request.method, err)
			}
		}
	}
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/inlayHint", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12},"range":{"start":{"line":2,"character":4},"end":{"line":1,"character":0}}}`, uri))); err == nil {
		t.Fatal("reversed inlay range was accepted")
	}
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/selectionRange", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12},"positions":[{"line":0,"character":10}]}`, uri))); err == nil {
		t.Fatal("invalid UTF-16 selection position was accepted")
	}
	for _, method := range []string{"textDocument/inlayHint", "textDocument/foldingRange", "textDocument/selectionRange"} {
		var params json.RawMessage
		switch method {
		case "textDocument/inlayHint":
			params = json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":11},"range":%s}`, uri, mustJSON(t, full)))
		case "textDocument/foldingRange":
			params = json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":11}}`, uri))
		default:
			params = json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":11},"positions":[{"line":4,"character":7}]}`, uri))
		}
		if _, _, err := server.handleIDERequest(context.Background(), method, params); err == nil {
			t.Fatalf("stale %s request was accepted", method)
		}
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, method := range []string{"textDocument/inlayHint", "textDocument/foldingRange", "textDocument/selectionRange"} {
		params := inlayParams
		if method == "textDocument/foldingRange" {
			params = json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":12}}`, uri))
		} else if method == "textDocument/selectionRange" {
			params = selectionParams
		}
		result, handled, err := server.handleIDERequest(cancelled, method, params)
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
			t.Fatalf("cancelled %s = result %#v, handled=%v, err=%v", method, result, handled, err)
		}
	}
	postCopyCancelled := &lspWorkspaceDiagnosticCancelContext{cancelAtCheck: 2}
	if _, err := server.inlayHintsContext(postCopyCancelled, uri, full); err == nil {
		t.Fatal("inlay hints returned after cancellation")
	}
}

func mustJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestLSPWorkspaceIDEHintsFoldingAndSelectionProtocolFixtureIsDeterministic(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-ide-ranges.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("IDE range fixture is nondeterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawHints, sawFolds, sawSelections, sawMalformed, sawStale bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			var hints []lspInlayHint
			data, err := json.Marshal(frame["result"])
			if err != nil || json.Unmarshal(data, &hints) != nil || len(hints) == 0 || hints[0].Label == "" {
				t.Fatalf("fixture inlay hints = %s, err=%v, frame=%#v", data, err, frame)
			}
			sawHints = true
		case 3:
			var folds []lspFoldingRange
			data, err := json.Marshal(frame["result"])
			if err != nil || json.Unmarshal(data, &folds) != nil || len(folds) < 2 {
				t.Fatalf("fixture folding ranges = %s, err=%v", data, err)
			}
			sawFolds = true
		case 4:
			var selections []lspSelectionRange
			data, err := json.Marshal(frame["result"])
			if err != nil || json.Unmarshal(data, &selections) != nil || len(selections) != 2 || selections[0].Parent == nil {
				t.Fatalf("fixture selection ranges = %s, err=%v", data, err)
			}
			sawSelections = true
		case 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawMalformed = true
		case 6:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		}
	}
	if !sawHints || !sawFolds || !sawSelections || !sawMalformed || !sawStale {
		t.Fatalf("IDE range fixture frames = %#v; hints=%v folds=%v selections=%v malformed=%v stale=%v", frames, sawHints, sawFolds, sawSelections, sawMalformed, sawStale)
	}
}

func TestLSPSemanticLSPVersionsCancellationAndTypeHierarchy(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-semantic-types.osr")
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

func TestLSPSemanticStdlibExportsNavigateWithUTF16AndIdentity(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-semantic-stdlib.osr")
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

func TestLSPSemanticStdlibExportsFramedUTF16AndStale(t *testing.T) {
	uri := pathToURI("/tmp/lsp-semantic-stdlib-framed.osr")
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
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, []byte(fixture)))
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		seen[id] = true
		if id == 6 {
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{2, 3, 4, 5, 6, 7} {
		if !seen[id] {
			t.Fatalf("stdlib framed request %d did not receive a response: %#v", id, frames)
		}
	}
	var completions []lspCompletionItem
	for _, frame := range frames {
		if id, _ := lspProtocolFrameID(frame); id != 2 {
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

func TestLSPStdlibTextExportInventoryMatchesRuntimeModule(t *testing.T) {
	moduleValue, err := stdlibModule(NewRuntime(), "std/text", "")
	if err != nil {
		t.Fatal(err)
	}
	module, err := moduleValue.AsModule()
	if err != nil {
		t.Fatal(err)
	}
	expected := make(map[string]bool, len(module.members))
	for name := range module.members {
		expected[name] = true
	}

	uri := pathToURI("/tmp/lsp-stdlib-text-inventory.osr")
	source := "import \"std/text\" as text\nlet value = text.trim(\"x\")\nemit value\n"
	encodedSource, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	fixture := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":2,"text":%s}}}
{"jsonrpc":"2.0","id":2,"method":"textDocument/completion","params":{"textDocument":{"uri":%q,"version":2},"position":{"line":1,"character":%d}}}
{"jsonrpc":"2.0","id":3,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`,
		uri, encodedSource, uri, utf16Units("let value = text."))
	first := runLSPProtocolFixture(t, []byte(fixture))
	second := runLSPProtocolFixture(t, []byte(fixture))
	if first != second {
		t.Fatalf("stdlib export inventory output is not deterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	var completions []lspCompletionItem
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok || id != 2 {
			continue
		}
		payload, err := json.Marshal(frame["result"])
		if err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(payload, &completions); err != nil {
			t.Fatal(err)
		}
	}
	got := make(map[string]bool, len(completions))
	for _, item := range completions {
		if expected[item.Label] && strings.Contains(item.Detail, "std/text.") {
			got[item.Label] = true
		}
	}
	for name := range expected {
		if !got[name] {
			t.Fatalf("stdlib text export %q missing from completion inventory: %#v", name, completions)
		}
	}
}

func TestLSPStdlibExportInventoriesMatchRuntimeModules(t *testing.T) {
	modules := []struct {
		path  string
		alias string
	}{
		{path: "std/math", alias: "math"},
		{path: "std/crypto", alias: "crypto"},
		{path: "std/signal", alias: "signal"},
		{path: "std/list", alias: "list"},
		{path: "std/time", alias: "time"},
		{path: "std/net", alias: "net"},
		{path: "std/os", alias: "os"},
	}
	for _, moduleSpec := range modules {
		t.Run(moduleSpec.alias, func(t *testing.T) {
			moduleValue, err := stdlibModule(NewRuntime(), moduleSpec.path, "")
			if err != nil {
				t.Fatal(err)
			}
			module, err := moduleValue.AsModule()
			if err != nil {
				t.Fatal(err)
			}
			names := make([]string, 0, len(module.members))
			for name := range module.members {
				names = append(names, name)
			}
			sort.Strings(names)
			if len(names) == 0 {
				t.Fatal("stdlib module has no exported members")
			}
			uri := pathToURI("/tmp/lsp-stdlib-" + moduleSpec.alias + "-inventory.osr")
			source := fmt.Sprintf("import %q as %s\nlet value = %s.%s\nemit value\n", moduleSpec.path, moduleSpec.alias, moduleSpec.alias, names[0])
			encodedSource, err := json.Marshal(source)
			if err != nil {
				t.Fatal(err)
			}
			fixture := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":2,"text":%s}}}
{"jsonrpc":"2.0","id":2,"method":"textDocument/completion","params":{"textDocument":{"uri":%q,"version":2},"position":{"line":1,"character":%d}}}
{"jsonrpc":"2.0","id":3,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`,
				uri, encodedSource, uri, utf16Units("let value = "+moduleSpec.alias+"."))
			first := runLSPProtocolFixture(t, []byte(fixture))
			second := runLSPProtocolFixture(t, []byte(fixture))
			if first != second {
				t.Fatalf("stdlib export inventory is not deterministic")
			}
			frames := decodeLSPProtocolFrames(t, first)
			var completions []lspCompletionItem
			for _, frame := range frames {
				id, ok := lspProtocolFrameID(frame)
				if !ok || id != 2 {
					continue
				}
				payload, err := json.Marshal(frame["result"])
				if err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(payload, &completions); err != nil {
					t.Fatal(err)
				}
			}
			got := make(map[string]bool, len(completions))
			for _, item := range completions {
				if strings.Contains(item.Detail, moduleSpec.path+".") {
					got[item.Label] = true
				}
			}
			for _, name := range names {
				if !got[name] {
					t.Fatalf("stdlib %s export %q missing from completion inventory", moduleSpec.path, name)
				}
			}
		})
	}
}

func TestLSPSemanticStdlibExportsFramedCancellation(t *testing.T) {
	uri := pathToURI("/tmp/lsp-semantic-stdlib-cancel.osr")
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
	responses := decodeLSPProtocolFrames(t, output.String())
	var sawCancelled bool
	for _, frame := range responses {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Fatalf("cancelled framed request produced no cancellation response: %#v", responses)
	}
}

func TestLSPSemanticGenericTypeParametersNavigateWithUTF16AndSharedChecker(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-semantic-generics.osr")
	source := "func identity<T>(value T) T => value\n" +
		"func wrap<T>(value T) T => identity<T>(value)\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}
	if doc := server.document(uri); doc == nil || len(doc.diagnostics) != 0 {
		t.Fatalf("generic source diagnostics = %#v", doc)
	}
	identityLine := "func identity<T>(value T) T => value"
	definitionIndex := strings.Index(identityLine, "<") + 1
	parameterIndex := strings.Index(identityLine, "value T") + len("value ")
	parameterPosition := lspPosition{Line: 0, Character: utf16Units(identityLine[:parameterIndex])}
	hover, err := server.hover(uri, parameterPosition)
	if err != nil || hover == nil || !strings.Contains(hover.Contents.Value, "type parameter") {
		t.Fatalf("generic type-parameter hover = %#v, %v", hover, err)
	}
	definition, err := server.definition(uri, parameterPosition)
	if err != nil {
		t.Fatal(err)
	}
	location, ok := definition.(lspLocation)
	if !ok || location.URI != uri || location.Range.Start.Line != 0 || location.Range.Start.Character != utf16Units(identityLine[:definitionIndex]) {
		t.Fatalf("generic type-parameter definition = %#v", definition)
	}
	references, err := server.references(uri, parameterPosition)
	if err != nil || len(references) != 3 {
		t.Fatalf("generic type-parameter references = %#v, %v; want declaration plus parameter and result uses", references, err)
	}
	wrapLine := "func wrap<T>(value T) T => identity<T>(value)"
	completionPosition := lspPosition{Line: 1, Character: utf16Units(wrapLine[:strings.Index(wrapLine, "identity<T>")+len("identity<")])}
	items, err := server.completion(uri, completionPosition)
	if err != nil {
		t.Fatal(err)
	}
	var foundTypeParameter bool
	for _, item := range items {
		if item.Label == "T" && item.Kind == 25 && strings.Contains(item.Detail, "type parameter") {
			foundTypeParameter = true
		}
	}
	if !foundTypeParameter {
		t.Fatalf("generic type-parameter completion = %#v", items)
	}
	callTypeIndex := strings.Index(wrapLine, "identity<T>") + len("identity<")
	callPosition := lspPosition{Line: 1, Character: utf16Units(wrapLine[:callTypeIndex])}
	callDefinition, err := server.definition(uri, callPosition)
	if err != nil {
		t.Fatal(err)
	}
	callLocation, ok := callDefinition.(lspLocation)
	if !ok || callLocation.Range.Start.Line != 1 || callLocation.Range.Start.Character != utf16Units(wrapLine[:strings.Index(wrapLine, "<")+1]) {
		t.Fatalf("generic call type-argument definition = %#v", callDefinition)
	}
}

func TestLSPSemanticGenericTypeParametersFramedMalformedStaleAndCancellation(t *testing.T) {
	uri := pathToURI("/tmp/lsp-semantic-generics-protocol.osr")
	source := "func identity<T>(value T) T => value\n" +
		"func wrap<T>(value T) T => identity<T>(value)\n"
	encodedSource, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	line := "func identity<T>(value T) T => value"
	parameterIndex := strings.Index(line, "value T") + len("value ")
	position := fmt.Sprintf(`{"line":0,"character":%d}`, utf16Units(line[:parameterIndex]))
	fixture := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}` + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":7,"text":%s}}}`, uri, encodedSource) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":3,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"textDocument/references","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":5,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":6},"position":%s}}`, uri, position) + "\n" +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"textDocument":"malformed"}}`) + "\n" +
		`{"jsonrpc":"2.0","id":7,"method":"shutdown"}` + "\n" +
		`{"jsonrpc":"2.0","method":"exit"}`
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, []byte(fixture)))
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		seen[id] = true
		if id == 5 || id == 6 {
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{2, 3, 4, 5, 6, 7} {
		if !seen[id] {
			t.Fatalf("generic framed request %d did not receive a response: %#v", id, frames)
		}
	}

	var inputChunks [][]byte
	for _, payload := range []string{
		`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`,
		fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":7,"text":%s}}}`, uri, encodedSource),
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":7},"position":%s}}`, uri, position),
	} {
		inputChunks = append(inputChunks, []byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: inputChunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server := NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled generic framed Serve error = %v", err)
	}
	responses := decodeLSPProtocolFrames(t, output.String())
	var sawCancelled bool
	for _, frame := range responses {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Fatalf("cancelled generic framed request produced no cancellation response: %#v", responses)
	}
}

func TestLSPSemanticMacroNavigationFramedUTF16MalformedStaleAndCancellation(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-semantic-macros.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, fixture))
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		seen[id] = true
		switch id {
		case 2:
			contents, ok := frame["result"].(map[string]any)
			if !ok || !strings.Contains(fmt.Sprint(contents["contents"]), "compile-time macro") {
				t.Fatalf("macro call hover = %#v", frame)
			}
			if r, ok := contents["range"].(map[string]any); !ok || r["start"].(map[string]any)["character"] != float64(15) {
				t.Fatalf("macro call UTF-16 hover range = %#v", frame)
			}
		case 3:
			var location lspLocation
			data, marshalErr := json.Marshal(frame["result"])
			if marshalErr != nil || json.Unmarshal(data, &location) != nil {
				t.Fatalf("macro definition response = %#v", frame)
			}
			if location.URI != "file:///tmp/lsp-semantic-macros-fixture.osr" || location.Range.Start.Line != 0 || location.Range.Start.Character != 6 {
				t.Fatalf("macro definition location = %#v", location)
			}
		case 4:
			var references []lspLocation
			data, marshalErr := json.Marshal(frame["result"])
			if marshalErr != nil || json.Unmarshal(data, &references) != nil || len(references) != 2 {
				t.Fatalf("macro references = %#v", frame)
			}
		case 5, 6:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{2, 3, 4, 5, 6, 7} {
		if !seen[id] {
			t.Fatalf("macro framed request %d did not receive a response: %#v", id, frames)
		}
	}

	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-semantic-macros.osr")
	source := "macro wrap(value, suffix) => cat(value, suffix)\n" +
		"emit wrap(\"hi\", \"!\")\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}
	macroLine := "macro wrap(value, suffix) => cat(value, suffix)"
	param := lspPosition{Line: 0, Character: utf16Units(macroLine[:strings.Index(macroLine, "value")])}
	definition, err := server.definition(uri, param)
	if err != nil {
		t.Fatal(err)
	}
	paramLocation, ok := definition.(lspLocation)
	if !ok || paramLocation.Range.Start.Character != utf16Units(macroLine[:strings.Index(macroLine, "value")]) || paramLocation.Range.End.Character != utf16Units(macroLine[:strings.Index(macroLine, "value")])+len("value") {
		t.Fatalf("macro parameter definition = %#v", definition)
	}

	encodedSource, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	call := `{"jsonrpc":"2.0","id":2,"method":"textDocument/hover","params":{"textDocument":{"uri":` + fmt.Sprintf("%q", uri) + `,"version":7},"position":{"line":1,"character":10}}}`
	inputChunks := [][]byte{
		[]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`), `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}`)),
		[]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":7,"text":%s}}}`, uri, encodedSource)), fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":7,"text":%s}}}`, uri, encodedSource))),
		[]byte(fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(call), call)),
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: inputChunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server = NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled macro framed Serve error = %v", err)
	}
	responses := decodeLSPProtocolFrames(t, output.String())
	var sawCancelled bool
	for _, frame := range responses {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			sawCancelled = true
		}
	}
	if !sawCancelled {
		t.Fatalf("cancelled macro framed request produced no cancellation response: %#v", responses)
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

func TestLSPSemanticDidSaveRevalidatesOverlayWithoutAdvancingVersion(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-semantic-save.osr")
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

func TestLSPSemanticDidSaveProtocolNotificationRevalidatesAndDoesNotRespond(t *testing.T) {
	fixture := []byte(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":"file:///tmp/lsp-semantic-save-protocol.osr","languageId":"osiris","version":7,"text":"let answer = 1\nemit answer\n"}}}
{"jsonrpc":"2.0","method":"textDocument/didSave","params":{"textDocument":{"uri":"file:///tmp/lsp-semantic-save-protocol.osr"},"text":"let =\n"}}
{"jsonrpc":"2.0","id":2,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`)
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, fixture))
	if got := countLSPProtocolNotifications(frames, "textDocument/publishDiagnostics"); got != 2 {
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

func TestLSPWorkspaceVersionedWorkspaceEditsAndColocatedInventory(t *testing.T) {
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

func TestLSPWorkspaceOrganizeImportsProducesMinimalVersionedEdits(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-imports.osr")
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

func TestLSPWorkspaceColocatedInventoryDrivesCodeLensesAndUsesOverlay(t *testing.T) {
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

func TestLSPWorkspaceTestInventoryHonorsCancellationAndSymlinkRootBoundary(t *testing.T) {
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

func TestLSPMatrixStableLSPCellsRequireFixtureEvidence(t *testing.T) {
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

func TestLSPProtocolFixturesAreDeterministic(t *testing.T) {
	for _, name := range []string{"lsp-semantic.jsonl", "lsp-semantic-generics.jsonl", "lsp-semantic-macros.jsonl", "lsp-workspace.jsonl", "lsp-workspace-import-actions.jsonl", "lsp-workspace-test-inventory.jsonl", "lsp-workspace-semantic-tokens.jsonl", "lsp-workspace-symbols.jsonl", "lsp-workspace-rename.jsonl"} {
		fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", name))
		if err != nil {
			t.Fatal(err)
		}
		first := runLSPProtocolFixture(t, fixture)
		second := runLSPProtocolFixture(t, fixture)
		if first != second {
			t.Fatalf("fixture %s is not deterministic", name)
		}
		frames := decodeLSPProtocolFrames(t, first)
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

func TestLSPSourceBackedRenameAcrossSemanticKinds(t *testing.T) {
	tests := []struct {
		name        string
		source      string
		line        int
		character   int
		replacement string
		wantRanges  int
		wantName    string
	}{
		{
			name:        "local binding excludes shadow",
			source:      "let answer = 42\nfunc show(answer) => answer\nemit answer\n",
			line:        2,
			character:   5,
			replacement: "result",
			wantRanges:  2,
			wantName:    "answer",
		},
		{
			name:        "struct field",
			source:      "struct User { name string }\nlet user = User{name: \"Ada\"}\nemit user.name\n",
			line:        2,
			character:   10,
			replacement: "displayName",
			wantRanges:  2,
			wantName:    "name",
		},
		{
			name:        "method",
			source:      "struct User { name string }\ntrait Render { func render() string }\nimpl Render for User { func render() string => \"ok\" }\nlet user = User{name: \"Ada\"}\nemit user.render()\n",
			line:        4,
			character:   11,
			replacement: "format",
			wantRanges:  2,
			wantName:    "render",
		},
		{
			name:        "generic type parameter",
			source:      "func wrap<T>(value T) T => value\nemit wrap<u64>(1)\n",
			line:        0,
			character:   10,
			replacement: "Element",
			wantRanges:  3,
			wantName:    "T",
		},
		{
			name:        "macro symbol",
			source:      "macro identity(value) => value\nemit identity(1)\n",
			line:        1,
			character:   6,
			replacement: "same",
			wantRanges:  2,
			wantName:    "identity",
		},
		{
			name:        "import alias",
			source:      "import \"std/text\" as text\nemit text.slug(\"Osiris\")\n",
			line:        1,
			character:   5,
			replacement: "words",
			wantRanges:  2,
			wantName:    "text",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
			uri := pathToURI("/tmp/lsp-workspace-rename-" + strings.ReplaceAll(tc.name, " ", "-") + ".osr")
			if err := server.upsertDocumentContext(context.Background(), uri, tc.source, 7); err != nil {
				t.Fatal(err)
			}
			prepared, err := server.prepareRename(uri, lspPosition{Line: tc.line, Character: tc.character})
			if err != nil {
				t.Fatalf("prepareRename: %v", err)
			}
			preparedRange, ok := prepared.(lspRange)
			if !ok || preparedRange.Start.Line != tc.line {
				t.Fatalf("prepareRename = %#v, want source range on line %d", prepared, tc.line)
			}
			edit, err := server.renameWorkspaceEditAtVersion(uri, lspPosition{Line: tc.line, Character: tc.character}, tc.replacement, intPointer(7))
			if err != nil {
				t.Fatalf("renameWorkspaceEdit = %v", err)
			}
			if len(edit.DocumentChanges) != 1 || len(edit.DocumentChanges[0].Edits) != tc.wantRanges {
				t.Fatalf("rename edits = %#v, want %d edits", edit, tc.wantRanges)
			}
			for i, item := range edit.DocumentChanges[0].Edits {
				if item.NewText != tc.replacement {
					t.Fatalf("edit %d = %#v, want replacement %q", i, item, tc.replacement)
				}
				if i > 0 {
					previous := edit.DocumentChanges[0].Edits[i-1].Range
					if lspRangesOverlap(previous, item.Range) {
						t.Fatalf("rename edits overlap: %#v", edit.DocumentChanges[0].Edits)
					}
					if item.Range.Start.Line > previous.Start.Line || (item.Range.Start.Line == previous.Start.Line && item.Range.Start.Character >= previous.Start.Character) {
						t.Fatalf("edits are not deterministic descending source order: %#v", edit.DocumentChanges[0].Edits)
					}
				}
			}
			_, selected := server.document(uri).analysis.findAt(toSourcePosInText(tc.source, lspPosition{Line: tc.line, Character: tc.character}))
			if selected == nil || spanText(tc.source, selected.span) != tc.wantName {
				got := ""
				if selected != nil {
					got = spanText(tc.source, selected.span)
				}
				t.Fatalf("selected symbol text = %q, want %q", got, tc.wantName)
			}
			if _, err := server.renameWorkspaceEditAtVersion(uri, lspPosition{Line: tc.line, Character: tc.character}, tc.replacement, intPointer(8)); err == nil {
				t.Fatal("stale rename request was accepted")
			}
		})
	}
}

func TestLSPRenameMalformedCancellationAndSourceBoundary(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-rename-boundary.osr")
	source := "import \"std/text\" as text\nemit text.slug(\"Osiris\")\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 3); err != nil {
		t.Fatal(err)
	}
	if _, err := server.prepareRename(uri, lspPosition{Line: 1, Character: 10}); err != nil {
		t.Fatalf("virtual module-member prepareRename returned error: %v", err)
	}
	if prepared, err := server.prepareRename(uri, lspPosition{Line: 1, Character: 5}); err != nil || prepared == nil {
		t.Fatalf("source-backed alias prepareRename = %#v, %v", prepared, err)
	}
	if _, _, err := server.handleIDERequest(context.Background(), "textDocument/prepareRename", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":3},"position":{"line":99,"character":0}}`, uri))); err == nil {
		t.Fatal("invalid rename position was accepted")
	}
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(cancelled, "textDocument/prepareRename", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":3},"position":{"line":1,"character":5}}`, uri)))
	if result != nil || !handled {
		t.Fatalf("cancelled prepareRename result = %#v, handled=%v, err=%v", result, handled, err)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled prepareRename error = %v", err)
	}
	if _, err := server.renameWorkspaceEditContext(cancelled, uri, lspPosition{Line: 1, Character: 5}, "words", intPointer(3)); !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("cancelled rename error = %v", err)
	}
}

func TestLSPWorkspaceRenameProtocolFixtureHasExactRangesAndErrors(t *testing.T) {
	fixture, err := os.ReadFile(filepath.Join("testdata", "lsp", "lsp-workspace-rename.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	if first != runLSPProtocolFixture(t, fixture) {
		t.Fatal("rename fixture is nondeterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawPrepare, sawRename, sawStale, sawMalformed bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, _ := json.Marshal(frame["result"])
			var r lspRange
			if err := json.Unmarshal(data, &r); err != nil || r.Start.Line != 1 || r.Start.Character != 5 || r.End.Character != 11 {
				t.Fatalf("prepareRename fixture result = %s, err=%v", data, err)
			}
			sawPrepare = true
		case 3:
			data, _ := json.Marshal(frame["result"])
			var edit lspWorkspaceEdit
			if err := json.Unmarshal(data, &edit); err != nil || len(edit.DocumentChanges) != 1 || len(edit.DocumentChanges[0].Edits) != 2 {
				t.Fatalf("rename fixture result = %s, err=%v", data, err)
			}
			sawRename = true
		case 4:
			if _, ok := frame["error"]; !ok {
				t.Fatalf("stale rename fixture frame = %#v", frame)
			}
			sawStale = true
		case 5:
			if _, ok := frame["error"]; !ok {
				t.Fatalf("malformed rename fixture frame = %#v", frame)
			}
			sawMalformed = true
		}
	}
	if !sawPrepare || !sawRename || !sawStale || !sawMalformed {
		t.Fatalf("rename fixture frames = %#v", frames)
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
