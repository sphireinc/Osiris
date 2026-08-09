package osiris

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func runNativeHostResourceProgram(t *testing.T, source string, capabilities ...string) string {
	t.Helper()
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("nativeHost-native-resource.osr", source)
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: capabilities}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-resource")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	output, err := exec.Command(executable).CombinedOutput()
	if err != nil {
		t.Fatalf("native executable: %v (%s)", err, output)
	}
	return string(output)
}

func TestNativeHostTCPResourceExecutes(t *testing.T) {
	got := runNativeHostResourceProgram(t, `import "std/net" as net
import "std/os" as os
let listener = net.tcp_listen("127.0.0.1:0")
emit os.close(listener)
`, "net.tcp_listen", "net.close", "os.file_read")
	if !strings.Contains(got, "true") {
		t.Fatalf("native TCP close output = %q, want true", got)
	}
}

func TestNativeHostUDPDNSResourcesExecute(t *testing.T) {
	got := runNativeHostResourceProgram(t, `import "std/net" as net
import "std/os" as os
let socket = net.udp_bind("127.0.0.1:0")
emit len(net.dns_lookup("localhost")) > 0
emit os.close(socket)
`, "net.udp_bind", "net.dns_lookup", "net.close", "os.file_read")
	if strings.Count(got, "true") < 2 {
		t.Fatalf("native UDP/DNS output = %q, want true predicate and successful close", got)
	}
}

func TestNativeHostProcessResourceExecutes(t *testing.T) {
	got := runNativeHostResourceProgram(t, `import "std/os" as os
let options = map_new("cwd", "/tmp", "env", map_new("PATH", "/usr/bin:/bin"), "inherit_env", false)
let process = os.process_spawn("/bin/echo", ["native"], options)
let status = os.process_wait(process.process, 1000)
emit status
emit os.process_release(process.process)
`, "os.process_spawn", "os.process", "os.process_inspect")
	if !strings.Contains(got, "0i64") && !strings.Contains(got, "0") {
		t.Fatalf("native process output = %q, want successful exit status", got)
	}
}

func TestNativeHostFileResourceExecutes(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "native-resource.bin")
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/os" as os
let file = os.create(%q)
emit os.write(file, [65u8, 66u8, 67u8])
emit os.write_at(file, [90u8], 1)
emit os.seek_file(file, 0, 0)
emit os.read(file, 3)
emit os.read_at(file, 3, 0)
emit os.sync(file)
emit os.close(file)
emit os.stat(%q).size
emit os.read_dir(%q)
`, path, path, directory), "os.file_read", "os.file_write", "os.file_metadata", "os.directory_read")
	if !strings.Contains(got, "3u64") || !strings.Contains(got, "[65u8, 90u8, 67u8]") || !strings.Contains(got, "3i64") || !strings.Contains(got, "native-resource.bin") || !strings.Contains(got, "true") {
		t.Fatalf("native file resource output = %q, want write/read/stat/dir/close results", got)
	}
}

func TestNativeHostRootResourceExecutesBoundedPOSIXSubset(t *testing.T) {
	directory := t.TempDir()
	if err := os.Symlink("created.txt", filepath.Join(directory, "created-link.txt")); err != nil {
		t.Fatal(err)
	}
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/os" as os
let root = os.open_root(%q)
let created = os.root_create(root, "created.txt")
emit os.write(created, [65u8, 66u8, 67u8])
emit os.close(created)
let reopened = os.root_open(root, "created.txt")
emit os.read(reopened, 3)
emit os.close(reopened)
let opened = os.root_open_file(root, "opened.txt", 514, 420)
emit os.write(opened, [88u8])
emit os.close(opened)
emit os.root_write_file(root, "payload.txt", [80u8, 81u8], 420)
emit os.root_read_file(root, "payload.txt")
emit os.root_stat(root, "payload.txt").size
emit os.root_lstat(root, "payload.txt").dir
	emit os.root_readlink(root, "created-link.txt")
	emit os.root_read_file(root, "created-link.txt")
emit os.root_mkdir(root, "nested", 493)
emit os.root_mkdir_all(root, "deep/a", 493)
emit os.root_symlink(root, "created.txt", "nested/created-link.txt")
emit os.root_link(root, "created.txt", "nested/hard-link.txt")
emit os.root_readlink(root, "nested/created-link.txt")
emit os.root_rename(root, "payload.txt", "nested/renamed.txt")
emit os.root_chmod(root, "nested/renamed.txt", 384)
emit os.root_stat(root, "nested/renamed.txt").size
emit os.root_remove_all(root, "deep")
emit os.root_remove_all(root, "nested")
emit os.root_close(root)
`, directory), "os.file_read", "os.file_write", "os.file_metadata", "os.directory_mutate", "os.file_mutate", "os.symlink")
	if !strings.Contains(got, "[80u8, 81u8]") || strings.Count(got, "true") < 8 {
		t.Fatalf("native root resource output = %q, want bounded root file/stat/mutation results", got)
	}
}

func TestNativeHostRootedResourceExecutes(t *testing.T) {
	directory := t.TempDir()
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/os" as os
let root = os.open_root(%q)
emit os.root_write_file(root, "one.txt", [65u8, 66u8], 420)
emit os.root_read_file(root, "one.txt")
emit os.root_stat(root, "one.txt").size
emit os.root_mkdir(root, "nested", 493)
emit os.root_rename(root, "one.txt", "nested/moved.txt")
emit os.root_read_file(root, "nested/moved.txt")
emit os.root_remove(root, "nested/moved.txt")
emit os.root_remove(root, "nested")
emit os.root_close(root)
`, directory), "os.file_read", "os.file_write", "os.file_metadata", "os.directory_mutate", "os.file_mutate")
	if !strings.Contains(got, "[65u8, 66u8]") || !strings.Contains(got, "2i64") || strings.Count(got, "true") < 5 {
		t.Fatalf("native rooted resource output = %q, want confined read/write/stat/mutation/close results", got)
	}
}

func TestNativeHostRootOwnershipTimestampAndFSBridgeExecute(t *testing.T) {
	directory := t.TempDir()
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/os" as os
let root = os.open_root(%q)
let fs = os.root_fs(root)
emit os.root_write_file(fs, "owned.txt", [65u8], 420)
emit os.root_chtimes(fs, "owned.txt", "2001-02-03T04:05:06.123456789Z", "2001-02-03T04:05:06.123456789Z")
emit os.root_stat(fs, "owned.txt").size
emit os.root_chown(fs, "owned.txt", -1, -1)
emit os.root_symlink(fs, "owned.txt", "owned-link.txt")
emit os.root_lchown(fs, "owned-link.txt", -1, -1)
emit os.root_read_file(fs, "owned-link.txt")
emit os.root_close(root)
`, directory), "os.file_read", "os.file_write", "os.file_metadata", "os.file_mutate", "os.directory_mutate", "os.symlink")
	if !strings.Contains(got, "1i64") || !strings.Contains(got, "[65u8]") || strings.Count(got, "true") < 5 {
		t.Fatalf("native root ownership/timestamp output = %q, want bridge, timestamp, ownership, and close results", got)
	}
	info, err := os.Stat(filepath.Join(directory, "owned.txt"))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := time.Parse(time.RFC3339Nano, "2001-02-03T04:05:06.123456789Z")
	if err != nil || info.ModTime().UTC().UnixNano() != expected.UnixNano() {
		t.Fatalf("native root timestamp = %s, want %s", info.ModTime().UTC().Format(time.RFC3339Nano), expected.Format(time.RFC3339Nano))
	}
}

func TestNativeHostRootRejectsSymlinkEscape(t *testing.T) {
	directory := t.TempDir()
	outside := t.TempDir()
	outsidePath := filepath.Join(outside, "outside.txt")
	if err := os.WriteFile(outsidePath, []byte("unchanged"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outsidePath, filepath.Join(directory, "escape.txt")); err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath("cc"); err != nil {
		t.Skip("cc not available")
	}
	program, err := Parse("nativeHost-native-root-symlink-escape.osr", fmt.Sprintf(`import "std/os" as os
let root = os.open_root(%q)
emit os.root_write_file(root, "escape.txt", [88u8], 420)
`, directory))
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.file_read", "os.file_write"}}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-root-symlink-escape")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command(executable).CombinedOutput(); err == nil {
		t.Fatalf("root symlink escape unexpectedly succeeded: %s", output)
	}
	data, err := os.ReadFile(outsidePath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "unchanged" {
		t.Fatalf("root symlink escape modified outside file: %q", data)
	}
}

func TestNativeHostHTTPExecutesWithPortableTransport(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.Header().Set("Content-Type", "text/plain")
		writer.WriteHeader(http.StatusCreated)
		_, _ = writer.Write([]byte("native-http"))
	}))
	defer server.Close()
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/net" as net
let response = net.http_get(%q)
emit response.status
emit response.body
`, server.URL), "net.http_client")
	if !strings.Contains(got, "201") || !strings.Contains(got, "native-http") {
		t.Fatalf("native HTTP output = %q, want status and body", got)
	}
}

func TestNativeHostHTTPChunkedAndStreamingResourcesExecute(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	defer listener.Close()
	served := make(chan error, 1)
	go func() {
		for i := 0; i < 2; i++ {
			connection, acceptErr := listener.Accept()
			if acceptErr != nil {
				served <- acceptErr
				return
			}
			request := make([]byte, 4096)
			_, _ = connection.Read(request)
			_, writeErr := connection.Write([]byte("HTTP/1.1 200 OK\r\nTransfer-Encoding: chunked\r\nConnection: close\r\n\r\n5\r\nchunk\r\n0\r\n\r\n"))
			connection.Close()
			if writeErr != nil {
				served <- writeErr
				return
			}
		}
		served <- nil
	}()
	address := listener.Addr().String()
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/net" as net
let response = net.http_get("http://%s/chunked")
emit response.body
let streamed = net.http_request_stream("GET", "http://%s/chunked")
emit readchunk(streamed.body_stream, 5, 1000)
emit close(streamed.body_stream)
`, address, address), "net.http_client", "net.http_stream")
	if !strings.Contains(got, "chunk") || !strings.Contains(got, "true") {
		t.Fatalf("native chunked HTTP output = %q, want decoded body and close result", got)
	}
	if err := <-served; err != nil {
		t.Fatal(err)
	}
}

func TestNativeHostTLSExecutesWithOpenSSLProvider(t *testing.T) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	template := &x509.Certificate{SerialNumber: big.NewInt(148), Subject: pkix.Name{CommonName: "localhost"}, DNSNames: []string{"localhost"}, NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(time.Hour), KeyUsage: x509.KeyUsageKeyEncipherment | x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, IsCA: true}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	certificatePEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	privatePEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	certificate, err := tls.X509KeyPair(certificatePEM, privatePEM)
	if err != nil {
		t.Fatal(err)
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		if strings.Contains(err.Error(), "operation not permitted") {
			t.Skip("network sockets are unavailable in this test sandbox")
		}
		t.Fatal(err)
	}
	defer listener.Close()
	accepted := make(chan error, 1)
	go func() {
		connection, acceptErr := listener.Accept()
		if acceptErr != nil {
			accepted <- acceptErr
			return
		}
		secure := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{certificate}, MinVersion: tls.VersionTLS12, NextProtos: []string{"http/1.1"}})
		if _, acceptErr = secure.Write([]byte("native-tls")); acceptErr == nil {
			acceptErr = secure.Close()
		}
		accepted <- acceptErr
	}()
	address := listener.Addr().String()
	got := runNativeHostResourceProgram(t, fmt.Sprintf(`import "std/net" as net
let secure = net.tls_dial_config(%q, "localhost", %q, 1000)
let bytes = readchunk(secure, 64, 1000)
emit bytes
emit close(secure)
	`, address, string(certificatePEM)), "net.tls_dial_config")
	if !strings.Contains(got, "110u8") || !strings.Contains(got, "true") {
		t.Fatalf("native TLS output = %q, want payload and close result", got)
	}
	if err := <-accepted; err != nil {
		t.Fatal(err)
	}
}

func TestNativeHostWatchExecutesWithBoundedSnapshotEvents(t *testing.T) {
	directory := t.TempDir()
	program, err := Parse("nativeHost-native-watch.osr", fmt.Sprintf(`import "std/os" as os
let watcher = os.watch(%q, false)
	let event = readchunk(watcher, 4096, 5000)
emit event
emit close(watcher)
`, directory))
	if err != nil {
		t.Fatal(err)
	}
	options := CCompileOptions{NativeCapabilities: []string{"os.watch"}}
	artifact, err := CompileCWithOptions(context.Background(), NewRuntime(), program, options)
	if err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(t.TempDir(), "nativeHost-native-watch")
	if err := CompileCExecutableWithOptions(context.Background(), artifact.Source, executable, options); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	command := exec.Command(executable)
	command.Stdout = &output
	command.Stderr = &output
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	time.Sleep(time.Second)
	if err := os.WriteFile(filepath.Join(directory, "created.txt"), []byte("watch"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := command.Wait(); err != nil {
		t.Fatalf("native watcher: %v (%s)", err, output.String())
	}
	if !strings.Contains(output.String(), `"kind":"create"`) || !strings.Contains(output.String(), "true") {
		t.Fatalf("native watcher output = %q, want create event and close result", output.String())
	}
}
