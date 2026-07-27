package osiris

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

// BenchmarkTCPLoopbackRoundTrip64B is the low-latency baseline for task 049.
// It exercises Osiris's buffered TCP stream operations, rather than raw Go
// net.Conn calls, over one established loopback connection.
func BenchmarkTCPLoopbackRoundTrip64B(b *testing.B) {
	benchmarkTCPLoopbackRoundTrip(b, 64, DefaultSocketConfig())
}

// BenchmarkTCPLoopbackRoundTrip64KiB is the throughput baseline for task 049.
func BenchmarkTCPLoopbackRoundTrip64KiB(b *testing.B) {
	benchmarkTCPLoopbackRoundTrip(b, 64<<10, DefaultSocketConfig())
}

// BenchmarkTCPLoopbackBulkRoundTrip64KiB compares the explicit bulk-transfer
// override with the default low-latency configuration. It is intentionally a
// separate workload: task 049 must not change defaults without matching-host
// evidence from this comparison.
func BenchmarkTCPLoopbackBulkRoundTrip64KiB(b *testing.B) {
	config := DefaultSocketConfig()
	config.NoDelay = socketBool(false)
	config.WriteBatchBytes = 64 << 10
	benchmarkTCPLoopbackRoundTrip(b, 64<<10, config)
}

func benchmarkTCPLoopbackRoundTrip(b *testing.B, payloadBytes int, config SocketConfig) {
	b.Helper()
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			b.Skip("loopback listeners are forbidden by this sandbox")
		}
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = listener.close() })

	serverConfig := config
	clientConfig := config
	clientConfig.ConnectTimeout = time.Second
	accepted := make(chan *streamValue, 1)
	acceptErr := make(chan error, 1)
	go func() {
		stream, acceptErrValue := listener.acceptContextWithConfig(context.Background(), 1000, serverConfig)
		accepted <- stream
		acceptErr <- acceptErrValue
	}()
	client, err := tcpDialWithConfig(context.Background(), listener.address(), clientConfig)
	if err != nil {
		b.Fatal(err)
	}
	server := <-accepted
	if err := <-acceptErr; err != nil || server == nil {
		b.Fatalf("accept = %v, %v", server, err)
	}

	serverDone := make(chan error, 1)
	go func() {
		defer func() { _, _ = server.close() }()
		for {
			line, readErr := server.readLine()
			if readErr != nil {
				if errors.Is(readErr, io.EOF) {
					serverDone <- nil
				} else {
					serverDone <- readErr
				}
				return
			}
			payload, payloadErr := line.AsString()
			if payloadErr != nil {
				serverDone <- payloadErr
				return
			}
			if _, writeErr := server.writeChunk(stringValue(payload + "\n")); writeErr != nil {
				serverDone <- writeErr
				return
			}
			if _, flushErr := server.flush(); flushErr != nil {
				serverDone <- flushErr
				return
			}
		}
	}()
	b.Cleanup(func() {
		_, _ = client.close()
		if err := <-serverDone; err != nil {
			b.Errorf("echo server: %v", err)
		}
	})

	payload := strings.Repeat("x", payloadBytes)
	b.SetBytes(int64(len(payload) + 1))
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := client.writeChunk(stringValue(payload + "\n")); err != nil {
			b.Fatal(err)
		}
		if _, err := client.flush(); err != nil {
			b.Fatal(err)
		}
		line, err := client.readLine()
		response, responseErr := line.AsString()
		if err != nil || responseErr != nil || response != payload {
			b.Fatalf("loopback response = %q, read = %v, decode = %v", response, err, responseErr)
		}
	}
}

// BenchmarkTCPLoopbackCancelledAccept records the allocation cost of the
// cancellation path without relying on sleeps or a live peer.
func BenchmarkTCPLoopbackCancelledAccept(b *testing.B) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			b.Skip("loopback listeners are forbidden by this sandbox")
		}
		b.Fatal(err)
	}
	b.Cleanup(func() { _ = listener.close() })
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if _, err := listener.acceptContext(ctx, 0); !errors.Is(err, context.Canceled) {
			b.Fatalf("cancelled accept = %v", err)
		}
	}
}
