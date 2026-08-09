package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"reflect"
	"testing"
	"time"
)

type lspSymbolCancellationContext struct {
	checks   int
	cancelAt int
}

func (ctx *lspSymbolCancellationContext) Deadline() (time.Time, bool) { return time.Time{}, false }
func (ctx *lspSymbolCancellationContext) Done() <-chan struct{}       { return nil }
func (ctx *lspSymbolCancellationContext) Value(any) any               { return nil }
func (ctx *lspSymbolCancellationContext) Err() error {
	ctx.checks++
	if ctx.checks > ctx.cancelAt {
		return context.Canceled
	}
	return nil
}

func TestLSPSymbolRequestsCancelAfterCollectionWithoutPartialResults(t *testing.T) {
	source, err := os.ReadFile("testdata/lsp/lsp-workspace-symbol-cancellation.osr")
	if err != nil {
		t.Fatal(err)
	}
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-workspace-symbol-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, string(source), 1); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name   string
		method string
		params json.RawMessage
	}{
		{
			name:   "document symbols",
			method: "textDocument/documentSymbol",
			params: json.RawMessage(`{"textDocument":{"uri":"` + uri + `","version":1}}`),
		},
		{
			name:   "workspace symbols",
			method: "workspace/symbol",
			params: json.RawMessage(`{"query":""}`),
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := &lspSymbolCancellationContext{cancelAt: 1 << 30}
			result, handled, err := server.handleIDERequest(probe, tc.method, tc.params)
			if err != nil || !handled || result == nil {
				t.Fatalf("probe request = %#v, handled=%v, err=%v", result, handled, err)
			}

			cancelled := &lspSymbolCancellationContext{cancelAt: probe.checks - 1}
			result, handled, err = server.handleIDERequest(cancelled, tc.method, tc.params)
			if result != nil {
				value := reflect.ValueOf(result)
				if value.Kind() != reflect.Slice || !value.IsNil() {
					t.Fatalf("cancelled request result = %#v", result)
				}
			}
			if !handled {
				t.Fatalf("cancelled request handled=%v", handled)
			}
			var coded interface{ LSPCode() int }
			if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
				t.Fatalf("cancelled request error = %v", err)
			}
		})
	}
}
