package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestLSPNavigationPositionValidationFixtureIsDeterministic(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-navigation-validation.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatal("navigation validation protocol output was not deterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	byID := make(map[int]map[string]any)
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok {
			byID[id] = frame
		}
	}
	if result, ok := byID[2]["result"].(map[string]any); !ok || len(result) == 0 {
		t.Fatalf("prepareRename result = %#v, want a range", byID[2])
	}
	if result, ok := byID[8]["result"].([]any); !ok || len(result) != 1 {
		t.Fatalf("callHierarchy/prepare result = %#v, want one item", byID[8])
	}
	for _, id := range []int{3, 4, 5, 6, 7, 9, 10, 11, 12, 13, 14} {
		lspProtocolAssertErrorCode(t, byID[id], -32602)
	}
}

func TestLSPNavigationPositionValidationRejectsMalformedWireShapes(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-navigation-malformed.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "func greet() string => \"ok\"\n", 4); err != nil {
		t.Fatal(err)
	}
	requests := []struct {
		method string
		params string
	}{
		{"textDocument/prepareRename", `{"textDocument":{"uri":"` + uri + `","version":4},"position":{"line":0}}`},
		{"textDocument/prepareRename", `{"textDocument":{"uri":"` + uri + `","version":4},"position":{"character":5}}`},
		{"callHierarchy/prepare", `{"textDocument":{"uri":"` + uri + `","version":4},"position":null}`},
		{"callHierarchy/prepare", `{"textDocument":{"uri":"` + uri + `","version":4},"position":[]}`},
	}
	for _, request := range requests {
		result, handled, err := server.handleIDERequest(context.Background(), request.method, json.RawMessage(request.params))
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("%s malformed request = result %#v, handled=%v, err=%v", request.method, result, handled, err)
		}
	}
}

func TestLSPNavigationPositionValidationHonorsCancellation(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-navigation-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "func greet() string => \"ok\"\n", 2); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, request := range []struct {
		method string
		params string
	}{
		{"textDocument/prepareRename", `{"textDocument":{"uri":"` + uri + `","version":2},"position":{"line":0,"character":5}}`},
		{"callHierarchy/prepare", `{"textDocument":{"uri":"` + uri + `","version":2},"position":{"line":0,"character":5}}`},
	} {
		result, handled, err := server.handleIDERequest(canceled, request.method, json.RawMessage(request.params))
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
			t.Fatalf("cancelled %s = result %#v, handled=%v, err=%v", request.method, result, handled, err)
		}
	}
}

func TestLSPNavigationPositionValidationUsesUTF16Boundaries(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-navigation-utf16.osr")
	source := "func greet() string => \"ok\"\nfunc caller() string => cat(\"😀\", greet())\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := server.prepareCallHierarchyContext(context.Background(), uri, lspPosition{Line: 1, Character: 5}); err != nil {
		t.Fatalf("valid call position = %v", err)
	}
	for _, method := range []string{"textDocument/prepareRename", "callHierarchy/prepare"} {
		params := json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":1},"position":{"line":1,"character":30}}`)
		result, handled, err := server.handleIDERequest(context.Background(), method, params)
		var coded interface{ LSPCode() int }
		resultValue := reflect.ValueOf(result)
		nonNilResult := result != nil && !(resultValue.Kind() == reflect.Slice && resultValue.IsNil())
		if nonNilResult || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("surrogate-split %s = result %#v, handled=%v, err=%v", method, result, handled, err)
		}
	}
	if got := utf16Units("😀"); got != 2 {
		t.Fatalf("astral UTF-16 width = %d, want 2", got)
	}
}
