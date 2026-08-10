package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

const lspCoreControlFlowSource = `const limit = 2
let seed = 1
func sum(value u64) u64 {
  var current = value
  defer { emit current }
  while current < limit {
    current = current + 1
    break
  }
  for item in [1u64, 2u64] {
    let snapshot = item
    continue
  }
  return current
}
emit { let result = sum(seed); result }
`

func lspCoreFrameErrorCode(frame map[string]any) int {
	errObj, ok := frame["error"].(map[string]any)
	if !ok {
		return 0
	}
	code, _ := errObj["code"].(float64)
	return int(code)
}

func TestLSPCoreSynchronousControlFlowTracksLexicalTargets(t *testing.T) {
	program, err := Parse("lsp-core-control-flow.osr", lspCoreControlFlowSource)
	if err != nil {
		t.Fatal(err)
	}
	analysis := analyzeOnlyForLSPProtocol(t, program)
	if len(analysis.diagnostics) != 0 {
		t.Fatalf("control-flow analysis diagnostics = %#v", analysis.diagnostics)
	}

	wantDefinitions := []string{"sum", "value", "current", "item", "snapshot", "result"}
	for _, name := range wantDefinitions {
		found := false
		for _, def := range analysis.defs {
			if def.name == name {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("missing lexical definition %q", name)
		}
	}

	refsByTarget := make(map[string]int)
	for _, ref := range analysis.refs {
		if ref.target == nil {
			continue
		}
		if got := spanText(lspCoreControlFlowSource, ref.span); got == "" {
			t.Fatalf("reference to %q has an empty source span", ref.target.name)
		}
		refsByTarget[ref.target.name]++
	}
	for _, name := range []string{"sum", "seed", "value", "limit", "current", "item"} {
		if refsByTarget[name] == 0 {
			t.Fatalf("missing reference target %q in %#v", name, refsByTarget)
		}
	}
}

func TestLSPCoreSynchronousControlFlowProtocolIsDeterministicAndValidatesRequests(t *testing.T) {
	uri := pathToURI("/tmp/lsp-core-control-flow.osr")
	encodedSource, err := json.Marshal(lspCoreControlFlowSource)
	if err != nil {
		t.Fatal(err)
	}
	line := "emit { let result = sum(seed); result }"
	lineStart := strings.Index(lspCoreControlFlowSource, line)
	sumOffset := strings.Index(line, "sum")
	if lineStart < 0 || sumOffset < 0 {
		t.Fatal("call-site fixture is missing")
	}
	position := lspPosition{Line: strings.Count(lspCoreControlFlowSource[:lineStart], "\n"), Character: utf16Units(line[:sumOffset])}
	positionJSON := fmt.Sprintf(`{"line":%d,"character":%d}`, position.Line, position.Character)
	fixture := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}
` + fmt.Sprintf(`{"jsonrpc":"2.0","method":"textDocument/didOpen","params":{"textDocument":{"uri":%q,"languageId":"osiris","version":5,"text":%s}}}
`, uri, encodedSource) +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":5},"position":%s}}
`, uri, positionJSON) +
		`{"jsonrpc":"2.0","id":3,"method":"textDocument/definition","params":{"textDocument":"malformed"}}
` +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":4,"method":"textDocument/definition","params":{"textDocument":{"uri":%q,"version":4},"position":%s}}
`, uri, positionJSON) +
 		fmt.Sprintf(`{"jsonrpc":"2.0","id":6,"method":"textDocument/hover","params":{"textDocument":{"uri":%q,"version":5},"position":%s}}
`, uri, positionJSON) +
		fmt.Sprintf(`{"jsonrpc":"2.0","id":7,"method":"textDocument/references","params":{"textDocument":{"uri":%q,"version":5},"position":%s}}
`, uri, positionJSON) +
		`{"jsonrpc":"2.0","id":5,"method":"shutdown","params":{}}
{"jsonrpc":"2.0","method":"exit"}`
	frames := decodeLSPProtocolFrames(t, runLSPProtocolFixture(t, []byte(fixture)))
	var firstResult json.RawMessage
	var sawMalformed, sawStale, sawHover, sawReferences bool
	for _, frame := range frames {
		id, _ := frame["id"].(float64)
		switch int(id) {
		case 2:
			firstResult, err = json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
		case 3:
			sawMalformed = lspCoreFrameErrorCode(frame) == -32602
		case 4:
			sawStale = lspCoreFrameErrorCode(frame) == -32602
		case 6:
			hover, marshalErr := json.Marshal(frame["result"])
			if marshalErr == nil && len(hover) > 2 && !strings.Contains(string(hover), "null") {
				sawHover = true
			}
		case 7:
			var references []lspLocation
			encoded, marshalErr := json.Marshal(frame["result"])
			if marshalErr == nil && json.Unmarshal(encoded, &references) == nil && len(references) >= 2 {
				sawReferences = true
			}
		}
	}
	if len(firstResult) == 0 {
		t.Fatal("definition protocol response is missing")
	}
	var location lspLocation
	if err := json.Unmarshal(firstResult, &location); err != nil || location.URI != uri || location.Range.Start.Line != 2 {
		t.Fatalf("definition protocol response = %s, want sum declaration on line 2: %v", firstResult, err)
	}
	// A second identical request is issued through the direct API as well as
	// the framed fixture above so deterministic source-backed identity is tested
	// independently of JSON map ordering.
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	if err := server.upsertDocumentContext(context.Background(), uri, lspCoreControlFlowSource, 5); err != nil {
		t.Fatal(err)
	}
	first, err := server.definitionContext(context.Background(), uri, position)
	if err != nil {
		t.Fatal(err)
	}
	second, err := server.definitionContext(context.Background(), uri, position)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatalf("definition response changed between identical requests: %#v vs %#v", first, second)
	}
	var secondLocation lspLocation
	secondResult, err := json.Marshal(second)
	if err != nil || json.Unmarshal(secondResult, &secondLocation) != nil || !reflect.DeepEqual(location, secondLocation) {
		t.Fatalf("definition response value changed: first=%#v second=%#v (%v)", location, secondLocation, err)
	}
	if !sawMalformed || !sawStale || !sawHover || !sawReferences {
		t.Fatalf("core-control-flow protocol frames = %#v; malformed=%v stale=%v hover=%v references=%v", frames, sawMalformed, sawStale, sawHover, sawReferences)
	}

	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := server.definitionContext(canceled, uri, position); !isLSPRequestCancelled(err) {
		t.Fatalf("cancelled definition error = %v, want request cancellation", err)
	}
}
