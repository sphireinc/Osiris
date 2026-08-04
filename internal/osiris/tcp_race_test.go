package osiris

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestTCPListenerCloseOnceIsAtomicAndIdempotent(t *testing.T) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("loopback listeners are forbidden by this sandbox")
		}
		t.Fatal(err)
	}
	const callers = 16
	results := make(chan bool, callers)
	var group sync.WaitGroup
	for i := 0; i < callers; i++ {
		group.Add(1)
		go func() {
			defer group.Done()
			closed, closeErr := listener.closeOnce()
			if closeErr != nil {
				t.Errorf("close error = %v", closeErr)
			}
			results <- closed
		}()
	}
	group.Wait()
	close(results)
	closedCount := 0
	for closed := range results {
		if closed {
			closedCount++
		}
	}
	if closedCount != 1 {
		t.Fatalf("close winners = %d, want 1", closedCount)
	}
}

func TestStreamContextCloseInterruptsTCPFlush(t *testing.T) {
	server, client := net.Pipe()
	defer server.Close()
	stream := &streamValue{
		id:     "pipe-client",
		path:   "pipe-client",
		mode:   "tcp",
		conn:   client,
		writer: bufio.NewWriterSize(client, 1024),
	}
	if _, err := stream.writer.WriteString("pending"); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Millisecond)
	defer cancel()
	started := time.Now()
	_, err := stream.closeContext(ctx)
	if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(err, context.Canceled) {
		t.Fatalf("close error = %v, want context cancellation", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("context close took %s", elapsed)
	}
	if _, err := client.Write([]byte("after-close")); err == nil {
		t.Fatal("closed client connection accepted a write")
	}
}

func TestTCPTransferInitializesLazySourceReader(t *testing.T) {
	left, right := net.Pipe()
	source := &streamValue{conn: left, path: "source", mode: "tcp"}
	destinationBuffer := &bytes.Buffer{}
	destination := &streamValue{
		path:   "destination",
		mode:   "memory",
		writer: bufio.NewWriter(destinationBuffer),
	}
	defer left.Close()
	defer right.Close()

	go func() {
		_, _ = right.Write([]byte("payload"))
		_ = right.Close()
	}()

	copied, err := source.transferToContext(context.Background(), destination, -1)
	if err != nil {
		t.Fatalf("transfer: %v", err)
	}
	if copied != int64(len("payload")) || destinationBuffer.String() != "payload" {
		t.Fatalf("transfer = (%d, %q), want payload", copied, destinationBuffer.String())
	}
}

func TestStreamCloseDetachesOwnedReferences(t *testing.T) {
	file, err := os.CreateTemp(t.TempDir(), "stream-close-")
	if err != nil {
		t.Fatal(err)
	}
	stream := &streamValue{
		file:   file,
		writer: bufio.NewWriter(file),
		reader: bufio.NewReader(file),
		closer: file,
		path:   file.Name(),
	}
	if _, err := stream.close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	stream.mu.Lock()
	defer stream.mu.Unlock()
	if stream.file != nil || stream.reader != nil || stream.writer != nil || stream.closer != nil || stream.scratch != nil {
		t.Fatalf("close retained owned references: %+v", stream)
	}
}
