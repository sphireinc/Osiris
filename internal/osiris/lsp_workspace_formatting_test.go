package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestLSPFormattingCancellationIsTransactional(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-formatting-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let answer=1\nemit answer\n", 7); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(canceled, "textDocument/formatting", json.RawMessage(`{"textDocument":{"uri":"`+uri+`","version":7}}`))
	if result != nil || !handled {
		t.Fatalf("canceled formatting result = %#v, handled=%v", result, handled)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("canceled formatting error = %v, want -32800", err)
	}
	if server.document(uri).text != "let answer=1\nemit answer\n" {
		t.Fatal("canceled formatting mutated the open document")
	}
}
