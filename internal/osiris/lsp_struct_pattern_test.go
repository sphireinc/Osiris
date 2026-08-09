package osiris

import (
	"context"
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
