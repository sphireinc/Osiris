package osiris

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"testing"
)

func TestLSPCancelRequestCancelsRegisteredRequest(t *testing.T) {
	server := NewLanguageServer(NewRuntime(), io.Discard, io.Discard)
	requestCtx, finish := server.beginLSPRequest(context.Background(), json.RawMessage(`17`))
	defer finish()

	frame := func(payload string) string {
		return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
	}
	var input bytes.Buffer
	input.WriteString(frame(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":17}}`))
	input.WriteString(frame(`{"jsonrpc":"2.0","method":"exit"}`))
	if err := server.Serve(context.Background(), &input); err != nil {
		t.Fatal(err)
	}
	if !errors.Is(requestCtx.Err(), context.Canceled) {
		t.Fatalf("request context error = %v, want context canceled", requestCtx.Err())
	}
}

func TestLSPCancelRequestIsNotificationAndNormalRequestsRemainOrdered(t *testing.T) {
	frame := func(payload string) string {
		return fmt.Sprintf("Content-Length: %d\r\n\r\n%s", len(payload), payload)
	}
	fixture := []byte(
		frame(`{"jsonrpc":"2.0","method":"$/cancelRequest","params":{"id":999}}`) +
			frame(`{"jsonrpc":"2.0","id":1,"method":"osiris/unknown","params":{}}`) +
			frame(`{"jsonrpc":"2.0","id":2,"method":"shutdown"}`) +
			frame(`{"jsonrpc":"2.0","method":"exit"}`),
	)
	var output bytes.Buffer
	server := NewLanguageServer(NewRuntime(), &output, io.Discard)
	if err := server.Serve(context.Background(), bytes.NewReader(fixture)); err != nil {
		t.Fatal(err)
	}
	frames := decodeLSPProtocolFrames(t, output.String())
	if len(frames) != 2 {
		t.Fatalf("response frames = %#v, want only normal request and shutdown responses", frames)
	}
	if id, ok := lspProtocolFrameID(frames[0]); !ok || id != 1 {
		t.Fatalf("first response = %#v, want request id 1", frames[0])
	}
	lspProtocolAssertErrorCode(t, frames[0], -32601)
	if id, ok := lspProtocolFrameID(frames[1]); !ok || id != 2 {
		t.Fatalf("second response = %#v, want shutdown id 2", frames[1])
	}
	if result, ok := frames[1]["result"]; !ok || result != nil {
		t.Fatalf("shutdown response = %#v, want null result", frames[1])
	}
}
