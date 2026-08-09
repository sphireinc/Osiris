package osiris

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"sync/atomic"
	"testing"
	"time"
)

func TestLSPReferencesProtocolFixtureIsDeterministicAndValidatesPositions(t *testing.T) {
	fixture, err := os.ReadFile("testdata/lsp/lsp-references.jsonl")
	if err != nil {
		t.Fatal(err)
	}
	first := runLSPProtocolFixture(t, fixture)
	second := runLSPProtocolFixture(t, fixture)
	if first != second {
		t.Fatalf("references fixture is not deterministic\nfirst:\n%s\nsecond:\n%s", first, second)
	}

	frames := decodeLSPProtocolFrames(t, first)
	expectedIDs := map[int]bool{1: false, 2: false, 3: false, 4: false, 5: false}
	for _, frame := range frames {
		id, ok := lspProtocolFrameID(frame)
		if !ok {
			continue
		}
		if _, ok := expectedIDs[id]; !ok {
			t.Fatalf("unexpected references response id %d: %#v", id, frame)
		}
		expectedIDs[id] = true
		switch id {
		case 1, 2:
			if _, ok := frame["result"]; !ok {
				t.Fatalf("references request %d returned no result: %#v", id, frame)
			}
		case 3, 4, 5:
			lspProtocolAssertErrorCode(t, frame, -32602)
		}
		if id == 2 {
			encoded, err := json.Marshal(frame["result"])
			if err != nil {
				t.Fatal(err)
			}
			var locations []lspLocation
			if err := json.Unmarshal(encoded, &locations); err != nil {
				t.Fatal(err)
			}
			if len(locations) != 2 {
				t.Fatalf("references result = %#v, want definition and use", locations)
			}
			if locations[0].URI != pathToURI("/tmp/references.osr") || locations[1].URI != pathToURI("/tmp/references.osr") {
				t.Fatalf("references result URIs = %#v", locations)
			}
		}
	}
	for id, seen := range expectedIDs {
		if !seen {
			t.Errorf("references fixture request %d did not receive a response", id)
		}
	}
}

func TestLSPReferencesCancellationDoesNotPublishPartialResults(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), ioDiscardWriter{}, ioDiscardWriter{})
	uri := pathToURI("/tmp/references-cancellation.osr")
	server.upsertDocument(uri, "let answer = 42\nemit answer\n")
	position := lspPosition{Line: 1, Character: 5}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	result, err := server.referencesContext(ctx, uri, position)
	assertLSPRequestCancellation(t, result, err)

	checks := &cancelAfterChecks{cancelAfter: 2}
	result, err = server.referencesContext(checks, uri, position)
	assertLSPRequestCancellation(t, result, err)
}

func assertLSPRequestCancellation(t *testing.T, result []lspLocation, err error) {
	t.Helper()
	if result != nil {
		t.Fatalf("canceled references result = %#v, want nil", result)
	}
	var rpcErr lspRPCError
	if !errors.As(err, &rpcErr) || rpcErr.code != -32800 {
		t.Fatalf("canceled references error = %v, want -32800", err)
	}
}

type cancelAfterChecks struct {
	cancelAfter int32
	checks      int32
}

func (c *cancelAfterChecks) Deadline() (time.Time, bool) { return time.Time{}, false }

func (c *cancelAfterChecks) Done() <-chan struct{} { return nil }

func (c *cancelAfterChecks) Err() error {
	if atomic.AddInt32(&c.checks, 1) > c.cancelAfter {
		return context.Canceled
	}
	return nil
}

func (c *cancelAfterChecks) Value(any) any { return nil }
