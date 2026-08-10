package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestLSPFormattingProtocolFixtureIsExactDeterministicAndValidated(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-formatting.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatal("formatting protocol output was not deterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	byID := make(map[int]map[string]any)
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok {
			byID[id] = frame
		}
	}

	formattedSource := "let greeting = \"😀\"\nemit greeting\n"
	wantEdit := []any{map[string]any{
		"range": map[string]any{
			"start": map[string]any{"line": float64(0), "character": float64(0)},
			"end":   map[string]any{"line": float64(2), "character": float64(0)},
		},
		"newText": formattedSource,
	}}
	assertFormattingResult(t, byID[2], wantEdit)
	assertFormattingResult(t, byID[3], wantEdit)
	assertFormattingResult(t, byID[4], []any{})
	lspProtocolAssertErrorCode(t, byID[5], -32602)
	lspProtocolAssertErrorCode(t, byID[6], -32602)
	lspProtocolAssertErrorCode(t, byID[7], -32602)
}

func TestLSPFormattingCancellationReturnsNoPartialEdit(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-formatting-protocol-cancel.osr")
	original := "let answer=1\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, original, 7); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(canceled, "textDocument/formatting", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":7}}`))
	var coded interface{ LSPCode() int }
	if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("canceled formatting = result %#v, handled=%v, err=%v", result, handled, err)
	}
	if server.document(uri).text != original {
		t.Fatal("canceled formatting mutated the open overlay")
	}
}

func assertFormattingResult(t *testing.T, frame map[string]any, want any) {
	t.Helper()
	if frame == nil {
		t.Fatal("missing formatting response frame")
	}
	got, ok := frame["result"]
	if !ok {
		t.Fatalf("formatting response has no result: %#v", frame)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("formatting result = %#v, want %#v", got, want)
	}
}
