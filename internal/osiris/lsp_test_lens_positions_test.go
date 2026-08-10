package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"strings"
	"testing"
)

func TestLSPTestCodeLensUsesUTF16AndRejectsStaleVersions(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-test-lens-positions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	if first != runLSPProtocolFixture(t, fixture) {
		t.Fatal("test code-lens fixture is nondeterministic")
	}
	frames := decodeLSPProtocolFrames(t, first)
	var sawValid, sawStale bool
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		switch id {
		case 2:
			data, marshalErr := json.Marshal(frame["result"])
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			var lenses []lspCodeLens
			if err := json.Unmarshal(data, &lenses); err != nil {
				t.Fatalf("test code-lens result = %s: %v", data, err)
			}
			if len(lenses) != 1 {
				t.Fatalf("test code-lens result = %#v, want one lens", lenses)
			}
			want := lspRange{Start: lspPosition{Line: 0, Character: 0}, End: lspPosition{Line: 0, Character: 26}}
			if lenses[0].Range != want {
				t.Fatalf("test code-lens range = %#v, want %#v", lenses[0].Range, want)
			}
			if lenses[0].Command == nil || lenses[0].Command.Title != "Run sm😀ke" {
				t.Fatalf("test code-lens command = %#v", lenses[0].Command)
			}
			sawValid = true
		case 3:
			lspProtocolAssertErrorCode(t, frame, -32602)
			sawStale = true
		}
	}
	if !sawValid || !sawStale {
		t.Fatalf("test code-lens fixture frames = %#v", frames)
	}
}

func TestLSPTestCodeLensCancellationIsWireLevel(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-test-lens-positions.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	var chunks [][]byte
	for index, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		if index > 2 {
			break
		}
		chunks = append(chunks, lspFrame(strings.TrimSpace(line)))
	}
	ctx, cancel := context.WithCancel(context.Background())
	reader := &cancelAfterLSPRead{chunks: chunks, cancel: cancel, cancelAfter: 2}
	var output strings.Builder
	server := NewLanguageServer(NewRuntime(), &output, ioDiscardWriter{})
	if err := server.Serve(ctx, reader); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled test code-lens Serve error = %v", err)
	}
	frames := decodeLSPProtocolFrames(t, output.String())
	for _, frame := range frames {
		if id, ok := lspProtocolFrameID(frame); ok && id == 2 {
			lspProtocolAssertErrorCode(t, frame, -32800)
			if _, hasResult := frame["result"]; hasResult {
				t.Fatalf("canceled test code-lens returned a result: %#v", frame)
			}
			return
		}
	}
	t.Fatalf("canceled test code-lens produced no wire response: %#v", frames)
}
