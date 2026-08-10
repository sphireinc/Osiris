package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestLSPDeclarationProtocolFixtureIsDeterministicAndValidatesRequests(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-declaration.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("declaration fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	seen := map[int]bool{}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		seen[id] = true
		switch id {
		case 2, 3:
			payload, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var location lspLocation
			if err := json.Unmarshal(payload, &location); err != nil {
				t.Fatal(err)
			}
			if location.URI != "file:///tmp/lsp-declaration-fixture.osr" || location.Range.Start != (lspPosition{Line: 0, Character: 4}) || location.Range.End != (lspPosition{Line: 0, Character: 10}) {
				t.Fatalf("declaration response %d = %#v", id, location)
			}
		case 4, 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{1, 2, 3, 4, 5, 6} {
		if !seen[id] {
			t.Errorf("declaration fixture response %d missing", id)
		}
	}
}

func TestLSPDeclarationNavigationMatchesSourceDefinition(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-declaration.osr")
	source := "let answer = \"😀\"\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":7},"position":{"line":1,"character":5}}`, uri))
	result, handled, err := server.handleIDERequest(context.Background(), "textDocument/declaration", raw)
	if err != nil || !handled {
		t.Fatalf("declaration request = %#v, handled=%v, err=%v", result, handled, err)
	}
	location, ok := result.(lspLocation)
	if !ok {
		t.Fatalf("declaration result = %#v, want lspLocation", result)
	}
	if location.URI != uri || location.Range.Start != (lspPosition{Line: 0, Character: 4}) || location.Range.End != (lspPosition{Line: 0, Character: 10}) {
		t.Fatalf("declaration location = %#v, want answer declaration at [0:4,0:10]", location)
	}
	direct, err := server.declaration(uri, lspPosition{Line: 1, Character: 5})
	if err != nil {
		t.Fatal(err)
	}
	if direct != result {
		t.Fatalf("direct declaration = %#v, request declaration = %#v", direct, result)
	}
	definition, err := server.definition(uri, lspPosition{Line: 1, Character: 5})
	if err != nil {
		t.Fatal(err)
	}
	if definition != result {
		t.Fatalf("declaration = %#v, definition = %#v; source-backed declarations should share identity", result, definition)
	}
}

func TestLSPDeclarationNavigationValidatesRequests(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-declaration-validation.osr")
	source := "let answer = \"😀\"\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}
	valid := func(position string, version int) json.RawMessage {
		return json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":%d},"position":%s}`, uri, version, position))
	}
	cases := []struct {
		name string
		raw  json.RawMessage
	}{
		{name: "missing position", raw: json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":7}}`, uri))},
		{name: "stale version", raw: valid(`{"line":1,"character":5}`, 6)},
		{name: "negative line", raw: valid(`{"line":-1,"character":0}`, 7)},
		{name: "outside document", raw: valid(`{"line":9,"character":0}`, 7)},
		{name: "surrogate split", raw: valid(`{"line":0,"character":15}`, 7)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			result, handled, err := server.handleIDERequest(context.Background(), "textDocument/declaration", tc.raw)
			if result != nil || !handled || !isLSPInvalidParams(err) {
				t.Fatalf("declaration validation result=%#v handled=%v err=%v, want -32602", result, handled, err)
			}
		})
	}
}

func TestLSPDeclarationNavigationCancellationIsTransactional(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-declaration-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let answer = 42\nemit answer\n", 3); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(ctx, "textDocument/declaration", json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":3},"position":{"line":1,"character":5}}`, uri)))
	if result != nil || !handled || !isLSPRequestCancelled(err) {
		t.Fatalf("canceled declaration result=%#v handled=%v err=%v, want -32800 and no result", result, handled, err)
	}
}
