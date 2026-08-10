package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func TestLSPStructPatternNavigationAndNestedBindings(t *testing.T) {
	source := `struct Point { x u64, label string }
let point = Point { x: 7u64, label: "😀" }
emit match point { Point { x: x, label: label } => [x, label], _ => [] }
`
	program, err := Parse("lsp-struct-pattern.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeOnlyForLSPProtocol(t, program)
	if len(analysis.diagnostics) != 0 {
		t.Fatalf("struct-pattern analysis produced diagnostics: %#v", analysis.diagnostics)
	}

	var fieldRefs, bindingDefs, bindingRefs int
	for _, ref := range analysis.refs {
		if ref == nil || ref.target == nil {
			continue
		}
		switch ref.detail {
		case "struct pattern field":
			fieldRefs++
		case "pattern":
			bindingRefs++
		}
	}
	for _, def := range analysis.defs {
		if def != nil && def.kind == "pattern" && (def.name == "x" || def.name == "label") {
			bindingDefs++
		}
	}
	if fieldRefs != 2 {
		t.Fatalf("struct-pattern field references = %d, want two", fieldRefs)
	}
	if bindingDefs != 2 || bindingRefs < 2 {
		t.Fatalf("nested pattern bindings = defs %d refs %d, want two definitions and references", bindingDefs, bindingRefs)
	}

	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-struct-pattern.osr")
	if err := server.upsertDocumentContext(context.Background(), uri, source, 4); err != nil {
		t.Fatal(err)
	}
	line := strings.Split(source, "\n")[2]
	fieldOffset := strings.Index(line, "label:")
	location, err := server.definition(uri, lspPosition{Line: 2, Character: utf16Units(line[:fieldOffset])})
	if err != nil {
		t.Fatal(err)
	}
	field, ok := location.(lspLocation)
	if !ok || field.URI != uri || field.Range.Start.Line != 0 {
		t.Fatalf("struct-pattern field definition = %#v, want Point.label definition", location)
	}
	if field.Range.Start.Character != strings.Index(source, "label string") {
		t.Fatalf("struct-pattern field UTF-16 range = %#v, want declaration column", field.Range)
	}

	bad, err := Parse("lsp-struct-pattern-invalid.osr", `struct Point { x u64 }
let point = Point { x: 7u64 }
emit match point { Point { missing: _ } => 1, _ => 0 }
`)
	if err != nil {
		t.Fatal(err)
	}
	badAnalysis := analyzeOnlyForLSPProtocol(t, bad)
	if len(badAnalysis.diagnostics) != 1 || !strings.Contains(badAnalysis.diagnostics[0].Message, `undefined field "missing"`) {
		t.Fatalf("invalid struct-pattern diagnostics = %#v, want one field diagnostic", badAnalysis.diagnostics)
	}
}

func TestLSPPatternProtocolFixtureIsDeterministicAndValidatesStaleRequests(t *testing.T) {
	uri := "file:///tmp/lsp-pattern-protocol.osr"
	source := `struct Point { x u64, label string }
let point = Point { x: 7u64, label: "😀" }
emit match point { Point { x: x, label: label } => [x, label], _ => [] }
`
	encoded, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	line := strings.Split(source, "\n")[2]
	pointOffset := strings.Index(line, "Point {")
	fieldOffset := strings.Index(line, "label:")
	if pointOffset < 0 || fieldOffset < 0 {
		t.Fatal("pattern fixture lost its source-backed type or field")
	}
	fixture := fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":4,"text":%s}}}
{"jsonrpc":"2.0","id":2,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":4},"position":{"line":2,"character":%d}}}
{"jsonrpc":"2.0","id":3,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":4},"position":{"line":2,"character":%d}}}
{"jsonrpc":"2.0","id":4,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":3},"position":{"line":2,"character":%d}}}
{"jsonrpc":"2.0","id":5,"method":"textDocument/definition","params":{"textDocument":"malformed"}}
{"jsonrpc":"2.0","id":6,"method":"shutdown"}
{"jsonrpc":"2.0","method":"exit"}`,
		uri, encoded, uri, utf16Units(line[:pointOffset]), uri, utf16Units(line[:fieldOffset]), uri, utf16Units(line[:pointOffset]))
	first := runLSPProtocolFixture(t, []byte(fixture))
	second := runLSPProtocolFixture(t, []byte(fixture))
	if first != second {
		t.Fatalf("pattern protocol output is not deterministic:\nfirst=%s\nsecond=%s", first, second)
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
			if frame["error"] != nil || frame["result"] == nil {
				t.Fatalf("pattern definition response %d = %#v", id, frame)
			}
		case 4, 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{2, 3, 4, 5, 6} {
		if !seen[id] {
			t.Fatalf("pattern protocol response %d missing: %#v", id, frames)
		}
	}
}
