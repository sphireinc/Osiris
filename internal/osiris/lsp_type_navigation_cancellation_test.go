package osiris

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestLSPTypeNavigationContextResultsAreDeterministicAndUTF16Safe(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/type-navigation.osr")
	source := "let emoji = \"😀\"\n" +
		"trait Render { func render() string }\n" +
		"struct User { name string }\n" +
		"impl Render for User { func render() string => \"ok\" }\n" +
		"func use(value User) User => value\n" +
		"func invoke(value User) string => value.render()\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 7); err != nil {
		t.Fatal(err)
	}

	implLine := "impl Render for User { func render() string => \"ok\" }"
	typePosition := lspPosition{Line: 3, Character: utf16Units(implLine[:strings.Index(implLine, "User")])}
	traitLine := "trait Render { func render() string }"
	implementationPosition := lspPosition{Line: 1, Character: utf16Units(traitLine[:strings.Index(traitLine, "render")])}

	firstType, err := server.typeDefinitionContext(context.Background(), uri, typePosition)
	if err != nil {
		t.Fatalf("type definition: %v", err)
	}
	secondType, err := server.typeDefinitionContext(context.Background(), uri, typePosition)
	if err != nil {
		t.Fatalf("second type definition: %v", err)
	}
	if !reflect.DeepEqual(firstType, secondType) {
		t.Fatalf("type definition changed between identical requests: first=%#v second=%#v", firstType, secondType)
	}
	location, ok := firstType.(lspLocation)
	if !ok || location.URI != uri {
		t.Fatalf("type definition = %#v, want a location in %s", firstType, uri)
	}

	firstImplementations, err := server.implementationsContext(context.Background(), uri, implementationPosition)
	if err != nil {
		t.Fatalf("implementations: %v", err)
	}
	secondImplementations, err := server.implementationsContext(context.Background(), uri, implementationPosition)
	if err != nil {
		t.Fatalf("second implementations: %v", err)
	}
	if !reflect.DeepEqual(firstImplementations, secondImplementations) {
		t.Fatalf("implementations changed between identical requests: first=%#v second=%#v", firstImplementations, secondImplementations)
	}
	if len(firstImplementations) == 0 {
		t.Fatalf("implementations = %#v, want at least one implementation", firstImplementations)
	}
	for i := 1; i < len(firstImplementations); i++ {
		if firstImplementations[i-1].URI > firstImplementations[i].URI ||
			(firstImplementations[i-1].URI == firstImplementations[i].URI && firstImplementations[i-1].Range.Start.Line > firstImplementations[i].Range.Start.Line) {
			t.Fatalf("implementations are not deterministically ordered: %#v", firstImplementations)
		}
	}

	emojiOffset := strings.Index(source, "😀")
	if _, ok := lspUTF16Offset(source, lspPosition{Line: 0, Character: utf16Units(source[:emojiOffset]) + 1}); ok {
		t.Fatal("UTF-16 position inside a surrogate pair was accepted")
	}
	if _, err := server.typeDefinitionContext(context.Background(), uri, lspPosition{Line: 0, Character: utf16Units(source[:emojiOffset]) + 1}); !isLSPInvalidParams(err) {
		t.Fatalf("invalid UTF-16 type position error = %v", err)
	}
	if _, err := server.implementationsContext(context.Background(), uri, lspPosition{Line: 0, Character: utf16Units(source[:emojiOffset]) + 1}); !isLSPInvalidParams(err) {
		t.Fatalf("invalid UTF-16 implementation position error = %v", err)
	}
}

func TestLSPTypeNavigationCancellationIsTransactional(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/type-navigation-cancellation.osr")
	source := "trait Render { func render() string }\n" +
		"struct User { name string }\n" +
		"impl Render for User { func render() string => \"ok\" }\n" +
		"func invoke(value User) string => value.render()\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 3); err != nil {
		t.Fatal(err)
	}
	position := lspPosition{Line: 0, Character: utf16Units("trait Render { func ")}
	cases := []struct {
		name string
		run  func(context.Context) (any, error)
	}{
		{name: "type definition", run: func(ctx context.Context) (any, error) {
			return server.typeDefinitionContext(ctx, uri, lspPosition{Line: 2, Character: utf16Units("impl Render for ")})
		}},
		{name: "implementations", run: func(ctx context.Context) (any, error) {
			return server.implementationsContext(ctx, uri, position)
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			result, err := tc.run(canceled)
			if !isNilLSPResult(result) || !isLSPRequestCancelled(err) {
				t.Fatalf("pre-cancelled result=%#v err=%v", result, err)
			}

			probe := &cancelAfterChecks{cancelAfter: 1 << 30}
			result, err = tc.run(probe)
			if err != nil || isNilLSPResult(result) {
				t.Fatalf("probe result=%#v checks=%d err=%v", result, probe.checks, err)
			}
			cancelAfter := probe.checks - 1
			if cancelAfter < 1 {
				cancelAfter = 1
			}
			cancelled := &cancelAfterChecks{cancelAfter: cancelAfter}
			result, err = tc.run(cancelled)
			if !isNilLSPResult(result) || !isLSPRequestCancelled(err) {
				t.Fatalf("mid-request result=%#v checks=%d err=%v", result, cancelled.checks, err)
			}
		})
	}
}

func TestLSPTypeNavigationRequestValidationAndContextDispatch(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/type-navigation-dispatch.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, "struct User { name string }\nfunc use(value User) User => value\n", 5); err != nil {
		t.Fatal(err)
	}
	for _, method := range []string{"textDocument/typeDefinition", "textDocument/implementation"} {
		t.Run(method, func(t *testing.T) {
			position := `{"line":1,"character":16}`
			if method == "textDocument/implementation" {
				position = `{"line":0,"character":20}`
			}
			raw := []byte(`{"textDocument":{"uri":"` + uri + `","version":5},"position":` + position + `}`)
			result, handled, err := server.handleIDERequest(context.Background(), method, raw)
			if !handled || err != nil {
				t.Fatalf("valid dispatch result=%#v handled=%v err=%v", result, handled, err)
			}

			result, handled, err = server.handleIDERequest(context.Background(), method, []byte(`{"textDocument":{"uri":"`+uri+`","version":5}}`))
			if !isNilLSPResult(result) || !handled || !isLSPInvalidParams(err) {
				t.Fatalf("missing position result=%#v handled=%v err=%v", result, handled, err)
			}
			result, handled, err = server.handleIDERequest(context.Background(), method, []byte(`{"textDocument":{"uri":"`+uri+`","version":4},"position":`+position+`}`))
			if !isNilLSPResult(result) || !handled || !isLSPInvalidParams(err) {
				t.Fatalf("stale request result=%#v handled=%v err=%v", result, handled, err)
			}
			canceled, cancel := context.WithCancel(context.Background())
			cancel()
			result, handled, err = server.handleIDERequest(canceled, method, []byte(`{"textDocument":{"uri":"`+uri+`","version":5},"position":`+position+`}`))
			if !isNilLSPResult(result) || !handled || !isLSPRequestCancelled(err) {
				t.Fatalf("cancelled request result=%#v handled=%v err=%v", result, handled, err)
			}
		})
	}
}

func isLSPInvalidParams(err error) bool {
	var coded interface{ LSPCode() int }
	return errors.As(err, &coded) && coded.LSPCode() == -32602
}
