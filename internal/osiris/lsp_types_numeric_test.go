package osiris

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestLSPSourceBackedTypeReferencesCoverAliasesNewtypesAndTypedSignatures(t *testing.T) {
	source := `type Count = u64
newtype UserID u64
struct User { id UserID, total Count }
func load(id UserID) Count => 1u64
let user = User{id: 7u64, total: 3u64}
emit load(user.id)
`
	program, err := Parse("lsp-types-numeric.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeOnlyForLSPProtocol(t, program)
	if len(analysis.diagnostics) != 0 {
		t.Fatalf("type reference analysis diagnostics = %#v", analysis.diagnostics)
	}

	wantDefinitions := map[string]string{
		"Count":  "type alias",
		"UserID": "newtype",
	}
	for name, kind := range wantDefinitions {
		found := false
		for _, def := range analysis.defs {
			if def.name == name && def.kind == kind {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing %s definition for %q", kind, name)
		}
	}

	seen := map[string]int{}
	for _, ref := range analysis.refs {
		if ref.target == nil || !strings.Contains(wantDefinitions[ref.target.name], ref.target.kind) {
			continue
		}
		if got := spanText(source, ref.span); got != ref.target.name {
			t.Fatalf("reference span = %q, target = %q, span=%+v", got, ref.target.name, ref.span)
		}
		seen[ref.target.name]++
	}
	for _, name := range []string{"Count", "UserID"} {
		if seen[name] < 2 {
			t.Fatalf("references for %q = %d, want struct/signature references", name, seen[name])
		}
	}
}

func TestLSPTypesAndNumericFamiliesProtocolIsDeterministicAndValidatesRequests(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-types-numeric-protocol.osr")
	source := "type Count = u64\n" +
		"newtype UserID u64\n" +
		"trait Render { func render() string }\n" +
		"impl Render for Count { func render() string => \"count\" }\n" +
		"impl Render for UserID { func render() string => \"id\" }\n" +
		"func identity<T>(value T) T => value\n" +
		"let emoji = \"😀\"\n" +
		"let count = identity<u64>(1u64)\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 11); err != nil {
		t.Fatal(err)
	}

	line := "impl Render for UserID { func render() string => \"id\" }"
	lineStart := strings.Index(source, line)
	nameOffset := strings.Index(line, "UserID")
	if lineStart < 0 || nameOffset < 0 {
		t.Fatal("newtype implementation source line is missing")
	}
	position := lspPosition{Line: strings.Count(source[:lineStart], "\n"), Character: utf16Units(line[:nameOffset])}
	raw := []byte(`{"textDocument":{"uri":"` + uri + `","version":11},"position":{"line":` + stringInt(position.Line) + `,"character":` + stringInt(position.Character) + `}}`)
	first, handled, err := server.handleIDERequest(context.Background(), "textDocument/typeDefinition", raw)
	if !handled || err != nil {
		t.Fatalf("valid type-definition request = %#v, handled=%v, err=%v", first, handled, err)
	}
	location, ok := first.(lspLocation)
	if !ok || location.URI != uri || location.Range.Start.Line != 1 {
		t.Fatalf("valid type-definition result = %#v, want UserID declaration on line 1", first)
	}
	second, handled, err := server.handleIDERequest(context.Background(), "textDocument/typeDefinition", raw)
	if !handled || err != nil || !reflect.DeepEqual(first, second) {
		t.Fatalf("repeated type-definition request = %#v, %#v, handled=%v, err=%v", first, second, handled, err)
	}

	for _, tc := range []struct {
		name string
		raw  string
	}{
		{name: "missing position", raw: `{"textDocument":{"uri":"` + uri + `","version":11}}`},
		{name: "stale version", raw: `{"textDocument":{"uri":"` + uri + `","version":10},"position":{"line":4,"character":16}}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			result, handled, err := server.handleIDERequest(context.Background(), "textDocument/typeDefinition", []byte(tc.raw))
			if !isNilLSPResult(result) || !handled || !isLSPInvalidParams(err) {
				t.Fatalf("request result=%#v handled=%v err=%v, want invalid params", result, handled, err)
			}
		})
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	result, handled, err := server.handleIDERequest(canceled, "textDocument/typeDefinition", raw)
	if !isNilLSPResult(result) || !handled || !isLSPRequestCancelled(err) {
		t.Fatalf("canceled type-definition request result=%#v handled=%v err=%v", result, handled, err)
	}

	emojiLine := "let emoji = \"😀\""
	emojiStart := strings.Index(source, emojiLine)
	emojiOffset := strings.Index(emojiLine, "😀")
	if _, err := server.typeDefinitionContext(context.Background(), uri, lspPosition{
		Line: strings.Count(source[:emojiStart], "\n"), Character: utf16Units(emojiLine[:emojiOffset]) + 1,
	}); !isLSPInvalidParams(err) {
		t.Fatalf("surrogate-split type position error = %v, want invalid params", err)
	}
}
