package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"testing"
)

func TestLSPCallHierarchyFiltersByDefinitionAndPreservesUTF16Ranges(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/call-hierarchy.osr")
	source := "func leaf(value string) string => value\n" +
		"func side(value string) string => value\n" +
		"func caller() string => cat(\"😀\", leaf(\"x\"))\n" +
		"func unrelated() string => side(\"y\")\n" +
		"emit caller()\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 1); err != nil {
		t.Fatal(err)
	}

	callerRaw := json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":1},"position":{"line":2,"character":5}}`)
	callerResult, handled, err := server.handleIDERequest(context.Background(), "callHierarchy/prepare", callerRaw)
	if err != nil || !handled {
		t.Fatalf("prepare caller = %#v, handled=%v, err=%v", callerResult, handled, err)
	}
	callers, ok := callerResult.([]lspCallHierarchyItem)
	if !ok || len(callers) != 1 || callers[0].Name != "caller" {
		t.Fatalf("prepare caller = %#v, want one caller item", callerResult)
	}
	if callers[0].Range.Start != (lspPosition{Line: 2, Character: 5}) || callers[0].Range.End != (lspPosition{Line: 2, Character: 11}) {
		t.Fatalf("caller range = %#v, want [2:5, 2:11]", callers[0].Range)
	}
	for _, params := range []json.RawMessage{
		json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":0},"position":{"line":2,"character":5}}`),
		json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":1}}`),
	} {
		result, handled, err := server.handleIDERequest(context.Background(), "callHierarchy/prepare", params)
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("invalid prepare request = result %#v, handled=%v, err=%v", result, handled, err)
		}
	}

	leafResult, handled, err := server.handleIDERequest(context.Background(), "callHierarchy/prepare", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":1},"position":{"line":0,"character":5}}`))
	if err != nil || !handled {
		t.Fatalf("prepare leaf = %#v, handled=%v, err=%v", leafResult, handled, err)
	}
	leaves, ok := leafResult.([]lspCallHierarchyItem)
	if !ok || len(leaves) != 1 || leaves[0].Name != "leaf" {
		t.Fatalf("prepare leaf = %#v, want one leaf item", leafResult)
	}

	itemJSON, err := json.Marshal(leaves[0])
	if err != nil {
		t.Fatal(err)
	}
	incomingResult, handled, err := server.handleIDERequest(context.Background(), "callHierarchy/incomingCalls", json.RawMessage(`{"item":`+string(itemJSON)+`}`))
	if err != nil || !handled {
		t.Fatalf("incoming calls = %#v, handled=%v, err=%v", incomingResult, handled, err)
	}
	incoming, ok := incomingResult.([]lspCallHierarchyIncoming)
	if !ok || len(incoming) != 1 || incoming[0].From.Name != "caller" || len(incoming[0].FromRanges) != 1 {
		t.Fatalf("incoming calls = %#v, want only caller", incomingResult)
	}
	wantCallRange := lspRange{Start: lspPosition{Line: 2, Character: 34}, End: lspPosition{Line: 2, Character: 38}}
	if incoming[0].FromRanges[0] != wantCallRange {
		t.Fatalf("incoming range = %#v, want %#v", incoming[0].FromRanges[0], wantCallRange)
	}

	outgoingResult, handled, err := server.handleIDERequest(context.Background(), "callHierarchy/outgoingCalls", json.RawMessage(`{"item":`+string(mustLSPJSON(t, callers[0]))+`}`))
	if err != nil || !handled {
		t.Fatalf("outgoing calls = %#v, handled=%v, err=%v", outgoingResult, handled, err)
	}
	outgoing, ok := outgoingResult.([]lspCallHierarchyOutgoing)
	if !ok || len(outgoing) != 1 || outgoing[0].To.Name != "leaf" || len(outgoing[0].FromRanges) != 1 {
		t.Fatalf("outgoing calls = %#v, want only leaf from caller", outgoingResult)
	}
	if outgoing[0].FromRanges[0] != wantCallRange {
		t.Fatalf("outgoing range = %#v, want %#v", outgoing[0].FromRanges[0], wantCallRange)
	}

	for _, method := range []string{"callHierarchy/incomingCalls", "callHierarchy/outgoingCalls"} {
		result, handled, err := server.handleIDERequest(context.Background(), method, json.RawMessage(`{"item":{}}`))
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("malformed %s = result %#v, handled=%v, err=%v", method, result, handled, err)
		}
	}

	cancelled, cancel := context.WithCancel(context.Background())
	cancel()
	for _, method := range []string{"callHierarchy/prepare", "callHierarchy/incomingCalls", "callHierarchy/outgoingCalls"} {
		params := callerRaw
		if method != "callHierarchy/prepare" {
			params = json.RawMessage(`{"item":` + string(itemJSON) + `}`)
		}
		result, handled, err := server.handleIDERequest(cancelled, method, params)
		var coded interface{ LSPCode() int }
		if result != nil || !handled || !errors.As(err, &coded) || coded.LSPCode() != -32800 {
			t.Fatalf("cancelled %s = result %#v, handled=%v, err=%v", method, result, handled, err)
		}
	}
}

func TestLSPCallHierarchyProtocolFixtureIsDeterministicAndExact(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-call-hierarchy.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatal("call-hierarchy protocol output was not deterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	if len(frames) < 6 {
		t.Fatalf("call-hierarchy response frames = %d, want initialize, prepare, incoming, outgoing, malformed, shutdown", len(frames))
	}
	byID := make(map[int]map[string]any)
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok {
			byID[id] = frame
		}
	}
	for _, id := range []int{2, 3, 4} {
		if _, ok := byID[id]["result"].([]any); !ok {
			t.Fatalf("response %d = %#v, want array result", id, byID[id])
		}
	}
	lspProtocolAssertErrorCode(t, byID[5], -32602)
}

func mustLSPJSON(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}
