package osiris

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestLSPDocumentChangesApplyInOrderAndCommitAtomically(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-document-change-transaction.osr")
	initial := "let value = 1\nemit value\n"
	if err := server.upsertDocumentContext(context.Background(), uri, initial, 1); err != nil {
		t.Fatal(err)
	}
	changes := []lspContentChange{
		{Range: &lspRange{Start: lspPosition{Line: 0, Character: 12}, End: lspPosition{Line: 0, Character: 13}}, Text: "2"},
		{Range: &lspRange{Start: lspPosition{Line: 1, Character: 5}, End: lspPosition{Line: 1, Character: 10}}, Text: "result"},
	}
	if err := server.applyDocumentChangesContext(context.Background(), uri, 2, changes); err != nil {
		t.Fatal(err)
	}
	want := "let value = 2\nemit result\n"
	committed := server.document(uri)
	if committed == nil || committed.text != want || committed.version != 2 {
		t.Fatalf("ordered didChange state = %#v, want text %q at version 2", committed, want)
	}
	prior := committed
	invalid := []lspContentChange{
		{Range: &lspRange{Start: lspPosition{Line: 0, Character: 12}, End: lspPosition{Line: 0, Character: 13}}, Text: "3"},
		{Range: &lspRange{Start: lspPosition{Line: 1, Character: 100}, End: lspPosition{Line: 1, Character: 101}}, Text: "ignored"},
	}
	if err := server.applyDocumentChangesContext(context.Background(), uri, 3, invalid); err == nil {
		t.Fatal("malformed multi-change batch was accepted")
	}
	if got := server.document(uri); got != prior || got.text != want || got.version != 2 {
		t.Fatalf("malformed batch changed overlay: %#v, want prior state %#v", got, prior)
	}
	if err := server.applyDocumentChangesContext(context.Background(), uri, 2, changes); err == nil || !strings.Contains(err.Error(), "stale document version") {
		t.Fatalf("stale didChange error = %v", err)
	}
}

func TestLSPDocumentChangesHonorCancellationBeforeCommit(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-document-change-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let value = 1\n", 4); err != nil {
		t.Fatal(err)
	}
	prior := server.document(uri)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := server.applyDocumentChangesContext(ctx, uri, 5, []lspContentChange{{
		Range: &lspRange{Start: lspPosition{Line: 0, Character: 12}, End: lspPosition{Line: 0, Character: 13}},
		Text:  "2",
	}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled didChange error = %v", err)
	}
	if got := server.document(uri); got != prior || got.text != "let value = 1\n" || got.version != 4 {
		t.Fatalf("canceled didChange changed overlay: %#v, want prior state %#v", got, prior)
	}
}

func TestLSPDocumentChangesRejectConcurrentStaleSnapshots(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-document-change-concurrent.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let value = 1\n", 1); err != nil {
		t.Fatal(err)
	}
	changes := []lspContentChange{{
		Range: &lspRange{Start: lspPosition{Line: 0, Character: 12}, End: lspPosition{Line: 0, Character: 13}},
		Text:  "2",
	}}
	const workers = 12
	start := make(chan struct{})
	results := make(chan error, workers)
	var group sync.WaitGroup
	group.Add(workers)
	for i := 0; i < workers; i++ {
		go func() {
			defer group.Done()
			<-start
			results <- server.applyDocumentChangesContext(context.Background(), uri, 2, changes)
		}()
	}
	close(start)
	group.Wait()
	close(results)

	successes := 0
	for err := range results {
		if err == nil {
			successes++
			continue
		}
		if !strings.Contains(err.Error(), "document changed while applying didChange") && !strings.Contains(err.Error(), "stale document version") {
			t.Fatalf("unexpected concurrent didChange error = %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent didChange successes = %d, want exactly 1", successes)
	}
	if got := server.document(uri); got == nil || got.text != "let value = 2\n" || got.version != 2 {
		t.Fatalf("concurrent didChange final state = %#v", got)
	}
}

func TestLSPDocumentChangeProtocolFixtureIsDeterministicAndNotificationOnly(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-document-changes.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("document-change fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}
	frames := decodeLSPProtocolFrames(t, first)
	for _, frame := range frames {
		if method, ok := frame["method"].(string); ok && method == "textDocument/didChange" {
			t.Fatalf("didChange notification produced a protocol frame: %#v", frame)
		}
	}
	if got := countLSPProtocolNotifications(frames, "textDocument/publishDiagnostics"); got != 2 {
		t.Fatalf("publishDiagnostics notifications = %d, want didOpen plus one committed didChange", got)
	}
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			result, ok := frame["result"].(map[string]any)
			if !ok || result["kind"] != "full" {
				t.Fatalf("diagnostic result = %#v, want full report", frame)
			}
		}
	}
}
