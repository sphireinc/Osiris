package osiris

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSocketConfigDefaultsAndValidation(t *testing.T) {
	defaults := DefaultSocketConfig()
	if defaults.NoDelay == nil || !*defaults.NoDelay {
		t.Fatalf("default no-delay = %v, want true", defaults.NoDelay)
	}
	if defaults.KeepAlive == nil || !*defaults.KeepAlive {
		t.Fatalf("default keepalive = %v, want true", defaults.KeepAlive)
	}
	if defaults.WriteBatchBytes != 4096 {
		t.Fatalf("default write batch = %d, want 4096", defaults.WriteBatchBytes)
	}
	for name, config := range map[string]SocketConfig{
		"connect timeout": {ConnectTimeout: -time.Millisecond},
		"read timeout":    {ReadTimeout: -time.Millisecond},
		"write timeout":   {WriteTimeout: -time.Millisecond},
		"read buffer":     {ReadBufferBytes: -1},
		"write buffer":    {WriteBufferBytes: -1},
		"write batch":     {WriteBatchBytes: -1},
	} {
		t.Run(name, func(t *testing.T) {
			if err := config.validate(); err == nil {
				t.Fatal("invalid configuration unexpectedly succeeded")
			}
		})
	}
}

func TestSocketConfigExplicitNoDelayOverride(t *testing.T) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("loopback listeners are forbidden by this sandbox")
		}
		t.Fatal(err)
	}
	defer listener.close()
	accepted := make(chan *streamValue, 1)
	acceptErr := make(chan error, 1)
	go func() {
		stream, err := listener.accept(1000)
		accepted <- stream
		acceptErr <- err
	}()
	config := DefaultSocketConfig()
	config.NoDelay = socketBool(false)
	client, err := tcpDialWithConfig(context.Background(), listener.address(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	server := <-accepted
	if err := <-acceptErr; err != nil || server == nil {
		t.Fatalf("accept = %v, %v", server, err)
	}
	defer server.close()
	if client.conn == nil || client.mode != "tcp" {
		t.Fatalf("configured client = %#v", client)
	}
}

func TestSocketConfigBulkBatchAppliesToBothEndpoints(t *testing.T) {
	listener, err := tcpListen("127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("loopback listeners are forbidden by this sandbox")
		}
		t.Fatal(err)
	}
	defer listener.close()

	config := DefaultSocketConfig()
	config.NoDelay = socketBool(false)
	config.WriteBatchBytes = 64 << 10
	accepted := make(chan *streamValue, 1)
	acceptErr := make(chan error, 1)
	go func() {
		stream, err := listener.acceptContextWithConfig(context.Background(), 1000, config)
		accepted <- stream
		acceptErr <- err
	}()
	client, err := tcpDialWithConfig(context.Background(), listener.address(), config)
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	server := <-accepted
	if err := <-acceptErr; err != nil || server == nil {
		t.Fatalf("accept = %v, %v", server, err)
	}
	defer server.close()

	for _, endpoint := range []struct {
		name   string
		stream *streamValue
	}{
		{name: "client", stream: client},
		{name: "server", stream: server},
	} {
		name, stream := endpoint.name, endpoint.stream
		if stream.writer == nil {
			t.Fatalf("%s has no buffered writer", name)
		}
		if got := stream.writer.Size(); got != config.WriteBatchBytes {
			t.Fatalf("%s write batch = %d, want %d", name, got, config.WriteBatchBytes)
		}
	}
}
