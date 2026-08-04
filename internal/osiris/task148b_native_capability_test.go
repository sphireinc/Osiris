package osiris

import (
	"context"
	"strings"
	"testing"
)

func compileTask148BResourceCapability(t *testing.T, source string, capabilities ...string) error {
	t.Helper()
	program, err := Parse("task148b-resource-capability.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = CompileCWithOptions(context.Background(), NewRuntime(), program, CCompileOptions{NativeCapabilities: capabilities})
	return err
}

func TestTask148BGenericResourceCapabilitiesFollowFileProducer(t *testing.T) {
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let input = os.open("/tmp/input")
emit readchunk(input, 1, 0)
emit close(input)
`, "os.file_read"); err != nil {
		t.Fatalf("file read/close with os.file_read = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let output = os.create("/tmp/output")
emit writechunk(output, "x", 0)
emit close(output)
`, "os.file_write"); err != nil {
		t.Fatalf("file write/close with os.file_write = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let output = os.open_file("/tmp/output", 1, 420)
emit writechunk(output, "x", 0)
`, "os.file_read"); err == nil || !strings.Contains(err.Error(), `native capability "os.file_write"`) || !strings.Contains(err.Error(), `producer "std/os.open_file"`) {
		t.Fatalf("open_file write error = %v, want producer-specific os.file_write denial", err)
	}
}

func TestTask148BGenericResourceCapabilitiesFollowHostProducers(t *testing.T) {
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let watcher = os.watch("/tmp", false)
emit readchunk(watcher, 1, 0)
emit close(watcher)
`, "os.watch"); err != nil {
		t.Fatalf("watch read/close with os.watch = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let subscription = os.signal_subscribe("SIGUSR1")
emit readchunk(subscription, 1, 0)
emit close(subscription)
`, "os.signal"); err != nil {
		t.Fatalf("signal read/close with os.signal = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/os" as os
let launched = os.process_spawn("/bin/echo", ["native"], null)
let process = launched.process
emit close(process)
`, "os.process_spawn", "os.process"); err != nil {
		t.Fatalf("process close with os.process = %v", err)
	}
}

func TestTask148BGenericResourceCapabilitiesFollowNetworkProducers(t *testing.T) {
	if err := compileTask148BResourceCapability(t, `import "std/net" as net
let socket = net.tcp_dial("127.0.0.1:1")
emit readchunk(socket, 1, 0)
emit close(socket)
`, "net.tcp_dial", "net.tcp_read", "net.close"); err != nil {
		t.Fatalf("socket read/close = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/net" as net
let secure = net.tls_dial("127.0.0.1:1", "localhost", 0)
emit readchunk(secure, 1, 0)
emit close(secure)
`, "net.tls_dial"); err != nil {
		t.Fatalf("TLS read/close with producer capability = %v", err)
	}
	if err := compileTask148BResourceCapability(t, `import "std/net" as net
let response = net.http_request_stream("GET", "http://127.0.0.1:1")
emit readchunk(response.body_stream, 1, 0)
emit close(response.body_stream)
`, "net.http_stream"); err != nil {
		t.Fatalf("HTTP body read/close with net.http_stream = %v", err)
	}
}

func TestTask148BGenericResourceCapabilityDenialIsProducerSpecific(t *testing.T) {
	err := compileTask148BResourceCapability(t, `import "std/os" as os
let input = os.open("/tmp/input")
emit readchunk(input, 1, 0)
`, "os.file_read", "net.tcp_read")
	if err != nil {
		t.Fatalf("file read incorrectly depends on net.tcp_read: %v", err)
	}

	err = compileTask148BResourceCapability(t, `func consume(resource) => close(resource)
`, "net.close")
	if err == nil || !strings.Contains(err.Error(), "resource producer") || !strings.Contains(err.Error(), "cannot be proven statically") {
		t.Fatalf("unknown producer error = %v, want explicit producer diagnostic", err)
	}
}
