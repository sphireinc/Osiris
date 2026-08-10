package osiris

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestLSPTestResultsProtocolFixtureIsDeterministicAndValidated(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-test-results.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if normalizeLSPTestResultDurations(t, first) != normalizeLSPTestResultDurations(t, second) {
		t.Fatalf("test-results fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
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
		case 2:
			payload, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var results []lspTestResult
			if err := json.Unmarshal(payload, &results); err != nil {
				t.Fatal(err)
			}
			if len(results) != 2 || results[0].Name != "passing" || results[0].Status != "ok" || !results[0].Passed || results[1].Name != "skipped" || results[1].Status != "skip" || !results[1].Skipped {
				t.Fatalf("test-results response = %#v", results)
			}
			if results[0].Location.URI != "file:///tmp/lsp-test-results-fixture.test.osr" || results[0].Location.Range.Start.Line != 0 {
				t.Fatalf("test-results location = %#v", results[0].Location)
			}
		case 3, 4:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
	}
	for _, id := range []int{1, 2, 3, 4, 5} {
		if !seen[id] {
			t.Errorf("test-results fixture response %d missing", id)
		}
	}
}

func normalizeLSPTestResultDurations(t *testing.T, output string) string {
	t.Helper()
	frames := decodeLSPProtocolFrames(t, output)
	for _, frame := range frames {
		results, ok := frame["result"].([]any)
		if !ok {
			continue
		}
		for _, raw := range results {
			result, ok := raw.(map[string]any)
			if ok {
				delete(result, "durationNS")
			}
		}
	}
	normalized, err := json.Marshal(frames)
	if err != nil {
		t.Fatal(err)
	}
	return string(normalized)
}

func TestLSPTestResultsExecutesOverlayWithStableFailureProjection(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/lsp-test-results.test.osr")
	source := "// osiris:test name=passing\n" +
		"// osiris:expect \"3u64\"\n" +
		"emit 1 + 2\n\n" +
		"// osiris:test name=skipped skip=\"not today\"\n" +
		"emit missing_name\n"
	if err := server.upsertDocumentContext(context.Background(), uri, source, 9); err != nil {
		t.Fatal(err)
	}
	raw := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":9},"backend":"interpreter"}`, uri))
	result, handled, err := server.handleIDERequest(context.Background(), "osiris/testResults", raw)
	if err != nil || !handled {
		t.Fatalf("test result request = %#v, handled=%v, err=%v", result, handled, err)
	}
	results, ok := result.([]lspTestResult)
	if !ok || len(results) != 2 {
		t.Fatalf("test results = %#v, want two results", result)
	}
	if results[0].Name != "passing" || results[0].Status != "ok" || !results[0].Passed || len(results[0].Assertions) != 1 {
		t.Fatalf("passing result = %#v", results[0])
	}
	if results[1].Name != "skipped" || results[1].Status != "skip" || !results[1].Skipped || !results[1].Passed {
		t.Fatalf("skipped result = %#v", results[1])
	}
	if results[0].Location.URI != uri || results[0].Location.Range.Start.Line != 0 {
		t.Fatalf("result location = %#v", results[0].Location)
	}
	wire, err := json.Marshal(results)
	if err != nil || len(wire) == 0 {
		t.Fatalf("result wire = %s, err=%v", wire, err)
	}

	stale := json.RawMessage(fmt.Sprintf(`{"textDocument":{"uri":%q,"version":8}}`, uri))
	if value, handled, err := server.handleIDERequest(context.Background(), "osiris/testResults", stale); value != nil || !handled || !isLSPInvalidParams(err) {
		t.Fatalf("stale result = %#v, handled=%v, err=%v", value, handled, err)
	}
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if value, handled, err := server.handleIDERequest(canceled, "osiris/testResults", raw); value != nil || !handled || !isLSPRequestCancelled(err) {
		t.Fatalf("cancelled result = %#v, handled=%v, err=%v", value, handled, err)
	}
}
