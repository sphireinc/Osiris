package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
)

func TestLSPCodeActionValidatesUTF16RangesBeforeProducingEdits(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-code-action-validation.osr")
	source := "import \"std/time\" as time\nimport \"std/math\" as math\nlet answer = \"😀\"\nemit answer\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 4); err != nil {
		t.Fatal(err)
	}

	valid := codeActionRequest(uri, 4, `{"start":{"line":0,"character":0},"end":{"line":1,"character":0}}`)
	result, handled, err := server.handleIDERequest(context.Background(), "textDocument/codeAction", valid)
	if !handled || err != nil {
		t.Fatalf("valid code action request = %#v, handled=%v, err=%v", result, handled, err)
	}
	actions, ok := result.([]lspCodeAction)
	if !ok || len(actions) != 1 || actions[0].Kind != "source.organizeImports" {
		t.Fatalf("valid code action result = %#v, want one organize-import action", result)
	}

	for name, raw := range map[string]json.RawMessage{
		"missing range":   codeActionRequestWithoutRange(uri, 4),
		"reversed range":  codeActionRequest(uri, 4, `{"start":{"line":1,"character":4},"end":{"line":1,"character":1}}`),
		"out of document": codeActionRequest(uri, 4, `{"start":{"line":99,"character":0},"end":{"line":99,"character":0}}`),
	} {
		result, handled, err := server.handleIDERequest(context.Background(), "textDocument/codeAction", raw)
		if !handled {
			t.Fatalf("%s result = %#v, handled=%v, err=%v", name, result, handled, err)
		}
		var coded interface{ LSPCode() int }
		if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
			t.Fatalf("%s error = %v, want -32602", name, err)
		}
	}
}

func TestLSPCodeActionRangeValidationIsConcurrentAndDeterministic(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-code-action-concurrent.osr")
	source := "import \"std/time\" as time\nimport \"std/math\" as math\nlet answer = 1\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 8); err != nil {
		t.Fatal(err)
	}
	raw := codeActionRequest(uri, 8, `{"start":{"line":7,"character":0},"end":{"line":7,"character":0}}`)
	const workers = 8
	const attempts = 32
	var wg sync.WaitGroup
	errs := make(chan error, workers*attempts)
	for worker := 0; worker < workers; worker++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for attempt := 0; attempt < attempts; attempt++ {
				result, handled, err := server.handleIDERequest(context.Background(), "textDocument/codeAction", raw)
				if !handled {
					errs <- fmt.Errorf("invalid code action request was not rejected: result=%#v handled=%v err=%v", result, handled, err)
					continue
				}
				var coded interface{ LSPCode() int }
				if !errors.As(err, &coded) || coded.LSPCode() != -32602 {
					errs <- err
				}
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err == nil {
			t.Fatal("code action validation returned an untyped error")
		}
		t.Fatalf("concurrent code action validation error = %v", err)
	}
}

func TestLSPCodeActionValidationProtocolFixtureIsDeterministic(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-workspace-code-actions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	if first != runLSPProtocolFixture(t, fixture) {
		t.Fatal("code action validation fixture is nondeterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawValid, sawMissing, sawReversed, sawOutOfDocument bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, _ := json.Marshal(frame["result"])
			var actions []lspCodeAction
			if err := json.Unmarshal(data, &actions); err != nil || len(actions) != 1 || actions[0].Kind != "source.organizeImports" {
				t.Fatalf("valid code action fixture result = %s, err=%v", data, err)
			}
			sawValid = true
		case 3, 4, 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
			switch id {
			case 3:
				sawMissing = true
			case 4:
				sawReversed = true
			case 5:
				sawOutOfDocument = true
			}
		}
	}
	if !sawValid || !sawMissing || !sawReversed || !sawOutOfDocument {
		t.Fatalf("code action validation fixture frames = %#v", frames)
	}
}

func TestLSPCodeActionCancellationIsWireLevelAndTransactional(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-workspace-code-actions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var inputChunks [][]byte
	for index, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		if index > 2 {
			break
		}
		inputChunks = append(inputChunks, lspFrame(strings.TrimSpace(line)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: inputChunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server := NewLanguageServer(NewRuntime(), &output, ioDiscardWriter{})
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled code action Serve error = %v", err)
	}
	frames := decodeLSPProtocolFrames(t, output.String())
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			if _, hasResult := frame["result"]; hasResult {
				t.Fatalf("canceled code action returned a result: %#v", frame)
			}
			return
		}
	}
	t.Fatalf("canceled code action produced no wire response: %#v", frames)
}

func codeActionRequest(uri string, version int, r string) json.RawMessage {
	encodedURI, _ := json.Marshal(uri)
	return json.RawMessage(`{"textDocument":{"uri":` + string(encodedURI) + `,"version":` + stringInt(version) + `},"range":` + r + `,"context":{"diagnostics":[]}}`)
}

func codeActionRequestWithoutRange(uri string, version int) json.RawMessage {
	encodedURI, _ := json.Marshal(uri)
	return json.RawMessage(`{"textDocument":{"uri":` + string(encodedURI) + `,"version":` + stringInt(version) + `},"context":{"diagnostics":[]}}`)
}
