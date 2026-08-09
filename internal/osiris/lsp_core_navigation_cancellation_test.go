package osiris

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"reflect"
	"testing"
)

func TestLSPCoreNavigationContextResultsAreDeterministic(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/core-navigation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let answer = 42\nemit answer\n", 1); err != nil {
		t.Fatal(err)
	}
	position := lspPosition{Line: 1, Character: 5}
	cases := []struct {
		name string
		run  func(context.Context) (any, error)
	}{
		{name: "hover", run: func(ctx context.Context) (any, error) {
			return server.hoverContext(ctx, uri, position)
		}},
		{name: "definition", run: func(ctx context.Context) (any, error) {
			return server.definitionContext(ctx, uri, position)
		}},
		{name: "completion", run: func(ctx context.Context) (any, error) {
			return server.completionContext(ctx, uri, position)
		}},
		{name: "prepare rename", run: func(ctx context.Context) (any, error) {
			return server.prepareRenameContext(ctx, uri, position)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			first, err := tc.run(context.Background())
			if err != nil {
				t.Fatalf("first request: %v", err)
			}
			second, err := tc.run(context.Background())
			if err != nil {
				t.Fatalf("second request: %v", err)
			}
			if !reflect.DeepEqual(first, second) {
				t.Fatalf("request result changed between identical calls:\nfirst=%#v\nsecond=%#v", first, second)
			}
			if isNilLSPResult(first) {
				t.Fatalf("successful %s request returned no result", tc.name)
			}
		})
	}
}

func TestLSPCoreNavigationCancellationDoesNotPublishPartialResults(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/core-navigation-cancellation.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "let answer = 42\nemit answer\n", 1); err != nil {
		t.Fatal(err)
	}
	position := lspPosition{Line: 1, Character: 5}
	cases := []struct {
		name string
		run  func(context.Context) (any, error)
	}{
		{name: "hover", run: func(ctx context.Context) (any, error) {
			return server.hoverContext(ctx, uri, position)
		}},
		{name: "definition", run: func(ctx context.Context) (any, error) {
			return server.definitionContext(ctx, uri, position)
		}},
		{name: "completion", run: func(ctx context.Context) (any, error) {
			return server.completionContext(ctx, uri, position)
		}},
		{name: "prepare rename", run: func(ctx context.Context) (any, error) {
			return server.prepareRenameContext(ctx, uri, position)
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			probe := &cancelAfterChecks{cancelAfter: 1 << 30}
			if result, err := tc.run(probe); err != nil || isNilLSPResult(result) {
				t.Fatalf("probe request = %#v, checks=%d, err=%v", result, probe.checks, err)
			}
			cancelled := &cancelAfterChecks{cancelAfter: probe.checks - 1}
			result, err := tc.run(cancelled)
			if !isNilLSPResult(result) {
				t.Fatalf("cancelled %s result = %#v", tc.name, result)
			}
			var coded interface{ LSPCode() int }
			if !errors.As(err, &coded) || coded.LSPCode() != -32800 {
				t.Fatalf("cancelled %s error = %v, want -32800", tc.name, err)
			}
		})
	}
}

func TestLSPCoreNavigationWireDispatchHonorsCancellation(t *testing.T) {
	uri := pathToURI("/tmp/core-navigation-wire-cancellation.osr")
	position := `{"line":1,"character":5}`
	methods := []string{
		"textDocument/hover",
		"textDocument/definition",
		"textDocument/completion",
		"textDocument/prepareRename",
	}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			server := NewLanguageServer(NewRuntime(), &bytes.Buffer{}, io.Discard)
			if err := server.upsertDocumentContext(context.Background(), uri, "let answer = 42\nemit answer\n", 1); err != nil {
				t.Fatal(err)
			}
			payload := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":%q,"params":{"textDocument":{"uri":%q,"version":1},"position":%s}}`, method, uri, position)
			var input bytes.Buffer
			fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(payload), payload)
			var output bytes.Buffer
			server.out = &output
			if err := server.Serve(&cancelAfterChecks{cancelAfter: 1}, &input); err != nil {
				t.Fatalf("Serve: %v", err)
			}
			frames := decodeLSPProtocolFrames(t, output.String())
			if len(frames) != 1 {
				t.Fatalf("cancellation response frames = %#v", frames)
			}
			lspProtocolAssertErrorCode(t, frames[0], -32800)
		})
	}
}

func isNilLSPResult(result any) bool {
	if result == nil {
		return true
	}
	v := reflect.ValueOf(result)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
