package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestLSPTypeHierarchyCancellationIsTransactional(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-type-hierarchy-cancellation.osr")
	source := `trait Render { func render() string }
struct User { name string }
impl Render for User { func render() string => "ok" }
`
	if err := server.upsertDocumentContext(context.Background(), uri, source, 3); err != nil {
		t.Fatal(err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	item := lspTypeHierarchyItem{Name: "Render", Kind: lspSymbolKind("trait"), Data: "Render"}
	if result, err := server.typeSubtypesContext(canceled, item); result != nil || !isLSPRequestCancelled(err) {
		t.Fatalf("canceled type subtypes = %#v, err=%v", result, err)
	}
	item = lspTypeHierarchyItem{Name: "User", Kind: lspSymbolKind("struct"), Data: "User"}
	if result, err := server.typeSupertypesContext(canceled, item); result != nil || !isLSPRequestCancelled(err) {
		t.Fatalf("canceled type supertypes = %#v, err=%v", result, err)
	}
	result, handled, err := server.handleIDERequest(canceled, "typeHierarchy/prepare", json.RawMessage(`{"textDocument":{"uri":"`+uri+`"},"position":{"line":1,"character":7}}`))
	if result != nil || !handled {
		t.Fatalf("canceled type prepare = %#v, handled=%v, err=%v", result, handled, err)
	}
	var coded interface{ LSPCode() int }
	if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
		t.Fatalf("canceled type prepare error = %v, want -32800", err)
	}
}

func isLSPRequestCancelled(err error) bool {
	var coded interface{ LSPCode() int }
	return errors.As(err, &coded) && coded.LSPCode() == -32800
}
