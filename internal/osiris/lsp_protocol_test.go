package osiris

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"testing"
)

func TestLSPProtocolFixtureAdvertisesAndExercisesAllLSPMethods(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-protocol.jsonl")
	if err != nil {
		t.Fatal(err)
	}

	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("protocol fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	frames := decodeLSPProtocolFrames(t, first)
	expectedIDs := map[int]bool{}
	for id := 1; id <= 36; id++ {
		if id != 2 {
			expectedIDs[id] = false
		}
	}
	capabilityKeys := []string{
		"hoverProvider", "declarationProvider", "definitionProvider", "typeDefinitionProvider", "implementationProvider",
		"referencesProvider", "completionProvider", "signatureHelpProvider", "renameProvider",
		"documentSymbolProvider", "workspaceSymbolProvider", "semanticTokensProvider", "inlayHintProvider",
		"foldingRangeProvider", "selectionRangeProvider", "callHierarchyProvider", "typeHierarchyProvider",
		"codeActionProvider", "documentFormattingProvider", "diagnosticProvider", "codeLensProvider",
	}
	for _, frame := range frames {
		id, hasID := lspProtocolFrameID(frame)
		if !hasID {
			continue
		}
		if _, ok := expectedIDs[id]; !ok {
			t.Fatalf("unexpected response id %d: %#v", id, frame)
		}
		expectedIDs[id] = true
		if id == 29 {
			lspProtocolAssertErrorCode(t, frame, -32602)
			continue
		}
		if id == 30 {
			lspProtocolAssertErrorCode(t, frame, -32601)
			continue
		}
		if id >= 32 && id <= 35 {
			lspProtocolAssertErrorCode(t, frame, -32602)
			continue
		}
		if _, ok := frame["result"]; !ok {
			t.Fatalf("request %d returned no result: %#v", id, frame)
		}
		if id == 1 {
			result, ok := frame["result"].(map[string]any)
			if !ok {
				t.Fatalf("initialize result = %#v", frame["result"])
			}
			capabilities, ok := result["capabilities"].(map[string]any)
			if !ok {
				t.Fatalf("initialize capabilities = %#v", result["capabilities"])
			}
			for _, key := range capabilityKeys {
				if _, ok := capabilities[key]; !ok {
					t.Fatalf("initialize capabilities missing %q: %#v", key, capabilities)
				}
			}
		}
	}
	for id, seen := range expectedIDs {
		if !seen {
			t.Errorf("fixture request %d did not receive a response", id)
		}
	}
	if got := countLSPProtocolNotifications(frames, "textDocument/publishDiagnostics"); got != 1 {
		t.Fatalf("publishDiagnostics notifications = %d, want 1", got)
	}
}

func TestLPSSignatureHelpFixtureIsDeterministicAndValidatesRequests(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-signature-help.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("signature-help fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	frames := decodeLSPProtocolFrames(t, first)
	expectedIDs := map[int]bool{1: false, 2: false, 3: false, 4: false, 5: false, 6: false, 7: false, 8: false}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		if _, ok := expectedIDs[id]; !ok {
			t.Fatalf("unexpected signature-help response id %d: %#v", id, frame)
		}
		expectedIDs[id] = true
		switch id {
		case 1, 2, 7, 8:
			if _, ok := frame["result"]; !ok {
				t.Fatalf("signature-help request %d returned no result: %#v", id, frame)
			}
		case 3, 4, 5, 6:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
		if id == 2 {
			encoded, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var help lspSignatureHelp
			if err := json.Unmarshal(encoded, &help); err != nil {
				t.Fatal(err)
			}
			if len(help.Signatures) != 1 || help.ActiveParameter != 1 || len(help.Signatures[0].Parameters) != 2 {
				t.Fatalf("signature-help fixture result = %#v", help)
			}
		}
		if id == 7 {
			encoded, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var help lspSignatureHelp
			if err := json.Unmarshal(encoded, &help); err != nil {
				t.Fatal(err)
			}
			if len(help.Signatures) != 1 || help.ActiveParameter != 0 {
				t.Fatalf("opening-parenthesis signature-help result = %#v", help)
			}
		}
	}
	for id, seen := range expectedIDs {
		if !seen {
			t.Errorf("signature-help fixture request %d did not receive a response", id)
		}
	}
}

func runLSPProtocolFixture(t *testing.T, fixture []byte) string {
	t.Helper()
	var input bytes.Buffer
	for _, line := range strings.Split(strings.TrimSpace(string(fixture)), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		var payload map[string]any
		if err := json.Unmarshal([]byte(line), &payload); err != nil {
			t.Fatalf("invalid protocol fixture line %q: %v", line, err)
		}
		encoded := []byte(line)
		fmt.Fprintf(&input, "Content-Length: %d\r\n\r\n%s", len(encoded), encoded)
	}
	var output bytes.Buffer
	server := NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(context.Background(), &input); err != nil {
		t.Fatalf("Serve fixture: %v", err)
	}
	return output.String()
}

func decodeLSPProtocolFrames(t *testing.T, output string) []map[string]any {
	t.Helper()
	r := bufio.NewReader(strings.NewReader(output))
	var frames []map[string]any
	for {
		length := -1
		for {
			line, err := r.ReadString('\n')
			if err == io.EOF && line == "" {
				return frames
			}
			if err != nil && err != io.EOF {
				t.Fatalf("read response header: %v", err)
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			name, value, ok := strings.Cut(line, ":")
			if ok && strings.EqualFold(strings.TrimSpace(name), "content-length") {
				length, err = strconv.Atoi(strings.TrimSpace(value))
				if err != nil {
					t.Fatalf("response Content-Length %q: %v", value, err)
				}
			}
			if err == io.EOF {
				t.Fatalf("truncated response headers")
			}
		}
		if length < 0 {
			t.Fatalf("response missing Content-Length")
		}
		payload := make([]byte, length)
		if _, err := io.ReadFull(r, payload); err != nil {
			t.Fatalf("read response payload: %v", err)
		}
		var frame map[string]any
		if err := json.Unmarshal(payload, &frame); err != nil {
			t.Fatalf("decode response payload %q: %v", payload, err)
		}
		frames = append(frames, frame)
	}
}

func lspProtocolFrameID(frame map[string]any) (int, bool) {
	raw, ok := frame["id"]
	if !ok || raw == nil {
		return 0, false
	}
	switch value := raw.(type) {
	case float64:
		return int(value), true
	case json.Number:
		id, err := strconv.Atoi(value.String())
		return id, err == nil
	default:
		return 0, false
	}
}

func lspProtocolAssertErrorCode(t *testing.T, frame map[string]any, want int) {
	t.Helper()
	errorObject, ok := frame["error"].(map[string]any)
	if !ok {
		t.Fatalf("frame = %#v, want JSON-RPC error %d", frame, want)
	}
	code, ok := errorObject["code"].(float64)
	if !ok || int(code) != want {
		t.Fatalf("error = %#v, want code %d", errorObject, want)
	}
}

func countLSPProtocolNotifications(frames []map[string]any, method string) int {
	count := 0
	for _, frame := range frames {
		if frame["method"] == method {
			count++
		}
	}
	return count
}
