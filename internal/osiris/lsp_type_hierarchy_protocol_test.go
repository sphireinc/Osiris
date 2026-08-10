package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
)

func TestLSPTypeHierarchyProtocolFixtureIsExactDeterministicAndValidated(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-type-hierarchy.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatal("type-hierarchy protocol output was not deterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	byID := make(map[int]map[string]any)
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok {
			byID[id] = frame
		}
	}

	assertLSPJSONResult(t, byID[2], `[{"data":"Render","kind":11,"name":"Render","range":{"end":{"character":12,"line":0},"start":{"character":6,"line":0}},"selectionRange":{"end":{"character":12,"line":0},"start":{"character":6,"line":0}},"uri":"file:///tmp/lspTypeHierarchy.osr"}]`)
	assertLSPJSONResult(t, byID[3], `[{"data":"User","kind":23,"name":"User","range":{"end":{"character":11,"line":1},"start":{"character":7,"line":1}},"selectionRange":{"end":{"character":11,"line":1},"start":{"character":7,"line":1}},"uri":"file:///tmp/lspTypeHierarchy.osr"}]`)
	assertLSPJSONResult(t, byID[4], `[{"data":"Render","kind":11,"name":"Render","range":{"end":{"character":12,"line":0},"start":{"character":6,"line":0}},"selectionRange":{"end":{"character":12,"line":0},"start":{"character":6,"line":0}},"uri":"file:///tmp/lspTypeHierarchy.osr"}]`)
	assertLSPJSONResult(t, byID[5], `[{"data":"Admin","kind":23,"name":"Admin","range":{"end":{"character":12,"line":2},"start":{"character":7,"line":2}},"selectionRange":{"end":{"character":12,"line":2},"start":{"character":7,"line":2}},"uri":"file:///tmp/lspTypeHierarchy.osr"},{"data":"User","kind":23,"name":"User","range":{"end":{"character":11,"line":1},"start":{"character":7,"line":1}},"selectionRange":{"end":{"character":11,"line":1},"start":{"character":7,"line":1}},"uri":"file:///tmp/lspTypeHierarchy.osr"}]`)
	assertLSPJSONResult(t, byID[6], `[]`)
	lspProtocolAssertErrorCode(t, byID[7], -32602)
	lspProtocolAssertErrorCode(t, byID[8], -32602)
	lspProtocolAssertErrorCode(t, byID[9], -32602)
	lspProtocolAssertErrorCode(t, byID[10], -32602)
	lspProtocolAssertErrorCode(t, byID[11], -32602)
}

func TestLSPTypeHierarchyCancellationReturnsNoPartialResult(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-type-hierarchy-protocol-cancel.osr")
	source := "trait Render { func render() string }\nstruct User { name string }\nimpl Render for User { func render() string => \"ok\" }\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 1); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, request := range []struct {
		method string
		params string
	}{
		{"typeHierarchy/prepare", `{"textDocument":{"uri":"` + uri + `","version":1},"position":{"line":1,"character":8}}`},
		{"typeHierarchy/supertypes", `{"item":{"name":"User","uri":"` + uri + `"}}`},
		{"typeHierarchy/subtypes", `{"item":{"name":"Render","uri":"` + uri + `"}}`},
	} {
		result, handled, err := server.handleIDERequest(canceled, request.method, json.RawMessage(request.params))
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
			t.Fatalf("canceled %s = result %#v, handled=%v, err=%v", request.method, result, handled, err)
		}
	}
}

func assertLSPJSONResult(t *testing.T, frame map[string]any, want string) {
	t.Helper()
	if frame == nil {
		t.Fatal("missing protocol response frame")
	}
	got, ok := frame["result"]
	if !ok {
		t.Fatalf("response has no result: %#v", frame)
	}
	var wantValue, gotValue any
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(encoded, &gotValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Fatalf("result = %s, want %s", encoded, want)
	}
}
