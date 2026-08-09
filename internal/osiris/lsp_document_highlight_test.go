package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
)

func TestLSPDocumentHighlightProtocolFixtureIsDeterministicAndValidatesRequests(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-document-highlight.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("document-highlight fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	frames := decodeLSPProtocolFrames(t, first)
	expectedIDs := map[int]bool{1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false, 8: false, 9: false}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		if _, ok := expectedIDs[id]; !ok {
			t.Fatalf("unexpected document-highlight response id %d: %#v", id, frame)
		}
		expectedIDs[id] = true
		switch id {
		case 1:
			result, ok := frame["result"].(map[string]any)
			if !ok {
				t.Fatalf("initialize result = %#v", frame["result"])
			}
			capabilities, ok := result["capabilities"].(map[string]any)
			if !ok || capabilities["documentHighlightProvider"] != true {
				t.Fatalf("initialize capabilities = %#v, want documentHighlightProvider=true", result["capabilities"])
			}
		case 2, 3:
			encoded, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var highlights []lspDocumentHighlight
			if err := json.Unmarshal(encoded, &highlights); err != nil {
				t.Fatal(err)
			}
			if len(highlights) == 0 {
				t.Fatalf("document-highlight result %d is empty", id)
			}
			for i := 1; i < len(highlights); i++ {
				if lspPositionLess(highlights[i].Range.Start, highlights[i-1].Range.Start) {
					t.Fatalf("document-highlight result %d is not sorted: %#v", id, highlights)
				}
			}
			if id == 2 {
				if got := highlights[1].Range; got != (lspRange{Start: lspPosition{Line: 3, Character: 12}, End: lspPosition{Line: 3, Character: 18}}) {
					t.Fatalf("UTF-16 answer highlight = %#v", got)
				}
				if highlights[0].Kind != "write" || highlights[1].Kind != "read" {
					t.Fatalf("answer highlight kinds = %#v", highlights)
				}
			}
			if id == 3 && (highlights[0].Kind != "write" || highlights[1].Kind != "write") {
				t.Fatalf("assignment highlight kinds = %#v", highlights)
			}
		case 4, 5, 6, 7, 8:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for id, seen := range expectedIDs {
		if !seen {
			t.Errorf("document-highlight fixture request %d did not receive a response", id)
		}
	}
}

func TestLSPDocumentHighlightsAreDeterministicAndConcurrent(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/document-highlight-concurrent.osr")
	source := "let answer = \"value\"\nvar total = 1\ntotal = total + 1\nemit \"😀\" + answer\nemit total\n"
	server.upsertDocument(uri, source)
	position := lspPosition{Line: 3, Character: 13}
	version := 0
	want, err := server.documentHighlightsContext(context.Background(), uri, position, &version)
	if err != nil {
		t.Fatal(err)
	}
	if len(want) != 2 {
		t.Fatalf("answer highlights = %#v, want definition and use", want)
	}

	const workers = 16
	results := make([][]lspDocumentHighlight, workers)
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = server.documentHighlightsContext(context.Background(), uri, position, &version)
		}(i)
	}
	wg.Wait()
	for i := range results {
		if errs[i] != nil {
			t.Fatalf("concurrent request %d: %v", i, errs[i])
		}
		if got := mustJSONHighlight(t, results[i]); got != mustJSONHighlight(t, want) {
			t.Fatalf("concurrent request %d = %s, want %s", i, got, mustJSONHighlight(t, want))
		}
	}
}

func TestLSPDocumentHighlightsCancellationAndStaleVersionAreTransactional(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/document-highlight-cancellation.osr")
	server.upsertDocument(uri, "let answer = 1\nanswer = answer + 1\nemit answer\n")
	position := lspPosition{Line: 2, Character: 5}
	version := 0

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := server.documentHighlightsContext(canceled, uri, position, &version)
	if result != nil || !isLSPRequestCancellation(err) {
		t.Fatalf("canceled highlight result=%#v err=%v, want -32800 with no result", result, err)
	}
	if _, err := server.documentHighlightsContext(context.Background(), uri, position, intPointer(1)); err == nil || !isLSPInvalidParams(err) {
		t.Fatalf("stale highlight error = %v, want -32602", err)
	}

	checks := &cancelAfterChecks{cancelAfter: 2}
	result, err = server.documentHighlightsContext(checks, uri, position, &version)
	var rpcErr lspRPCError
	if result != nil || !errors.As(err, &rpcErr) || rpcErr.code != -32800 {
		t.Fatalf("late-canceled highlight result=%#v err=%v, want -32800 with no result", result, err)
	}
}

func mustJSONHighlight(t *testing.T, value any) string {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(encoded)
}
